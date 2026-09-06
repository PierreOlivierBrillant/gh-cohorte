package app_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
)

// La règle du dépôt veut qu'une capacité existe dans les trois interfaces. Le
// registre en est une : distribuer depuis la ligne de commande doit y inscrire
// les personnes, et l'annuaire du terminal doit les y lire — même sur une
// machine qui n'a jamais rien déclaré.

// Distribuer depuis la ligne de commande inscrit la cohorte au registre.
func TestDistributionEnLigneDeCommandeInscritAuRegistre(t *testing.T) {
	state := fakegh.NewState()
	h := nouveau(t, state)
	h.Options.Assignment = "tp1"
	h.Options.Roster = h.cohorteCSV(
		"nom_complet,github_username",
		"Émilie Côté,emilie-cote",
		"Jean-Luc Picard,jlpicard",
	)
	h.Options.Yes = true
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}

	depot := state.Repos["acme/"+registry.RepoName]
	if depot == nil || !depot.Private {
		t.Fatalf("registre = %+v", depot)
	}
	contenu := state.Files("acme/"+registry.RepoName, registry.Branch)[registry.StudentsFile]
	for _, attendu := range []string{"emilie-cote", "Émilie Côté", "jlpicard"} {
		if !strings.Contains(contenu, attendu) {
			t.Fatalf("« %s » manque au registre :\n%s", attendu, contenu)
		}
	}
}

// Une simulation n'écrit rien, registre compris.
func TestSimulationNInscritRienAuRegistre(t *testing.T) {
	state := fakegh.NewState()
	h := nouveau(t, state)
	h.Options.Assignment = "tp1"
	h.Options.Roster = h.cohorteCSV("nom_complet,github_username", "Émilie Côté,emilie-cote")
	h.Options.DryRun = true
	h.Options.Yes = true
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if _, cree := state.Repos["acme/"+registry.RepoName]; cree {
		t.Error("une simulation ne doit rien écrire, le registre compris")
	}
}

// Un refus de confirmation non plus : rien n'est écrit sans un récapitulatif
// suivi d'un accord, et le registre ne fait pas exception.
func TestConfirmationRefuseeNInscritRienAuRegistre(t *testing.T) {
	state := fakegh.NewState()
	h := nouveau(t, state)
	h.Options.Assignment = "tp1"
	h.Options.Roster = h.cohorteCSV()
	code, _ := h.script(
		"oui",  // Vérifier les comptes ?
		"",     // Gabarit de nom
		"",     // Dépôt modèle
		"",     // Fichiers de départ
		"",     // Visibilité
		"oui",  // Inviter ?
		"push", // Droit
		"non",  // Confirmation finale
	)
	if code != app.ExitAborted {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if _, cree := state.Repos["acme/"+registry.RepoName]; cree {
		t.Error("le registre a été écrit malgré le refus")
	}
}

// L'annuaire du terminal lit le registre : une machine qui n'a jamais rien
// déclaré y voit quand même les noms.
//
// Le registre est ici déposé tel qu'une autre machine l'aurait écrit — ou tel
// qu'on l'aurait corrigé à la main sur github.com. C'est bien le cas à
// éprouver : rien de local ne dit qui sont ces gens.
func TestAnnuaireDuTerminalLitLeRegistre(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.jean-luc-picard",
	} {
		state.AddRepo("acme", nom, true)
	}
	state.AddRepo("acme", registry.RepoName, true)
	state.SeedCommit("acme/"+registry.RepoName, map[string]string{
		registry.StudentsFile: `{
  "version": 1,
  "students": [
    {"username": "emilie-cote", "full_name": "Émilie Côté", "slugs": ["emilie-cote"]},
    {"username": "jlpicard", "full_name": "Jean-Luc Picard", "slugs": ["jean-luc-picard"]}
  ]
}`,
	}, registry.Branch)

	h := nouveau(t, state)
	h.Options.StudentsRequested = true
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("annuaire : code = %d\n%s", code, h.texte())
	}
	h.contient("Émilie Côté", "Jean-Luc Picard")
	h.absent("Aucun étudiant connu")
}
