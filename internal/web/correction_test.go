package web_test

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// Corriger un nom dans la liste d'un groupe sans renommer ses dépôts les laisse
// à la personne, même quand l'ancien nom n'était écrit que sur ce poste : le
// registre ne l'a jamais reçu, et c'est pourtant lui qui a nommé ses dépôts.
func TestCorrigerSansLesDepotsNeLesDetachePas(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "h27.5n6.02.tp1.aleksi-lepa", true)
	h := avantLeRegistre(t, state, cohorte("h27", "5n6", "02", "Aleksi Lepa", "aleksilepaj"))

	h.json(http.MethodPost, "/api/classrooms/h27.5n6.02/students/rename", map[string]any{
		"username": "aleksilepaj", "full_name": "Aleksi Lepaj", "repos": false,
	}, nil)

	fiche := h.fiche("aleksilepaj")
	if fiche.User.FullName != "Aleksi Lepaj" || fiche.User.Repos != 1 {
		t.Fatalf("fiche = %q, %d dépôt(s) : le dépôt s'est détaché",
			fiche.User.FullName, fiche.User.Repos)
	}
}

// Un nom corrigé dans un groupe vaut pour la personne : la liste d'un autre
// groupe qui la nommait encore autrement ne le masque pas.
func TestUnNomCorrigeDansUnGroupeValePourLesAutres(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "h27.5n6.02.tp1.emlie-cote", true)
	state.AddRepo("acme", "a26.5n6.01.tp1.emlie-cote", true)
	h := avantLeRegistre(t, state,
		cohorte("h27", "5n6", "02", "Emlie Côté", "emilie-cote"),
		cohorte("a26", "5n6", "01", "Emlie Côté", "emilie-cote"))

	h.json(http.MethodPost, "/api/classrooms/h27.5n6.02/students/rename", map[string]any{
		"username": "emilie-cote", "full_name": "Émilie Côté", "repos": false,
	}, nil)

	fiche := h.fiche("emilie-cote")
	if fiche.User.FullName != "Émilie Côté" || fiche.User.Repos != 2 {
		t.Fatalf("fiche = %q, %d dépôt(s)", fiche.User.FullName, fiche.User.Repos)
	}
}

// Un nom corrigé depuis la fiche laisse ses dépôts sous l'ancien. Les renommer
// depuis la liste du groupe, sans rien changer d'autre, est le geste qui suit :
// il ne doit pas être refusé parce que la fiche, elle, n'a plus rien à changer.
func TestRenommerLesDepotsApresUneCorrectionDepuisLaFiche(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.emlie-cote", true)
	h := avantLeRegistre(t, state, cohorte("a26", "5n6", "01", "Emlie Côté", "emilie-cote"))

	h.json(http.MethodPut, "/api/users/emilie-cote/name",
		map[string]any{"full_name": "Émilie Côté"}, nil)
	bilan := h.travail(http.MethodPost, "/api/classrooms/a26.5n6.01/students/rename",
		map[string]any{"username": "emilie-cote", "full_name": "Émilie Côté", "repos": true})
	if resultat, _ := bilan["result"].(map[string]any); resultat["renamed"] != float64(1) {
		t.Fatalf("bilan : %+v", bilan)
	}
	if noms := h.depots(); strings.Join(noms, ",") != "a26.5n6.01.tp1.emilie-cote" {
		t.Fatalf("dépôts : %v", noms)
	}

	// Une seconde fois, il n'y a plus rien à renommer : le refus le dit.
	reponse, contenu := h.requete(http.MethodPost, "/api/classrooms/a26.5n6.01/students/rename",
		map[string]any{"username": "emilie-cote", "full_name": "Émilie Côté", "repos": true})
	if reponse.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(contenu), "Rien à corriger") {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
}

// Corriger un nom ne fait pas perdre à la ligne ce qu'elle porte d'autre : son
// matricule et ses autres comptes sont à la personne.
func TestCorrigerUnNomGardeLeMatricule(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	h := avantLeRegistre(t, state, classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "01",
		Students: []roster.Person{{FullName: "Émilie Côté", Username: "emilie-cote",
			StudentID: "2100123", Also: []string{"emilie-perso"}}},
	})

	h.json(http.MethodPost, "/api/classrooms/a26.5n6.01/students/rename", map[string]any{
		"username": "emilie-cote", "full_name": "Émilie Côté-Roy", "repos": false,
	}, nil)

	contenu, err := os.ReadFile(h.Groupes)
	if err != nil {
		t.Fatal(err)
	}
	for _, attendu := range []string{"2100123", "emilie-perso"} {
		if !strings.Contains(string(contenu), attendu) {
			t.Errorf("« %s » a disparu de la liste :\n%s", attendu, contenu)
		}
	}
}
