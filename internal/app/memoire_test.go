package app_test

import (
	"os"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
)

func TestOrganisationMemoriseeApresUnParcoursComplet(t *testing.T) {
	h := nouveau(t, nil)
	h.Options.Org = ""
	csv := h.cohorteCSV("Émilie Côté,emilie-cote")

	code, _ := h.script(
		"creer", "acme", "fichier", csv, "oui", "tp1", "", "", "", "", "oui", "push", "oui",
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}

	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.Org = ""
	code, scripte := suivant.script("quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	_ = scripte
}

func TestOrganisationMemoriseeApresUneSortieSansCreation(t *testing.T) {
	h := nouveau(t, nil)
	h.Options.Org = ""

	// L'organisation est choisie, puis la session s'arrête sans rien créer.
	code, _ := h.script("gerer", "acme", "revenir")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}

	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.Org = ""
	code, scripte := suivant.script("gerer", "", "revenir")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	menu, trouve := scripte.MenuFor("Organisation GitHub")
	if !trouve {
		t.Fatal("la liste des organisations n'a pas été proposée")
	}
	if menu.Default != "acme" {
		t.Errorf("organisation proposée = %q, attendu « acme »", menu.Default)
	}
}

func TestOrganisationMemoriseeApresUneAnnulation(t *testing.T) {
	h := nouveau(t, nil)
	h.Options.Org = ""
	csv := h.cohorteCSV("Émilie Côté,emilie-cote")

	// L'assistant est mené jusqu'au récapitulatif, puis la création est refusée.
	code, _ := h.script(
		"creer", "acme", "fichier", csv, "oui", "tp1", "", "", "", "", "oui", "push", "non",
	)
	if code != app.ExitAborted {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}

	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.Org = ""
	if code, _ := suivant.script("gerer", "", "revenir"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	menu, trouve := suivant.dernierMenu("Organisation GitHub")
	if !trouve || menu.Default != "acme" {
		t.Errorf("organisation proposée = %q (trouvée : %v)", menu.Default, trouve)
	}
}

func TestOrganisationMemoriseeApresUneInterruption(t *testing.T) {
	h := nouveau(t, nil)
	h.Options.Org = ""

	// L'organisation est saisie, puis la session est interrompue.
	code, _ := h.script("creer", "acme", "\x03")
	if code != app.ExitAborted {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}

	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.Org = ""
	if code, _ := suivant.script("gerer", "", "revenir"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	menu, trouve := suivant.dernierMenu("Organisation GitHub")
	if !trouve || menu.Default != "acme" {
		t.Errorf("organisation proposée = %q (trouvée : %v)", menu.Default, trouve)
	}
}

func TestReglagesInchangesNeSontPasReecrits(t *testing.T) {
	h := nouveau(t, nil)
	if code, _ := h.script("quitter"); code != app.ExitOK {
		t.Fatalf("code = %d", code)
	}
	h.absent("Réglages mémorisés")
}

func TestOubliVolontaireNEstPasDefait(t *testing.T) {
	h := nouveau(t, nil)
	h.Options.Org = ""
	if code, _ := h.script("gerer", "acme", "revenir"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if reglages := config.Load(h.Reglages); reglages.Org != "acme" {
		t.Fatalf("l'organisation devait être mémorisée : %+v", reglages)
	}

	// L'oubli demandé ne doit pas être défait par l'enregistrement de fin de session.
	suivant := nouveauDansLeMemeDossier(t, h)
	if code, _ := suivant.script("avance", "reglages", "oui", "revenir", "quitter"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	suivant.contient("Réglages oubliés")
	if _, err := os.Stat(h.Reglages); !os.IsNotExist(err) {
		t.Errorf("le fichier de réglages est revenu : %v", err)
	}
}
