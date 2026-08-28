package ui

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
)

// LinePrompter pose les questions en texte simple : listes numérotées et
// sélections saisies comme une expression. Il prend le relais quand les listes
// aux flèches sont impossibles — sortie redirigée, terminal réduit — mais qu'une
// personne reste au clavier.
type LinePrompter struct {
	console *Console
	reader  *bufio.Reader
}

// NewLinePrompter construit un questionneur en mode ligne.
func NewLinePrompter(console *Console, input io.Reader) *LinePrompter {
	return &LinePrompter{console: console, reader: bufio.NewReader(input)}
}

// Interactive indique que des questions peuvent être posées.
func (p *LinePrompter) Interactive() bool { return true }

// readLine lit une réponse ; la fin du flux vaut interruption.
func (p *LinePrompter) readLine() (string, error) {
	line, err := p.reader.ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return "", ErrAborted
	}
	return strings.TrimSpace(line), nil
}

func (p *LinePrompter) prompt(text string) {
	fmt.Fprintf(p.console.Out, "  %s ", text)
}

// Ask demande une valeur libre et valide la saisie.
func (p *LinePrompter) Ask(question Question) (string, error) {
	for {
		title := question.Title
		if question.Default != "" {
			title += " [" + question.Default + "]"
		}
		p.prompt(title + " :")
		answer, err := p.readLine()
		if err != nil {
			return "", err
		}
		if answer == "" {
			answer = question.Default
		}
		if answer == "" {
			if question.AllowEmpty {
				return "", nil
			}
			p.console.Failure("Une valeur est attendue.")
			continue
		}
		if question.Validate == nil {
			return answer, nil
		}
		cleaned, err := question.Validate(answer)
		if err != nil {
			p.console.Failure("%v", err)
			continue
		}
		return cleaned, nil
	}
}

// Confirm pose une question fermée.
func (p *LinePrompter) Confirm(title string, defaultValue bool) (bool, error) {
	hint := "[o/N]"
	if defaultValue {
		hint = "[O/n]"
	}
	for {
		p.prompt(title + " " + hint)
		answer, err := p.readLine()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return defaultValue, nil
		case "o", "oui", "y", "yes":
			return true, nil
		case "n", "non", "no":
			return false, nil
		default:
			p.console.Failure("Répondez par « o » ou « n ».")
		}
	}
}

// Choose affiche une liste numérotée et accepte un numéro ou une valeur.
func (p *LinePrompter) Choose(title string, options []Option, defaultValue string) (string, error) {
	if len(options) == 0 {
		return defaultValue, nil
	}
	for {
		p.console.Blank()
		p.console.Print("  " + p.console.Bold(title))
		for index, option := range options {
			marker := " "
			if option.Value == defaultValue {
				marker = "*"
			}
			p.console.Printf("   %s %2d  %s", marker, index+1, option.Label)
		}
		p.prompt("Votre choix (numéro) :")
		answer, err := p.readLine()
		if err != nil {
			return "", err
		}
		if answer == "" && defaultValue != "" {
			return defaultValue, nil
		}
		if number, err := strconv.Atoi(answer); err == nil && number >= 1 && number <= len(options) {
			return options[number-1].Value, nil
		}
		for _, option := range options {
			if strings.EqualFold(option.Value, answer) {
				return option.Value, nil
			}
		}
		p.console.Failure("« %s » ne correspond à aucune entrée (1 à %d).", answer, len(options))
	}
}

// MultiSelect affiche une liste numérotée et accepte une expression :
// « tous », « 1,3 », « 2-5 », un nom, ou un mélange.
func (p *LinePrompter) MultiSelect(title string, options []Option, selected []bool) ([]int, error) {
	if len(options) == 0 {
		return nil, nil
	}
	for {
		p.console.Blank()
		p.console.Print("  " + p.console.Bold(title))
		for index, option := range options {
			p.console.Printf("     %2d  %s", index+1, option.Label)
		}
		p.console.Note("« tous » (défaut), « 1,3 », « 2-5 », un nom, ou un mélange ; « - » pour ne rien choisir.")
		p.prompt("Votre sélection :")
		answer, err := p.readLine()
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(answer) == "-" {
			return nil, nil
		}
		indices, err := groups.ParseSelection(answer, len(options), func(token string) int {
			for index, option := range options {
				if strings.EqualFold(option.Value, token) || strings.EqualFold(option.Label, token) {
					return index
				}
			}
			return -1
		})
		if err != nil {
			p.console.Failure("%v", err)
			continue
		}
		return indices, nil
	}
}
