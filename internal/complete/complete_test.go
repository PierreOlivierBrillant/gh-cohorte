package complete_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/complete"
)

// séparateur est celui que portent les suggestions quand la saisie emploie
// celui du système — la barre oblique inversée sous Windows.
const séparateur = string(filepath.Separator)

// arborescence prépare un dossier représentatif et renvoie son chemin.
func arborescence(t *testing.T) string {
	t.Helper()
	racine := t.TempDir()
	for _, nom := range []string{"cohorte.csv", "cohorte-hiver.csv", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(racine, nom), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, nom := range []string{"depart", "documents", ".cache"} {
		if err := os.MkdirAll(filepath.Join(racine, nom), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	complete.Forget()
	return racine
}

func contient(valeurs []string, attendu string) bool {
	for _, valeur := range valeurs {
		if valeur == attendu {
			return true
		}
	}
	return false
}

func TestSuggestFichiers(t *testing.T) {
	racine := arborescence(t)
	suggestions := complete.Suggest(filepath.Join(racine, "coh"), complete.Path)

	if len(suggestions) != 2 {
		t.Fatalf("suggestions = %v", suggestions)
	}
	for _, attendu := range []string{"cohorte.csv", "cohorte-hiver.csv"} {
		if !contient(suggestions, filepath.Join(racine, attendu)) {
			t.Errorf("« %s » absent de %v", attendu, suggestions)
		}
	}
}

func TestSuggestProlongeToujoursLaSaisie(t *testing.T) {
	racine := arborescence(t)
	saisie := filepath.Join(racine, "c")
	for _, suggestion := range complete.Suggest(saisie, complete.Path) {
		if !strings.HasPrefix(suggestion, saisie) {
			t.Errorf("« %s » ne prolonge pas « %s »", suggestion, saisie)
		}
	}
}

func TestSuggestDossiersSeulement(t *testing.T) {
	racine := arborescence(t)
	suggestions := complete.Suggest(racine+string(filepath.Separator), complete.Dir)

	if len(suggestions) != 2 {
		t.Fatalf("suggestions = %v", suggestions)
	}
	for _, suggestion := range suggestions {
		if !strings.HasSuffix(suggestion, séparateur) {
			t.Errorf("un dossier doit se terminer par « %s » : %s", séparateur, suggestion)
		}
		if strings.Contains(suggestion, ".csv") || strings.Contains(suggestion, "notes") {
			t.Errorf("un fichier s'est glissé dans les dossiers : %s", suggestion)
		}
	}
}

func TestSuggestIgnoreLesFichiersCaches(t *testing.T) {
	racine := arborescence(t)
	suggestions := complete.Suggest(racine+string(filepath.Separator), complete.Path)
	for _, suggestion := range suggestions {
		if strings.Contains(suggestion, ".cache") {
			t.Errorf("les fichiers cachés ne se proposent pas d'eux-mêmes : %v", suggestions)
		}
	}

	complete.Forget()
	// filepath.Join nettoierait le point : la saisie est construite telle quelle.
	demandes := complete.Suggest(racine+string(filepath.Separator)+".", complete.Path)
	if !contient(demandes, filepath.Join(racine, ".cache")+séparateur) {
		t.Errorf("un point saisi doit les faire apparaître : %v", demandes)
	}
}

func TestSuggestAucuneCorrespondance(t *testing.T) {
	racine := arborescence(t)
	if suggestions := complete.Suggest(filepath.Join(racine, "zzz"), complete.Path); len(suggestions) != 0 {
		t.Errorf("suggestions = %v", suggestions)
	}
}

func TestSuggestModeNone(t *testing.T) {
	racine := arborescence(t)
	if suggestions := complete.Suggest(racine, complete.None); suggestions != nil {
		t.Errorf("aucune complétion attendue : %v", suggestions)
	}
}

func TestSuggestTilde(t *testing.T) {
	maison, err := os.UserHomeDir()
	if err != nil {
		t.Skip("pas de dossier personnel")
	}
	entrées, err := os.ReadDir(maison)
	if err != nil || len(entrées) == 0 {
		t.Skip("dossier personnel illisible ou vide")
	}
	var visible string
	for _, entrée := range entrées {
		if !strings.HasPrefix(entrée.Name(), ".") {
			visible = entrée.Name()
			break
		}
	}
	if visible == "" {
		t.Skip("aucune entrée visible dans le dossier personnel")
	}

	// Les deux formes sont acceptées : celle du système, et la barre oblique
	// que l'on tape par habitude même là où elle n'est pas la sienne.
	for _, tête := range []string{"~/", "~" + séparateur} {
		complete.Forget()
		saisie := tête + visible[:1]
		suggestions := complete.Suggest(saisie, complete.Path)
		if len(suggestions) == 0 {
			t.Fatalf("aucune suggestion pour %q", saisie)
		}
		for _, suggestion := range suggestions {
			if !strings.HasPrefix(suggestion, tête) {
				t.Errorf("la forme « %s » doit être conservée : %s", tête, suggestion)
			}
		}
	}
}

// Une saisie à la barre oblique se prolonge sans changer de forme, y compris
// là où le système préfère l'autre séparateur : la suggestion remplace le
// champ de saisie, elle ne doit jamais en réécrire le début.
func TestSuggestConserveLaBarreObliqueSaisie(t *testing.T) {
	racine := arborescence(t)
	saisie := filepath.ToSlash(racine) + "/dep"
	complete.Forget()

	suggestions := complete.Suggest(saisie, complete.Path)
	if len(suggestions) != 1 {
		t.Fatalf("suggestions = %v", suggestions)
	}
	if attendu := filepath.ToSlash(racine) + "/depart/"; suggestions[0] != attendu {
		t.Errorf("suggestion = %q, attendu %q", suggestions[0], attendu)
	}
}

func TestSuggestCheminRelatif(t *testing.T) {
	racine := arborescence(t)
	precedent, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(racine); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(precedent)

	complete.Forget()
	if suggestions := complete.Suggest("coh", complete.Path); len(suggestions) != 2 {
		t.Errorf("chemin nu : %v", suggestions)
	}
	complete.Forget()
	suggestions := complete.Suggest("./coh", complete.Path)
	if len(suggestions) != 2 {
		t.Fatalf("chemin « ./ » : %v", suggestions)
	}
	for _, suggestion := range suggestions {
		if !strings.HasPrefix(suggestion, "./") {
			t.Errorf("« ./ » doit être conservé : %s", suggestion)
		}
	}
}

func TestSuggestSaisieDangereuseNExecuteRien(t *testing.T) {
	racine := arborescence(t)
	temoin := filepath.Join(racine, "temoin")

	// Une saisie qui ressemble à une commande ne doit jamais être confiée au shell.
	for _, saisie := range []string{
		"$(touch " + temoin + ")",
		"`touch " + temoin + "`",
		"x; touch " + temoin,
		"x && touch " + temoin,
	} {
		complete.Forget()
		if suggestions := complete.Suggest(saisie, complete.Path); len(suggestions) != 0 {
			t.Errorf("Suggest(%q) = %v", saisie, suggestions)
		}
		if _, err := os.Stat(temoin); err == nil {
			t.Fatalf("la saisie %q a été exécutée", saisie)
		}
	}
}

func TestSuggestSansShell(t *testing.T) {
	racine := arborescence(t)
	t.Setenv("COHORTE_NO_SHELL_COMPLETION", "1")
	complete.Forget()

	suggestions := complete.Suggest(filepath.Join(racine, "coh"), complete.Path)
	if len(suggestions) != 2 {
		t.Fatalf("la complétion native doit prendre le relais : %v", suggestions)
	}
}

func TestSuggestShellEtNatifSAccordent(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash absent")
	}
	racine := arborescence(t)
	saisie := filepath.Join(racine, "c")

	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("COHORTE_NO_SHELL_COMPLETION", "")
	complete.Forget()
	parLeShell := complete.Suggest(saisie, complete.Path)

	t.Setenv("COHORTE_NO_SHELL_COMPLETION", "1")
	complete.Forget()
	parLeNatif := complete.Suggest(saisie, complete.Path)

	if strings.Join(parLeShell, ",") != strings.Join(parLeNatif, ",") {
		t.Errorf("shell = %v, natif = %v", parLeShell, parLeNatif)
	}
}

func TestSuggestShellInconnuRetombeSurLeNatif(t *testing.T) {
	racine := arborescence(t)
	t.Setenv("SHELL", "/usr/bin/un-shell-qui-nexiste-pas")
	complete.Forget()

	if suggestions := complete.Suggest(filepath.Join(racine, "coh"), complete.Path); len(suggestions) != 2 {
		t.Errorf("suggestions = %v", suggestions)
	}
}

func TestSuggestMiseEnCache(t *testing.T) {
	racine := arborescence(t)
	saisie := filepath.Join(racine, "coh")
	premier := complete.Suggest(saisie, complete.Path)

	// Le fichier ajouté ensuite n'apparaît pas : la réponse vient du cache.
	if err := os.WriteFile(filepath.Join(racine, "cohorte-ete.csv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if second := complete.Suggest(saisie, complete.Path); len(second) != len(premier) {
		t.Errorf("cache ignoré : %v puis %v", premier, second)
	}
	complete.Forget()
	if apres := complete.Suggest(saisie, complete.Path); len(apres) != 3 {
		t.Errorf("après oubli : %v", apres)
	}
}

func TestSuggestDossierIllisible(t *testing.T) {
	if suggestions := complete.Suggest("/nulle-part-du-tout/xyz", complete.Path); len(suggestions) != 0 {
		t.Errorf("suggestions = %v", suggestions)
	}
}
