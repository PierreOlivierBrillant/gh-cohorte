package app_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-milou/internal/app"
	"github.com/PierreOlivierBrillant/gh-milou/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-milou/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-milou/internal/roster"
	"github.com/PierreOlivierBrillant/gh-milou/internal/users"
	"github.com/PierreOlivierBrillant/gh-milou/internal/valid"
)

// Au terminal comme au navigateur, la colonne « Dernier envoi » ignore ce que
// l'enseignant a poussé dès que l'historique a été relevé, et le filtre sur
// l'envoi lit la même date.
func TestLeDernierEnvoiIgnoreLEnseignantAuTerminal(t *testing.T) {
	state := fakegh.NewState()
	depot := state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	depot.PushedAt = "2025-09-12T16:00:00Z"
	depot.History = []fakegh.HistoryEntry{
		{At: "2025-09-12T16:00:00Z", Login: "prof"},
		{At: "2025-09-05T10:00:00Z", Login: "emilie-cote"},
	}
	state.AddContributors("acme/a26.5n6.01.tp1.emilie-cote", "emilie-cote", "prof")

	h := nouveau(t, state)
	h.declarer(classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "01",
		Students: []roster.Person{{FullName: "Émilie Côté", Username: "emilie-cote"}},
	})
	h.Options.User = "prof"
	h.Options.TeacherSet, h.Options.Teacher = true, true
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("cooptation : code = %d\n%s", code, h.texte())
	}

	releve := nouveauDansLeMemeDossier(t, h)
	releve.Options.ManageRequested, releve.Options.Manage = true, "a26.5n6.01.tp1"
	releve.Options.Handins = true
	if code := releve.muet(); code != app.ExitOK {
		t.Fatalf("relevé : code = %d\n%s", code, releve.texte())
	}

	liste := nouveauDansLeMemeDossier(t, releve)
	liste.Options.ManageRequested, liste.Options.Manage = true, "a26.5n6.01.tp1"
	if code, _ := liste.script("quitter"); code != app.ExitOK {
		t.Fatalf("liste : code = %d\n%s", code, liste.texte())
	}
	liste.contient(valid.Moment("2025-09-05T10:00:00Z"))
	liste.absent(valid.Moment("2025-09-12T16:00:00Z"))

	filtree := nouveauDansLeMemeDossier(t, releve)
	filtree.Options.ManageRequested, filtree.Options.Manage = true, "a26.5n6.01.tp1"
	filtree.Options.Filter = users.Filter{PushedAfter: "2025-09-10"}
	if code, _ := filtree.script("quitter"); code != app.ExitOK {
		t.Fatalf("filtre : code = %d\n%s", code, filtree.texte())
	}
	if !strings.Contains(filtree.texte(), "Aucun dépôt ne répond aux critères") {
		t.Errorf("le filtre retient Émilie pour le commit du prof :\n%s", filtree.texte())
	}
}
