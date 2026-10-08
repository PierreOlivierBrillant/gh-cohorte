package web_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-nestor/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-nestor/internal/registry"
)

// commetuveux monte le cas de l'issue : la même personne, @Mr-Commetuveux à
// l'automne et @commetuveuxx à l'hiver, chacun inscrit dans son groupe.
func commetuveux(t *testing.T) (*harnais, *fakegh.State) {
	t.Helper()
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jean-commetuveux", true).PushedAt = "2026-09-20T10:00:00Z"
	state.AddRepo("acme", "h27.4w6.01.tp1.jean-commetuveux", true).PushedAt = "2027-02-10T10:00:00Z"
	h := nouveau(t, state)
	h.groupe("a26", "5n6", "01", "Jean Commetuveux", "Mr-Commetuveux")
	h.groupe("h27", "4w6", "01", "Jean Commetuveux", "commetuveuxx")
	return h, state
}

// reunionRendue est ce que l'API rend d'une réunion.
type reunionRendue struct {
	Joining struct {
		Account   string   `json:"account"`
		Principal string   `json:"principal"`
		FullName  string   `json:"full_name"`
		Role      string   `json:"role"`
		Accounts  []string `json:"accounts"`
	} `json:"joining"`
	DryRun bool `json:"dry_run"`
}

// Le geste entier, depuis le navigateur : les candidats, l'aperçu qui n'écrit
// rien, la réunion, puis la séparation qui rend l'annuaire tel qu'il était.
func TestReunirPuisSeparerDepuisLeNavigateur(t *testing.T) {
	h, state := commetuveux(t)
	if rendu := h.annuaire(""); rendu.Total != 2 {
		t.Fatalf("avant : %s", comptesDe(rendu))
	}

	// L'homonyme est proposé d'abord, et signalé comme tel.
	var candidats struct {
		Candidates []struct {
			Username string `json:"username"`
			SameName bool   `json:"same_name"`
		} `json:"candidates"`
	}
	h.json(http.MethodGet, "/api/users/Mr-Commetuveux/same-as", nil, &candidats)
	if len(candidats.Candidates) != 1 || candidats.Candidates[0].Username != "commetuveuxx" ||
		!candidats.Candidates[0].SameName {
		t.Fatalf("candidats = %+v", candidats)
	}

	// L'aperçu dit ce qui l'emporte sans rien écrire.
	var apercu reunionRendue
	h.json(http.MethodPost, "/api/users/Mr-Commetuveux/same-as",
		map[string]any{"same_as": "commetuveuxx", "dry_run": true}, &apercu)
	if !apercu.DryRun || apercu.Joining.Principal != "commetuveuxx" ||
		strings.Join(apercu.Joining.Accounts, ",") != "commetuveuxx,Mr-Commetuveux" {
		t.Fatalf("aperçu = %+v", apercu)
	}
	if rendu := h.annuaire(""); rendu.Total != 2 {
		t.Fatalf("l'aperçu ne doit rien écrire : %s", comptesDe(rendu))
	}

	var reunion reunionRendue
	h.json(http.MethodPost, "/api/users/Mr-Commetuveux/same-as",
		map[string]any{"same_as": "commetuveuxx"}, &reunion)
	rendu := h.annuaire("")
	if rendu.Total != 1 || rendu.Users[0].Username != "commetuveuxx" ||
		len(rendu.Users[0].Enrollments) != 2 || rendu.Users[0].Repos != 2 {
		t.Fatalf("après : %+v", rendu.Users)
	}
	// La réunion vit au registre, sous forme de renvoi.
	contenu := state.Files("acme/"+registry.RepoName, registry.Branch)[registry.UsersFile]
	if !strings.Contains(contenu, `"same_as": "commetuveuxx"`) {
		t.Fatalf("registre :\n%s", contenu)
	}

	// La fiche, ouverte par l'ancien compte, montre la personne entière.
	fiche := h.fiche("Mr-Commetuveux")
	if fiche.User.Username != "commetuveuxx" || fiche.User.Courses != 2 ||
		len(fiche.User.Accounts) != 2 {
		t.Fatalf("fiche = %+v", fiche.User)
	}

	h.json(http.MethodDelete, "/api/users/Mr-Commetuveux/same-as", nil, nil)
	if rendu := h.annuaire(""); rendu.Total != 2 {
		t.Fatalf("après séparation : %s", comptesDe(rendu))
	}
}

// Réunir est une décision d'enseignant : le refus se dit dans les mots de
// l'outil, et la page sait d'avance si elle doit proposer le geste.
func TestUnEtudiantNeReunitPas(t *testing.T) {
	h, _ := commetuveux(t)
	if reponse, contenu := h.coopter("commetuveuxx", true); reponse.StatusCode != http.StatusOK {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	reponse, contenu := h.requete(http.MethodPost, "/api/users/Mr-Commetuveux/same-as",
		map[string]any{"same_as": "commetuveuxx"})
	if reponse.StatusCode == http.StatusOK || !strings.Contains(string(contenu), "Seul un enseignant") {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	var rendu struct {
		MayDecide bool `json:"may_decide"`
	}
	h.json(http.MethodGet, "/api/users", nil, &rendu)
	if rendu.MayDecide {
		t.Error("la page ne doit pas proposer le geste à qui ne peut pas le faire")
	}
}

// Séparer un compte que rien ne réunit se refuse, sans rien écrire.
func TestSeparerUnCompteSeulSeRefuse(t *testing.T) {
	h, _ := commetuveux(t)
	reponse, contenu := h.requete(http.MethodDelete, "/api/users/commetuveuxx/same-as", nil)
	if reponse.StatusCode == http.StatusOK || !strings.Contains(string(contenu), "rien à séparer") {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
}
