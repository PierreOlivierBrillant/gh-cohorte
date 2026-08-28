package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Scripted rejoue une suite de réponses préparées à l'avance. Il sert aux tests
// à dérouler un parcours interactif complet sans terminal.
type Scripted struct {
	Answers []string // réponses consommées dans l'ordre
	Asked   []string // journal des questions posées
	index   int
}

// NewScripted construit un questionneur scripté.
func NewScripted(answers ...string) *Scripted { return &Scripted{Answers: answers} }

// Interactive indique que des questions peuvent être posées.
func (s *Scripted) Interactive() bool { return true }

// Remaining renvoie le nombre de réponses non consommées.
func (s *Scripted) Remaining() int { return len(s.Answers) - s.index }

func (s *Scripted) next(question string) (string, error) {
	s.Asked = append(s.Asked, question)
	if s.index >= len(s.Answers) {
		return "", fmt.Errorf("scénario épuisé à la question « %s »", question)
	}
	answer := s.Answers[s.index]
	s.index++
	return answer, nil
}

// Ask renvoie la prochaine réponse, après validation.
func (s *Scripted) Ask(question Question) (string, error) {
	raw, err := s.next(question.Title)
	if err != nil {
		return "", err
	}
	if raw == "\x03" {
		return "", ErrAborted
	}
	value := strings.TrimSpace(raw)
	if value == "" && question.Default != "" {
		value = question.Default
	}
	if value == "" {
		if question.AllowEmpty {
			return "", nil
		}
		return "", valid.Errorf("%s : une valeur est attendue.", question.Title)
	}
	if question.Validate == nil {
		return value, nil
	}
	return question.Validate(value)
}

// Confirm interprète oui/non.
func (s *Scripted) Confirm(title string, defaultValue bool) (bool, error) {
	raw, err := s.next(title)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "o", "oui", "y", "yes", "true", "1":
		return true, nil
	case "n", "non", "no", "false", "0":
		return false, nil
	case "\x03":
		return false, ErrAborted
	case "":
		return defaultValue, nil
	default:
		return false, fmt.Errorf("réponse « %s » incomprise pour « %s »", raw, title)
	}
}

// Choose accepte la valeur d'une option, son numéro ou un fragment de libellé.
func (s *Scripted) Choose(title string, options []Option, defaultValue string) (string, error) {
	raw, err := s.next(title)
	if err != nil {
		return "", err
	}
	answer := strings.TrimSpace(raw)
	if answer == "\x03" {
		return "", ErrAborted
	}
	if answer == "" {
		return defaultValue, nil
	}
	for _, option := range options {
		if option.Value == answer {
			return option.Value, nil
		}
	}
	if number, err := strconv.Atoi(answer); err == nil && number >= 1 && number <= len(options) {
		return options[number-1].Value, nil
	}
	for _, option := range options {
		if strings.Contains(strings.ToLower(option.Label), strings.ToLower(answer)) {
			return option.Value, nil
		}
	}
	return "", fmt.Errorf("choix « %s » introuvable pour « %s »", answer, title)
}

// MultiSelect accepte les mêmes expressions que le mode non interactif :
// « tous », « 1,3 », « 2-5 », un nom, ou un mélange.
func (s *Scripted) MultiSelect(title string, options []Option, selected []bool) ([]int, error) {
	raw, err := s.next(title)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "\x03" {
		return nil, ErrAborted
	}
	if strings.TrimSpace(raw) == "-" {
		return nil, nil
	}
	return groups.ParseSelection(raw, len(options), func(token string) int {
		for index, option := range options {
			if strings.EqualFold(option.Value, token) || strings.EqualFold(option.Label, token) {
				return index
			}
		}
		return -1
	})
}
