package corpus

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/rules"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Jusqu'où comparer.
//
// Comparer un groupe avec lui-même trouve les copies entre voisins de classe.
// Comparer les sessions passées trouve autre chose, et souvent davantage : le
// travail d'un ancien qui circule, la solution reprise d'une année sur l'autre.
// Ce sont deux questions différentes, et c'est pourquoi la portée se choisit
// plutôt que de s'imposer — la seconde coûte bien plus cher que la première.

// Reach dit jusqu'où le corpus s'étend.
type Reach string

const (
	// ReachGroup ne retient que les copies de ce groupe.
	ReachGroup Reach = "groupe"
	// ReachCourse retient tous les groupes du cours, dans cette session.
	ReachCourse Reach = "cours"
	// ReachYears retient toutes les sessions, sigles équivalents compris.
	ReachYears Reach = "annees"
)

// Reaches énumère les portées, de la plus étroite à la plus large.
var Reaches = []Reach{ReachGroup, ReachCourse, ReachYears}

// ReachLabels décrit chaque portée, pour les menus des trois interfaces.
var ReachLabels = map[Reach]string{
	ReachGroup:  "ce groupe seulement",
	ReachCourse: "tous les groupes du cours, cette session",
	ReachYears:  "toutes les sessions, anciens sigles compris",
}

// ParseReach valide une portée saisie. Sans valeur, c'est le groupe : c'est la
// moins chère, et celle qu'on veut neuf fois sur dix.
func ParseReach(value string) (Reach, error) {
	reach := Reach(strings.ToLower(strings.TrimSpace(value)))
	if reach == "" {
		return ReachGroup, nil
	}
	for _, connue := range Reaches {
		if connue == reach {
			return reach, nil
		}
	}
	noms := make([]string, 0, len(Reaches))
	for _, connue := range Reaches {
		noms = append(noms, string(connue))
	}
	return ReachGroup, valid.Errorf(
		"Portée de la comparaison : « %s » est inconnue (attendu : %s).",
		value, strings.Join(noms, ", "))
}

// Select dresse la liste des copies à comparer.
//
// Les équivalences de sigles s'appliquent ici, et nulle part ailleurs. C'est le
// seul endroit où l'on demande « quels dépôts », et c'est donc le seul où la
// question « 5N6 et 5M6 sont-ils le même cours » se pose.
func Select(org string, inventory []groups.RepoInfo, assignment string,
	reach Reach, declared rules.Rules) ([]Target, error) {

	scope, name, ok := naming.SplitAssignment(assignment)
	if !ok {
		return nil, valid.Errorf(
			"Travail « %s » : il faut son identifiant complet — « a26.5n6.01.tp1 ».",
			assignment)
	}
	niveaux := strings.Split(scope, naming.Separator)
	session, course, group := niveaux[0], niveaux[1], niveaux[2]

	sigles := set(declared.SameCourse(course))
	noms := set(declared.SameAssignment(name))

	cibles := make([]Target, 0, 32)
	for _, repo := range groups.Ordinary(inventory) {
		parts, reconnu := naming.Parse(repo.Name)
		if !reconnu || !noms[lower(parts.Assignment)] {
			continue
		}
		if !keep(parts, reach, session, group, sigles) {
			continue
		}
		place := naming.Prefix(parts.Session, parts.Course, parts.Group)
		cibles = append(cibles, Target{
			ID: repo.Name, Label: repo.Name, Origin: place,
			Owner: org, Repo: repo.Name,
		})
	}

	sort.Slice(cibles, func(first, second int) bool { return cibles[first].ID < cibles[second].ID })
	if len(cibles) < 2 {
		return nil, valid.Errorf(
			"Détection de plagiat : %d copie trouvée pour « %s » avec la portée « %s ». "+
				"Il en faut au moins deux.", len(cibles), assignment, reach)
	}
	return cibles, nil
}

// keep dit si un dépôt entre dans la portée demandée.
func keep(parts naming.Parts, reach Reach, session, group string, sigles map[string]bool) bool {
	if !sigles[lower(parts.Course)] {
		return false
	}
	switch reach {
	case ReachGroup:
		return strings.EqualFold(parts.Session, session) &&
			strings.EqualFold(parts.Group, group)
	case ReachCourse:
		return strings.EqualFold(parts.Session, session)
	}
	return true
}

// Places rend les places d'où viennent les copies, de la plus récente à la plus
// ancienne. C'est ce qui colore les nuages de points, et ce qui permet de dire
// « ces deux-là ne sont même pas de la même année ».
func Places(targets []Target) []string {
	vues := map[string]bool{}
	places := make([]string, 0, 8)
	for _, cible := range targets {
		if cible.Origin != "" && !vues[cible.Origin] {
			vues[cible.Origin] = true
			places = append(places, cible.Origin)
		}
	}
	sort.Slice(places, func(first, second int) bool {
		gauche := strings.SplitN(places[first], naming.Separator, 2)
		droite := strings.SplitN(places[second], naming.Separator, 2)
		if gauche[0] != droite[0] {
			return naming.CompareSessions(gauche[0], droite[0]) < 0
		}
		return places[first] < places[second]
	})
	return places
}

func set(values []string) map[string]bool {
	membres := make(map[string]bool, len(values))
	for _, value := range values {
		membres[lower(value)] = true
	}
	return membres
}

func lower(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
