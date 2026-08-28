package valid_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

func TestLoginAccepte(t *testing.T) {
	cas := map[string]string{
		"octocat":                        "octocat",
		"@octocat":                       "octocat",
		"  emilie-cote  ":                "emilie-cote",
		"https://github.com/jlpicard":    "jlpicard",
		"https://github.com/jlpicard/tp": "jlpicard",
		"github.com/aminata-d/":          "aminata-d",
		"a":                              "a",
		strings.Repeat("a", 39):          strings.Repeat("a", 39),
		"a-1-b":                          "a-1-b",
	}
	for entree, attendu := range cas {
		obtenu, err := valid.Login(entree, "")
		if err != nil {
			t.Fatalf("Login(%q) : erreur inattendue %v", entree, err)
		}
		if obtenu != attendu {
			t.Errorf("Login(%q) = %q, attendu %q", entree, obtenu, attendu)
		}
	}
}

func TestLoginRefuse(t *testing.T) {
	cas := []string{
		"", "   ", "@", "-octocat", "octocat-", "octo--cat", "octo_cat", "octo cat",
		"octo.cat", strings.Repeat("a", 40), "élise",
	}
	for _, entree := range cas {
		if obtenu, err := valid.Login(entree, ""); err == nil {
			t.Errorf("Login(%q) = %q, une erreur était attendue", entree, obtenu)
		} else if !valid.IsValidation(err) {
			t.Errorf("Login(%q) : erreur non reconnue comme validation", entree)
		}
	}
}

func TestFullName(t *testing.T) {
	if nom, err := valid.FullName("  Émilie   Côté "); err != nil || nom != "Émilie Côté" {
		t.Fatalf("FullName = %q, %v", nom, err)
	}
	if _, err := valid.FullName("   "); err == nil {
		t.Error("un nom vide doit être refusé")
	}
	if _, err := valid.FullName("Jean\x07Luc"); err == nil {
		t.Error("un caractère de contrôle doit être refusé")
	}
	if _, err := valid.FullName(strings.Repeat("é", 121)); err == nil {
		t.Error("un nom de 121 caractères doit être refusé")
	}
	if _, err := valid.FullName(strings.Repeat("é", 120)); err != nil {
		t.Errorf("un nom de 120 caractères doit être accepté : %v", err)
	}
}

func TestRepoName(t *testing.T) {
	for _, bon := range []string{"tp1-jlpicard", "a", "un_depot.v2", strings.Repeat("z", 100)} {
		if _, err := valid.RepoName(bon); err != nil {
			t.Errorf("RepoName(%q) refusé : %v", bon, err)
		}
	}
	for _, mauvais := range []string{"", ".", "..", "tp1/jlpicard", "tp 1", strings.Repeat("z", 101), "dépôt"} {
		if _, err := valid.RepoName(mauvais); err == nil {
			t.Errorf("RepoName(%q) accepté à tort", mauvais)
		}
	}
}

func TestRepoRef(t *testing.T) {
	cas := []string{
		"acme/modele-tp",
		"https://github.com/acme/modele-tp",
		"github.com/acme/modele-tp.git",
		"  acme/modele-tp/  ",
	}
	for _, entree := range cas {
		owner, repo, err := valid.RepoRef(entree)
		if err != nil || owner != "acme" || repo != "modele-tp" {
			t.Errorf("RepoRef(%q) = %q, %q, %v", entree, owner, repo, err)
		}
	}
	for _, mauvais := range []string{"", "acme", "acme/modele/tp", "/modele"} {
		if _, _, err := valid.RepoRef(mauvais); err == nil {
			t.Errorf("RepoRef(%q) accepté à tort", mauvais)
		}
	}
}

func TestSlugify(t *testing.T) {
	cas := map[string]string{
		"Émilie Côté":     "emilie-cote",
		"Jean-Luc Picard": "jean-luc-picard",
		"  TP 1  ":        "tp-1",
		"Aminata  Diallo": "aminata-diallo",
		"Ægis Ærø":        "gis-r",
		"":                "",
		"---":             "",
		"Œuvre":           "uvre",
	}
	for entree, attendu := range cas {
		if obtenu := valid.Slugify(entree); obtenu != attendu {
			t.Errorf("Slugify(%q) = %q, attendu %q", entree, obtenu, attendu)
		}
	}
}

func TestSlugFragment(t *testing.T) {
	if slug, err := valid.SlugFragment("TP 1", "Travail"); err != nil || slug != "tp-1" {
		t.Fatalf("SlugFragment = %q, %v", slug, err)
	}
	if _, err := valid.SlugFragment("###", "Travail"); err == nil {
		t.Error("un fragment sans caractère utilisable doit être refusé")
	}
	if _, err := valid.SlugFragment(strings.Repeat("a", 61), "Travail"); err == nil {
		t.Error("un fragment de 61 caractères doit être refusé")
	}
}
