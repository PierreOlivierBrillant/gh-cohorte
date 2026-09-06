package ghapi_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
)

// client monte un faux GitHub et le client qui l'interroge.
func client(t *testing.T, state *fakegh.State) (*ghapi.Client, *fakegh.Server) {
	t.Helper()
	if state == nil {
		state = fakegh.NewState()
	}
	serveur := fakegh.New(state)
	t.Cleanup(serveur.Close)

	c, err := ghapi.New(ghapi.Options{
		Host:    "127.0.0.1",
		Token:   "jeton-de-test",
		BaseURL: serveur.URL(),
		Sleep:   func(time.Duration) {}, // les tests n'attendent jamais
	})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	return c, serveur
}

func TestAuthenticatedUserEtOrg(t *testing.T) {
	c, _ := client(t, nil)
	utilisateur, err := c.AuthenticatedUser()
	if err != nil || utilisateur.Login != "prof" {
		t.Fatalf("AuthenticatedUser = %+v, %v", utilisateur, err)
	}
	org, err := c.GetOrg("acme")
	if err != nil || org.Name != "ACME Éducation" {
		t.Fatalf("GetOrg = %+v, %v", org, err)
	}
	if _, err := c.GetOrg("inconnue"); err == nil {
		t.Error("une organisation absente doit produire une erreur")
	} else if ghapi.StatusOf(err) != 404 {
		t.Errorf("statut = %d", ghapi.StatusOf(err))
	}
}

func TestPortéesDuJeton(t *testing.T) {
	c, _ := client(t, nil)
	if _, err := c.AuthenticatedUser(); err != nil {
		t.Fatal(err)
	}
	if présent, connu := c.HasScope("delete_repo"); !connu || !présent {
		t.Errorf("delete_repo = %v, %v", présent, connu)
	}
	if présent, connu := c.HasScope("admin:org"); !connu || présent {
		t.Errorf("admin:org = %v, %v", présent, connu)
	}
}

func TestPortéesInconnuesJetonFinegrained(t *testing.T) {
	state := fakegh.NewState()
	state.Scopes = "" // un jeton « fine-grained » n'annonce aucune portée
	c, _ := client(t, state)
	if _, err := c.AuthenticatedUser(); err != nil {
		t.Fatal(err)
	}
	if _, connu := c.HasScope("repo"); connu {
		t.Error("aucune portée annoncée : rien ne doit être affirmé")
	}
}

func TestUserExists(t *testing.T) {
	c, _ := client(t, nil)
	if existe, err := c.UserExists("jlpicard"); err != nil || !existe {
		t.Errorf("jlpicard : %v, %v", existe, err)
	}
	if existe, err := c.UserExists("fantome"); err != nil || existe {
		t.Errorf("fantome : %v, %v", existe, err)
	}
}

func TestCreateOrgRepoEtIdempotence(t *testing.T) {
	c, serveur := client(t, nil)
	repo, err := c.CreateOrgRepo("acme", "tp1-jlpicard", true, "TP1", false)
	if err != nil {
		t.Fatalf("CreateOrgRepo : %v", err)
	}
	if repo.Name != "tp1-jlpicard" || !repo.Private || repo.DefaultBranch != "main" {
		t.Fatalf("dépôt créé = %+v", repo)
	}
	if _, err := c.CreateOrgRepo("acme", "tp1-jlpicard", true, "TP1", false); err == nil {
		t.Error("recréer un dépôt existant doit échouer")
	}
	if len(serveur.State.RepoNames("acme")) != 1 {
		t.Errorf("dépôts = %v", serveur.State.RepoNames("acme"))
	}
	// auto_init=false : le dépôt reste vide, ce qui rend la reprise possible.
	if head, err := c.BranchHead("acme", "tp1-jlpicard", "main"); err != nil || head != "" {
		t.Errorf("BranchHead = %q, %v", head, err)
	}
}

func TestCreateOrgRepoAutoInit(t *testing.T) {
	c, _ := client(t, nil)
	if _, err := c.CreateOrgRepo("acme", "tp1-a", true, "", true); err != nil {
		t.Fatal(err)
	}
	head, err := c.BranchHead("acme", "tp1-a", "main")
	if err != nil || head == "" {
		t.Errorf("auto_init doit créer un premier commit : %q, %v", head, err)
	}
}

func TestGenerateFromTemplate(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "modele-tp", false)
	state.SeedCommit("acme/modele-tp", map[string]string{"README.md": "# modèle\n"}, "main")
	c, serveur := client(t, state)

	repo, err := c.GenerateFromTemplate("acme", "modele-tp", "acme", "tp1-jlpicard", true, "TP1", false)
	if err != nil {
		t.Fatalf("GenerateFromTemplate : %v", err)
	}
	if repo.Name != "tp1-jlpicard" {
		t.Fatalf("dépôt = %+v", repo)
	}
	if fichiers := serveur.State.Files("acme/tp1-jlpicard", "main"); fichiers["README.md"] != "# modèle\n" {
		t.Errorf("le contenu du modèle doit être recopié : %+v", fichiers)
	}
	// Le modèle d'origine se relit sur le dépôt créé.
	relu, err := c.GetRepo("acme", "tp1-jlpicard")
	if err != nil || relu.TemplateRepository == nil || relu.TemplateRepository.FullName != "acme/modele-tp" {
		t.Errorf("template_repository = %+v, %v", relu, err)
	}
}

func TestGenerateDepuisUnDepotNonModele(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "ordinaire", false)
	c, _ := client(t, state)
	if _, err := c.GenerateFromTemplate("acme", "ordinaire", "acme", "tp1-a", true, "", false); err == nil {
		t.Error("un dépôt non marqué « template » doit être refusé")
	}
}

func TestAddCollaborator(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-jlpicard", true)
	c, serveur := client(t, state)

	// 201 : invitation créée.
	if état, err := c.AddCollaborator("acme", "tp1-jlpicard", "jlpicard", "push"); err != nil ||
		état != ghapi.CollaboratorInvited {
		t.Fatalf("AddCollaborator = %q, %v", état, err)
	}
	invitations, err := c.ListInvitations("acme", "tp1-jlpicard")
	if err != nil || len(invitations) != 1 || invitations[0].Invitee.Login != "jlpicard" {
		t.Fatalf("invitations = %+v, %v", invitations, err)
	}

	// 204 : la personne est déjà membre de l'organisation.
	if état, err := c.AddCollaborator("acme", "tp1-jlpicard", "prof", "admin"); err != nil ||
		état != ghapi.CollaboratorAdded {
		t.Fatalf("AddCollaborator(prof) = %q, %v", état, err)
	}
	serveur.State.AcceptInvitations("acme/tp1-jlpicard")
	collaborateurs, err := c.ListCollaborators("acme", "tp1-jlpicard")
	if err != nil {
		t.Fatal(err)
	}
	if len(collaborateurs) != 2 {
		t.Fatalf("collaborateurs = %+v", collaborateurs)
	}
	for _, item := range collaborateurs {
		if item.Login == "admin-org" {
			t.Error("affiliation=direct doit exclure les administrateurs de l'organisation")
		}
	}
}

func TestRetraitEtAnnulation(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-a", true)
	c, _ := client(t, state)

	if _, err := c.AddCollaborator("acme", "tp1-a", "jlpicard", "push"); err != nil {
		t.Fatal(err)
	}
	invitations, _ := c.ListInvitations("acme", "tp1-a")
	if err := c.CancelInvitation("acme", "tp1-a", invitations[0].ID); err != nil {
		t.Fatalf("CancelInvitation : %v", err)
	}
	if restantes, _ := c.ListInvitations("acme", "tp1-a"); len(restantes) != 0 {
		t.Errorf("invitations restantes = %+v", restantes)
	}
	// Retirer un collaborateur absent est sans effet et sans erreur.
	if err := c.RemoveCollaborator("acme", "tp1-a", "emilie-cote"); err != nil {
		t.Errorf("RemoveCollaborator : %v", err)
	}
}

func TestDeleteRepo(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-a", true)
	c, serveur := client(t, state)
	if err := c.DeleteRepo("acme", "tp1-a"); err != nil {
		t.Fatalf("DeleteRepo : %v", err)
	}
	if len(serveur.State.Deleted) != 1 || serveur.State.Deleted[0] != "acme/tp1-a" {
		t.Errorf("supprimés = %+v", serveur.State.Deleted)
	}
	if err := c.DeleteRepo("acme", "tp1-a"); err == nil {
		t.Error("supprimer deux fois doit échouer")
	}
}

func TestErreurPortéeDeleteRepo(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-a", true)
	state.FailOn["DELETE /repos/acme/tp1-a"] = fakegh.Failure{
		Status:  403,
		Message: "Must have admin rights to Repository. (delete_repo scope)",
	}
	c, _ := client(t, state)
	err := c.DeleteRepo("acme", "tp1-a")
	if err == nil {
		t.Fatal("une erreur était attendue")
	}
	if !strings.Contains(err.Error(), "gh auth refresh -s delete_repo") {
		t.Errorf("le message doit indiquer la correction : %v", err)
	}
}

func TestErreurJetonInvalide(t *testing.T) {
	state := fakegh.NewState()
	state.FailOn["GET /user"] = fakegh.Failure{Status: 401, Message: "Bad credentials"}
	c, _ := client(t, state)
	_, err := c.AuthenticatedUser()
	if err == nil || !strings.Contains(err.Error(), "Jeton invalide") {
		t.Errorf("erreur = %v", err)
	}
}

func TestPaginationDesDepots(t *testing.T) {
	state := fakegh.NewState()
	state.PerPage = 7 // force plusieurs pages
	for index := 0; index < 25; index++ {
		state.AddRepo("acme", "tp1-"+string(rune('a'+index)), true)
	}
	c, _ := client(t, state)

	var progression []int
	depots, err := c.ListOrgRepos("acme", func(total int) { progression = append(progression, total) })
	if err != nil {
		t.Fatalf("ListOrgRepos : %v", err)
	}
	if len(depots) != 25 {
		t.Fatalf("%d dépôt(s) listé(s)", len(depots))
	}
	if len(progression) != 4 {
		t.Errorf("pages parcourues = %v", progression)
	}
}

// noms rend les noms des dépôts dans l'ordre où ils ont été listés.
func noms(depots []groups.RepoInfo) []string {
	liste := make([]string, 0, len(depots))
	for _, depot := range depots {
		liste = append(liste, depot.Name)
	}
	return liste
}

// attendus compose les noms que le faux serveur rend, dans son ordre à lui.
func attendus(nombre int) []string {
	liste := make([]string, 0, nombre)
	for index := 0; index < nombre; index++ {
		liste = append(liste, fmt.Sprintf("tp1-%03d", index))
	}
	return liste
}

// peuple ajoute des dépôts nommés de façon à ce que l'ordre alphabétique du
// faux serveur soit aussi leur ordre de création.
func peuple(state *fakegh.State, nombre int) {
	for index := 0; index < nombre; index++ {
		state.AddRepo("acme", fmt.Sprintf("tp1-%03d", index), true)
	}
}

// Les pages se chargent de front, mais rien ne doit sortir de l'ordre : c'est
// ce qui permet à l'appelant d'accumuler sans verrou, et ce qui garde le même
// ordre de départ d'une exécution à l'autre.
func TestPaginationParalleleGardeLOrdre(t *testing.T) {
	state := fakegh.NewState()
	state.PerPage = 10 // 25 pages, soit plusieurs volées
	peuple(state, 250)
	c, serveur := client(t, state)

	depots, err := c.ListOrgRepos("acme", nil)
	if err != nil {
		t.Fatalf("ListOrgRepos : %v", err)
	}
	if obtenu, voulu := noms(depots), attendus(250); !slices.Equal(obtenu, voulu) {
		t.Fatalf("ordre rompu : %v", premiereDifference(obtenu, voulu))
	}
	// Chaque page une fois, jamais deux : la première en série, les autres de front.
	if appels := serveur.State.CallCount("GET /orgs/acme/repos"); appels != 25 {
		t.Errorf("%d requête(s) de page, 25 attendues", appels)
	}
}

// Compter les requêtes ne dit pas si elles ont été menées de front. La barrière
// le dit : elle retient chaque page jusqu'à ce qu'un certain nombre soit en
// vol, ce qui ne peut arriver que si le client en a plusieurs ensemble.
func TestPaginationChargeLesPagesDeFront(t *testing.T) {
	const front = 4

	state := fakegh.NewState()
	state.PerPage = 10 // 20 pages, largement plus qu'une volée
	peuple(state, 200)

	barriere := nouvelleBarriere(front, 2*time.Second)
	state.Hook = func(request *http.Request) {
		// La première page est chargée seule, avant qu'on sache combien il y en
		// a : la retenir bloquerait tout le reste.
		if !strings.Contains(request.URL.Path, "/orgs/acme/repos") ||
			request.URL.Query().Get("page") == "1" {
			return
		}
		barriere.attendre()
	}

	c, _ := client(t, state)
	if _, err := c.ListOrgRepos("acme", nil); err != nil {
		t.Fatalf("ListOrgRepos : %v", err)
	}
	if !barriere.atteinte() {
		t.Errorf("jamais %d pages en vol ensemble : elles sont restées en série", front)
	}
}

// barriere retient les requêtes jusqu'à ce que « seuil » d'entre elles soient
// en vol. Le délai la libère si le compte n'est jamais atteint : un client
// resté en série doit échouer sur une assertion, pas se figer.
type barriere struct {
	seuil  int
	delai  time.Duration
	ouvrir sync.Once
	ouvert chan struct{}

	mutex   sync.Mutex
	envol   int
	franchi bool
}

func nouvelleBarriere(seuil int, delai time.Duration) *barriere {
	return &barriere{seuil: seuil, delai: delai, ouvert: make(chan struct{})}
}

func (b *barriere) attendre() {
	b.mutex.Lock()
	b.envol++
	assez := b.envol >= b.seuil
	if assez {
		b.franchi = true
	}
	b.mutex.Unlock()

	if assez {
		b.ouvrir.Do(func() { close(b.ouvert) })
		return
	}
	select {
	case <-b.ouvert:
	case <-time.After(b.delai):
		// Une seule attente perdue : la barrière s'ouvre pour toutes les autres.
		b.ouvrir.Do(func() { close(b.ouvert) })
	}
}

func (b *barriere) atteinte() bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.franchi
}

// Un point d'API qui n'annonce pas de dernière page — pagination par curseur,
// instance Enterprise — doit rester parcouru page après page.
func TestPaginationSansDernierePageSuitNext(t *testing.T) {
	state := fakegh.NewState()
	state.PerPage = 10
	state.NoLastLink = true
	peuple(state, 95)
	c, serveur := client(t, state)

	var progression []int
	depots, err := c.ListOrgRepos("acme", func(total int) {
		progression = append(progression, total)
	})
	if err != nil {
		t.Fatalf("ListOrgRepos : %v", err)
	}
	if obtenu, voulu := noms(depots), attendus(95); !slices.Equal(obtenu, voulu) {
		t.Fatalf("ordre rompu : %v", premiereDifference(obtenu, voulu))
	}
	if len(progression) != 10 {
		t.Errorf("pages parcourues = %v", progression)
	}
	if appels := serveur.State.CallCount("GET /orgs/acme/repos"); appels != 10 {
		t.Errorf("%d requête(s) de page, 10 attendues", appels)
	}
}

// premiereDifference dit où deux listes divergent, plutôt que de les afficher
// en entier : deux cent cinquante noms ne se lisent pas dans un message d'échec.
func premiereDifference(obtenu, voulu []string) string {
	if len(obtenu) != len(voulu) {
		return fmt.Sprintf("%d élément(s) au lieu de %d", len(obtenu), len(voulu))
	}
	for index := range obtenu {
		if obtenu[index] != voulu[index] {
			return fmt.Sprintf("rang %d : « %s » au lieu de « %s »",
				index, obtenu[index], voulu[index])
		}
	}
	return "aucune"
}

func TestPushFilesSurDepotVide(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-a", true)
	c, serveur := client(t, state)

	fichiers := []ghapi.PushFile{
		{Path: "README.md", Mode: "100644", Content: []byte("# TP1\n")},
		{Path: "src/main.py", Mode: "100644", Content: []byte("print('bonjour')\n")},
	}
	nombre, err := c.PushFiles("acme", "tp1-a", fichiers, "Fichiers de départ", "main")
	if err != nil || nombre != 2 {
		t.Fatalf("PushFiles = %d, %v", nombre, err)
	}
	contenu := serveur.State.Files("acme/tp1-a", "main")
	if contenu["README.md"] != "# TP1\n" || contenu["src/main.py"] != "print('bonjour')\n" {
		t.Fatalf("contenu = %+v", contenu)
	}
	// Un seul commit : blobs + arbre + commit + référence.
	if créés := serveur.State.CallCount("POST /repos/acme/tp1-a/git/commits"); créés != 1 {
		t.Errorf("%d commit(s) créé(s)", créés)
	}
}

func TestPushFilesConserveLExistant(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-a", true)
	state.SeedCommit("acme/tp1-a", map[string]string{
		"README.md":    "# rendu de l'étudiant\n",
		"src/rendu.py": "print('mon travail')\n",
	}, "main")
	c, serveur := client(t, state)

	fichiers := []ghapi.PushFile{{Path: "CONSIGNES.md", Mode: "100644", Content: []byte("À faire\n")}}
	if _, err := c.PushFiles("acme", "tp1-a", fichiers, "Consignes", "main"); err != nil {
		t.Fatalf("PushFiles : %v", err)
	}
	contenu := serveur.State.Files("acme/tp1-a", "main")
	if contenu["src/rendu.py"] != "print('mon travail')\n" {
		t.Error("base_tree doit conserver les fichiers déjà présents")
	}
	if contenu["CONSIGNES.md"] != "À faire\n" {
		t.Error("le nouveau fichier doit être ajouté")
	}
}

func TestReessaiSurErreurPassagere(t *testing.T) {
	state := fakegh.NewState()
	state.Flaky["POST /orgs/acme/repos"] = 2 // deux 500 avant de réussir
	c, serveur := client(t, state)

	if _, err := c.CreateOrgRepo("acme", "tp1-a", true, "", false); err != nil {
		t.Fatalf("le client doit retenter : %v", err)
	}
	if appels := serveur.State.CallCount("POST /orgs/acme/repos"); appels != 3 {
		t.Errorf("%d tentative(s), 3 attendues", appels)
	}
}

func TestReessaiAbandonneApresQuatreTentatives(t *testing.T) {
	state := fakegh.NewState()
	state.Flaky["POST /orgs/acme/repos"] = 10
	c, serveur := client(t, state)

	if _, err := c.CreateOrgRepo("acme", "tp1-a", true, "", false); err == nil {
		t.Fatal("une panne persistante doit finir en erreur")
	}
	if appels := serveur.State.CallCount("POST /orgs/acme/repos"); appels != 4 {
		t.Errorf("%d tentative(s), 4 attendues", appels)
	}
}

func TestReessaiSurLimiteSecondaire(t *testing.T) {
	var appels int
	serveur := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		appels++
		if appels == 1 {
			writer.Header().Set("Retry-After", "1")
			writer.WriteHeader(403)
			_, _ = writer.Write([]byte(`{"message":"You have exceeded a secondary rate limit"}`))
			return
		}
		writer.WriteHeader(200)
		_, _ = writer.Write([]byte(`{"login":"prof"}`))
	}))
	defer serveur.Close()

	var attentes []time.Duration
	c, err := ghapi.New(ghapi.Options{
		Host: "127.0.0.1", Token: "jeton", BaseURL: serveur.URL,
		Sleep: func(delay time.Duration) { attentes = append(attentes, delay) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AuthenticatedUser(); err != nil {
		t.Fatalf("la limite secondaire doit être absorbée : %v", err)
	}
	if len(attentes) != 1 || attentes[0] != time.Second {
		t.Errorf("attentes = %v, attendu [1s]", attentes)
	}
}

func TestJetonTransmisEtJamaisDivulgue(t *testing.T) {
	var entetes http.Header
	serveur := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		entetes = request.Header.Clone()
		writer.WriteHeader(200)
		_, _ = writer.Write([]byte(`{"login":"prof"}`))
	}))
	defer serveur.Close()

	c, err := ghapi.New(ghapi.Options{Host: "127.0.0.1", Token: "jeton-secret", BaseURL: serveur.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AuthenticatedUser(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(entetes.Get("Authorization"), "jeton-secret") {
		t.Errorf("le jeton doit être transmis par en-tête : %q", entetes.Get("Authorization"))
	}
	// Une erreur ne doit jamais reproduire le jeton.
	state := fakegh.NewState()
	state.FailOn["GET /user"] = fakegh.Failure{Status: 500, Message: "boum"}
	autre, _ := client(t, state)
	if _, err := autre.AuthenticatedUser(); err != nil && strings.Contains(err.Error(), "jeton-de-test") {
		t.Errorf("le jeton apparaît dans l'erreur : %v", err)
	}
}

func TestOrgMembership(t *testing.T) {
	state := fakegh.NewState()
	state.MembershipRole = "member"
	c, _ := client(t, state)
	rôle, err := c.OrgMembership("acme", "prof")
	if err != nil || rôle != "member" {
		t.Fatalf("OrgMembership = %q, %v", rôle, err)
	}

	sansRôle := fakegh.NewState()
	sansRôle.MembershipRole = "" // portée read:org absente
	autre, _ := client(t, sansRôle)
	if rôle, err := autre.OrgMembership("acme", "prof"); err != nil || rôle != "" {
		t.Errorf("OrgMembership = %q, %v", rôle, err)
	}
}
