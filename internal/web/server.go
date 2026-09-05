// Package web sert une interface graphique locale au-dessus des mêmes paquets
// que l'assistant du terminal : un petit serveur sur la boucle locale, une API
// JSON, et une page embarquée dans le binaire.
//
// Le serveur n'écoute que sur 127.0.0.1, exige un jeton tiré au hasard à chaque
// lancement, et vérifie l'origine de toute requête qui écrit. Le jeton GitHub
// ne quitte jamais le processus : le navigateur ne parle qu'à cette API.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

//go:embed assets
var assets embed.FS

// MaxBody borne la taille d'un corps de requête : une liste collée reste
// modeste, et rien ne justifie d'avaler davantage.
const MaxBody = 8 << 20

// Deps rassemble ce que la session du terminal transmet à l'interface.
type Deps struct {
	Client     *ghapi.Client
	Cache      *cache.Cache
	Settings   config.Settings
	ConfigFile string
	Viewer     string
	Host       string
	Version    string
	ReportDir  string
	Jobs       int
	Depth      int
	SaveConfig bool
}

// Server est l'interface web d'une session.
type Server struct {
	deps       Deps
	jobs       *Jobs
	classrooms *classroom.Store
	listener   net.Listener
	token      string
	port       string
	handler    http.Handler
	stop       chan struct{}
	stopOnce   sync.Once

	mutex     sync.Mutex
	settings  config.Settings
	inventory map[string][]groups.RepoInfo  // organisation → dépôts connus
	resolvers map[string]*identity.Resolver // organisation → noms complets
}

// New prépare le serveur et réserve son port sur la boucle locale.
func New(deps Deps) (*Server, error) {
	if deps.Jobs < 1 {
		deps.Jobs = 4
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		listener.Close()
		return nil, err
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		listener.Close()
		return nil, err
	}

	server := &Server{
		deps:       deps,
		jobs:       NewJobs(),
		listener:   listener,
		token:      token,
		port:       port,
		classrooms: classroom.Open(classroom.PathNextTo(deps.ConfigFile)),
		stop:       make(chan struct{}),
		settings:   deps.Settings,
		inventory:  map[string][]groups.RepoInfo{},
		resolvers:  map[string]*identity.Resolver{},
	}
	server.handler = server.guard(server.routes())
	return server, nil
}

// URL est l'adresse à ouvrir, jeton compris. Elle n'est affichée qu'une fois.
func (s *Server) URL() string {
	return "http://127.0.0.1:" + s.port + "/?" + tokenParam + "=" + s.token
}

// Address est l'adresse sans jeton, pour les messages du terminal.
func (s *Server) Address() string { return "http://127.0.0.1:" + s.port }

// Settings renvoie les réglages tels que l'interface les a laissés : la session
// du terminal les mémorise en quittant.
func (s *Server) Settings() config.Settings {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.settings
}

// Serve tient l'interface jusqu'à l'annulation du contexte ou la fermeture
// demandée depuis le navigateur.
func (s *Server) Serve(lifetime context.Context) error {
	server := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	failures := make(chan error, 1)
	go func() {
		err := server.Serve(s.listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		failures <- err
	}()

	select {
	case err := <-failures:
		return err
	case <-lifetime.Done():
	case <-s.stop:
	}

	// Les flux d'événements restent ouverts : la fermeture est bornée.
	shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
	return nil
}

// Close libère le port sans avoir servi (erreur au démarrage).
func (s *Server) Close() error { return s.listener.Close() }

// routes déclare l'API et la page.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// --- contexte et réglages
	mux.HandleFunc("GET /api/context", s.handleContext)
	mux.HandleFunc("PUT /api/settings", s.handleSaveSettings)
	mux.HandleFunc("POST /api/cache/clear", s.handleClearCache)
	mux.HandleFunc("POST /api/quit", s.handleQuit)

	// --- organisations et inventaire
	mux.HandleFunc("GET /api/orgs", s.handleOrgs)
	mux.HandleFunc("GET /api/orgs/{org}", s.handleOrg)

	// --- groupes
	// Un groupe se désigne par sa place — « a26.5n6.1010 » —, celle-là même qui
	// est écrite dans le nom de chacun de ses dépôts.
	mux.HandleFunc("GET /api/classrooms", s.handleClassrooms)
	mux.HandleFunc("POST /api/classrooms", s.handleCreateClassroom)
	mux.HandleFunc("GET /api/classrooms/{scope}", s.handleClassroom)
	mux.HandleFunc("PUT /api/classrooms/{scope}", s.handleUpdateClassroom)
	mux.HandleFunc("DELETE /api/classrooms/{scope}", s.handleForgetClassroom)
	mux.HandleFunc("GET /api/classrooms/{scope}/students", s.handleClassroomStudents)
	mux.HandleFunc("POST /api/classrooms/{scope}/students", s.handleSetStudents)
	mux.HandleFunc("POST /api/classrooms/{scope}/students/add", s.handleAddStudent)
	mux.HandleFunc("POST /api/classrooms/{scope}/students/names", s.handleResolveStudentNames)
	mux.HandleFunc("POST /api/classrooms/{scope}/students/move", s.handleMoveStudent)
	mux.HandleFunc("POST /api/classrooms/{scope}/students/rename", s.handleRenameStudent)
	mux.HandleFunc("POST /api/classrooms/{scope}/assignments", s.handleCreateAssignment)
	mux.HandleFunc("POST /api/classrooms/{scope}/assignments/preview", s.handlePreviewAssignment)
	mux.HandleFunc("POST /api/classrooms/{scope}/assignments/move/preview", s.handleRelocatePreview)
	mux.HandleFunc("POST /api/classrooms/{scope}/assignments/move", s.handleRelocate)
	mux.HandleFunc("POST /api/classrooms/{scope}/assignments/rename/preview", s.handleRenameAssignmentPreview)
	mux.HandleFunc("POST /api/classrooms/{scope}/assignments/rename", s.handleRenameAssignment)
	mux.HandleFunc("GET /api/classrooms/{scope}/assignments/{name}", s.handleAssignment)
	mux.HandleFunc("POST /api/classrooms/{scope}/assignments/{name}/access", s.handleAssignmentAccess)
	mux.HandleFunc("POST /api/classrooms/{scope}/migration/preview", s.handleMigrationPreview)
	mux.HandleFunc("POST /api/classrooms/{scope}/migration/apply", s.handleMigrationApply)
	mux.HandleFunc("GET /api/orgs/{org}/candidates", s.handleCandidates)
	mux.HandleFunc("POST /api/orgs/{org}/match", s.handleMatchPattern)

	// --- listes et code de départ
	mux.HandleFunc("POST /api/roster/parse", s.handleParseRoster)
	mux.HandleFunc("POST /api/roster/load", s.handleLoadRoster)
	mux.HandleFunc("POST /api/roster/save", s.handleSaveRoster)
	mux.HandleFunc("POST /api/template/check", s.handleCheckTemplate)
	mux.HandleFunc("POST /api/starter/inspect", s.handleInspectStarter)
	mux.HandleFunc("POST /api/accounts/verify", s.handleVerifyAccounts)

	// --- accès et dépôts
	mux.HandleFunc("GET /api/orgs/{org}/repos/{repo}/access", s.handleAccess)
	mux.HandleFunc("POST /api/orgs/{org}/repos/{repo}/collaborators", s.handleAddCollaborator)
	mux.HandleFunc("DELETE /api/orgs/{org}/repos/{repo}/collaborators/{login}", s.handleRemoveCollaborator)
	mux.HandleFunc("DELETE /api/orgs/{org}/repos/{repo}/invitations/{id}", s.handleCancelInvitation)
	mux.HandleFunc("DELETE /api/orgs/{org}/repos/{repo}", s.handleDeleteRepo)

	// --- clones
	mux.HandleFunc("POST /api/clones/find", s.handleFindClones)
	mux.HandleFunc("POST /api/clones/clone", s.handleClone)
	mux.HandleFunc("POST /api/clones/pull", s.handlePull)
	mux.HandleFunc("POST /api/paths/suggest", s.handleSuggestPath)
	mux.HandleFunc("POST /api/paths/pick", s.handlePickPath)
	mux.HandleFunc("POST /api/paths/browse", s.handleBrowsePath)

	// --- travaux en arrière-plan
	mux.HandleFunc("GET /api/jobs/{id}", s.handleJob)
	mux.HandleFunc("GET /api/jobs/{id}/events", s.handleJobEvents)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.handleCancelJob)

	mux.Handle("/", s.page())
	return mux
}

// page sert les fichiers embarqués ; toute autre adresse renvoie l'interface.
func (s *Server) page() http.Handler {
	files, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	server := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/") {
			http.NotFound(writer, request)
			return
		}
		if request.URL.Path != "/" {
			if _, err := fs.Stat(files, strings.TrimPrefix(request.URL.Path, "/")); err != nil {
				request = request.Clone(request.Context())
				request.URL.Path = "/"
			}
		}
		server.ServeHTTP(writer, request)
	})
}

// handleQuit referme l'interface à la demande du navigateur.
func (s *Server) handleQuit(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "fermeture"})
	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	}
	s.stopOnce.Do(func() { close(s.stop) })
}

// ------------------------------------------------------------------ échanges

// writeJSON écrit une réponse JSON.
func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(writer).Encode(payload)
}

// decode lit un corps JSON, en bornant sa taille.
func decode(request *http.Request, target any) error {
	body := http.MaxBytesReader(nil, request.Body, MaxBody)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return valid.Errorf("Requête vide.")
		}
		return valid.Errorf("Requête illisible : %v", err)
	}
	return nil
}

// fail traduit une erreur du domaine en réponse HTTP.
func fail(writer http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case valid.IsValidation(err):
		status = http.StatusBadRequest
	case ghapi.IsGitHub(err):
		// Le code de GitHub est repris tel quel s'il décrit le problème.
		switch code := ghapi.StatusOf(err); code {
		case 401, 403, 404, 422:
			status = code
		default:
			status = http.StatusBadGateway
		}
	}
	writeJSON(writer, status, map[string]string{"error": err.Error()})
}
