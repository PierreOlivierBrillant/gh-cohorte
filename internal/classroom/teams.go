package classroom

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
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
	// Ils portent le nom que le registre de l'organisation leur donne, quand
	// il en connaît un : un compte qu'on sait nommer doit être nommé, même
	// s'il n'est inscrit nulle part.
	Strangers []roster.Person `json:"strangers"`
}

// Describe habille les équipes du groupe de ce que sa liste sait de leurs
// membres, dans l'ordre des équipes.
func (c Classroom) Describe(equipes []teams.Team) []TeamRoster {
	nommes := c.Named()
	fiches := make([]TeamRoster, 0, len(equipes))
	for _, equipe := range equipes {
		fiche := TeamRoster{
			Team:      equipe,
			People:    make([]roster.Person, 0, len(equipe.Members)),
			Strangers: make([]roster.Person, 0),
		}
		// Une personne dont deux comptes sont dans l'équipe n'y est qu'une
		// fois : elle y figurerait sinon deux fois sous le même nom, ce qui se
		// lit comme deux personnes.
		vues := map[string]bool{}
		for _, membre := range equipe.Members {
			personne, inscrit := c.Find(membre)
			if !inscrit {
				fiche.Strangers = append(fiche.Strangers, roster.Person{
					FullName: nommes[strings.ToLower(membre)], Username: membre,
				})
				continue
			}
			if vues[strings.ToLower(personne.Username)] {
				continue
			}
			vues[strings.ToLower(personne.Username)] = true
			// Les comptes retenus sont ceux par lesquels elle est dans
			// l'équipe : c'est ce qu'on retire en l'en retirant.
			fiche.People = append(fiche.People, roster.Person{
				FullName: personne.FullName, Username: membre,
				Also: autresDansLEquipe(personne, equipe, membre),
			})
		}
		SortPeople(fiche.People)
		SortPeople(fiche.Strangers)
		fiches = append(fiches, fiche)
	}
	return fiches
}

// Members nomme les membres d'une équipe : leur nom complet quand le groupe ou
// le registre le connaît, leur compte sinon. Un dépôt d'équipe ne porte le nom
// de personne ; dire qui il concerne n'a d'intérêt qu'en le disant par leur nom.
func (c Classroom) Members(equipe teams.Team) []roster.Person {
	nommes := c.Named()
	membres := make([]roster.Person, 0, len(equipe.Members))
	for _, compte := range equipe.Members {
		nom := nommes[strings.ToLower(compte)]
		if personne, inscrit := c.Find(compte); inscrit && personne.FullName != "" {
			nom = personne.FullName
		}
		membres = append(membres, roster.Person{FullName: nom, Username: compte})
	}
	SortPeople(membres)
	return membres
}

// autresDansLEquipe rend les autres comptes d'une personne que l'équipe porte
// aussi. Les taire ferait croire qu'en retirer un suffit à l'en sortir.
func autresDansLEquipe(personne roster.Person, equipe teams.Team,
	sauf string) []string {
	autres := make([]string, 0, 1)
	for _, compte := range personne.Accounts() {
		if strings.EqualFold(compte, sauf) || !equipe.Has(compte) {
			continue
		}
		autres = append(autres, compte)
	}
	if len(autres) == 0 {
		return nil
	}
	return autres
}

// Unassigned renvoie les étudiants du groupe qui ne sont dans aucune équipe :
// ceux qu'un travail d'équipe laisserait de côté.
func (c Classroom) Unassigned(equipes []teams.Team) []roster.Person {
	restants := make([]roster.Person, 0)
	for _, student := range c.Students {
		dedans := false
		for _, compte := range student.Accounts() {
			if _, membre := teams.Of(equipes, compte); membre {
				dedans = true
				break
			}
		}
		if dedans {
			continue
		}
		restants = append(restants, student)
	}
	return restants
}
