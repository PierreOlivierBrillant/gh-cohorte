package roster

import (
	"strings"
	"unicode/utf8"
)

// Une liste d'étudiants exportée d'Omnivox n'est pas un CSV ordinaire, et trois
// de ses particularités feraient échouer une lecture naïve.
//
// Son encodage d'abord : Windows-1252, pas UTF-8. « Félix » y tient sur six
// octets, dont un seul pour le « é » — que le lecteur UTF-8 rejette. Ce n'est
// pas un fichier abîmé, c'est celui qu'Excel produit sous Windows depuis
// toujours.
//
// Ses champs ensuite : chacun est écrit « ="valeur" ». C'est une armure contre
// Excel, qui sans elle lirait « 1030 » comme un nombre et perdrait les zéros de
// tête d'un numéro de groupe ou d'un code permanent.
//
// Ses fins de ligne enfin : CRLF. Le lecteur CSV s'en accommode, mais le
// dernier champ d'une ligne qui se termine par un séparateur reste vide, et
// une colonne vide de plus ne doit pas décaler la lecture des en-têtes.

// OmnivoxHelp dit où prendre la liste et comment la régler. Le texte vit dans
// le paquet qui lit ce format, pour que les trois interfaces l'expliquent de la
// même façon.
const OmnivoxHelp = `Dans Léa : Liste des étudiants › Paramètres d'affichage.
  1. Mode d'affichage      : « Pour Excel »
  2. Séparateur            : «  ; »  (point-virgule)
  3. Éléments à inclure    : cochez « Numéro d'étudiant », « Nom de l'étudiant »
                             et « Code permanent ». Décochez le reste.
  4. Visualiser, puis enregistrez le fichier .csv proposé.

Le fichier arrive en Windows-1252 avec des champs « ="…" » : l'outil le lit tel
quel, il n'y a rien à convertir.

Ajoutez-y une colonne « GitHub » si vous connaissez les comptes ; sans elle,
l'outil les rapproche des noms et des numéros d'étudiant, et vous montre chaque
rapprochement avant d'écrire.`

// decode rend un contenu lisible, quel que soit son encodage.
//
// La règle est sûre dans les deux sens : un texte accentué en Windows-1252
// n'est jamais de l'UTF-8 valide — un « é » y est l'octet 0xE9, que l'UTF-8
// n'admet pas seul —, et un texte UTF-8 est lu tel quel. Un fichier tout en
// ASCII se lit pareil des deux façons.
func decode(content []byte) string {
	if utf8.Valid(content) {
		return strings.TrimPrefix(string(content), "\ufeff")
	}
	return fromWindows1252(content)
}

// windows1252High donne les caractères des octets 0x80 à 0x9F, seul endroit où
// Windows-1252 s'écarte de l'ISO-8859-1. Ailleurs, l'octet est le point de code.
// Les cinq trous sont des octets que la table ne définit pas ; ils deviennent le
// caractère de remplacement plutôt que de disparaître en silence.
var windows1252High = [32]rune{
	'€', '�', '‚', 'ƒ', '„', '…', '†', '‡',
	'ˆ', '‰', 'Š', '‹', 'Œ', '�', 'Ž', '�',
	'�', '‘', '’', '“', '”', '•', '–', '—',
	'˜', '™', 'š', '›', 'œ', '�', 'ž', 'Ÿ',
}

// fromWindows1252 relit un contenu octet par octet.
func fromWindows1252(content []byte) string {
	var texte strings.Builder
	texte.Grow(len(content))
	for _, octet := range content {
		switch {
		case octet < 0x80:
			texte.WriteByte(octet)
		case octet < 0xA0:
			texte.WriteRune(windows1252High[octet-0x80])
		default:
			texte.WriteRune(rune(octet))
		}
	}
	return texte.String()
}

// unarmor retire l'armure « ="valeur" » qu'Excel comprend comme du texte.
// Une valeur qui ne la porte pas est rendue telle quelle : le même lecteur sert
// aux listes écrites à la main.
func unarmor(cell string) string {
	cell = strings.TrimSpace(cell)
	if reste, coupe := strings.CutPrefix(cell, `="`); coupe {
		if valeur, ferme := strings.CutSuffix(reste, `"`); ferme {
			return strings.TrimSpace(valeur)
		}
	}
	return cell
}
