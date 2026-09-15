package valid

import (
	"strings"
	"time"
)

// Un instant relevé sur GitHub arrive en UTC — « 2026-08-21T22:32:11Z ». Le
// montrer tel quel demanderait à qui enseigne de faire la soustraction de tête,
// et une remise du soir paraîtrait faite le lendemain. Il se met donc à l'heure
// de la machine : c'est dans ce fuseau que l'échéance a été annoncée à la
// classe, et c'est à lui que l'affichage doit répondre.

// Stamp est la forme lisible d'un instant : le jour et l'heure, sans seconde.
// Elle se compare comme elle se lit — deux instants se rangent dans l'ordre de
// leurs chaînes —, ce dont vivent le tri et les filtres.
const Stamp = "2006-01-02 15:04"

// Moment met un instant rendu par GitHub sous sa forme lisible, dans le fuseau
// de la machine. Une valeur vide reste vide : un dépôt qui n'a rien reçu n'a
// pas de date.
//
// Ce qui ne se lit pas est rendu tel quel. Une forme inattendue s'affiche alors
// sans être belle, ce qui vaut mieux que de la faire disparaître.
func Moment(value string) string {
	texte := strings.TrimSpace(value)
	if texte == "" {
		return ""
	}
	moment, err := time.Parse(time.RFC3339, texte)
	if err != nil {
		return texte
	}
	return moment.Local().Format(Stamp)
}

// Day rend le jour d'un instant lisible — « 2026-08-21 ». C'est ce sur quoi
// portent les bornes d'un filtre : « dernier envoi avant le 1er octobre » parle
// de journées, et l'heure de l'envoi n'y change rien.
func Day(value string) string {
	texte := strings.TrimSpace(value)
	if len(texte) > len(DueDate) {
		return texte[:len(DueDate)]
	}
	return texte
}
