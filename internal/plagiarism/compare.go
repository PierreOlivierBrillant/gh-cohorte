package plagiarism

import (
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/similarity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/tokens"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Un score qu'on ne peut pas vérifier ne vaut rien. C'est ici que le rapport
// redevient du texte : les deux copies d'une paire sont retéléchargées, au
// commit exact que l'analyse avait archivé, et les fragments — qui ne sont que
// des rangs de jetons — retrouvent des lignes et des colonnes.
//
// Deux copies, pas trois cents. Garder tout le corpus en mémoire pour le cas où
// l'on ouvrirait une paire coûterait des gigaoctets ; le retélécharger au
// moment où on l'ouvre coûte deux requêtes et une seconde.

// Side est une copie d'une paire.
type Side struct {
	ID     string `json:"id"`
	Label  string `json:"label,omitempty"`
	Origin string `json:"origin,omitempty"`
	Commit string `json:"commit,omitempty"`
	Repo   string `json:"repo,omitempty"`
}

// Pair est une paire chargée : les deux copies, et leur contenu.
type Pair struct {
	Left  Side
	Right Side
	Match similarity.Match

	texts map[string]map[string][]byte // copie → chemin → contenu
}

// OpenPair retélécharge les deux copies d'une paire.
func OpenPair(client corpus.Client, report *Report, left, right string) (*Pair, error) {
	match, found := report.Match(left, right)
	if !found {
		return nil, valid.Errorf(
			"Paire « %s » et « %s » : elle ne figure pas dans ce rapport.", left, right)
	}
	inspector, err := inspect.New(report.Request.Inspection, report.Request.Rules.Profiles)
	if err != nil {
		return nil, err
	}

	pair := &Pair{Match: match, texts: map[string]map[string][]byte{}}
	for _, id := range []string{match.Left, match.Right} {
		side, texts, err := fetch(client, report, inspector, id)
		if err != nil {
			return nil, err
		}
		pair.texts[id] = texts
		if id == match.Left {
			pair.Left = side
		} else {
			pair.Right = side
		}
	}
	return pair, nil
}

// Match retrouve une paire dans le rapport, dans un sens comme dans l'autre.
func (r *Report) Match(left, right string) (similarity.Match, bool) {
	for _, match := range r.Result.Matches {
		if (match.Left == left && match.Right == right) ||
			(match.Left == right && match.Right == left) {
			return match, true
		}
	}
	return similarity.Match{}, false
}

// fetch récupère une copie et en rend les fichiers retenus.
//
// L'inspection est refaite avec exactement les réglages du rapport : c'est ce
// qui garantit que les rangs de jetons des fragments désignent bien les mêmes
// jetons qu'au moment de l'analyse. Les refaire autrement décalerait tout sans
// que rien ne le signale.
func fetch(client corpus.Client, report *Report, inspector *inspect.Inspector,
	id string) (Side, map[string][]byte, error) {

	target, known := report.Target(id)
	if !known {
		return Side{}, nil, valid.Errorf(
			"Copie « %s » : elle ne figure pas dans ce rapport.", id)
	}
	// Le commit archivé plutôt que la branche : le dépôt a pu bouger depuis
	// l'analyse, et comparer d'après un autre état montrerait des fragments
	// décalés, voire des fichiers disparus.
	ref := report.Commit(id)
	if ref == "" {
		ref = target.Ref
	}

	archive, err := client.Archive(target.Owner, target.Repo, ref)
	if err != nil {
		return Side{}, nil, valid.Errorf(
			"Copie « %s » : %v. Le dépôt a peut-être été supprimé depuis l'analyse.",
			id, err)
	}
	sources, _, err := corpus.Untar(archive)
	if err != nil {
		return Side{}, nil, err
	}

	texts := map[string][]byte{}
	for _, kept := range inspector.Select(sources).Kept {
		texts[kept.Path] = kept.Content
	}
	side := Side{ID: id, Label: target.Label, Origin: target.Origin,
		Commit: ref, Repo: target.FullName()}
	return side, texts, nil
}

// ---------------------------------------------------------- vue de projet

// Project est la vue « explorer deux projets » : les fichiers des deux côtés et
// ce qui les relie, sans une ligne de code.
type Project struct {
	Left  Side       `json:"left"`
	Right Side       `json:"right"`
	Links []FileLink `json:"links"`
	// LeftOnly et RightOnly nomment les fichiers qui n'ont de correspondant
	// nulle part. Ils comptent : un travail recopié puis complété n'a de
	// fragments que sur une partie de ses fichiers, et voir laquelle est ce
	// qui permet de juger.
	LeftOnly  []Lonely `json:"left_only,omitempty"`
	RightOnly []Lonely `json:"right_only,omitempty"`
}

// Lonely est un fichier qu'aucun fragment ne relie, et la raison.
//
// Les deux raisons ne veulent pas du tout dire la même chose. « Aucun passage
// commun » est un résultat ; « trop court pour être comparé » est un aveu. Les
// confondre laisserait croire qu'un fichier a été regardé alors qu'il n'a
// jamais pu l'être.
type Lonely struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Raisons pour lesquelles un fichier n'est relié à rien.
const (
	NothingShared     = "aucun passage commun"
	TooShortToCompare = "trop court pour former un k-gramme"
)

// FileLink relie deux fichiers, avec ce qu'ils ont en commun.
type FileLink struct {
	LeftPath        string `json:"left_path"`
	RightPath       string `json:"right_path"`
	LongestFragment int    `json:"longest_fragment"`
	LeftTokens      int    `json:"left_tokens"`
	RightTokens     int    `json:"right_tokens"`
	LeftCovered     int    `json:"left_covered"`
	RightCovered    int    `json:"right_covered"`
	Fragments       int    `json:"fragments"`
	// SameName dit que les deux fichiers portent le même chemin. Ce n'est pas
	// une preuve — un gabarit impose les noms —, mais un fichier renommé qui
	// se retrouve apparié à un autre est un signe de plus.
	SameName bool `json:"same_name"`
}

// Project bâtit la vue de projet d'une paire.
func (p *Pair) Project(report *Report) Project {
	view := Project{Left: p.Left, Right: p.Right}
	linkedLeft, linkedRight := map[string]bool{}, map[string]bool{}

	for _, file := range p.Match.Files {
		view.Links = append(view.Links, FileLink{
			LeftPath: file.LeftPath, RightPath: file.RightPath,
			LongestFragment: file.LongestFragment,
			LeftTokens:      file.LeftTokens, RightTokens: file.RightTokens,
			LeftCovered: file.LeftCovered, RightCovered: file.RightCovered,
			Fragments: len(file.Fragments),
			SameName:  file.LeftPath == file.RightPath,
		})
		linkedLeft[file.LeftPath] = true
		linkedRight[file.RightPath] = true
	}

	view.LeftOnly = unlinked(report, p.Match.Left, linkedLeft)
	view.RightOnly = unlinked(report, p.Match.Right, linkedRight)
	return view
}

// unlinked nomme les fichiers d'une copie qu'aucun fragment ne relie, et dit
// pourquoi.
func unlinked(report *Report, id string, linked map[string]bool) []Lonely {
	alone := make([]Lonely, 0, 8)
	for _, work := range report.Index.Works {
		if work.ID != id {
			continue
		}
		for _, file := range work.Files {
			if linked[file.Path] {
				continue
			}
			reason := NothingShared
			if len(file.Prints) == 0 {
				reason = TooShortToCompare
			}
			alone = append(alone, Lonely{Path: file.Path, Reason: reason})
		}
	}
	return alone
}

// --------------------------------------------------------- vue de fichier

// FileView est la vue « comparer deux fichiers » : le texte des deux côtés, les
// fragments situés, et le flux de jetons réellement comparé.
type FileView struct {
	Left  FileSide `json:"left"`
	Right FileSide `json:"right"`
	Spans []Span   `json:"spans"`
}

// FileSide est un fichier d'une comparaison.
type FileSide struct {
	Path  string `json:"path"`
	Text  string `json:"text"`
	Lines int    `json:"lines"`
	// Tokens est le flux réellement comparé. C'est lui qui rend limpide ce que
	// chaque réglage fait : on y voit qu'un nom de variable est devenu « ID »,
	// donc que le renommer n'aurait rien changé.
	Tokens []tokens.Token `json:"tokens"`
}

// Span est un fragment commun, situé dans les deux fichiers.
type Span struct {
	// Les octets servent à surligner au caractère près, les lignes à naviguer
	// d'un fragment au suivant.
	LeftStart  int `json:"left_start"`
	LeftEnd    int `json:"left_end"`
	RightStart int `json:"right_start"`
	RightEnd   int `json:"right_end"`
	LeftFrom   int `json:"left_from"`
	LeftTo     int `json:"left_to"`
	RightFrom  int `json:"right_from"`
	RightTo    int `json:"right_to"`
	Tokens     int `json:"tokens"`
}

// File bâtit la vue de deux fichiers appariés.
func (p *Pair) File(leftPath, rightPath string) (FileView, error) {
	var matched *similarity.FileMatch
	for index, file := range p.Match.Files {
		if file.LeftPath == leftPath && file.RightPath == rightPath {
			matched = &p.Match.Files[index]
		}
	}
	if matched == nil {
		return FileView{}, valid.Errorf(
			"Fichiers « %s » et « %s » : aucun fragment ne les relie dans ce rapport.",
			leftPath, rightPath)
	}

	left, err := p.side(p.Match.Left, leftPath)
	if err != nil {
		return FileView{}, err
	}
	right, err := p.side(p.Match.Right, rightPath)
	if err != nil {
		return FileView{}, err
	}

	view := FileView{Left: left, Right: right}
	for _, fragment := range matched.Fragments {
		span := Span{Tokens: fragment.Length()}
		span.LeftStart, span.LeftEnd = byteSpan(left.Tokens, fragment.LeftStart, fragment.LeftEnd)
		span.RightStart, span.RightEnd = byteSpan(right.Tokens, fragment.RightStart, fragment.RightEnd)
		span.LeftFrom, span.LeftTo = similarity.Lines(left.Tokens, fragment.LeftStart, fragment.LeftEnd)
		span.RightFrom, span.RightTo = similarity.Lines(right.Tokens, fragment.RightStart, fragment.RightEnd)
		view.Spans = append(view.Spans, span)
	}
	return view, nil
}

// side relit un fichier d'une copie et le réanalyse.
func (p *Pair) side(id, path string) (FileSide, error) {
	content, found := p.texts[id][path]
	if !found {
		return FileSide{}, valid.Errorf(
			"Fichier « %s » : il n'est plus dans la copie « %s ». Le dépôt a "+
				"peut-être changé depuis l'analyse.", path, id)
	}
	language, _ := tokens.Detect(path)
	analysis := tokens.Lex(language.ID, content)
	return FileSide{
		Path: path, Text: string(content),
		Lines: countLines(content), Tokens: analysis.Tokens,
	}, nil
}

// byteSpan traduit un intervalle de jetons en un intervalle d'octets.
func byteSpan(stream []tokens.Token, start, end int) (int, int) {
	if len(stream) == 0 || start < 0 || start >= len(stream) {
		return 0, 0
	}
	if end > len(stream) {
		end = len(stream)
	}
	if end <= start {
		return 0, 0
	}
	return stream[start].Start, stream[end-1].End
}

func countLines(content []byte) int {
	lines := 1
	for _, octet := range content {
		if octet == '\n' {
			lines++
		}
	}
	return lines
}
