package naming

import (
	"strconv"
	"strings"
)

// Le nom court d'une session dit la saison et l'année : « a26 », « h27 ». La
// convention est celle des collèges du Québec, où l'année scolaire commence à
// l'automne.
var saisons = map[byte]string{
	'a': "Automne",
	'h': "Hiver",
	'e': "Été",
	'p': "Printemps",
}

// SessionLabel déduit le nom long d'une session de son nom court : « a26 »
// donne « Automne 2026 ». Un nom court qui ne suit pas la convention est rendu
// tel quel — rien n'oblige à la suivre, et l'inventer serait pire que se taire.
func SessionLabel(short string) string {
	short = strings.TrimSpace(short)
	if len(short) != 3 {
		return short
	}
	saison, connue := saisons[lower(short[0])]
	if !connue {
		return short
	}
	annee, err := strconv.Atoi(short[1:])
	if err != nil {
		return short
	}
	return saison + " " + strconv.Itoa(2000+annee)
}

func lower(letter byte) byte {
	if letter >= 'A' && letter <= 'Z' {
		return letter + ('a' - 'A')
	}
	return letter
}
