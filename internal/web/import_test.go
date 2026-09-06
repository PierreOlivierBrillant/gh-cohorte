package web_test

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
)

// classroomOrg monte une organisation telle que GitHub Classroom la laisse.
func classroomOrg() *fakegh.State {
	state := fakegh.NewState()
	for _, nom := range []string{
		"tp1-ladamlarocque", "tp1-felixbourassa", "tp1-lyonnais",
		"projet-final-lyonnais", "projet-final-felixbourassa",
	} {
		state.AddRepo("acme", nom, true)
	}
	return state
}

// listeOmnivox écrit une liste telle qu'Omnivox l'exporte : Windows-1252, CRLF,
// champs blindés, aucun compte GitHub.
func listeOmnivox(t *testing.T) string {
	t.Helper()
	lignes := []string{
		"No étudiant;Groupe;Nom de l'étudiant;Prénom de l'étudiant;Code perm.;",
		`="1680229";="1030";="Adam-Larocque";="Laurent";="ADAL20059908";`,
		`="2143020";="1030";="Bourassa";="Félix";="BOUF68040412";`,
		`="1983429";="1030";="Lyonnais";="Étienne";="LYOE78040203";`,
	}
	var octets []byte
	for _, ligne := range lignes {
		for _, r := range ligne {
			octets = append(octets, byte(r))
		}
		octets = append(octets, '\r', '\n')
	}
	chemin := filepath.Join(t.TempDir(), "ListeEtudiants.csv")
	if err := os.WriteFile(chemin, octets, 0o600); err != nil {
		t.Fatal(err)
	}
	return chemin
}

// planVue est ce que l'aperçu rend.
type planVue struct {
	Prefix   string `json:"prefix"`
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	Pairings []struct {
		Login string `json:"login"`
		Entry struct {
			FullName string `json:"FullName"`
		} `json:"entry"`
		Score  int    `json:"score"`
		Reason string `json:"reason"`
	} `json:"pairings"`
	Moves []struct {
		Repo   string `json:"repo"`
		Target string `json:"target"`
	} `json:"moves"`
	Unmatched []string `json:"unmatched"`
	Absent    []string `json:"absent"`
}

// Ce que l'organisation porte hors nomenclature, et l'aide pour aller chercher
// la liste.
func TestDepotsHorsNomenclatureEtAide(t *testing.T) {
	h := nouveau(t, classroomOrg())
	var vue struct {
		Repos       []string `json:"repos"`
		Assignments []struct {
			Prefix string `json:"prefix"`
			Count  int    `json:"count"`
		} `json:"assignments"`
		Help string `json:"help"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/foreign", nil, &vue)
	if len(vue.Repos) != 5 {
		t.Fatalf("dépôts = %v", vue.Repos)
	}
	trouves := map[string]int{}
	for _, travail := range vue.Assignments {
		trouves[travail.Prefix] = travail.Count
	}
	if trouves["tp1"] != 3 || trouves["projet-final"] != 2 {
		t.Fatalf("travaux = %+v", vue.Assignments)
	}
	// L'aide dit où prendre la liste, à l'endroit où on la demande.
	for _, attendu := range []string{"Léa", "Pour Excel", "Code permanent"} {
		if !strings.Contains(vue.Help, attendu) {
			t.Fatalf("l'aide ne dit pas « %s » :\n%s", attendu, vue.Help)
		}
	}
}

// L'aperçu rapproche et compose le renommage, sans rien écrire.
func TestApercuDeLImportationNecritRien(t *testing.T) {
	state := classroomOrg()
	h := nouveau(t, state)

	var plan planVue
	h.json(http.MethodPost, "/api/orgs/acme/import/preview", map[string]any{
		"prefix": "tp1", "name": "tp1", "scope": "a26.5n6.1030",
		"path": listeOmnivox(t),
	}, &plan)

	if len(plan.Moves) != 3 || len(plan.Unmatched) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	cibles := map[string]string{}
	for _, ligne := range plan.Moves {
		cibles[ligne.Repo] = ligne.Target
	}
	if cibles["tp1-ladamlarocque"] != "a26.5n6.1030.tp1.laurent-adam-larocque" {
		t.Fatalf("cibles = %v", cibles)
	}
	// Chaque rapprochement dit par quoi il a été reconnu.
	for _, trouve := range plan.Pairings {
		if trouve.Entry.FullName == "" || trouve.Reason == "" {
			t.Fatalf("rapprochement muet : %+v", trouve)
		}
	}
	if noms := h.depots(); !slices.Contains(noms, "tp1-ladamlarocque") {
		t.Fatalf("un aperçu a renommé : %v", noms)
	}
}

// L'importation renomme, déclare le groupe, et confie les noms au registre.
func TestImportationParLInterfaceWeb(t *testing.T) {
	state := classroomOrg()
	h := nouveau(t, state)

	bilan := h.travail(http.MethodPost, "/api/orgs/acme/import", map[string]any{
		"prefix": "tp1", "name": "tp1", "scope": "a26.5n6.1030",
		"path": listeOmnivox(t),
	})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["renamed"] != float64(3) || resultat["failed"] != float64(0) {
		t.Fatalf("bilan = %+v", resultat)
	}

	noms := h.depots()
	if !slices.Contains(noms, "a26.5n6.1030.tp1.etienne-lyonnais") {
		t.Fatalf("dépôts = %v", noms)
	}
	// L'autre travail n'a pas bougé.
	if !slices.Contains(noms, "projet-final-lyonnais") {
		t.Fatalf("un travail non demandé a bougé : %v", noms)
	}
	// Les noms sont montés au registre.
	contenu := state.Files("acme/"+registry.RepoName, registry.Branch)[registry.StudentsFile]
	if !strings.Contains(contenu, "Laurent Adam-Larocque") {
		t.Fatalf("registre =\n%s", contenu)
	}
	// Et le groupe est déclaré, avec sa liste.
	var fiche struct {
		Students []struct {
			Username string `json:"username"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/a26.5n6.1030", nil, &fiche)
	if len(fiche.Students) != 3 {
		t.Fatalf("liste du groupe = %+v", fiche.Students)
	}
}

// Un rapprochement corrigé à l'écran l'emporte sur ce que l'outil avait deviné.
func TestUnRapprochementCorrigeLEmporte(t *testing.T) {
	h := nouveau(t, classroomOrg())
	var plan planVue
	h.json(http.MethodPost, "/api/orgs/acme/import/preview", map[string]any{
		"prefix": "tp1", "name": "tp1", "scope": "a26.5n6.1030",
		"people": []map[string]string{
			{"full_name": "Quelqu'un d'Autre", "username": "ladamlarocque"},
		},
	}, &plan)

	if len(plan.Moves) != 3 {
		t.Fatalf("plan = %+v", plan.Moves)
	}
	cibles := map[string]string{}
	for _, ligne := range plan.Moves {
		cibles[ligne.Repo] = ligne.Target
	}
	if cibles["tp1-ladamlarocque"] != "a26.5n6.1030.tp1.quelqu-un-d-autre" {
		t.Fatalf("la correction n'a pas été suivie : %v", cibles)
	}
	// Les deux autres comptes ne mènent à personne : ils gardent leur nom.
	if len(plan.Unmatched) != 2 {
		t.Fatalf("comptes sans personne = %v", plan.Unmatched)
	}
}

// Un travail qu'aucun dépôt ne porte se refuse plutôt que de rendre un plan
// vide qu'on prendrait pour un succès.
func TestUnTravailAbsentEstRefuse(t *testing.T) {
	h := nouveau(t, classroomOrg())
	reponse, contenu := h.requete(http.MethodPost, "/api/orgs/acme/import/preview",
		map[string]any{"prefix": "absent", "scope": "a26.5n6.1030", "path": listeOmnivox(t)})
	if reponse.StatusCode < 400 {
		t.Fatalf("statut = %d", reponse.StatusCode)
	}
	if !strings.Contains(string(contenu), "absent") {
		t.Fatalf("message = %s", contenu)
	}
}
