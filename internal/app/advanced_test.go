package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
)

func TestOptionsAvanceesSansAuthentification(t *testing.T) {
	h := nouveau(t, nil)
	code, _ := h.script(
		"avance",       // Que voulez-vous faire ?
		"emplacements", // Afficher les emplacements
		"revenir",      // Retour au menu principal
		"quitter",      // Quitter
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Options avancées", "Réglages", "Cache", "Bilans")
	// Rien de tout cela n'exige de jeton : aucun appel réseau ne doit partir.
	if appels := h.State.AllCalls(); len(appels) != 0 {
		t.Errorf("appels réseau = %v", appels)
	}
}

func TestOptionsAvanceesViderLeCache(t *testing.T) {
	h := nouveau(t, nil)
	stockage := cache.NewIn(h.CacheDir, true)
	stockage.Set(cache.ReposKey("acme"), []string{"tp1-a"})

	code, _ := h.script("avance", "vider", "oui", "revenir", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("1 entrée(s) supprimée(s)")
	if _, err := os.Stat(filepath.Join(h.CacheDir, "cache.json")); !os.IsNotExist(err) {
		t.Error("le fichier de cache aurait dû disparaître")
	}
}

func TestOptionsAvanceesCacheDejaVide(t *testing.T) {
	h := nouveau(t, nil)
	code, _ := h.script("avance", "vider", "revenir", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Le cache est déjà vide")
}

func TestOptionsAvanceesPurgeAnnulee(t *testing.T) {
	h := nouveau(t, nil)
	stockage := cache.NewIn(h.CacheDir, true)
	stockage.Set("k", "v")

	code, _ := h.script("avance", "vider", "non", "revenir", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Annulé")
	if _, err := os.Stat(filepath.Join(h.CacheDir, "cache.json")); err != nil {
		t.Error("le cache devait rester intact")
	}
}

func TestOptionsAvanceesOublierLesReglages(t *testing.T) {
	h := nouveau(t, nil)
	if err := os.WriteFile(h.Reglages, []byte(`{"org":"acme","assignment":"tp1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _ := h.script("avance", "reglages", "oui", "revenir", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Réglages oubliés")
	if _, err := os.Stat(h.Reglages); !os.IsNotExist(err) {
		t.Error("le fichier de réglages aurait dû disparaître")
	}
}

func TestOptionsAvanceesAucunReglage(t *testing.T) {
	h := nouveau(t, nil)
	code, _ := h.script("avance", "reglages", "revenir", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Aucun réglage mémorisé")
}

func TestMenuPrincipalQuitter(t *testing.T) {
	h := nouveau(t, nil)
	code, _ := h.script("quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if appels := h.State.AllCalls(); len(appels) != 0 {
		t.Errorf("quitter ne doit rien demander à GitHub : %v", appels)
	}
}
