package ui_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
)

func console() (*ui.Console, *bytes.Buffer) {
	tampon := &bytes.Buffer{}
	return ui.NewConsoleFor(tampon), tampon
}

func TestSortieRedirigeeSansCouleurNiRetourChariot(t *testing.T) {
	c, tampon := console()
	if c.TTY || c.Color {
		t.Fatal("une sortie redirigée n'est ni un terminal ni colorée")
	}
	c.Heading("Titre")
	c.Success("créé %s", "tp1-a")
	c.Warning("attention")
	c.Failure("raté")
	c.Note("détail")
	c.Table([]string{"Dépôt", "Personne"}, [][]string{{"tp1-a", "Émilie Côté"}}, 0)

	sortie := tampon.String()
	if strings.Contains(sortie, "\x1b[") {
		t.Errorf("séquence d'échappement dans la sortie : %q", sortie)
	}
	if strings.Contains(sortie, "\r") {
		t.Errorf("retour chariot dans la sortie : %q", sortie)
	}
	if !strings.Contains(sortie, "créé tp1-a") || !strings.Contains(sortie, "Émilie Côté") {
		t.Errorf("sortie incomplète : %q", sortie)
	}
}

func TestTableAligneEtTronque(t *testing.T) {
	c, tampon := console()
	lignes := [][]string{
		{"1", "tp1-emilie-cote", "Émilie Côté"},
		{"2", "tp1-jlpicard", "Jean-Luc Picard"},
		{"3", "tp1-aminata-d", "Aminata Diallo"},
	}
	c.Table([]string{"#", "Dépôt", "Nom complet"}, lignes, 2)
	sortie := tampon.String()
	if !strings.Contains(sortie, "tp1-emilie-cote") || strings.Contains(sortie, "tp1-aminata-d") {
		t.Errorf("la limite n'est pas respectée : %q", sortie)
	}
	if !strings.Contains(sortie, "et 1 ligne(s) de plus") {
		t.Errorf("les lignes masquées doivent être annoncées : %q", sortie)
	}
}

func TestProgressionHorsTerminal(t *testing.T) {
	c, tampon := console()
	barre := ui.NewProgress(c, "Comptes GitHub", 8)
	for done := 1; done <= 8; done++ {
		barre.Update(done, "@quelquun")
	}
	barre.Finish("  terminé")

	sortie := tampon.String()
	if strings.Contains(sortie, "\r") || strings.Contains(sortie, "\x1b[") {
		t.Errorf("la progression doit rester lisible dans un journal : %q", sortie)
	}
	if lignes := strings.Count(sortie, "Comptes GitHub"); lignes > 5 {
		t.Errorf("%d lignes de progression, c'est trop : %q", lignes, sortie)
	}
	if !strings.Contains(sortie, "8/8") {
		t.Errorf("la fin doit être annoncée : %q", sortie)
	}
}

func TestScriptedAsk(t *testing.T) {
	scripte := ui.NewScripted("acme", "", "  tp1  ")
	valeur, err := scripte.Ask(ui.Question{Title: "Organisation"})
	if err != nil || valeur != "acme" {
		t.Fatalf("Ask = %q, %v", valeur, err)
	}
	valeur, err = scripte.Ask(ui.Question{Title: "Travail", Default: "tp0"})
	if err != nil || valeur != "tp0" {
		t.Fatalf("la réponse vide doit reprendre le défaut : %q, %v", valeur, err)
	}
	valeur, err = scripte.Ask(ui.Question{Title: "Travail", Validate: func(v string) (string, error) {
		return strings.ToUpper(v), nil
	}})
	if err != nil || valeur != "TP1" {
		t.Fatalf("Ask = %q, %v", valeur, err)
	}
	if _, err := scripte.Ask(ui.Question{Title: "De trop"}); err == nil {
		t.Error("un scénario épuisé doit échouer")
	}
}

func TestScriptedChoixEtConfirmation(t *testing.T) {
	options := ui.Options("creer", "Créer des dépôts", "gerer", "Gérer un groupe")
	scripte := ui.NewScripted("gerer", "2", "Créer", "", "oui", "non")

	for _, attendu := range []string{"gerer", "gerer", "creer", "creer"} {
		choix, err := scripte.Choose("Que faire ?", options, "creer")
		if err != nil || choix != attendu {
			t.Fatalf("Choose = %q, %v (attendu %q)", choix, err, attendu)
		}
	}
	if reponse, err := scripte.Confirm("Continuer ?", false); err != nil || !reponse {
		t.Errorf("Confirm = %v, %v", reponse, err)
	}
	if reponse, err := scripte.Confirm("Continuer ?", true); err != nil || reponse {
		t.Errorf("Confirm = %v, %v", reponse, err)
	}
}

func TestScriptedMultiSelect(t *testing.T) {
	options := ui.Options("a", "tp1-a", "b", "tp1-b", "c", "tp1-c")
	scripte := ui.NewScripted("tous", "1,3", "tp1-b", "-", "9")

	if choix, err := scripte.MultiSelect("Dépôts", options, nil); err != nil || len(choix) != 3 {
		t.Fatalf("« tous » = %v, %v", choix, err)
	}
	if choix, err := scripte.MultiSelect("Dépôts", options, nil); err != nil ||
		len(choix) != 2 || choix[0] != 0 || choix[1] != 2 {
		t.Fatalf("« 1,3 » = %v, %v", choix, err)
	}
	if choix, err := scripte.MultiSelect("Dépôts", options, nil); err != nil ||
		len(choix) != 1 || choix[0] != 1 {
		t.Fatalf("par nom = %v, %v", choix, err)
	}
	if choix, err := scripte.MultiSelect("Dépôts", options, nil); err != nil || len(choix) != 0 {
		t.Fatalf("« - » = %v, %v", choix, err)
	}
	if _, err := scripte.MultiSelect("Dépôts", options, nil); err == nil {
		t.Error("une sélection hors liste doit échouer")
	}
}

func TestScriptedInterruption(t *testing.T) {
	scripte := ui.NewScripted("\x03")
	if _, err := scripte.Ask(ui.Question{Title: "Organisation"}); err != ui.ErrAborted {
		t.Errorf("erreur = %v", err)
	}
}

func TestScriptPrompterRefuseToutesLesQuestions(t *testing.T) {
	prompteur := &ui.ScriptPrompter{}
	if prompteur.Interactive() {
		t.Error("le mode script n'est pas interactif")
	}
	if _, err := prompteur.Ask(ui.Question{Title: "Organisation"}); err == nil {
		t.Error("Ask doit échouer")
	} else if !strings.Contains(err.Error(), "non interactif") {
		t.Errorf("message = %v", err)
	}
	if _, err := prompteur.Confirm("Créer ?", false); err == nil {
		t.Error("Confirm doit échouer")
	} else if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("le message doit indiquer la correction : %v", err)
	}
	if _, err := prompteur.Choose("Quoi ?", nil, ""); err == nil {
		t.Error("Choose doit échouer")
	}
	if _, err := prompteur.MultiSelect("Quoi ?", nil, nil); err == nil {
		t.Error("MultiSelect doit échouer")
	}
}

func TestTableSansLigne(t *testing.T) {
	c, tampon := console()
	c.Table([]string{"Dépôt"}, nil, 10)
	if tampon.Len() != 0 {
		t.Errorf("un tableau vide ne doit rien écrire : %q", tampon.String())
	}
}

func TestBanniereEtTitres(t *testing.T) {
	c, tampon := console()
	c.Banner("gh cohorte 1.0", "Un dépôt par personne")
	c.Heading("Organisation cible")
	sortie := tampon.String()
	if !strings.Contains(sortie, "gh cohorte 1.0") || !strings.Contains(sortie, "Organisation cible") {
		t.Errorf("sortie = %q", sortie)
	}
}

func TestLinePrompterAsk(t *testing.T) {
	c, tampon := console()
	p := ui.NewLinePrompter(c, strings.NewReader("\nacme\n"))

	valeur, err := p.Ask(ui.Question{Title: "Organisation", Default: "defaut"})
	if err != nil || valeur != "defaut" {
		t.Fatalf("Ask = %q, %v", valeur, err)
	}
	valeur, err = p.Ask(ui.Question{Title: "Organisation"})
	if err != nil || valeur != "acme" {
		t.Fatalf("Ask = %q, %v", valeur, err)
	}
	if !strings.Contains(tampon.String(), "Organisation [defaut] :") {
		t.Errorf("invite = %q", tampon.String())
	}
}

func TestLinePrompterAskRedemandeApresErreur(t *testing.T) {
	c, tampon := console()
	p := ui.NewLinePrompter(c, strings.NewReader("-invalide-\noctocat\n"))
	valeur, err := p.Ask(ui.Question{
		Title: "Compte GitHub",
		Validate: func(value string) (string, error) {
			if strings.HasPrefix(value, "-") {
				return "", ui.ErrAborted
			}
			return value, nil
		},
	})
	if err != nil || valeur != "octocat" {
		t.Fatalf("Ask = %q, %v", valeur, err)
	}
	if !strings.Contains(tampon.String(), "✗") {
		t.Errorf("l'erreur doit être signalée : %q", tampon.String())
	}
}

func TestLinePrompterConfirm(t *testing.T) {
	c, _ := console()
	p := ui.NewLinePrompter(c, strings.NewReader("\no\nnon\npeut-être\noui\n"))

	if reponse, err := p.Confirm("Continuer ?", true); err != nil || !reponse {
		t.Fatalf("défaut = %v, %v", reponse, err)
	}
	if reponse, err := p.Confirm("Continuer ?", false); err != nil || !reponse {
		t.Fatalf("« o » = %v, %v", reponse, err)
	}
	if reponse, err := p.Confirm("Continuer ?", true); err != nil || reponse {
		t.Fatalf("« non » = %v, %v", reponse, err)
	}
	// Une réponse incomprise fait reposer la question.
	if reponse, err := p.Confirm("Continuer ?", false); err != nil || !reponse {
		t.Fatalf("après reprise = %v, %v", reponse, err)
	}
}

func TestLinePrompterChoose(t *testing.T) {
	c, tampon := console()
	options := ui.Options("creer", "Créer des dépôts", "gerer", "Gérer un groupe")
	p := ui.NewLinePrompter(c, strings.NewReader("2\n\n9\ngerer\n"))

	if choix, err := p.Choose("Que faire ?", options, "creer"); err != nil || choix != "gerer" {
		t.Fatalf("par numéro = %q, %v", choix, err)
	}
	if choix, err := p.Choose("Que faire ?", options, "creer"); err != nil || choix != "creer" {
		t.Fatalf("par défaut = %q, %v", choix, err)
	}
	// « 9 » sort de la liste : la question est reposée.
	if choix, err := p.Choose("Que faire ?", options, "creer"); err != nil || choix != "gerer" {
		t.Fatalf("après reprise = %q, %v", choix, err)
	}
	sortie := tampon.String()
	if !strings.Contains(sortie, "1  Créer des dépôts") || !strings.Contains(sortie, "2  Gérer un groupe") {
		t.Errorf("liste numérotée attendue : %q", sortie)
	}
	if strings.Contains(sortie, "\x1b[") {
		t.Errorf("aucune séquence d'échappement hors terminal : %q", sortie)
	}
}

func TestLinePrompterMultiSelect(t *testing.T) {
	c, _ := console()
	options := ui.Options("a", "tp1-a", "b", "tp1-b", "c", "tp1-c")
	p := ui.NewLinePrompter(c, strings.NewReader("\n1,3\ntp1-b\n42\n2-3\n-\n"))

	cas := [][]int{{0, 1, 2}, {0, 2}, {1}, {1, 2}}
	for _, attendu := range cas {
		indices, err := p.MultiSelect("Dépôts", options, nil)
		if err != nil {
			t.Fatalf("MultiSelect : %v", err)
		}
		if len(indices) != len(attendu) {
			t.Fatalf("MultiSelect = %v, attendu %v", indices, attendu)
		}
		for position, valeur := range attendu {
			if indices[position] != valeur {
				t.Fatalf("MultiSelect = %v, attendu %v", indices, attendu)
			}
		}
	}
	if indices, err := p.MultiSelect("Dépôts", options, nil); err != nil || len(indices) != 0 {
		t.Errorf("« - » = %v, %v", indices, err)
	}
}

func TestLinePrompterFinDeFlux(t *testing.T) {
	c, _ := console()
	p := ui.NewLinePrompter(c, strings.NewReader(""))
	if _, err := p.Ask(ui.Question{Title: "Organisation"}); err != ui.ErrAborted {
		t.Errorf("erreur = %v", err)
	}
}
