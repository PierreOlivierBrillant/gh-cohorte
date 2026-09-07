package web_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// ------------------------------------------------------ adoption par gabarit

// ---------------------------------------------------- déplacer un étudiant

func TestEtudiantDeplaceAvecSesDepots(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"a26.5n6.01.tp1.jean-luc-picard", "a26.5n6.01.tp1.emilie-cote",
		"a26.5n6.01.travailsession.jean-luc-picard",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	depart := h.groupe("a26", "5n6", "01",
		"Jean-Luc Picard", "jlpicard", "Émilie Côté", "emilie-cote")
	arrivee := h.groupe("a26", "5n6", "02", "Aminata Diallo", "aminata-d")

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+depart+"/students/move",
		map[string]any{"username": "jlpicard", "target": arrivee, "repos": true})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["renamed"] != float64(2) || resultat["failed"] != float64(0) {
		t.Fatalf("bilan : %+v", resultat)
	}

	noms := h.depots()
	sort.Strings(noms)
	attendu := "a26.5n6.01.tp1.emilie-cote," +
		"a26.5n6.02.tp1.jean-luc-picard,a26.5n6.02.travailsession.jean-luc-picard"
	if strings.Join(noms, ",") != attendu {
		t.Fatalf("dépôts : %v", noms)
	}

	// Les deux listes ont suivi.
	var avant, apres struct {
		Students []struct {
			Username string `json:"username"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+depart, nil, &avant)
	h.json(http.MethodGet, "/api/classrooms/"+arrivee, nil, &apres)
	if len(avant.Students) != 1 || avant.Students[0].Username != "emilie-cote" {
		t.Fatalf("groupe de départ : %+v", avant.Students)
	}
	if len(apres.Students) != 2 {
		t.Fatalf("groupe d'arrivée : %+v", apres.Students)
	}
}

func TestEtudiantDeplaceSansSesDepots(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	h := nouveau(t, state)
	depart := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")
	arrivee := h.groupe("a26", "5n6", "02", "Aminata Diallo", "aminata-d")

	var bilan struct {
		Moved   []string `json:"moved"`
		Count   int      `json:"count"`
		Renamed int      `json:"renamed"`
	}
	h.json(http.MethodPost, "/api/classrooms/"+depart+"/students/move",
		map[string]any{"username": "jlpicard", "target": arrivee, "repos": false}, &bilan)
	if bilan.Count != 1 || bilan.Moved[0] != "jlpicard" || bilan.Renamed != 0 {
		t.Fatalf("bilan : %+v", bilan)
	}
	if noms := h.depots(); len(noms) != 1 ||
		noms[0] != "a26.5n6.01.tp1.jean-luc-picard" {
		t.Fatalf("aucun dépôt ne devait bouger : %v", noms)
	}
}

func TestPlusieursEtudiantsDeplacesEnsemble(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"a26.5n6.01.tp1.jean-luc-picard", "a26.5n6.01.tp1.emilie-cote",
		"a26.5n6.01.tp1.aminata-diallo",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	depart := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard",
		"Émilie Côté", "emilie-cote", "Aminata Diallo", "aminata-d")
	arrivee := h.groupe("a26", "5n6", "02")

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+depart+"/students/move",
		map[string]any{
			"usernames": []string{"jlpicard", "aminata-d"},
			"target":    arrivee, "repos": true,
		})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["count"] != float64(2) || resultat["renamed"] != float64(2) {
		t.Fatalf("bilan : %+v", resultat)
	}

	noms := h.depots()
	sort.Strings(noms)
	attendu := "a26.5n6.01.tp1.emilie-cote," +
		"a26.5n6.02.tp1.aminata-diallo,a26.5n6.02.tp1.jean-luc-picard"
	if strings.Join(noms, ",") != attendu {
		t.Fatalf("dépôts : %v", noms)
	}

	var reste struct {
		Students []struct {
			Username string `json:"username"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+depart, nil, &reste)
	if len(reste.Students) != 1 || reste.Students[0].Username != "emilie-cote" {
		t.Fatalf("groupe de départ : %+v", reste.Students)
	}
}

// Le groupe d'arrivée n'a pas à exister d'avance : c'est le déplacement qui le
// déclare, sans qu'on ait à sortir de la liste des étudiants pour le créer.
func TestDeplacementDeclareLeGroupeDArrivee(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	h := nouveau(t, state)
	depart := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+depart+"/students/move",
		map[string]any{
			"usernames": []string{"jlpicard"},
			"new_group": map[string]string{"session": "a26", "course": "5n6", "group": "03"},
			"repos":     true,
		})
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["created"] != true || resultat["target_scope"] != "a26.5n6.03" {
		t.Fatalf("bilan : %+v", resultat)
	}
	if noms := h.depots(); len(noms) != 1 ||
		noms[0] != "a26.5n6.03.tp1.jean-luc-picard" {
		t.Fatalf("dépôts : %v", noms)
	}

	var neuf struct {
		Students []struct {
			Username string `json:"username"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/a26.5n6.03", nil, &neuf)
	if len(neuf.Students) != 1 || neuf.Students[0].Username != "jlpicard" {
		t.Fatalf("groupe déclaré : %+v", neuf.Students)
	}
}

// Déclarer sur une place déjà occupée serait une fusion déguisée : elle est
// refusée, et la place se choisit alors dans la liste.
func TestNouveauGroupeSurUnePlaceOccupeeRefuse(t *testing.T) {
	h := nouveau(t, nil)
	depart := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")
	h.groupe("a26", "5n6", "02", "Émilie Côté", "emilie-cote")

	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+depart+"/students/move", map[string]any{
			"usernames": []string{"jlpicard"},
			"new_group": map[string]string{"session": "a26", "course": "5n6", "group": "02"},
		})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	if !strings.Contains(string(contenu), "existe déjà") {
		t.Fatalf("message : %s", contenu)
	}
}

// ------------------------------------------------- renommer un groupe déclaré

func TestGroupeDeclareSeRenomme(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	h := nouveau(t, state)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")

	var apercu struct {
		Scope  string `json:"scope"`
		Ready  int    `json:"ready"`
		Switch bool   `json:"switch"`
	}
	h.json(http.MethodPost, "/api/classrooms/"+id+"/migration/preview",
		map[string]any{"session": "h27", "course": "5n6", "group": "02"}, &apercu)
	if apercu.Scope != "h27.5n6.02" || apercu.Ready != 1 || !apercu.Switch {
		t.Fatalf("aperçu : %+v", apercu)
	}

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+id+"/migration/apply",
		map[string]any{"session": "h27", "course": "5n6", "group": "02"})
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["renamed"] != float64(1) || resultat["switched"] != true {
		t.Fatalf("bilan : %+v", resultat)
	}
	if noms := h.depots(); len(noms) != 1 ||
		noms[0] != "h27.5n6.02.tp1.jean-luc-picard" {
		t.Fatalf("dépôts : %v", noms)
	}
}

func TestGroupeDejaALaBonnePlaceRefuse(t *testing.T) {
	h := nouveau(t, nil)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")
	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+id+"/migration/preview",
		map[string]any{"session": "a26", "course": "5n6", "group": "01"})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
}

// ------------------------------------------------------------- chemins locaux

func TestExplorateurListeUnDossier(t *testing.T) {
	h := nouveau(t, nil)
	dossier := t.TempDir()

	var listing struct {
		Path    string `json:"path"`
		Parent  string `json:"parent"`
		Entries []struct {
			Name string `json:"name"`
			Dir  bool   `json:"dir"`
		} `json:"entries"`
	}
	h.json(http.MethodPost, "/api/paths/browse",
		map[string]any{"path": dossier, "dirs": false}, &listing)
	if listing.Path != dossier || listing.Parent == "" {
		t.Fatalf("listing : %+v", listing)
	}

	// Un chemin qui n'existe pas remonte à ce qui existe : ouvrir
	// l'explorateur ne doit jamais échouer sur un souvenir.
	h.json(http.MethodPost, "/api/paths/browse",
		map[string]any{"path": dossier + "/rien/du/tout", "dirs": true}, &listing)
	if listing.Path != dossier {
		t.Fatalf("repli : %+v", listing)
	}
}
