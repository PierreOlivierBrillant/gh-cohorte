package ui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/complete"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
	"github.com/charmbracelet/bubbles/key"
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
	// Complete active la complétion par tabulation : chemins de fichiers,
	// dossiers seulement, ou rien.
	Complete complete.Mode
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
type HuhPrompter struct {
	console *Console
	// input et output ne sont fournis que par les tests, qui jouent une suite
	// de touches sans terminal.
	input  io.Reader
	output io.Writer
}

// NewPrompter construit le questionneur interactif.
func NewPrompter(console *Console) *HuhPrompter { return &HuhPrompter{console: console} }

// NewPrompterWithIO construit un questionneur dont l'entrée et la sortie sont
// fournies : les tests y jouent des touches sans avoir besoin d'un terminal.
func NewPrompterWithIO(console *Console, input io.Reader, output io.Writer) *HuhPrompter {
	return &HuhPrompter{console: console, input: input, output: output}
}

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
// Les questions attendant un chemin se complètent à la tabulation.
func (p *HuhPrompter) Ask(question Question) (string, error) {
	// Une question de chemin a son propre champ : la tabulation y complète, et
	// une seconde tabulation liste les possibilités.
	if question.Complete != complete.None {
		return p.askPath(question)
	}
	input, cleaned := buildInput(question)
	if err := p.runForm(input, promptTheme()); err != nil {
		return "", err
	}
	return *cleaned, nil
}

// buildInput assemble le champ de saisie et le réceptacle de la valeur validée.
func buildInput(question Question) (*huh.Input, *string) {
	value := question.Default
	title := question.Title
	if question.Default != "" {
		title = fmt.Sprintf("%s (défaut : %s)", title, question.Default)
	}
	cleaned := new(string)
	input := huh.NewInput().
		Title(title).
		Value(&value).
		Validate(func(raw string) error {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				if question.AllowEmpty {
					*cleaned = ""
					return nil
				}
				return errors.New("une valeur est attendue")
			}
			if question.Validate == nil {
				*cleaned = trimmed
				return nil
			}
			result, err := question.Validate(trimmed)
			if err != nil {
				return err
			}
			*cleaned = result
			return nil
		})
	return input, cleaned
}

// Confirm pose une question fermée.
func (p *HuhPrompter) Confirm(title string, defaultValue bool) (bool, error) {
	answer := defaultValue
	field := huh.NewConfirm().Title(title).Affirmative("Oui").Negative("Non").Value(&answer)
	if err := p.runForm(field, promptTheme()); err != nil {
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
	if err := p.runForm(field, promptTheme()); err != nil {
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
	if err := p.runForm(field, multiSelectTheme()); err != nil {
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

// runForm affiche un champ unique, avec des raccourcis en français. Le thème
// est celui du champ posé : les cases à cocher ne marquent pas la sélection
// comme les listes à choix unique (voir theme.go).
func (p *HuhPrompter) runForm(field huh.Field, theme *huh.Theme) error {
	form := huh.NewForm(huh.NewGroup(field)).
		WithShowHelp(true).
		WithTheme(theme).
		WithKeyMap(frenchKeyMap())
	if p.input != nil {
		form = form.WithInput(p.input)
	}
	if p.output != nil {
		form = form.WithOutput(p.output)
	}
	return convertError(form.Run())
}

// frenchKeyMap reprend les raccourcis de huh en français. Les questions de
// chemin, elles, ont leur propre champ : voir pathinput.go.
func frenchKeyMap() *huh.KeyMap {
	keys := huh.NewDefaultKeyMap()

	keys.Input.Prev = key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("maj+tab", "retour"))
	keys.Input.Submit = key.NewBinding(key.WithKeys("enter"), key.WithHelp("entrée", "valider"))
	keys.Input.Next = key.NewBinding(key.WithKeys("enter", "tab"), key.WithHelp("entrée", "valider"))
	// Les suggestions de huh ne servent pas : les chemins ont leur propre champ.
	keys.Input.AcceptSuggestion = key.NewBinding(
		key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "compléter"), key.WithDisabled())

	keys.Select.Prev = key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("maj+tab", "retour"))
	keys.Select.Next = key.NewBinding(key.WithKeys("enter", "tab"), key.WithHelp("entrée", "choisir"))
	keys.Select.Submit = key.NewBinding(key.WithKeys("enter"), key.WithHelp("entrée", "choisir"))
	keys.Select.Up = key.NewBinding(
		key.WithKeys("up", "k", "ctrl+k", "ctrl+p"), key.WithHelp("↑", "monter"))
	keys.Select.Down = key.NewBinding(
		key.WithKeys("down", "j", "ctrl+j", "ctrl+n"), key.WithHelp("↓", "descendre"))
	keys.Select.Filter = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filtrer"))
	keys.Select.GotoTop = key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("début", "au début"))
	keys.Select.GotoBottom = key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("fin", "à la fin"))

	keys.MultiSelect.Prev = key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("maj+tab", "retour"))
	keys.MultiSelect.Next = key.NewBinding(key.WithKeys("enter", "tab"), key.WithHelp("entrée", "valider"))
	keys.MultiSelect.Submit = key.NewBinding(key.WithKeys("enter"), key.WithHelp("entrée", "valider"))
	keys.MultiSelect.Toggle = key.NewBinding(key.WithKeys(" ", "x"), key.WithHelp("espace", "cocher"))
	keys.MultiSelect.Up = key.NewBinding(key.WithKeys("up", "k", "ctrl+p"), key.WithHelp("↑", "monter"))
	keys.MultiSelect.Down = key.NewBinding(key.WithKeys("down", "j", "ctrl+n"), key.WithHelp("↓", "descendre"))
	keys.MultiSelect.Filter = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filtrer"))
	keys.MultiSelect.SelectAll = key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "tout cocher"))
	keys.MultiSelect.SelectNone = key.NewBinding(
		key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "tout décocher"), key.WithDisabled())

	keys.Confirm.Prev = key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("maj+tab", "retour"))
	keys.Confirm.Next = key.NewBinding(key.WithKeys("enter", "tab"), key.WithHelp("entrée", "valider"))
	keys.Confirm.Submit = key.NewBinding(key.WithKeys("enter"), key.WithHelp("entrée", "valider"))
	keys.Confirm.Toggle = key.NewBinding(
		key.WithKeys("h", "l", "right", "left"), key.WithHelp("←/→", "basculer"))
	keys.Confirm.Accept = key.NewBinding(key.WithKeys("o", "O", "y", "Y"), key.WithHelp("o", "Oui"))
	keys.Confirm.Reject = key.NewBinding(key.WithKeys("n", "N"), key.WithHelp("n", "Non"))

	return keys
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
