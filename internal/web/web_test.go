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

func TestGroupesDetectesEtOuverts(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{"tp1-jlpicard", "tp1-emilie-cote", "tp2-jlpicard", "notes"} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)

	var inventaire struct {
		Total  int `json:"total"`
		Groups []struct {
			Prefix string `json:"prefix"`
			Count  int    `json:"count"`
		} `json:"groups"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/groups", nil, &inventaire)
	if inventaire.Total != 4 {
		t.Fatalf("%d dépôt(s) inventorié(s), attendu 4", inventaire.Total)
	}
	if len(inventaire.Groups) == 0 || inventaire.Groups[0].Prefix != "tp1" {
		t.Fatalf("groupes détectés : %+v", inventaire.Groups)
	}

	var groupe struct {
		Prefix string `json:"prefix"`
		Repos  []struct {
			Name   string `json:"name"`
			Suffix string `json:"suffix"`
		} `json:"repos"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/groups/tp1", nil, &groupe)
	if len(groupe.Repos) != 2 {
		t.Fatalf("%d dépôt(s) dans le groupe, attendu 2", len(groupe.Repos))
	}
	if groupe.Repos[0].Suffix != "emilie-cote" {
		t.Fatalf("suffixe %q", groupe.Repos[0].Suffix)
	}
}

func TestGroupeInconnuRefuse(t *testing.T) {
	h := nouveau(t, nil)
	reponse, _ := h.requete(http.MethodGet, "/api/orgs/acme/groups/absent", nil)
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400", reponse.StatusCode)
	}
}

func TestListeCollectiveLueEtPlanifiee(t *testing.T) {
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
		"text": "Jean-Luc Picard, jlpicard\nÉmilie Côté, emilie-cote\nligne bancale\n",
	}, &liste)
	if len(liste.People) != 2 {
		t.Fatalf("%d personne(s) retenue(s), attendu 2", len(liste.People))
	}
	if len(liste.Issues) != 1 {
		t.Fatalf("%d rejet(s), attendu 1", len(liste.Issues))
	}

	reglages := config.Default()
	reglages.Org, reglages.Assignment = "acme", "tp1"
	var plan struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	h.json(http.MethodPost, "/api/plan", map[string]any{
		"settings": reglages, "people": liste.People,
	}, &plan)
	if len(plan.Items) != 2 || plan.Items[0].Name != "tp1-jlpicard" {
		t.Fatalf("plan inattendu : %+v", plan.Items)
	}
}

func TestGabaritSansChampDistinctifRefuse(t *testing.T) {
	h := nouveau(t, nil)
	reglages := config.Default()
	reglages.Org, reglages.Assignment = "acme", "tp1"
	reglages.NamePattern = "{assignment}"

	reponse, contenu := h.requete(http.MethodPost, "/api/plan", map[string]any{
		"settings": reglages,
		"people":   []map[string]string{{"full_name": "Jean-Luc Picard", "username": "jlpicard"}},
	})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400", reponse.StatusCode)
	}
	if !strings.Contains(string(contenu), "{username}") {
		t.Fatalf("message peu explicite : %s", contenu)
	}
}

// ------------------------------------------------------------------ écriture

func TestCreationDesDepots(t *testing.T) {
	h := nouveau(t, nil)
	reglages := config.Default()
	reglages.Org, reglages.Assignment, reglages.DelaySeconds = "acme", "tp1", 0

	bilan := h.travail(http.MethodPost, "/api/create", map[string]any{
		"settings": reglages,
		"people": []map[string]string{
			{"full_name": "Jean-Luc Picard", "username": "jlpicard"},
			{"full_name": "Émilie Côté", "username": "emilie-cote"},
		},
	})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["created"] != float64(2) {
		t.Fatalf("%v dépôt(s) créé(s), attendu 2", resultat["created"])
	}
	noms := h.State.RepoNames("acme")
	if len(noms) != 2 {
		t.Fatalf("dépôts créés : %v", noms)
	}
}

func TestSimulationNeCreeRien(t *testing.T) {
	h := nouveau(t, nil)
	reglages := config.Default()
	reglages.Org, reglages.Assignment, reglages.DelaySeconds = "acme", "tp1", 0

	bilan := h.travail(http.MethodPost, "/api/create", map[string]any{
		"settings": reglages,
		"people":   []map[string]string{{"full_name": "Jean-Luc Picard", "username": "jlpicard"}},
		"dry_run":  true,
	})
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v", bilan["status"])
	}
	if noms := h.State.RepoNames("acme"); len(noms) != 0 {
		t.Fatalf("la simulation a créé %v", noms)
	}
}

func TestAjoutAuGroupeEcarteLesPersonnesDejaServies(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-jlpicard", true)
	h := nouveau(t, state)

	reglages := config.Default()
	reglages.Org, reglages.Assignment, reglages.DelaySeconds = "acme", "tp1", 0
	bilan := h.travail(http.MethodPost, "/api/create", map[string]any{
		"settings": reglages,
		"people": []map[string]string{
			{"full_name": "Jean-Luc Picard", "username": "jlpicard"},
			{"full_name": "Émilie Côté", "username": "emilie-cote"},
		},
		"group": "tp1",
	})
	resultat, _ := bilan["result"].(map[string]any)
	if resultat["created"] != float64(1) {
		t.Fatalf("%v dépôt(s) créé(s), attendu 1", resultat["created"])
	}
	ecartees, _ := resultat["skipped"].([]any)
	if len(ecartees) != 1 {
		t.Fatalf("%d personne(s) écartée(s), attendu 1", len(ecartees))
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
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-jlpicard", true)
	state.AddRepo("acme", "tp1-emilie-cote", true)
	h := nouveau(t, state)

	bilan := h.travail(http.MethodPost, "/api/orgs/acme/groups/tp1/names", nil)
	if bilan["status"] != "terminé" {
		t.Fatalf("travail %v : %v", bilan["status"], bilan["failure"])
	}
	noms, _ := bilan["result"].(map[string]any)
	if noms["tp1-jlpicard"] != "Jean-Luc Picard" {
		t.Fatalf("noms résolus : %v", noms)
	}

	// Une fois connus, ils accompagnent le groupe sans nouvel appel.
	var groupe struct {
		Repos []struct {
			Name     string `json:"name"`
			FullName string `json:"full_name"`
		} `json:"repos"`
		Missing int `json:"missing_names"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/groups/tp1", nil, &groupe)
	if groupe.Missing != 0 {
		t.Fatalf("%d nom(s) encore inconnu(s)", groupe.Missing)
	}
	if groupe.Repos[1].FullName != "Jean-Luc Picard" {
		t.Fatalf("groupe : %+v", groupe.Repos)
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

func TestListeDesEtudiantsCroiseeAvecLesTravaux(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{"tp1-jlpicard", "tp1-emilie-cote", "tp2-jlpicard", "tp2-emilie-cote"} {
		state.AddRepo("acme", nom, true)
	}
	h := nouveau(t, state)

	var reponse struct {
		Students []struct {
			FullName    string `json:"full_name"`
			Username    string `json:"username"`
			Assignments []struct {
				Prefix string `json:"prefix"`
				Repo   string `json:"repo"`
				URL    string `json:"url"`
			} `json:"assignments"`
		} `json:"students"`
	}
	h.json(http.MethodPost, "/api/orgs/acme/students", map[string]any{
		"people": []map[string]string{
			{"full_name": "Jean-Luc Picard", "username": "jlpicard"},
			{"full_name": "Aminata Diallo", "username": "aminata-d"},
		},
	}, &reponse)

	if len(reponse.Students) != 2 {
		t.Fatalf("%d étudiant(s), attendu 2", len(reponse.Students))
	}
	if len(reponse.Students[0].Assignments) != 2 {
		t.Fatalf("travaux de @jlpicard : %+v", reponse.Students[0].Assignments)
	}
	if reponse.Students[0].Assignments[0].Repo != "tp1-jlpicard" {
		t.Fatalf("dépôt inattendu : %+v", reponse.Students[0].Assignments[0])
	}
	// Une personne sans dépôt reste dans la liste, sans travail.
	if len(reponse.Students[1].Assignments) != 0 {
		t.Fatalf("@aminata-d n'a aucun dépôt : %+v", reponse.Students[1].Assignments)
	}
}
