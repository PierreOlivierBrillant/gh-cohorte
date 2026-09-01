package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// vue rend un champ tel qu'il s'affichera, mais sans couleur : c'est ce qu'il
// reste sur un terminal monochrome, où seules les marques distinguent la
// réponse retenue.
func vue(field huh.Field, theme *huh.Theme) string {
	field.WithTheme(theme)
	field.Init()
	field.Focus()
	return field.View()
}

func TestLeBoutonRetenuPorteUneMarqueQueLautreNaPas(t *testing.T) {
	oui := true
	champ := huh.NewConfirm().Title("Créer les dépôts ?").
		Affirmative("Oui").Negative("Non").Value(&oui)

	rendu := vue(champ, promptTheme())
	if !strings.Contains(rendu, chosenButtonMark+" Oui") {
		t.Errorf("« Oui » doit porter la marque du bouton retenu :\n%s", rendu)
	}
	if strings.Contains(rendu, chosenButtonMark+" Non") {
		t.Errorf("« Non » ne doit pas la porter :\n%s", rendu)
	}
}

func TestLesDeuxBoutonsOccupentLaMemePlace(t *testing.T) {
	theme := promptTheme()
	retenu := theme.Focused.FocusedButton.Render("Oui")
	autre := theme.Focused.BlurredButton.Render("Oui")
	if lipgloss.Width(retenu) != lipgloss.Width(autre) {
		t.Errorf("basculer d'un bouton à l'autre ne doit rien déplacer : %d ≠ %d",
			lipgloss.Width(retenu), lipgloss.Width(autre))
	}
	if lipgloss.Height(retenu) != lipgloss.Height(autre) {
		t.Errorf("hauteurs différentes : %d ≠ %d", lipgloss.Height(retenu), lipgloss.Height(autre))
	}
}

func TestLaLigneSousLeCurseurEstLaSeuleMarquee(t *testing.T) {
	choix := "gerer"
	champ := huh.NewSelect[string]().Title("Que faire ?").Options(
		huh.NewOption("Créer des dépôts", "creer"),
		huh.NewOption("Gérer un groupe", "gerer"),
		huh.NewOption("Options avancées", "avance"),
	).Value(&choix)

	rendu := vue(champ, promptTheme())
	marquees := 0
	for _, ligne := range strings.Split(rendu, "\n") {
		if strings.Contains(ligne, cursorMark) {
			marquees++
			if !strings.Contains(ligne, "Gérer un groupe") {
				t.Errorf("la marque désigne la mauvaise ligne : %q", ligne)
			}
		}
	}
	if marquees != 1 {
		t.Errorf("%d lignes marquées, une seule est attendue :\n%s", marquees, rendu)
	}
}

func TestLaLigneSousLeCurseurEstSurlignee(t *testing.T) {
	theme := promptTheme()
	if theme.Focused.SelectedOption.GetBackground() != accentColor {
		t.Error("la ligne retenue doit être surlignée, pas seulement marquée")
	}
	if theme.Focused.UnselectedOption.GetBackground() == accentColor {
		t.Error("les autres lignes ne doivent pas l'être")
	}
}

func TestCocheEtCurseurNeSeConfondentPas(t *testing.T) {
	theme := multiSelectTheme()
	focused := theme.Focused

	if !strings.Contains(focused.MultiSelectSelector.String(), strings.TrimSpace(cursorMark)) {
		t.Error("le curseur doit rester marqué par un chevron")
	}
	if !strings.Contains(focused.SelectedPrefix.String(), "✓") {
		t.Error("une case cochée doit se voir sans couleur")
	}
	if strings.Contains(focused.UnselectedPrefix.String(), "✓") {
		t.Error("une case décochée ne doit pas porter de croix")
	}
	// Deux états distincts, deux apparences distinctes : sans quoi une ligne
	// cochée ressemblerait à la ligne sous le curseur.
	if focused.SelectedOption.GetBackground() == focused.MultiSelectSelector.GetBackground() {
		t.Error("le coché ne doit pas se peindre comme le curseur")
	}
	if lipgloss.Width(focused.SelectedPrefix.String()) != lipgloss.Width(focused.UnselectedPrefix.String()) {
		t.Error("cocher une ligne ne doit pas décaler son libellé")
	}
}

func TestLesCouleursNeDependentPasDeLaPaletteDuTerminal(t *testing.T) {
	// Un numéro de palette (« 4 ») vaut ce que le thème du terminal en fait :
	// bleu franc sur l'un, bleu délavé sur l'autre, et la ligne retenue s'y
	// perd. Ce qui distingue la réponse se dit donc en toutes lettres.
	couleurs := map[string]lipgloss.Color{
		"accentColor":     accentColor,
		"onAccentColor":   onAccentColor,
		"accentTextColor": accentTextColor,
		"checkedColor":    checkedColor,
		"mutedColor":      mutedColor,
		"failureColor":    failureColor,
	}
	for nom, couleur := range couleurs {
		if !strings.HasPrefix(string(couleur), "#") {
			t.Errorf("%s = %q : une couleur de palette suit le thème du terminal", nom, string(couleur))
		}
	}
}
