package registry_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
)

// magasin monte un faux GitHub et le registre qui s'y écrit.
func magasin(t *testing.T, state *fakegh.State) (*registry.Store, *fakegh.Server) {
	t.Helper()
	if state == nil {
		state = fakegh.NewState()
	}
	serveur := fakegh.New(state)
	t.Cleanup(serveur.Close)

	client, err := ghapi.New(ghapi.Options{
		Host: "127.0.0.1", Token: "jeton-de-test", BaseURL: serveur.URL(),
		Sleep: func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	return registry.New(client, "acme", nil), serveur
}

// Une organisation où l'on n'a rien écrit n'est pas une panne : elle a
// simplement un registre vide.
func TestRegistreAbsentSeLitVide(t *testing.T) {
	store, serveur := magasin(t, nil)
	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("Load : %v", err)
	}
	if snapshot.Set.Len() != 0 || snapshot.Head != "" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	// Lire n'écrit rien : le dépôt du registre n'est pas créé au passage.
	if noms := serveur.State.RepoNames("acme"); len(noms) != 0 {
		t.Errorf("dépôts créés par une simple lecture : %v", noms)
	}
}

// La première écriture crée le dépôt, privé, et y dépose de quoi l'expliquer.
func TestLaPremiereEcritureAmorceLeDepot(t *testing.T) {
	store, serveur := magasin(t, nil)
	set, err := store.Apply(registry.Learn(
		personne("Émilie Côté", "ecote"), personne("Jean-Luc Picard", "jlpicard")))
	if err != nil {
		t.Fatalf("Apply : %v", err)
	}
	if set.Len() != 2 {
		t.Fatalf("%d fiche(s)", set.Len())
	}

	depot := serveur.State.Repos["acme/"+registry.RepoName]
	if depot == nil {
		t.Fatal("le dépôt du registre n'a pas été créé")
	}
	if !depot.Private {
		t.Error("le registre porte des noms d'étudiants : il doit être privé")
	}
	fichiers := serveur.State.Files("acme/"+registry.RepoName, registry.Branch)
	if _, present := fichiers[registry.StudentsFile]; !present {
		t.Fatalf("fichiers = %v", fichiers)
	}
	if lisezmoi := fichiers[registry.ReadmeFile]; !strings.Contains(lisezmoi, "privé") {
		t.Errorf("le LISEZMOI doit dire que le dépôt reste privé : %q", lisezmoi)
	}

	// Relu depuis GitHub, c'est bien ce qu'on y a mis.
	relu, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if relu.Set.Name("ecote") != "Émilie Côté" || relu.Head == "" {
		t.Fatalf("relu = %+v", relu)
	}
	if len(relu.Issues) != 0 {
		t.Errorf("soucis à la relecture : %v", relu.Issues)
	}
}

// Un changement qui ne change rien n'écrit rien : un commit sans effet salit
// l'historique sans rien apprendre à personne.
func TestUnChangementSansEffetNEcritRien(t *testing.T) {
	store, serveur := magasin(t, nil)
	if _, err := store.Apply(registry.Learn(personne("Émilie Côté", "ecote"))); err != nil {
		t.Fatal(err)
	}
	commits := serveur.State.CallCount("POST /repos/acme/.cohorte/git/commits")

	if _, err := store.Apply(registry.Learn(personne("Émilie Côté", "ecote"))); err != nil {
		t.Fatal(err)
	}
	if apres := serveur.State.CallCount("POST /repos/acme/.cohorte/git/commits"); apres != commits {
		t.Errorf("%d commit(s) de plus pour un changement sans effet", apres-commits)
	}
}

// Deux personnes écrivent en même temps. Le refus de GitHub fait relire et
// rejouer celle qui arrive après, et aucune des deux ne perd son travail.
func TestDeuxEcrituresConcurrentesSeConserventToutesDeux(t *testing.T) {
	state := fakegh.NewState()
	store, serveur := magasin(t, state)

	// Le registre existe déjà : les deux écritures partent du même commit.
	if _, err := store.Apply(registry.Learn(personne("Prof Une", "prof"))); err != nil {
		t.Fatal(err)
	}

	// La barrière retient les deux écrivains le temps qu'ils aient tous deux
	// relevé la même tête. Sans elle, ils se suivraient sagement et le rejeu
	// ne serait jamais éprouvé.
	depart := fakegh.NewBarrier(2, 2*time.Second)
	state.Hook = func(request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/git/ref/heads/"+registry.Branch) {
			depart.Wait()
		}
	}

	// Chaque écrivain a son propre magasin : le verrou interne d'un même
	// magasin les sérialiserait avant même d'atteindre GitHub.
	second := registry.New(clientVers(t, serveur), "acme", nil)

	var groupe sync.WaitGroup
	echecs := make([]error, 2)
	groupe.Add(2)
	go func() {
		defer groupe.Done()
		_, echecs[0] = store.Apply(registry.Learn(personne("Émilie Côté", "ecote")))
	}()
	go func() {
		defer groupe.Done()
		_, echecs[1] = second.Apply(registry.Learn(personne("Jean-Luc Picard", "jlpicard")))
	}()
	groupe.Wait()

	for index, err := range echecs {
		if err != nil {
			t.Fatalf("écrivain %d : %v", index+1, err)
		}
	}
	if !depart.Reached() {
		t.Fatal("les deux écritures ne se sont jamais croisées : le rejeu n'est pas éprouvé")
	}

	relu, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, compte := range []string{"prof", "ecote", "jlpicard"} {
		if _, connu := relu.Set.Find(compte); !connu {
			t.Errorf("@%s a disparu : une écriture en a effacé une autre", compte)
		}
	}
}

// Le registre porte des noms d'étudiants. S'il a été rendu public, rien n'y est
// écrit : mieux vaut une écriture qui échoue bruyamment qu'une liste de noms
// exposée sans que personne s'en aperçoive.
func TestRienNEstEcritDansUnRegistrePublic(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", registry.RepoName, false)
	store, serveur := magasin(t, state)

	_, err := store.Apply(registry.Learn(personne("Émilie Côté", "ecote")))
	if err == nil {
		t.Fatal("écrire dans un registre public doit être refusé")
	}
	if !strings.Contains(err.Error(), "public") {
		t.Errorf("le refus doit dire pourquoi : %v", err)
	}
	if commits := serveur.State.CallCount("POST /repos/acme/.cohorte/git/commits"); commits != 0 {
		t.Errorf("%d commit(s) écrit(s) malgré le refus", commits)
	}
}

// Une organisation qui accorde d'office un droit de lecture à ses membres doit
// être signalée : des étudiants membres y liraient la liste de leurs camarades.
func TestPermissionDeBaseSignalee(t *testing.T) {
	state := fakegh.NewState()
	state.DefaultRepoPermission["acme"] = "read"
	store, _ := magasin(t, state)
	avertissement := store.Exposure()
	if !strings.Contains(avertissement, "read") || !strings.Contains(avertissement, registry.RepoName) {
		t.Fatalf("avertissement = %q", avertissement)
	}

	// « none » n'expose rien, et un réglage invisible ne dit rien non plus.
	state.DefaultRepoPermission["acme"] = "none"
	if avertissement := store.Exposure(); avertissement != "" {
		t.Errorf("« none » ne doit rien signaler : %q", avertissement)
	}
	delete(state.DefaultRepoPermission, "acme")
	if avertissement := store.Exposure(); avertissement != "" {
		t.Errorf("un réglage invisible ne permet rien d'affirmer : %q", avertissement)
	}
}

// clientVers monte un second client sur le même faux GitHub.
func clientVers(t *testing.T, serveur *fakegh.Server) *ghapi.Client {
	t.Helper()
	client, err := ghapi.New(ghapi.Options{
		Host: "127.0.0.1", Token: "jeton-de-test", BaseURL: serveur.URL(),
		Sleep: func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	return client
}

// Le commit relevé scelle ce qu'on a lu : tant qu'il n'a pas changé, le fichier
// n'est pas retéléchargé. Une lecture courante coûte une seule requête.
func TestUneLectureInchangeeNeReteleschargeRien(t *testing.T) {
	state := fakegh.NewState()
	serveur := fakegh.New(state)
	t.Cleanup(serveur.Close)
	local := cache.NewIn(t.TempDir(), true)
	store := registry.New(clientVers(t, serveur), "acme", local)

	if _, err := store.Apply(registry.Learn(personne("Émilie Côté", "ecote"))); err != nil {
		t.Fatal(err)
	}
	lectures := state.CallCount("GET /repos/acme/.cohorte/contents/")

	for range 3 {
		snapshot, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Set.Name("ecote") != "Émilie Côté" || snapshot.Stale {
			t.Fatalf("snapshot = %+v", snapshot)
		}
	}
	if apres := state.CallCount("GET /repos/acme/.cohorte/contents/"); apres != lectures {
		t.Errorf("%d relecture(s) du fichier alors que le commit n'a pas bougé", apres-lectures)
	}
}

// Le commit change dès que quelqu'un écrit : le fichier est alors relu.
func TestUnCommitDifferentFaitRelireLeFichier(t *testing.T) {
	state := fakegh.NewState()
	serveur := fakegh.New(state)
	t.Cleanup(serveur.Close)
	local := cache.NewIn(t.TempDir(), true)
	store := registry.New(clientVers(t, serveur), "acme", local)

	if _, err := store.Apply(registry.Learn(personne("Émilie Côté", "ecote"))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	// Un collègue écrit, sur son propre magasin et sans notre cache.
	ailleurs := registry.New(clientVers(t, serveur), "acme", nil)
	if _, err := ailleurs.Apply(registry.Learn(personne("Jean-Luc Picard", "jlpicard"))); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, connu := snapshot.Set.Find("jlpicard"); !connu {
		t.Fatal("le registre n'a pas été relu alors que le commit avait changé")
	}
}

// Hors ligne, le registre déjà lu reste ce qu'on sait de mieux — mais il est
// annoncé comme tel : afficher des noms périmés en silence serait pire que de
// n'en afficher aucun.
func TestHorsLigneLeRegistreConnuSertEtLeDit(t *testing.T) {
	state := fakegh.NewState()
	serveur := fakegh.New(state)
	local := cache.NewIn(t.TempDir(), true)
	store := registry.New(clientVers(t, serveur), "acme", local)

	if _, err := store.Apply(registry.Learn(personne("Émilie Côté", "ecote"))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	serveur.Close() // plus de GitHub

	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("hors ligne, la lecture doit aboutir : %v", err)
	}
	if !snapshot.Stale {
		t.Error("un registre venu du disque doit s'annoncer comme périmé")
	}
	if snapshot.Set.Name("ecote") != "Émilie Côté" {
		t.Fatalf("registre = %+v", snapshot.Set.All())
	}
	// Écrire, en revanche, n'a aucun sens hors ligne.
	if _, err := store.Apply(registry.Learn(personne("Jean-Luc Picard", "jlpicard"))); err == nil {
		t.Error("une écriture hors ligne doit échouer, pas partir d'un état périmé")
	}
}

// Un refus de GitHub n'est pas une panne de liaison : montrer des noms périmés
// au lieu de dire « votre jeton a expiré » égarerait.
func TestUnJetonRefuseNeSeReplieePasSurLeDisque(t *testing.T) {
	state := fakegh.NewState()
	serveur := fakegh.New(state)
	t.Cleanup(serveur.Close)
	local := cache.NewIn(t.TempDir(), true)
	store := registry.New(clientVers(t, serveur), "acme", local)

	if _, err := store.Apply(registry.Learn(personne("Émilie Côté", "ecote"))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	state.FailOn["GET /repos/acme/.cohorte/git/ref/heads/main"] = fakegh.Failure{
		Status: 401, Message: "Bad credentials"}

	if _, err := store.Load(); err == nil {
		t.Fatal("un jeton refusé doit remonter, pas se replier sur le disque")
	}
}
