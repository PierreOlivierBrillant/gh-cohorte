package naming

import (
	"strconv"
	"strings"
)

// Le nom court d'une session dit la saison et l'année : « a26 », « h27 ». La
// convention est celle des collèges du Québec, où l'année scolaire commence à
// l'automne.
//
// L'ordre de cette liste est celui où les saisons se suivent au fil d'une
// année civile. L'année scolaire commence bien à l'automne, mais le nom court
// porte l'année civile : « h26 » précède « a26 » de six mois, il ne le suit pas
// d'un an.
var saisons = []struct {
	lettre byte
	nom    string
}{
	{'h', "Hiver"},
	{'p', "Printemps"},
	{'e', "Été"},
	{'a', "Automne"},
}

// SessionLabel déduit le nom long d'une session de son nom court : « a26 »
// donne « Automne 2026 ». Un nom court qui ne suit pas la convention est rendu
// tel quel — rien n'oblige à la suivre, et l'inventer serait pire que se taire.
func SessionLabel(short string) string {
	rang, annee, suit := readSession(short)
	if !suit {
		return strings.TrimSpace(short)
	}
	return saisons[rang].nom + " " + strconv.Itoa(2000+annee)
}

// CompareSessions range deux noms courts de session de la plus récente à la
// plus ancienne : l'année d'abord, la saison ensuite — automne, été,
// printemps, hiver. C'est la session en cours qu'on ouvre, pas celle d'il y a
// trois ans : elle doit tomber sous les yeux la première. Un nom court qui ne
// suit pas la convention n'a pas de place dans cette suite : il passe après,
// avec ses semblables, par ordre alphabétique.
func CompareSessions(first, second string) int {
	rangFirst, anneeFirst, suitFirst := readSession(first)
	rangSecond, anneeSecond, suitSecond := readSession(second)
	switch {
	case suitFirst && !suitSecond:
		return -1
	case !suitFirst && suitSecond:
		return 1
	case !suitFirst && !suitSecond:
		return strings.Compare(strings.ToLower(strings.TrimSpace(first)),
			strings.ToLower(strings.TrimSpace(second)))
	case anneeFirst != anneeSecond:
		return compare(anneeSecond, anneeFirst)
	default:
		return compare(rangSecond, rangFirst)
	}
}

// readSession découpe un nom court en saison et en année. Le rang rendu est
// celui de la saison dans l'année ; il ne veut rien dire si la convention n'est
// pas suivie.
func readSession(short string) (rang, annee int, suit bool) {
	short = strings.TrimSpace(short)
	if len(short) != 3 {
		return 0, 0, false
	}
	rang = -1
	for index, saison := range saisons {
		if saison.lettre == lower(short[0]) {
			rang = index
			break
		}
	}
	if rang < 0 {
		return 0, 0, false
	}
	annee, err := strconv.Atoi(short[1:])
	if err != nil {
		return 0, 0, false
	}
	return rang, annee, true
}

func compare(first, second int) int {
	if first < second {
		return -1
	}
	if first > second {
		return 1
	}
	return 0
}

func lower(letter byte) byte {
	if letter >= 'A' && letter <= 'Z' {
		return letter + ('a' - 'A')
	}
	return letter
}
