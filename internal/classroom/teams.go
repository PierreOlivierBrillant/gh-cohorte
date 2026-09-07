package classroom

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Un travail d'équipe, c'est un dépôt par équipe au lieu d'un dépôt par
// personne. Rien d'autre ne change : même nomenclature, même gabarit de nom,
// même plan. Le dernier niveau du nom porte l'équipe — « a26.5n6.01.tp1.eq1 » —
// là où un travail individuel porte l'étudiant.
//
// C'est ce qui permet de lire la nature d'un travail sans l'avoir déclarée
// nulle part : le dernier niveau nomme une équipe du groupe, ou il n'en nomme
// pas. Un travail fait en équipe avant l'arrivée de l'outil s'adopte donc en
// adoptant ses équipes — les dépôts, eux, n'ont rien à changer.

// Teams retient les équipes du groupe parmi celles de l'organisation.
func (c Classroom) Teams(infos []teams.Info) []teams.Team {
	return teams.In(c.Session, c.Course, c.Group, infos)
}

// TeamName compose le nom complet d'une équipe du groupe.
func (c Classroom) TeamName(short string) string {
	return naming.TeamName(c.Session, c.Course, c.Group, strings.TrimSpace(short))
}

// TeamOf retrouve l'équipe à laquelle un dépôt du groupe appartient.
func (c Classroom) TeamOf(repoName string, equipes []teams.Team) (teams.Team, bool) {
	parts, reconnu := naming.Parse(repoName)
	if !reconnu || !naming.Belongs(parts, c.Session, c.Course, c.Group) {
		return teams.Team{}, false
	}
	return teams.Find(equipes, parts.Student)
}

// ServedTeams renvoie les équipes qui ont déjà un dépôt pour ce travail. C'est
// l'équivalent de « Served » pour un travail d'équipe : ce qui a déjà été
// distribué ne l'est pas deux fois.
func (c Classroom) ServedTeams(assignmentID string, repos []groups.RepoInfo,
	equipes []teams.Team) map[string]bool {
	servis := map[string]bool{}
	for _, repo := range repos {
		parts, reconnu := naming.Parse(repo.Name)
		if !reconnu {
			continue
		}
		id := naming.AssignmentID(parts.Session, parts.Course, parts.Group, parts.Assignment)
		if !strings.EqualFold(id, assignmentID) {
			continue
		}
		if equipe, connue := teams.Find(equipes, parts.Student); connue {
			servis[strings.ToLower(equipe.Short)] = true
		}
	}
	return servis
}

// TeamTargets met les équipes sous la forme que le plan attend.
func TeamTargets(equipes []teams.Team) []plan.TeamTarget {
	cibles := make([]plan.TeamTarget, 0, len(equipes))
	for _, equipe := range equipes {
		cibles = append(cibles, plan.TeamTarget{
			Short: equipe.Short, Slug: equipe.Slug, Members: equipe.Members,
		})
	}
	return cibles
}

// TeamRoster décrit une équipe pour l'affichage : ses membres nommés d'après la
// liste du groupe, et ceux qui n'y figurent pas. Une équipe peut contenir
// quelqu'un qui a quitté le cours, ou que la liste n'a jamais connu : le taire
// laisserait croire que l'équipe est plus petite qu'elle n'est.
type TeamRoster struct {
	teams.Team
	// People nomme les membres que la liste du groupe connaît.
	People []roster.Person `json:"people"`
	// Strangers rassemble les comptes de l'équipe qui ne sont pas du groupe.
	Strangers []string `json:"strangers"`
}

// Describe habille les équipes du groupe de ce que sa liste sait de leurs
// membres, dans l'ordre des équipes.
func (c Classroom) Describe(equipes []teams.Team) []TeamRoster {
	fiches := make([]TeamRoster, 0, len(equipes))
	for _, equipe := range equipes {
		fiche := TeamRoster{
			Team:      equipe,
			People:    make([]roster.Person, 0, len(equipe.Members)),
			Strangers: make([]string, 0),
		}
		for _, membre := range equipe.Members {
			if personne, inscrit := c.Find(membre); inscrit {
				fiche.People = append(fiche.People, personne)
				continue
			}
			fiche.Strangers = append(fiche.Strangers, membre)
		}
		sort.Slice(fiche.People, func(i, j int) bool {
			return valid.Slugify(fiche.People[i].FullName+fiche.People[i].Username) <
				valid.Slugify(fiche.People[j].FullName+fiche.People[j].Username)
		})
		sort.Strings(fiche.Strangers)
		fiches = append(fiches, fiche)
	}
	return fiches
}

// Unassigned renvoie les étudiants du groupe qui ne sont dans aucune équipe :
// ceux qu'un travail d'équipe laisserait de côté.
func (c Classroom) Unassigned(equipes []teams.Team) []roster.Person {
	restants := make([]roster.Person, 0)
	for _, student := range c.Students {
		if _, membre := teams.Of(equipes, student.Username); membre {
			continue
		}
		restants = append(restants, student)
	}
	return restants
}
