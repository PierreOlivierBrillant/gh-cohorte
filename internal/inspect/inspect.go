// Package inspect décide quels fichiers d'un dépôt sont comparés, et sait dire
// pourquoi les autres ne l'ont pas été.
//
// C'est le module dont tout le reste dépend. Comparer trop large coûte le temps
// de calcul et noie le rapport sous les ressemblances d'un squelette que
// personne n'a écrit ; comparer trop étroit laisse passer ce qu'on cherche. Et
// comme régler cela à la main à chaque analyse est fastidieux, un outil qui
// l'exige est un outil qu'on n'utilise pas.
//
// Trois couches y répondent, dans cet ordre, et chacune peut être montrée :
//
//  1. ce qui est écarté d'office — dépendances, verrous, code engendré,
//     binaires — et ne se discute pas ;
//  2. un profil d'inspection, choisi d'un clic et partagé par l'équipe ;
//  3. les ajouts et retraits propres à cette analyse.
//
// Chaque fichier écarté l'est avec un motif et la règle qui a décidé. Un
// rapport qui dirait « 412 fichiers ignorés » sans dire lesquels ni pourquoi
// serait invérifiable, et personne ne saurait s'il faut corriger le profil ou
// le dépôt.
package inspect

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/tokens"
)

// Motifs d'écartement, tels que les trois interfaces les affichent. Ils sont
// décidés ici pour que le terminal et le navigateur disent le même mot.
const (
	Excluded       = "dépendance ou fichier engendré"
	IsBinary       = "fichier binaire"
	TooLarge       = "trop volumineux"
	IsEmpty        = "vide"
	UnknownTongue  = "langage non reconnu"
	OtherTongue    = "langage non retenu"
	OutsideProfile = "hors du profil d'inspection"
	Requested      = "exclusion demandée"
)

// Source est un fichier tel qu'il sort d'une archive.
type Source struct {
	Path    string
	Content []byte
}

// Kept est un fichier retenu pour la comparaison.
type Kept struct {
	// Path est le chemin vu depuis la racine du projet — c'est celui qu'on
	// affiche et celui sur lequel les motifs portent. Raw est celui de
	// l'archive, gardé pour pouvoir retrouver le fichier.
	Path     string `json:"path"`
	Raw      string `json:"raw,omitempty"`
	Language string `json:"language"`
	Bytes    int    `json:"bytes"`
	Content  []byte `json:"-"`
}

// Skipped est un fichier écarté, et ce qui l'a écarté.
type Skipped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	// Rule est le motif qui a répondu, quand c'est un motif qui a décidé.
	// Sans lui, corriger un profil relèverait de la divination.
	Rule string `json:"rule,omitempty"`
}

// Selection est ce qu'une inspection retient d'un dépôt.
type Selection struct {
	// Root est la racine du projet, telle qu'elle a été devinée ou imposée.
	// Elle est rendue pour être montrée : une racine mal devinée explique à
	// elle seule un dépôt qui paraît vide.
	Root    string    `json:"root"`
	Guessed bool      `json:"guessed"`
	Kept    []Kept    `json:"kept"`
	Skipped []Skipped `json:"skipped"`
	// Reasons compte les écartements par motif, pour le dire en une ligne.
	Reasons map[string]int `json:"reasons,omitempty"`
}

// Bytes rend le poids total de ce qui est retenu.
func (s Selection) Bytes() int {
	total := 0
	for _, file := range s.Kept {
		total += file.Bytes
	}
	return total
}

// Summary résume les écartements, du motif le plus fréquent au moins fréquent.
func (s Selection) Summary() []string {
	motifs := make([]string, 0, len(s.Reasons))
	for reason := range s.Reasons {
		motifs = append(motifs, reason)
	}
	sort.Slice(motifs, func(first, second int) bool {
		if s.Reasons[motifs[first]] != s.Reasons[motifs[second]] {
			return s.Reasons[motifs[first]] > s.Reasons[motifs[second]]
		}
		return motifs[first] < motifs[second]
	})
	return motifs
}

// Settings règle une inspection.
type Settings struct {
	// Profile est l'identifiant du profil ; vide ne restreint rien.
	Profile string `json:"profile,omitempty"`
	// Languages remplace les langages du profil. Vide garde les siens.
	Languages []string `json:"languages,omitempty"`
	// Include et Exclude sont les ajouts et retraits de cette analyse.
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
	// Root impose la racine du projet plutôt que de la laisser deviner.
	Root string `json:"root,omitempty"`
	// NoStrip garde les chemins de l'archive tels quels.
	NoStrip bool `json:"no_strip,omitempty"`
}

// Inspector applique un réglage à des dépôts.
type Inspector struct {
	profile   Profile
	settings  Settings
	languages map[string]bool
	include   []string
	exclude   []string
}

// New prépare une inspection, ou refuse un réglage qui ne peut pas s'appliquer.
// Les profils déclarés par l'organisation s'ajoutent à ceux que l'outil connaît.
func New(settings Settings, declared []Profile) (*Inspector, error) {
	profile, err := FindProfile(Catalog(declared), settings.Profile)
	if err != nil {
		return nil, err
	}

	// Les langages de l'analyse l'emportent sur ceux du profil : c'est une
	// décision prise pour cette fois, et elle doit pouvoir élargir comme
	// restreindre.
	wanted := settings.Languages
	if len(wanted) == 0 {
		wanted = profile.Languages
	}
	wanted, err = ParseLanguages(wanted)
	if err != nil {
		return nil, err
	}
	languages := map[string]bool{}
	for _, language := range wanted {
		languages[language] = true
	}

	return &Inspector{
		profile: profile, settings: settings, languages: languages,
		include: append(append([]string{}, profile.Include...), settings.Include...),
		exclude: append(append([]string{}, profile.Exclude...), settings.Exclude...),
	}, nil
}

// Profile rend le profil retenu, pour que l'interface puisse le nommer.
func (i *Inspector) Profile() Profile { return i.profile }

// Select trie les fichiers d'un dépôt.
func (i *Inspector) Select(sources []Source) Selection {
	selection := Selection{Reasons: map[string]int{}}
	selection.Root, selection.Guessed = i.root(sources)

	for _, source := range sources {
		name := source.Path
		if !i.settings.NoStrip {
			name = Strip(selection.Root, source.Path)
		}
		if reason, rule := i.reject(name, source.Content); reason != "" {
			selection.Skipped = append(selection.Skipped,
				Skipped{Path: name, Reason: reason, Rule: rule})
			selection.Reasons[reason]++
			continue
		}
		language, _ := tokens.Detect(name)
		selection.Kept = append(selection.Kept, Kept{
			Path: name, Raw: source.Path, Language: language.ID,
			Bytes: len(source.Content), Content: source.Content,
		})
	}

	sort.Slice(selection.Kept, func(first, second int) bool {
		return selection.Kept[first].Path < selection.Kept[second].Path
	})
	sort.Slice(selection.Skipped, func(first, second int) bool {
		return selection.Skipped[first].Path < selection.Skipped[second].Path
	})
	return selection
}

// root arrête la racine du projet.
func (i *Inspector) root(sources []Source) (string, bool) {
	if i.settings.NoStrip {
		return "", false
	}
	if i.settings.Root != "" {
		return strings.Trim(i.settings.Root, "/"), false
	}
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		paths = append(paths, source.Path)
	}
	return Root(paths), true
}

// reject dit pourquoi un fichier est écarté, ou rend un motif vide.
//
// L'ordre des épreuves n'est pas indifférent : ce qui se décide sur le nom
// passe avant ce qui demande de lire le contenu, et ce qui est écarté d'office
// passe avant le profil. Un « node_modules » ne doit pas être annoncé comme
// « hors du profil » : ce n'est pas le profil qui l'a écarté, et corriger le
// profil n'y changerait rien.
func (i *Inspector) reject(name string, content []byte) (string, string) {
	if rule, matched := Matches(defaultPatterns(), name); matched {
		return Excluded, rule
	}
	if rule, matched := Matches(i.exclude, name); matched {
		if _, fromProfile := Matches(i.profile.Exclude, name); fromProfile {
			return OutsideProfile, rule
		}
		return Requested, rule
	}
	if BinaryName(name) {
		return IsBinary, ""
	}
	if len(content) == 0 {
		return IsEmpty, ""
	}
	if len(content) > MaxFileBytes {
		return TooLarge, ""
	}
	if Binary(content) {
		return IsBinary, ""
	}

	language, known := tokens.Detect(name)
	if !known {
		return UnknownTongue, ""
	}
	if len(i.languages) > 0 && !i.languages[language.ID] {
		return OtherTongue, language.Label
	}
	if len(i.include) > 0 {
		if _, matched := Matches(i.include, name); !matched {
			return OutsideProfile, ""
		}
	}
	return "", ""
}
