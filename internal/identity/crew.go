package identity

import (
	"sort"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
)

// Qui a fait un travail d'équipe ? Trois choses peuvent le dire, et aucune ne
// suffit seule.
//
//   - L'équipe GitHub à qui le dépôt est partagé. C'est ainsi que GitHub
//     Classroom procède : le dépôt n'a aucun collaborateur direct, et sa liste
//     d'accès revient vide. C'est la source la plus sûre quand elle existe.
//   - Les collaborateurs directs, quand quelqu'un a partagé le dépôt à la main.
//   - Les auteurs des commits. Ni accès ni équipe ne disent toujours qui a
//     travaillé ; ce qui a été poussé, si.
//
// Les trois se réunissent, et chacun garde d'où il vient : une composition
// devinée doit pouvoir être démentie, et pour cela il faut voir sur quoi elle
// repose.

// Sources d'un membre, de la plus sûre à la moins sûre.
const (
	FromTeam    = "équipe"
	FromAccess  = "accès"
	FromCommits = "commits"
)

// Member est une personne d'un dépôt d'équipe, et ce qui l'y a fait reconnaître.
type Member struct {
	Login  string `json:"login"`
	Source string `json:"source"`
	// Team est l'équipe GitHub qui l'a désigné, quand c'est elle. Sans quoi
	// une équipe partagée par tous les dépôts — celle qui enseigne — ne
	// pourrait pas être démêlée des équipes d'étudiants.
	Team string `json:"team,omitempty"`
}

// Crew est ce qu'on a appris des personnes d'un dépôt.
type Crew struct {
	Members []Member `json:"members"`
	// Teams nomme les équipes GitHub qui ont accès au dépôt. Une équipe
	// partagée par plusieurs dépôts du même travail n'est pas celle d'une
	// équipe d'étudiants : c'est celle qui enseigne.
	Teams []string `json:"teams"`
}

// Logins rend les comptes seuls, dans l'ordre où ils ont été reconnus.
func (c Crew) Logins() []string {
	comptes := make([]string, 0, len(c.Members))
	for _, membre := range c.Members {
		comptes = append(comptes, membre.Login)
	}
	return comptes
}

// Crews relève, pour chaque dépôt, qui l'a fait. Les dépôts déjà connus ne
// coûtent aucun appel ; les autres sont demandés en parallèle.
//
// Les équipes qui reviennent sur plus d'un dépôt sont écartées après coup : une
// équipe enseignante a accès à tout, et prendre ses membres pour ceux d'une
// équipe d'étudiants les mettrait dans toutes.
func (r *Resolver) Crews(org string, repos []string, viewer string,
	onProgress func(done, total int, repo string)) map[string]Crew {
	trouves := make(map[string]Crew, len(repos))
	var manquants []string
	for _, repo := range repos {
		var crew Crew
		if r.store.Get(cache.CrewKey(org, repo), cache.AccessTTL, &crew) {
			trouves[repo] = crew
			continue
		}
		manquants = append(manquants, repo)
	}
	if len(manquants) > 0 && r.client != nil {
		for repo, crew := range r.fetchCrews(org, manquants, onProgress) {
			trouves[repo] = crew
		}
	}
	return ecarterLesPartagees(trouves, viewer)
}

// fetchCrews interroge GitHub pour les dépôts qu'on ne connaît pas encore.
func (r *Resolver) fetchCrews(org string, repos []string,
	onProgress func(done, total int, repo string)) map[string]Crew {
	workers := r.jobs
	if workers > len(repos) {
		workers = len(repos)
	}
	type resultat struct {
		repo string
		crew Crew
	}
	file := make(chan string)
	sorties := make(chan resultat)
	var groupe sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		groupe.Add(1)
		go func() {
			defer groupe.Done()
			for repo := range file {
				sorties <- resultat{repo: repo, crew: r.crewOf(org, repo)}
			}
		}()
	}
	go func() {
		for _, repo := range repos {
			file <- repo
		}
		close(file)
		groupe.Wait()
		close(sorties)
	}()

	trouves := make(map[string]Crew, len(repos))
	appris := map[string]any{}
	faits := 0
	for item := range sorties {
		faits++
		trouves[item.repo] = item.crew
		// Un dépôt dont personne ne ressort est mémorisé aussi : c'est une
		// réponse, et y revenir coûterait les mêmes appels pour la même chose.
		appris[cache.CrewKey(org, item.repo)] = item.crew
		if onProgress != nil {
			onProgress(faits, len(repos), item.repo)
		}
	}
	r.store.SetMany(appris)
	return trouves
}

// crewOf interroge les trois sources d'un dépôt. Une source muette n'est pas
// une erreur : c'est la suivante qui répond.
func (r *Resolver) crewOf(org, repo string) Crew {
	crew := Crew{Members: make([]Member, 0, 3), Teams: make([]string, 0, 1)}
	vus := map[string]bool{}
	ajouter := func(login, source, equipe string) {
		login = strings.TrimSpace(login)
		if login == "" || vus[strings.ToLower(login)] {
			return
		}
		vus[strings.ToLower(login)] = true
		crew.Members = append(crew.Members,
			Member{Login: login, Source: source, Team: equipe})
	}

	if equipes, err := r.client.ListRepoTeams(org, repo); err == nil {
		for _, equipe := range equipes {
			crew.Teams = append(crew.Teams, equipe.Slug)
			membres, invites, err := r.client.ListTeamMembers(org, equipe.Slug)
			if err != nil {
				continue
			}
			// Une invitation en attente compte autant qu'une adhésion : la
			// personne a été mise dans l'équipe, elle n'a pas encore cliqué.
			for _, login := range append(membres, invites...) {
				ajouter(login, FromTeam, equipe.Slug)
			}
		}
	}
	// Un dépôt dont les accès ne se lisent pas garde ses deux autres sources :
	// les équipes et les commits disent déjà qui s'y trouve.
	if acces, err := r.accessOf(org, repo); err == nil {
		for _, login := range acces.Logins() {
			ajouter(login, FromAccess, "")
		}
	}
	if auteurs, err := r.client.ListContributors(org, repo); err == nil {
		for _, login := range auteurs {
			ajouter(login, FromCommits, "")
		}
	}
	return crew
}

// ecarterLesPartagees retire ce qui ne peut pas être une équipe d'étudiants :
// l'enseignant lui-même, et les membres d'une équipe GitHub que plusieurs
// dépôts du même travail partagent — celle qui enseigne, ou qui administre.
func ecarterLesPartagees(trouves map[string]Crew, viewer string) map[string]Crew {
	compte := map[string]int{}
	for _, crew := range trouves {
		for _, slug := range crew.Teams {
			compte[strings.ToLower(slug)]++
		}
	}
	// Avec un seul dépôt, rien ne distingue une équipe partagée d'une autre :
	// il n'y a personne à qui la comparer.
	partagees := map[string]bool{}
	if len(trouves) > 1 {
		for slug, fois := range compte {
			if fois > 1 {
				partagees[slug] = true
			}
		}
	}

	propres := make(map[string]Crew, len(trouves))
	for repo, crew := range trouves {
		garde := Crew{Members: make([]Member, 0, len(crew.Members)), Teams: crew.Teams}
		for _, membre := range crew.Members {
			if viewer != "" && strings.EqualFold(membre.Login, viewer) {
				continue
			}
			garde.Members = append(garde.Members, membre)
		}
		if len(partagees) > 0 {
			garde.Members = sansLesPartagees(garde.Members, partagees)
		}
		sort.SliceStable(garde.Members, func(i, j int) bool {
			return rang(garde.Members[i].Source) < rang(garde.Members[j].Source)
		})
		propres[repo] = garde
	}
	return propres
}

// sansLesPartagees retire les membres qu'une équipe partagée seule désignait.
// Quelqu'un qu'une autre source confirme reste : être dans l'équipe enseignante
// n'empêche pas d'avoir écrit dans le dépôt.
func sansLesPartagees(membres []Member, partagees map[string]bool) []Member {
	gardes := make([]Member, 0, len(membres))
	for _, membre := range membres {
		if membre.Source == FromTeam && partagees[strings.ToLower(membre.Team)] {
			continue
		}
		gardes = append(gardes, membre)
	}
	return gardes
}

// rang ordonne les sources de la plus sûre à la moins sûre.
func rang(source string) int {
	switch source {
	case FromTeam:
		return 0
	case FromAccess:
		return 1
	default:
		return 2
	}
}
