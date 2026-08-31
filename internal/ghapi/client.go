// Package ghapi enveloppe les quelques points de l'API GitHub nécessaires à
// l'outil. Le transport, le jeton, l'hôte et les en-têtes viennent de go-gh.
package ghapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
)

// Client donne accès aux points d'API utilisés par l'outil.
// Il est sûr d'emploi depuis plusieurs goroutines : la résolution des noms
// complets et les inventaires interrogent l'API en parallèle.
type Client struct {
	rest    *api.RESTClient
	baseURL string // vide en production : go-gh résout l'hôte lui-même
	host    string

	mutex   sync.RWMutex
	scopes  string
	hasSeen bool
}

// Options règle la construction du client.
type Options struct {
	Host    string
	Token   string
	BaseURL string              // utilisé par les tests, jamais en production
	Sleep   func(time.Duration) // injecté par les tests pour ne pas attendre
	Now     func() time.Time
}

// DefaultHost renvoie l'hôte GitHub configuré pour gh (github.com ou une instance Enterprise).
func DefaultHost() string {
	host, _ := auth.DefaultHost()
	return host
}

// TokenForHost renvoie le jeton connu de gh et sa provenance, sans jamais l'afficher.
func TokenForHost(host string) (string, string) {
	return auth.TokenForHost(host)
}

// New construit un client pour l'hôte donné.
func New(options Options) (*Client, error) {
	host := options.Host
	if host == "" {
		host = DefaultHost()
	}
	token := options.Token
	if token == "" {
		token, _ = auth.TokenForHost(host)
	}
	rest, err := api.NewRESTClient(api.ClientOptions{
		Host:      host,
		AuthToken: token,
		Timeout:   30 * time.Second,
		Headers: map[string]string{
			"X-GitHub-Api-Version": "2022-11-28",
			"Accept":               "application/vnd.github+json",
		},
		Transport: &retryTransport{base: http.DefaultTransport, sleep: options.Sleep, now: options.Now},
	})
	if err != nil {
		return nil, &Error{Message: "Client GitHub inutilisable : " + err.Error()}
	}
	return &Client{rest: rest, baseURL: strings.TrimSuffix(options.BaseURL, "/"), host: host}, nil
}

// Host renvoie l'hôte interrogé.
func (c *Client) Host() string { return c.host }

// Scopes renvoie les portées annoncées par GitHub, ou une chaîne vide si
// aucune réponse ne les a encore révélées (cas d'un jeton « fine-grained »).
func (c *Client) Scopes() (string, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.scopes, c.hasSeen
}

// Response est une réponse HTTP décodée.
type Response struct {
	Status int
	Body   []byte
	Header http.Header
}

// JSON décode le corps de la réponse.
func (r *Response) JSON(target any) error {
	if len(bytes.TrimSpace(r.Body)) == 0 {
		return nil
	}
	return json.Unmarshal(r.Body, target)
}

func (c *Client) url(path string) string {
	if c.baseURL == "" {
		return path
	}
	return c.baseURL + "/" + strings.TrimPrefix(path, "/")
}

// do exécute une requête ; les statuts listés dans allow sont renvoyés sans erreur.
func (c *Client) do(method, path string, body any, allow ...int) (*Response, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, &Error{Message: "Requête impossible : " + err.Error()}
		}
		payload = bytes.NewReader(encoded)
	}

	response, err := c.rest.Request(method, c.url(path), payload)
	if err != nil {
		converted := convert(err)
		status := StatusOf(converted)
		c.rememberScopesFromError(err)
		for _, allowed := range allow {
			if status == allowed {
				return &Response{Status: status}, nil
			}
		}
		return nil, converted
	}
	defer response.Body.Close()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, &Error{Status: response.StatusCode, Message: "Réponse illisible : " + err.Error()}
	}
	c.rememberScopes(response.Header)
	return &Response{Status: response.StatusCode, Body: content, Header: response.Header}, nil
}

func (c *Client) rememberScopes(header http.Header) {
	if header == nil {
		return
	}
	values, found := header["X-Oauth-Scopes"]
	if !found || len(values) == 0 {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.scopes, c.hasSeen = values[0], true
}

func (c *Client) rememberScopesFromError(err error) {
	var httpError *api.HTTPError
	if asHTTPError(err, &httpError) {
		c.rememberScopes(httpError.Headers)
	}
}

// HasScope indique si une portée est présente. Le second retour vaut faux quand
// GitHub n'annonce aucune portée : rien ne peut alors être affirmé.
func (c *Client) HasScope(scope string) (bool, bool) {
	scopes, seen := c.Scopes()
	if !seen || strings.TrimSpace(scopes) == "" {
		return false, false
	}
	for _, item := range strings.Split(scopes, ",") {
		if strings.TrimSpace(item) == scope {
			return true, true
		}
	}
	return false, true
}

// ---------------------------------------------------------------------- lecture

// User décrit un compte GitHub.
type User struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

// Org décrit une organisation. Le droit accordé aux membres de créer des dépôts
// n'est visible que des propriétaires : un pointeur nul signifie « on ne sait pas ».
type Org struct {
	Login                        string `json:"login"`
	Name                         string `json:"name"`
	MembersCanCreateRepositories *bool  `json:"members_can_create_repositories"`
}

// Membership décrit l'appartenance du compte connecté à une organisation.
type Membership struct {
	State        string `json:"state"`
	Role         string `json:"role"`
	Organization struct {
		Login string `json:"login"`
	} `json:"organization"`
}

// Repo décrit un dépôt.
type Repo struct {
	Name               string `json:"name"`
	FullName           string `json:"full_name"`
	Private            bool   `json:"private"`
	HTMLURL            string `json:"html_url"`
	DefaultBranch      string `json:"default_branch"`
	PushedAt           string `json:"pushed_at"`
	IsTemplate         bool   `json:"is_template"`
	TemplateRepository *struct {
		FullName string `json:"full_name"`
	} `json:"template_repository"`
}

// Collaborator décrit un collaborateur d'un dépôt.
type Collaborator struct {
	Login string `json:"login"`
}

// Invitation décrit une invitation en attente.
type Invitation struct {
	ID      int64 `json:"id"`
	Invitee struct {
		Login string `json:"login"`
	} `json:"invitee"`
}

// AuthenticatedUser vérifie le jeton et renvoie le compte associé.
func (c *Client) AuthenticatedUser() (*User, error) {
	response, err := c.do(http.MethodGet, "user", nil)
	if err != nil {
		return nil, err
	}
	user := &User{}
	return user, response.JSON(user)
}

// GetOrg renvoie l'organisation, ou une erreur explicite si elle est absente.
func (c *Client) GetOrg(org string) (*Org, error) {
	response, err := c.do(http.MethodGet, "orgs/"+url.PathEscape(org), nil)
	if err != nil {
		return nil, err
	}
	value := &Org{}
	return value, response.JSON(value)
}

// GetRepoOwnerOrg renvoie l'organisation, ou nil si elle est invisible.
func (c *Client) GetRepoOwnerOrg(org string) (*Org, error) {
	response, err := c.do(http.MethodGet, "orgs/"+url.PathEscape(org), nil,
		http.StatusNotFound, http.StatusForbidden)
	if err != nil {
		return nil, err
	}
	if response.Status != http.StatusOK {
		return nil, nil
	}
	value := &Org{}
	return value, response.JSON(value)
}

// GetRepo renvoie le dépôt s'il existe déjà, sinon nil.
func (c *Client) GetRepo(owner, repo string) (*Repo, error) {
	response, err := c.do(http.MethodGet, repoPath(owner, repo), nil, http.StatusNotFound)
	if err != nil {
		return nil, err
	}
	if response.Status == http.StatusNotFound {
		return nil, nil
	}
	value := &Repo{}
	return value, response.JSON(value)
}

// UserExists vérifie qu'un compte GitHub existe réellement.
func (c *Client) UserExists(login string) (bool, error) {
	response, err := c.do(http.MethodGet, "users/"+url.PathEscape(login), nil, http.StatusNotFound)
	if err != nil {
		return false, err
	}
	return response.Status != http.StatusNotFound, nil
}

// GetUser renvoie le profil public d'un compte, ou nil s'il est introuvable.
func (c *Client) GetUser(login string) (*User, error) {
	response, err := c.do(http.MethodGet, "users/"+url.PathEscape(login), nil, http.StatusNotFound)
	if err != nil {
		return nil, err
	}
	if response.Status == http.StatusNotFound {
		return nil, nil
	}
	user := &User{}
	return user, response.JSON(user)
}

// OrgMembership renvoie le rôle d'une personne dans l'organisation, ou une chaîne vide.
func (c *Client) OrgMembership(org, username string) (string, error) {
	path := "orgs/" + url.PathEscape(org) + "/memberships/" + url.PathEscape(username)
	response, err := c.do(http.MethodGet, path, nil, http.StatusForbidden, http.StatusNotFound)
	if err != nil {
		return "", err
	}
	if response.Status != http.StatusOK {
		return "", nil
	}
	var membership struct {
		Role string `json:"role"`
	}
	if err := response.JSON(&membership); err != nil {
		return "", &Error{Message: "Rôle illisible : " + err.Error()}
	}
	return membership.Role, nil
}

// --------------------------------------------------------------------- écriture

// CreateOrgRepo crée un dépôt vide dans l'organisation.
func (c *Client) CreateOrgRepo(org, name string, private bool, description string, autoInit bool) (*Repo, error) {
	body := map[string]any{
		"name":         name,
		"private":      private,
		"description":  description,
		"auto_init":    autoInit,
		"has_issues":   true,
		"has_wiki":     false,
		"has_projects": false,
	}
	response, err := c.do(http.MethodPost, "orgs/"+url.PathEscape(org)+"/repos", body)
	if err != nil {
		return nil, err
	}
	repo := &Repo{}
	return repo, response.JSON(repo)
}

// GenerateFromTemplate crée un dépôt à partir d'un dépôt modèle.
func (c *Client) GenerateFromTemplate(templateOwner, templateRepo, owner, name string,
	private bool, description string, includeAllBranches bool) (*Repo, error) {
	body := map[string]any{
		"owner":                owner,
		"name":                 name,
		"private":              private,
		"description":          description,
		"include_all_branches": includeAllBranches,
	}
	path := repoPath(templateOwner, templateRepo) + "/generate"
	response, err := c.do(http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	repo := &Repo{}
	return repo, response.JSON(repo)
}

// Résultats possibles d'une invitation.
const (
	CollaboratorInvited = "invitée"
	CollaboratorAdded   = "ajoutée"
)

// AddCollaborator invite une personne sur le dépôt.
// 201 : invitation créée. 204 : la personne était déjà membre et a l'accès direct.
func (c *Client) AddCollaborator(owner, repo, username, permission string) (string, error) {
	path := repoPath(owner, repo) + "/collaborators/" + url.PathEscape(username)
	response, err := c.do(http.MethodPut, path, map[string]any{"permission": permission})
	if err != nil {
		return "", err
	}
	if response.Status == http.StatusCreated {
		return CollaboratorInvited, nil
	}
	return CollaboratorAdded, nil
}

// RemoveCollaborator retire l'accès d'une personne au dépôt.
func (c *Client) RemoveCollaborator(owner, repo, username string) error {
	path := repoPath(owner, repo) + "/collaborators/" + url.PathEscape(username)
	_, err := c.do(http.MethodDelete, path, nil, http.StatusNotFound)
	return err
}

// CancelInvitation annule une invitation encore en attente.
func (c *Client) CancelInvitation(owner, repo string, invitationID int64) error {
	path := repoPath(owner, repo) + "/invitations/" + itoa(invitationID)
	_, err := c.do(http.MethodDelete, path, nil, http.StatusNotFound)
	return err
}

// DeleteRepo supprime définitivement un dépôt. Irréversible : à n'appeler
// qu'après confirmation explicite de la personne qui enseigne.
func (c *Client) DeleteRepo(owner, repo string) error {
	_, err := c.do(http.MethodDelete, repoPath(owner, repo), nil)
	return err
}

// ------------------------------------------------------------------ inventaire

var nextLinkRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// paginate parcourt toutes les pages d'une collection en suivant l'en-tête Link.
func (c *Client) paginate(path string, onPage func(total int), collect func([]byte) (int, error)) error {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	next := c.url(path + separator + "per_page=100")
	total := 0
	for next != "" {
		response, err := c.rest.Request(http.MethodGet, next, nil)
		if err != nil {
			return convert(err)
		}
		content, readErr := io.ReadAll(response.Body)
		c.rememberScopes(response.Header)
		link := response.Header.Get("Link")
		response.Body.Close()
		if readErr != nil {
			return &Error{Status: response.StatusCode, Message: "Réponse illisible : " + readErr.Error()}
		}
		count, err := collect(content)
		if err != nil {
			return &Error{Message: "Réponse inattendue de GitHub : " + err.Error()}
		}
		total += count
		if onPage != nil {
			onPage(total)
		}
		next = ""
		if match := nextLinkRe.FindStringSubmatch(link); match != nil {
			next = match[1]
		}
	}
	return nil
}

// ListOrgRepos liste tous les dépôts de l'organisation.
func (c *Client) ListOrgRepos(org string, onPage func(total int)) ([]groups.RepoInfo, error) {
	var all []groups.RepoInfo
	err := c.paginate("orgs/"+url.PathEscape(org)+"/repos", onPage, func(content []byte) (int, error) {
		var page []groups.RepoInfo
		if err := json.Unmarshal(content, &page); err != nil {
			return 0, err
		}
		all = append(all, page...)
		return len(page), nil
	})
	return all, err
}

// ListOrgMemberships renvoie les organisations du compte connecté et le rôle
// qu'il y tient, en un seul parcours. La portée « read:org » est nécessaire.
func (c *Client) ListOrgMemberships(onPage func(total int)) ([]Membership, error) {
	var all []Membership
	err := c.paginate("user/memberships/orgs", onPage, func(content []byte) (int, error) {
		var page []Membership
		if err := json.Unmarshal(content, &page); err != nil {
			return 0, err
		}
		for _, item := range page {
			// Une invitation encore en attente ne donne aucun droit.
			if item.State == "active" {
				all = append(all, item)
			}
		}
		return len(page), nil
	})
	return all, err
}

// ListCollaborators renvoie les collaborateurs directs du dépôt. Sans
// « affiliation=direct », la réponse inclurait tous les administrateurs de
// l'organisation et deviendrait inexploitable.
func (c *Client) ListCollaborators(owner, repo string) ([]Collaborator, error) {
	var all []Collaborator
	err := c.paginate(repoPath(owner, repo)+"/collaborators?affiliation=direct", nil,
		func(content []byte) (int, error) {
			var page []Collaborator
			if err := json.Unmarshal(content, &page); err != nil {
				return 0, err
			}
			all = append(all, page...)
			return len(page), nil
		})
	return all, err
}

// ListInvitations renvoie les invitations envoyées mais pas encore acceptées.
func (c *Client) ListInvitations(owner, repo string) ([]Invitation, error) {
	var all []Invitation
	err := c.paginate(repoPath(owner, repo)+"/invitations", nil, func(content []byte) (int, error) {
		var page []Invitation
		if err := json.Unmarshal(content, &page); err != nil {
			return 0, err
		}
		all = append(all, page...)
		return len(page), nil
	})
	return all, err
}

// ------------------------------------------------------------- contenu initial

// BranchHead renvoie le SHA du dernier commit d'une branche, ou une chaîne vide.
// Un dépôt sans aucun commit répond 404 ou 409.
func (c *Client) BranchHead(owner, repo, branch string) (string, error) {
	path := repoPath(owner, repo) + "/git/ref/heads/" + url.PathEscape(branch)
	response, err := c.do(http.MethodGet, path, nil, http.StatusNotFound, http.StatusConflict)
	if err != nil {
		return "", err
	}
	if response.Status != http.StatusOK {
		return "", nil
	}
	var reference struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := response.JSON(&reference); err != nil {
		return "", &Error{Message: "Référence illisible : " + err.Error()}
	}
	return reference.Object.SHA, nil
}

// CommitTree renvoie le SHA de l'arbre associé à un commit.
func (c *Client) CommitTree(owner, repo, commitSHA string) (string, error) {
	path := repoPath(owner, repo) + "/git/commits/" + url.PathEscape(commitSHA)
	response, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	var commit struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := response.JSON(&commit); err != nil {
		return "", &Error{Message: "Commit illisible : " + err.Error()}
	}
	return commit.Tree.SHA, nil
}

// CreateBlob téléverse le contenu d'un fichier et renvoie son SHA.
func (c *Client) CreateBlob(owner, repo string, content []byte) (string, error) {
	body := map[string]any{
		"content":  base64.StdEncoding.EncodeToString(content),
		"encoding": "base64",
	}
	response, err := c.do(http.MethodPost, repoPath(owner, repo)+"/git/blobs", body)
	if err != nil {
		return "", err
	}
	var blob struct {
		SHA string `json:"sha"`
	}
	if err := response.JSON(&blob); err != nil {
		return "", &Error{Message: "Blob illisible : " + err.Error()}
	}
	return blob.SHA, nil
}

// TreeEntry est une entrée d'arbre Git.
type TreeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

// CreateTree assemble un arbre Git ; baseTree conserve les fichiers déjà présents.
func (c *Client) CreateTree(owner, repo string, entries []TreeEntry, baseTree string) (string, error) {
	body := map[string]any{"tree": entries}
	if baseTree != "" {
		body["base_tree"] = baseTree
	}
	response, err := c.do(http.MethodPost, repoPath(owner, repo)+"/git/trees", body)
	if err != nil {
		return "", err
	}
	var tree struct {
		SHA string `json:"sha"`
	}
	if err := response.JSON(&tree); err != nil {
		return "", &Error{Message: "Arbre illisible : " + err.Error()}
	}
	return tree.SHA, nil
}

// CreateCommit crée un commit et renvoie son SHA.
func (c *Client) CreateCommit(owner, repo, message, tree string, parents []string) (string, error) {
	if parents == nil {
		parents = []string{}
	}
	body := map[string]any{"message": message, "tree": tree, "parents": parents}
	response, err := c.do(http.MethodPost, repoPath(owner, repo)+"/git/commits", body)
	if err != nil {
		return "", err
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := response.JSON(&commit); err != nil {
		return "", &Error{Message: "Commit illisible : " + err.Error()}
	}
	return commit.SHA, nil
}

// SetBranchHead fait pointer la branche sur le commit, en la créant si nécessaire.
func (c *Client) SetBranchHead(owner, repo, branch, commitSHA string, create bool) error {
	base := repoPath(owner, repo) + "/git/refs"
	if create {
		_, err := c.do(http.MethodPost, base,
			map[string]any{"ref": "refs/heads/" + branch, "sha": commitSHA})
		return err
	}
	_, err := c.do(http.MethodPatch, base+"/heads/"+url.PathEscape(branch),
		map[string]any{"sha": commitSHA, "force": false})
	return err
}

// PushFile est un fichier à déposer dans un dépôt.
type PushFile struct {
	Path    string
	Mode    string
	Content []byte
}

// PushFiles dépose tous les fichiers en un seul commit et renvoie leur nombre.
func (c *Client) PushFiles(owner, repo string, files []PushFile, message, branch string) (int, error) {
	entries := make([]TreeEntry, 0, len(files))
	for _, file := range files {
		sha, err := c.CreateBlob(owner, repo, file.Content)
		if err != nil {
			return 0, err
		}
		entries = append(entries, TreeEntry{Path: file.Path, Mode: file.Mode, Type: "blob", SHA: sha})
	}

	head, err := c.BranchHead(owner, repo, branch)
	if err != nil {
		return 0, err
	}
	baseTree := ""
	var parents []string
	if head != "" {
		if baseTree, err = c.CommitTree(owner, repo, head); err != nil {
			return 0, err
		}
		parents = []string{head}
	}
	tree, err := c.CreateTree(owner, repo, entries, baseTree)
	if err != nil {
		return 0, err
	}
	commit, err := c.CreateCommit(owner, repo, message, tree, parents)
	if err != nil {
		return 0, err
	}
	if err := c.SetBranchHead(owner, repo, branch, commit, head == ""); err != nil {
		return 0, err
	}
	return len(entries), nil
}

func repoPath(owner, repo string) string {
	return "repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

func itoa(value int64) string { return strconv.FormatInt(value, 10) }
