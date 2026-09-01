package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/complete"
	tea "github.com/charmbracelet/bubbletea"
)

// séparateur est celui que portent les suggestions quand la saisie emploie
// celui du système — la barre oblique inversée sous Windows.
const séparateur = string(filepath.Separator)

// champDEssai monte le champ de saisie sur une arborescence connue.
func champDEssai(t *testing.T, question Question) *pathModel {
	t.Helper()
	console, _ := consoleDEssai()
	modele := newPathModel(console, question)
	modele.Update(tea.WindowSizeMsg{Width: 80})
	return modele
}

func consoleDEssai() (*Console, *strings.Builder) {
	tampon := &strings.Builder{}
	return NewConsoleFor(tampon), tampon
}

// arbo prépare des fichiers et des dossiers, et renvoie le dossier racine.
func arbo(t *testing.T) string {
	t.Helper()
	racine := t.TempDir()
	for _, nom := range []string{"cohorte.csv", "cohorte-hiver.csv", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(racine, nom), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, nom := range []string{"depart", "documents"} {
		if err := os.MkdirAll(filepath.Join(racine, nom), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	complete.Forget()
	return racine
}

func tabulation(modele *pathModel) { modele.Update(tea.KeyMsg{Type: tea.KeyTab}) }

func TestChampTabulationCompleteJusquAuPrefixeCommun(t *testing.T) {
	racine := arbo(t)
	modele := champDEssai(t, Question{
		Title:    "Fichier CSV",
		Default:  filepath.Join(racine, "coh"),
		Complete: complete.Path,
	})

	tabulation(modele)
	attendu := filepath.Join(racine, "cohorte")
	if valeur := modele.input.Value(); valeur != attendu {
		t.Fatalf("valeur = %q, attendu %q", valeur, attendu)
	}
	if modele.listed {
		t.Error("la première tabulation complète, elle ne liste pas encore")
	}
}

func TestChampDoubleTabulationListeLesPossibilites(t *testing.T) {
	racine := arbo(t)
	modele := champDEssai(t, Question{
		Title:    "Fichier CSV",
		Default:  filepath.Join(racine, "coh"),
		Complete: complete.Path,
	})

	tabulation(modele) // complète jusqu'à « cohorte »
	tabulation(modele) // plus rien à ajouter : les possibilités s'affichent
	if !modele.listed {
		t.Fatal("la seconde tabulation doit lister les possibilités")
	}
	vue := modele.View()
	if !strings.Contains(vue, "cohorte.csv") || !strings.Contains(vue, "cohorte-hiver.csv") {
		t.Errorf("liste attendue dans la vue :\n%s", vue)
	}
	// La liste ne montre que ce qui reste à choisir, pas le chemin entier.
	if strings.Contains(vue, filepath.Join(racine, "cohorte.csv")) {
		t.Errorf("les possibilités doivent être abrégées :\n%s", vue)
	}
}

func TestChampVideDoubleTabulationListeLeRepertoireCourant(t *testing.T) {
	racine := arbo(t)
	precedent, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(racine); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(precedent)
	complete.Forget()

	modele := champDEssai(t, Question{Title: "Fichier CSV", Complete: complete.Path})
	tabulation(modele)
	tabulation(modele)

	if !modele.listed {
		t.Fatal("le répertoire courant doit s'afficher")
	}
	vue := modele.View()
	for _, attendu := range []string{"cohorte.csv", "notes.txt", "depart" + séparateur, "documents" + séparateur} {
		if !strings.Contains(vue, attendu) {
			t.Errorf("« %s » absent de la vue :\n%s", attendu, vue)
		}
	}
}

func TestChampUneSeulePossibiliteEstAdoptee(t *testing.T) {
	racine := arbo(t)
	modele := champDEssai(t, Question{
		Title:    "Dossier",
		Default:  filepath.Join(racine, "dep"),
		Complete: complete.Dir,
	})

	tabulation(modele)
	attendu := filepath.Join(racine, "depart") + séparateur
	if valeur := modele.input.Value(); valeur != attendu {
		t.Fatalf("valeur = %q, attendu %q", valeur, attendu)
	}
	if modele.listed {
		t.Error("une possibilité unique n'a pas à être listée")
	}
}

func TestChampDossiersSeulement(t *testing.T) {
	racine := arbo(t)
	modele := champDEssai(t, Question{
		Title:    "Dossier",
		Default:  racine + string(filepath.Separator),
		Complete: complete.Dir,
	})

	tabulation(modele)
	tabulation(modele)
	vue := modele.View()
	if !strings.Contains(vue, "depart"+séparateur) || !strings.Contains(vue, "documents"+séparateur) {
		t.Errorf("dossiers attendus :\n%s", vue)
	}
	if strings.Contains(vue, "cohorte.csv") {
		t.Errorf("aucun fichier ne doit être proposé :\n%s", vue)
	}
}

func TestChampSansCorrespondance(t *testing.T) {
	racine := arbo(t)
	modele := champDEssai(t, Question{
		Title:    "Fichier",
		Default:  filepath.Join(racine, "zzz"),
		Complete: complete.Path,
	})

	tabulation(modele)
	if modele.message == "" {
		t.Error("l'absence de correspondance doit être signalée")
	}
	if valeur := modele.input.Value(); valeur != filepath.Join(racine, "zzz") {
		t.Errorf("la saisie ne doit pas changer : %q", valeur)
	}
}

func TestChampUneFrappeRangeLaListe(t *testing.T) {
	racine := arbo(t)
	modele := champDEssai(t, Question{
		Title:    "Fichier CSV",
		Default:  filepath.Join(racine, "coh"),
		Complete: complete.Path,
	})

	tabulation(modele)
	tabulation(modele)
	if !modele.listed {
		t.Fatal("la liste devait être affichée")
	}
	modele.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	if modele.listed || modele.tabs != 0 {
		t.Error("reprendre la saisie doit ranger la liste")
	}
}

func TestChampValidation(t *testing.T) {
	racine := arbo(t)
	modele := champDEssai(t, Question{
		Title:   "Fichier CSV",
		Default: filepath.Join(racine, "cohorte.csv"),
		Validate: func(value string) (string, error) {
			return strings.TrimSuffix(value, ".csv"), nil
		},
		Complete: complete.Path,
	})

	modele.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !modele.done || modele.aborted {
		t.Fatal("la question devait être validée")
	}
	if modele.value != filepath.Join(racine, "cohorte") {
		t.Errorf("valeur validée = %q", modele.value)
	}
}

func TestChampValeurRefuseeLaisseLaQuestionOuverte(t *testing.T) {
	modele := champDEssai(t, Question{
		Title:    "Compte GitHub",
		Default:  "-invalide-",
		Complete: complete.Path,
		Validate: func(string) (string, error) { return "", ErrAborted },
	})

	modele.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if modele.done {
		t.Fatal("une valeur refusée ne doit pas clore la question")
	}
	if modele.message == "" {
		t.Error("le motif du refus doit être affiché")
	}
}

func TestChampVideRefuseEtAutorise(t *testing.T) {
	modele := champDEssai(t, Question{Title: "Chemin", Complete: complete.Path})
	modele.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if modele.done || modele.message == "" {
		t.Error("une valeur vide doit être refusée")
	}

	facultatif := champDEssai(t, Question{Title: "Chemin", AllowEmpty: true, Complete: complete.Path})
	facultatif.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !facultatif.done || facultatif.value != "" {
		t.Error("une valeur vide doit être acceptée quand elle est facultative")
	}
}

func TestChampAnnulation(t *testing.T) {
	for _, touche := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		modele := champDEssai(t, Question{Title: "Chemin", Complete: complete.Path})
		modele.Update(tea.KeyMsg{Type: touche})
		if !modele.aborted || !modele.done {
			t.Errorf("la touche %v doit annuler", touche)
		}
	}
}

func TestChampAideAffichee(t *testing.T) {
	modele := champDEssai(t, Question{Title: "Chemin du fichier CSV", Complete: complete.Path})
	vue := modele.View()
	for _, attendu := range []string{"Chemin du fichier CSV", "⇥ complète", "⇥⇥ liste", "↵ valide"} {
		if !strings.Contains(vue, attendu) {
			t.Errorf("« %s » absent de la vue :\n%s", attendu, vue)
		}
	}
}

func TestChampDeuxTabulationsApresAvoirAtteintUnDossier(t *testing.T) {
	racine := t.TempDir()
	dossier := filepath.Join(racine, "cours")
	if err := os.MkdirAll(dossier, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, nom := range []string{"seance-1.md", "seance-2.md", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dossier, nom), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	complete.Forget()

	modele := champDEssai(t, Question{
		Title:    "Chemin",
		Default:  filepath.Join(racine, "cou"),
		Complete: complete.Path,
	})

	// La première tabulation atteint le dossier…
	tabulation(modele)
	if valeur := modele.input.Value(); valeur != dossier+séparateur {
		t.Fatalf("valeur = %q, attendu %q", valeur, dossier+séparateur)
	}
	// … et une seule de plus suffit à voir ce qu'il contient.
	tabulation(modele)
	if !modele.listed {
		t.Fatalf("la liste devait s'afficher (tabs = %d)", modele.tabs)
	}
	vue := modele.View()
	for _, attendu := range []string{"seance-1.md", "seance-2.md", "notes.txt"} {
		if !strings.Contains(vue, attendu) {
			t.Errorf("« %s » absent de la vue :\n%s", attendu, vue)
		}
	}
}

func TestChampTabulationSansEffetLeDitQuandMeme(t *testing.T) {
	racine := arbo(t)
	modele := champDEssai(t, Question{
		Title:    "Fichier CSV",
		Default:  racine + string(filepath.Separator),
		Complete: complete.Path,
	})

	// Rien à compléter : la tabulation doit tout de même donner signe de vie.
	tabulation(modele)
	if modele.listed {
		t.Fatal("la liste attend la tabulation suivante")
	}
	if !strings.Contains(modele.View(), "⇥ pour les lister") {
		t.Errorf("l'utilisateur doit savoir quoi faire :\n%s", modele.View())
	}
	tabulation(modele)
	if !modele.listed {
		t.Error("la liste devait s'afficher")
	}
}

func TestChampPossibiliteUniqueDejaAtteinte(t *testing.T) {
	racine := arbo(t)
	modele := champDEssai(t, Question{
		Title:    "Fichier CSV",
		Default:  filepath.Join(racine, "notes.txt"),
		Complete: complete.Path,
	})

	tabulation(modele)
	tabulation(modele)
	if modele.input.Value() != filepath.Join(racine, "notes.txt") {
		t.Errorf("la saisie ne doit pas changer : %q", modele.input.Value())
	}
	if !strings.Contains(modele.View(), "Aucune autre possibilité") {
		t.Errorf("vue = %s", modele.View())
	}
}
