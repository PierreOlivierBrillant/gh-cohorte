package app_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/students"
)

func analyser(t *testing.T, args ...string) *app.Options {
	t.Helper()
	options, err := app.Parse(args, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse(%v) : %v", args, err)
	}
	return options
}

func TestParseDrapeauxSimples(t *testing.T) {
	options := analyser(t, "--org", "acme", "--assignment", "tp1", "--roster", "liste.csv",
		"--visibility", "public", "--permission", "maintain", "--dry-run", "-y")
	if options.Org != "acme" || options.Assignment != "tp1" || options.Roster != "liste.csv" {
		t.Fatalf("options = %+v", options)
	}
	if options.Visibility != "public" || options.Permission != "maintain" {
		t.Fatalf("options = %+v", options)
	}
	if !options.DryRun || !options.Yes {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseManageAvecEtSansPrefixe(t *testing.T) {
	cas := map[string]string{
		"--manage":     "",
		"--manage=tp1": "tp1",
		"--manage tp1": "tp1",
		"-manage tp1":  "tp1",
		"-manage":      "",
	}
	for entree, attendu := range cas {
		options := analyser(t, strings.Fields(entree)...)
		if !options.ManageRequested {
			t.Errorf("%q : le mode gestion n'est pas demandé", entree)
			continue
		}
		if options.Manage != attendu {
			t.Errorf("%q : préfixe = %q, attendu %q", entree, options.Manage, attendu)
		}
	}
	// Sans le drapeau, le mode gestion n'est pas demandé.
	if analyser(t, "--org", "acme").ManageRequested {
		t.Error("le mode gestion ne doit pas être demandé par défaut")
	}
}

func TestParseManageSuiviDUnAutreDrapeau(t *testing.T) {
	options := analyser(t, "--manage", "--org", "acme")
	if !options.ManageRequested || options.Manage != "" || options.Org != "acme" {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseValeursVidesExplicites(t *testing.T) {
	options := analyser(t, "--template=", "--starter=")
	if !options.TemplateSet || options.Template != "" {
		t.Errorf("--template= doit effacer le modèle : %+v", options)
	}
	if !options.StarterSet || options.Starter != "" {
		t.Errorf("--starter= doit effacer le dossier : %+v", options)
	}

	sansDrapeau := analyser(t, "--org", "acme")
	if sansDrapeau.TemplateSet || sansDrapeau.StarterSet {
		t.Errorf("aucun drapeau ne doit être marqué comme fourni : %+v", sansDrapeau)
	}
}

func TestParseDelaiEtParallelisme(t *testing.T) {
	options := analyser(t, "--delay", "2.5", "--jobs", "8", "--depth", "1")
	if !options.DelaySet || options.Delay != 2.5 || options.Jobs != 8 || options.Depth != 1 {
		t.Fatalf("options = %+v", options)
	}
	// Des valeurs aberrantes sont ramenées à un minimum utilisable.
	borne := analyser(t, "--jobs", "0", "--depth", "-5")
	if borne.Jobs != 1 || borne.Depth != 0 {
		t.Fatalf("options = %+v", borne)
	}
	if analyser(t).DelaySet {
		t.Error("sans --delay, la marge mémorisée doit rester en vigueur")
	}
}

func TestParseArgumentInattendu(t *testing.T) {
	if _, err := app.Parse([]string{"tp1"}, &bytes.Buffer{}); err == nil {
		t.Error("un argument libre doit être refusé")
	}
}

func TestParseDrapeauInconnu(t *testing.T) {
	sortie := &bytes.Buffer{}
	if _, err := app.Parse([]string{"--inconnu"}, sortie); err == nil {
		t.Error("un drapeau inconnu doit être refusé")
	}
}

func TestUsageEnFrancais(t *testing.T) {
	sortie := &bytes.Buffer{}
	app.Usage(sortie)
	texte := sortie.String()
	for _, fragment := range []string{
		"organisation GitHub cible", "gérer un groupe existant",
		"simuler sans rien créer", "{assignment}", "Codes de retour",
		"régénérer le jeton GitHub", "portées à obtenir",
	} {
		if !strings.Contains(texte, fragment) {
			t.Errorf("aide sans « %s » :\n%s", fragment, texte)
		}
	}
}

func TestParseMessagesEnFrancais(t *testing.T) {
	sortie := &bytes.Buffer{}
	_, err := app.Parse([]string{"--inconnu"}, sortie)
	if err == nil {
		t.Fatal("un drapeau inconnu doit être refusé")
	}
	if !strings.Contains(err.Error(), "Drapeau inconnu") {
		t.Errorf("message = %v", err)
	}

	_, err = app.Parse([]string{"--org"}, sortie)
	if err == nil || !strings.Contains(err.Error(), "Valeur manquante") {
		t.Errorf("message = %v", err)
	}

	_, err = app.Parse([]string{"--delay", "beaucoup"}, sortie)
	if err == nil || !strings.Contains(err.Error(), "Valeur invalide") {
		t.Errorf("message = %v", err)
	}
}

func TestParseAide(t *testing.T) {
	for _, drapeau := range []string{"-h", "--help", "help"} {
		sortie := &bytes.Buffer{}
		if _, err := app.Parse([]string{drapeau}, sortie); err == nil {
			t.Errorf("%s : l'aide doit interrompre l'analyse", drapeau)
		}
		if !strings.Contains(sortie.String(), "assistant interactif au terminal") {
			t.Errorf("%s : aide absente :\n%s", drapeau, sortie.String())
		}
	}
}

// Les critères de liste sont validés dès la ligne de commande : une date mal
// écrite doit arrêter l'outil, pas se perdre en cours de route.
func TestOptionsFiltreEtTriDeLaListe(t *testing.T) {
	options, err := app.Parse([]string{
		"--filter", "cote", "--pushed-after", "2026-10-01",
		"--never-pushed", "--sort", "envoi", "--sort-desc",
	}, io.Discard)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	if options.Filter.Text != "cote" || options.Filter.PushedAfter != "2026-10-01" {
		t.Fatalf("filtre : %+v", options.Filter)
	}
	if options.Filter.Activity != students.Silent {
		t.Fatalf("activité : %q", options.Filter.Activity)
	}
	if options.Sort != students.ByPushed || !options.SortDesc {
		t.Fatalf("tri : %q (décroissant : %v)", options.Sort, options.SortDesc)
	}

	if _, err := app.Parse([]string{"--pushed-before", "hier"}, io.Discard); err == nil {
		t.Fatal("une date qui n'en est pas une doit être refusée")
	}
	if _, err := app.Parse([]string{"--sort", "popularite"}, io.Discard); err == nil {
		t.Fatal("un tri inconnu doit être refusé")
	}
}

func TestParseRenouvellementDuJeton(t *testing.T) {
	options := analyser(t, "--refresh-token", "--scopes", "workflow,delete_repo")
	if !options.RefreshToken || options.Scopes != "workflow,delete_repo" {
		t.Fatalf("options = %+v", options)
	}
	// Sans « --scopes », ce sont toutes les portées dont l'outil se sert.
	if seul := analyser(t, "--refresh-token"); seul.Scopes != "" {
		t.Fatalf("portées = %q", seul.Scopes)
	}
}
