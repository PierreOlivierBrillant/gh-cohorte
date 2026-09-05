package web_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/scopes"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/web"
)

// jeton est la description du jeton telle que la page la reçoit.
type jeton struct {
	Viewer      string `json:"viewer"`
	Origin      string `json:"origin"`
	Refreshable bool   `json:"refreshable"`
	Command     string `json:"command"`
	Scopes      []struct {
		Name    string `json:"name"`
		Label   string `json:"label"`
		Minimal bool   `json:"minimal"`
		State   string `json:"state"`
	} `json:"scopes"`
	Missing []string `json:"missing"`
}

// etat renvoie ce que le jeton annonce d'une portée.
func (j jeton) etat(nom string) string {
	for _, portee := range j.Scopes {
		if portee.Name == nom {
			return portee.State
		}
	}
	return ""
}

// faussaire imite « gh auth refresh » : il note l'appel et accorde les portées
// demandées, comme GitHub le ferait après un passage par le navigateur.
type faussaire struct {
	Appels []string
}

func (f *faussaire) refresher(state *fakegh.State) *scopes.Refresher {
	return &scopes.Refresher{
		Locate: func() (string, error) { return "gh", nil },
		Run: func(_ context.Context, _ string, args []string, _ scopes.Request) error {
			f.Appels = append(f.Appels, strings.Join(args, " "))
			state.Scopes = strings.Join(accordees(args), ", ")
			return nil
		},
		Read: func(string) (string, string) { return "jeton-renouvele", "oauth_token" },
	}
}

// accordees rejoue ce que gh ferait des drapeaux : ajouter, puis retirer.
func accordees(args []string) []string {
	var ajoutees, retirees []string
	for index, argument := range args {
		if index+1 >= len(args) {
			break
		}
		switch argument {
		case "--scopes":
			ajoutees = strings.Split(args[index+1], ",")
		case "--remove-scopes":
			retirees = strings.Split(args[index+1], ",")
		}
	}
	var restantes []string
	for _, nom := range ajoutees {
		if !scopes.Has(retirees, nom) {
			restantes = append(restantes, nom)
		}
	}
	return restantes
}

func TestJetonDecritSesPortees(t *testing.T) {
	state := fakegh.NewState()
	state.Scopes = "repo, read:org"
	h := nouveau(t, state)

	var vu jeton
	h.json(http.MethodGet, "/api/token", nil, &vu)
	if vu.etat("repo") != scopes.Present {
		t.Errorf("repo : %q", vu.etat("repo"))
	}
	if vu.etat("delete_repo") != scopes.Absent {
		t.Errorf("delete_repo : %q", vu.etat("delete_repo"))
	}
	if !vu.Refreshable {
		t.Error("un jeton rangé par gh doit pouvoir être renouvelé")
	}
	if !strings.Contains(vu.Command, "gh auth refresh") {
		t.Errorf("commande annoncée : %q", vu.Command)
	}
}

// Le renouvellement doit demander la portée voulue sans lâcher les autres :
// c'est tout l'intérêt de proposer la reprise au lieu d'un simple refus.
func TestRenouvellementGardeLesPorteesDejaAcquises(t *testing.T) {
	state := fakegh.NewState()
	state.Scopes = "repo, read:org, gist"
	faux := &faussaire{}
	h := nouveauAvec(t, state, func(deps *web.Deps) {
		deps.Refresher = faux.refresher(state)
	})

	var renouvele jeton
	h.json(http.MethodPost, "/api/token/refresh",
		map[string]any{"scopes": []string{"repo", "read:org", "delete_repo"}}, &renouvele)

	if len(faux.Appels) != 1 {
		t.Fatalf("appels à gh : %v", faux.Appels)
	}
	appel := faux.Appels[0]
	for _, attendue := range []string{"repo", "read:org", "delete_repo", "gist"} {
		if !strings.Contains(appel, attendue) {
			t.Errorf("« %s » manque à l'appel : %s", attendue, appel)
		}
	}
	if strings.Contains(appel, "--remove-scopes") {
		t.Errorf("rien ne devait être retiré : %s", appel)
	}
	if renouvele.etat("delete_repo") != scopes.Present {
		t.Errorf("delete_repo après renouvellement : %q", renouvele.etat("delete_repo"))
	}
	if len(renouvele.Missing) != 0 {
		t.Errorf("il ne devait rien manquer : %v", renouvele.Missing)
	}
}

// Décocher une portée dans les réglages la retire ; le socle exigé par gh et
// ce qu'un autre outil a obtenu ne sont jamais touchés.
func TestRenouvellementRetireUnePorteeDecochee(t *testing.T) {
	state := fakegh.NewState()
	state.Scopes = "repo, read:org, gist, delete_repo, workflow"
	faux := &faussaire{}
	h := nouveauAvec(t, state, func(deps *web.Deps) {
		deps.Refresher = faux.refresher(state)
	})

	var renouvele jeton
	h.json(http.MethodPost, "/api/token/refresh",
		map[string]any{"scopes": []string{"repo", "read:org", "workflow"}}, &renouvele)

	appel := faux.Appels[0]
	if !strings.Contains(appel, "--remove-scopes delete_repo") {
		t.Errorf("delete_repo devait être retiré : %s", appel)
	}
	if renouvele.etat("delete_repo") != scopes.Absent {
		t.Errorf("delete_repo : %q", renouvele.etat("delete_repo"))
	}
	if renouvele.etat("workflow") != scopes.Present {
		t.Errorf("workflow : %q", renouvele.etat("workflow"))
	}
	if !strings.Contains(state.Scopes, "gist") {
		t.Errorf("« gist » ne regarde pas cet outil et devait rester : %q", state.Scopes)
	}
}

// GitHub accorde ce qu'on lui accorde : refuser une portée au navigateur doit
// se voir, et non passer pour un succès.
func TestRenouvellementSignaleUnePorteeRefusee(t *testing.T) {
	state := fakegh.NewState()
	state.Scopes = "repo, read:org"
	h := nouveauAvec(t, state, func(deps *web.Deps) {
		deps.Refresher = &scopes.Refresher{
			Locate: func() (string, error) { return "gh", nil },
			Run:    func(context.Context, string, []string, scopes.Request) error { return nil },
			Read:   func(string) (string, string) { return "jeton-renouvele", "oauth_token" },
		}
	})

	var renouvele jeton
	h.json(http.MethodPost, "/api/token/refresh",
		map[string]any{"scopes": []string{"repo", "read:org", "delete_repo"}}, &renouvele)
	if len(renouvele.Missing) != 1 || renouvele.Missing[0] != "delete_repo" {
		t.Errorf("portées encore absentes : %v", renouvele.Missing)
	}
}

func TestJetonDEnvironnementNeSeRenouvellePas(t *testing.T) {
	state := fakegh.NewState()
	h := nouveauAvec(t, state, func(deps *web.Deps) {
		deps.TokenOrigin = "GH_TOKEN"
	})

	var vu jeton
	h.json(http.MethodGet, "/api/token", nil, &vu)
	if vu.Refreshable {
		t.Error("un jeton posé par l'environnement échappe à gh")
	}

	reponse, contenu := h.requete(http.MethodPost, "/api/token/refresh",
		map[string]any{"scopes": []string{"delete_repo"}})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	if !strings.Contains(string(contenu), "GH_TOKEN") {
		t.Errorf("le refus doit nommer la variable : %s", contenu)
	}
}

// Un refus doit nommer la portée : c'est ce qui permet à l'interface de
// proposer la reprise au lieu d'énoncer une commande à recopier.
func TestSuppressionRefuseeNommeLaPortee(t *testing.T) {
	state := fakegh.NewState()
	state.Scopes = "repo, read:org"
	state.AddRepo("acme", "tp1-alice", true)
	h := nouveau(t, state)

	reponse, contenu := h.requete(http.MethodDelete, "/api/orgs/acme/repos/tp1-alice",
		map[string]any{"confirm": "tp1-alice"})
	if reponse.StatusCode != http.StatusForbidden {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	var refus struct {
		Error string `json:"error"`
		Scope string `json:"scope"`
	}
	h.decoder(contenu, &refus)
	if refus.Scope != "delete_repo" {
		t.Errorf("portée nommée : %q (%s)", refus.Scope, refus.Error)
	}
}

// Un jeton « fine-grained » n'annonce aucune portée : rien ne peut être vérifié
// d'avance, et c'est le refus de GitHub qui nomme ce qui manque. L'interface
// doit le reconnaître aussi bien que le refus anticipé.
func TestRefusDeGitHubNommeLaPortee(t *testing.T) {
	state := fakegh.NewState()
	state.Scopes = ""
	state.AddRepo("acme", "tp1-alice", true)
	state.FailOn["DELETE /repos/acme/tp1-alice"] = fakegh.Failure{
		Status:   403,
		Message:  "Must have admin rights to Repository.",
		Accepted: "delete_repo",
	}
	h := nouveau(t, state)

	reponse, contenu := h.requete(http.MethodDelete, "/api/orgs/acme/repos/tp1-alice",
		map[string]any{"confirm": "tp1-alice"})
	if reponse.StatusCode != http.StatusForbidden {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	var refus struct {
		Scope string `json:"scope"`
	}
	h.decoder(contenu, &refus)
	if refus.Scope != "delete_repo" {
		t.Errorf("portée nommée : %q", refus.Scope)
	}
}
