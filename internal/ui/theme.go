package ui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Couleurs des questions. Elles sont dites en toutes lettres plutôt que reprises
// de la palette du terminal : le « bleu » d'un thème clair y est souvent délavé
// et son « gris » aussi sombre que le texte ordinaire, si bien que la ligne
// retenue s'y perdait. Chacune est choisie pour rester lisible sur fond clair
// comme sur fond sombre — le blanc sur l'accent le reste largement, et les
// couleurs de texte gardent de quoi se détacher des deux côtés.
//
// Suivre le fond de plus près demanderait de le demander au terminal ; la
// question part alors au milieu des touches que lit le formulaire, et sans
// réponse elle fait patienter cinq secondes pour conclure « sombre ».
const (
	accentColor     = lipgloss.Color("#2557D6") // le fond de ce qui est retenu
	onAccentColor   = lipgloss.Color("#FFFFFF") // le texte posé dessus
	accentTextColor = lipgloss.Color("#3B72F0") // l'accent porté par du texte
	checkedColor    = lipgloss.Color("#1E9E57") // ce qui est coché
	mutedColor      = lipgloss.Color("#7C8496") // ce qui n'est pas retenu
	failureColor    = lipgloss.Color("#E5484D") // un refus de saisie
)

// Marques de la sélection. Elles la portent à elles seules quand la couleur
// manque — NO_COLOR, terminal monochrome, capture en noir et blanc — d'où le
// chevron devant le bouton retenu et l'espace de même largeur devant l'autre.
const (
	cursorMark        = "▸ "
	chosenButtonMark  = "▸"
	blurredButtonMark = " "
	checkedMark       = "[✓] "
	uncheckedMark     = "[ ] "
)

// promptTheme habille les questions pour que la réponse retenue saute aux yeux :
// la ligne sous le curseur est surlignée et précédée d'un chevron, le bouton
// retenu d'une question fermée est plein quand l'autre reste éteint.
func promptTheme() *huh.Theme {
	theme := huh.ThemeBase()
	focused := &theme.Focused

	highlight := lipgloss.NewStyle().Bold(true).Foreground(onAccentColor).Background(accentColor)
	button := lipgloss.NewStyle().Padding(0, 2).MarginRight(1)

	focused.Base = focused.Base.BorderForeground(accentTextColor)
	focused.Card = focused.Base
	focused.Title = focused.Title.Bold(true).Foreground(accentTextColor)
	focused.Description = focused.Description.Foreground(mutedColor)
	focused.ErrorIndicator = focused.ErrorIndicator.Foreground(failureColor)
	focused.ErrorMessage = focused.ErrorMessage.Foreground(failureColor)

	// Liste à choix unique : chevron et surlignage désignent la même ligne,
	// celle qu'« entrée » retiendra. Les autres gardent la couleur du terminal
	// plutôt qu'un gris qui les rendrait difficiles à lire.
	focused.SelectSelector = highlight.SetString(cursorMark)
	focused.SelectedOption = highlight
	focused.UnselectedOption = lipgloss.NewStyle()
	focused.NextIndicator = focused.NextIndicator.Foreground(accentTextColor)
	focused.PrevIndicator = focused.PrevIndicator.Foreground(accentTextColor)

	// Question fermée : le bouton retenu est plein et bordé d'un trait épais,
	// l'autre n'est qu'un contour. Les deux occupent la même place, si bien que
	// passer de l'un à l'autre ne déplace rien.
	focused.FocusedButton = button.
		SetString(chosenButtonMark).
		Bold(true).
		Foreground(onAccentColor).
		Background(accentColor).
		Border(lipgloss.ThickBorder()).
		BorderForeground(accentColor)
	focused.BlurredButton = button.
		SetString(blurredButtonMark).
		Foreground(mutedColor).
		Border(lipgloss.NormalBorder()).
		BorderForeground(mutedColor)

	focused.TextInput.Prompt = focused.TextInput.Prompt.Foreground(accentTextColor)
	focused.TextInput.Cursor = focused.TextInput.Cursor.Foreground(accentTextColor)
	focused.TextInput.Placeholder = focused.TextInput.Placeholder.Foreground(mutedColor)

	copyToBlurred(theme)
	return theme
}

// multiSelectTheme reprend promptTheme pour les cases à cocher, où « coché » et
// « sous le curseur » sont deux états distincts : huh surligne les lignes
// cochées avec le style de la ligne retenue, ce qui brouillerait les deux. La
// croix verte marque donc ce qui est coché, et le chevron garde le curseur.
func multiSelectTheme() *huh.Theme {
	theme := promptTheme()
	focused := &theme.Focused

	focused.MultiSelectSelector = focused.SelectSelector
	focused.SelectedPrefix = lipgloss.NewStyle().Bold(true).Foreground(checkedColor).SetString(checkedMark)
	focused.SelectedOption = lipgloss.NewStyle().Bold(true).Foreground(checkedColor)
	focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(mutedColor).SetString(uncheckedMark)
	focused.UnselectedOption = lipgloss.NewStyle()

	copyToBlurred(theme)
	return theme
}

// copyToBlurred reporte les styles sur l'état non focalisé — un formulaire ne
// pose ici qu'une question à la fois, mais huh dessine les deux états. Le champ
// en attente perd sa barre latérale, son chevron et ses indicateurs de défilement.
func copyToBlurred(theme *huh.Theme) {
	theme.Blurred = theme.Focused
	theme.Blurred.Base = theme.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	theme.Blurred.Card = theme.Blurred.Base
	theme.Blurred.MultiSelectSelector = lipgloss.NewStyle().SetString("  ")
	theme.Blurred.NextIndicator = lipgloss.NewStyle()
	theme.Blurred.PrevIndicator = lipgloss.NewStyle()
}
