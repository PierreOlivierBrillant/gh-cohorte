package web_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// Le cas qui motive la fonctionnalité : un groupe rassemble les travaux de deux
// cohortes, aucun nom complet n'est connu, et il faut bien en sortir un. Les
// dépôts arrivent à la bonne place en gardant leur compte.
func TestTravailSortiDunPrefixeFourreTout(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"h25.5n6.02.tp1.jlpicard", "h25.5n6.02.tp1.aminata-d",
		"h25.5n6.02.tp2.emilie-cote",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	depart := h.sansNoms("h25", "5n6", "02",
		"jlpicard", "aminata-d", "emilie-cote")

	bilan := h.travail(http.MethodPost,
		"/api/classrooms/"+depart+"/assignments/move", map[string]any{
			"assignments": []map[string]string{{"id": "h25.5n6.02.tp1"}},
			"new_group":   map[string]string{"session": "a26", "course": "5n6", "group": "01"},
		})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["renamed"] != float64(2) || resultat["failed"] != float64(0) {
		t.Fatalf("bilan : %+v", resultat)
	}
	if resultat["created"] != true {
		t.Fatalf("le groupe d'arrivée aurait dû être déclaré : %+v", resultat)
	}

	noms := h.depots()
	sort.Strings(noms)
	attendu := "a26.5n6.01.tp1.aminata-d,a26.5n6.01.tp1.jlpicard,h25.5n6.02.tp2.emilie-cote"
	if strings.Join(noms, ",") != attendu {
		t.Fatalf("dépôts : %v", noms)
	}

	// Les fiches des deux personnes ont suivi, et elles ont quitté le groupe
	// de départ : il ne leur reste rien là-bas.
	var arrivee, reste struct {
		Students []struct {
			Username string `json:"username"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/a26.5n6.01", nil, &arrivee)
	h.json(http.MethodGet, "/api/classrooms/"+depart, nil, &reste)
	if len(arrivee.Students) != 2 {
		t.Fatalf("groupe d'arrivée : %+v", arrivee.Students)
	}
	if len(reste.Students) != 1 || reste.Students[0].Username != "emilie-cote" {
		t.Fatalf("groupe de départ : %+v", reste.Students)
	}
}

// Une fois le travail à la bonne place, le nom complet retrouvé renomme ses
// dépôts : c'est la deuxième moitié du chemin, et elle ne marche que parce que
// le compte GitHub rattache encore le dépôt à son étudiant.
func TestLeNomCompletSeCorrigeApresLeDeplacement(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "h25.5n6.02.tp1.jlpicard", true)
	h := nouveau(t, state)
	depart := h.sansNoms("h25", "5n6", "02",
		"jlpicard")

	h.travail(http.MethodPost, "/api/classrooms/"+depart+"/assignments/move",
		map[string]any{
			"assignments": []map[string]string{{"id": "h25.5n6.02.tp1"}},
			"new_group":   map[string]string{"session": "a26", "course": "5n6", "group": "01"},
		})

	bilan := h.travail(http.MethodPost, "/api/classrooms/a26.5n6.01/students/rename",
		map[string]any{
			"username": "jlpicard", "full_name": "Jean-Luc Picard", "repos": true,
		})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	if noms := h.depots(); len(noms) != 1 ||
		noms[0] != "a26.5n6.01.tp1.jean-luc-picard" {
		t.Fatalf("dépôts : %v", noms)
	}
}

// L'aperçu montre le renommage sans rien écrire : c'est la seule façon de
// vérifier que ce sont bien ces dépôts-là qu'on sort du fourre-tout.
func TestApercuDuDeplacementNecritRien(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "h25.5n6.02.tp1.jlpicard", true)
	h := nouveau(t, state)
	depart := h.sansNoms("h25", "5n6", "02",
		"jlpicard")

	var apercu struct {
		Ready       int    `json:"ready"`
		Target      string `json:"target"`
		TargetScope string `json:"target_scope"`
		Created     bool   `json:"created"`
		Rows        []struct {
			Repo   string `json:"repo"`
			Target string `json:"target"`
		} `json:"rows"`
		Students []string `json:"students"`
	}
	h.json(http.MethodPost, "/api/classrooms/"+depart+"/assignments/move/preview",
		map[string]any{
			"assignments": []map[string]string{{"id": "h25.5n6.02.tp1", "name": "Travail de session"}},
			"new_group":   map[string]string{"session": "a26", "course": "5n6", "group": "01"},
		}, &apercu)

	if apercu.Ready != 1 || apercu.TargetScope != "a26.5n6.01" || !apercu.Created {
		t.Fatalf("aperçu : %+v", apercu)
	}
	if apercu.Rows[0].Target != "a26.5n6.01.travail-de-session.jlpicard" {
		t.Fatalf("cible composée : %+v", apercu.Rows)
	}
	if len(apercu.Students) != 1 || apercu.Students[0] != "jlpicard" {
		t.Fatalf("fiches qui suivraient : %+v", apercu.Students)
	}
	if noms := h.depots(); len(noms) != 1 ||
		noms[0] != "h25.5n6.02.tp1.jlpicard" {
		t.Fatalf("un aperçu n'écrit rien : %v", noms)
	}
}

// Un nom déjà pris arrête le déplacement au lieu de l'interrompre à mi-chemin.
func TestDeplacementDeTravailRefuseUneCollision(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"h25.5n6.02.tp1.jlpicard", "h25.5n6.02.tp1.aminata-d",
		"a26.5n6.01.tp1.jlpicard",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	depart := h.sansNoms("h25", "5n6", "02",
		"jlpicard", "aminata-d")
	arrivee := h.groupe("a26", "5n6", "01")

	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+depart+"/assignments/move",
		map[string]any{
			"assignments": []map[string]string{{"id": "h25.5n6.02.tp1"}}, "target": arrivee,
		})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	if !strings.Contains(string(contenu), "a26.5n6.01.tp1.jlpicard") {
		t.Fatalf("message : %s", contenu)
	}
	if noms := h.depots(); len(noms) != 3 {
		t.Fatalf("rien ne devait bouger : %v", noms)
	}
}

// Plusieurs travaux partent ensemble, et le groupe de départ garde ce qui
// n'était pas du voyage.
func TestPlusieursTravauxDeplacesEnsemble(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"h25.5n6.02.tp1.jlpicard", "h25.5n6.02.tp2.jlpicard",
		"h25.5n6.02.tp3.emilie-cote",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	depart := h.sansNoms("h25", "5n6", "02",
		"jlpicard", "emilie-cote")
	arrivee := h.groupe("h27", "420", "02")

	bilan := h.travail(http.MethodPost,
		"/api/classrooms/"+depart+"/assignments/move", map[string]any{
			"assignments": []map[string]string{{"id": "h25.5n6.02.tp1"}, {"id": "h25.5n6.02.tp2"}},
			"target":      arrivee,
		})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["renamed"] != float64(2) {
		t.Fatalf("bilan : %+v", resultat)
	}

	noms := h.depots()
	sort.Strings(noms)
	attendu := "h25.5n6.02.tp3.emilie-cote,h27.420.02.tp1.jlpicard,h27.420.02.tp2.jlpicard"
	if strings.Join(noms, ",") != attendu {
		t.Fatalf("dépôts : %v", noms)
	}
}
