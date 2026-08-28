package clone_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/clone"
)

// git lance une commande git isolée de la configuration de la machine.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	complet := append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.invalid",
		"-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false"}, args...)
	command := exec.Command("git", complet...)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+filepath.Join(t.TempDir(), "gitconfig-absent"),
		"GIT_CONFIG_SYSTEM="+filepath.Join(t.TempDir(), "gitconfig-absent"),
		"GIT_TERMINAL_PROMPT=0")
	sortie, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v : %v\n%s", args, err, sortie)
	}
	return string(sortie)
}

// depotDistant crée un dépôt git local qui servira d'origine.
func depotDistant(t *testing.T, nom string) string {
	t.Helper()
	racine := filepath.Join(t.TempDir(), nom)
	if err := os.MkdirAll(racine, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, racine, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(racine, "README.md"), []byte("# "+nom+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, racine, "add", ".")
	git(t, racine, "commit", "--quiet", "-m", "Premier commit")
	return racine
}

func urlDe(chemin string) string { return "file://" + filepath.ToSlash(chemin) }

func TestClonageParallele(t *testing.T) {
	cibles := []clone.Target{
		{Name: "tp1-emilie-cote", URL: urlDe(depotDistant(t, "tp1-emilie-cote"))},
		{Name: "tp1-jlpicard", URL: urlDe(depotDistant(t, "tp1-jlpicard"))},
		{Name: "tp1-aminata-d", URL: urlDe(depotDistant(t, "tp1-aminata-d"))},
	}
	destination := t.TempDir()

	var progression int
	resultats, err := clone.New(4, 0).Run(cibles, destination,
		func(done, total int, result clone.Result) {
			progression++
			if total != 3 {
				t.Errorf("total = %d", total)
			}
		})
	if err != nil {
		t.Fatalf("Run : %v", err)
	}
	if len(resultats) != 3 || progression != 3 {
		t.Fatalf("résultats = %+v", resultats)
	}
	for index, resultat := range resultats {
		if resultat.Status != clone.Cloned {
			t.Errorf("%s : %s (%s)", resultat.Name, resultat.Status, resultat.Error)
		}
		if resultat.Name != cibles[index].Name {
			t.Errorf("l'ordre des résultats doit suivre l'entrée : %+v", resultats)
		}
		if _, err := os.Stat(filepath.Join(destination, resultat.Name, "README.md")); err != nil {
			t.Errorf("%s non cloné : %v", resultat.Name, err)
		}
	}
}

func TestOriginPropreSansSecret(t *testing.T) {
	distant := depotDistant(t, "tp1-a")
	destination := t.TempDir()
	if _, err := clone.New(1, 0).Run([]clone.Target{{Name: "tp1-a", URL: urlDe(distant)}},
		destination, nil); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(destination, "tp1-a", ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	contenu := string(config)
	if !strings.Contains(contenu, urlDe(distant)) {
		t.Errorf("origin attendu dans .git/config : %s", contenu)
	}
	for _, interdit := range []string{"extraheader", "Authorization", "x-access-token", "credential"} {
		if strings.Contains(strings.ToLower(contenu), strings.ToLower(interdit)) {
			t.Errorf(".git/config contient « %s » : %s", interdit, contenu)
		}
	}
}

func TestRelanceMetAJourAuLieuDeRecreer(t *testing.T) {
	distant := depotDistant(t, "tp1-a")
	destination := t.TempDir()
	cloneur := clone.New(2, 0)
	cibles := []clone.Target{{Name: "tp1-a", URL: urlDe(distant)}}

	if _, err := cloneur.Run(cibles, destination, nil); err != nil {
		t.Fatal(err)
	}
	// Nouveau travail publié côté origine.
	if err := os.WriteFile(filepath.Join(distant, "consignes.md"), []byte("# consignes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, distant, "add", ".")
	git(t, distant, "commit", "--quiet", "-m", "Consignes")

	resultats, err := cloneur.Run(cibles, destination, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resultats[0].Status != clone.Updated {
		t.Fatalf("statut = %s (%s)", resultats[0].Status, resultats[0].Error)
	}
	if _, err := os.Stat(filepath.Join(destination, "tp1-a", "consignes.md")); err != nil {
		t.Errorf("la mise à jour n'a pas rapatrié le nouveau fichier : %v", err)
	}
}

func TestDossierOccupeNonGitIntact(t *testing.T) {
	distant := depotDistant(t, "tp1-a")
	destination := t.TempDir()
	occupé := filepath.Join(destination, "tp1-a")
	if err := os.MkdirAll(occupé, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(occupé, "mes-notes.txt"), []byte("précieux"), 0o644); err != nil {
		t.Fatal(err)
	}

	resultats, err := clone.New(1, 0).Run([]clone.Target{{Name: "tp1-a", URL: urlDe(distant)}},
		destination, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resultats[0].Status != clone.Skipped {
		t.Fatalf("statut = %s", resultats[0].Status)
	}
	contenu, err := os.ReadFile(filepath.Join(occupé, "mes-notes.txt"))
	if err != nil || string(contenu) != "précieux" {
		t.Errorf("le dossier a été touché : %q, %v", contenu, err)
	}
}

func TestTravailLocalJamaisEcrase(t *testing.T) {
	distant := depotDistant(t, "tp1-a")
	destination := t.TempDir()
	cloneur := clone.New(1, 0)
	cibles := []clone.Target{{Name: "tp1-a", URL: urlDe(distant)}}
	if _, err := cloneur.Run(cibles, destination, nil); err != nil {
		t.Fatal(err)
	}

	// Un commit local diverge de l'origine, qui avance de son côté.
	local := filepath.Join(destination, "tp1-a")
	if err := os.WriteFile(filepath.Join(local, "notes.md"), []byte("mon travail\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, local, "add", ".")
	git(t, local, "commit", "--quiet", "-m", "Travail local")

	if err := os.WriteFile(filepath.Join(distant, "autre.md"), []byte("côté prof\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, distant, "add", ".")
	git(t, distant, "commit", "--quiet", "-m", "Côté prof")

	resultats, err := cloneur.Update([]clone.Clone{{Name: "tp1-a", Path: local}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resultats[0].Status != clone.Failed {
		t.Fatalf("--ff-only doit refuser la fusion : %+v", resultats[0])
	}
	contenu, err := os.ReadFile(filepath.Join(local, "notes.md"))
	if err != nil || string(contenu) != "mon travail\n" {
		t.Errorf("le travail local a été modifié : %q, %v", contenu, err)
	}
}

func TestMiseAJourAvanceEnAvanceRapide(t *testing.T) {
	distant := depotDistant(t, "tp1-a")
	destination := t.TempDir()
	cloneur := clone.New(1, 0)
	if _, err := cloneur.Run([]clone.Target{{Name: "tp1-a", URL: urlDe(distant)}}, destination, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distant, "suite.md"), []byte("suite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, distant, "add", ".")
	git(t, distant, "commit", "--quiet", "-m", "Suite")

	local := filepath.Join(destination, "tp1-a")
	resultats, err := cloneur.Update([]clone.Clone{{Name: "tp1-a", Path: local}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resultats[0].Status != clone.Updated {
		t.Fatalf("statut = %s (%s)", resultats[0].Status, resultats[0].Error)
	}
	if _, err := os.Stat(filepath.Join(local, "suite.md")); err != nil {
		t.Errorf("mise à jour incomplète : %v", err)
	}
}

func TestUpdateDossierSansGit(t *testing.T) {
	dossier := t.TempDir()
	resultats, err := clone.New(1, 0).Update([]clone.Clone{{Name: "notes", Path: dossier}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resultats[0].Status != clone.Skipped {
		t.Errorf("statut = %+v", resultats[0])
	}
}

func TestProfondeurLimitee(t *testing.T) {
	distant := depotDistant(t, "tp1-a")
	for index := 0; index < 3; index++ {
		nom := filepath.Join(distant, "fichier"+string(rune('a'+index))+".txt")
		if err := os.WriteFile(nom, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		git(t, distant, "add", ".")
		git(t, distant, "commit", "--quiet", "-m", "Commit supplémentaire")
	}
	destination := t.TempDir()
	if _, err := clone.New(1, 1).Run([]clone.Target{{Name: "tp1-a", URL: urlDe(distant)}},
		destination, nil); err != nil {
		t.Fatal(err)
	}
	sortie := git(t, filepath.Join(destination, "tp1-a"), "rev-list", "--count", "HEAD")
	if strings.TrimSpace(sortie) != "1" {
		t.Errorf("--depth 1 attendu, %s commit(s) présents", strings.TrimSpace(sortie))
	}
}

func TestCloneIntrouvable(t *testing.T) {
	destination := t.TempDir()
	resultats, err := clone.New(1, 0).Run(
		[]clone.Target{{Name: "tp1-absent", URL: urlDe(filepath.Join(t.TempDir(), "nulle-part"))}},
		destination, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resultats[0].IsFailed() || resultats[0].Error == "" {
		t.Errorf("résultat = %+v", resultats[0])
	}
}

func TestFindClones(t *testing.T) {
	dossier := t.TempDir()
	for _, nom := range []string{"tp1-b", "tp1-a"} {
		chemin := filepath.Join(dossier, nom)
		if err := os.MkdirAll(filepath.Join(chemin, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dossier, "notes-personnelles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dossier, "fichier.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	clones, err := clone.FindClones(dossier)
	if err != nil {
		t.Fatal(err)
	}
	if len(clones) != 2 || clones[0].Name != "tp1-a" || clones[1].Name != "tp1-b" {
		t.Fatalf("clones = %+v", clones)
	}
	if _, err := clone.FindClones(filepath.Join(dossier, "absent")); err == nil {
		t.Error("un dossier absent doit produire une erreur")
	}
}

func TestPrepareDestination(t *testing.T) {
	dossier := filepath.Join(t.TempDir(), "cible", "tp1")
	chemin, err := clone.PrepareDestination(dossier)
	if err != nil {
		t.Fatalf("PrepareDestination : %v", err)
	}
	if info, err := os.Stat(chemin); err != nil || !info.IsDir() {
		t.Errorf("dossier non créé : %v", err)
	}
	// Rien ne doit rester du test d'écriture.
	entrées, err := os.ReadDir(chemin)
	if err != nil || len(entrées) != 0 {
		t.Errorf("dossier non vide : %+v, %v", entrées, err)
	}

	fichier := filepath.Join(t.TempDir(), "fichier.txt")
	if err := os.WriteFile(fichier, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := clone.PrepareDestination(fichier); err == nil {
		t.Error("un fichier ne peut pas servir de destination")
	}
}

func TestRunSansCible(t *testing.T) {
	resultats, err := clone.New(4, 0).Run(nil, t.TempDir(), nil)
	if err != nil || len(resultats) != 0 {
		t.Errorf("Run = %+v, %v", resultats, err)
	}
}
