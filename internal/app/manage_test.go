package app_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/students"
)

// groupe prépare une organisation contenant un groupe de dépôts déjà créés.
func groupe(t *testing.T) *fakegh.State {
	t.Helper()
	state := fakegh.NewState()
	for _, nom := range []string{"tp1-emilie-cote", "tp1-jlpicard", "tp1-aminata-d"} {
		depot := state.AddRepo("acme", nom, true)
		depot.PushedAt = "2026-08-21T09:00:00Z"
		depot.Template = "acme/modele-tp"
	}
	state.AddRepo("acme", "projet-final-emilie-cote", true)
	state.AddRepo("acme", "projet-final-jlpicard", true)
	state.AddRepo("acme", "notes-du-cours", false)
	state.AddRepo("acme", "modele-tp", false)
	return state
}

func gestion(t *testing.T, state *fakegh.State, prefixe string) *harnais {
	t.Helper()
	h := nouveau(t, state)
	h.Options.ManageRequested = true
	h.Options.Manage = prefixe
	return h
}

func TestGestionAfficheLeGroupe(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	code, scripte := h.script("quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if scripte.Remaining() != 0 {
		t.Errorf("%d réponse(s) inutilisée(s)", scripte.Remaining())
	}
	h.contient("Groupe « tp1 » — 3 dépôt(s)", "tp1-emilie-cote", "Émilie Côté",
		"Jean-Luc Picard", "privé", "2026-08-21")
	// Le suffixe tient lieu de nom quand le profil GitHub n'en porte aucun.
	h.contient("aminata-d")
}

// La liste du terminal se filtre et se trie comme celle du navigateur : ce sont
// les mêmes critères, décidés au même endroit.
func TestGestionListeFiltreeEtTriee(t *testing.T) {
	state := fakegh.NewState()
	envois := map[string]string{
		"tp1-emilie-cote": "2026-09-20T09:00:00Z",
		"tp1-jlpicard":    "2026-10-15T09:00:00Z",
		"tp1-aminata-d":   "",
	}
	for nom, envoi := range envois {
		state.AddRepo("acme", nom, true).PushedAt = envoi
	}

	h := gestion(t, state, "tp1")
	h.Options.Filter = students.Filter{PushedAfter: "2026-10-01"}
	if code, _ := h.script("quitter"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("1 affiché(s)", "tp1-jlpicard", "envoi après le 2026-10-01")
	h.absent("tp1-emilie-cote")

	// Les dépôts sans aucun envoi se retrouvent à part : une borne ne les
	// range ni avant ni après.
	muet := gestion(t, state, "tp1")
	muet.Options.Filter = students.Filter{Activity: students.Silent}
	if code, _ := muet.script("quitter"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, muet.texte())
	}
	muet.contient("tp1-aminata-d", "aucun envoi")
	muet.absent("tp1-jlpicard")
}

// Le menu du terminal offre les mêmes critères que la barre du navigateur, et
// la sélection suit ce que la liste montre.
func TestGestionFiltreDepuisLeMenu(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	code, scripte := h.script(
		"filtrer", "chercher", "picard", "retour", // ne garder que Picard
		"urls", "", "n", // les URL ne portent plus que sur lui
		"quitter",
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if scripte.Remaining() != 0 {
		t.Errorf("%d réponse(s) inutilisée(s)", scripte.Remaining())
	}
	h.contient("1 affiché(s)", "« picard »", "https://github.com/acme/tp1-jlpicard")
	h.absent("https://github.com/acme/tp1-emilie-cote")
}

func TestGestionDetecteLesGroupes(t *testing.T) {
	h := gestion(t, groupe(t), "")
	code, _ := h.script("1", "quitter") // premier groupe détecté
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Groupes existants", "tp1", "3 dépôt(s)", "projet-final", "2 dépôt(s)")
	h.absent("notes-du-cours")
}

func TestGestionPrefixeSaisi(t *testing.T) {
	h := gestion(t, groupe(t), "")
	code, _ := h.script("saisir", "projet-final", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Groupe « projet-final » — 2 dépôt(s)")
}

func TestGestionPrefixeInexistant(t *testing.T) {
	h := gestion(t, groupe(t), "tp9")
	code, _ := h.script("revenir")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Aucun dépôt ne commence par « tp9- »")
}

func TestGestionChangerDeGroupe(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	code, _ := h.script(
		"changer",              // quitter le groupe courant
		"prefixe:projet-final", // en choisir un autre
		"quitter",
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Groupe « tp1 »", "Groupe « projet-final »")
}

func TestGestionAffichageDesAcces(t *testing.T) {
	state := groupe(t)
	state.Collaborators["acme/tp1-emilie-cote"] = map[string]string{"emilie-cote": "push"}
	state.Invitations["acme/tp1-jlpicard"] = nil
	h := gestion(t, state, "tp1")

	// Une invitation en attente est créée par l'ajout d'un collaborateur.
	code, _ := h.script(
		"collaborateurs", "tp1-jlpicard", "ajouter", "jlpicard", "push", "revenir",
		"acces",
		"quitter",
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("invitation envoyée", "Accès des dépôts", "emilie-cote", "jlpicard (invité)")
	// Sans « affiliation=direct », les administrateurs de l'organisation
	// pollueraient la liste : ils ne doivent jamais apparaître.
	h.absent("admin-org")
}

func TestGestionRetraitDUnCollaborateur(t *testing.T) {
	state := groupe(t)
	state.Collaborators["acme/tp1-emilie-cote"] = map[string]string{"emilie-cote": "push"}
	h := gestion(t, state, "tp1")

	code, _ := h.script(
		"collaborateurs", "tp1-emilie-cote",
		"retirer", "collaborateur:emilie-cote", "oui",
		"revenir",
		"quitter",
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("n'a plus accès")
	if reste := state.Collaborators["acme/tp1-emilie-cote"]; len(reste) != 0 {
		t.Errorf("collaborateurs restants = %+v", reste)
	}
}

func TestGestionAnnulationDUneInvitation(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	code, _ := h.script(
		"collaborateurs", "tp1-jlpicard",
		"ajouter", "jlpicard", "push",
		"retirer", "1", "oui", // seule l'invitation en attente figure dans la liste
		"revenir",
		"quitter",
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Invitation annulée")
	if invitations := h.State.Invitations["acme/tp1-jlpicard"]; len(invitations) != 0 {
		t.Errorf("invitations = %+v", invitations)
	}
}

func TestGestionCollaborateurInexistant(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	code, _ := h.script(
		"collaborateurs", "tp1-jlpicard", "ajouter", "fantome",
		"revenir", "quitter",
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("n'existe pas sur GitHub")
}

func TestGestionAjoutDeDepots(t *testing.T) {
	state := groupe(t)
	h := gestion(t, state, "tp1")
	csv := h.cohorteCSV(
		"Émilie Côté,emilie-cote", // déjà servie
		"Professeure Adjointe,prof",
	)

	code, _ := h.script(
		"ajouter",
		"",        // gabarit de nom, défaut
		"fichier", // source de la liste
		csv,       // chemin
		"oui",     // vérifier les comptes
		"oui",     // confirmer la création
		"quitter",
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Modèle réutilisé : acme/modele-tp", "1 personne(s) déjà présente(s)", "1 créé(s)")
	if _, existe := state.Repos["acme/tp1-prof"]; !existe {
		t.Fatalf("dépôts = %v", state.RepoNames("acme"))
	}
	if state.Repos["acme/tp1-prof"].Template != "acme/modele-tp" {
		t.Errorf("le modèle détecté doit être réutilisé : %+v", state.Repos["acme/tp1-prof"])
	}
}

func TestGestionAjoutSansPersonneNouvelle(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	csv := h.cohorteCSV("Émilie Côté,emilie-cote", "Jean-Luc Picard,jlpicard")

	code, _ := h.script("ajouter", "", "fichier", csv, "oui", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Rien à ajouter")
}

func TestGestionURLs(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	cible := filepath.Join(t.TempDir(), "urls.csv")

	code, _ := h.script("urls", "1,2", "oui", cible, "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("https://github.com/acme/tp1-aminata-d", "https://github.com/acme/tp1-emilie-cote")
	contenu, err := os.ReadFile(cible)
	if err != nil {
		t.Fatalf("CSV non écrit : %v", err)
	}
	if !strings.Contains(string(contenu), "nom_complet,depot,url") ||
		!strings.Contains(string(contenu), "Émilie Côté") {
		t.Errorf("CSV = %s", contenu)
	}
	if strings.Count(string(contenu), "\n") != 3 {
		t.Errorf("le CSV doit porter deux dépôts : %s", contenu)
	}
}

func TestGestionSuppressionAnnuleeSiLeNomNeCorrespondPas(t *testing.T) {
	state := groupe(t)
	h := gestion(t, state, "tp1")

	code, _ := h.script("supprimer", "tp1-jlpicard", "tp1-jlpicar", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Annulé : rien n'a été supprimé")
	if _, existe := state.Repos["acme/tp1-jlpicard"]; !existe {
		t.Fatal("le dépôt a été supprimé malgré une confirmation incorrecte")
	}
	if len(state.Deleted) != 0 {
		t.Errorf("suppressions = %v", state.Deleted)
	}
}

func TestGestionSuppressionConfirmee(t *testing.T) {
	state := groupe(t)
	h := gestion(t, state, "tp1")

	code, _ := h.script("supprimer", "tp1-jlpicard", "tp1-jlpicard", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("« tp1-jlpicard » supprimé")
	if _, existe := state.Repos["acme/tp1-jlpicard"]; existe {
		t.Fatal("le dépôt aurait dû être supprimé")
	}
	// La liste rechargée ne montre plus que deux dépôts.
	h.contient("Groupe « tp1 » — 2 dépôt(s)")
}

func TestGestionSuppressionPorteeManquante(t *testing.T) {
	state := groupe(t)
	state.Scopes = "repo, read:org"
	state.FailOn["DELETE /repos/acme/tp1-jlpicard"] = fakegh.Failure{
		Status: 403, Message: "Must have admin rights to Repository. (delete_repo scope)",
	}
	h := gestion(t, state, "tp1")

	code, _ := h.script("supprimer", "tp1-jlpicard", "tp1-jlpicard", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("gh auth refresh -s delete_repo", "Suppression impossible")
	if _, existe := state.Repos["acme/tp1-jlpicard"]; !existe {
		t.Error("rien ne doit être supprimé")
	}
}

func TestGestionCacheEvitelesAppels(t *testing.T) {
	state := groupe(t)
	h := gestion(t, state, "tp1")
	if code, _ := h.script("quitter"); code != app.ExitOK {
		t.Fatalf("code = %d", code)
	}
	premier := state.CallCount("GET /orgs/acme/repos")
	if premier == 0 {
		t.Fatal("la première session doit interroger l'API")
	}

	// Une seconde session lit le cache disque : plus aucun appel d'inventaire.
	suivant := gestion(t, state, "tp1")
	suivant.Options.CacheDir = h.CacheDir
	if code, _ := suivant.script("quitter"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	if state.CallCount("GET /orgs/acme/repos") != premier {
		t.Errorf("le cache n'a pas été utilisé : %d appel(s)", state.CallCount("GET /orgs/acme/repos"))
	}
	suivant.contient("cache :")
	// Les noms complets aussi sont mis en cache : aucun profil relu.
	if profils := state.CallCount("GET /users/"); profils != 3 {
		t.Errorf("%d appel(s) de profil, 3 attendus", profils)
	}
}

func TestGestionRechargementForce(t *testing.T) {
	state := groupe(t)
	h := gestion(t, state, "tp1")
	if code, _ := h.script("rafraichir", "quitter"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if appels := state.CallCount("GET /orgs/acme/repos"); appels < 2 {
		t.Errorf("%d appel(s) : le rechargement doit forcer un nouvel inventaire", appels)
	}
}

func TestGestionSansCache(t *testing.T) {
	state := groupe(t)
	h := gestion(t, state, "tp1")
	h.Options.NoCache = true
	if code, _ := h.script("quitter"); code != app.ExitOK {
		t.Fatalf("code = %d", code)
	}
	suivant := gestion(t, state, "tp1")
	suivant.Options.CacheDir = h.CacheDir
	suivant.Options.NoCache = true
	if code, _ := suivant.script("quitter"); code != app.ExitOK {
		t.Fatalf("code = %d", code)
	}
	if appels := state.CallCount("GET /orgs/acme/repos"); appels != 2 {
		t.Errorf("%d appel(s) : --no-cache doit tout redemander", appels)
	}
}

func TestGestionModeScriptExigeUnPrefixe(t *testing.T) {
	h := gestion(t, groupe(t), "")
	if code := h.muet(); code != app.ExitValidation {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("--manage")
}

// ------------------------------------------------------------------ clonage

// depotGit crée un vrai dépôt git local, qui servira d'origine au clonage.
func depotGit(t *testing.T, racine, nom string) string {
	t.Helper()
	chemin := filepath.Join(racine, nom)
	if err := os.MkdirAll(chemin, 0o755); err != nil {
		t.Fatal(err)
	}
	lancer := func(args ...string) {
		complet := append([]string{
			"-c", "user.name=Test", "-c", "user.email=test@example.invalid",
			"-c", "init.defaultBranch=main",
		}, args...)
		command := exec.Command("git", complet...)
		command.Dir = chemin
		command.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL="+filepath.Join(racine, "gitconfig-absent"),
			"GIT_CONFIG_SYSTEM="+filepath.Join(racine, "gitconfig-absent"))
		if sortie, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v : %v\n%s", args, err, sortie)
		}
	}
	lancer("init", "--quiet")
	if err := os.WriteFile(filepath.Join(chemin, "README.md"), []byte("# "+nom+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lancer("add", ".")
	lancer("commit", "--quiet", "-m", "Premier commit")
	return "file://" + filepath.ToSlash(chemin)
}

func TestGestionClonage(t *testing.T) {
	state := fakegh.NewState()
	origines := t.TempDir()
	for _, nom := range []string{"tp1-emilie-cote", "tp1-jlpicard"} {
		depot := state.AddRepo("acme", nom, true)
		depot.URLOverride = depotGit(t, origines, nom)
	}
	h := gestion(t, state, "tp1")
	destination := filepath.Join(t.TempDir(), "clones")

	code, _ := h.script("cloner", "tous", destination, "oui", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("2 cloné(s)")
	for _, nom := range []string{"tp1-emilie-cote", "tp1-jlpicard"} {
		if _, err := os.Stat(filepath.Join(destination, nom, "README.md")); err != nil {
			t.Errorf("%s non cloné : %v", nom, err)
		}
	}
	// Aucun secret ne doit se retrouver dans la configuration des clones.
	config, err := os.ReadFile(filepath.Join(destination, "tp1-jlpicard", ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(config)), "extraheader") ||
		strings.Contains(string(config), "jeton") {
		t.Errorf(".git/config = %s", config)
	}
}

func TestGestionClonageSelectionPartielle(t *testing.T) {
	state := fakegh.NewState()
	origines := t.TempDir()
	for _, nom := range []string{"tp1-a", "tp1-b", "tp1-c"} {
		depot := state.AddRepo("acme", nom, true)
		depot.URLOverride = depotGit(t, origines, nom)
	}
	h := gestion(t, state, "tp1")
	destination := filepath.Join(t.TempDir(), "clones")

	code, _ := h.script("cloner", "1,3", destination, "oui", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	entrées, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(entrées) != 2 {
		t.Fatalf("%d dossier(s) cloné(s)", len(entrées))
	}
	if entrées[0].Name() != "tp1-a" || entrées[1].Name() != "tp1-c" {
		t.Errorf("clones = %v, %v", entrées[0].Name(), entrées[1].Name())
	}
}

func TestGestionMiseAJourDesClones(t *testing.T) {
	state := fakegh.NewState()
	origines := t.TempDir()
	depot := state.AddRepo("acme", "tp1-a", true)
	depot.URLOverride = depotGit(t, origines, "tp1-a")
	h := gestion(t, state, "tp1")
	destination := filepath.Join(t.TempDir(), "clones")

	code, _ := h.script(
		"cloner", "tous", destination, "oui",
		"pull", destination, "tous", "oui",
		"quitter",
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("1 cloné(s)", "1 mis à jour")
}

func TestGestionClonageAnnule(t *testing.T) {
	state := groupe(t)
	h := gestion(t, state, "tp1")
	code, _ := h.script("cloner", "tous", "-", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Annulé")
}

func TestGestionMiseAJourDossierSansClone(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	vide := t.TempDir()
	code, _ := h.script("pull", vide, "-", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Aucun dépôt git directement sous")
}

func TestGestionAjoutSansModeleProposeUnDossierDeDepart(t *testing.T) {
	state := fakegh.NewState()
	// Aucun dépôt du groupe ne vient d'un modèle.
	state.AddRepo("acme", "tp1-emilie-cote", true)
	state.AddRepo("acme", "tp1-aminata-d", true)
	h := gestion(t, state, "tp1")
	csv := h.cohorteCSV("Jean-Luc Picard,jlpicard")
	depart := h.squelette()

	code, _ := h.script(
		"ajouter",
		"",        // gabarit par défaut
		depart,    // dossier de fichiers de départ
		"fichier", // source de la liste
		csv,
		"oui", // vérifier les comptes
		"oui", // confirmer
		"quitter",
	)
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Aucun dépôt modèle détecté", "1 créé(s)")
	if fichiers := state.Files("acme/tp1-jlpicard", "main"); len(fichiers) != 2 {
		t.Errorf("les fichiers de départ devaient être déposés : %+v", fichiers)
	}
}

func TestGestionURLsSelectionVide(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	code, _ := h.script("urls", "-", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Aucun dépôt sélectionné")
}

func TestGestionSelectionInvalideSignalee(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	// Une sélection hors liste est signalée, sans quitter le mode gestion.
	code, _ := h.script("urls", "42", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Sélection : « 42 » sort de la liste (1 à 3)")
}

func TestCacheHeriteDeClassroomEvitelesAppels(t *testing.T) {
	state := groupe(t)
	h := gestion(t, state, "tp1")

	// Un cache laissé par la version Python de l'outil, au format d'origine.
	ancien := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "classroom")
	if err := os.MkdirAll(ancien, 0o700); err != nil {
		t.Fatal(err)
	}
	contenu := fmt.Sprintf(`{
      "repos:acme": {"at": %d, "value": [
        {"name": "tp1-emilie-cote", "private": true, "html_url": "https://github.com/acme/tp1-emilie-cote", "pushed_at": "2026-08-21T09:00:00Z"},
        {"name": "tp1-jlpicard", "private": true, "html_url": "https://github.com/acme/tp1-jlpicard", "pushed_at": "2026-08-20T09:00:00Z"}
      ]},
      "profile:emilie-cote": {"at": %d, "value": "Émilie Côté"},
      "profile:jlpicard": {"at": %d, "value": "Jean-Luc Picard"}
    }`, time.Now().Unix(), time.Now().Unix(), time.Now().Unix())
	if err := os.WriteFile(filepath.Join(ancien, "cache.json"), []byte(contenu), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _ := h.script("quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("3 entrée(s) reprises du cache de « classroom »", "Émilie Côté", "Jean-Luc Picard")
	if appels := state.CallCount("GET /orgs/acme/repos"); appels != 0 {
		t.Errorf("%d inventaire(s) demandé(s) : le cache hérité devait suffire", appels)
	}
	if profils := state.CallCount("GET /users/"); profils != 0 {
		t.Errorf("%d profil(s) demandé(s) : les noms hérités devaient suffire", profils)
	}
	// Le groupe affiché vient bien du cache repris.
	h.contient("Groupe « tp1 » — 2 dépôt(s)")
}

func TestCachePurgeNeRevientPas(t *testing.T) {
	state := groupe(t)
	h := gestion(t, state, "tp1")
	ancien := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "classroom")
	if err := os.MkdirAll(ancien, 0o700); err != nil {
		t.Fatal(err)
	}
	contenu := fmt.Sprintf(`{"profile:jlpicard": {"at": %d, "value": "Jean-Luc Picard"}}`, time.Now().Unix())
	if err := os.WriteFile(filepath.Join(ancien, "cache.json"), []byte(contenu), 0o600); err != nil {
		t.Fatal(err)
	}

	if code, _ := h.script("quitter"); code != app.ExitOK {
		t.Fatalf("code = %d", code)
	}
	h.contient("reprises du cache")

	// Après une purge, l'ancien cache ne doit pas se réinviter.
	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.ClearCache = true
	if code, _ := suivant.script(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	dernier := nouveauDansLeMemeDossier(t, h)
	if code, _ := dernier.script("quitter"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, dernier.texte())
	}
	dernier.absent("reprises du cache")
}

// ------------------------------------------------ déplacer un travail entier

// L'assistant ne tient pas de liste d'étudiants : le dernier niveau du nom est
// conservé tel quel. Le travail arrive quand même à sa place.
func TestTravailDeplaceVersUneAutrePlace(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	code, _ := h.script("deplacer", "a26.5n6.01", "tp1", "oui", "revenir")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	noms := h.State.RepoNames("acme")
	sort.Strings(noms)
	for _, attendu := range []string{
		"a26.5n6.01.tp1.aminata-d", "a26.5n6.01.tp1.emilie-cote",
		"a26.5n6.01.tp1.jlpicard",
	} {
		if !slices.Contains(noms, attendu) {
			t.Fatalf("dépôts : %v", noms)
		}
	}
	// Les dépôts qui n'étaient pas du voyage n'ont pas bougé.
	if !slices.Contains(noms, "projet-final-jlpicard") {
		t.Fatalf("dépôts : %v", noms)
	}
}

// Refuser en fin de course ne laisse rien derrière.
func TestTravailNonDeplaceQuandOnRefuse(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	code, _ := h.script("deplacer", "a26.5n6.01", "tp1", "non", "quitter")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Annulé : rien n'a été renommé.")
	if noms := h.State.RepoNames("acme"); !slices.Contains(noms, "tp1-jlpicard") {
		t.Fatalf("dépôts : %v", noms)
	}
}

// « --move-to » ne pose aucune question : c'est la même opération, scriptable.
func TestTravailDeplaceEnLigneDeCommande(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	h.Options.MoveTo = "a26.5n6.01"
	h.Options.RenameTo = "travail-session"
	h.Options.Yes = true
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	noms := h.State.RepoNames("acme")
	if !slices.Contains(noms, "a26.5n6.01.travail-session.jlpicard") {
		t.Fatalf("dépôts : %v", noms)
	}
}

// La simulation montre le renommage et n'écrit rien.
func TestTravailDeplaceEnSimulation(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	h.Options.MoveTo = "a26.5n6.01"
	h.Options.DryRun = true
	h.Options.Yes = true
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("a26.5n6.01.tp1.jlpicard", "Simulation")
	if noms := h.State.RepoNames("acme"); !slices.Contains(noms, "tp1-jlpicard") {
		t.Fatalf("dépôts : %v", noms)
	}
}

// Une place d'arrivée qui ne sait pas nommer est refusée avant toute écriture.
func TestTravailRefuseUnePlaceHeritee(t *testing.T) {
	h := gestion(t, groupe(t), "tp1")
	h.Options.MoveTo = "vieux-prefixe"
	h.Options.Yes = true
	if code := h.muet(); code != app.ExitValidation {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if noms := h.State.RepoNames("acme"); !slices.Contains(noms, "tp1-jlpicard") {
		t.Fatalf("dépôts : %v", noms)
	}
}
