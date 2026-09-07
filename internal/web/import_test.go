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
			FullName string `json:"full_name"`
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
	Splits    []struct {
		Prefix string `json:"prefix"`
		Count  int    `json:"count"`
	} `json:"splits"`
	Unconfirmed []string `json:"unconfirmed"`
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

// L'interface propose la place d'arrivée avant qu'on ait rien tapé : le cours
// vient du nom du fichier, le groupe de sa colonne, la session du premier
// commit du travail.
func TestLInterfaceDevineLaPlaceDArrivee(t *testing.T) {
	state := classroomOrg()
	state.Repos["acme/tp1-lyonnais"].History = []string{"2027-02-03T10:00:00Z"}
	h := nouveau(t, state)

	var place struct {
		Session string   `json:"session"`
		Course  string   `json:"course"`
		Group   string   `json:"group"`
		Groups  []string `json:"groups"`
	}
	// Une liste déposée dans la page : pas de chemin, mais un contenu et un
	// nom — et c'est le nom qui porte le cours.
	contenu, err := os.ReadFile(listeOmnivox(t))
	if err != nil {
		t.Fatal(err)
	}
	h.json(http.MethodPost, "/api/orgs/acme/import/place", map[string]any{
		"prefix": "tp1", "content": contenu,
		"filename": "ListeEtudiants_cours4204N6EM_gr1030.csv",
	}, &place)

	// Février tombe dans la session d'hiver ; le printemps, lui, ne se devine
	// jamais.
	if place.Session != "h27" {
		t.Fatalf("session = %q", place.Session)
	}
	if place.Course != "4N6" {
		t.Fatalf("cours = %q", place.Course)
	}
	// La colonne « Groupe » de la liste l'emporte sur le nom du fichier.
	if place.Group != "1030" || len(place.Groups) != 0 {
		t.Fatalf("groupe = %q, groupes = %v", place.Group, place.Groups)
	}
}

// Un dépôt sans historique ne dit rien de la session, et se taire vaut mieux
// qu'inventer une place où des dépôts iraient atterrir.
func TestSansHistoriqueLaSessionResteAChoisir(t *testing.T) {
	h := nouveau(t, classroomOrg())
	var place struct {
		Session string `json:"session"`
		Course  string `json:"course"`
	}
	h.json(http.MethodPost, "/api/orgs/acme/import/place", map[string]any{
		"prefix": "tp1", "path": listeOmnivox(t),
	}, &place)
	if place.Session != "" {
		t.Fatalf("session inventée : %q", place.Session)
	}
}

// L'interface peut ne reprendre que les dépôts dont l'étudiant est connu :
// les autres restent où ils sont.
func TestLInterfaceLaisseLesDepotsSansEtudiant(t *testing.T) {
	state := classroomOrg()
	state.AddRepo("acme", "tp1-visiteur-anonyme-42", true)
	h := nouveau(t, state)

	var plan planVue
	h.json(http.MethodPost, "/api/orgs/acme/import/preview", map[string]any{
		"prefix": "tp1", "name": "tp1", "scope": "a26.5n6.1030",
		"path": listeOmnivox(t), "named_only": true,
	}, &plan)

	if len(plan.Moves) != 3 {
		t.Fatalf("renommages = %+v", plan.Moves)
	}
	for _, ligne := range plan.Moves {
		if strings.Contains(ligne.Repo, "visiteur-anonyme") {
			t.Fatalf("un dépôt sans étudiant a été repris : %+v", ligne)
		}
	}
	// Il est quand même nommé : le laisser derrière en silence ferait croire
	// le travail entièrement repris.
	if len(plan.Unmatched) != 1 {
		t.Fatalf("dépôts sans personne = %v", plan.Unmatched)
	}
}

// Le cas rapporté : « kickmyb-firebase » est le travail, « Walid7Akk » le
// compte, et le nom seul ne dit pas où couper. Sans les accès, l'interface
// affichait « @firebase-Walid7Akk » et lui cherchait un nom.
func TestLeCompteVientDesAccesPasDuNom(t *testing.T) {
	state := fakegh.NewState()
	for nom, compte := range map[string]string{
		"kickmyb-firebase-ladamlarocque": "ladamlarocque",
		"kickmyb-firebase-felixbourassa": "felixbourassa",
		"kickmyb-firebase-lyonnais":      "lyonnais",
	} {
		state.AddRepo("acme", nom, true)
		state.AddCollaborator("acme/"+nom, compte, "push")
		// L'enseignant a accès à tout : il ne doit désigner personne.
		state.AddCollaborator("acme/"+nom, "prof", "admin")
	}
	h := nouveau(t, state)

	var plan planVue
	h.json(http.MethodPost, "/api/orgs/acme/import/preview", map[string]any{
		// Le préfixe deviné s'arrête à « kickmyb » : c'est ce que l'écran propose.
		"prefix": "kickmyb", "name": "kickmyb", "scope": "a26.5n6.1030",
		"path": listeOmnivox(t),
	}, &plan)

	if plan.Prefix != "kickmyb-firebase" {
		t.Fatalf("travail = %q : les accès devaient le révéler", plan.Prefix)
	}
	for _, trouve := range plan.Pairings {
		if strings.Contains(strings.ToLower(trouve.Login), "firebase") {
			t.Fatalf("le compte porte encore le travail : %+v", trouve)
		}
		if trouve.Entry.FullName == "" {
			t.Fatalf("le bon compte devait mener à quelqu'un : %+v", trouve)
		}
	}
	if len(plan.Unconfirmed) != 0 {
		t.Fatalf("tous les comptes viennent des accès : %v", plan.Unconfirmed)
	}
	cibles := map[string]string{}
	for _, ligne := range plan.Moves {
		cibles[ligne.Repo] = ligne.Target
	}
	if cibles["kickmyb-firebase-ladamlarocque"] != "a26.5n6.1030.kickmyb-firebase.laurent-adam-larocque" {
		t.Fatalf("cibles = %v", cibles)
	}
}

// Deux travaux sous un même préfixe : l'aperçu les nomme et n'écrit rien, et la
// reprise est refusée tant qu'on n'a pas choisi.
func TestUnPrefixeQuiCacheDeuxTravauxDemandeAChoisir(t *testing.T) {
	state := fakegh.NewState()
	for nom, compte := range map[string]string{
		"kickmyb-firebase-ladamlarocque": "ladamlarocque",
		"kickmyb-firebase-felixbourassa": "felixbourassa",
		"kickmyb-android-lyonnais":       "lyonnais",
	} {
		state.AddRepo("acme", nom, true)
		state.AddCollaborator("acme/"+nom, compte, "push")
	}
	h := nouveau(t, state)
	corps := map[string]any{
		"prefix": "kickmyb", "name": "kickmyb", "scope": "a26.5n6.1030",
		"path": listeOmnivox(t),
	}

	var plan planVue
	h.json(http.MethodPost, "/api/orgs/acme/import/preview", corps, &plan)
	if len(plan.Splits) != 2 || len(plan.Moves) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Splits[0].Prefix != "kickmyb-firebase" || plan.Splits[0].Count != 2 {
		t.Fatalf("travaux = %+v", plan.Splits)
	}

	// Écrire sans avoir choisi est refusé, et rien n'a bougé.
	reponse, contenu := h.requete(http.MethodPost, "/api/orgs/acme/import", corps)
	if reponse.StatusCode < 400 {
		t.Fatalf("statut = %d : la reprise devait être refusée", reponse.StatusCode)
	}
	if !strings.Contains(string(contenu), "un à la fois") {
		t.Fatalf("message = %s", contenu)
	}
	if noms := h.depots(); !slices.Contains(noms, "kickmyb-android-lyonnais") {
		t.Fatalf("un dépôt a été renommé : %v", noms)
	}

	// Le travail choisi, la reprise redevient ordinaire.
	corps["prefix"] = "kickmyb-firebase"
	corps["name"] = "kickmyb-firebase"
	var choisi planVue
	h.json(http.MethodPost, "/api/orgs/acme/import/preview", corps, &choisi)
	if len(choisi.Splits) != 1 || len(choisi.Moves) != 2 {
		t.Fatalf("plan = %+v", choisi)
	}
}
