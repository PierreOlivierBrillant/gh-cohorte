package scopes_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/scopes"
)

func TestUnionGardeCeQuiEstDejaAcquis(t *testing.T) {
	obtenu := scopes.Union([]string{"gist", "repo", "read:org"}, []string{"delete_repo", "repo"})
	attendu := []string{"repo", "read:org", "delete_repo", "gist"}
	if strings.Join(obtenu, ",") != strings.Join(attendu, ",") {
		t.Errorf("union = %v, attendu %v", obtenu, attendu)
	}
}

func TestMissingNommeCeQuiManque(t *testing.T) {
	absentes := scopes.Missing([]string{"repo", "read:org"}, []string{"repo", "workflow", "workflow"})
	if len(absentes) != 1 || absentes[0] != "workflow" {
		t.Errorf("manquantes = %v", absentes)
	}
}

func TestParseDecoupeLEnTete(t *testing.T) {
	liste := scopes.Parse("repo, read:org,  delete_repo ,")
	if len(liste) != 3 || liste[2] != "delete_repo" {
		t.Errorf("portées = %v", liste)
	}
}

func TestInventaireDitCeQuOnIgnore(t *testing.T) {
	// Un jeton « fine-grained » n'annonce rien : ni présente, ni absente.
	for _, portee := range scopes.Inventory(nil, false) {
		if portee.State != scopes.Unknown {
			t.Fatalf("%s : %q", portee.Name, portee.State)
		}
	}
	inventaire := scopes.Inventory([]string{"repo"}, true)
	if inventaire[0].State != scopes.Present {
		t.Errorf("repo : %q", inventaire[0].State)
	}
	if inventaire[len(inventaire)-1].State != scopes.Absent {
		t.Errorf("dernière portée : %q", inventaire[len(inventaire)-1].State)
	}
}

func TestValidateRefuseCeQuiNestPasUnePortee(t *testing.T) {
	// Un tiret de tête passerait pour un drapeau sur la ligne de commande de gh.
	for _, mauvaise := range []string{"--reset-scopes", "Repo", "repo;rm -rf /", ""} {
		if _, err := scopes.Validate(mauvaise); err == nil {
			t.Errorf("« %s » aurait dû être refusée", mauvaise)
		}
	}
	if _, err := scopes.Validate(" delete_repo "); err != nil {
		t.Errorf("delete_repo : %v", err)
	}
}

func TestCommandeEnonceCeQuIlFaudraitTaper(t *testing.T) {
	commande := scopes.Command("github.com", []string{"repo", "workflow"}, []string{"delete_repo"})
	attendu := "gh auth refresh --hostname github.com --scopes repo,workflow --remove-scopes delete_repo"
	if commande != attendu {
		t.Errorf("commande = %q", commande)
	}
}

// factice remplace gh sans rien lancer, et note ce qui lui aurait été demandé.
func factice(args *[]string, echec error) *scopes.Refresher {
	return &scopes.Refresher{
		Locate: func() (string, error) { return "/usr/bin/gh", nil },
		Run: func(_ context.Context, _ string, passes []string, _ scopes.Request) error {
			*args = passes
			return echec
		},
		Read: func(string) (string, string) { return "jeton-neuf", "oauth_token" },
	}
}

func TestRenouvellementAppelleGh(t *testing.T) {
	var args []string
	jeton, err := factice(&args, nil).Do(context.Background(), scopes.Request{
		Host: "github.com", Origin: "oauth_token",
		Add: []string{"delete_repo", "repo"},
	})
	if err != nil {
		t.Fatalf("renouvellement : %v", err)
	}
	if jeton != "jeton-neuf" {
		t.Errorf("jeton = %q", jeton)
	}
	if strings.Join(args, " ") != "auth refresh --hostname github.com --scopes repo,delete_repo" {
		t.Errorf("arguments = %v", args)
	}
}

func TestRenouvellementRefuseUnJetonDEnvironnement(t *testing.T) {
	var args []string
	_, err := factice(&args, nil).Do(context.Background(), scopes.Request{
		Origin: "GITHUB_TOKEN", Add: []string{"repo"},
	})
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("erreur = %v", err)
	}
	if args != nil {
		t.Errorf("gh n'aurait pas dû être lancé : %v", args)
	}
}

func TestRenouvellementRefuseDeRetirerLeSocle(t *testing.T) {
	var args []string
	_, err := factice(&args, nil).Do(context.Background(), scopes.Request{
		Add: []string{"workflow"}, Remove: []string{"repo"},
	})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Fatalf("erreur = %v", err)
	}
}

func TestRenouvellementRapporteLaCommandeQuandGhEchoue(t *testing.T) {
	var args []string
	refresher := factice(&args, errors.New("exit status 1"))
	_, err := refresher.Do(context.Background(), scopes.Request{
		Host: "github.com", Add: []string{"delete_repo"},
	})
	if err == nil || !strings.Contains(err.Error(), "gh auth refresh") {
		t.Fatalf("erreur = %v", err)
	}
}

func TestRenouvellementSansGhDonneLaCommande(t *testing.T) {
	refresher := &scopes.Refresher{
		Locate: func() (string, error) { return "", errors.New("introuvable") },
		Read:   func(string) (string, string) { return "", "" },
	}
	_, err := refresher.Do(context.Background(), scopes.Request{
		Host: "github.com", Add: []string{"delete_repo"},
	})
	if err == nil || !strings.Contains(err.Error(), "gh auth refresh --hostname github.com") {
		t.Fatalf("erreur = %v", err)
	}
}
