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

func TestOwnersLitLeCompteDansLesAcces(t *testing.T) {
	client, serveur := monter(t)
	// Le nom ne dit pas où finit le travail : « kickmyb-firebase » en fait
	// partie, et le découper au jugé donnerait « firebase-Walid7Akk ».
	serveur.State.AddRepo("acme", "kickmyb-firebase-Walid7Akk", true)
	serveur.State.AddCollaborator("acme/kickmyb-firebase-Walid7Akk", "Walid7Akk", "push")
	// L'enseignant a accès à tout : il n'est l'indice de rien.
	serveur.State.AddCollaborator("acme/kickmyb-firebase-Walid7Akk", "prof", "admin")

	resolveur := identity.New(client, cache.NewIn(t.TempDir(), true), 4)
	trouves := resolveur.Owners("acme", []string{"kickmyb-firebase-Walid7Akk"}, "prof", nil)
	if trouves["kickmyb-firebase-Walid7Akk"].Login != "Walid7Akk" {
		t.Fatalf("propriétaire = %+v", trouves["kickmyb-firebase-Walid7Akk"])
	}
}

func TestOwnersCompteUneInvitationNonAcceptee(t *testing.T) {
	client, serveur := monter(t)
	serveur.State.AddRepo("acme", "tp1-jlpicard", true)
	serveur.State.Invite("acme/tp1-jlpicard", "jlpicard", "push")

	resolveur := identity.New(client, cache.NewIn(t.TempDir(), true), 4)
	trouves := resolveur.Owners("acme", []string{"tp1-jlpicard"}, "prof", nil)
	if trouves["tp1-jlpicard"].Login != "jlpicard" {
		t.Fatalf("propriétaire = %+v : une invitation en attente vaut un accès",
			trouves["tp1-jlpicard"])
	}
}

func TestOwnersSansAccesNeTranchePas(t *testing.T) {
	client, serveur := monter(t)
	serveur.State.AddRepo("acme", "tp1-orphelin", true)

	resolveur := identity.New(client, cache.NewIn(t.TempDir(), true), 4)
	trouves := resolveur.Owners("acme", []string{"tp1-orphelin"}, "prof", nil)
	if trouves["tp1-orphelin"].Login != "" {
		t.Fatalf("propriétaire = %+v : rien n'y donne accès", trouves["tp1-orphelin"])
	}
}

func TestOwnersNeRedemandePasCeQuIlSait(t *testing.T) {
	client, serveur := monter(t)
	serveur.State.AddRepo("acme", "tp1-jlpicard", true)
	serveur.State.AddCollaborator("acme/tp1-jlpicard", "jlpicard", "push")
	stockage := cache.NewIn(t.TempDir(), true)

	identity.New(client, stockage, 4).Owners("acme", []string{"tp1-jlpicard"}, "prof", nil)
	appels := serveur.State.CallCount("/collaborators")
	if appels == 0 {
		t.Fatal("aucun appel : le premier passage doit lire les accès")
	}

	// Un résolveur neuf, mais le même cache : plus rien ne part sur le réseau.
	trouves := identity.New(client, stockage, 4).
		Owners("acme", []string{"tp1-jlpicard"}, "prof", nil)
	if trouves["tp1-jlpicard"].Login != "jlpicard" {
		t.Fatalf("propriétaire = %+v", trouves["tp1-jlpicard"])
	}
	if serveur.State.CallCount("/collaborators") != appels {
		t.Fatalf("appels = %d, attendu %d : le cache devait répondre",
			serveur.State.CallCount("/collaborators"), appels)
	}
}
