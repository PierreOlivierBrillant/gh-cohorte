// Package fakegh monte un serveur HTTP local imitant les points de l'API GitHub
// utilisés par l'outil. Il permet de jouer des parcours complets sans réseau.
package fakegh

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Failure décrit une réponse d'erreur injectée volontairement.
type Failure struct {
	Status  int
	Message string
}

// RepoState est un dépôt tel que le faux serveur le connaît.
type RepoState struct {
	Name          string
	Org           string
	Private       bool
	Description   string
	DefaultBranch string
	IsTemplate    bool
	Template      string // « owner/repo » du modèle utilisé à la création
	PushedAt      string
	URLOverride   string // adresse renvoyée à la place de github.com (tests de clonage)
}

// FullName renvoie « organisation/depot ».
func (r *RepoState) FullName() string { return r.Org + "/" + r.Name }

type commit struct {
	Tree    string
	Parents []string
	Message string
}

// State est l'état partagé du faux serveur.
type State struct {
	mutex sync.Mutex

	Viewer         string
	Scopes         string
	MembershipRole string
	PerPage        int // taille de page forcée, pour éprouver la pagination

	Orgs map[string]string // login → nom affiché
	// Rôle du compte connecté par organisation ; à défaut, MembershipRole vaut
	// pour toutes. Une valeur vide simule une portée « read:org » absente.
	OrgRoles map[string]string
	// Droit accordé aux membres de créer des dépôts, quand il est connu.
	MembersCanCreate map[string]bool
	Users            map[string]string // login → nom complet (vide = profil sans nom)
	Repos            map[string]*RepoState
	Templates        map[string]bool

	Collaborators map[string]map[string]string // dépôt → compte → droit
	Invitations   map[string][]invitation
	Deleted       []string

	Blobs   map[string][]byte
	Trees   map[string]map[string]treeEntry
	Commits map[string]commit
	Refs    map[string]string // « org/depot@branche » → SHA

	// Injection de pannes, par « MÉTHODE /chemin ».
	FailOn map[string]Failure
	Flaky  map[string]int

	Calls []string

	nextInvitation int64
}

type invitation struct {
	ID         int64
	Login      string
	Permission string
}

type treeEntry struct {
	Mode string
	Blob string
}

// NewState prépare un état par défaut : une organisation « acme » et trois comptes.
func NewState() *State {
	return &State{
		Viewer:           "prof",
		Scopes:           "repo, read:org, delete_repo, workflow",
		MembershipRole:   "admin",
		Orgs:             map[string]string{"acme": "ACME Éducation"},
		OrgRoles:         map[string]string{},
		MembersCanCreate: map[string]bool{},
		Users: map[string]string{
			"emilie-cote": "Émilie Côté",
			"jlpicard":    "Jean-Luc Picard",
			"aminata-d":   "", // profil sans nom complet
			"prof":        "Professeure",
		},
		Repos:          map[string]*RepoState{},
		Templates:      map[string]bool{"acme/modele-tp": true},
		Collaborators:  map[string]map[string]string{},
		Invitations:    map[string][]invitation{},
		Blobs:          map[string][]byte{},
		Trees:          map[string]map[string]treeEntry{},
		Commits:        map[string]commit{},
		Refs:           map[string]string{},
		FailOn:         map[string]Failure{},
		Flaky:          map[string]int{},
		nextInvitation: 1,
	}
}

// roleLocked renvoie le rôle du compte connecté dans une organisation.
func (s *State) roleLocked(org string) string {
	if role, found := s.OrgRoles[org]; found {
		return role
	}
	return s.MembershipRole
}

// AddRepo enregistre un dépôt existant.
func (s *State) AddRepo(org, name string, private bool) *RepoState {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.addRepoLocked(org, name, private)
}

func (s *State) addRepoLocked(org, name string, private bool) *RepoState {
	repo := &RepoState{Org: org, Name: name, Private: private, DefaultBranch: "main"}
	s.Repos[repo.FullName()] = repo
	return repo
}

// SeedCommit place un premier commit dans un dépôt (contenu déjà remis, par exemple).
func (s *State) SeedCommit(fullName string, files map[string]string, branch string) string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if branch == "" {
		branch = "main"
	}
	tree := map[string]treeEntry{}
	for path, content := range files {
		blob := digest("blob:" + path + ":" + content)
		s.Blobs[blob] = []byte(content)
		tree[path] = treeEntry{Mode: "100644", Blob: blob}
	}
	treeSHA := digest("tree:" + fullName + ":" + fmt.Sprint(sortedKeys(tree)))
	s.Trees[treeSHA] = tree
	commitSHA := digest("commit:" + fullName + ":" + treeSHA)
	s.Commits[commitSHA] = commit{Tree: treeSHA}
	s.Refs[fullName+"@"+branch] = commitSHA
	return commitSHA
}

// Files renvoie le contenu texte d'un dépôt, tel qu'il serait cloné.
func (s *State) Files(fullName, branch string) map[string]string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if branch == "" {
		branch = "main"
	}
	commitSHA, found := s.Refs[fullName+"@"+branch]
	if !found {
		return map[string]string{}
	}
	files := map[string]string{}
	for path, entry := range s.Trees[s.Commits[commitSHA].Tree] {
		files[path] = string(s.Blobs[entry.Blob])
	}
	return files
}

// AcceptInvitations transforme les invitations en attente en collaborateurs établis.
func (s *State) AcceptInvitations(fullName string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, item := range s.Invitations[fullName] {
		if s.Collaborators[fullName] == nil {
			s.Collaborators[fullName] = map[string]string{}
		}
		s.Collaborators[fullName][item.Login] = item.Permission
	}
	delete(s.Invitations, fullName)
}

// CallCount compte les appels reçus dont le chemin contient le fragment donné.
func (s *State) CallCount(fragment string) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	count := 0
	for _, call := range s.Calls {
		if strings.Contains(call, fragment) {
			count++
		}
	}
	return count
}

// AllCalls renvoie une copie du journal des appels.
func (s *State) AllCalls() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]string(nil), s.Calls...)
}

// RepoNames renvoie les dépôts d'une organisation, triés.
func (s *State) RepoNames(org string) []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var names []string
	for _, repo := range s.Repos {
		if repo.Org == org {
			names = append(names, repo.Name)
		}
	}
	sort.Strings(names)
	return names
}

// Server est le faux serveur GitHub.
type Server struct {
	*httptest.Server
	State *State
}

// New démarre un faux serveur GitHub ; l'appelant doit fermer le serveur.
func New(state *State) *Server {
	if state == nil {
		state = NewState()
	}
	server := &Server{State: state}
	server.Server = httptest.NewServer(http.HandlerFunc(server.handle))
	return server
}

// URL renvoie la racine à donner au client.
func (s *Server) URL() string { return s.Server.URL }

var (
	orgRe          = regexp.MustCompile(`^/orgs/([^/]+)$`)
	orgReposRe     = regexp.MustCompile(`^/orgs/([^/]+)/repos$`)
	membershipRe   = regexp.MustCompile(`^/orgs/([^/]+)/memberships/([^/]+)$`)
	userRe         = regexp.MustCompile(`^/users/([^/]+)$`)
	repoRe         = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)$`)
	generateRe     = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/generate$`)
	collaboratorRe = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/collaborators/([^/]+)$`)
	collabsRe      = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/collaborators$`)
	invitationsRe  = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/invitations$`)
	invitationRe   = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/invitations/(\d+)$`)
	blobsRe        = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/blobs$`)
	treesRe        = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/trees$`)
	commitsRe      = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/commits$`)
	commitRe       = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/commits/([^/]+)$`)
	refsRe         = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/refs$`)
	refRe          = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/ref/heads/(.+)$`)
	refUpdateRe    = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/refs/heads/(.+)$`)
)

func (s *Server) handle(writer http.ResponseWriter, request *http.Request) {
	state := s.State
	path := request.URL.Path
	key := request.Method + " " + path

	state.mutex.Lock()
	state.Calls = append(state.Calls, key)
	if remaining, found := state.Flaky[key]; found && remaining > 0 {
		state.Flaky[key] = remaining - 1
		state.mutex.Unlock()
		s.send(writer, 500, map[string]string{"message": "Panne passagère"})
		return
	}
	if failure, found := state.FailOn[key]; found {
		state.mutex.Unlock()
		s.send(writer, failure.Status, map[string]string{"message": failure.Message})
		return
	}
	state.mutex.Unlock()

	switch request.Method {
	case http.MethodGet:
		s.get(writer, request, path)
	case http.MethodPost:
		s.post(writer, request, path)
	case http.MethodPut:
		s.put(writer, request, path)
	case http.MethodPatch:
		s.patch(writer, request, path)
	case http.MethodDelete:
		s.delete(writer, request, path)
	default:
		s.send(writer, 405, map[string]string{"message": "Method Not Allowed"})
	}
}

func (s *Server) get(writer http.ResponseWriter, request *http.Request, path string) {
	state := s.State
	state.mutex.Lock()
	defer state.mutex.Unlock()

	if path == "/user" {
		s.send(writer, 200, map[string]any{"login": state.Viewer})
		return
	}
	if path == "/user/memberships/orgs" {
		payload := make([]map[string]any, 0, len(state.Orgs))
		for _, login := range sortedKeys(state.Orgs) {
			role := state.roleLocked(login)
			if role == "" {
				continue // sans rôle lisible, l'organisation reste invisible
			}
			payload = append(payload, map[string]any{
				"state":        "active",
				"role":         role,
				"organization": map[string]any{"login": login},
			})
		}
		s.send(writer, 200, payload)
		return
	}
	if match := orgRe.FindStringSubmatch(path); match != nil {
		name, found := state.Orgs[match[1]]
		if !found {
			s.notFound(writer)
			return
		}
		payload := map[string]any{"login": match[1], "name": name}
		// GitHub ne montre ce réglage qu'aux propriétaires : le test décide
		// s'il est visible en renseignant, ou non, MembersCanCreate.
		if permis, connu := state.MembersCanCreate[match[1]]; connu {
			payload["members_can_create_repositories"] = permis
		}
		s.send(writer, 200, payload)
		return
	}
	if match := membershipRe.FindStringSubmatch(path); match != nil {
		role := state.roleLocked(match[1])
		if role == "" {
			s.send(writer, 403, map[string]string{"message": "Forbidden"})
			return
		}
		s.send(writer, 200, map[string]any{"role": role})
		return
	}
	if match := userRe.FindStringSubmatch(path); match != nil {
		name, found := state.Users[strings.ToLower(match[1])]
		if !found {
			s.notFound(writer)
			return
		}
		s.send(writer, 200, map[string]any{"login": match[1], "name": name})
		return
	}
	if match := orgReposRe.FindStringSubmatch(path); match != nil {
		if _, found := state.Orgs[match[1]]; !found {
			s.notFound(writer)
			return
		}
		var repos []*RepoState
		for _, repo := range state.Repos {
			if repo.Org == match[1] {
				repos = append(repos, repo)
			}
		}
		sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })
		s.sendPage(writer, request, repos)
		return
	}
	if match := collabsRe.FindStringSubmatch(path); match != nil {
		full := match[1] + "/" + match[2]
		if request.URL.Query().Get("affiliation") != "direct" {
			// Sans « affiliation=direct », GitHub ajoute les administrateurs de
			// l'organisation : le faux serveur reproduit ce piège.
			s.send(writer, 200, []map[string]any{{"login": "admin-org"}})
			return
		}
		var logins []string
		for login := range state.Collaborators[full] {
			logins = append(logins, login)
		}
		sort.Strings(logins)
		payload := make([]map[string]any, 0, len(logins))
		for _, login := range logins {
			payload = append(payload, map[string]any{"login": login})
		}
		s.send(writer, 200, payload)
		return
	}
	if match := invitationsRe.FindStringSubmatch(path); match != nil {
		full := match[1] + "/" + match[2]
		payload := make([]map[string]any, 0)
		for _, item := range state.Invitations[full] {
			payload = append(payload, map[string]any{
				"id":      item.ID,
				"invitee": map[string]any{"login": item.Login},
			})
		}
		s.send(writer, 200, payload)
		return
	}
	if match := commitRe.FindStringSubmatch(path); match != nil {
		found, exists := state.Commits[match[3]]
		if !exists {
			s.notFound(writer)
			return
		}
		s.send(writer, 200, map[string]any{"sha": match[3], "tree": map[string]any{"sha": found.Tree}})
		return
	}
	if match := refRe.FindStringSubmatch(path); match != nil {
		full := match[1] + "/" + match[2]
		if _, exists := state.Repos[full]; !exists {
			s.notFound(writer)
			return
		}
		sha, exists := state.Refs[full+"@"+match[3]]
		if !exists {
			// 409 : dépôt sans aucun commit, comme le fait GitHub.
			s.send(writer, 409, map[string]string{"message": "Git Repository is empty."})
			return
		}
		s.send(writer, 200, map[string]any{"object": map[string]any{"sha": sha}})
		return
	}
	if match := repoRe.FindStringSubmatch(path); match != nil {
		full := match[1] + "/" + match[2]
		repo, exists := state.Repos[full]
		if !exists {
			s.notFound(writer)
			return
		}
		s.send(writer, 200, s.repoPayload(repo))
		return
	}
	s.notFound(writer)
}

func (s *Server) post(writer http.ResponseWriter, request *http.Request, path string) {
	state := s.State
	body := decode(request)
	state.mutex.Lock()
	defer state.mutex.Unlock()

	if match := orgReposRe.FindStringSubmatch(path); match != nil {
		org := match[1]
		name, _ := body["name"].(string)
		full := org + "/" + name
		if _, exists := state.Repos[full]; exists {
			s.send(writer, 422, map[string]any{
				"message": "Repository creation failed.",
				"errors":  []map[string]string{{"message": "name already exists on this account"}},
			})
			return
		}
		repo := state.addRepoLocked(org, name, boolOf(body["private"], true))
		repo.Description, _ = body["description"].(string)
		if boolOf(body["auto_init"], false) {
			// auto_init place un README : le dépôt n'est alors plus vide.
			s.seedLocked(full, map[string]string{"README.md": "# " + name + "\n"}, "main")
		}
		s.send(writer, 201, s.repoPayload(repo))
		return
	}
	if match := generateRe.FindStringSubmatch(path); match != nil {
		templateFull := match[1] + "/" + match[2]
		if !state.Templates[templateFull] {
			s.send(writer, 422, map[string]string{"message": "Repository is not a template"})
			return
		}
		org, _ := body["owner"].(string)
		name, _ := body["name"].(string)
		full := org + "/" + name
		if _, exists := state.Repos[full]; exists {
			s.send(writer, 422, map[string]any{
				"message": "Repository creation failed.",
				"errors":  []map[string]string{{"message": "name already exists on this account"}},
			})
			return
		}
		repo := state.addRepoLocked(org, name, boolOf(body["private"], true))
		repo.Description, _ = body["description"].(string)
		repo.Template = templateFull
		// Le contenu du modèle est recopié dans le nouveau dépôt.
		s.seedLocked(full, s.filesLocked(templateFull, "main"), "main")
		s.send(writer, 201, s.repoPayload(repo))
		return
	}
	if match := blobsRe.FindStringSubmatch(path); match != nil {
		content, _ := body["content"].(string)
		raw, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			s.send(writer, 422, map[string]string{"message": "Invalid base64 content"})
			return
		}
		sha := digest("blob:" + content)
		state.Blobs[sha] = raw
		s.send(writer, 201, map[string]any{"sha": sha})
		return
	}
	if match := treesRe.FindStringSubmatch(path); match != nil {
		tree := map[string]treeEntry{}
		if base, ok := body["base_tree"].(string); ok && base != "" {
			for path, entry := range state.Trees[base] {
				tree[path] = entry
			}
		}
		entries, _ := body["tree"].([]any)
		for _, item := range entries {
			entry, _ := item.(map[string]any)
			filePath, _ := entry["path"].(string)
			mode, _ := entry["mode"].(string)
			blob, _ := entry["sha"].(string)
			tree[filePath] = treeEntry{Mode: mode, Blob: blob}
		}
		sha := digest("tree:" + match[1] + "/" + match[2] + ":" + fmt.Sprint(sortedKeys(tree)) + fmt.Sprint(len(state.Trees)))
		state.Trees[sha] = tree
		s.send(writer, 201, map[string]any{"sha": sha})
		return
	}
	if match := commitsRe.FindStringSubmatch(path); match != nil {
		tree, _ := body["tree"].(string)
		message, _ := body["message"].(string)
		var parents []string
		if list, ok := body["parents"].([]any); ok {
			for _, item := range list {
				if parent, ok := item.(string); ok {
					parents = append(parents, parent)
				}
			}
		}
		sha := digest("commit:" + match[1] + "/" + match[2] + ":" + tree + fmt.Sprint(parents))
		state.Commits[sha] = commit{Tree: tree, Parents: parents, Message: message}
		s.send(writer, 201, map[string]any{"sha": sha})
		return
	}
	if match := refsRe.FindStringSubmatch(path); match != nil {
		full := match[1] + "/" + match[2]
		ref, _ := body["ref"].(string)
		sha, _ := body["sha"].(string)
		branch := strings.TrimPrefix(ref, "refs/heads/")
		if _, exists := state.Refs[full+"@"+branch]; exists {
			s.send(writer, 422, map[string]string{"message": "Reference already exists"})
			return
		}
		state.Refs[full+"@"+branch] = sha
		state.touchLocked(full)
		s.send(writer, 201, map[string]any{"ref": ref, "object": map[string]any{"sha": sha}})
		return
	}
	s.notFound(writer)
}

func (s *Server) put(writer http.ResponseWriter, request *http.Request, path string) {
	state := s.State
	body := decode(request)
	state.mutex.Lock()
	defer state.mutex.Unlock()

	if match := collaboratorRe.FindStringSubmatch(path); match != nil {
		full := match[1] + "/" + match[2]
		login := match[3]
		if _, exists := state.Repos[full]; !exists {
			s.notFound(writer)
			return
		}
		if _, known := state.Users[strings.ToLower(login)]; !known {
			s.send(writer, 422, map[string]string{"message": "Invalid user"})
			return
		}
		permission, _ := body["permission"].(string)
		// Un membre de l'organisation obtient l'accès direct : 204, sans invitation.
		if login == state.Viewer {
			if state.Collaborators[full] == nil {
				state.Collaborators[full] = map[string]string{}
			}
			state.Collaborators[full][login] = permission
			writer.WriteHeader(204)
			return
		}
		for _, existing := range state.Invitations[full] {
			if existing.Login == login {
				writer.WriteHeader(204)
				return
			}
		}
		if _, already := state.Collaborators[full][login]; already {
			writer.WriteHeader(204)
			return
		}
		item := invitation{ID: state.nextInvitation, Login: login, Permission: permission}
		state.nextInvitation++
		state.Invitations[full] = append(state.Invitations[full], item)
		s.send(writer, 201, map[string]any{
			"id":      item.ID,
			"invitee": map[string]any{"login": login},
		})
		return
	}
	s.notFound(writer)
}

func (s *Server) patch(writer http.ResponseWriter, request *http.Request, path string) {
	state := s.State
	body := decode(request)
	state.mutex.Lock()
	defer state.mutex.Unlock()

	if match := refUpdateRe.FindStringSubmatch(path); match != nil {
		full := match[1] + "/" + match[2]
		branch := match[3]
		if _, exists := state.Refs[full+"@"+branch]; !exists {
			s.notFound(writer)
			return
		}
		sha, _ := body["sha"].(string)
		state.Refs[full+"@"+branch] = sha
		state.touchLocked(full)
		s.send(writer, 200, map[string]any{"object": map[string]any{"sha": sha}})
		return
	}
	s.notFound(writer)
}

func (s *Server) delete(writer http.ResponseWriter, request *http.Request, path string) {
	state := s.State
	state.mutex.Lock()
	defer state.mutex.Unlock()

	if match := collaboratorRe.FindStringSubmatch(path); match != nil {
		full := match[1] + "/" + match[2]
		delete(state.Collaborators[full], match[3])
		writer.WriteHeader(204)
		return
	}
	if match := invitationRe.FindStringSubmatch(path); match != nil {
		full := match[1] + "/" + match[2]
		identifier, _ := strconv.ParseInt(match[3], 10, 64)
		var kept []invitation
		for _, item := range state.Invitations[full] {
			if item.ID != identifier {
				kept = append(kept, item)
			}
		}
		state.Invitations[full] = kept
		writer.WriteHeader(204)
		return
	}
	if match := repoRe.FindStringSubmatch(path); match != nil {
		full := match[1] + "/" + match[2]
		if _, exists := state.Repos[full]; !exists {
			s.notFound(writer)
			return
		}
		delete(state.Repos, full)
		state.Deleted = append(state.Deleted, full)
		writer.WriteHeader(204)
		return
	}
	s.notFound(writer)
}

// ------------------------------------------------------------------ utilitaires

func (s *Server) repoPayload(repo *RepoState) map[string]any {
	url := "https://github.com/" + repo.FullName()
	if repo.URLOverride != "" {
		url = repo.URLOverride
	}
	payload := map[string]any{
		"name":           repo.Name,
		"full_name":      repo.FullName(),
		"private":        repo.Private,
		"html_url":       url,
		"default_branch": repo.DefaultBranch,
		"description":    repo.Description,
		"pushed_at":      repo.PushedAt,
		"is_template":    s.State.Templates[repo.FullName()],
	}
	if repo.Template != "" {
		payload["template_repository"] = map[string]any{"full_name": repo.Template}
	}
	return payload
}

// sendPage renvoie une page de dépôts avec l'en-tête Link attendu par le client.
func (s *Server) sendPage(writer http.ResponseWriter, request *http.Request, repos []*RepoState) {
	perPage := s.State.PerPage
	if perPage <= 0 {
		perPage, _ = strconv.Atoi(request.URL.Query().Get("per_page"))
	}
	if perPage <= 0 {
		perPage = 30
	}
	page, _ := strconv.Atoi(request.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * perPage
	end := start + perPage
	if start > len(repos) {
		start = len(repos)
	}
	if end > len(repos) {
		end = len(repos)
	}
	payload := make([]map[string]any, 0, end-start)
	for _, repo := range repos[start:end] {
		payload = append(payload, s.repoPayload(repo))
	}
	if end < len(repos) {
		next := *request.URL
		query := next.Query()
		query.Set("page", strconv.Itoa(page+1))
		query.Set("per_page", strconv.Itoa(perPage))
		next.RawQuery = query.Encode()
		writer.Header().Set("Link",
			fmt.Sprintf("<%s%s>; rel=\"next\"", s.Server.URL, next.RequestURI()))
	}
	s.send(writer, 200, payload)
}

func (s *Server) send(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	if s.State.Scopes != "" {
		writer.Header().Set("X-OAuth-Scopes", s.State.Scopes)
	}
	writer.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(writer).Encode(payload)
	}
}

func (s *Server) notFound(writer http.ResponseWriter) {
	s.send(writer, 404, map[string]string{"message": "Not Found"})
}

// seedLocked place un commit ; l'appelant détient déjà le verrou.
func (s *Server) seedLocked(fullName string, files map[string]string, branch string) {
	if len(files) == 0 {
		return
	}
	tree := map[string]treeEntry{}
	for path, content := range files {
		blob := digest("blob:" + path + ":" + content)
		s.State.Blobs[blob] = []byte(content)
		tree[path] = treeEntry{Mode: "100644", Blob: blob}
	}
	treeSHA := digest("tree:" + fullName + ":" + fmt.Sprint(sortedKeys(tree)))
	s.State.Trees[treeSHA] = tree
	commitSHA := digest("commit:" + fullName + ":" + treeSHA)
	s.State.Commits[commitSHA] = commit{Tree: treeSHA}
	s.State.Refs[fullName+"@"+branch] = commitSHA
	s.State.touchLocked(fullName)
}

func (s *Server) filesLocked(fullName, branch string) map[string]string {
	commitSHA, found := s.State.Refs[fullName+"@"+branch]
	if !found {
		return map[string]string{}
	}
	files := map[string]string{}
	for path, entry := range s.State.Trees[s.State.Commits[commitSHA].Tree] {
		files[path] = string(s.State.Blobs[entry.Blob])
	}
	return files
}

func (s *State) touchLocked(fullName string) {
	if repo, found := s.Repos[fullName]; found {
		repo.PushedAt = "2026-08-28T12:00:00Z"
	}
}

func decode(request *http.Request) map[string]any {
	body := map[string]any{}
	if request.Body == nil {
		return body
	}
	defer request.Body.Close()
	_ = json.NewDecoder(request.Body).Decode(&body)
	return body
}

func boolOf(value any, fallback bool) bool {
	if found, ok := value.(bool); ok {
		return found
	}
	return fallback
}

func digest(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
