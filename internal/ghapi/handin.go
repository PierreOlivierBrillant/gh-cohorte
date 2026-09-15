package ghapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
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
// « commits » comble ce manque. Une page pleine porte les cent commits les plus
// récents, chacun avec sa date et le compte qui l'a fait : de quoi dater la
// remise de chaque personne, et non seulement la dernière touche au dépôt. Un
// auteur qui n'y paraît pas a commis plus tôt que les cent derniers, et ne peut
// donc être le dernier de personne.
//
// Le nombre de commits, lui, ne se lit pas sur une page pleine : « Link »
// annonce des pages de cent, pas des commits. Tant que l'historique tient sur
// une page, les compter suffit ; au-delà, une seconde requête d'un commit par
// page rend le compte exact par son numéro de dernière page. Dérouler
// l'historique pour le savoir coûterait une requête par centaine de commits et
// n'apprendrait rien de plus.

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
	// Author est le compte GitHub derrière l'adresse du commit. Il est nul
	// quand elle ne mène à personne : une machine mal configurée en donne un.
	Author *struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"author"`
}

// at rend la date à retenir pour ce commit.
func (c commitTime) at() time.Time {
	if c.Commit.Committer.Date.After(c.Commit.Author.Date) {
		return c.Commit.Committer.Date
	}
	return c.Commit.Author.Date
}

// by rend le compte qui a fait le commit, en minuscules, et dit s'il compte.
// Un robot — « github-classroom[bot] », une action — écrit lui aussi : ce qu'il
// a fait n'est pas une remise, et ne doit dater celle de personne.
func (c commitTime) by() (string, bool) {
	if c.Author == nil {
		return "", true
	}
	if strings.EqualFold(c.Author.Type, "Bot") || strings.Contains(c.Author.Login, "[bot]") {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(c.Author.Login)), true
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
	total, recents, err := c.handinHead(owner, repo)
	if err != nil {
		return remise, err
	}
	// Le décompte des commits vient d'ici : « contributors » ne compte que les
	// commits qu'il sait attribuer, et GitHub s'arrête aux cinq cents premières
	// adresses. Le nombre de pages de l'historique, lui, est exact.
	remise.Commits = total
	for index, item := range recents {
		quand := item.at()
		if index == 0 {
			remise.Last = quand.UTC().Format(time.RFC3339)
		}
		login, compte := item.by()
		if !compte {
			continue
		}
		if remise.LastBy == nil {
			remise.LastBy = map[string]string{}
		}
		// Les commits viennent du plus récent au plus ancien, mais une date
		// d'auteur écrite à la main peut rompre cet ordre : c'est la plus
		// tardive qui date le passage de la personne.
		if fixe := quand.UTC().Format(time.RFC3339); fixe > remise.LastBy[login] {
			remise.LastBy[login] = fixe
		}
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

// handinHead rend le nombre de commits de la branche par défaut et les plus
// récents d'entre eux, du plus récent au plus ancien.
func (c *Client) handinHead(owner, repo string) (int, []commitTime, error) {
	content, link, err := c.fetchPage(
		c.url(repoPath(owner, repo) + "/commits?per_page=" + strconv.Itoa(PageSize) + "&page=1"))
	if err != nil {
		if emptyRepo(err) {
			return 0, nil, nil
		}
		return 0, nil, err
	}
	var page []commitTime
	if err := json.Unmarshal(content, &page); err != nil {
		return 0, nil, &Error{Message: "Historique illisible : " + err.Error()}
	}
	if len(page) == 0 {
		return 0, nil, nil
	}
	// Tant que l'historique tient sur une page, il se compte de lui-même.
	if lastPage(link) <= 1 {
		return len(page), page, nil
	}
	total, err := c.commitCount(owner, repo)
	if err != nil {
		return 0, nil, err
	}
	return total, page, nil
}

// commitCount rend le nombre exact de commits de la branche par défaut. Une
// page par commit : le numéro de la dernière est le nombre de commits.
func (c *Client) commitCount(owner, repo string) (int, error) {
	_, link, err := c.fetchPage(c.url(repoPath(owner, repo) + "/commits?per_page=1&page=1"))
	if err != nil {
		return 0, err
	}
	if total := lastPage(link); total > 1 {
		return total, nil
	}
	return 1, nil
}

// emptyRepo dit qu'un dépôt n'a rien à raconter plutôt qu'il ait échoué : vide,
// GitHub répond 409 ; sans historique visible, 404.
func emptyRepo(err error) bool {
	var echec *Error
	return errors.As(err, &echec) &&
		(echec.Status == http.StatusConflict || echec.Status == http.StatusNotFound)
}
