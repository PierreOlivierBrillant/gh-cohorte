package runner_test

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/runner"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/starter"
)

var cohorte = []roster.Person{
	{FullName: "Émilie Côté", Username: "emilie-cote"},
	{FullName: "Jean-Luc Picard", Username: "jlpicard"},
	{FullName: "Aminata Diallo", Username: "aminata-d"},
}

func reglages() config.Settings {
	settings := config.Default()
	settings.Org = "acme"
	settings.Assignment = "tp1"
	settings.DelaySeconds = 0
	return settings
}

func monter(t *testing.T, state *fakegh.State) (*ghapi.Client, *fakegh.Server) {
	t.Helper()
	if state == nil {
		state = fakegh.NewState()
	}
	serveur := fakegh.New(state)
	t.Cleanup(serveur.Close)
	client, err := ghapi.New(ghapi.Options{
		Host: "127.0.0.1", Token: "jeton-de-test", BaseURL: serveur.URL(),
		Sleep: func(time.Duration) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	return client, serveur
}

func construire(t *testing.T, settings config.Settings, gens []roster.Person) []plan.PlannedRepo {
	t.Helper()
	items, err := plan.Build(gens, settings)
	if err != nil {
		t.Fatalf("plan.Build : %v", err)
	}
	return items
}

func squelette(t *testing.T) *starter.Bundle {
	t.Helper()
	racine := t.TempDir()
	if err := os.WriteFile(filepath.Join(racine, "README.md"), []byte("# Départ\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(racine, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(racine, "src", "main.py"), []byte("print('à faire')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle, err := starter.Load(racine)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestCreationComplete(t *testing.T) {
	client, serveur := monter(t, nil)
	settings := reglages()
	items := construire(t, settings, cohorte)

	rapport, err := runner.New(client, settings, nil).Run(items, runner.Options{})
	if err != nil {
		t.Fatalf("Run : %v", err)
	}
	if rapport.Count(runner.Created) != 3 || len(rapport.Failures()) != 0 {
		t.Fatalf("bilan = %+v", rapport.Results)
	}
	attendu := []string{"tp1-aminata-d", "tp1-emilie-cote", "tp1-jlpicard"}
	if noms := serveur.State.RepoNames("acme"); strings.Join(noms, ",") != strings.Join(attendu, ",") {
		t.Fatalf("dépôts = %v", noms)
	}
	for _, resultat := range rapport.Results {
		if resultat.Collaborator != ghapi.CollaboratorInvited {
			t.Errorf("%s : invitation = %q", resultat.Repo, resultat.Collaborator)
		}
		if resultat.URL == "" {
			t.Errorf("%s : URL absente", resultat.Repo)
		}
	}
}

func TestIdempotenceRelance(t *testing.T) {
	client, serveur := monter(t, nil)
	settings := reglages()
	items := construire(t, settings, cohorte)
	executeur := runner.New(client, settings, nil)

	if _, err := executeur.Run(items, runner.Options{}); err != nil {
		t.Fatal(err)
	}
	serveur.State.AcceptInvitations("acme/tp1-emilie-cote")

	second, err := executeur.Run(items, runner.Options{})
	if err != nil {
		t.Fatalf("relance : %v", err)
	}
	if second.Count(runner.Existing) != 3 || second.Count(runner.Created) != 0 {
		t.Fatalf("la relance doit tout signaler « déjà présent » : %+v", second.Results)
	}
	if len(second.Failures()) != 0 {
		t.Errorf("la relance ne doit rien échouer : %+v", second.Failures())
	}
	if noms := serveur.State.RepoNames("acme"); len(noms) != 3 {
		t.Errorf("aucun dépôt ne doit être recréé : %v", noms)
	}
}

func TestInvitationManquanteRenvoyee(t *testing.T) {
	state := fakegh.NewState()
	// Le dépôt existe déjà, sans invitation : la relance doit la créer.
	state.AddRepo("acme", "tp1-jlpicard", true)
	client, serveur := monter(t, state)
	settings := reglages()
	items := construire(t, settings, []roster.Person{cohorte[1]})

	rapport, err := runner.New(client, settings, nil).Run(items, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rapport.Results[0].Status != runner.Existing {
		t.Fatalf("statut = %q", rapport.Results[0].Status)
	}
	if rapport.Results[0].Collaborator != ghapi.CollaboratorInvited {
		t.Fatalf("invitation = %q", rapport.Results[0].Collaborator)
	}
	if invitations := serveur.State.Invitations["acme/tp1-jlpicard"]; len(invitations) != 1 {
		t.Errorf("invitations = %+v", invitations)
	}
}

func TestEchecIsoleNInterrompsPasLeLot(t *testing.T) {
	state := fakegh.NewState()
	// L'invitation de la deuxième personne échoue, la création reste possible.
	state.FailOn["PUT /repos/acme/tp1-jlpicard/collaborators/jlpicard"] = fakegh.Failure{
		Status: 422, Message: "Invalid user",
	}
	client, serveur := monter(t, state)
	settings := reglages()
	items := construire(t, settings, cohorte)

	rapport, err := runner.New(client, settings, nil).Run(items, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rapport.Failures()) != 1 {
		t.Fatalf("échecs = %+v", rapport.Failures())
	}
	if rapport.Failures()[0].Repo != "tp1-jlpicard" {
		t.Errorf("échec sur %q", rapport.Failures()[0].Repo)
	}
	if !strings.Contains(rapport.Failures()[0].Error, "invitation impossible") {
		t.Errorf("motif = %q", rapport.Failures()[0].Error)
	}
	// Les deux autres personnes ont bien été servies malgré l'échec du milieu.
	if len(serveur.State.RepoNames("acme")) != 3 {
		t.Errorf("dépôts = %v", serveur.State.RepoNames("acme"))
	}
	if rapport.Count(runner.Created) != 2 {
		t.Errorf("créés = %d", rapport.Count(runner.Created))
	}
}

func TestFichiersDeDepart(t *testing.T) {
	client, serveur := monter(t, nil)
	settings := reglages()
	items := construire(t, settings, cohorte[:1])

	rapport, err := runner.New(client, settings, squelette(t)).Run(items, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rapport.Results[0].Starter != "2 fichier(s)" {
		t.Fatalf("fichiers de départ = %q", rapport.Results[0].Starter)
	}
	contenu := serveur.State.Files("acme/tp1-emilie-cote", "main")
	if contenu["README.md"] != "# Départ\n" || contenu["src/main.py"] != "print('à faire')\n" {
		t.Fatalf("contenu = %+v", contenu)
	}
	// auto_init doit rester faux : aucun README généré par GitHub ne s'ajoute.
	if len(contenu) != 2 {
		t.Errorf("%d fichier(s) dans le dépôt : %+v", len(contenu), contenu)
	}
}

func TestDepotDejaGarniPreserve(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-emilie-cote", true)
	state.SeedCommit("acme/tp1-emilie-cote", map[string]string{
		"README.md":    "# mon travail\n",
		"src/rendu.py": "print('déjà remis')\n",
	}, "main")
	client, serveur := monter(t, state)
	settings := reglages()
	items := construire(t, settings, cohorte[:1])

	rapport, err := runner.New(client, settings, squelette(t)).Run(items, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rapport.Results[0].Starter != runner.StarterSkipped {
		t.Fatalf("fichiers de départ = %q", rapport.Results[0].Starter)
	}
	contenu := serveur.State.Files("acme/tp1-emilie-cote", "main")
	if contenu["README.md"] != "# mon travail\n" {
		t.Error("le travail déjà remis doit primer")
	}
	if _, présent := contenu["src/main.py"]; présent {
		t.Error("aucun fichier de départ ne doit être ajouté")
	}
}

func TestForcerLesFichiersDeDepart(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-emilie-cote", true)
	state.SeedCommit("acme/tp1-emilie-cote", map[string]string{"README.md": "# mon travail\n"}, "main")
	client, serveur := monter(t, state)
	settings := reglages()
	items := construire(t, settings, cohorte[:1])

	rapport, err := runner.New(client, settings, squelette(t)).
		Run(items, runner.Options{ForceStarter: true})
	if err != nil {
		t.Fatal(err)
	}
	if rapport.Results[0].Starter != "2 fichier(s)" {
		t.Fatalf("fichiers de départ = %q", rapport.Results[0].Starter)
	}
	contenu := serveur.State.Files("acme/tp1-emilie-cote", "main")
	if contenu["README.md"] != "# Départ\n" {
		t.Error("--force-starter doit écraser le fichier de même nom")
	}
}

func TestRepriseApresEnvoiInterrompu(t *testing.T) {
	// Le dépôt a été créé mais l'envoi des fichiers a échoué : il est resté vide.
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-emilie-cote", true)
	client, serveur := monter(t, state)
	settings := reglages()
	items := construire(t, settings, cohorte[:1])

	rapport, err := runner.New(client, settings, squelette(t)).Run(items, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rapport.Results[0].Status != runner.Existing {
		t.Fatalf("statut = %q", rapport.Results[0].Status)
	}
	if rapport.Results[0].Starter != "2 fichier(s)" {
		t.Fatalf("un dépôt vide doit être complété : %q", rapport.Results[0].Starter)
	}
	if len(serveur.State.Files("acme/tp1-emilie-cote", "main")) != 2 {
		t.Error("les fichiers de départ devraient être déposés")
	}
}

func TestEchecDeLEnvoiDesFichiers(t *testing.T) {
	state := fakegh.NewState()
	state.FailOn["POST /repos/acme/tp1-emilie-cote/git/blobs"] = fakegh.Failure{
		Status: 403, Message: "refusing to allow an OAuth App to create or update workflow",
	}
	client, _ := monter(t, state)
	settings := reglages()
	items := construire(t, settings, cohorte[:1])

	rapport, err := runner.New(client, settings, squelette(t)).Run(items, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	resultat := rapport.Results[0]
	if resultat.Status != runner.Failed || resultat.Starter != runner.StarterFailed {
		t.Fatalf("résultat = %+v", resultat)
	}
	if !strings.Contains(resultat.Error, "workflow") {
		t.Errorf("le motif doit rappeler la portée manquante : %q", resultat.Error)
	}
	if resultat.Collaborator != runner.CollaboratorNo {
		t.Errorf("l'invitation ne doit pas être tentée après un échec : %q", resultat.Collaborator)
	}
}

func TestDepuisUnModele(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "modele-tp", false)
	state.SeedCommit("acme/modele-tp", map[string]string{"consignes.md": "# TP1\n"}, "main")
	client, serveur := monter(t, state)
	settings := reglages()
	settings.Template = "acme/modele-tp"
	items := construire(t, settings, cohorte[:1])

	rapport, err := runner.New(client, settings, nil).Run(items, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rapport.Count(runner.Created) != 1 {
		t.Fatalf("bilan = %+v", rapport.Results)
	}
	if contenu := serveur.State.Files("acme/tp1-emilie-cote", "main"); contenu["consignes.md"] == "" {
		t.Errorf("le contenu du modèle doit être repris : %+v", contenu)
	}
}

func TestSimulationNEcritRien(t *testing.T) {
	client, serveur := monter(t, nil)
	settings := reglages()
	items := construire(t, settings, cohorte)

	rapport, err := runner.New(client, settings, squelette(t)).
		Run(items, runner.Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if rapport.Count(runner.Created) != 3 {
		t.Fatalf("bilan = %+v", rapport.Results)
	}
	if noms := serveur.State.RepoNames("acme"); len(noms) != 0 {
		t.Fatalf("la simulation a créé des dépôts : %v", noms)
	}
	for _, appel := range serveur.State.AllCalls() {
		if strings.HasPrefix(appel, "POST") || strings.HasPrefix(appel, "PUT") ||
			strings.HasPrefix(appel, "DELETE") || strings.HasPrefix(appel, "PATCH") {
			t.Errorf("écriture pendant une simulation : %s", appel)
		}
	}
	if rapport.Results[0].Collaborator != runner.CollaboratorYes {
		t.Errorf("invitation annoncée = %q", rapport.Results[0].Collaborator)
	}
	if rapport.Results[0].Starter != "2 fichier(s) prévus" {
		t.Errorf("fichiers annoncés = %q", rapport.Results[0].Starter)
	}
}

func TestSimulationSignaleUnDepotDejaGarni(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "tp1-emilie-cote", true)
	state.SeedCommit("acme/tp1-emilie-cote", map[string]string{"README.md": "# remis\n"}, "main")
	client, _ := monter(t, state)
	settings := reglages()
	items := construire(t, settings, cohorte[:1])

	rapport, err := runner.New(client, settings, squelette(t)).
		Run(items, runner.Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if rapport.Results[0].Starter != runner.StarterSkipped {
		t.Errorf("annonce = %q", rapport.Results[0].Starter)
	}
}

func TestMargeEntreDeuxCreations(t *testing.T) {
	client, _ := monter(t, nil)
	settings := reglages()
	settings.DelaySeconds = 1.5
	items := construire(t, settings, cohorte)

	var pauses []time.Duration
	executeur := runner.New(client, settings, nil).
		WithClock(func(delay time.Duration) { pauses = append(pauses, delay) }, time.Now)
	if _, err := executeur.Run(items, runner.Options{}); err != nil {
		t.Fatal(err)
	}
	// Deux pauses pour trois créations : aucune après la dernière.
	if len(pauses) != 2 {
		t.Fatalf("pauses = %v", pauses)
	}
	for _, pause := range pauses {
		if pause != 1500*time.Millisecond {
			t.Errorf("pause = %v, attendu 1.5s", pause)
		}
	}
}

func TestAucunePauseEnSimulation(t *testing.T) {
	client, _ := monter(t, nil)
	settings := reglages()
	settings.DelaySeconds = 2
	items := construire(t, settings, cohorte)

	var pauses []time.Duration
	executeur := runner.New(client, settings, nil).
		WithClock(func(delay time.Duration) { pauses = append(pauses, delay) }, time.Now)
	if _, err := executeur.Run(items, runner.Options{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if len(pauses) != 0 {
		t.Errorf("la simulation ne doit pas temporiser : %v", pauses)
	}
}

func TestSansInvitation(t *testing.T) {
	client, serveur := monter(t, nil)
	settings := reglages()
	settings.AddCollaborator = false
	items := construire(t, settings, cohorte[:1])

	rapport, err := runner.New(client, settings, nil).Run(items, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rapport.Results[0].Collaborator != runner.CollaboratorNo {
		t.Errorf("invitation = %q", rapport.Results[0].Collaborator)
	}
	if serveur.State.CallCount("/collaborators/") != 0 {
		t.Error("aucune invitation ne devait être envoyée")
	}
}

func TestBilanJSONetCSV(t *testing.T) {
	client, _ := monter(t, nil)
	settings := reglages()
	items := construire(t, settings, cohorte)

	rapport, err := runner.New(client, settings, nil).Run(items, runner.Options{})
	if err != nil {
		t.Fatal(err)
	}
	dossier := t.TempDir()
	cheminJSON, cheminCSV, err := rapport.Save(dossier)
	if err != nil {
		t.Fatalf("Save : %v", err)
	}

	contenu, err := os.ReadFile(cheminJSON)
	if err != nil {
		t.Fatal(err)
	}
	var relu runner.Report
	if err := json.Unmarshal(contenu, &relu); err != nil {
		t.Fatalf("JSON illisible : %v", err)
	}
	if relu.Org != "acme" || relu.Assignment != "tp1" || len(relu.Results) != 3 {
		t.Fatalf("bilan relu = %+v", relu)
	}
	if relu.Results[0].FullName != "Émilie Côté" {
		t.Errorf("le nom complet doit figurer au bilan : %+v", relu.Results[0])
	}

	fichier, err := os.Open(cheminCSV)
	if err != nil {
		t.Fatal(err)
	}
	defer fichier.Close()
	lignes, err := csv.NewReader(fichier).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(lignes) != 4 || lignes[0][0] != "nom_complet" {
		t.Fatalf("CSV = %+v", lignes)
	}
	// Aucun secret ne doit se retrouver dans un fichier écrit.
	for _, chemin := range []string{cheminJSON, cheminCSV} {
		données, _ := os.ReadFile(chemin)
		if strings.Contains(string(données), "jeton-de-test") {
			t.Errorf("%s contient le jeton", chemin)
		}
	}
}

func TestProgressionAppeleeUneFoisParDepot(t *testing.T) {
	client, _ := monter(t, nil)
	settings := reglages()
	items := construire(t, settings, cohorte)

	var vus []string
	_, err := runner.New(client, settings, nil).Run(items, runner.Options{
		OnProgress: func(index, total int, result runner.Result) {
			if total != 3 || index != len(vus)+1 {
				t.Errorf("progression incohérente : %d/%d", index, total)
			}
			vus = append(vus, result.Repo)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(vus) != 3 {
		t.Errorf("progression = %v", vus)
	}
}
