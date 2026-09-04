package classroom

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Une personne change de groupe en cours de session : c'est fréquent. Sa fiche
// suit toujours ; ses dépôts, eux, ne bougent que si on le demande — les
// renommer est une écriture sur GitHub, pas un rangement local.
//
// Le plan se compose entièrement avant que le premier renommage ne parte : une
// collision refuse ainsi le déplacement au lieu de l'interrompre à mi-chemin.

// Move est un dépôt à renommer parce que son étudiant change de groupe.
type Move struct {
	Repo     string `json:"repo"`
	Target   string `json:"target"`
	Student  string `json:"student"`
	Username string `json:"username"`
}

// PlanMove compose le renommage des dépôts des personnes qui passent d'un
// groupe à l'autre. Un groupe d'arrivée qui ne suit pas la nomenclature
// courante ne sait pas nommer : le déplacement se fait alors sans les dépôts.
func PlanMove(depart, arrivee Classroom, personnes []roster.Person,
	repos []groups.RepoInfo) ([]Move, error) {
	if arrivee.Legacy() {
		return nil, valid.Errorf(
			"« %s » suit une nomenclature dépassée : ses dépôts ne peuvent pas être "+
				"nommés. Renommez-les d'abord, ou déplacez sans les dépôts.",
			arrivee.Label())
	}

	fragments := map[string]string{}
	for _, personne := range personnes {
		fragment, err := naming.Student(personne.FullName)
		if err != nil {
			return nil, valid.Errorf(
				"Le nom complet de @%s manque : sans lui, ses dépôts ne peuvent pas être "+
					"renommés.", personne.Username)
		}
		fragments[strings.ToLower(personne.Username)] = fragment
	}

	existants := map[string]bool{}
	for _, repo := range repos {
		existants[strings.ToLower(repo.Name)] = true
	}

	var lignes []Move
	for _, travail := range depart.Assignments(repos) {
		nom, err := naming.Fragment(depart.ShortName(travail.ID), "Travail")
		if err != nil {
			continue
		}
		for _, repo := range depart.Repos(travail.ID, repos) {
			student, inscrit := depart.StudentOf(repo.Name)
			if !inscrit {
				continue
			}
			fragment, concerne := fragments[strings.ToLower(student.Username)]
			if !concerne {
				continue
			}
			cible := naming.Compose(arrivee.Session, arrivee.Course, arrivee.Group,
				nom, fragment)
			if existants[strings.ToLower(cible)] {
				return nil, valid.Errorf("« %s » existe déjà.", cible)
			}
			lignes = append(lignes, Move{
				Repo: repo.Name, Target: cible,
				Student: fragment, Username: student.Username,
			})
		}
	}
	return lignes, nil
}

// Without rend le groupe privé des comptes donnés.
func (c Classroom) Without(usernames ...string) Classroom {
	partants := map[string]bool{}
	for _, username := range usernames {
		partants[strings.ToLower(strings.TrimSpace(username))] = true
	}
	restants := make([]roster.Person, 0, len(c.Students))
	for _, personne := range c.Students {
		if partants[strings.ToLower(personne.Username)] {
			continue
		}
		restants = append(restants, personne)
	}
	c.Students = restants
	return c
}

// With rend le groupe augmenté des personnes données, sans doublon de compte.
func (c Classroom) With(people ...roster.Person) Classroom {
	c.Students = dedupe(append(append([]roster.Person(nil), c.Students...), people...))
	return c
}
