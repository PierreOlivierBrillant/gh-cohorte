package app_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// groupeAvecHistoriques monte un travail de deux dépôts : l'un remis après la
// date cible, l'autre où l'étudiant n'a rien écrit.
func groupeAvecHistoriques(t *testing.T) *harnais {
	t.Helper()
	state := fakegh.NewState()

	tard := state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	tard.History = []string{"2026-10-05T14:00:00Z", "2026-09-30T09:00:00Z"}
	state.AddContributors("acme/a26.5n6.01.tp1.emilie-cote", "emilie-cote", "emilie-cote")

	muet := state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	muet.History = []string{"2026-09-01T08:00:00Z"}
	state.AddContributors("acme/a26.5n6.01.tp1.jean-luc-picard", "prof")

	h := nouveau(t, state)
	// Le silence se juge par rapport à quelqu'un : sans liste, un dépôt ne vise
	// personne et rien ne peut lui manquer.
	h.declarer(classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "01",
		Students: []roster.Person{
			{FullName: "Émilie Côté", Username: "emilie-cote"},
			{FullName: "Jean-Luc Picard", Username: "jlpicard"},
		},
	})
	h.Options.ManageRequested = true
	h.Options.Manage = "a26.5n6.01.tp1"
	return h
}

func TestDrapeauDueRetientLaDateCible(t *testing.T) {
	h := groupeAvecHistoriques(t)
	h.Options.DueSet = true
	h.Options.Due = "2026-10-01"

	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("est à remettre le 2026-10-01")
	// La date monte au registre de l'organisation, sous l'identifiant complet
	// du travail : c'est ce qui la rend vraie d'un poste à l'autre.
	contenu := travauxDuRegistre(h)
	if !strings.Contains(contenu, `"id": "a26.5n6.01.tp1"`) ||
		!strings.Contains(contenu, `"due": "2026-10-01"`) {
		t.Errorf("le registre ne porte pas l'échéance :\n%s", contenu)
	}
	// Elle n'est pas redite sur ce poste : deux exemplaires finiraient par
	// diverger, et c'est le registre qui fait foi.
	if local := h.groupesLocaux(); strings.Contains(local, "2026-10-01") {
		t.Errorf("le fichier local redit l'échéance :\n%s", local)
	}
}

// travauxDuRegistre rend le fichier des dates de remise de l'organisation.
func travauxDuRegistre(h *harnais) string {
	return string(h.State.Files("acme/"+registry.RepoName,
		registry.Branch)[registry.AssignmentsFile])
}

func TestDrapeauHandinsSignaleLeRetardEtLeSilence(t *testing.T) {
	h := groupeAvecHistoriques(t)
	h.Options.DueSet = true
	h.Options.Due = "2026-10-01"
	h.Options.Handins = true

	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient(
		"Date cible de « tp1 » : 2026-10-01",
		"Commits", "Remise",
		"en retard",
		// Le dépôt que seul l'enseignant a garni nomme celui qui n'a rien remis.
		"rien de",
	)
}

func TestDrapeauDueViderRetireLEcheance(t *testing.T) {
	h := groupeAvecHistoriques(t)
	h.Options.DueSet = true
	h.Options.Due = "2026-10-01"
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}

	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.DueSet = true
	suivant.Options.Due = ""
	if code := suivant.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	suivant.contient("n'a plus de date cible")
	if contenu := travauxDuRegistre(suivant); strings.Contains(contenu, "2026-10-01") {
		t.Errorf("l'échéance survit à son retrait :\n%s", contenu)
	}
}

func TestUneDateCibleSurUnPrefixeSansPlaceEstRefusee(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-jlpicard", true)
	h := nouveau(t, state)
	h.Options.ManageRequested = true
	h.Options.Manage = "tp1"
	h.Options.DueSet = true
	h.Options.Due = "2026-10-01"

	if code := h.muet(); code != app.ExitValidation {
		t.Fatalf("code = %d, attendu un refus\n%s", code, h.texte())
	}
	h.contient("ne dit pas à quel groupe il appartient")
}

func TestLaLigneDeCommandeRefuseUneDateQuiNEnEstPasUne(t *testing.T) {
	if _, err := app.Parse([]string{"--due", "le 1er octobre"}, nil); err == nil {
		t.Error("--due accepte « le 1er octobre »")
	}
	options, err := app.Parse([]string{"--due", "2026-10-01T23:59"}, nil)
	if err != nil {
		t.Fatalf("Parse : %v", err)
	}
	if !options.DueSet || options.Due != "2026-10-01T23:59" {
		t.Errorf("Due = %q, DueSet = %v", options.Due, options.DueSet)
	}
	// Le drapeau absent ne demande rien ; « --due "" » retire l'échéance.
	sans, err := app.Parse(nil, nil)
	if err != nil || sans.DueSet {
		t.Errorf("DueSet = %v sans drapeau", sans.DueSet)
	}
	vide, err := app.Parse([]string{"--due", ""}, nil)
	if err != nil || !vide.DueSet || vide.Due != "" {
		t.Errorf("« --due \"\" » : DueSet = %v, Due = %q", vide.DueSet, vide.Due)
	}
}
