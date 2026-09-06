package web

import (
	"net/http"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/students"
)

// L'annuaire regarde l'organisation entière plutôt qu'un groupe : c'est la
// seule vue où « qui a suivi quoi » se lit d'un coup, une personne par ligne et
// ses sessions en face. Tout ce qu'il montre est calculé dans « students » —
// l'interface ne fait que transmettre les critères et rendre les lignes.

// directoryRow est un étudiant de l'organisation, avec les cours qu'il a suivis.
type directoryRow struct {
	FullName    string                `json:"full_name"`
	Username    string                `json:"username"`
	Enrollments []directoryEnrollment `json:"enrollments"`
	// Repos est le nombre de dépôts de la personne, tous groupes confondus.
	Repos    int    `json:"repos"`
	PushedAt string `json:"pushed_at,omitempty"`
}

// directoryEnrollment est un cours suivi, avec les dépôts qu'il a laissés.
type directoryEnrollment struct {
	Scope       string              `json:"scope"`
	Session     string              `json:"session,omitempty"`
	SessionName string              `json:"session_name,omitempty"`
	Course      string              `json:"course,omitempty"`
	Group       string              `json:"group,omitempty"`
	Label       string              `json:"label"`
	Assignments []studentAssignment `json:"assignments"`
	PushedAt    string              `json:"pushed_at,omitempty"`
}

// directoryQuery lit les critères de l'annuaire. Ce sont ceux de la liste d'un
// groupe, plus la session et le cours — les deux seuls qu'un groupe seul ne
// pouvait pas poser.
func directoryQuery(request *http.Request) (students.Filter, students.Key, bool, error) {
	valeurs := request.URL.Query()
	filtre, err := students.Filter{
		Text:         valeurs.Get("q"),
		Session:      valeurs.Get("session"),
		Course:       valeurs.Get("course"),
		PushedAfter:  valeurs.Get("after"),
		PushedBefore: valeurs.Get("before"),
		Activity:     students.Activity(valeurs.Get("activity")),
	}.Validate()
	if err != nil {
		return filtre, students.ByName, false, err
	}
	tri, err := students.ParseKey(valeurs.Get("sort"))
	if err != nil {
		return filtre, tri, false, err
	}
	return filtre, tri, valeurs.Get("desc") == "1", nil
}

// handleDirectory dresse l'annuaire des étudiants de l'organisation : une ligne
// par personne, tous groupes confondus, avec ce qu'elle a suivi.
func (s *Server) handleDirectory(writer http.ResponseWriter, request *http.Request) {
	org := s.org()
	filtre, tri, decroissant, err := directoryQuery(request)
	if err != nil {
		fail(writer, err)
		return
	}
	repos, source, err := s.repos(org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}

	visibles := s.visibles(org, repos)
	toutes := students.Directory(visibles, repos)
	retenues := students.Apply(toutes, filtre, tri, decroissant)

	lignes := make([]directoryRow, 0, len(retenues))
	for _, ligne := range retenues {
		lignes = append(lignes, s.directoryRow(org, ligne))
	}

	// Les sessions et les cours proposés viennent de l'annuaire entier, pas de
	// ce qui reste affiché : un filtre ne doit pas retirer de la liste ce qui
	// permettrait d'en sortir.
	writeJSON(writer, http.StatusOK, map[string]any{
		"students": lignes,
		"sessions": students.SessionsIn(toutes),
		"courses":  students.CoursesIn(toutes, filtre.Session),
		"total":    len(toutes), "shown": len(lignes),
		// Les dépôts que personne ne réclame : sans eux, une liste incomplète
		// se lirait comme si elle était entière.
		"unmatched": students.Unmatched(visibles, repos),
		"org":       org, "source": source,
	})
}

// directoryRow rassemble les dépôts d'une personne sous le groupe d'où ils
// viennent : deux groupes peuvent avoir chacun leur « tp1 ».
func (s *Server) directoryRow(org string, ligne students.Row) directoryRow {
	parPlace := map[string][]studentAssignment{}
	for _, depot := range ligne.Repos {
		parPlace[depot.Scope] = append(parPlace[depot.Scope], studentAssignment{
			Name: depot.Assignment, ID: depot.ID, Repo: depot.Name,
			URL:      s.urlOf(org, groups.Repo{Name: depot.Name, URL: depot.URL}),
			PushedAt: depot.PushedAt,
		})
	}

	inscriptions := make([]directoryEnrollment, 0, len(ligne.Enrollments))
	for _, inscription := range ligne.Enrollments {
		travaux := parPlace[inscription.Scope]
		if travaux == nil {
			travaux = []studentAssignment{}
		}
		inscriptions = append(inscriptions, directoryEnrollment{
			Scope: inscription.Scope, Session: inscription.Session,
			SessionName: inscription.SessionName, Course: inscription.Course,
			Group: inscription.Group, Label: inscription.Label,
			Assignments: travaux, PushedAt: inscription.PushedAt,
		})
	}
	return directoryRow{
		FullName: ligne.FullName, Username: ligne.Username,
		Enrollments: inscriptions, Repos: len(ligne.Repos), PushedAt: ligne.PushedAt,
	}
}
