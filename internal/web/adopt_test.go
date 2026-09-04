package web_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// ------------------------------------------------------ adoption par gabarit

func TestGabaritConfronteAuxDepots(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"projet-tp1-jlpicard", "projet-tp1-emilie-cote", "projet-tp2-jlpicard",
		"angular-equipe-3", // hors gabarit
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)

	var essai struct {
		Pattern     string   `json:"pattern"`
		Prefix      string   `json:"prefix"`
		Matched     int      `json:"matched"`
		Total       int      `json:"total"`
		Assignments []string `json:"assignments"`
		Students    []string `json:"students"`
		Rows        []struct {
			Repo       string `json:"repo"`
			Assignment string `json:"assignment"`
			Student    string `json:"student"`
		} `json:"rows"`
	}
	h.json(http.MethodPost, "/api/orgs/acme/match",
		map[string]any{"pattern": "projet-{assignment}-{student}"}, &essai)

	if essai.Matched != 3 || essai.Total != 4 {
		t.Fatalf("essai : %+v", essai)
	}
	if essai.Prefix != "projet" {
		t.Fatalf("préfixe %q", essai.Prefix)
	}
	if strings.Join(essai.Assignments, ",") != "tp1,tp2" {
		t.Fatalf("travaux : %v", essai.Assignments)
	}
	// Sans liste d'étudiants, la découpe la plus longue l'emporte : le nom
	// avec tiret se coupe mal, et c'est visible avant d'adopter.
	trouve := map[string]string{}
	for _, ligne := range essai.Rows {
		trouve[ligne.Repo] = ligne.Assignment + "/" + ligne.Student
	}
	if trouve["projet-tp1-jlpicard"] != "tp1/jlpicard" {
		t.Fatalf("découpe : %v", trouve)
	}
}

func TestGabaritInvalideRefuse(t *testing.T) {
	h := nouveau(t, nil)
	reponse, contenu := h.requete(http.MethodPost, "/api/orgs/acme/match",
		map[string]any{"pattern": "projet-{assignment}"})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	if !strings.Contains(string(contenu), "{student}") {
		t.Fatalf("message : %s", contenu)
	}
}

func TestGroupeAdopteParGabaritLitSesTravaux(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"projet-tp1-jlpicard", "projet-tp1-emilie-cote", "projet-tp2-jlpicard",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)

	var cree struct {
		ID          string `json:"id"`
		Pattern     string `json:"pattern"`
		Assignments []struct {
			Name  string `json:"name"`
			Repos int    `json:"repos"`
		} `json:"assignments"`
	}
	h.json(http.MethodPost, "/api/classrooms", map[string]any{
		"org": "acme", "pattern": "projet-{assignment}-{student}", "name": "Projets",
		"students": []map[string]string{
			{"username": "jlpicard", "full_name": ""},
			{"username": "emilie-cote", "full_name": ""},
		},
	}, &cree)
	if cree.Pattern != "projet-{assignment}-{student}" {
		t.Fatalf("gabarit retenu %q", cree.Pattern)
	}

	var fiche struct {
		Assignments []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Repos    int    `json:"repos"`
			Students int    `json:"students"`
		} `json:"assignments"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+cree.ID, nil, &fiche)
	if len(fiche.Assignments) != 2 {
		t.Fatalf("travaux : %+v", fiche.Assignments)
	}
	for _, travail := range fiche.Assignments {
		if travail.Name == "tp1" && (travail.Repos != 2 || travail.Students != 2) {
			t.Fatalf("tp1 : %+v", travail)
		}
	}

	// Un groupe adopté ainsi reste à migrer : on ne lui distribue pas.
	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+cree.ID+"/assignments/preview",
		map[string]any{"name": "tp3", "settings": map[string]any{}})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
}

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

	noms := h.State.RepoNames("acme")
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
		Moved   string `json:"moved"`
		Renamed int    `json:"renamed"`
	}
	h.json(http.MethodPost, "/api/classrooms/"+depart+"/students/move",
		map[string]any{"username": "jlpicard", "target": arrivee, "repos": false}, &bilan)
	if bilan.Moved != "jlpicard" || bilan.Renamed != 0 {
		t.Fatalf("bilan : %+v", bilan)
	}
	if noms := h.State.RepoNames("acme"); len(noms) != 1 ||
		noms[0] != "a26.5n6.01.tp1.jean-luc-picard" {
		t.Fatalf("aucun dépôt ne devait bouger : %v", noms)
	}
}

func TestDeplacementRefuseUnGroupeQuiNeSaitPasNommer(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	state.AddRepo("acme", "vieux-tp1-jlpicard", true)
	h := nouveau(t, state)
	depart := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")
	arrivee := h.heritage("vieux", "emilie-cote")

	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+depart+"/students/move",
		map[string]any{"username": "jlpicard", "target": arrivee, "repos": true})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d — %s", reponse.StatusCode, contenu)
	}
	if !strings.Contains(string(contenu), "nomenclature dépassée") {
		t.Fatalf("message : %s", contenu)
	}
	// Rien n'a bougé : ni les dépôts, ni les listes.
	if noms := h.State.RepoNames("acme"); len(noms) != 2 {
		t.Fatalf("dépôts : %v", noms)
	}
}

// ------------------------------------------------- renommer un groupe déclaré

func TestGroupeDeclareSeRenomme(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	h := nouveau(t, state)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")

	var apercu struct {
		Scope string `json:"scope"`
		Ready int    `json:"ready"`
	}
	h.json(http.MethodPost, "/api/classrooms/"+id+"/migration/preview",
		map[string]any{"session": "h27", "course": "5n6", "group": "02"}, &apercu)
	if apercu.Scope != "h27.5n6.02" || apercu.Ready != 1 {
		t.Fatalf("aperçu : %+v", apercu)
	}

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+id+"/migration/apply",
		map[string]any{"session": "h27", "course": "5n6", "group": "02"})
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["renamed"] != float64(1) || resultat["switched"] != true {
		t.Fatalf("bilan : %+v", resultat)
	}
	if noms := h.State.RepoNames("acme"); len(noms) != 1 ||
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
