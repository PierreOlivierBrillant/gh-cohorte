package ghapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
)

// Les équipes d'organisation sont ce que GitHub Classroom utilisait pour les
// travaux d'équipe, et l'outil fait pareil : l'accès à un dépôt est accordé à
// l'équipe, pas à chacun de ses membres.
//
// Ces points d'API demandent la portée « admin:org » : lire les équipes se
// contente de « read:org », mais en créer, en renommer, en supprimer ou en
// changer la composition n'est possible qu'avec la première.

// ListTeamMembers renvoie les comptes GitHub inscrits dans une équipe.
func (c *Client) ListTeamMembers(org, slug string) ([]string, error) {
	var all []string
	path := "orgs/" + url.PathEscape(org) + "/teams/" + url.PathEscape(slug) + "/members"
	err := c.paginate(path, nil, func(content []byte) (int, error) {
		var page []User
		if err := json.Unmarshal(content, &page); err != nil {
			return 0, err
		}
		for _, item := range page {
			all = append(all, item.Login)
		}
		return len(page), nil
	})
	return all, err
}

// CreateTeam crée une équipe dans l'organisation et renvoie ce qu'elle est
// devenue : GitHub tire le « slug » du nom, et c'est lui qui l'adresse ensuite.
func (c *Client) CreateTeam(org, name, description, privacy string) (*teams.Info, error) {
	body := map[string]any{
		"name":        name,
		"description": description,
		"privacy":     privacy,
	}
	response, err := c.do(http.MethodPost, "orgs/"+url.PathEscape(org)+"/teams", body)
	if err != nil {
		return nil, err
	}
	return readTeam(response)
}

// UpdateTeam renomme une équipe et met sa description à jour. Le « slug »
// change avec le nom : c'est celui que GitHub renvoie qui vaut ensuite.
func (c *Client) UpdateTeam(org, slug, name, description string) (*teams.Info, error) {
	body := map[string]any{"name": name, "description": description}
	response, err := c.do(http.MethodPatch, teamPath(org, slug), body)
	if err != nil {
		return nil, err
	}
	return readTeam(response)
}

// DeleteTeam supprime définitivement une équipe. Les dépôts qu'elle voyait
// restent ; c'est l'accès qui disparaît.
func (c *Client) DeleteTeam(org, slug string) error {
	_, err := c.do(http.MethodDelete, teamPath(org, slug), nil, http.StatusNotFound)
	return err
}

// AddTeamMember inscrit une personne dans l'équipe. Le rôle « member » ne lui
// donne aucun droit sur l'équipe elle-même.
func (c *Client) AddTeamMember(org, slug, username, role string) error {
	path := teamPath(org, slug) + "/memberships/" + url.PathEscape(username)
	_, err := c.do(http.MethodPut, path, map[string]any{"role": role})
	return err
}

// RemoveTeamMember retire une personne de l'équipe.
func (c *Client) RemoveTeamMember(org, slug, username string) error {
	path := teamPath(org, slug) + "/memberships/" + url.PathEscape(username)
	_, err := c.do(http.MethodDelete, path, nil, http.StatusNotFound)
	return err
}

func readTeam(response *Response) (*teams.Info, error) {
	value := &Team{}
	if err := response.JSON(value); err != nil {
		return nil, &Error{Message: "Équipe illisible : " + err.Error()}
	}
	return &teams.Info{
		Slug: value.Slug, Name: value.Name, Description: value.Description,
	}, nil
}

func teamPath(org, slug string) string {
	return "orgs/" + url.PathEscape(org) + "/teams/" + url.PathEscape(strings.TrimSpace(slug))
}

// LoadOrgTeams lit les équipes de l'organisation et leur composition. Les
// membres se lisent une équipe à la fois : les requêtes partent en parallèle,
// car une organisation d'établissement peut en compter des centaines.
//
// Une équipe dont les membres restent illisibles n'arrête pas le tout : elle
// est rendue sans eux, et l'interface la montre vide plutôt que de refuser
// d'afficher quoi que ce soit.
func (c *Client) LoadOrgTeams(org string, jobs int) ([]teams.Info, error) {
	listees, err := c.ListOrgTeams(org)
	if err != nil {
		return nil, err
	}
	found := make([]teams.Info, 0, len(listees))
	for _, item := range listees {
		found = append(found, teams.Info{
			Slug: item.Slug, Name: item.Name, Description: item.Description,
		})
	}
	if jobs < 1 {
		jobs = 1
	}
	var wait sync.WaitGroup
	tickets := make(chan struct{}, jobs)
	for index := range found {
		wait.Add(1)
		tickets <- struct{}{}
		go func(position int) {
			defer wait.Done()
			defer func() { <-tickets }()
			members, err := c.ListTeamMembers(org, found[position].Slug)
			if err != nil {
				return
			}
			found[position].Members = members
		}(index)
	}
	wait.Wait()
	return found, nil
}
