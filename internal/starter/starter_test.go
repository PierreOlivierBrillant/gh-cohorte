package starter_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/starter"
)

// squelette crée un dossier de départ représentatif et renvoie son chemin.
func squelette(t *testing.T) string {
	t.Helper()
	racine := t.TempDir()
	fichiers := map[string]string{
		"README.md":                    "# Travail de session\n",
		".gitignore":                   "*.pyc\n",
		"src/main.py":                  "print('bonjour')\n",
		".github/workflows/tests.yml":  "name: tests\n",
		".git/HEAD":                    "ref: refs/heads/main\n",
		"__pycache__/main.cpython.pyc": "binaire",
		".DS_Store":                    "poubelle",
	}
	for chemin, contenu := range fichiers {
		complet := filepath.Join(racine, filepath.FromSlash(chemin))
		if err := os.MkdirAll(filepath.Dir(complet), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(complet, []byte(contenu), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return racine
}

func TestLoadRetientLesBonsFichiers(t *testing.T) {
	bundle, err := starter.Load(squelette(t))
	if err != nil {
		t.Fatalf("Load : %v", err)
	}
	var chemins []string
	for _, fichier := range bundle.Files {
		chemins = append(chemins, fichier.Path)
	}
	attendu := []string{".github/workflows/tests.yml", ".gitignore", "README.md", "src/main.py"}
	if strings.Join(chemins, ",") != strings.Join(attendu, ",") {
		t.Fatalf("fichiers = %v, attendu %v", chemins, attendu)
	}
	for _, fichier := range bundle.Files {
		if strings.HasPrefix(fichier.Path, ".git/") || strings.Contains(fichier.Path, "__pycache__") {
			t.Errorf("%s n'aurait pas dû être retenu", fichier.Path)
		}
	}
	if len(bundle.Skipped) != 1 || bundle.Skipped[0].Path != ".DS_Store" {
		t.Errorf("écartés = %+v", bundle.Skipped)
	}
	if !bundle.NeedsWorkflowScope() {
		t.Error("un fichier .github/workflows exige la portée workflow")
	}
	if bundle.IsLarge() {
		t.Error("ce petit dossier ne doit pas être jugé volumineux")
	}
}

func TestLoadModeExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("le bit d'exécution n'existe pas sous Windows")
	}
	racine := t.TempDir()
	script := filepath.Join(racine, "build.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(racine, "notes.txt"), []byte("rien"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle, err := starter.Load(racine)
	if err != nil {
		t.Fatalf("Load : %v", err)
	}
	modes := map[string]string{}
	for _, fichier := range bundle.Files {
		modes[fichier.Path] = fichier.Mode
	}
	if modes["build.sh"] != starter.ModeExecutable || modes["notes.txt"] != starter.ModeFile {
		t.Errorf("modes = %+v", modes)
	}
}

func TestLoadIgnoreLesLiensSymboliques(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("les liens symboliques demandent des droits sous Windows")
	}
	racine := t.TempDir()
	if err := os.WriteFile(filepath.Join(racine, "reel.txt"), []byte("contenu"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(racine, "reel.txt"), filepath.Join(racine, "lien.txt")); err != nil {
		t.Fatal(err)
	}
	bundle, err := starter.Load(racine)
	if err != nil {
		t.Fatalf("Load : %v", err)
	}
	if len(bundle.Files) != 1 || bundle.Files[0].Path != "reel.txt" {
		t.Fatalf("fichiers = %+v", bundle.Files)
	}
	if len(bundle.Skipped) != 1 || bundle.Skipped[0].Reason != "lien symbolique" {
		t.Errorf("écartés = %+v", bundle.Skipped)
	}
}

func TestLoadDossierVideOuAbsent(t *testing.T) {
	if _, err := starter.Load(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("un dossier absent doit être refusé")
	}
	if _, err := starter.Load(t.TempDir()); err == nil {
		t.Error("un dossier vide doit être refusé")
	}
	fichier := filepath.Join(t.TempDir(), "fichier.txt")
	if err := os.WriteFile(fichier, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := starter.Load(fichier); err == nil {
		t.Error("un fichier ordinaire doit être refusé")
	}
}

func TestLoadTropDeFichiers(t *testing.T) {
	racine := t.TempDir()
	for index := 0; index <= starter.MaxFiles; index++ {
		nom := filepath.Join(racine, "fichier-"+strings.Repeat("0", 3)+string(rune('a'+index%26))+"-"+itoa(index)+".txt")
		if err := os.WriteFile(nom, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := starter.Load(racine); err == nil {
		t.Error("un dossier de plus de 500 fichiers doit être refusé")
	} else if !strings.Contains(err.Error(), "dépôt modèle") {
		t.Errorf("le message doit suggérer un dépôt modèle : %v", err)
	}
}

func TestHumanSize(t *testing.T) {
	cas := map[int]string{0: "0 o", 512: "512 o", 1536: "1.5 Kio", 5 * 1024 * 1024: "5.0 Mio"}
	for taille, attendu := range cas {
		if obtenu := starter.HumanSize(taille); obtenu != attendu {
			t.Errorf("HumanSize(%d) = %q, attendu %q", taille, obtenu, attendu)
		}
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var chiffres []byte
	for value > 0 {
		chiffres = append([]byte{byte('0' + value%10)}, chiffres...)
		value /= 10
	}
	return string(chiffres)
}
