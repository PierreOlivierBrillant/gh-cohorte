package roster

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

// Ce qu'une liste laisse deviner de la place où ses dépôts doivent arriver.
//
// Deux sources, et elles ne disent pas la même chose. Le fichier d'Omnivox
// porte le cours et le groupe dans son nom — « ListeEtudiants_cours4203N5EM_
// gr1040.csv » —, mais ce nom peut avoir été changé. La colonne « Groupe » du
// contenu, elle, suit chaque étudiant : c'est la seule qui sache dire qu'une
// liste en mêle plusieurs, ce qui arrive dès qu'on colle deux exports.

// PlaceHints est ce qu'une liste laisse deviner. Un champ vide veut dire que
// rien ne permettait de le remplir : inventer serait pire que se taire.
type PlaceHints struct {
	// Course est le numéro du cours, « 3N5 » pour 420-3N5-EM.
	Course string `json:"course"`
	// Group est le groupe, quand la liste n'en porte qu'un.
	Group string `json:"group"`
	// Groups nomme les groupes que la liste mêle, quand elle en mêle. Aucun ne
	// peut alors être choisi à la place de quelqu'un.
	Groups []string `json:"groups,omitempty"`
}

// coursDansFichier reconnaît le cours dans le nom d'un export d'Omnivox.
// Un numéro de cours collégial s'écrit « 420-3N5-EM » : trois chiffres de
// discipline, trois caractères de numéro, deux lettres d'établissement. Le nom
// de fichier les colle ; seul le milieu nous intéresse.
var coursDansFichier = regexp.MustCompile(`(?i)cours[0-9]{3}([0-9A-Z]{3})[A-Z]{2}`)

// groupeDansFichier reconnaît le groupe dans le même nom.
var groupeDansFichier = regexp.MustCompile(`(?i)_gr([0-9A-Z]+)`)

// HintsFrom lit ce qu'une liste et son nom de fichier laissent deviner.
//
// Le nom peut être un chemin complet ou le seul nom d'un fichier déposé dans
// une page : seule sa dernière portion compte.
func HintsFrom(filename string, entries []Entry) PlaceHints {
	var indices PlaceHints
	nom := path.Base(strings.ReplaceAll(strings.TrimSpace(filename), `\`, "/"))
	if trouve := coursDansFichier.FindStringSubmatch(nom); trouve != nil {
		indices.Course = strings.ToUpper(trouve[1])
	}

	// Le contenu passe avant le nom du fichier : il suit chaque étudiant, là
	// où le nom peut avoir été changé.
	indices.Groups = Groups(entries)
	switch len(indices.Groups) {
	case 0:
		if trouve := groupeDansFichier.FindStringSubmatch(nom); trouve != nil {
			indices.Group = trouve[1]
		}
	case 1:
		indices.Group = indices.Groups[0]
		indices.Groups = nil
	}
	return indices
}

// Groups rend les groupes que la liste porte, sans doublons et rangés.
func Groups(entries []Entry) []string {
	vus := map[string]bool{}
	var trouves []string
	for _, entree := range entries {
		groupe := strings.TrimSpace(entree.Group)
		if groupe == "" || vus[groupe] {
			continue
		}
		vus[groupe] = true
		trouves = append(trouves, groupe)
	}
	sort.Strings(trouves)
	return trouves
}
