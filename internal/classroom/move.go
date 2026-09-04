package classroom

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Une personne change de groupe en cours de session, ou son nom était mal
// orthographié : c'est fréquent. Sa fiche suit toujours ; ses dépôts, eux, ne
// bougent que si on le demande — les renommer est une écriture sur GitHub, pas
// un rangement local.
//
// Le plan se compose entièrement avant que le premier renommage ne parte : une
// collision refuse ainsi l'opération au lieu de l'interrompre à mi-chemin.

// Move est un dépôt à renommer parce que son étudiant a changé de groupe ou de
// nom.
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
		fragment, err := fragmentDe(personne)
		if err != nil {
			return nil, err
		}
		fragments[strings.ToLower(personne.Username)] = fragment
	}
	return planRenames(depart, arrivee, fragments, repos)
}

// PlanRenameStudent compose le renommage des dépôts d'une personne dont le nom
// complet change. Le nom de l'étudiant est le dernier niveau du nom d'un dépôt :
// corriger sa fiche ne suffit pas, les dépôts déjà créés portent l'ancien.
//
// La personne est désignée par le compte qu'elle avait : c'est lui qui la
// retrouve dans le groupe tel qu'il est encore.
func PlanRenameStudent(cours Classroom, avant, apres roster.Person,
	repos []groups.RepoInfo) ([]Move, error) {
	if cours.Legacy() {
		return nil, valid.Errorf(
			"« %s » suit une nomenclature dépassée : ses dépôts ne peuvent pas être "+
				"nommés. Renommez-les d'abord, ou renommez sans les dépôts.", cours.Label())
	}
	fragment, err := fragmentDe(apres)
	if err != nil {
		return nil, err
	}
	return planRenames(cours, cours,
		map[string]string{strings.ToLower(avant.Username): fragment}, repos)
}

// fragmentDe rend le fragment qui nomme une personne dans un dépôt.
func fragmentDe(personne roster.Person) (string, error) {
	fragment, err := naming.Student(personne.FullName)
	if err != nil {
		return "", valid.Errorf(
			"Le nom complet de @%s manque : sans lui, ses dépôts ne peuvent pas être "+
				"renommés.", personne.Username)
	}
	return fragment, nil
}

// planRenames compose les nouveaux noms : chaque dépôt du groupe de départ dont
// l'étudiant figure dans « fragments » rejoint la place d'arrivée sous le
// fragment qui lui est associé. Départ et arrivée peuvent être le même groupe —
// c'est le cas quand seul le nom de la personne change.
func planRenames(depart, arrivee Classroom, fragments map[string]string,
	repos []groups.RepoInfo) ([]Move, error) {
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
			// Un dépôt qui porte déjà le nom visé n'a rien à faire dans le
			// plan : il se heurterait à lui-même.
			if strings.EqualFold(cible, repo.Name) {
				continue
			}
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
