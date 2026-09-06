package students

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
)

// L'annuaire répond à une question que la liste d'un groupe ne peut pas poser :
// qui sont les étudiants de l'organisation, et quels cours ont-ils suivis ?
//
// Un groupe ne connaît que les siens ; l'annuaire les rassemble tous et les
// fond par compte GitHub — c'est lui qui identifie une personne, et c'est
// ainsi qu'on voit qu'Émilie Côté a suivi le même cours à deux sessions.
//
// Il ne montre que les étudiants qu'on connaît : ceux des listes retenues. Un
// dépôt dont le dernier niveau ne correspond à personne n'est pas une personne
// de plus — c'est un nom slugifié, sans compte GitHub derrière. Les compter à
// part est plus honnête que d'inventer quelqu'un ; Unmatched le dit.

// Enrollment est un cours suivi : la place du groupe dans la hiérarchie, et ce
// que les dépôts de la personne y montrent.
type Enrollment struct {
	Scope       string
	Session     string
	SessionName string
	Course      string
	Group       string
	Label       string
	Repos       int
	// PushedAt est le plus récent envoi de la personne dans ce groupe.
	PushedAt string
}

// Directory dresse l'annuaire des étudiants de plusieurs groupes : une ligne
// par personne, avec les cours qu'elle a suivis. Les groupes viennent de
// l'appelant — déclarés ou lus dans les noms de dépôts, l'annuaire ne fait pas
// la différence.
func Directory(courses []classroom.Classroom, repos []groups.RepoInfo) []Row {
	parCompte := map[string]int{}
	annuaire := make([]Row, 0)
	for _, cours := range courses {
		for _, ligne := range Build(cours, repos) {
			cle := strings.ToLower(ligne.Username)
			if position, connu := parCompte[cle]; connu {
				annuaire[position] = fusionner(annuaire[position], ligne)
				continue
			}
			parCompte[cle] = len(annuaire)
			annuaire = append(annuaire, ligne)
		}
	}
	sort.SliceStable(annuaire, func(i, j int) bool {
		return fallback(annuaire[i], annuaire[j])
	})
	for position := range annuaire {
		sortEnrollments(annuaire[position].Enrollments)
	}
	return annuaire
}

// sortEnrollments range les cours suivis de la session la plus récente à la
// plus ancienne : c'est l'ordre où l'on cherche quelqu'un, et il est décidé
// ici pour que le terminal et le navigateur montrent le même.
func sortEnrollments(inscriptions []Enrollment) {
	sort.SliceStable(inscriptions, func(i, j int) bool {
		if rang := naming.CompareSessions(
			inscriptions[i].Session, inscriptions[j].Session); rang != 0 {
			return rang < 0
		}
		if inscriptions[i].Course != inscriptions[j].Course {
			return strings.ToLower(inscriptions[i].Course) <
				strings.ToLower(inscriptions[j].Course)
		}
		return strings.ToLower(inscriptions[i].Group) < strings.ToLower(inscriptions[j].Group)
	})
}

// Unmatched compte les dépôts que l'annuaire n'a pu rattacher à personne : ils
// suivent la nomenclature, mais leur dernier niveau ne désigne aucun étudiant
// connu. C'est ce qu'il manque pour que l'annuaire soit complet — le dire
// évite de lire une liste trouée comme si elle était entière.
func Unmatched(courses []classroom.Classroom, repos []groups.RepoInfo) int {
	orphelins := 0
	for _, cours := range courses {
		for _, travail := range cours.Assignments(repos) {
			orphelins += travail.Others
		}
	}
	return orphelins
}

// fusionner réunit deux lignes de la même personne, vues par deux groupes.
func fusionner(gauche, droite Row) Row {
	if gauche.FullName == "" {
		gauche.FullName = droite.FullName
	}
	gauche.Repos = append(gauche.Repos, droite.Repos...)
	gauche.Enrollments = append(gauche.Enrollments, droite.Enrollments...)
	if droite.PushedAt > gauche.PushedAt {
		gauche.PushedAt = droite.PushedAt
	}
	return gauche
}

// enrollmentOf décrit l'inscription d'une personne à un groupe. Elle vaut même
// sans dépôt : être du groupe est ce que la liste dit, pas ce que GitHub
// montre.
func enrollmentOf(cours classroom.Classroom, depots []Repo) Enrollment {
	inscription := Enrollment{
		Scope: cours.Scope(), Session: cours.Session, Course: cours.Course,
		Group: cours.Group, Label: cours.Label(), Repos: len(depots),
	}
	if cours.Session != "" {
		inscription.SessionName = cours.SessionName()
	}
	for _, depot := range depots {
		if depot.PushedAt > inscription.PushedAt {
			inscription.PushedAt = depot.PushedAt
		}
	}
	return inscription
}

// SessionsIn énumère les sessions de l'annuaire, de la plus récente à la plus
// ancienne — le même ordre que partout ailleurs.
func SessionsIn(rows []Row) []classroom.Session {
	courts := make([]string, 0, len(rows))
	for _, ligne := range rows {
		for _, inscription := range ligne.Enrollments {
			courts = append(courts, inscription.Session)
		}
	}
	return classroom.SessionsOf(courts)
}

// CoursesIn énumère les cours de l'annuaire, par ordre alphabétique. Une
// session donnée les restreint à ceux qu'elle porte : choisir une session puis
// un cours ne doit pas proposer un cours qui n'y a pas eu lieu.
func CoursesIn(rows []Row, session string) []string {
	vus := map[string]string{}
	for _, ligne := range rows {
		for _, inscription := range ligne.Enrollments {
			if inscription.Course == "" {
				continue
			}
			if session != "" && !strings.EqualFold(inscription.Session, session) {
				continue
			}
			vus[strings.ToLower(inscription.Course)] = inscription.Course
		}
	}
	trouves := make([]string, 0, len(vus))
	for _, cours := range vus {
		trouves = append(trouves, cours)
	}
	sort.Slice(trouves, func(i, j int) bool {
		return strings.ToLower(trouves[i]) < strings.ToLower(trouves[j])
	})
	return trouves
}
