package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
	"github.com/charmbracelet/huh"
)

// ErrAborted signale une interruption volontaire (Échap, Ctrl-C).
var ErrAborted = errors.New("interrompu")

// Option est une entrée de menu.
type Option struct {
	Value string
	Label string
}

// Options construit une liste d'options à partir de couples valeur/libellé.
func Options(pairs ...string) []Option {
	options := make([]Option, 0, len(pairs)/2)
	for index := 0; index+1 < len(pairs); index += 2 {
		options = append(options, Option{Value: pairs[index], Label: pairs[index+1]})
	}
	return options
}

// Question décrit une saisie libre.
type Question struct {
	Title      string
	Default    string
	AllowEmpty bool
	Validate   func(string) (string, error) // renvoie la valeur nettoyée
}

// Prompter pose les questions. Deux implémentations existent : l'une interactive
// (listes aux flèches, cases à cocher), l'autre qui échoue en mode script.
type Prompter interface {
	Interactive() bool
	Ask(question Question) (string, error)
	Confirm(title string, defaultValue bool) (bool, error)
	Choose(title string, options []Option, defaultValue string) (string, error)
	MultiSelect(title string, options []Option, selected []bool) ([]int, error)
}

// ------------------------------------------------------------------ interactif

// HuhPrompter pose les questions au terminal.
type HuhPrompter struct{ console *Console }

// NewPrompter construit le questionneur interactif.
func NewPrompter(console *Console) *HuhPrompter { return &HuhPrompter{console: console} }

// Interactive indique que des questions peuvent être posées.
func (p *HuhPrompter) Interactive() bool { return true }

func convertError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrAborted
	}
	return err
}

// Ask demande une valeur libre, en validant la saisie sur place.
func (p *HuhPrompter) Ask(question Question) (string, error) {
	value := question.Default
	title := question.Title
	if question.Default != "" {
		title = fmt.Sprintf("%s (défaut : %s)", title, question.Default)
	}
	cleaned := ""
	input := huh.NewInput().
		Title(title).
		Value(&value).
		Validate(func(raw string) error {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				if question.AllowEmpty {
					cleaned = ""
					return nil
				}
				return errors.New("une valeur est attendue")
			}
			if question.Validate == nil {
				cleaned = trimmed
				return nil
			}
			result, err := question.Validate(trimmed)
			if err != nil {
				return err
			}
			cleaned = result
			return nil
		})
	if err := runForm(input); err != nil {
		return "", err
	}
	return cleaned, nil
}

// Confirm pose une question fermée.
func (p *HuhPrompter) Confirm(title string, defaultValue bool) (bool, error) {
	answer := defaultValue
	field := huh.NewConfirm().Title(title).Affirmative("Oui").Negative("Non").Value(&answer)
	if err := runForm(field); err != nil {
		return false, err
	}
	return answer, nil
}

// visibleRows borne la hauteur d'une liste : au-delà, elle défile.
const visibleRows = 12

// Choose propose une liste à parcourir aux flèches.
func (p *HuhPrompter) Choose(title string, options []Option, defaultValue string) (string, error) {
	choice := defaultValue
	field := huh.NewSelect[string]().
		Title(title).
		Options(huhOptions(options)...).
		Value(&choice)
	if len(options) > visibleRows {
		field = field.Height(visibleRows)
	}
	if err := runForm(field); err != nil {
		return "", err
	}
	return choice, nil
}

// MultiSelect propose une liste de cases à cocher.
func (p *HuhPrompter) MultiSelect(title string, options []Option, selected []bool) ([]int, error) {
	values := []string{}
	for index, option := range options {
		if index < len(selected) && selected[index] {
			values = append(values, option.Value)
		}
	}
	choices := make([]huh.Option[string], 0, len(options))
	for index, option := range options {
		choice := huh.NewOption(option.Label, option.Value)
		if index < len(selected) && selected[index] {
			choice = choice.Selected(true)
		}
		choices = append(choices, choice)
	}
	field := huh.NewMultiSelect[string]().
		Title(title).
		Description("Espace coche · Entrée valide").
		Options(choices...).
		Value(&values)
	if len(choices) > visibleRows {
		field = field.Height(visibleRows)
	}
	if err := runForm(field); err != nil {
		return nil, err
	}

	positions := map[string]int{}
	for index, option := range options {
		positions[option.Value] = index
	}
	indices := make([]int, 0, len(values))
	for _, value := range values {
		if index, found := positions[value]; found {
			indices = append(indices, index)
		}
	}
	sortInts(indices)
	return indices, nil
}

func runForm(field huh.Field) error {
	form := huh.NewForm(huh.NewGroup(field)).WithShowHelp(true).WithTheme(huh.ThemeBase())
	return convertError(form.Run())
}

func huhOptions(options []Option) []huh.Option[string] {
	converted := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		converted = append(converted, huh.NewOption(option.Label, option.Value))
	}
	return converted
}

func sortInts(values []int) {
	for outer := 1; outer < len(values); outer++ {
		for inner := outer; inner > 0 && values[inner-1] > values[inner]; inner-- {
			values[inner-1], values[inner] = values[inner], values[inner-1]
		}
	}
}

// -------------------------------------------------------------- non interactif

// ScriptPrompter refuse toute question : en mode script, une valeur manquante
// est une erreur explicite plutôt qu'une invite restée sans réponse.
type ScriptPrompter struct{}

// Interactive indique qu'aucune question ne peut être posée.
func (p *ScriptPrompter) Interactive() bool { return false }

// Ask échoue systématiquement.
func (p *ScriptPrompter) Ask(question Question) (string, error) {
	return "", valid.Errorf("%s : valeur manquante en mode non interactif.", question.Title)
}

// Confirm échoue systématiquement.
func (p *ScriptPrompter) Confirm(title string, _ bool) (bool, error) {
	return false, valid.Errorf("%s : confirmation impossible en mode non interactif (ajoutez --yes).", title)
}

// Choose échoue systématiquement.
func (p *ScriptPrompter) Choose(title string, _ []Option, _ string) (string, error) {
	return "", valid.Errorf("%s : choix impossible en mode non interactif.", title)
}

// MultiSelect échoue systématiquement.
func (p *ScriptPrompter) MultiSelect(title string, _ []Option, _ []bool) ([]int, error) {
	return nil, valid.Errorf("%s : sélection impossible en mode non interactif.", title)
}
