package similarity

import (
	"sort"
	"strings"
)

// Comparer, c'est trois choses : écarter ce qui est commun à tout le monde,
// mesurer ce qui reste, et ne montrer que ce qui vaut d'être regardé.
//
// La première est la plus importante et c'est celle que les outils génériques
// font mal. Trente copies parties du même gabarit se ressemblent à quatre-vingt
// pour cent sans que personne n'ait rien copié ; un rapport qui les remonte
// toutes est un rapport qu'on referme. Deux mécanismes s'en chargent ici : les
// empreintes du gabarit distribué, que l'outil connaît puisque c'est lui qui
// l'a distribué, et celles qui reviennent chez trop de monde pour vouloir dire
// quoi que ce soit.

// Valeurs par défaut du réglage.
const (
	// DefaultNoise écarte une empreinte présente chez plus de trois copies sur
	// dix. C'est assez haut pour ne pas effacer une fraude à trois, et assez
	// bas pour balayer ce qu'un générateur de projet a écrit pour tout le
	// monde.
	DefaultNoise = 0.30
	// DefaultMinSimilarity borne ce qui est matérialisé. En dessous, deux
	// copies partagent une poignée d'empreintes — une ligne, une signature de
	// méthode imposée —, et les garder noierait le rapport.
	DefaultMinSimilarity = 0.05
	// DefaultMaxMatches borne le nombre de paires détaillées. Les fragments
	// coûtent cher à reconstituer, et personne n'ouvre la cinq centième paire
	// d'une liste triée par suspicion.
	DefaultMaxMatches = 500
	// MinCorpus est le nombre de copies en deçà duquel la règle de bruit ne
	// s'applique pas du tout.
	//
	// « Présent chez trop de monde » n'a de sens que s'il y a du monde. Sur
	// quatre copies, trois qui partagent un passage font soixante-quinze pour
	// cent — la règle les écarterait comme banales, alors que trois copies
	// identiques sur quatre sont exactement ce qu'on cherche. En deçà de dix,
	// la statistique ne distingue plus le squelette de la fraude, et seul le
	// retrait du gabarit — qui, lui, ne devine rien — reste défendable.
	MinCorpus = 10
	// MinHolders est le nombre de copies en deçà duquel une empreinte n'est
	// jamais tenue pour banale, quel que soit le réglage. Une empreinte
	// partagée par deux copies seulement est le cas que l'outil existe pour
	// trouver ; l'écarter serait se tirer une balle dans le pied.
	MinHolders = 3
	// DenseLimit est le nombre de copies jusqu'auquel les compteurs de paires
	// tiennent dans un tableau plat plutôt que dans un dictionnaire. Au-delà,
	// le tableau pèserait plus que ce qu'il fait gagner.
	DenseLimit = 2048
	// MinSharedComment est la longueur en deçà de laquelle un commentaire
	// partagé ne dit rien : « à faire », « constructeur » s'écrivent seuls.
	MinSharedComment = 40
	// MinSharedLiteral est la longueur en deçà de laquelle une chaîne partagée
	// ne dit rien. Le seuil est plus bas que celui des commentaires parce
	// qu'une phrase affichée est plus contrainte qu'un commentaire — mais il
	// s'accompagne d'une exigence de forme : une chaîne ne compte que si elle
	// contient une espace, donc si c'est une phrase et non une constante
	// technique (« application/json », « SELECT », un nom de fichier).
	MinSharedLiteral = 20
	// MinSignalHolders est le nombre de porteurs en deçà duquel une coïncidence
	// est toujours rapportée, quelle que soit la taille du corpus.
	MinSignalHolders = 3
)

// Natures de signaux relevés hors de la mesure de similarité.
const (
	SharedSignature = "signature"
	SharedComment   = "commentaire"
	SharedLiteral   = "chaîne"
)

// Options règle une comparaison.
type Options struct {
	// Noise est la part des copies au-delà de laquelle une empreinte est tenue
	// pour commune. Zéro prend la valeur par défaut ; une valeur négative
	// désactive la règle.
	Noise float64
	// MinSimilarity et MaxMatches bornent ce que le rapport détaille.
	MinSimilarity float64
	MaxMatches    int
	// Baseline porte les empreintes du gabarit distribué, écartées d'office.
	Baseline map[uint64]struct{}
	// Ordinary porte ce que le gabarit distribué dit lui-même : ses
	// commentaires, ses chaînes. Aucun n'est rapporté comme coïncidence.
	Ordinary Signals
}

func (o Options) normalized() Options {
	if o.Noise == 0 {
		o.Noise = DefaultNoise
	}
	if o.MinSimilarity == 0 {
		o.MinSimilarity = DefaultMinSimilarity
	}
	if o.MaxMatches == 0 {
		o.MaxMatches = DefaultMaxMatches
	}
	return o
}

// Match est une paire de copies, et ce qu'elles ont en commun.
type Match struct {
	Left      string `json:"left"`
	Right     string `json:"right"`
	LeftName  string `json:"left_name,omitempty"`
	RightName string `json:"right_name,omitempty"`
	// LeftOrigin et RightOrigin disent d'où chaque copie vient. Deux copies du
	// même groupe qui se ressemblent, cela s'explique ; deux copies de deux
	// sessions séparées par trois ans, beaucoup moins.
	LeftOrigin  string `json:"left_origin,omitempty"`
	RightOrigin string `json:"right_origin,omitempty"`
	// Similarity est la part d'empreintes communes, entre 0 et 1.
	Similarity float64 `json:"similarity"`
	// LeftCoverage et RightCoverage disent la part de chaque copie qui est
	// commune. Elles se lisent avec Similarity et non à sa place : une copie
	// courte entièrement recopiée dans une copie longue a une couverture de
	// cent pour cent et une similarité faible.
	LeftCoverage  float64 `json:"left_coverage"`
	RightCoverage float64 `json:"right_coverage"`
	// Shared est le nombre d'empreintes communes retenues.
	Shared int `json:"shared"`
	// LongestFragment est la longueur du plus long passage commun, en jetons.
	LongestFragment int `json:"longest_fragment"`
	// LeftCovered et RightCovered comptent les jetons recouverts de chaque
	// côté, doublons fusionnés.
	LeftCovered  int         `json:"left_covered"`
	RightCovered int         `json:"right_covered"`
	Files        []FileMatch `json:"files,omitempty"`
}

// FileMatch est une paire de fichiers d'une paire de copies.
type FileMatch struct {
	LeftPath        string     `json:"left_path"`
	RightPath       string     `json:"right_path"`
	LongestFragment int        `json:"longest_fragment"`
	LeftCovered     int        `json:"left_covered"`
	RightCovered    int        `json:"right_covered"`
	LeftTokens      int        `json:"left_tokens"`
	RightTokens     int        `json:"right_tokens"`
	Fragments       []Fragment `json:"fragments"`
}

// Signal est une coïncidence relevée hors du winnowing.
type Signal struct {
	Kind   string   `json:"kind"`
	Detail string   `json:"detail,omitempty"`
	Works  []string `json:"works"`
}

// Report est le résultat d'une comparaison.
type Report struct {
	Works int `json:"works"`
	// Compared est le nombre de paires qui partageaient au moins une empreinte
	// retenue ; Matches ne montre que celles qui passent le seuil minimal.
	Compared int `json:"compared"`
	// IgnoredBaseline et IgnoredCommon comptent les empreintes écartées, par
	// motif. Le rapport les annonce : un enseignant doit savoir ce qui n'a pas
	// été comparé, et pourquoi.
	IgnoredBaseline int     `json:"ignored_baseline"`
	IgnoredCommon   int     `json:"ignored_common"`
	Threshold       float64 `json:"threshold"`
	// ThresholdNote dit ce que le seuil vaut, ou pourquoi il n'y en a pas.
	ThresholdNote string   `json:"threshold_note"`
	Histogram     []Bucket `json:"histogram"`
	Matches       []Match  `json:"matches"`
	Signals       []Signal `json:"signals,omitempty"`
}

// Compare mesure un corpus entier.
func Compare(corpus Corpus, options Options) Report {
	options = options.normalized()
	works := corpus.Works
	report := Report{Works: len(works)}
	if len(works) < 2 {
		return report
	}

	sets := make([]map[uint64]struct{}, len(works))
	for index, work := range works {
		sets[index] = distinct(work)
	}
	ignored, holders := common(sets, options, &report)

	// kept est le nombre d'empreintes retenues par copie : c'est le
	// dénominateur de la similarité, et il doit être calculé après l'écartement
	// du bruit, sinon deux copies qui ne partagent que du gabarit paraîtraient
	// proches.
	kept := make([]int, len(works))
	for index, set := range sets {
		for hash := range set {
			if _, muted := ignored[hash]; !muted {
				kept[index]++
			}
		}
	}

	shared := newCounter(len(works))
	for _, list := range holders {
		for first := 0; first < len(list); first++ {
			for second := first + 1; second < len(list); second++ {
				shared.add(list[first], list[second])
			}
		}
	}

	matches := make([]Match, 0, 64)
	report.Histogram = newHistogram()
	shared.each(func(left, right, count int) {
		total := kept[left] + kept[right]
		if total == 0 {
			return
		}
		report.Compared++
		similarity := 2 * float64(count) / float64(total)
		// L'histogramme reçoit toutes les paires mesurées, y compris celles
		// qu'on ne détaillera pas : c'est la masse des copies honnêtes qui
		// donne son sens au seuil, et la retirer le ferait tomber n'importe
		// où.
		bump(report.Histogram, similarity)
		if similarity < options.MinSimilarity {
			return
		}
		matches = append(matches, Match{
			Left: works[left].ID, Right: works[right].ID,
			LeftName: works[left].Name(), RightName: works[right].Name(),
			LeftOrigin: works[left].Origin, RightOrigin: works[right].Origin,
			Similarity:   similarity,
			LeftCoverage: ratio(count, kept[left]), RightCoverage: ratio(count, kept[right]),
			Shared: count,
		})
	})

	sort.SliceStable(matches, func(first, second int) bool {
		return matches[first].Similarity > matches[second].Similarity
	})
	report.Threshold = threshold(report.Histogram)
	report.ThresholdNote = ThresholdNote
	if report.Threshold == 0 {
		report.ThresholdNote = NoThreshold
	}

	if len(matches) > options.MaxMatches {
		matches = matches[:options.MaxMatches]
	}
	byID := index(works)
	for position := range matches {
		detail(&matches[position], works[byID[matches[position].Left]],
			works[byID[matches[position].Right]], ignored)
	}
	report.Matches = matches
	report.Signals = signals(works, options)
	return report
}

// distinct rend les empreintes d'une copie, sans doublon. Une même empreinte
// répétée dans un fichier — une structure qui revient — ne doit compter qu'une
// fois, sans quoi un fichier répétitif paraîtrait ressembler à tout le monde.
func distinct(work Work) map[uint64]struct{} {
	set := make(map[uint64]struct{}, work.PrintCount())
	for _, file := range work.Files {
		for _, print := range file.Prints {
			set[print.Hash] = struct{}{}
		}
	}
	return set
}

// common décide quelles empreintes sont écartées, et rend pour chaque
// empreinte retenue la liste des copies qui la portent.
func common(sets []map[uint64]struct{}, options Options, report *Report) (
	map[uint64]struct{}, map[uint64][]int) {

	holders := map[uint64][]int{}
	for index, set := range sets {
		for hash := range set {
			holders[hash] = append(holders[hash], index)
		}
	}

	ignored := make(map[uint64]struct{}, len(options.Baseline))
	for hash := range options.Baseline {
		if _, present := holders[hash]; present {
			ignored[hash] = struct{}{}
			report.IgnoredBaseline++
		}
	}

	limit := noiseLimit(len(sets), options.Noise)
	for hash, list := range holders {
		if _, already := ignored[hash]; already {
			delete(holders, hash)
			continue
		}
		if limit > 0 && len(list) >= limit {
			ignored[hash] = struct{}{}
			report.IgnoredCommon++
			delete(holders, hash)
		}
	}
	return ignored, holders
}

// noiseLimit rend le nombre de copies à partir duquel une empreinte est tenue
// pour commune, ou zéro quand la règle ne s'applique pas.
func noiseLimit(works int, noise float64) int {
	if noise < 0 || works < MinCorpus {
		return 0
	}
	limit := int(noise*float64(works) + 0.5)
	if limit < MinHolders {
		limit = MinHolders
	}
	return limit
}

// signalLimit rend le nombre de porteurs à partir duquel une coïncidence cesse
// d'en être une.
//
// La règle de bruit s'applique dès que le corpus est assez grand pour qu'elle
// veuille dire quelque chose. En dessous, elle s'efface — et il faut bien une
// borne quand même : une phrase que les deux tiers du groupe portent vient de
// l'énoncé ou du gabarit, qu'ils soient six ou soixante.
//
// Jamais moins de MinSignalHolders : deux copies qui partagent une phrase sont
// le cas que l'outil existe pour trouver.
func signalLimit(works int, noise float64) int {
	limit := 2*works/3 + 1
	if bruit := noiseLimit(works, noise); bruit > 0 && bruit < limit {
		limit = bruit
	}
	if limit < MinSignalHolders {
		limit = MinSignalHolders
	}
	return limit
}

// ordinaire range ce que le gabarit distribué porte lui-même.
func ordinaire(signals Signals) map[string]bool {
	banal := make(map[string]bool, len(signals.Comments)+len(signals.Literals))
	for _, texte := range signals.Comments {
		banal[texte] = true
	}
	for _, texte := range signals.Literals {
		banal[texte] = true
	}
	return banal
}

// phrase dit qu'une chaîne est une phrase, et non une constante technique.
//
// Une espace suffit à faire la différence : « Entrez un nombre entre 1 et 100 »
// en porte, « application/json » et « SELECT * FROM inventaire » — non, la
// seconde en porte aussi. Le critère n'est donc pas parfait ; il écarte le gros
// du bruit, et la règle de majorité fait le reste.
func phrase(texte string) bool { return strings.Contains(strings.TrimSpace(texte), " ") }

// detail reconstitue les fragments d'une paire, fichier par fichier.
func detail(match *Match, left, right Work, ignored map[uint64]struct{}) {
	// Une empreinte peut revenir plusieurs fois dans un même fichier ; toutes
	// ses positions comptent, car c'est peut-être la deuxième qui est sur la
	// bonne diagonale.
	placed := map[uint64][]struct{ file, token int }{}
	for number, file := range left.Files {
		for _, print := range file.Prints {
			if _, muted := ignored[print.Hash]; muted {
				continue
			}
			placed[print.Hash] = append(placed[print.Hash],
				struct{ file, token int }{number, print.Index})
		}
	}

	seeds := map[[2]int][]seed{}
	for number, file := range right.Files {
		for _, print := range file.Prints {
			for _, spot := range placed[print.Hash] {
				key := [2]int{spot.file, number}
				seeds[key] = append(seeds[key], seed{left: spot.token, right: print.Index})
			}
		}
	}

	files := make([]FileMatch, 0, len(seeds))
	for key, list := range seeds {
		leftFile, rightFile := left.Files[key[0]], right.Files[key[1]]
		// Les bornes des deux côtés peuvent différer — deux index calculés
		// séparément, ou deux langages. La plus prudente vaut pour les deux :
		// un fragment annoncé trop long serait un mensonge.
		fragments := weave(list, min(leftFile.Kgram, rightFile.Kgram),
			min(leftFile.Window, rightFile.Window))
		files = append(files, FileMatch{
			LeftPath: leftFile.Path, RightPath: rightFile.Path,
			LongestFragment: longest(fragments),
			LeftCovered:     covered(fragments, true),
			RightCovered:    covered(fragments, false),
			LeftTokens:      leftFile.Tokens, RightTokens: rightFile.Tokens,
			Fragments: fragments,
		})
	}
	sort.Slice(files, func(first, second int) bool {
		if files[first].LongestFragment != files[second].LongestFragment {
			return files[first].LongestFragment > files[second].LongestFragment
		}
		return files[first].LeftPath < files[second].LeftPath
	})

	match.Files = files
	for _, file := range files {
		if file.LongestFragment > match.LongestFragment {
			match.LongestFragment = file.LongestFragment
		}
		match.LeftCovered += file.LeftCovered
		match.RightCovered += file.RightCovered
	}
}

// signals relève les coïncidences que le winnowing ne voit pas.
func signals(works []Work, options Options) []Signal {
	found := make([]Signal, 0, 4)

	marks := map[string][]string{}
	for _, work := range works {
		if mark := work.Extras.Signature; mark != "" {
			marks[mark] = append(marks[mark], work.ID)
		}
	}
	for mark, holders := range marks {
		if len(holders) > 1 {
			sort.Strings(holders)
			found = append(found, Signal{Kind: SharedSignature, Detail: mark, Works: holders})
		}
	}

	// Un commentaire que tout le monde porte vient du gabarit ou de l'énoncé ;
	// un commentaire long que deux copies partagent n'a pas d'explication
	// innocente. Il en va de même d'une phrase affichée.
	limit := signalLimit(len(works), options.Noise)
	banal := ordinaire(options.Ordinary)
	for _, partage := range []struct {
		kind    string
		minimum int
		of      func(Signals) []string
		keep    func(string) bool
	}{
		{SharedComment, MinSharedComment,
			func(s Signals) []string { return s.Comments }, nil},
		{SharedLiteral, MinSharedLiteral,
			func(s Signals) []string { return s.Literals }, phrase},
	} {
		porteurs := map[string][]string{}
		for _, work := range works {
			vus := map[string]bool{}
			for _, texte := range partage.of(work.Extras) {
				if len([]rune(texte)) < partage.minimum || vus[texte] || banal[texte] {
					continue
				}
				if partage.keep != nil && !partage.keep(texte) {
					continue
				}
				vus[texte] = true
				porteurs[texte] = append(porteurs[texte], work.ID)
			}
		}
		for texte, holders := range porteurs {
			if len(holders) < 2 || len(holders) >= limit {
				continue
			}
			sort.Strings(holders)
			found = append(found,
				Signal{Kind: partage.kind, Detail: texte, Works: holders})
		}
	}

	sort.Slice(found, func(first, second int) bool {
		if found[first].Kind != found[second].Kind {
			return found[first].Kind < found[second].Kind
		}
		return found[first].Detail < found[second].Detail
	})
	return found
}

// index range les copies par identifiant.
func index(works []Work) map[string]int {
	byID := make(map[string]int, len(works))
	for position, work := range works {
		byID[work.ID] = position
	}
	return byID
}

func ratio(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total)
}
