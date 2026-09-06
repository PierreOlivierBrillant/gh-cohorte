package web_test

import (
	"net/http"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
)

// C'est le cas qui justifie le registre. Jusqu'ici, la correspondance entre le
// dernier niveau d'un nom de dépôt et le compte GitHub de la personne vivait
// dans un fichier local : un collègue ouvrant la même organisation depuis sa
// machine n'y voyait que des slugs, sans savoir qui ils désignaient.

// autreMachine monte une seconde interface sur le même GitHub, avec ses propres
// réglages : c'est le collègue, qui n'a jamais rien déclaré chez lui.
func autreMachine(t *testing.T, state *fakegh.State) *harnais {
	t.Helper()
	return nouveau(t, state)
}

func TestUnCollegueVoitLesNomsSansRienAvoirDeclare(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.jean-luc-picard",
	} {
		state.AddRepo("acme", nom, true)
	}

	// Première machine : le groupe est déclaré avec sa liste.
	premiere := nouveau(t, state)
	premiere.groupe("a26", "5N6", "01",
		"Émilie Côté", "emilie-cote", "Jean-Luc Picard", "jlpicard")

	// Le registre a bien été écrit dans l'organisation, privé.
	depot := state.Repos["acme/"+registry.RepoName]
	if depot == nil || !depot.Private {
		t.Fatalf("registre = %+v", depot)
	}

	// Seconde machine : rien de déclaré, et pourtant les noms sont là.
	collegue := autreMachine(t, state)
	annuaire := collegue.annuaire("")
	if annuaire.Total != 2 {
		t.Fatalf("%d personne(s) vue(s) par le collègue : %+v", annuaire.Total, annuaire.Students)
	}
	noms := map[string]string{}
	for _, ligne := range annuaire.Students {
		noms[ligne.Username] = ligne.FullName
	}
	if noms["emilie-cote"] != "Émilie Côté" || noms["jlpicard"] != "Jean-Luc Picard" {
		t.Fatalf("noms vus par le collègue = %v", noms)
	}
	if annuaire.Unmatched != 0 {
		t.Errorf("%d dépôt(s) rattaché(s) à personne", annuaire.Unmatched)
	}
}

// Le collègue voit aussi la liste du groupe lui-même, pas seulement l'annuaire.
func TestUnCollegueVoitLaListeDuGroupe(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)

	premiere := nouveau(t, state)
	place := premiere.groupe("a26", "5N6", "01", "Émilie Côté", "emilie-cote")

	collegue := autreMachine(t, state)
	var vue struct {
		Students []struct {
			FullName    string `json:"full_name"`
			Username    string `json:"username"`
			Assignments []struct {
				Repo string `json:"repo"`
			} `json:"assignments"`
		} `json:"students"`
	}
	collegue.json(http.MethodGet, "/api/classrooms/"+place+"/students", nil, &vue)
	if len(vue.Students) != 1 {
		t.Fatalf("liste vue par le collègue = %+v", vue.Students)
	}
	if vue.Students[0].FullName != "Émilie Côté" || vue.Students[0].Username != "emilie-cote" {
		t.Fatalf("étudiante = %+v", vue.Students[0])
	}
	if len(vue.Students[0].Assignments) != 1 {
		t.Errorf("ses dépôts n'ont pas suivi : %+v", vue.Students[0].Assignments)
	}
}

// Ce que le registre a révélé n'est pas écrit dans le fichier local : ce que la
// machine déclare doit rester ce qu'on lui a dit, non ce qu'elle a déduit.
func TestCeQueLeRegistreRevelaNEstPasDeclare(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)

	premiere := nouveau(t, state)
	place := premiere.groupe("a26", "5N6", "01",
		"Émilie Côté", "emilie-cote", "Jean-Luc Picard", "jlpicard")

	// Le collègue déclare le même groupe, mais avec une seule personne.
	collegue := autreMachine(t, state)
	collegue.groupe("a26", "5N6", "01", "Aminata Diallo", "aminata-d")

	// Il voit les trois — deux déduites du registre, une déclarée…
	var vue struct {
		Students []struct {
			Username string `json:"username"`
		} `json:"students"`
	}
	collegue.json(http.MethodGet, "/api/classrooms/"+place+"/students", nil, &vue)
	if len(vue.Students) != 3 {
		t.Fatalf("liste vue = %+v", vue.Students)
	}

	// … mais son fichier local ne retient que ce qu'il a déclaré. Une écriture
	// sur le groupe — ici l'ajout d'une personne — ne doit pas y verser au
	// passage les deux que le registre avait révélées.
	collegue.json(http.MethodPost, "/api/classrooms/"+place+"/students/add", map[string]any{
		"full_name": "Marc Aurèle", "username": "prof",
	}, nil)
	declares := collegue.declares(place)
	if len(declares) != 2 || declares[0] != "aminata-d" || declares[1] != "prof" {
		t.Fatalf("le fichier local a adopté des personnes déduites : %v", declares)
	}
}

// Corriger un nom le corrige pour tout le monde, et les dépôts déjà créés sous
// l'ancien slug restent rattachés à leur personne.
func TestUnNomCorrigeVautPourToutLeMondeSansOrphelinerLesDepots(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.emlie-cote", true)

	premiere := nouveau(t, state)
	place := premiere.groupe("a26", "5N6", "01", "Emlie Côté", "emilie-cote")

	// La faute est corrigée, sans renommer les dépôts.
	premiere.json(http.MethodPost, "/api/classrooms/"+place+"/students/rename", map[string]any{
		"username": "emilie-cote", "full_name": "Émilie Côté", "repos": false,
	}, nil)

	collegue := autreMachine(t, state)
	annuaire := collegue.annuaire("")
	if annuaire.Total != 1 {
		t.Fatalf("annuaire = %+v", annuaire.Students)
	}
	if annuaire.Students[0].FullName != "Émilie Côté" {
		t.Fatalf("nom = %q", annuaire.Students[0].FullName)
	}
	// Le dépôt porte encore l'ancien slug, et reste pourtant le sien.
	if annuaire.Students[0].Repos != 1 || annuaire.Unmatched != 0 {
		t.Fatalf("le dépôt sous l'ancien slug s'est détaché : %d dépôt(s), %d orphelin(s)",
			annuaire.Students[0].Repos, annuaire.Unmatched)
	}
}
