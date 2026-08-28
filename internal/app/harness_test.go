package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
)

// harnais monte un faux GitHub, des dossiers jetables et une console captée,
// pour dérouler des parcours complets sans réseau ni terminal.
type harnais struct {
	t         *testing.T
	State     *fakegh.State
	Serveur   *fakegh.Server
	Options   *app.Options
	Console   *ui.Console
	Sortie    *bytes.Buffer
	Rapports  string
	Reglages  string
	CacheDir  string
	Pauses    []time.Duration
	dernierRC int
}

func nouveau(t *testing.T, state *fakegh.State) *harnais {
	t.Helper()
	if state == nil {
		state = fakegh.NewState()
	}
	serveur := fakegh.New(state)
	t.Cleanup(serveur.Close)

	dossier := t.TempDir()
	sortie := &bytes.Buffer{}
	h := &harnais{
		t:        t,
		State:    state,
		Serveur:  serveur,
		Sortie:   sortie,
		Console:  ui.NewConsoleFor(sortie),
		Rapports: filepath.Join(dossier, "rapports"),
		Reglages: filepath.Join(dossier, "config.json"),
		CacheDir: filepath.Join(dossier, "cache"),
	}
	h.Options = &app.Options{
		Org:        "acme",
		ReportDir:  h.Rapports,
		ConfigPath: h.Reglages,
		CacheDir:   h.CacheDir,
		BaseURL:    serveur.URL(),
		Jobs:       4,
		Delay:      0,
		DelaySet:   true,
	}
	return h
}

// executer déroule une session avec le questionneur donné.
func (h *harnais) executer(prompter ui.Prompter) int {
	h.t.Helper()
	session := app.New(h.Options, h.Console, prompter)
	session.Sleep = func(delay time.Duration) { h.Pauses = append(h.Pauses, delay) }
	h.dernierRC = session.Run()
	return h.dernierRC
}

// script déroule une session interactive avec des réponses préparées.
func (h *harnais) script(reponses ...string) (int, *ui.Scripted) {
	h.t.Helper()
	scripte := ui.NewScripted(reponses...)
	code := h.executer(scripte)
	return code, scripte
}

// muet déroule une session non interactive.
func (h *harnais) muet() int {
	h.t.Helper()
	h.Options.NonInteractive = true
	return h.executer(&ui.ScriptPrompter{})
}

// texte renvoie tout ce qui a été affiché.
func (h *harnais) texte() string { return h.Sortie.String() }

// contient vérifie la présence d'un fragment dans la sortie.
func (h *harnais) contient(fragments ...string) {
	h.t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(h.texte(), fragment) {
			h.t.Errorf("sortie sans « %s » :\n%s", fragment, h.texte())
		}
	}
}

// absent vérifie l'absence d'un fragment dans la sortie.
func (h *harnais) absent(fragments ...string) {
	h.t.Helper()
	for _, fragment := range fragments {
		if strings.Contains(h.texte(), fragment) {
			h.t.Errorf("sortie contenant « %s » à tort :\n%s", fragment, h.texte())
		}
	}
}

// cohorteCSV écrit une liste de personnes et renvoie son chemin.
func (h *harnais) cohorteCSV(lignes ...string) string {
	h.t.Helper()
	contenu := "nom_complet,github_username\n"
	if len(lignes) == 0 {
		lignes = []string{
			"Émilie Côté,emilie-cote",
			"Jean-Luc Picard,jlpicard",
			"Aminata Diallo,aminata-d",
		}
	}
	contenu += strings.Join(lignes, "\n") + "\n"
	chemin := filepath.Join(h.t.TempDir(), "cohorte.csv")
	if err := os.WriteFile(chemin, []byte(contenu), 0o644); err != nil {
		h.t.Fatal(err)
	}
	return chemin
}

// squelette écrit un dossier de fichiers de départ et renvoie son chemin.
func (h *harnais) squelette() string {
	h.t.Helper()
	racine := filepath.Join(h.t.TempDir(), "depart")
	if err := os.MkdirAll(filepath.Join(racine, "src"), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(racine, "README.md"), []byte("# À faire\n"), 0o644); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(racine, "src", "main.py"), []byte("# à compléter\n"), 0o644); err != nil {
		h.t.Fatal(err)
	}
	return racine
}

// bilans renvoie les bilans JSON écrits par la session.
func (h *harnais) bilans() []string {
	h.t.Helper()
	matches, err := filepath.Glob(filepath.Join(h.Rapports, "*.json"))
	if err != nil {
		h.t.Fatal(err)
	}
	return matches
}
