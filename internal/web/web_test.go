package web_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/web"
)

// harnais monte l'interface web au-dessus d'un faux GitHub, sur un vrai port
// local : le filtrage des requêtes est ainsi éprouvé comme en usage réel.
type harnais struct {
	t       *testing.T
	State   *fakegh.State
	Serveur *web.Server
	Client  *http.Client
	Base    string
}

func nouveau(t *testing.T, state *fakegh.State) *harnais {
	t.Helper()
	if state == nil {
		state = fakegh.NewState()
	}
	faux := fakegh.New(state)
	t.Cleanup(faux.Close)

	client, err := ghapi.New(ghapi.Options{
		Host: "github.com", Token: "jeton-de-test", BaseURL: faux.URL(),
		Sleep: func(time.Duration) {}, Now: time.Now,
	})
	if err != nil {
		t.Fatalf("client GitHub : %v", err)
	}
	// L'interface reçoit un client déjà authentifié, comme le fait la session
	// du terminal : c'est cet appel qui révèle les portées du jeton.
	if _, err := client.AuthenticatedUser(); err != nil {
		t.Fatalf("authentification : %v", err)
	}

	dossier := t.TempDir()
	serveur, err := web.New(web.Deps{
		Client:     client,
		Cache:      cache.NewIn(filepath.Join(dossier, "cache"), true),
		Settings:   config.Default(),
		ConfigFile: filepath.Join(dossier, "config.json"),
		Viewer:     state.Viewer,
		Host:       "github.com",
		Version:    "test",
		ReportDir:  filepath.Join(dossier, "rapports"),
		Jobs:       2,
		SaveConfig: true,
	})
	if err != nil {
		t.Fatalf("interface web : %v", err)
	}

	lifetime, arret := context.WithCancel(context.Background())
	fini := make(chan struct{})
	go func() {
		defer close(fini)
		if err := serveur.Serve(lifetime); err != nil {
			t.Errorf("service interrompu : %v", err)
		}
	}()
	t.Cleanup(func() {
		arret()
		<-fini
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("bocal à témoins : %v", err)
	}
	h := &harnais{
		t: t, State: state, Serveur: serveur, Base: serveur.Address(),
		Client: &http.Client{Jar: jar, Timeout: 20 * time.Second},
	}
	// L'adresse remise dans le terminal ouvre la session et pose le témoin.
	reponse, err := h.Client.Get(serveur.URL())
	if err != nil {
		t.Fatalf("ouverture de session : %v", err)
	}
	reponse.Body.Close()
	return h
}

// requete envoie une requête d'interface complète, en-tête comprise.
func (h *harnais) requete(methode, chemin string, corps any) (*http.Response, []byte) {
	h.t.Helper()
	var lecteur io.Reader
	if corps != nil {
		encode, err := json.Marshal(corps)
		if err != nil {
			h.t.Fatalf("encodage : %v", err)
		}
		lecteur = bytes.NewReader(encode)
	}
	requete, err := http.NewRequest(methode, h.Base+chemin, lecteur)
	if err != nil {
		h.t.Fatalf("requête : %v", err)
	}
	requete.Header.Set("Origin", h.Base)
	requete.Header.Set("X-Cohorte", "1")
	if corps != nil {
		requete.Header.Set("Content-Type", "application/json")
	}
	reponse, err := h.Client.Do(requete)
	if err != nil {
		h.t.Fatalf("appel %s %s : %v", methode, chemin, err)
	}
	defer reponse.Body.Close()
	contenu, err := io.ReadAll(reponse.Body)
	if err != nil {
		h.t.Fatalf("lecture : %v", err)
	}
	return reponse, contenu
}

// json envoie une requête et décode la réponse, en exigeant un succès.
func (h *harnais) json(methode, chemin string, corps any, cible any) {
	h.t.Helper()
	reponse, contenu := h.requete(methode, chemin, corps)
	if reponse.StatusCode >= 300 {
		h.t.Fatalf("%s %s : statut %d — %s", methode, chemin, reponse.StatusCode, contenu)
	}
	if cible == nil {
		return
	}
	if err := json.Unmarshal(contenu, cible); err != nil {
		h.t.Fatalf("réponse illisible (%s) : %v", contenu, err)
	}
}

// groupe déclare un groupe de la nomenclature courante.
func (h *harnais) groupe(session, cours, section string, couples ...string) string {
	h.t.Helper()
	etudiants := make([]map[string]string, 0, len(couples)/2)
	for index := 0; index+1 < len(couples); index += 2 {
		etudiants = append(etudiants, map[string]string{
			"full_name": couples[index], "username": couples[index+1],
		})
	}
	var cree struct {
		ID string `json:"id"`
	}
	h.json(http.MethodPost, "/api/classrooms", map[string]any{
		"org": "acme", "session": session, "course": cours, "group": section,
		"name": cours + " " + section, "students": etudiants,
	}, &cree)
	if cree.ID == "" {
		h.t.Fatal("groupe sans identifiant")
	}
	return cree.ID
}

// heritage déclare un groupe resté à l'ancienne nomenclature.
func (h *harnais) heritage(prefixe string, comptes ...string) string {
	h.t.Helper()
	// Un groupe adopté depuis des dépôts hérités ne connaît que les comptes :
	// les noms complets restent à retrouver.
	etudiants := make([]map[string]string, 0, len(comptes))
	for _, compte := range comptes {
		etudiants = append(etudiants, map[string]string{"username": compte, "full_name": ""})
	}
	var cree struct {
		ID string `json:"id"`
	}
	h.json(http.MethodPost, "/api/classrooms", map[string]any{
		"org": "acme", "prefix": prefixe, "name": prefixe, "students": etudiants,
	}, &cree)
	if cree.ID == "" {
		h.t.Fatal("groupe sans identifiant")
	}
	return cree.ID
}

// travail lance une opération en arrière-plan et attend son bilan.
func (h *harnais) travail(methode, chemin string, corps any) map[string]any {
	h.t.Helper()
	var fiche struct {
		ID string `json:"id"`
	}
	h.json(methode, chemin, corps, &fiche)
	if fiche.ID == "" {
		h.t.Fatal("aucun identifiant de travail")
	}
	limite := time.Now().Add(15 * time.Second)
	for time.Now().Before(limite) {
		var etat map[string]any
		h.json(http.MethodGet, "/api/jobs/"+fiche.ID, nil, &etat)
		if etat["status"] != "en cours" {
			return etat
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("travail %s toujours en cours", fiche.ID)
	return nil
}

// ------------------------------------------------------------------ sécurité

func TestSessionRefuseeSansTemoin(t *testing.T) {
	h := nouveau(t, nil)
	nu := &http.Client{Timeout: 5 * time.Second}
	reponse, err := nu.Get(h.Base + "/api/context")
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	defer reponse.Body.Close()
	if reponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("statut %d, attendu 401", reponse.StatusCode)
	}
}

func TestJetonInvalideRefuse(t *testing.T) {
	h := nouveau(t, nil)
	nu := &http.Client{Timeout: 5 * time.Second}
	reponse, err := nu.Get(h.Base + "/?jeton=faux")
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	defer reponse.Body.Close()
	if reponse.StatusCode != http.StatusForbidden {
		t.Fatalf("statut %d, attendu 403", reponse.StatusCode)
	}
}

func TestHoteInattenduRefuse(t *testing.T) {
	h := nouveau(t, nil)
	requete, _ := http.NewRequest(http.MethodGet, h.Base+"/api/context", nil)
	requete.Host = "exemple.test"
	reponse, err := h.Client.Do(requete)
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	defer reponse.Body.Close()
	if reponse.StatusCode != http.StatusForbidden {
		t.Fatalf("statut %d, attendu 403", reponse.StatusCode)
	}
}

func TestOrigineEtrangereRefusee(t *testing.T) {
	h := nouveau(t, nil)
	requete, _ := http.NewRequest(http.MethodPost, h.Base+"/api/cache/clear", nil)
	requete.Header.Set("Origin", "http://exemple.test")
	requete.Header.Set("X-Cohorte", "1")
	reponse, err := h.Client.Do(requete)
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	defer reponse.Body.Close()
	if reponse.StatusCode != http.StatusForbidden {
		t.Fatalf("statut %d, attendu 403", reponse.StatusCode)
	}
}

func TestEcritureSansEnteteRefusee(t *testing.T) {
	h := nouveau(t, nil)
	requete, _ := http.NewRequest(http.MethodPost, h.Base+"/api/cache/clear", nil)
	requete.Header.Set("Origin", h.Base)
	reponse, err := h.Client.Do(requete)
	if err != nil {
		t.Fatalf("appel : %v", err)
	}
	defer reponse.Body.Close()
	if reponse.StatusCode != http.StatusForbidden {
		t.Fatalf("statut %d, attendu 403", reponse.StatusCode)
	}
}

// ------------------------------------------------------------------- lecture

func TestContexteDecritLaSession(t *testing.T) {
	h := nouveau(t, nil)
	var contexte struct {
		Viewer   string            `json:"viewer"`
		Version  string            `json:"version"`
		Scopes   map[string]string `json:"scopes"`
		Settings config.Settings   `json:"settings"`
	}
	h.json(http.MethodGet, "/api/context", nil, &contexte)

	if contexte.Viewer != "prof" {
		t.Fatalf("compte %q, attendu « prof »", contexte.Viewer)
	}
	if contexte.Scopes["repo"] != "présente" {
		t.Fatalf("portée repo : %q", contexte.Scopes["repo"])
	}
	if contexte.Settings.NamePattern != config.DefaultNamePattern {
		t.Fatalf("gabarit %q", contexte.Settings.NamePattern)
	}
}

func TestListeCollectiveLue(t *testing.T) {
	h := nouveau(t, nil)

	var liste struct {
		People []struct {
			FullName string `json:"full_name"`
			Username string `json:"username"`
		} `json:"people"`
		Issues []struct {
			Message string `json:"message"`
		} `json:"issues"`
	}
	h.json(http.MethodPost, "/api/roster/parse", map[string]any{
		"text": "Jean-Luc Picard, jlpicard\n\u00c9milie C\u00f4t\u00e9, emilie-cote\nligne bancale\n",
	}, &liste)
	if len(liste.People) != 2 {
		t.Fatalf("%d personne(s) retenue(s), attendu 2", len(liste.People))
	}
	if len(liste.Issues) != 1 {
		t.Fatalf("%d rejet(s), attendu 1", len(liste.Issues))
	}
}

// ------------------------------------------------------------------- groupes

func TestGroupeDeclareEtRelu(t *testing.T) {
	h := nouveau(t, nil)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard", "Émilie Côté", "emilie-cote")

	var fiche struct {
		ID       string `json:"id"`
		Session  string `json:"session"`
		Course   string `json:"course"`
		Group    string `json:"group"`
		Students []struct {
			Username string `json:"username"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+id, nil, &fiche)
	if fiche.Session != "a26" || fiche.Course != "5n6" || fiche.Group != "01" || len(fiche.Students) != 2 {
		t.Fatalf("groupe relu : %+v", fiche)
	}

	var liste struct {
		Classrooms []struct {
			ID string `json:"id"`
		} `json:"classrooms"`
	}
	h.json(http.MethodGet, "/api/classrooms", nil, &liste)
	if len(liste.Classrooms) != 1 || liste.Classrooms[0].ID != id {
		t.Fatalf("liste des groupes : %+v", liste)
	}
}

func TestPlusieursGroupesDansUnCours(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	state.AddRepo("acme", "a26.5n6.02.tp1.emilie-cote", true)
	h := nouveau(t, state)

	premier := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")
	second := h.groupe("a26", "5n6", "02", "Émilie Côté", "emilie-cote")

	for _, cas := range []struct {
		id      string
		attendu string
	}{{premier, "a26.5n6.01.tp1"}, {second, "a26.5n6.02.tp1"}} {
		var fiche struct {
			Assignments []struct {
				ID       string `json:"id"`
				Repos    int    `json:"repos"`
				Students int    `json:"students"`
			} `json:"assignments"`
		}
		h.json(http.MethodGet, "/api/classrooms/"+cas.id, nil, &fiche)
		if len(fiche.Assignments) != 1 || fiche.Assignments[0].ID != cas.attendu {
			t.Fatalf("travaux du groupe : %+v", fiche.Assignments)
		}
		if fiche.Assignments[0].Repos != 1 || fiche.Assignments[0].Students != 1 {
			t.Fatalf("comptage : %+v", fiche.Assignments[0])
		}
	}
}

func TestGroupeHeriteResteLisibleMaisPasDistribuable(t *testing.T) {
	// La nomenclature déjà en place continue de s'afficher sans rien renommer.
	state := fakegh.NewState()
	for _, nom := range []string{
		"a26-5n6-travailsession-jlpicard", "a26-5n6-travailsession-emilie-cote",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	id := h.heritage("a26-5n6", "jlpicard", "emilie-cote")

	var fiche struct {
		Legacy      bool `json:"-"`
		Assignments []struct {
			Name  string `json:"name"`
			ID    string `json:"id"`
			Repos int    `json:"repos"`
		} `json:"assignments"`
		Prefix string `json:"prefix"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+id, nil, &fiche)
	if fiche.Prefix != "a26-5n6" {
		t.Fatalf("préfixe hérité perdu : %+v", fiche)
	}
	if len(fiche.Assignments) != 1 || fiche.Assignments[0].ID != "a26-5n6-travailsession" ||
		fiche.Assignments[0].Repos != 2 {
		t.Fatalf("travaux du groupe hérité : %+v", fiche.Assignments)
	}

	// Mais on ne lui distribue plus : il faut d'abord le migrer.
	reponse, contenu := h.requete(http.MethodPost, "/api/classrooms/"+id+"/assignments",
		map[string]any{"name": "tp1"})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400", reponse.StatusCode)
	}
	if !strings.Contains(string(contenu), "ancienne nomenclature") {
		t.Fatalf("message : %s", contenu)
	}
}

func TestCandidatsProposesDepuisLesDepots(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"a26.5n6.01.tp1.jean-luc-picard", "a26.5n6.01.tp1.emilie-cote",
		"a26-4w6-tp1-jlpicard", "a26-4w6-tp1-emilie-cote",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)

	var reponse struct {
		Candidates []struct {
			Prefix   string   `json:"prefix"`
			Session  string   `json:"session"`
			Course   string   `json:"course"`
			Group    string   `json:"group"`
			Legacy   bool     `json:"legacy"`
			Students []string `json:"students"`
		} `json:"candidates"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/candidates", nil, &reponse)
	trouves := map[string]bool{}
	for _, candidat := range reponse.Candidates {
		trouves[candidat.Prefix] = candidat.Legacy
	}
	if herite, present := trouves["a26.5n6.01"]; !present || herite {
		t.Fatalf("candidat courant : %+v", reponse.Candidates)
	}
	if herite, present := trouves["a26-4w6"]; !present || !herite {
		t.Fatalf("candidat hérité : %+v", reponse.Candidates)
	}

	// Une fois le groupe déclaré, sa place n'est plus proposée.
	h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")
	h.json(http.MethodGet, "/api/orgs/acme/candidates", nil, &reponse)
	for _, candidat := range reponse.Candidates {
		if candidat.Prefix == "a26.5n6.01" {
			t.Fatalf("la place déjà couverte est encore proposée : %+v", reponse.Candidates)
		}
	}
}

func TestApercuAvantDistribution(t *testing.T) {
	h := nouveau(t, nil)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard", "Émilie Côté", "emilie-cote")

	var apercu struct {
		Assignment string `json:"assignment"`
		ShortName  string `json:"short_name"`
		Items      []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	h.json(http.MethodPost, "/api/classrooms/"+id+"/assignments/preview",
		map[string]any{"name": "tp1"}, &apercu)

	if apercu.Assignment != "a26.5n6.01.tp1" || apercu.ShortName != "tp1" {
		t.Fatalf("aperçu : %+v", apercu)
	}
	// Le nom du dépôt porte le nom de l'étudiant, plus son compte.
	noms := make([]string, 0, len(apercu.Items))
	for _, item := range apercu.Items {
		noms = append(noms, item.Name)
	}
	sort.Strings(noms)
	attendu := "a26.5n6.01.tp1.emilie-cote,a26.5n6.01.tp1.jean-luc-picard"
	if strings.Join(noms, ",") != attendu {
		t.Fatalf("dépôts prévus : %v", noms)
	}
}

func TestNomCompletManquantBloqueLaDistribution(t *testing.T) {
	h := nouveau(t, nil)
	// « aminata-d » n'a pas de nom complet : son dépôt ne peut pas être nommé.
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard", "", "aminata-d")

	reponse, contenu := h.requete(http.MethodPost, "/api/classrooms/"+id+"/assignments/preview",
		map[string]any{"name": "tp1"})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400", reponse.StatusCode)
	}
	if !strings.Contains(string(contenu), "aminata-d") {
		t.Fatalf("message peu explicite : %s", contenu)
	}
}

func TestHomonymesRefusesAvantTouteEcriture(t *testing.T) {
	h := nouveau(t, nil)
	id := h.groupe("a26", "5n6", "01",
		"Jean Tremblay", "jtremblay", "Jean Tremblay", "jean-t")

	reponse, contenu := h.requete(http.MethodPost, "/api/classrooms/"+id+"/assignments",
		map[string]any{"name": "tp1"})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400", reponse.StatusCode)
	}
	if !strings.Contains(string(contenu), "Jean Tremblay") {
		t.Fatalf("message : %s", contenu)
	}
	if noms := h.State.RepoNames("acme"); len(noms) != 0 {
		t.Fatalf("des dépôts ont été créés : %v", noms)
	}
}

// ------------------------------------------------------------------ écriture

func TestDistributionAToutLeGroupe(t *testing.T) {
	h := nouveau(t, nil)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard", "Émilie Côté", "emilie-cote")

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+id+"/assignments",
		map[string]any{"name": "tp1"})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["created"] != float64(2) {
		t.Fatalf("%v dépôt(s) créé(s), attendu 2", resultat["created"])
	}
	noms := h.State.RepoNames("acme")
	sort.Strings(noms)
	attendu := "a26.5n6.01.tp1.emilie-cote,a26.5n6.01.tp1.jean-luc-picard"
	if strings.Join(noms, ",") != attendu {
		t.Fatalf("dépôts créés : %v", noms)
	}
}

func TestSimulationNeCreeRien(t *testing.T) {
	h := nouveau(t, nil)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+id+"/assignments",
		map[string]any{"name": "tp1", "dry_run": true})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v", bilan["status"])
	}
	if noms := h.State.RepoNames("acme"); len(noms) != 0 {
		t.Fatalf("la simulation a créé %v", noms)
	}
}

func TestRedistributionEcarteLesDejaServis(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	h := nouveau(t, state)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard", "Émilie Côté", "emilie-cote")

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+id+"/assignments",
		map[string]any{"name": "tp1"})
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["created"] != float64(1) {
		t.Fatalf("%v dépôt(s) créé(s), attendu 1", resultat["created"])
	}
	ecartes, _ := resultat["skipped"].([]any)
	if len(ecartes) != 1 {
		t.Fatalf("%d étudiant(s) écarté(s), attendu 1", len(ecartes))
	}

	reponse, contenu := h.requete(http.MethodPost, "/api/classrooms/"+id+"/assignments",
		map[string]any{"name": "tp1"})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400 — %s", reponse.StatusCode, contenu)
	}
}

func TestDistributionRestreinteAQuelquesEtudiants(t *testing.T) {
	h := nouveau(t, nil)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard", "Émilie Côté", "emilie-cote")

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+id+"/assignments",
		map[string]any{"name": "rattrapage", "usernames": []string{"emilie-cote"}})
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["created"] != float64(1) {
		t.Fatalf("%v dépôt(s) créé(s), attendu 1", resultat["created"])
	}
	if noms := h.State.RepoNames("acme"); len(noms) != 1 ||
		noms[0] != "a26.5n6.01.rattrapage.emilie-cote" {
		t.Fatalf("dépôts créés : %v", noms)
	}
}

func TestSelectionVideNeSertPersonne(t *testing.T) {
	h := nouveau(t, nil)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard", "Émilie Côté", "emilie-cote")

	// Une sélection explicitement vide ne doit pas être lue comme « tout le
	// groupe » : décocher tout le monde ne crée rien.
	reponse, contenu := h.requete(http.MethodPost, "/api/classrooms/"+id+"/assignments",
		map[string]any{"name": "tp1", "usernames": []string{}})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400 — %s", reponse.StatusCode, contenu)
	}
	if noms := h.State.RepoNames("acme"); len(noms) != 0 {
		t.Fatalf("des dépôts ont été créés : %v", noms)
	}
}

func TestDetailDUnTravail(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"a26.5n6.01.tp1.jean-luc-picard", "a26.5n6.01.tp1.visiteur-inconnu",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	id := h.groupe("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard")

	var detail struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Repos []struct {
			Name     string `json:"name"`
			Student  string `json:"student"`
			FullName string `json:"full_name"`
			Username string `json:"username"`
		} `json:"repos"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+id+"/assignments/tp1", nil, &detail)
	if detail.ID != "a26.5n6.01.tp1" || len(detail.Repos) != 2 {
		t.Fatalf("détail : %+v", detail)
	}
	// Le dépôt d'un étudiant inscrit porte son nom et son compte ; celui d'un
	// inconnu reste visible, sans compte.
	parNom := map[string]string{}
	for _, repo := range detail.Repos {
		parNom[repo.Student] = repo.Username
	}
	if parNom["jean-luc-picard"] != "jlpicard" {
		t.Fatalf("dépôts : %+v", detail.Repos)
	}
	if parNom["visiteur-inconnu"] != "" {
		t.Fatalf("un dépôt hors liste a été rattaché : %+v", detail.Repos)
	}
}

func TestEtudiantsDuGroupeCroisesAvecLesTravaux(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"a26.5n6.01.tp1.jean-luc-picard", "a26.5n6.01.tp1.emilie-cote",
		"a26.5n6.01.travail-session.jean-luc-picard",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	id := h.groupe("a26", "5n6", "01",
		"Jean-Luc Picard", "jlpicard", "Émilie Côté", "emilie-cote", "Aminata Diallo", "aminata-d")

	var reponse struct {
		Students []struct {
			Username    string `json:"username"`
			Assignments []struct {
				Name string `json:"name"`
			} `json:"assignments"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+id+"/students", nil, &reponse)
	if len(reponse.Students) != 3 {
		t.Fatalf("%d étudiant(s), attendu 3", len(reponse.Students))
	}
	compte := map[string]int{}
	for _, etudiant := range reponse.Students {
		compte[etudiant.Username] = len(etudiant.Assignments)
	}
	if compte["jlpicard"] != 2 || compte["emilie-cote"] != 1 || compte["aminata-d"] != 0 {
		t.Fatalf("travaux par étudiant : %v", compte)
	}
}

func TestSuppressionExigeLeNomExact(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-jlpicard", true)
	h := nouveau(t, state)

	reponse, _ := h.requete(http.MethodDelete, "/api/orgs/acme/repos/tp1-jlpicard",
		map[string]string{"confirm": "tp1"})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400", reponse.StatusCode)
	}
	if len(h.State.RepoNames("acme")) != 1 {
		t.Fatal("le dépôt a été supprimé malgré une confirmation incorrecte")
	}

	h.json(http.MethodDelete, "/api/orgs/acme/repos/tp1-jlpicard",
		map[string]string{"confirm": "tp1-jlpicard"}, nil)
	if noms := h.State.RepoNames("acme"); len(noms) != 0 {
		t.Fatalf("dépôts restants : %v", noms)
	}
}

func TestAccesEtCollaborateurs(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-jlpicard", true)
	h := nouveau(t, state)

	h.json(http.MethodPost, "/api/orgs/acme/repos/tp1-jlpicard/collaborators",
		map[string]string{"username": "jlpicard", "permission": "push"}, nil)

	var acces struct {
		Collaborators []string `json:"collaborators"`
		Invitations   []struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
		} `json:"invitations"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/repos/tp1-jlpicard/access", nil, &acces)
	if len(acces.Invitations) != 1 || acces.Invitations[0].Login != "jlpicard" {
		t.Fatalf("invitations : %+v", acces.Invitations)
	}

	h.json(http.MethodDelete,
		"/api/orgs/acme/repos/tp1-jlpicard/invitations/"+itoa(acces.Invitations[0].ID), nil, nil)
	h.json(http.MethodGet, "/api/orgs/acme/repos/tp1-jlpicard/access", nil, &acces)
	if len(acces.Invitations) != 0 {
		t.Fatalf("invitation encore présente : %+v", acces.Invitations)
	}
}

func TestCompteInexistantRefuse(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-jlpicard", true)
	h := nouveau(t, state)

	reponse, contenu := h.requete(http.MethodPost, "/api/orgs/acme/repos/tp1-jlpicard/collaborators",
		map[string]string{"username": "fantome", "permission": "push"})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400", reponse.StatusCode)
	}
	if !strings.Contains(string(contenu), "n'existe pas") {
		t.Fatalf("message : %s", contenu)
	}
}

// ------------------------------------------------------------------- travaux

func TestFluxDEvenementsSuitUnTravail(t *testing.T) {
	h := nouveau(t, nil)
	var fiche struct {
		ID string `json:"id"`
	}
	h.json(http.MethodPost, "/api/accounts/verify", map[string]any{
		"people": []map[string]string{
			{"full_name": "Jean-Luc Picard", "username": "jlpicard"},
			{"full_name": "Fantôme", "username": "fantome"},
		},
	}, &fiche)

	requete, _ := http.NewRequest(http.MethodGet, h.Base+"/api/jobs/"+fiche.ID+"/events", nil)
	reponse, err := h.Client.Do(requete)
	if err != nil {
		t.Fatalf("flux : %v", err)
	}
	defer reponse.Body.Close()
	if type_ := reponse.Header.Get("Content-Type"); !strings.HasPrefix(type_, "text/event-stream") {
		t.Fatalf("type de contenu %q", type_)
	}

	var natures []string
	lecteur := bufio.NewScanner(reponse.Body)
	for lecteur.Scan() {
		ligne := lecteur.Text()
		if !strings.HasPrefix(ligne, "data: ") {
			continue
		}
		var evenement struct {
			Kind string `json:"kind"`
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(ligne, "data: ")), &evenement); err != nil {
			t.Fatalf("événement illisible : %v", err)
		}
		natures = append(natures, evenement.Kind)
		if evenement.Kind == "fin" {
			if evenement.Data.Status != "terminé" {
				t.Fatalf("issue %q", evenement.Data.Status)
			}
			break
		}
	}
	if len(natures) == 0 || natures[len(natures)-1] != "fin" {
		t.Fatalf("événements reçus : %v", natures)
	}
	if !contient(natures, "avancement") || !contient(natures, "avertissement") {
		t.Fatalf("le flux ne rapporte pas le détail : %v", natures)
	}
}

func TestFermetureDemandeeParLInterface(t *testing.T) {
	h := nouveau(t, nil)
	h.json(http.MethodPost, "/api/quit", nil, nil)

	// Le serveur se retire : les appels suivants ne joignent plus personne.
	limite := time.Now().Add(5 * time.Second)
	for time.Now().Before(limite) {
		requete, _ := http.NewRequest(http.MethodGet, h.Base+"/api/context", nil)
		if _, err := h.Client.Do(requete); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("le serveur répond encore après la demande de fermeture")
}

func contient(valeurs []string, cherche string) bool {
	for _, valeur := range valeurs {
		if valeur == cherche {
			return true
		}
	}
	return false
}

func itoa(valeur int64) string { return strconv.FormatInt(valeur, 10) }

// ------------------------------------------------------------------ réglages

func TestReglagesMemorises(t *testing.T) {
	h := nouveau(t, nil)
	reglages := config.Default()
	reglages.Org, reglages.Assignment = "acme", "tp1"

	var bilan struct {
		Saved bool   `json:"saved"`
		Path  string `json:"path"`
	}
	h.json(http.MethodPut, "/api/settings", reglages, &bilan)
	if !bilan.Saved {
		t.Fatal("réglages non enregistrés")
	}
	contenu, err := os.ReadFile(bilan.Path)
	if err != nil {
		t.Fatalf("lecture des réglages : %v", err)
	}
	if !strings.Contains(string(contenu), `"assignment": "tp1"`) {
		t.Fatalf("réglages écrits : %s", contenu)
	}
}

func TestReglagesInvalidesRefuses(t *testing.T) {
	h := nouveau(t, nil)
	reglages := config.Default()
	reglages.Visibility = "secret"

	reponse, contenu := h.requete(http.MethodPut, "/api/settings", reglages)
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400", reponse.StatusCode)
	}
	if !strings.Contains(string(contenu), "secret") {
		t.Fatalf("message : %s", contenu)
	}
}

func TestNomsCompletsRetrouves(t *testing.T) {
	h := nouveau(t, nil)
	// Les comptes sont connus, les noms complets non : c'est GitHub qui les donne.
	id := h.heritage("a26-5n6", "jlpicard", "emilie-cote")

	var avant struct {
		Students []struct {
			FullName string `json:"full_name"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+id, nil, &avant)

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+id+"/students/names", nil)
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["resolved"] != float64(2) {
		t.Fatalf("%v nom(s) retrouvé(s), attendu 2", resultat["resolved"])
	}

	var apres struct {
		Students []struct {
			Username string `json:"username"`
			FullName string `json:"full_name"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+id, nil, &apres)
	noms := map[string]string{}
	for _, student := range apres.Students {
		noms[student.Username] = student.FullName
	}
	if noms["jlpicard"] != "Jean-Luc Picard" {
		t.Fatalf("noms retenus : %v", noms)
	}
}

func TestPageServie(t *testing.T) {
	h := nouveau(t, nil)
	reponse, contenu := h.requete(http.MethodGet, "/", nil)
	if reponse.StatusCode != http.StatusOK {
		t.Fatalf("statut %d", reponse.StatusCode)
	}
	if !strings.Contains(string(contenu), "gh cohorte") {
		t.Fatalf("page inattendue : %.100s", contenu)
	}
	// Une adresse inconnue rend la page : l'interface se recharge sans erreur.
	reponse, _ = h.requete(http.MethodGet, "/groupes", nil)
	if reponse.StatusCode != http.StatusOK {
		t.Fatalf("statut %d pour une adresse inconnue", reponse.StatusCode)
	}
}

// ----------------------------------------------------------------- migration

func TestMigrationRenommeLesDepots(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{
		"a26-5n6-travailsession-jlpicard", "a26-5n6-travailsession-emilie-cote",
		"a26-5n6-tp1-jlpicard",
	} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)
	id := h.heritage("a26-5n6", "jlpicard", "emilie-cote")

	// Les noms complets sont nécessaires : c'est eux qui nomment les dépôts.
	h.travail(http.MethodPost, "/api/classrooms/"+id+"/students/names", nil)

	var apercu struct {
		Course  string `json:"course"`
		Group   string `json:"group"`
		Ready   int    `json:"ready"`
		Blocked int    `json:"blocked"`
		Rows    []struct {
			Repo    string `json:"repo"`
			Target  string `json:"target"`
			Problem string `json:"problem"`
		} `json:"rows"`
	}
	h.json(http.MethodPost, "/api/classrooms/"+id+"/migration/preview",
		map[string]any{"session": "a26", "course": "5n6", "group": "01"}, &apercu)
	if apercu.Ready != 3 || apercu.Blocked != 0 {
		t.Fatalf("aperçu : %+v", apercu)
	}
	cibles := map[string]string{}
	for _, ligne := range apercu.Rows {
		cibles[ligne.Repo] = ligne.Target
	}
	if cibles["a26-5n6-tp1-jlpicard"] != "a26.5n6.01.tp1.jean-luc-picard" {
		t.Fatalf("cibles : %v", cibles)
	}

	bilan := h.travail(http.MethodPost, "/api/classrooms/"+id+"/migration/apply",
		map[string]any{"session": "a26", "course": "5n6", "group": "01"})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["renamed"] != float64(3) || resultat["switched"] != true {
		t.Fatalf("bilan : %+v", resultat)
	}

	noms := h.State.RepoNames("acme")
	sort.Strings(noms)
	attendu := "a26.5n6.01.tp1.jean-luc-picard," +
		"a26.5n6.01.travailsession.emilie-cote,a26.5n6.01.travailsession.jean-luc-picard"
	if strings.Join(noms, ",") != attendu {
		t.Fatalf("dépôts après migration : %v", noms)
	}

	// Le groupe suit désormais la nouvelle nomenclature, et redevient
	// distribuable.
	var fiche struct {
		Session     string `json:"session"`
		Course      string `json:"course"`
		Group       string `json:"group"`
		Prefix      string `json:"prefix"`
		Assignments []struct {
			ID string `json:"id"`
		} `json:"assignments"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+id+"?refresh=1", nil, &fiche)
	if fiche.Session != "a26" || fiche.Course != "5n6" || fiche.Group != "01" || fiche.Prefix != "" {
		t.Fatalf("groupe après migration : %+v", fiche)
	}
	if len(fiche.Assignments) != 2 {
		t.Fatalf("travaux après migration : %+v", fiche.Assignments)
	}
}

func TestMigrationRefuseTantQuUnDepotEstBloque(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26-5n6-tp1-jlpicard", true)
	// « visiteur » n'est pas dans la liste du groupe : son dépôt ne peut pas
	// être renommé sans savoir de qui il s'agit.
	state.AddRepo("acme", "a26-5n6-tp1-visiteur", true)
	h := nouveau(t, state)
	id := h.heritage("a26-5n6", "jlpicard")
	h.travail(http.MethodPost, "/api/classrooms/"+id+"/students/names", nil)

	reponse, contenu := h.requete(http.MethodPost, "/api/classrooms/"+id+"/migration/apply",
		map[string]any{"session": "a26", "course": "5n6", "group": "01"})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400 — %s", reponse.StatusCode, contenu)
	}
	if noms := h.State.RepoNames("acme"); len(noms) != 2 ||
		!strings.HasPrefix(noms[0], "a26-5n6-") {
		t.Fatalf("des dépôts ont été renommés : %v", noms)
	}

	// En acceptant de les laisser en place, la migration passe — mais le
	// groupe ne bascule pas tant qu'un dépôt reste en arrière.
	bilan := h.travail(http.MethodPost, "/api/classrooms/"+id+"/migration/apply",
		map[string]any{"session": "a26", "course": "5n6", "group": "01", "skip_blocked": true})
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["renamed"] != float64(1) || resultat["skipped"] != float64(1) {
		t.Fatalf("bilan : %+v", resultat)
	}
	noms := h.State.RepoNames("acme")
	sort.Strings(noms)
	if strings.Join(noms, ",") != "a26-5n6-tp1-visiteur,a26.5n6.01.tp1.jean-luc-picard" {
		t.Fatalf("dépôts : %v", noms)
	}
}

func TestMigrationRefuseSansNomComplet(t *testing.T) {
	state := fakegh.NewState()
	// « aminata-d » a un profil GitHub sans nom complet.
	state.AddRepo("acme", "a26-5n6-tp1-aminata-d", true)
	h := nouveau(t, state)
	id := h.heritage("a26-5n6", "aminata-d")

	var apercu struct {
		Ready   int `json:"ready"`
		Blocked int `json:"blocked"`
		Rows    []struct {
			Problem string `json:"problem"`
		} `json:"rows"`
	}
	h.json(http.MethodPost, "/api/classrooms/"+id+"/migration/preview",
		map[string]any{"session": "a26", "course": "5n6", "group": "01"}, &apercu)
	if apercu.Blocked != 1 || apercu.Ready != 0 {
		t.Fatalf("aperçu : %+v", apercu)
	}
	if !strings.Contains(apercu.Rows[0].Problem, "aminata-d") {
		t.Fatalf("raison : %q", apercu.Rows[0].Problem)
	}
}
