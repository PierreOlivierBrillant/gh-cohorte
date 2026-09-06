package identity_test

import (
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
)

func monter(t *testing.T) (*ghapi.Client, *fakegh.Server) {
	t.Helper()
	serveur := fakegh.New(nil)
	t.Cleanup(serveur.Close)
	client, err := ghapi.New(ghapi.Options{
		Host: "127.0.0.1", Token: "jeton", BaseURL: serveur.URL(), Sleep: func(time.Duration) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	return client, serveur
}

func TestNomsRetrouvesParLAPIPuisMisEnCache(t *testing.T) {
	client, serveur := monter(t)
	stockage := cache.NewIn(t.TempDir(), true)
	resolveur := identity.New(client, stockage, 4)

	paires := []identity.Pair{
		{Repo: "tp1-emilie-cote", Login: "emilie-cote"},
		{Repo: "tp1-jlpicard", Login: "jlpicard"},
		{Repo: "tp1-aminata-d", Login: "aminata-d"}, // profil sans nom
	}
	var progression int
	noms := resolveur.Resolve(paires, true, func(done, total int, repo string) {
		progression++
		if total != 3 {
			t.Errorf("total = %d", total)
		}
	})
	if noms["tp1-emilie-cote"] != "Émilie Côté" || noms["tp1-jlpicard"] != "Jean-Luc Picard" {
		t.Fatalf("noms = %+v", noms)
	}
	if noms["tp1-aminata-d"] != "" {
		t.Errorf("un profil sans nom doit rester vide : %q", noms["tp1-aminata-d"])
	}
	if progression != 3 {
		t.Errorf("progression = %d", progression)
	}
	appels := serveur.State.CallCount("/users/")
	if appels != 3 {
		t.Fatalf("%d appel(s) de profil", appels)
	}

	// Une seconde résolution ne doit plus rien demander : tout est en cache,
	// y compris le profil sans nom.
	autre := identity.New(client, stockage, 4)
	noms = autre.Resolve(paires, true, nil)
	if noms["tp1-jlpicard"] != "Jean-Luc Picard" {
		t.Errorf("cache non utilisé : %+v", noms)
	}
	if serveur.State.CallCount("/users/") != appels {
		t.Errorf("le cache n'a pas évité les appels : %d", serveur.State.CallCount("/users/"))
	}
}

func TestSansReseau(t *testing.T) {
	client, serveur := monter(t)
	resolveur := identity.New(client, cache.NewIn(t.TempDir(), true), 4)
	paires := []identity.Pair{{Repo: "tp1-jlpicard", Login: "jlpicard"}}

	noms := resolveur.Resolve(paires, false, nil)
	if noms["tp1-jlpicard"] != "" {
		t.Errorf("noms = %+v", noms)
	}
	if serveur.State.CallCount("/users/") != 0 {
		t.Error("fetch=false interdit tout appel")
	}
	if manquants := resolveur.Missing(paires); len(manquants) != 1 {
		t.Errorf("manquants = %+v", manquants)
	}
}

func TestProfilIntrouvable(t *testing.T) {
	client, _ := monter(t)
	resolveur := identity.New(client, cache.NewIn(t.TempDir(), true), 4)
	noms := resolveur.Resolve([]identity.Pair{{Repo: "tp1-fantome", Login: "fantome"}}, true, nil)
	if noms["tp1-fantome"] != "" {
		t.Errorf("noms = %+v", noms)
	}
}
