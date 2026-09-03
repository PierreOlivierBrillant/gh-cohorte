package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/orgs"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// ------------------------------------------------------------------- contexte

// choice est une valeur proposée dans un menu de l'interface.
type choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// pathInfo décrit un fichier géré par l'outil.
type pathInfo struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	State string `json:"state"`
}

// contextPayload donne à la page tout ce qu'elle doit savoir au chargement.
type contextPayload struct {
	Viewer       string            `json:"viewer"`
	Host         string            `json:"host"`
	Version      string            `json:"version"`
	Settings     config.Settings   `json:"settings"`
	Scopes       map[string]string `json:"scopes"`
	Paths        []pathInfo        `json:"paths"`
	Permissions  []choice          `json:"permissions"`
	Placeholders []string          `json:"placeholders"`
	SaveConfig   bool              `json:"save_config"`
	Jobs         int               `json:"jobs"`
	Depth        int               `json:"depth"`
}

// handleContext décrit la session en cours.
func (s *Server) handleContext(writer http.ResponseWriter, _ *http.Request) {
	permissions := make([]choice, 0, len(config.Permissions))
	for _, value := range config.Permissions {
		permissions = append(permissions, choice{Value: value, Label: config.PermissionLabels[value]})
	}
	scopes := map[string]string{}
	for _, scope := range []string{"repo", "read:org", "workflow", "delete_repo"} {
		scopes[scope] = describeScope(s.deps.Client.HasScope(scope))
	}

	writeJSON(writer, http.StatusOK, contextPayload{
		Viewer:       s.deps.Viewer,
		Host:         s.deps.Host,
		Version:      s.deps.Version,
		Settings:     s.Settings(),
		Scopes:       scopes,
		Paths:        s.paths(),
		Permissions:  permissions,
		Placeholders: plan.Placeholders,
		SaveConfig:   s.deps.SaveConfig,
		Jobs:         s.deps.Jobs,
		Depth:        s.deps.Depth,
	})
}

// describeScope met en mots ce que le jeton annonce. Un jeton « fine-grained »
// n'annonce aucune portée : la réponse est alors « inconnue ».
func describeScope(present, known bool) string {
	switch {
	case !known:
		return "inconnue"
	case present:
		return "présente"
	default:
		return "absente"
	}
}

// paths renvoie l'emplacement et l'état des fichiers gérés par l'outil.
func (s *Server) paths() []pathInfo {
	settingsState := "absent"
	if info, err := os.Stat(s.deps.ConfigFile); err == nil && !info.IsDir() {
		settingsState = "présent"
	}
	return []pathInfo{
		{"Réglages", s.deps.ConfigFile, settingsState},
		{"Cache", s.deps.Cache.Path(), s.deps.Cache.Describe()},
		{"Bilans", s.reportDir(), countReports(s.reportDir())},
	}
}

// handleSaveSettings retient les réglages et les écrit sur le disque.
func (s *Server) handleSaveSettings(writer http.ResponseWriter, request *http.Request) {
	var incoming config.Settings
	if err := decode(request, &incoming); err != nil {
		fail(writer, err)
		return
	}
	incoming, err := sanitize(incoming)
	if err != nil {
		fail(writer, err)
		return
	}
	s.mutex.Lock()
	s.settings = incoming
	s.mutex.Unlock()

	if !s.deps.SaveConfig {
		writeJSON(writer, http.StatusOK, map[string]any{"saved": false})
		return
	}
	if err := incoming.Save(s.deps.ConfigFile); err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"saved": true, "path": s.deps.ConfigFile})
}

// handleClearCache vide le cache local et oublie l'inventaire en mémoire.
func (s *Server) handleClearCache(writer http.ResponseWriter, _ *http.Request) {
	removed := s.deps.Cache.Clear()
	s.mutex.Lock()
	s.inventory = map[string][]groups.RepoInfo{}
	s.resolvers = map[string]*identity.Resolver{}
	s.mutex.Unlock()
	writeJSON(writer, http.StatusOK, map[string]any{
		"removed": removed,
		"paths":   s.paths(),
	})
}

// -------------------------------------------------------------- organisations

// handleOrgs liste les organisations du compte connecté.
func (s *Server) handleOrgs(writer http.ResponseWriter, _ *http.Request) {
	accesses, err := orgs.List(s.deps.Client, s.deps.Cache, s.deps.Viewer, s.deps.Jobs, nil)
	if err != nil {
		// Sans « read:org », GitHub ne révèle pas les adhésions : l'interface
		// se rabat alors sur la saisie libre d'un nom.
		writeJSON(writer, http.StatusOK, map[string]any{
			"orgs":   []orgs.Access{},
			"notice": "Liste des organisations indisponible (" + err.Error() + "). Saisissez un nom.",
		})
		return
	}
	labelled := make([]map[string]any, 0, len(accesses))
	for _, access := range accesses {
		labelled = append(labelled, map[string]any{
			"login": access.Login, "name": access.Name, "role": access.Role,
			"can_create": access.CanCreate, "known": access.Known, "label": access.Label(),
		})
	}
	writeJSON(writer, http.StatusOK, map[string]any{"orgs": labelled})
}

// handleOrg vérifie une organisation et signale un rôle insuffisant.
func (s *Server) handleOrg(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	data, err := s.deps.Client.GetOrg(org)
	if err != nil {
		fail(writer, err)
		return
	}
	if data == nil {
		fail(writer, valid.Errorf("L'organisation « %s » est introuvable ou invisible pour @%s.",
			org, s.deps.Viewer))
		return
	}
	name := data.Name
	if name == "" {
		name = org
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"login": org, "name": name, "warning": s.roleWarning(org),
	})
}

// roleWarning dit, sans bloquer, si la création de dépôts risque d'échouer.
func (s *Server) roleWarning(org string) string {
	accesses, err := orgs.List(s.deps.Client, s.deps.Cache, s.deps.Viewer, s.deps.Jobs, nil)
	if err == nil {
		if access, found := orgs.Find(accesses, org); found {
			switch {
			case access.Role == "admin" || access.CanCreate:
				return ""
			case access.Known:
				return fmt.Sprintf("Vous êtes « %s » et la création de dépôts est réservée "+
					"aux propriétaires de cette organisation.", access.Role)
			default:
				return fmt.Sprintf("Vous êtes « %s » : la création de dépôts doit être autorisée "+
					"aux membres dans les réglages de l'organisation.", access.Role)
			}
		}
	}
	role, err := s.deps.Client.OrgMembership(org, s.deps.Viewer)
	switch {
	case err != nil:
		return ""
	case role == "":
		return "Rôle indéterminé dans l'organisation (portée « read:org » absente ?) : " +
			"la création peut échouer."
	case role == "admin":
		return ""
	default:
		return fmt.Sprintf("Vous êtes « %s » : la création de dépôts doit être autorisée "+
			"aux membres dans les réglages de l'organisation.", role)
	}
}

// ---------------------------------------------------------------- inventaire

// repos charge les dépôts de l'organisation : mémoire, puis cache, puis API.
func (s *Server) repos(org string, force bool) ([]groups.RepoInfo, string, error) {
	if !force {
		s.mutex.Lock()
		known, found := s.inventory[org]
		s.mutex.Unlock()
		if found {
			return known, "mémoire", nil
		}
		var cached []groups.RepoInfo
		if s.deps.Cache.Get(cache.ReposKey(org), cache.ReposTTL, &cached) && len(cached) > 0 {
			s.remember(org, cached)
			return cached, "cache (" + s.deps.Cache.Describe() + ")", nil
		}
	}

	fetched, err := s.deps.Client.ListOrgRepos(org, nil)
	if err != nil {
		return nil, "", err
	}
	s.deps.Cache.Set(cache.ReposKey(org), fetched)
	s.remember(org, fetched)
	return fetched, "GitHub", nil
}

// remember retient l'inventaire d'une organisation pour la suite de la session.
func (s *Server) remember(org string, repos []groups.RepoInfo) {
	s.mutex.Lock()
	s.inventory[org] = repos
	s.mutex.Unlock()
}

// forget oublie l'inventaire d'une organisation, après une écriture.
func (s *Server) forget(org string) {
	s.mutex.Lock()
	delete(s.inventory, org)
	s.mutex.Unlock()
	s.deps.Cache.Forget(cache.ReposKey(org))
}

// resolver retrouve, par organisation, le service qui nomme les personnes.
func (s *Server) resolver(org string) *identity.Resolver {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if existing, found := s.resolvers[org]; found {
		return existing
	}
	fresh := identity.New(s.deps.Client, s.deps.Cache, s.reportDir(), s.deps.Jobs)
	s.resolvers[org] = fresh
	return fresh
}

// handleGroups détecte les groupes de dépôts de l'organisation.
func (s *Server) handleGroups(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	repos, source, err := s.repos(org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}
	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		names = append(names, repo.Name)
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"total":  len(repos),
		"source": source,
		"groups": groups.Detect(names, 2),
	})
}

// groupRepo est un dépôt du groupe, tel que l'affiche l'interface.
type groupRepo struct {
	Name       string `json:"name"`
	Suffix     string `json:"suffix"`
	FullName   string `json:"full_name"`
	Private    bool   `json:"private"`
	Visibility string `json:"visibility"`
	URL        string `json:"url"`
	PushedAt   string `json:"pushed_at"`
}

// handleGroup ouvre un groupe. Les noms complets déjà connus localement sont
// joints ; les autres se demandent à part, l'API GitHub étant sollicitée une
// fois par personne.
func (s *Server) handleGroup(writer http.ResponseWriter, request *http.Request) {
	org, group, err := s.group(request)
	if err != nil {
		fail(writer, err)
		return
	}
	known := s.resolver(org).Resolve(pairsOf(group), false, nil)

	missing := 0
	rows := make([]groupRepo, 0, group.Len())
	for _, repo := range group.Repos {
		if known[repo.Name] == "" {
			missing++
		}
		rows = append(rows, groupRepo{
			Name: repo.Name, Suffix: repo.Suffix, FullName: known[repo.Name],
			Private: repo.Private, Visibility: repo.Visibility(),
			URL: s.urlOf(org, repo), PushedAt: repo.PushedAt,
		})
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"prefix": group.Prefix, "repos": rows, "missing_names": missing,
	})
}

// handleGroupNames lance la résolution des noms complets manquants.
func (s *Server) handleGroupNames(writer http.ResponseWriter, request *http.Request) {
	org, group, err := s.group(request)
	if err != nil {
		fail(writer, err)
		return
	}
	pairs := pairsOf(group)
	resolver := s.resolver(org)
	total := len(resolver.Missing(pairs))

	job := s.jobs.Start("noms", "Noms complets de « "+group.Prefix+" »", func(job *Job) (any, error) {
		names := resolver.Resolve(pairs, true, func(done, _ int, repo string) {
			job.Progress(done, total, repo)
		})
		return names, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// handleGroupTemplate retrouve le dépôt modèle utilisé par le groupe.
func (s *Server) handleGroupTemplate(writer http.ResponseWriter, request *http.Request) {
	org, group, err := s.group(request)
	if err != nil {
		fail(writer, err)
		return
	}
	template := ""
	if group.Len() > 0 {
		data, err := s.deps.Client.GetRepo(org, group.Repos[0].Name)
		if err == nil && data != nil && data.TemplateRepository != nil {
			template = data.TemplateRepository.FullName
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{"template": template})
}

// group résout l'organisation et le groupe désignés par l'adresse.
func (s *Server) group(request *http.Request) (string, *groups.Group, error) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		return "", nil, err
	}
	prefix, err := valid.SlugFragment(request.PathValue("prefix"), "Préfixe")
	if err != nil {
		return "", nil, err
	}
	repos, _, err := s.repos(org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		return "", nil, err
	}
	group := groups.Build(prefix, repos)
	if group.Len() == 0 {
		return "", nil, valid.Errorf("Aucun dépôt ne commence par « %s- » dans « %s ».", prefix, org)
	}
	return org, &group, nil
}

// pairsOf associe chaque dépôt au compte GitHub qu'il concerne.
func pairsOf(group *groups.Group) []identity.Pair {
	pairs := make([]identity.Pair, 0, group.Len())
	for _, repo := range group.Repos {
		pairs = append(pairs, identity.Pair{Repo: repo.Name, Login: repo.Suffix})
	}
	return pairs
}

// urlOf reconstitue l'adresse d'un dépôt quand l'API ne l'a pas donnée.
func (s *Server) urlOf(org string, repo groups.Repo) string {
	if repo.URL != "" {
		return repo.URL
	}
	host := s.deps.Host
	if host == "" {
		host = "github.com"
	}
	return "https://" + host + "/" + org + "/" + repo.Name
}

// reportDir renvoie le dossier des bilans, chemin développé.
func (s *Server) reportDir() string {
	expanded, err := roster.ExpandPath(s.deps.ReportDir)
	if err != nil {
		return s.deps.ReportDir
	}
	return expanded
}

// --------------------------------------------------------------------- travaux

// handleJob renvoie l'état d'un travail.
func (s *Server) handleJob(writer http.ResponseWriter, request *http.Request) {
	job, found := s.jobs.Get(request.PathValue("id"))
	if !found {
		fail(writer, valid.Errorf("Travail inconnu."))
		return
	}
	writeJSON(writer, http.StatusOK, job.State())
}

// handleCancelJob demande l'arrêt d'un travail.
func (s *Server) handleCancelJob(writer http.ResponseWriter, request *http.Request) {
	if !s.jobs.Cancel(request.PathValue("id")) {
		fail(writer, valid.Errorf("Travail inconnu."))
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": JobCanceled})
}

// handleJobEvents diffuse le déroulement d'un travail au fil de l'eau.
func (s *Server) handleJobEvents(writer http.ResponseWriter, request *http.Request) {
	job, found := s.jobs.Get(request.PathValue("id"))
	if !found {
		fail(writer, valid.Errorf("Travail inconnu."))
		return
	}
	flusher, streamable := writer.(http.Flusher)
	if !streamable {
		fail(writer, valid.Errorf("Diffusion impossible."))
		return
	}

	writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	writer.Header().Set("Connection", "keep-alive")
	writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Une reprise de connexion repart du dernier événement reçu.
	seq := 0
	if from := request.URL.Query().Get("from"); from != "" {
		seq, _ = strconv.Atoi(from)
	}
	for {
		events, finished, changed := job.since(seq)
		for _, event := range events {
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(writer, "data: %s\n\n", payload)
			seq = event.Seq
		}
		if len(events) > 0 {
			flusher.Flush()
		}
		if finished && len(events) == 0 {
			return
		}
		select {
		case <-changed:
		case <-request.Context().Done():
			return
		case <-time.After(20 * time.Second):
			// Un commentaire garde la connexion vivante à travers un proxy local.
			fmt.Fprint(writer, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// countReports résume ce que contient le dossier des bilans.
func countReports(directory string) string {
	matches, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil || len(matches) == 0 {
		return "aucun bilan"
	}
	return strconv.Itoa(len(matches)) + " bilan(s)"
}

// itoa met un nombre sous forme de texte.
func itoa(value int) string { return strconv.Itoa(value) }
