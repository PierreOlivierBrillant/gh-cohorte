package naming_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

func TestComposeEtRelecture(t *testing.T) {
	nom := naming.Compose("a26", "5n6", "01", "tp1", "emilie-cote")
	if nom != "a26.5n6.01.tp1.emilie-cote" {
		t.Fatalf("nom composé : %q", nom)
	}

	parts, reconnu := naming.Parse(nom)
	if !reconnu {
		t.Fatal("le nom composé ne se relit pas")
	}
	if parts.Session != "a26" || parts.Course != "5n6" || parts.Group != "01" ||
		parts.Assignment != "tp1" || parts.Student != "emilie-cote" {
		t.Fatalf("découpe : %+v", parts)
	}
}

func TestRelectureRefuseCeQuiNEstPasDeLaNomenclature(t *testing.T) {
	refuses := []string{
		"a26-5n6-tp1-emilie-cote", // ancienne nomenclature, tout en tirets
		"5n6.01.tp1.emilie-cote",  // un niveau de trop peu
		"a26.5n6.01.tp1.emilie.cote",
		"a26.5n6..tp1.emilie-cote", // un niveau vide
		"notes-du-cours",
		"",
	}
	for _, nom := range refuses {
		if _, reconnu := naming.Parse(nom); reconnu {
			t.Fatalf("« %s » a été accepté à tort", nom)
		}
	}
}

func TestLeSeparateurNePeutPasVenirDUnChamp(t *testing.T) {
	// Un travail, un cours, un groupe : tout passe par la slugification, qui
	// remplace le point par un tiret.
	cas := map[string]string{
		"tp1.bis":         "tp1-bis",
		"Travail.Session": "travail-session",
		"5N6.":            "5n6",
		"a26.01":          "a26-01",
	}
	for saisie, attendu := range cas {
		fragment, err := naming.Fragment(saisie, "Travail")
		if err != nil {
			t.Fatalf("« %s » : %v", saisie, err)
		}
		if fragment != attendu {
			t.Fatalf("« %s » → %q, attendu %q", saisie, fragment, attendu)
		}
		if strings.Contains(fragment, naming.Separator) {
			t.Fatalf("« %s » a laissé passer un séparateur", saisie)
		}
	}
}

func TestNomDEtudiantVenuDuCsv(t *testing.T) {
	// Un nom complet ponctué — « J.-P. Tremblay » — ne doit pas introduire de
	// séparateur dans le nom du dépôt.
	cas := map[string]string{
		"Émilie Côté":     "emilie-cote",
		"J.-P. Tremblay":  "j-p-tremblay",
		"Jean-Luc Picard": "jean-luc-picard",
		"O'Brien, Maëva":  "o-brien-maeva",
	}
	for complet, attendu := range cas {
		fragment, err := naming.Student(complet)
		if err != nil {
			t.Fatalf("« %s » : %v", complet, err)
		}
		if fragment != attendu {
			t.Fatalf("« %s » → %q, attendu %q", complet, fragment, attendu)
		}
		if strings.Contains(fragment, naming.Separator) {
			t.Fatalf("« %s » a laissé passer un séparateur", complet)
		}
	}
}

func TestNomCompletManquantRefuse(t *testing.T) {
	if _, err := naming.Student("   "); err == nil {
		t.Fatal("un nom complet vide a été accepté")
	} else if !valid.IsValidation(err) {
		t.Fatalf("erreur inattendue : %v", err)
	}
}

func TestAppartenanceAuGroupe(t *testing.T) {
	parts, _ := naming.Parse("A26.5N6.01.tp1.emilie-cote")
	if !naming.Belongs(parts, "a26", "5n6", "01") {
		t.Fatal("la casse ne devrait pas séparer un dépôt de son groupe")
	}
	if naming.Belongs(parts, "a26", "5n6", "02") {
		t.Fatal("un autre groupe a été reconnu")
	}
	if naming.Belongs(parts, "h27", "5n6", "01") {
		t.Fatal("une autre session a été reconnue")
	}
}

func TestIdentifiants(t *testing.T) {
	if prefixe := naming.Prefix("a26", "5n6", "01"); prefixe != "a26.5n6.01" {
		t.Fatalf("préfixe %q", prefixe)
	}
	if id := naming.AssignmentID("a26", "5n6", "01", "tp1"); id != "a26.5n6.01.tp1" {
		t.Fatalf("identifiant %q", id)
	}
}
