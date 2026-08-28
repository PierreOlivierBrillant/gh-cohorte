package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
)

func TestSaveEtLoad(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "config.json")
	settings := config.Default()
	settings.Org = "acme"
	settings.Assignment = "tp1"
	settings.Permission = "maintain"
	if err := settings.Save(chemin); err != nil {
		t.Fatalf("Save : %v", err)
	}

	relu := config.Load(chemin)
	if relu.Org != "acme" || relu.Assignment != "tp1" || relu.Permission != "maintain" {
		t.Fatalf("réglages relus = %+v", relu)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(chemin)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("permissions %o, attendu 600", mode)
		}
	}
}

func TestSaveNeContientJamaisDeJeton(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "config.json")
	settings := config.Default()
	settings.Org = "acme"
	if err := settings.Save(chemin); err != nil {
		t.Fatal(err)
	}
	contenu, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatal(err)
	}
	for _, interdit := range []string{"token", "jeton", "ghp_", "password"} {
		if strings.Contains(strings.ToLower(string(contenu)), interdit) {
			t.Errorf("le fichier de réglages contient « %s »", interdit)
		}
	}
}

func TestLoadFichierAbsentOuCorrompu(t *testing.T) {
	dossier := t.TempDir()
	defaut := config.Default()

	if relu := config.Load(filepath.Join(dossier, "absent.json")); relu != defaut {
		t.Errorf("fichier absent : %+v", relu)
	}

	corrompu := filepath.Join(dossier, "corrompu.json")
	if err := os.WriteFile(corrompu, []byte("{ ceci n'est pas du JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if relu := config.Load(corrompu); relu != defaut {
		t.Errorf("fichier corrompu : %+v", relu)
	}
}

func TestLoadNormaliseLesValeursAberrantes(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "config.json")
	contenu := `{"visibility":"secret","permission":"root","name_pattern":"","delay_seconds":-5}`
	if err := os.WriteFile(chemin, []byte(contenu), 0o600); err != nil {
		t.Fatal(err)
	}
	relu := config.Load(chemin)
	if relu.Visibility != "private" || relu.Permission != "push" {
		t.Errorf("valeurs non normalisées : %+v", relu)
	}
	if relu.NamePattern != config.DefaultNamePattern || relu.DelaySeconds != config.DefaultDelaySeconds {
		t.Errorf("valeurs non normalisées : %+v", relu)
	}
}

func TestPathRespecteXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	if chemin := config.Path(); chemin != filepath.Join("/tmp/xdg-test", "cohorte", "config.json") {
		t.Errorf("Path = %q", chemin)
	}
}

func TestValidations(t *testing.T) {
	if _, err := config.ValidatePermission("push"); err != nil {
		t.Errorf("push refusé : %v", err)
	}
	if _, err := config.ValidatePermission("root"); err == nil {
		t.Error("root doit être refusé")
	}
	if _, err := config.ValidateVisibility("public"); err != nil {
		t.Errorf("public refusé : %v", err)
	}
	if _, err := config.ValidateVisibility("secret"); err == nil {
		t.Error("secret doit être refusé")
	}
}
