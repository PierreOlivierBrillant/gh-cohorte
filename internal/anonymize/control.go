package anonymize

import (
	"regexp"
	"sort"
	"strings"
)

// Ce qui reste après le passage.
//
// Une anonymisation n'est jamais complète. Un prénom au détour d'un commentaire
// en français, un chemin absolu qui porte le nom du compte du poste, une
// adresse de courriel dans un en-tête, une capture d'écran : rien de tout cela
// ne se traite par un remplacement de chaîne, et prétendre le contraire ferait
// envoyer des noms en croyant n'en envoyer aucun.
//
// Le contrôle passe donc après, et son rôle n'est pas de corriger : c'est de
// montrer. Ce qu'il signale est rendu fichier par fichier avant que le ZIP ne
// soit écrit, et c'est à qui l'envoie de décider.

// Résidus reconnus, tels que les trois interfaces les nomment.
const (
	ResidueMail   = "adresse de courriel"
	ResidueGitHub = "adresse GitHub"
	ResidueHome   = "chemin personnel"
	ResidueAuthor = "mention d'auteur"
	ResidueName   = "fragment de nom connu"
)

// Residue est ce qui ressemble encore à quelqu'un.
type Residue struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Kind string `json:"kind"`
	// Text est ce qui a été vu, tel quel. Il faut bien le montrer : c'est ce
	// qu'on demande de juger.
	Text string `json:"text"`
}

var (
	mailRe   = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]{2,}`)
	githubRe = regexp.MustCompile(`(?i)github\.com[/:]([\w.-]+)`)
	homeRe   = regexp.MustCompile(`(?i)(?:/home/|/Users/|C:\\Users\\)([\w.-]+)`)
	authorRe = regexp.MustCompile(`(?i)(@author|auteur\s*:|cr[ée]{1,2}\s+par|created\s+by|written\s+by)\s*(\S.{0,40})`)
)

// Inspect relève ce qui, dans un fichier anonymisé, nomme encore quelqu'un.
func (a *Anonymizer) Inspect(path, content string) []Residue {
	residues := make([]Residue, 0, 4)
	for numero, ligne := range strings.Split(content, "\n") {
		numero++
		for _, motif := range []struct {
			expression *regexp.Regexp
			kind       string
		}{
			{mailRe, ResidueMail},
			{githubRe, ResidueGitHub},
			{homeRe, ResidueHome},
			{authorRe, ResidueAuthor},
		} {
			for _, trouve := range motif.expression.FindAllString(ligne, -1) {
				residues = append(residues, Residue{
					Path: path, Line: numero, Kind: motif.kind, Text: abbreviate(trouve),
				})
			}
		}
		residues = append(residues, a.survivingNames(path, numero, ligne)...)
	}
	return residues
}

// survivingNames cherche les fragments de noms que le remplacement n'a pas
// touchés — le nom de famille seul, le prénom seul.
//
// Ils ne sont pas remplacés par défaut, et c'est délibéré : « Côté » est un nom
// de famille courant et un mot français ordinaire. Mais leur présence mérite
// d'être vue, et c'est précisément le partage du travail entre le remplacement,
// qui n'ose pas, et le contrôle, qui montre.
func (a *Anonymizer) survivingNames(path string, line int, text string) []Residue {
	if a.options.Parts {
		return nil // ils ont été remplacés : il n'y a plus rien à signaler.
	}
	folded := string(foldRunes([]rune(text)))
	vus := map[string]bool{}
	residues := make([]Residue, 0, 2)
	for _, fragment := range a.parts {
		if vus[fragment] || !strings.Contains(folded, fragment) {
			continue
		}
		vus[fragment] = true
		residues = append(residues, Residue{
			Path: path, Line: line, Kind: ResidueName, Text: fragment,
		})
	}
	return residues
}

// Summary résume les résidus par nature, du plus fréquent au moins fréquent.
func Summary(residues []Residue) map[string]int {
	counts := map[string]int{}
	for _, residue := range residues {
		counts[residue.Kind]++
	}
	return counts
}

// Files rend les fichiers concernés, rangés.
func Files(residues []Residue) []string {
	vus := map[string]bool{}
	chemins := make([]string, 0, 8)
	for _, residue := range residues {
		if !vus[residue.Path] {
			vus[residue.Path] = true
			chemins = append(chemins, residue.Path)
		}
	}
	sort.Strings(chemins)
	return chemins
}

func abbreviate(text string) string {
	const limite = 60
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limite {
		return string(runes)
	}
	return string(runes[:limite-1]) + "…"
}
