package ghapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
)

// Relever ce qu'un dépôt a reçu tient en deux requêtes, et il faut les deux.
//
// « contributors » dit qui a écrit et combien de fois, sur tout l'historique et
// en une seule réponse, si long soit-il — un décompte exact sans dérouler quoi
// que ce soit. Il ne dit en revanche aucune date.
//
// « commits?per_page=1 » comble ce manque : la première page porte le commit le
// plus récent, et l'en-tête « Link » annonce du même coup le nombre de pages,
// donc le nombre de commits. Dérouler l'historique pour le savoir coûterait une
// requête par centaine de commits et n'apprendrait rien de plus.

// commitTime est la date d'un commit. GitHub en porte deux : celle de l'auteur,
// que « git commit --date » écrit à volonté, et celle du committer, posée au
// moment où le commit est fabriqué. La plus tardive des deux est retenue —
// antidater sa remise demande alors de mentir deux fois, et la seconde ne se
// fait pas sans y penser.
type commitTime struct {
	Commit struct {
		Author struct {
			Date  time.Time `json:"date"`
			Name  string    `json:"name"`
			Email string    `json:"email"`
		} `json:"author"`
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
}

// at rend la date à retenir pour ce commit.
func (c commitTime) at() time.Time {
	if c.Commit.Committer.Date.After(c.Commit.Author.Date) {
		return c.Commit.Committer.Date
	}
	return c.Commit.Author.Date
}

// Handin relève ce que l'historique d'un dépôt dit d'une remise.
//
// Un dépôt vide n'est pas une panne : GitHub répond 409 à qui demande ses
// commits, 204 à qui demande ses auteurs, et la remise rendue est simplement
// vide.
func (c *Client) Handin(owner, repo string) (groups.Handin, error) {
	remise, err := c.handinAuthors(owner, repo)
	if err != nil {
		return remise, err
	}
	total, dernier, err := c.handinHead(owner, repo)
	if err != nil {
		return remise, err
	}
	// Le décompte des commits vient d'ici : « contributors » ne compte que les
	// commits qu'il sait attribuer, et GitHub s'arrête aux cinq cents premières
	// adresses. Le nombre de pages de l'historique, lui, est exact.
	remise.Commits = total
	if !dernier.IsZero() {
		remise.Last = dernier.UTC().Format(time.RFC3339)
	}
	return remise, nil
}

// handinAuthors compte les commits de chaque auteur. « anon=1 » fait aussi
// paraître ceux dont l'adresse ne mène à aucun compte : un étudiant qui commet
// depuis une machine mal configurée en est, et le taire le ferait passer pour
// muet.
func (c *Client) handinAuthors(owner, repo string) (groups.Handin, error) {
	remise := groups.Handin{Authors: map[string]int{}}
	err := c.paginate(repoPath(owner, repo)+"/contributors?anon=1", nil,
		func(content []byte) (int, error) {
			// Un dépôt sans aucun commit répond 204, sans corps.
			if len(bytes.TrimSpace(content)) == 0 {
				return 0, nil
			}
			var page []struct {
				Login         string `json:"login"`
				Type          string `json:"type"`
				Name          string `json:"name"`
				Email         string `json:"email"`
				Contributions int    `json:"contributions"`
			}
			if err := json.Unmarshal(content, &page); err != nil {
				return 0, err
			}
			for _, item := range page {
				// Les robots — « github-classroom[bot] », les actions — écrivent
				// eux aussi : ce qu'ils ont fait n'est pas une remise.
				if strings.EqualFold(item.Type, "Bot") || strings.Contains(item.Login, "[bot]") {
					continue
				}
				if login := strings.TrimSpace(item.Login); login != "" {
					remise.Authors[strings.ToLower(login)] += item.Contributions
					continue
				}
				remise.Anonymous = append(remise.Anonymous, groups.Author{
					Name: item.Name, Email: item.Email, Commits: item.Contributions,
				})
			}
			return len(page), nil
		})
	if err != nil && !emptyRepo(err) {
		return remise, err
	}
	return remise, nil
}

// handinHead rend le nombre de commits de la branche par défaut et la date du
// plus récent.
func (c *Client) handinHead(owner, repo string) (int, time.Time, error) {
	base := c.url(repoPath(owner, repo) + "/commits?per_page=1")
	content, link, err := c.fetchPage(base + "&page=1")
	if err != nil {
		if emptyRepo(err) {
			return 0, time.Time{}, nil
		}
		return 0, time.Time{}, err
	}
	var page []commitTime
	if err := json.Unmarshal(content, &page); err != nil {
		return 0, time.Time{}, &Error{Message: "Historique illisible : " + err.Error()}
	}
	if len(page) == 0 {
		return 0, time.Time{}, nil
	}
	// Une page par commit : le numéro de la dernière est le nombre de commits.
	total := lastPage(link)
	if total < 1 {
		total = 1
	}
	return total, page[0].at(), nil
}

// emptyRepo dit qu'un dépôt n'a rien à raconter plutôt qu'il ait échoué : vide,
// GitHub répond 409 ; sans historique visible, 404.
func emptyRepo(err error) bool {
	var echec *Error
	return errors.As(err, &echec) &&
		(echec.Status == http.StatusConflict || echec.Status == http.StatusNotFound)
}
