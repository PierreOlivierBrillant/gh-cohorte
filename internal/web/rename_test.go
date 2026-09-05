package web_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// Le cas qui motive la fonctionnalité : un travail mal nommé, distribué à toute
// la cohorte. Le corriger ne demande plus de le déplacer ailleurs pour profiter
// du nom qu'on y choisit au passage.
func TestTravailRenommeSurPlace(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.jlpicard",
		"a26.5n6.01.tp2.jlpicard",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	place := h.groupe("a26", "5N6", "01",
		"Émilie Côté", "emilie-cote", "Jean-Luc Picard", "jlpicard")

	bilan := h.travail(http.MethodPost,
		"/api/classrooms/"+place+"/assignments/rename", map[string]any{
			"id": "a26.5n6.01.tp1", "name": "Projet final",
		})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["renamed"] != float64(2) || resultat["failed"] != float64(0) {
		t.Fatalf("bilan : %+v", resultat)
	}
	// Le nom rendu est celui des dépôts, pas celui qui a été tapé.
	if resultat["name"] != "projet-final" || resultat["previous"] != "tp1" ||
		resultat["id"] != "a26.5n6.01.projet-final" {
		t.Fatalf("bilan : %+v", resultat)
	}

	noms := h.State.RepoNames("acme")
	sort.Strings(noms)
	attendu := "a26.5n6.01.projet-final.emilie-cote," +
		"a26.5n6.01.projet-final.jlpicard,a26.5n6.01.tp2.jlpicard"
	if strings.Join(noms, ",") != attendu {
		t.Fatalf("dépôts : %v", noms)
	}
}

// On renomme le travail, pas les personnes : le dernier niveau reste ce qu'il
// était, même quand le groupe connaît le nom complet.
func TestRenommerUnTravailNeTouchePasAuxEtudiants(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jlpicard", true)
	h := nouveau(t, state)
	place := h.groupe("a26", "5N6", "01", "Jean-Luc Picard", "jlpicard")

	h.travail(http.MethodPost, "/api/classrooms/"+place+"/assignments/rename",
		map[string]any{"id": "a26.5n6.01.tp1", "name": "tp2"})

	if noms := h.State.RepoNames("acme"); len(noms) != 1 ||
		noms[0] != "a26.5n6.01.tp2.jlpicard" {
		t.Fatalf("dépôts : %v", noms)
	}
}

// L'aperçu montre le renommage sans rien écrire.
func TestApercuDuRenommageNecritRien(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jlpicard", true)
	h := nouveau(t, state)
	place := h.groupe("a26", "5N6", "01", "Jean-Luc Picard", "jlpicard")

	var apercu struct {
		Ready    int    `json:"ready"`
		Name     string `json:"name"`
		Previous string `json:"previous"`
		ID       string `json:"id"`
		Rows     []struct {
			Repo   string `json:"repo"`
			Target string `json:"target"`
		} `json:"rows"`
	}
	h.json(http.MethodPost, "/api/classrooms/"+place+"/assignments/rename/preview",
		map[string]any{"id": "a26.5n6.01.tp1", "name": "Projet final"}, &apercu)

	if apercu.Ready != 1 || apercu.Name != "projet-final" || apercu.Previous != "tp1" ||
		apercu.ID != "a26.5n6.01.projet-final" {
		t.Fatalf("aperçu : %+v", apercu)
	}
	if apercu.Rows[0].Target != "a26.5n6.01.projet-final.jlpicard" {
		t.Fatalf("cible composée : %+v", apercu.Rows)
	}
	if noms := h.State.RepoNames("acme"); len(noms) != 1 ||
		noms[0] != "a26.5n6.01.tp1.jlpicard" {
		t.Fatalf("un aperçu n'écrit rien : %v", noms)
	}
}

// Un nom déjà pris refuse l'opération entière : renommer la moitié des dépôts
// laisserait deux travaux là où il n'y en avait qu'un.
func TestRenommageRefuseUnNomDejaPris(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jlpicard", true)
	state.AddRepo("acme", "a26.5n6.01.tp2.jlpicard", true)
	h := nouveau(t, state)
	place := h.groupe("a26", "5N6", "01", "Jean-Luc Picard", "jlpicard")

	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+place+"/assignments/rename",
		map[string]any{"id": "a26.5n6.01.tp1", "name": "tp2"})
	if reponse.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(contenu), "existe déjà") {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	if noms := h.State.RepoNames("acme"); len(noms) != 2 {
		t.Fatalf("rien n'aurait dû être renommé : %v", noms)
	}
}

// Le nom déjà porté est refusé plutôt que de lancer une opération vide.
func TestRenommageRefuseLeMemeNom(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jlpicard", true)
	h := nouveau(t, state)
	place := h.groupe("a26", "5N6", "01", "Jean-Luc Picard", "jlpicard")

	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+place+"/assignments/rename/preview",
		map[string]any{"id": "a26.5n6.01.tp1", "name": "TP1"})
	if reponse.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(contenu), "porte déjà ce nom") {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
}

// Un groupe resté à l'ancienne nomenclature ne sait pas nommer : le refus dit
// par où passer plutôt que de laisser essayer.
func TestRenommageRefuseUnGroupeHerite(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "vieux-tp1-jlpicard", true)
	h := nouveau(t, state)
	place := h.heritage("vieux-tp1", "jlpicard")

	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+place+"/assignments/rename",
		map[string]any{"id": "vieux-tp1", "name": "tp2"})
	if reponse.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(contenu), "nomenclature") {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	if noms := h.State.RepoNames("acme"); noms[0] != "vieux-tp1-jlpicard" {
		t.Fatalf("rien n'aurait dû être renommé : %v", noms)
	}
}
