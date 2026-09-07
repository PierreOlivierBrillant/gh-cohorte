package fakegh

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// Les équipes d'organisation, telles que le faux serveur les connaît. GitHub
// tire le « slug » du nom en remplaçant tout ce qui n'est ni lettre ni chiffre
// par un tiret : le point de la nomenclature disparaît donc du slug, et deux
// noms qui ne diffèrent que par lui se heurteraient. Le faux serveur reproduit
// ce piège, c'est la seule façon de l'éprouver.

// TeamState est une équipe telle que le faux serveur la connaît.
type TeamState struct {
	Org         string
	Slug        string
	Name        string
	Description string
	Privacy     string
	Members     map[string]string // compte → rôle
}

var teamSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// newTeam construit une équipe dont le slug est le nom : le cas des équipes que
// l'outil n'a pas créées.
func newTeam(org, nom string) *TeamState {
	return &TeamState{
		Org: org, Slug: TeamSlug(nom), Name: nom,
		Privacy: "closed", Members: map[string]string{},
	}
}

// teamsOfLocked rend les équipes d'une organisation, prêtes à être servies.
// L'état est déjà verrouillé par l'appelant.
func (s *State) teamsOfLocked(org string) []map[string]any {
	var trouvees []*TeamState
	for _, equipe := range s.Teams {
		if equipe.Org == org {
			trouvees = append(trouvees, equipe)
		}
	}
	sort.Slice(trouvees, func(i, j int) bool { return trouvees[i].Slug < trouvees[j].Slug })
	payload := make([]map[string]any, 0, len(trouvees))
	for _, equipe := range trouvees {
		payload = append(payload, teamPayload(equipe))
	}
	return payload
}

// TeamSlug reproduit la façon dont GitHub tire le « slug » du nom d'une équipe.
func TeamSlug(name string) string {
	return strings.Trim(teamSlugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
}

// AddTeam enregistre une équipe existante et renvoie son état.
func (s *State) AddTeam(org, name string, members ...string) *TeamState {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	equipe := newTeam(org, name)
	for _, member := range members {
		equipe.Members[strings.ToLower(member)] = "member"
	}
	s.Teams[org+"/"+equipe.Slug] = equipe
	return equipe
}

// TeamNames renvoie les noms des équipes d'une organisation, triés.
func (s *State) TeamNames(org string) []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	noms := make([]string, 0, len(s.Teams))
	for _, equipe := range s.Teams {
		if equipe.Org == org {
			noms = append(noms, equipe.Name)
		}
	}
	sort.Strings(noms)
	return noms
}

// TeamMembers renvoie les membres d'une équipe, triés.
func (s *State) TeamMembers(org, slug string) []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	equipe, connue := s.Teams[org+"/"+slug]
	if !connue {
		return nil
	}
	membres := make([]string, 0, len(equipe.Members))
	for login := range equipe.Members {
		membres = append(membres, login)
	}
	sort.Strings(membres)
	return membres
}

// TeamRepoNames renvoie les dépôts partagés avec une équipe, triés. Ils y sont
// nommés « organisation/dépôt », comme le chemin de l'API les donne.
func (s *State) TeamRepoNames(org, slug string) []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	noms := make([]string, 0)
	for nom := range s.TeamRepos[org+"/"+slug] {
		noms = append(noms, nom)
	}
	sort.Strings(noms)
	return noms
}

var (
	teamRe        = regexp.MustCompile(`^/orgs/([^/]+)/teams/([^/]+)$`)
	teamMembersRe = regexp.MustCompile(`^/orgs/([^/]+)/teams/([^/]+)/members$`)
	teamMemberRe  = regexp.MustCompile(`^/orgs/([^/]+)/teams/([^/]+)/memberships/([^/]+)$`)
	teamReposRe   = regexp.MustCompile(`^/orgs/([^/]+)/teams/([^/]+)/repos$`)
)

// teamsGet répond aux lectures d'équipes ; faux quand la route n'en est pas une.
// L'état est déjà verrouillé par l'appelant.
func (s *Server) teamsGet(writer http.ResponseWriter, path string) bool {
	state := s.State
	if match := teamMembersRe.FindStringSubmatch(path); match != nil {
		equipe, connue := state.Teams[match[1]+"/"+match[2]]
		if !connue {
			s.notFound(writer)
			return true
		}
		logins := make([]string, 0, len(equipe.Members))
		for login := range equipe.Members {
			logins = append(logins, login)
		}
		sort.Strings(logins)
		payload := make([]map[string]any, 0, len(logins))
		for _, login := range logins {
			payload = append(payload, map[string]any{"login": login})
		}
		s.send(writer, 200, payload)
		return true
	}
	if match := teamReposRe.FindStringSubmatch(path); match != nil {
		if _, connue := state.Teams[match[1]+"/"+match[2]]; !connue {
			s.notFound(writer)
			return true
		}
		noms := make([]string, 0)
		for nom := range state.TeamRepos[match[1]+"/"+match[2]] {
			noms = append(noms, nom)
		}
		sort.Strings(noms)
		payload := make([]map[string]any, 0, len(noms))
		for _, nom := range noms {
			payload = append(payload, map[string]any{
				"name": nom, "full_name": match[1] + "/" + nom,
			})
		}
		s.send(writer, 200, payload)
		return true
	}
	if match := teamRe.FindStringSubmatch(path); match != nil {
		equipe, connue := state.Teams[match[1]+"/"+match[2]]
		if !connue {
			s.notFound(writer)
			return true
		}
		s.send(writer, 200, teamPayload(equipe))
		return true
	}
	return false
}

// teamsPost répond à la création d'une équipe.
func (s *Server) teamsPost(writer http.ResponseWriter, path string, body map[string]any) bool {
	match := orgTeamsRe.FindStringSubmatch(path)
	if match == nil {
		return false
	}
	state := s.State
	org := match[1]
	nom, _ := body["name"].(string)
	slug := TeamSlug(nom)
	if slug == "" {
		s.send(writer, 422, map[string]string{"message": "Name is invalid"})
		return true
	}
	if _, pris := state.Teams[org+"/"+slug]; pris {
		s.send(writer, 422, map[string]any{
			"message": "Validation Failed",
			"errors":  []map[string]string{{"message": "Name must be unique for this org"}},
		})
		return true
	}
	description, _ := body["description"].(string)
	privacy, _ := body["privacy"].(string)
	equipe := &TeamState{
		Org: org, Slug: slug, Name: nom, Description: description,
		Privacy: privacy, Members: map[string]string{},
	}
	state.Teams[org+"/"+slug] = equipe
	s.send(writer, 201, teamPayload(equipe))
	return true
}

// teamsPut répond à l'inscription d'un membre et au partage d'un dépôt.
func (s *Server) teamsPut(writer http.ResponseWriter, path string, body map[string]any) bool {
	state := s.State
	if match := teamMemberRe.FindStringSubmatch(path); match != nil {
		equipe, connue := state.Teams[match[1]+"/"+match[2]]
		if !connue {
			s.notFound(writer)
			return true
		}
		login := match[3]
		if _, existe := state.Users[strings.ToLower(login)]; !existe {
			s.send(writer, 422, map[string]string{"message": "Invalid user"})
			return true
		}
		role, _ := body["role"].(string)
		if role == "" {
			role = "member"
		}
		equipe.Members[strings.ToLower(login)] = role
		s.send(writer, 200, map[string]any{"role": role, "state": "active"})
		return true
	}
	return false
}

// teamsPatch répond au renommage d'une équipe.
func (s *Server) teamsPatch(writer http.ResponseWriter, path string, body map[string]any) bool {
	match := teamRe.FindStringSubmatch(path)
	if match == nil {
		return false
	}
	state := s.State
	cle := match[1] + "/" + match[2]
	equipe, connue := state.Teams[cle]
	if !connue {
		s.notFound(writer)
		return true
	}
	if description, donnee := body["description"].(string); donnee {
		equipe.Description = description
	}
	nom, donne := body["name"].(string)
	if !donne || nom == "" || nom == equipe.Name {
		s.send(writer, 200, teamPayload(equipe))
		return true
	}
	slug := TeamSlug(nom)
	cible := equipe.Org + "/" + slug
	if _, pris := state.Teams[cible]; pris && cible != cle {
		s.send(writer, 422, map[string]any{
			"message": "Validation Failed",
			"errors":  []map[string]string{{"message": "Name must be unique for this org"}},
		})
		return true
	}
	// Le renommage change le slug : l'équipe déménage, avec ses dépôts.
	delete(state.Teams, cle)
	equipe.Name, equipe.Slug = nom, slug
	state.Teams[cible] = equipe
	if depots, connus := state.TeamRepos[cle]; connus && cible != cle {
		state.TeamRepos[cible] = depots
		delete(state.TeamRepos, cle)
	}
	s.send(writer, 200, teamPayload(equipe))
	return true
}

// teamsDelete répond à la suppression d'une équipe et au retrait d'un membre.
func (s *Server) teamsDelete(writer http.ResponseWriter, path string) bool {
	state := s.State
	if match := teamMemberRe.FindStringSubmatch(path); match != nil {
		equipe, connue := state.Teams[match[1]+"/"+match[2]]
		if !connue {
			s.notFound(writer)
			return true
		}
		delete(equipe.Members, strings.ToLower(match[3]))
		writer.WriteHeader(204)
		return true
	}
	if match := teamRe.FindStringSubmatch(path); match != nil {
		cle := match[1] + "/" + match[2]
		if _, connue := state.Teams[cle]; !connue {
			s.notFound(writer)
			return true
		}
		delete(state.Teams, cle)
		delete(state.TeamRepos, cle)
		state.DeletedTeams = append(state.DeletedTeams, cle)
		writer.WriteHeader(204)
		return true
	}
	return false
}

func teamPayload(equipe *TeamState) map[string]any {
	return map[string]any{
		"slug":        equipe.Slug,
		"name":        equipe.Name,
		"description": equipe.Description,
		"privacy":     equipe.Privacy,
	}
}
