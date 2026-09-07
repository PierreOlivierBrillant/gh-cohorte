package roster

import (
	"sort"
	"strings"
	"unicode"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Rapprocher un compte GitHub d'un étudiant, quand rien ne les relie
// explicitement.
//
// Le problème vient de ce qu'une liste d'Omnivox ne connaît pas les comptes, et
// que les dépôts ne connaissent que ceux-là. Entre les deux, il n'y a que ce
// que la personne a bien voulu mettre dans son pseudonyme — parfois son nom,
// parfois son numéro d'étudiant, parfois rien.
//
// Ce qui suit ne devine pas : il note. Chaque indice vaut un score, le meilleur
// l'emporte, et ce score est rendu avec la raison qui l'a produit pour que la
// personne qui enseigne puisse juger. Un rapprochement noté 100 tient de la
// preuve — un numéro d'étudiant à sept chiffres ne se retrouve pas par hasard
// dans un pseudonyme. Un rapprochement noté 40 est une suggestion, et
// l'interface doit le présenter comme telle.
//
// Rien n'est jamais appliqué sans confirmation : c'est le prix d'une liste qui
// ne dit pas ce qu'elle sait.

// Seuils de lecture d'un score.
const (
	// Certain : l'indice ne peut pas être fortuit.
	Certain = 90
	// Probable : à confirmer d'un coup d'œil, mais rarement faux.
	Probable = 60
	// Faible : une piste, à vérifier une par une.
	Faible = 30
)

// Pairing est un compte GitHub et la personne qu'on lui suppose.
type Pairing struct {
	// Login est le compte tel que les dépôts le portent.
	Login string `json:"login"`
	// Entry est la personne retenue ; son nom est vide si aucune ne convient.
	Entry Entry `json:"entry"`
	// Score va de 0 à 100, et Reason dit ce qui l'a produit.
	Score  int    `json:"score"`
	Reason string `json:"reason"`
	// Ambiguous dit que deux personnes se valaient : aucune n'a été retenue,
	// et Rivals les nomme.
	Ambiguous bool     `json:"ambiguous"`
	Rivals    []string `json:"rivals,omitempty"`
}

// Found dit qu'une personne a été retenue.
func (p Pairing) Found() bool { return strings.TrimSpace(p.Entry.FullName) != "" }

// Match rapproche des comptes GitHub des personnes d'une liste.
//
// Les profils GitHub, quand on les connaît — compte vers nom affiché —, sont
// l'indice le plus sûr après le numéro d'étudiant. Les fournir est facultatif :
// sans eux, seul le pseudonyme parle.
//
// Une personne n'est retenue qu'une fois. Les rapprochements les plus sûrs sont
// attribués d'abord, si bien qu'un indice faible ne prend jamais la place d'un
// indice fort.
func Match(entries []Entry, logins []string, profiles map[string]string) []Pairing {
	var pistes []piste
	for _, login := range logins {
		for index, entree := range entries {
			score, raison := note(login, profiles[strings.ToLower(login)], entree)
			if score > 0 {
				pistes = append(pistes, piste{login, index, score, raison})
			}
		}
	}
	// Du plus sûr au moins sûr ; à égalité, l'ordre de la liste tranche pour
	// que deux exécutions disent la même chose.
	sort.SliceStable(pistes, func(i, j int) bool {
		if pistes[i].score != pistes[j].score {
			return pistes[i].score > pistes[j].score
		}
		if pistes[i].login != pistes[j].login {
			return pistes[i].login < pistes[j].login
		}
		return pistes[i].entree < pistes[j].entree
	})

	retenus := map[string]Pairing{}
	prises := map[int]bool{}
	for _, courante := range pistes {
		if _, deja := retenus[courante.login]; deja || prises[courante.entree] {
			continue
		}
		// Deux personnes qui se valent ne se départagent pas : les nommer vaut
		// mieux que d'en retenir une au hasard.
		if rivales := exaequo(pistes, courante.login, courante.score, courante.entree,
			prises, entries); len(rivales) > 0 {
			retenus[courante.login] = Pairing{
				Login: courante.login, Score: courante.score,
				Reason: courante.raison, Ambiguous: true,
				Rivals: append([]string{entries[courante.entree].FullName}, rivales...),
			}
			continue
		}
		prises[courante.entree] = true
		retenus[courante.login] = Pairing{
			Login: courante.login, Entry: entries[courante.entree],
			Score: courante.score, Reason: courante.raison,
		}
	}

	rapprochements := make([]Pairing, 0, len(logins))
	for _, login := range logins {
		if trouve, ok := retenus[login]; ok {
			rapprochements = append(rapprochements, trouve)
			continue
		}
		rapprochements = append(rapprochements, Pairing{Login: login})
	}
	return rapprochements
}

// piste est un rapprochement possible, avant qu'on ait tranché.
type piste struct {
	login  string
	entree int
	score  int
	raison string
}

// exaequo nomme les personnes qui obtiennent le même score que celle retenue,
// pour ce compte-là. Aucune n'est alors choisie : les nommer vaut mieux que
// d'en retenir une au hasard.
func exaequo(pistes []piste, login string, score, retenue int,
	prises map[int]bool, entries []Entry) []string {
	var rivales []string
	for _, autre := range pistes {
		if autre.login != login || autre.entree == retenue || autre.score != score {
			continue
		}
		if prises[autre.entree] {
			continue
		}
		rivales = append(rivales, entries[autre.entree].FullName)
	}
	return rivales
}

// note pèse ce qui relie un compte à une personne. Le premier indice qui
// s'applique l'emporte : ils sont rangés du plus décisif au plus ténu.
func note(login, profil string, entree Entry) (int, string) {
	compte := fold(login)
	chiffres := digits(login)
	tokens := nameTokens(entree.FullName)
	if len(tokens) == 0 {
		return 0, ""
	}

	// Un numéro d'étudiant dans un pseudonyme n'est pas une coïncidence.
	if numero := digits(entree.StudentID); len(numero) >= 5 && strings.Contains(chiffres, numero) {
		return 100, "numéro d'étudiant dans le compte"
	}
	if perm := fold(entree.Permanent); len(perm) >= 8 && strings.Contains(compte, perm) {
		return 100, "code permanent dans le compte"
	}

	nom := strings.Join(tokens, "")
	if profil != "" && fold(profil) == nom {
		return 95, "nom du profil GitHub"
	}
	if compte == nom {
		return 90, "nom complet dans le compte"
	}

	prenom := tokens[0]
	// Chaque nom de famille est essayé, pas seulement le plus long : « Diego
	// Garcia Rioja » peut se faire appeler « drioja » comme « dgarcia ». Le
	// meilleur indice l'emporte.
	meilleur, raison := 0, ""
	retenir := func(score int, motif string) {
		if score > meilleur {
			meilleur, raison = score, motif
		}
	}
	for _, famille := range tokens[1:] {
		// Les noms de deux ou trois lettres — « el », « de », « van » — se
		// retrouvent partout : les compter mènerait n'importe où.
		if len(famille) < 4 || !strings.Contains(compte, famille) {
			continue
		}
		switch {
		case len(prenom) >= 3 && strings.Contains(compte, prenom):
			retenir(85, "prénom et nom dans le compte")
		case strings.HasPrefix(compte, prenom[:1]):
			retenir(75, "initiale du prénom et nom dans le compte")
		default:
			retenir(60, "nom de famille dans le compte")
		}
	}
	if meilleur == 0 && len(prenom) >= 4 && strings.Contains(compte, prenom) {
		retenir(40, "prénom dans le compte")
		for _, famille := range tokens[1:] {
			if famille != "" && strings.Contains(compte, famille[:1]) {
				retenir(55, "prénom et initiale du nom dans le compte")
			}
		}
	}
	if meilleur > 0 {
		return meilleur, raison
	}

	// Le profil ne dit pas le même nom, mais peut en partager un morceau.
	if profil != "" {
		for _, token := range nameTokens(profil) {
			if len(token) < 4 {
				continue
			}
			for _, attendu := range tokens {
				if token == attendu {
					return 35, "un nom du profil GitHub concorde"
				}
			}
		}
	}
	return 0, ""
}

// nameTokens découpe un nom en mots comparables, accents et traits d'union
// effacés : « Sauvé-Labonté » donne « sauve » et « labonte ».
func nameTokens(name string) []string {
	slug := valid.Slugify(name)
	tokens := make([]string, 0, 4)
	for _, morceau := range strings.Split(slug, "-") {
		if morceau != "" {
			tokens = append(tokens, morceau)
		}
	}
	return tokens
}

// fold ramène un texte à ses seules lettres et chiffres, en minuscules et sans
// accents : c'est sous cette forme que deux écritures d'un même nom se
// ressemblent.
func fold(value string) string {
	return strings.ReplaceAll(valid.Slugify(value), "-", "")
}

// digits ne retient que les chiffres.
func digits(value string) string {
	var suite strings.Builder
	for _, char := range value {
		if unicode.IsDigit(char) {
			suite.WriteRune(char)
		}
	}
	return suite.String()
}
