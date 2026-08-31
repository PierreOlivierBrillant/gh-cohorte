package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

func lie(binding key.Binding, touche string) bool {
	for _, valeur := range binding.Keys() {
		if valeur == touche {
			return true
		}
	}
	return false
}

func TestTabulationValideUneQuestionOrdinaire(t *testing.T) {
	keys := frenchKeyMap()
	if !lie(keys.Input.Next, "tab") || !lie(keys.Input.Next, "enter") {
		t.Error("hors question de chemin, la tabulation valide la saisie")
	}
}

func TestConfirmationAccepteOuiEnFrancais(t *testing.T) {
	keys := frenchKeyMap()
	for _, touche := range []string{"o", "O", "y", "Y"} {
		if !lie(keys.Confirm.Accept, touche) {
			t.Errorf("la touche « %s » doit valoir oui", touche)
		}
	}
	if !lie(keys.Confirm.Reject, "n") {
		t.Error("la touche « n » doit valoir non")
	}
}

func TestAidesClavierEnFrancais(t *testing.T) {
	keys := frenchKeyMap()
	bindings := []key.Binding{
		keys.Input.Next, keys.Input.Prev, keys.Input.AcceptSuggestion,
		keys.Select.Up, keys.Select.Down, keys.Select.Next, keys.Select.Filter,
		keys.MultiSelect.Toggle, keys.MultiSelect.Next, keys.MultiSelect.SelectAll,
		keys.Confirm.Toggle, keys.Confirm.Accept, keys.Confirm.Reject,
	}
	anglais := []string{"next", "back", "toggle", "submit", "select", "up", "down",
		"filter", "complete", "yes", "no"}
	for _, binding := range bindings {
		aide := strings.ToLower(binding.Help().Desc)
		if aide == "" {
			t.Errorf("aide manquante pour %v", binding.Keys())
			continue
		}
		for _, mot := range anglais {
			if aide == mot {
				t.Errorf("aide en anglais : %q", aide)
			}
		}
	}
}
