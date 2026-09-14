package registry_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
)

// travaux rend le fichier des dates de remise tel qu'il est dans le dépôt.
func travaux(state *fakegh.State) string {
	return string(state.Files("acme/"+registry.RepoName,
		registry.Branch)[registry.AssignmentsFile])
}

// Une date fixée sur un poste se relit sur un autre : c'est tout l'objet de
// l'avoir mise dans l'organisation plutôt que dans un fichier local.
func TestUneDateDeRemiseSeRelitDepuisLOrganisation(t *testing.T) {
	store, serveur := magasin(t, nil)
	if _, err := store.Apply(
		registry.Schedule("a26.5n6.01.tp1", "2026-10-01")); err != nil {
		t.Fatalf("Apply : %v", err)
	}

	// Un second magasin, comme un second poste : il ne partage ni mémoire ni
	// cache avec le premier, et ne connaît que ce que GitHub porte.
	collegue := registry.New(clientDe(t, serveur), "acme", nil)
	snapshot, err := collegue.Load()
	if err != nil {
		t.Fatalf("Load : %v", err)
	}
	if due := snapshot.Set.Due("a26.5n6.01.tp1"); due != "2026-10-01" {
		t.Errorf("Due = %q depuis un autre poste", due)
	}
	// La casse d'un identifiant ne compte pas : GitHub n'en tient pas compte
	// dans un nom de dépôt, et la nomenclature non plus.
	if due := snapshot.Set.Due("A26.5N6.01.TP1"); due != "2026-10-01" {
		t.Errorf("Due (casse changée) = %q", due)
	}
	if due := snapshot.Set.Due("a26.5n6.01.tp2"); due != "" {
		t.Errorf("un travail sans échéance en rend une : %q", due)
	}
}

func TestUneDateDeRemiseSeChangeEtSeRetire(t *testing.T) {
	store, serveur := magasin(t, nil)
	if _, err := store.Apply(registry.Schedule("a26.5n6.01.tp1", "2026-10-01")); err != nil {
		t.Fatalf("Apply : %v", err)
	}
	set, err := store.Apply(registry.Schedule("a26.5n6.01.tp1", "2026-10-08T23:59"))
	if err != nil {
		t.Fatalf("Apply : %v", err)
	}
	if due := set.Due("a26.5n6.01.tp1"); due != "2026-10-08T23:59" {
		t.Errorf("Due après changement = %q", due)
	}
	if len(set.Assignments()) != 1 {
		t.Errorf("%d échéance(s), attendu 1 : changer n'ajoute pas", len(set.Assignments()))
	}

	// Une date vide retire l'échéance : c'est la même décision dans l'autre sens.
	sans, err := store.Apply(registry.Schedule("a26.5n6.01.tp1", ""))
	if err != nil {
		t.Fatalf("Apply : %v", err)
	}
	if len(sans.Assignments()) != 0 {
		t.Errorf("l'échéance survit à son retrait : %+v", sans.Assignments())
	}
	if contenu := travaux(serveur.State); strings.Contains(contenu, "2026-10") {
		t.Errorf("le fichier porte encore une date :\n%s", contenu)
	}
}

// Redire la même date n'écrit rien : un commit sans effet salit l'historique
// du registre, que deux personnes viendront relire sur github.com.
func TestUneDateRedeciteNEcritRien(t *testing.T) {
	store, serveur := magasin(t, nil)
	if _, err := store.Apply(registry.Schedule("a26.5n6.01.tp1", "2026-10-01")); err != nil {
		t.Fatalf("Apply : %v", err)
	}
	commits := serveur.State.CallCount("POST /repos/acme/.cohorte/git/commits")

	if _, err := store.Apply(registry.Schedule("a26.5n6.01.tp1", "2026-10-01")); err != nil {
		t.Fatalf("Apply : %v", err)
	}
	if apres := serveur.State.CallCount("POST /repos/acme/.cohorte/git/commits"); apres != commits {
		t.Errorf("%d commit(s) après une date redite, %d avant", apres, commits)
	}
}

// Les deux sections vivent dans deux fichiers, et un commit ne touche que la
// sienne : c'est ce qui rend lisible, sur github.com, ce qui a changé.
func TestChaqueSectionAUnFichierEtNeTouchePasLAutre(t *testing.T) {
	store, serveur := magasin(t, nil)
	if _, err := store.Apply(registry.Learn(personne("Émilie Côté", "ecote"))); err != nil {
		t.Fatalf("Apply : %v", err)
	}
	// Apprendre un nom ne crée pas le fichier des travaux : rien n'y serait.
	if contenu := travaux(serveur.State); contenu != "" {
		t.Errorf("travaux.json écrit sans échéance :\n%s", contenu)
	}

	if _, err := store.Apply(registry.Schedule("a26.5n6.01.tp1", "2026-10-01")); err != nil {
		t.Fatalf("Apply : %v", err)
	}
	fichiers := serveur.State.Files("acme/"+registry.RepoName, registry.Branch)
	// Le fichier des étudiants n'a pas bougé, et il est toujours là : « Push
	// FilesOnto » superpose sur l'arbre du parent.
	if !strings.Contains(string(fichiers[registry.StudentsFile]), "ecote") {
		t.Errorf("le nom a disparu en fixant une date :\n%s", fichiers[registry.StudentsFile])
	}
	if !strings.Contains(string(fichiers[registry.AssignmentsFile]), "a26.5n6.01.tp1") {
		t.Errorf("la date n'est pas écrite :\n%s", fichiers[registry.AssignmentsFile])
	}
}

// Un travail renommé ou déplacé change d'identifiant : son échéance quitte
// l'ancien et prend le nouveau, et les deux mouvements tiennent dans un seul
// changement — sinon l'un pourrait aboutir sans l'autre.
func TestUneEcheanceChangeDIdentifiantDUnSeulCoup(t *testing.T) {
	store, _ := magasin(t, nil)
	if _, err := store.Apply(registry.Schedule("a26.5n6.01.tp1", "2026-10-01")); err != nil {
		t.Fatalf("Apply : %v", err)
	}
	set, err := store.Apply(registry.Reschedule(
		registry.Assignment{ID: "a26.5n6.01.tp1"},
		registry.Assignment{ID: "a26.5n6.01.projet-final", Due: "2026-10-01"},
	))
	if err != nil {
		t.Fatalf("Apply : %v", err)
	}
	if due := set.Due("a26.5n6.01.projet-final"); due != "2026-10-01" {
		t.Errorf("le nouvel identifiant n'a pas l'échéance : %q", due)
	}
	if due := set.Due("a26.5n6.01.tp1"); due != "" {
		t.Errorf("l'ancien identifiant garde l'échéance : %q", due)
	}
}

// Une date écrite pendant qu'on en écrivait une autre n'est pas perdue : c'est
// le changement qui est rejoué, pas le registre qu'on avait en main.
func TestDeuxEcheancesEcritesEnMemeTempsSurviventToutesDeux(t *testing.T) {
	state := fakegh.NewState()
	premier, serveur := magasin(t, state)
	if _, err := premier.Apply(registry.Schedule("a26.5n6.01.tp1", "2026-10-01")); err != nil {
		t.Fatalf("Apply : %v", err)
	}

	// Le second a lu avant que le troisième n'écrive : son écriture sera
	// refusée, relue, et rejouée sur l'état frais.
	second := registry.New(clientDe(t, serveur), "acme", nil)
	if _, err := second.Load(); err != nil {
		t.Fatalf("Load : %v", err)
	}
	if _, err := premier.Apply(registry.Schedule("a26.5n6.01.tp2", "2026-11-05")); err != nil {
		t.Fatalf("Apply : %v", err)
	}
	set, err := second.Apply(registry.Schedule("a26.5n6.01.tp3", "2026-12-10"))
	if err != nil {
		t.Fatalf("Apply : %v", err)
	}
	for _, cas := range []struct{ id, due string }{
		{"a26.5n6.01.tp1", "2026-10-01"},
		{"a26.5n6.01.tp2", "2026-11-05"},
		{"a26.5n6.01.tp3", "2026-12-10"},
	} {
		if due := set.Due(cas.id); due != cas.due {
			t.Errorf("Due(%q) = %q, attendu %q", cas.id, due, cas.due)
		}
	}
}

// Le fichier se modifie à la main sur github.com : une ligne mal écrite
// s'écarte et se signale, elle ne prive pas l'organisation de ses échéances.
func TestUneLigneMalEcriteNePrivePasDesAutres(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", registry.RepoName, true)
	state.SeedCommit("acme/"+registry.RepoName, map[string]string{
		registry.AssignmentsFile: `{"version":1,"assignments":[
			{"id":"a26.5n6.01.tp1","due":"2026-10-01"},
			{"id":"a26.5n6.01.tp2","due":"le 5 novembre"},
			{"id":"","due":"2026-12-10"}
		]}`,
	}, registry.Branch)

	store, _ := magasin(t, state)
	snapshot, err := store.Load()
	if err != nil {
		t.Fatalf("Load : %v", err)
	}
	if due := snapshot.Set.Due("a26.5n6.01.tp1"); due != "2026-10-01" {
		t.Errorf("la ligne valable est perdue : %q", due)
	}
	if len(snapshot.Set.Assignments()) != 1 {
		t.Errorf("%d échéance(s) retenue(s) : %+v",
			len(snapshot.Set.Assignments()), snapshot.Set.Assignments())
	}
	if len(snapshot.Issues) != 2 {
		t.Errorf("%d souci(s) signalé(s), attendu 2 : %v", len(snapshot.Issues), snapshot.Issues)
	}
}
