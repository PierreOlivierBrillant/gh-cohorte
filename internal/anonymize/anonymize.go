// Package anonymize retire d'un travail ce qui nomme son auteur.
//
// Un collègue d'un autre collège, ou qui n'utilise pas l'outil, ne peut pas
// publier d'index : le seul chemin praticable est qu'il envoie les copies
// elles-mêmes. Mais des copies d'étudiants ne s'envoient pas telles quelles —
// ce sont des travaux nominatifs, et rien n'oblige à livrer des noms pour
// comparer du code.
//
// Ce paquet fabrique donc un jeu de copies dont les noms ont disparu, et une
// table de correspondance qui reste chez celui qui l'a produit. Deux règles
// tiennent tout le reste :
//
//   - Le remplacement conserve la longueur. Ce n'est pas cosmétique : un
//     remplacement plus court ou plus long déplacerait tout ce qui suit dans le
//     fichier, et les fragments communs ne tomberaient plus au même endroit des
//     deux côtés. Une anonymisation qui fausserait la mesure ne servirait à
//     rien.
//   - Ce qu'on n'a pas su effacer se dit. Une anonymisation n'est jamais
//     complète — un prénom au détour d'un commentaire en français, une capture
//     d'écran, un chemin absolu. Promettre le contraire serait le pire service
//     à rendre ; montrer ce qui reste avant d'écrire est le seul honnête.
package anonymize

import (
	"crypto/rand"
	"math/big"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
	"golang.org/x/text/unicode/norm"
)

// TokenLength est la longueur de la base d'un jeton. Six caractères d'un
// alphabet de trente-deux font un milliard de possibilités : de quoi ne jamais
// voir deux étudiants d'un même envoi porter le même jeton.
const TokenLength = 6

// alphabet écarte deux choses. Ce qui se confond à la lecture d'abord — ni I ni
// 1, ni O ni 0 —, parce qu'un jeton se recopie parfois à la main, d'un courriel
// à un tableur. Les voyelles ensuite : sans elles, aucune suite de six
// caractères ne peut former un mot, et un jeton qu'on lit dans un rapport ou
// qu'on écrit à un collègue ne dira jamais rien de fâcheux par accident.
//
// Vingt-huit caractères sur six positions font quatre cent quatre-vingts
// millions de possibilités : la contrainte ne coûte rien.
const alphabet = "BCDFGHJKLMNPQRSTVWXZ23456789"

// MinPartLength est la longueur en deçà de laquelle un fragment de nom n'est
// pas cherché. « Li » ou « Ana » se trouveraient au milieu de cent mots.
const MinPartLength = 4

// Identity est ce qu'il faut effacer d'une copie.
type Identity struct {
	// Work est l'identifiant de la copie chez nous — le nom du dépôt.
	Work     string
	Person   roster.Person
	Origin   string
	HandedIn string
	// Token impose le jeton plutôt que d'en tirer un.
	//
	// Il sert quand une copie a déjà voyagé sous un jeton : celui d'un index
	// publié, par exemple. Renvoyer la même copie sous un autre jeton
	// obligerait celui qui la reçoit à deviner qu'il s'agit de la même, et il
	// ne le pourrait pas — c'est tout le propos d'un jeton opaque.
	Token string
}

// Token est ce qui remplace une personne.
type Token struct {
	// Base est la forme courte du jeton. Les occurrences plus longues qu'elle
	// la répètent : « A7F3K2 » puis « A7F3K2A7 ». Elles se reconnaissent donc
	// à leur préfixe, et la table n'a qu'une entrée à porter.
	Base string `json:"token"`
	// Work, Origin et HandedIn disent d'où la copie vient. Ils restent ici,
	// jamais dans le ZIP.
	Work     string `json:"work"`
	Origin   string `json:"origin,omitempty"`
	HandedIn string `json:"handed_in,omitempty"`
	FullName string `json:"full_name,omitempty"`
	Username string `json:"username,omitempty"`
}

// Person rend la personne derrière un jeton.
func (t Token) Person() roster.Person {
	return roster.Person{FullName: t.FullName, Username: t.Username}
}

// Options règle une anonymisation.
type Options struct {
	// Parts cherche aussi les fragments de noms pris isolément — le nom de
	// famille seul, le prénom seul.
	//
	// C'est décoché par défaut, et ce n'est pas de la timidité : « Côté » est
	// un nom de famille courant et un mot français ordinaire. Le remplacer
	// partout abîmerait « le côté gauche » dans un commentaire, et pourrait
	// même créer de la ressemblance là où il n'y en avait pas. Ce qui n'est pas
	// remplacé est de toute façon signalé au contrôle : mieux vaut le montrer
	// et laisser décider.
	Parts bool
	// Seed fixe le tirage des jetons, pour que deux exécutions d'une épreuve
	// donnent le même résultat. Zéro tire au hasard.
	Seed int64
}

// Anonymizer efface les identités d'un envoi entier.
//
// Il les porte toutes à la fois, et non une par copie. C'est nécessaire : un
// travail qui nomme un camarade doit voir ce nom remplacé lui aussi — et par
// le jeton de ce camarade, ce qui rend la collusion visible au lieu de
// l'effacer.
type Anonymizer struct {
	tokens  map[string]Token // copie → jeton
	needles []needle
	// parts porte les fragments de noms — le prénom seul, le nom de famille
	// seul — mis à plat. Ils ne sont pas remplacés par défaut, mais le contrôle
	// les cherche pour signaler ce qui survit.
	parts   []string
	options Options
}

// needle est une chaîne à chercher, et ce qui la remplace.
type needle struct {
	folded []rune
	token  string
	// what dit ce qui a été reconnu, pour le rapporter.
	what string
	work string
}

// New prépare l'anonymisation d'un envoi.
func New(identities []Identity, options Options) (*Anonymizer, error) {
	anonymizer := &Anonymizer{tokens: map[string]Token{}, options: options}
	pris := map[string]bool{}
	tirage := newDraw(options.Seed)

	for _, identity := range identities {
		if strings.TrimSpace(identity.Work) == "" {
			return nil, valid.Errorf("Anonymisation : une copie sans identifiant.")
		}
		base, err := chosen(identity.Token, pris)
		if err != nil {
			return nil, err
		}
		if base == "" {
			if base, err = tirage(pris); err != nil {
				return nil, err
			}
		}
		anonymizer.tokens[identity.Work] = Token{
			Base: base, Work: identity.Work, Origin: identity.Origin,
			HandedIn: identity.HandedIn,
			FullName: identity.Person.FullName, Username: identity.Person.Username,
		}
		anonymizer.needles = append(anonymizer.needles,
			needlesOf(identity, base, options.Parts)...)
		anonymizer.parts = append(anonymizer.parts, partsOf(identity.Person.FullName)...)
	}
	sort.Strings(anonymizer.parts)
	anonymizer.parts = slices.Compact(anonymizer.parts)

	// Les chaînes les plus longues passent d'abord : sans cela, « emilie » dans
	// « emilie-cote » serait remplacé en premier, et le reste du nom
	// survivrait au milieu d'un jeton.
	sort.SliceStable(anonymizer.needles, func(first, second int) bool {
		return len(anonymizer.needles[first].folded) > len(anonymizer.needles[second].folded)
	})
	return anonymizer, nil
}

// Tokens rend la table de correspondance.
func (a *Anonymizer) Tokens() []Token {
	table := make([]Token, 0, len(a.tokens))
	for _, token := range a.tokens {
		table = append(table, token)
	}
	sort.Slice(table, func(first, second int) bool {
		return table[first].Work < table[second].Work
	})
	return table
}

// TokenOf rend le jeton d'une copie.
func (a *Anonymizer) TokenOf(work string) (Token, bool) {
	token, connu := a.tokens[work]
	return token, connu
}

// Hit est une occurrence remplacée.
type Hit struct {
	// What dit ce qui a été reconnu — « nom complet », « compte GitHub ».
	What string `json:"what"`
	// Work est la copie que cette occurrence nommait. Elle n'est pas forcément
	// celle du fichier : un travail qui nomme un camarade est justement ce
	// qu'on veut voir.
	Work  string `json:"work"`
	Count int    `json:"count"`
}

// Scrub remplace dans un texte tout ce qui nomme quelqu'un.
//
// Le texte est parcouru en runes et non en octets : un accent tient sur deux
// octets, et remplacer « Côté » par quatre octets en couperait un en deux. La
// longueur conservée est donc celle qu'on lit, ce qui est aussi ce qui garde
// les colonnes d'un fichier là où elles étaient.
func (a *Anonymizer) Scrub(text string) (string, []Hit) {
	runes := []rune(text)
	folded := foldRunes(runes)
	// pris marque ce qui a déjà été remplacé : une occurrence plus longue a la
	// priorité, et ce qui tombe dedans ne doit pas être remplacé une seconde
	// fois.
	pris := make([]bool, len(runes))
	compte := map[Hit]int{}

	for _, needle := range a.needles {
		if len(needle.folded) == 0 || len(needle.folded) > len(folded) {
			continue
		}
		for index := 0; index+len(needle.folded) <= len(folded); index++ {
			if !matchAt(folded, needle.folded, index) || overlaps(pris, index, len(needle.folded)) {
				continue
			}
			remplacement := stretch(needle.token, len(needle.folded))
			copy(runes[index:], remplacement)
			for offset := range needle.folded {
				pris[index+offset] = true
			}
			compte[Hit{What: needle.what, Work: needle.work}]++
			index += len(needle.folded) - 1
		}
	}

	hits := make([]Hit, 0, len(compte))
	for hit, count := range compte {
		hit.Count = count
		hits = append(hits, hit)
	}
	sort.Slice(hits, func(first, second int) bool {
		if hits[first].Work != hits[second].Work {
			return hits[first].Work < hits[second].Work
		}
		return hits[first].What < hits[second].What
	})
	return string(runes), hits
}

// ScrubPath anonymise un chemin, niveau par niveau.
func (a *Anonymizer) ScrubPath(path string) (string, []Hit) {
	return a.Scrub(path)
}

// ------------------------------------------------------------- les chaînes

// needlesOf dresse ce qu'il faut chercher pour effacer une personne.
//
// Les variantes viennent de ce qu'on voit vraiment dans des travaux : le nom
// tel quel, le nom sans accents, le nom collé, le nom avec un point, un tiret
// ou un souligné, le compte GitHub, le matricule, l'adresse de courriel, et le
// nom du dépôt lui-même.
func needlesOf(identity Identity, token string, parts bool) []needle {
	found := make([]needle, 0, 16)
	ajouter := func(value, what string) {
		value = strings.TrimSpace(value)
		if len([]rune(value)) < MinPartLength {
			return
		}
		found = append(found, needle{
			folded: foldRunes([]rune(value)), token: token,
			what: what, work: identity.Work,
		})
	}

	ajouter(identity.Work, "nom du dépôt")
	for _, compte := range identity.Person.Accounts() {
		ajouter(compte, "compte GitHub")
	}
	ajouter(identity.Person.StudentID, "matricule")

	nom := strings.TrimSpace(identity.Person.FullName)
	if nom == "" {
		return dedupe(found)
	}
	ajouter(nom, "nom complet")
	mots := strings.Fields(nom)
	if len(mots) > 1 {
		// « Émilie Côté » s'écrit aussi « Côté Émilie » dans une liste de
		// classe, et « emilie.cote », « emilie_cote », « EmilieCote » dans un
		// paquet Java ou un nom de dossier.
		ajouter(strings.Join(reversed(mots), " "), "nom complet")
		for _, liant := range []string{".", "-", "_", ""} {
			ajouter(strings.Join(mots, liant), "nom complet")
			ajouter(strings.Join(reversed(mots), liant), "nom complet")
		}
		ajouter(string([]rune(mots[0])[0])+mots[len(mots)-1], "nom complet")
	}
	if parts {
		for _, mot := range mots {
			ajouter(mot, "fragment de nom")
		}
	}
	return dedupe(found)
}

// dedupe retire les variantes qui se répètent : « emilie cote » composé avec un
// liant vide et « emiliecote » sont la même chaîne.
func dedupe(found []needle) []needle {
	vues := map[string]bool{}
	gardes := make([]needle, 0, len(found))
	for _, item := range found {
		cle := string(item.folded)
		if vues[cle] {
			continue
		}
		vues[cle] = true
		gardes = append(gardes, item)
	}
	return gardes
}

// partsOf rend les fragments d'un nom complet, mis à plat.
func partsOf(fullName string) []string {
	fragments := make([]string, 0, 3)
	for _, mot := range strings.Fields(fullName) {
		if aplati := string(foldRunes([]rune(mot))); len(aplati) >= MinPartLength {
			fragments = append(fragments, aplati)
		}
	}
	return fragments
}

func reversed(words []string) []string {
	inverse := make([]string, len(words))
	for index, word := range words {
		inverse[len(words)-1-index] = word
	}
	return inverse
}

// ------------------------------------------------------------------ outils

// foldRunes met un texte à plat : minuscules, accents retirés, une rune pour
// une rune.
//
// Le rapport de un à un est ce qui rend tout le reste possible : c'est lui qui
// permet de chercher dans la forme aplatie et de remplacer dans l'originale
// aux mêmes positions.
func foldRunes(runes []rune) []rune {
	folded := make([]rune, len(runes))
	for index, char := range runes {
		folded[index] = fold(char)
	}
	return folded
}

func fold(char rune) rune {
	if char < unicode.MaxASCII {
		return unicode.ToLower(char)
	}
	// La décomposition sépare la lettre de son accent ; la première rune est la
	// lettre de base. « é » donne « e », « ç » donne « c ».
	decomposed := []rune(norm.NFD.String(string(char)))
	if len(decomposed) == 0 {
		return unicode.ToLower(char)
	}
	return unicode.ToLower(decomposed[0])
}

func matchAt(haystack, needle []rune, index int) bool {
	for offset, char := range needle {
		if haystack[index+offset] != char {
			return false
		}
	}
	return true
}

func overlaps(taken []bool, index, length int) bool {
	for offset := 0; offset < length; offset++ {
		if taken[index+offset] {
			return true
		}
	}
	return false
}

// stretch étire un jeton à la longueur voulue, en le répétant puis en le
// coupant. Deux occurrences de longueurs différentes partagent donc leur
// préfixe, et la table n'a qu'une base à retenir.
func stretch(token string, length int) []rune {
	base := []rune(token)
	stretched := make([]rune, length)
	for index := range stretched {
		stretched[index] = base[index%len(base)]
	}
	return stretched
}

// chosen retient un jeton imposé, ou rend une chaîne vide s'il n'y en a pas.
func chosen(token string, pris map[string]bool) (string, error) {
	token = strings.ToUpper(strings.TrimSpace(token))
	if token == "" {
		return "", nil
	}
	if pris[token] {
		return "", valid.Errorf(
			"Anonymisation : le jeton « %s » est demandé deux fois.", token)
	}
	for _, char := range token {
		if !strings.ContainsRune(alphabet, char) {
			return "", valid.Errorf(
				"Anonymisation : « %s » n'est pas un jeton de cet outil.", token)
		}
	}
	pris[token] = true
	return token, nil
}

// newDraw rend un tireur de jetons. Une graine fixe le tirage, pour qu'une
// épreuve donne deux fois le même résultat.
func newDraw(seed int64) func(map[string]bool) (string, error) {
	état := seed
	return func(pris map[string]bool) (string, error) {
		for essai := 0; essai < 1000; essai++ {
			var base strings.Builder
			for index := 0; index < TokenLength; index++ {
				rang, err := pick(&état, len(alphabet))
				if err != nil {
					return "", err
				}
				base.WriteByte(alphabet[rang])
			}
			if jeton := base.String(); !pris[jeton] {
				pris[jeton] = true
				return jeton, nil
			}
		}
		return "", valid.Errorf(
			"Anonymisation : impossible de tirer un jeton libre après mille essais.")
	}
}

// pick tire un rang. Sans graine, c'est le hasard du système ; avec une graine,
// une suite reproductible — elle ne sert qu'aux épreuves, et un jeton prévisible
// n'y porte aucun renseignement.
func pick(state *int64, bound int) (int, error) {
	if *state == 0 {
		rang, err := rand.Int(rand.Reader, big.NewInt(int64(bound)))
		if err != nil {
			return 0, valid.Errorf("Anonymisation : tirage impossible (%v).", err)
		}
		return int(rang.Int64()), nil
	}
	*state = (*state*6364136223846793005 + 1442695040888963407) & 0x7fffffffffffffff
	return int((*state >> 33) % int64(bound)), nil
}
