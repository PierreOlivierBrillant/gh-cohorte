package web_test

import (
	"net/http"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// travailAvecHistoriques monte un groupe de deux étudiantes, chacune avec son
// dépôt et son historique : l'une a remis en retard, l'autre n'a rien écrit.
func travailAvecHistoriques(t *testing.T) *harnais {
	t.Helper()
	state := fakegh.NewState()

	tard := state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	tard.History = []string{"2026-10-05T14:00:00Z", "2026-09-30T09:00:00Z"}
	state.AddContributors("acme/a26.5n6.01.tp1.emilie-cote", "ecote", "ecote")

	muet := state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	// Seuls les fichiers de départ, déposés par qui enseigne.
	muet.History = []string{"2026-09-01T08:00:00Z"}
	state.AddContributors("acme/a26.5n6.01.tp1.jean-luc-picard", "prof")

	return avantLeRegistre(t, state,
		cohorte("a26", "5n6", "01", "Émilie Côté", "ecote", "Jean-Luc Picard", "jlpicard"))
}

func TestDateCibleSeFixeEtParaitDansLeTravail(t *testing.T) {
	h := travailAvecHistoriques(t)

	var pose struct {
		Name string `json:"name"`
		Due  string `json:"due"`
	}
	h.json(http.MethodPut, "/api/classrooms/a26.5n6.01/assignments/tp1/deadline",
		map[string]any{"due": "2026-10-01"}, &pose)
	if pose.Due != "2026-10-01" {
		t.Fatalf("due = %q", pose.Due)
	}

	var fiche struct {
		Assignments []struct {
			Name string `json:"name"`
			Due  string `json:"due"`
		} `json:"assignments"`
	}
	h.json(http.MethodGet, "/api/classrooms/a26.5n6.01", nil, &fiche)
	if len(fiche.Assignments) != 1 || fiche.Assignments[0].Due != "2026-10-01" {
		t.Fatalf("le travail ne porte pas sa date cible : %+v", fiche.Assignments)
	}

	// Une date vide la retire : c'est la même décision prise dans l'autre sens.
	h.json(http.MethodPut, "/api/classrooms/a26.5n6.01/assignments/tp1/deadline",
		map[string]any{"due": ""}, &pose)
	if pose.Due != "" {
		t.Errorf("la date cible survit à son retrait : %q", pose.Due)
	}
}

// Une date fixée ici se lit là-bas : c'est tout l'objet de l'avoir mise dans
// l'organisation. Le second poste ne partage ni fichier local ni cache.
func TestUneDateCibleSeLitDepuisUnAutrePoste(t *testing.T) {
	h := travailAvecHistoriques(t)
	h.json(http.MethodPut, "/api/classrooms/a26.5n6.01/assignments/tp1/deadline",
		map[string]any{"due": "2026-10-01"}, nil)

	// Un collègue ouvre la même organisation, sans avoir rien déclaré.
	collegue := nouveau(t, h.State)
	var fiche struct {
		Assignments []struct {
			Name string `json:"name"`
			Due  string `json:"due"`
		} `json:"assignments"`
	}
	collegue.json(http.MethodGet, "/api/classrooms/a26.5n6.01", nil, &fiche)
	if len(fiche.Assignments) != 1 {
		t.Fatalf("travaux vus par le collègue : %+v", fiche.Assignments)
	}
	if fiche.Assignments[0].Due != "2026-10-01" {
		t.Errorf("le collègue ne voit pas l'échéance : %+v", fiche.Assignments[0])
	}
}

// Fixer une date n'oblige pas à déclarer le groupe : il existe par ses dépôts,
// et l'échéance va dans l'organisation, pas dans un fichier local.
func TestUneDateCibleSeFixeSurUnGroupeNonDeclare(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	h := nouveau(t, state)

	var pose struct {
		Due string `json:"due"`
	}
	h.json(http.MethodPut, "/api/classrooms/a26.5n6.01/assignments/tp1/deadline",
		map[string]any{"due": "2026-10-01"}, &pose)
	if pose.Due != "2026-10-01" {
		t.Fatalf("due = %q", pose.Due)
	}
	if local := h.declares("a26.5n6.01"); local != nil {
		t.Errorf("le groupe a été déclaré localement pour une date : %v", local)
	}
}

func TestDateCibleMalEcriteEstRefusee(t *testing.T) {
	h := travailAvecHistoriques(t)
	reponse, contenu := h.requete(http.MethodPut,
		"/api/classrooms/a26.5n6.01/assignments/tp1/deadline",
		map[string]any{"due": "le 1er octobre"})
	if reponse.StatusCode < 400 {
		t.Fatalf("statut %d, attendu un refus — %s", reponse.StatusCode, contenu)
	}
}

func TestReleveDesRemisesSignaleLeRetardEtLeSilence(t *testing.T) {
	h := travailAvecHistoriques(t)
	h.json(http.MethodPut, "/api/classrooms/a26.5n6.01/assignments/tp1/deadline",
		map[string]any{"due": "2026-10-01"}, nil)

	etat := h.travail(http.MethodPost,
		"/api/classrooms/a26.5n6.01/assignments/tp1/handins", nil)
	bilans, ok := etat["result"].([]any)
	if !ok || len(bilans) != 2 {
		t.Fatalf("bilans = %#v", etat["result"])
	}

	parNom := map[string]map[string]any{}
	for _, brut := range bilans {
		bilan := brut.(map[string]any)
		parNom[bilan["repo"].(string)] = bilan
	}

	tard := parNom["a26.5n6.01.tp1.emilie-cote"]
	if tard["late"] != true {
		t.Errorf("le dépôt remis le 5 octobre n'est pas en retard : %+v", tard)
	}
	if tard["commits"] != float64(2) {
		t.Errorf("commits = %v, attendu 2", tard["commits"])
	}
	if tard["silent"] != nil {
		t.Errorf("celle qui a écrit est dite muette : %+v", tard["silent"])
	}

	muet := parNom["a26.5n6.01.tp1.jean-luc-picard"]
	if muet["late"] == true {
		t.Errorf("le dépôt du 1er septembre est dit en retard : %+v", muet)
	}
	silencieux, _ := muet["silent"].([]any)
	if len(silencieux) != 1 {
		t.Fatalf("silent = %#v, attendu la seule personne visée", muet["silent"])
	}
	if silencieux[0].(map[string]any)["username"] != "jlpicard" {
		t.Errorf("silent = %#v", silencieux[0])
	}
}

func TestLesPastillesDunTravailSeLisentSansRienRedemander(t *testing.T) {
	h := travailAvecHistoriques(t)
	h.json(http.MethodPut, "/api/classrooms/a26.5n6.01/assignments/tp1/deadline",
		map[string]any{"due": "2026-10-01"}, nil)

	// Avant tout relevé, le travail ne prétend rien savoir.
	var avant struct {
		Assignments []struct {
			Seen    int `json:"seen"`
			Commits int `json:"commits"`
			Late    int `json:"late"`
			Silent  int `json:"silent"`
		} `json:"assignments"`
	}
	h.json(http.MethodGet, "/api/classrooms/a26.5n6.01", nil, &avant)
	if len(avant.Assignments) != 1 || avant.Assignments[0].Seen != 0 {
		t.Fatalf("un travail jamais relevé prétend savoir : %+v", avant.Assignments)
	}

	h.travail(http.MethodPost, "/api/classrooms/a26.5n6.01/handins", nil)

	var apres struct {
		Assignments []struct {
			Seen    int `json:"seen"`
			Commits int `json:"commits"`
			Late    int `json:"late"`
			Silent  int `json:"silent"`
		} `json:"assignments"`
	}
	h.json(http.MethodGet, "/api/classrooms/a26.5n6.01", nil, &apres)
	travail := apres.Assignments[0]
	if travail.Seen != 2 {
		t.Errorf("Seen = %d, attendu 2", travail.Seen)
	}
	if travail.Commits != 3 {
		t.Errorf("Commits = %d, attendu 3", travail.Commits)
	}
	if travail.Late != 1 {
		t.Errorf("Late = %d, attendu 1", travail.Late)
	}
	if travail.Silent != 1 {
		t.Errorf("Silent = %d, attendu 1", travail.Silent)
	}
}

func TestLaDateCibleSuitLeTravailRenomme(t *testing.T) {
	h := travailAvecHistoriques(t)
	h.json(http.MethodPut, "/api/classrooms/a26.5n6.01/assignments/tp1/deadline",
		map[string]any{"due": "2026-10-01"}, nil)

	h.travail(http.MethodPost, "/api/classrooms/a26.5n6.01/assignments/rename",
		map[string]any{"id": "a26.5n6.01.tp1", "name": "projet-final"})

	var fiche struct {
		Assignments []struct {
			Name string `json:"name"`
			Due  string `json:"due"`
		} `json:"assignments"`
	}
	h.json(http.MethodGet, "/api/classrooms/a26.5n6.01?refresh=1", nil, &fiche)
	if len(fiche.Assignments) != 1 {
		t.Fatalf("travaux = %+v", fiche.Assignments)
	}
	if fiche.Assignments[0].Name != "projet-final" || fiche.Assignments[0].Due != "2026-10-01" {
		t.Errorf("la date cible n'a pas suivi le renommage : %+v", fiche.Assignments[0])
	}
}
