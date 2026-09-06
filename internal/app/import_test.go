package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
)

// classroomOrg monte une organisation telle que GitHub Classroom la laisse :
// des dépôts « travail-compte », et rien qui dise à qui ils appartiennent.
func classroomOrg(t *testing.T) *fakegh.State {
	t.Helper()
	state := fakegh.NewState()
	for _, nom := range []string{
		"tp1-ladamlarocque", "tp1-felixbourassa", "tp1-lyonnais",
		"projet-final-ladamlarocque", "projet-final-lyonnais",
	} {
		state.AddRepo("acme", nom, true)
	}
	return state
}

// listeOmnivox écrit une liste telle qu'Omnivox l'exporte : Windows-1252, CRLF,
// champs blindés, et pas un seul compte GitHub.
func listeOmnivox(t *testing.T, dossier string, lignes ...string) string {
	t.Helper()
	entete := "No étudiant;Groupe;Nom de l'étudiant;Prénom de l'étudiant;Code perm.;"
	var octets []byte
	for _, ligne := range append([]string{entete}, lignes...) {
		for _, r := range ligne {
			octets = append(octets, byte(r))
		}
		octets = append(octets, '\r', '\n')
	}
	chemin := filepath.Join(dossier, "ListeEtudiants.csv")
	if err := os.WriteFile(chemin, octets, 0o600); err != nil {
		t.Fatal(err)
	}
	return chemin
}

func liste(t *testing.T) string {
	return listeOmnivox(t, t.TempDir(),
		`="1680229";="1030";="Adam-Larocque";="Laurent";="ADAL20059908";`,
		`="2143020";="1030";="Bourassa";="Félix";="BOUF68040412";`,
		`="1983429";="1030";="Lyonnais";="Étienne";="LYOE78040203";`,
	)
}

// Le parcours complet : des dépôts de GitHub Classroom, une liste d'Omnivox
// sans comptes, et un travail qui entre dans la nomenclature.
func TestImportDepuisGitHubClassroom(t *testing.T) {
	state := classroomOrg(t)
	h := nouveau(t, state)
	h.Options.ImportRequested = true
	h.Options.Import = "tp1"
	h.Options.Into = "a26.5n6.1030"
	h.Options.Roster = liste(t)
	h.Options.Yes = true

	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	noms := h.depots()
	for _, attendu := range []string{
		"a26.5n6.1030.tp1.laurent-adam-larocque",
		"a26.5n6.1030.tp1.felix-bourassa",
		"a26.5n6.1030.tp1.etienne-lyonnais",
	} {
		if !slices.Contains(noms, attendu) {
			t.Fatalf("« %s » manque : %v", attendu, noms)
		}
	}
	// L'autre travail n'a pas bougé : on n'importe que ce qu'on a demandé.
	if !slices.Contains(noms, "projet-final-lyonnais") {
		t.Fatalf("un travail non demandé a bougé : %v", noms)
	}

	// Les noms sont montés au registre : c'est là qu'ils vivent désormais.
	contenu := state.Files("acme/"+registry.RepoName, registry.Branch)[registry.StudentsFile]
	for _, attendu := range []string{"Laurent Adam-Larocque", "ladamlarocque", "Étienne Lyonnais"} {
		if !strings.Contains(contenu, attendu) {
			t.Fatalf("« %s » manque au registre :\n%s", attendu, contenu)
		}
	}
}

// La simulation montre tout et n'écrit rien.
func TestImportEnSimulationNEcritRien(t *testing.T) {
	state := classroomOrg(t)
	h := nouveau(t, state)
	h.Options.ImportRequested = true
	h.Options.Import = "tp1"
	h.Options.Into = "a26.5n6.1030"
	h.Options.Roster = liste(t)
	h.Options.DryRun = true

	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	// Le rapprochement est montré, avec la raison qui l'a produit.
	h.contient("Laurent Adam-Larocque", "→ a26.5n6.1030.tp1.", "Simulation")
	if noms := h.depots(); !slices.Contains(noms, "tp1-ladamlarocque") {
		t.Fatalf("un dépôt a été renommé en simulation : %v", noms)
	}
}

// Sans travail nommé, le mode script refuse plutôt que de deviner — mais il a
// d'abord montré ce que les dépôts dessinent.
func TestImportSansTravailListeLesCandidats(t *testing.T) {
	h := nouveau(t, classroomOrg(t))
	h.Options.ImportRequested = true
	if code := h.muet(); code != app.ExitValidation {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("tp1", "projet-final", "--import")
}

// Le travail peut prendre un autre nom au passage.
func TestImportRenommeLeTravailAuPassage(t *testing.T) {
	state := classroomOrg(t)
	h := nouveau(t, state)
	h.Options.ImportRequested = true
	h.Options.Import = "projet-final"
	h.Options.RenameTo = "Projet de session"
	h.Options.Into = "h27.5n6.1030"
	h.Options.Roster = liste(t)
	h.Options.Yes = true

	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if noms := h.depots(); !slices.Contains(noms,
		"h27.5n6.1030.projet-de-session.etienne-lyonnais") {
		t.Fatalf("dépôts = %v", noms)
	}
}
