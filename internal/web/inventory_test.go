package web_test

import (
	"net/http"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// L'inventaire d'une organisation est ce qui coûte le plus cher à lire : à
// l'échelle d'un département, il tient des dizaines de pages. Une écriture dont
// on connaît exactement l'effet — un renommage, une suppression — doit donc s'y
// répercuter plutôt que le périmer.

// pagesLues compte les requêtes d'inventaire reçues par le faux GitHub.
func pagesLues(h *harnais) int {
	return h.State.CallCount("GET /orgs/acme/repos")
}

// travaux renvoie les noms courts des travaux que l'interface montre pour un
// groupe : c'est par là qu'on voit ce que l'inventaire retenu contient.
func travaux(h *harnais, place string) []string {
	var vue struct {
		Assignments []struct {
			Name string `json:"name"`
		} `json:"assignments"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+place, nil, &vue)
	noms := make([]string, 0, len(vue.Assignments))
	for _, travail := range vue.Assignments {
		noms = append(noms, travail.Name)
	}
	return noms
}

func TestRenommageSuitLInventaireSansLeRelire(t *testing.T) {
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
	travaux(h, place) // l'inventaire est désormais en main
	avant := pagesLues(h)

	bilan := h.travail(http.MethodPost,
		"/api/classrooms/"+place+"/assignments/rename", map[string]any{
			"id": "a26.5n6.01.tp1", "name": "projet-final",
		})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}

	// Le nouveau nom s'affiche, et l'ancien a disparu : l'inventaire a suivi.
	noms := travaux(h, place)
	vus := map[string]bool{}
	for _, nom := range noms {
		vus[nom] = true
	}
	if !vus["projet-final"] || vus["tp1"] || !vus["tp2"] {
		t.Fatalf("travaux après renommage = %v", noms)
	}
	if apres := pagesLues(h); apres != avant {
		t.Errorf("%d page(s) d'inventaire relue(s) : un renommage se suit, il ne se relit pas",
			apres-avant)
	}
}

func TestSuppressionRetireDeLInventaireSansLeRelire(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{"a26.5n6.01.tp1.jlpicard", "a26.5n6.01.tp2.jlpicard"} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	place := h.groupe("a26", "5N6", "01", "Jean-Luc Picard", "jlpicard")
	travaux(h, place)
	avant := pagesLues(h)

	h.json(http.MethodDelete, "/api/orgs/acme/repos/a26.5n6.01.tp1.jlpicard",
		map[string]any{"confirm": "a26.5n6.01.tp1.jlpicard"}, nil)

	noms := travaux(h, place)
	if len(noms) != 1 || noms[0] != "tp2" {
		t.Fatalf("travaux après suppression = %v", noms)
	}
	if apres := pagesLues(h); apres != avant {
		t.Errorf("%d page(s) d'inventaire relue(s) : une suppression se suit, elle ne se relit pas",
			apres-avant)
	}
}

// Une création, elle, doit bien relire : le dépôt neuf n'est pas dans
// l'inventaire, et sa date de dernier envoi ne s'invente pas.
func TestCreationReliteLInventaire(t *testing.T) {
	h := nouveau(t, nil)
	place := h.groupe("a26", "5N6", "01", "Jean-Luc Picard", "jlpicard")
	travaux(h, place)
	avant := pagesLues(h)

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+place+"/assignments",
		map[string]any{"name": "tp1"})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	if noms := travaux(h, place); len(noms) != 1 || noms[0] != "tp1" {
		t.Fatalf("travaux après distribution = %v", noms)
	}
	if apres := pagesLues(h); apres <= avant {
		t.Error("une création doit relire l'inventaire : le dépôt neuf n'y est pas")
	}
}
