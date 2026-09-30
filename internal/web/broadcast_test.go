package web_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// travailOuDeposer monte un travail de deux dépôts : Émilie a déjà un fichier
// du même nom, Jean-Luc non.
func travailOuDeposer(t *testing.T) *harnais {
	t.Helper()
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	state.SeedCommit("acme/a26.5n6.01.tp1.emilie-cote",
		map[string]string{"NOTE.md": "ma version\n"}, "main")
	state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	state.SeedCommit("acme/a26.5n6.01.tp1.jean-luc-picard",
		map[string]string{"main.py": "print(2)\n"}, "main")
	return avantLeRegistre(t, state, cohorte("a26", "5n6", "01",
		"Émilie Côté", "emilie-cote", "Jean-Luc Picard", "jlpicard"))
}

func TestLesChampsDuGabaritSontCeuxDuDomaine(t *testing.T) {
	h := travailOuDeposer(t)
	var champs []struct {
		Name string `json:"name"`
	}
	h.json(http.MethodGet, "/api/broadcast/fields", nil, &champs)
	if len(champs) == 0 || champs[0].Name != "nom_etudiant" {
		t.Errorf("champs = %+v", champs)
	}
}

// L'aperçu ne demande rien à GitHub : il remplit le gabarit pour le premier
// dépôt, et nomme ce qui risque de mal tourner.
func TestLApercuRemplitLeGabaritPourLePremierDepot(t *testing.T) {
	h := travailOuDeposer(t)
	var apercu struct {
		Templated bool     `json:"templated"`
		Unknown   []string `json:"unknown"`
		Sample    struct {
			Repo    string `json:"repo"`
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"sample"`
	}
	h.json(http.MethodPost, "/api/classrooms/a26.5n6.01/assignments/tp1/file/preview",
		map[string]any{
			"content": []byte("{nom_etudiant} — {cours} {grp}\n"), "name": "NOTE.md",
			"target": "docs/{compte}.md",
		}, &apercu)
	if !apercu.Templated || apercu.Sample.Repo != "a26.5n6.01.tp1.emilie-cote" ||
		apercu.Sample.Path != "docs/emilie-cote.md" ||
		apercu.Sample.Content != "Émilie Côté — 5n6 {grp}\n" {
		t.Errorf("aperçu = %+v", apercu)
	}
	if len(apercu.Unknown) != 1 || apercu.Unknown[0] != "grp" {
		t.Errorf("inconnus = %v", apercu.Unknown)
	}
}

func TestLeDepotDepuisLeNavigateurNEcrasePasSansDemande(t *testing.T) {
	h := travailOuDeposer(t)
	fichier := filepath.Join(t.TempDir(), "NOTE.md")
	if err := os.WriteFile(fichier, []byte("Bonjour {prenom}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// En simulation, rien n'est écrit.
	h.travail(http.MethodPost, "/api/classrooms/a26.5n6.01/assignments/tp1/file",
		map[string]any{"path": fichier, "dry_run": true})
	if _, existe := h.State.Files("acme/a26.5n6.01.tp1.jean-luc-picard", "main")["NOTE.md"]; existe {
		t.Fatal("la simulation a écrit")
	}

	etat := h.travail(http.MethodPost, "/api/classrooms/a26.5n6.01/assignments/tp1/file",
		map[string]any{"path": fichier})
	if etat["status"] != "terminé" {
		t.Fatalf("état = %+v", etat)
	}
	if got := h.State.Files("acme/a26.5n6.01.tp1.jean-luc-picard", "main")["NOTE.md"]; got != "Bonjour Jean-Luc\n" {
		t.Errorf("Jean-Luc a reçu %q", got)
	}
	if got := h.State.Files("acme/a26.5n6.01.tp1.emilie-cote", "main")["NOTE.md"]; got != "ma version\n" {
		t.Errorf("le fichier d'Émilie a été écrasé : %q", got)
	}

	h.travail(http.MethodPost, "/api/classrooms/a26.5n6.01/assignments/tp1/file",
		map[string]any{"path": fichier, "overwrite": true})
	if got := h.State.Files("acme/a26.5n6.01.tp1.emilie-cote", "main")["NOTE.md"]; got != "Bonjour Émilie\n" {
		t.Errorf("Émilie a %q", got)
	}
}

func TestLeDepotSansFichierEstRefuse(t *testing.T) {
	h := travailOuDeposer(t)
	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/a26.5n6.01/assignments/tp1/file/preview", map[string]any{})
	if reponse.StatusCode < 400 {
		t.Errorf("statut %d — %s", reponse.StatusCode, contenu)
	}
}
