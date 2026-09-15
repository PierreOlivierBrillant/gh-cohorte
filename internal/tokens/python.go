package tokens

import "strings"

// Python n'a pas d'accolades : c'est l'indentation qui dit où un bloc commence
// et où il finit. Un analyseur qui l'ignorerait rendrait le même flux pour deux
// programmes de structures différentes — une boucle imbriquée et une boucle
// suivie d'une autre —, et deux copies sans rapport paraîtraient jumelles.
//
// L'indentation est donc traduite en jetons, comme le fait l'interpréteur
// lui-même : un « ⇥ » quand un bloc s'ouvre, un « ⇤ » par bloc qui se ferme.
// Ce que les accolades disent ailleurs, ces deux jetons le disent ici.

var pythonKeywords = words(`False None True and as assert async await break
	class continue def del elif else except finally for from global if import
	in is lambda nonlocal not or pass raise return try while with yield match
	case`)

// pythonQuotes range les formes longues avant les courtes : « """ » doit être
// reconnu avant « " », sans quoi une chaîne de trois lignes se lirait comme une
// chaîne vide suivie de code.
var pythonQuotes = []quote{
	{open: `"""`, close: `"""`, escape: true, multiline: true},
	{open: `'''`, close: `'''`, escape: true, multiline: true},
	{open: `"`, close: `"`, escape: true},
	{open: `'`, close: `'`, escape: true},
}

var pythonSyntax = syntax{
	keywords:     pythonKeywords,
	lineComments: []string{"#"},
	quotes:       pythonQuotes,
}

// TabWidth est la largeur d'une tabulation pour le calcul de l'indentation.
// Huit est ce que dit la référence du langage, et la valeur n'a de toute façon
// d'importance que pour un fichier qui mêle tabulations et espaces — ce que
// Python refuse lui-même.
const TabWidth = 8

func lexPython(source string) Result {
	scan := newScanner(source)
	state := &pythonState{levels: []int{0}}

	for !scan.done() {
		// Une nouvelle ligne logique commence : c'est là, et seulement là, que
		// l'indentation veut dire quelque chose. À l'intérieur d'une
		// parenthèse ou après une barre oblique inverse, la ligne continue et
		// son indentation n'est que de la mise en page.
		if state.atLineStart && state.depth == 0 {
			if state.openBlocks(scan) {
				continue
			}
		}
		switch char := scan.peek(); {
		case char == '\n':
			state.atLineStart = true
			scan.advance()
		case isSpace(char):
			scan.advance()
		case char == '\\' && scan.at(1) == '\n':
			// Continuation explicite : la ligne suivante n'en est pas une.
			scan.skip(2)
		case pythonSyntax.readComment(scan):
		case state.readPrefixedQuote(scan):
		case pythonSyntax.readQuote(scan):
			state.atLineStart = false
		case pythonSyntax.readNumber(scan):
			state.atLineStart = false
		case pythonSyntax.readWord(scan):
			state.atLineStart = false
		default:
			state.track(char)
			pythonSyntax.readPunctuation(scan)
			state.atLineStart = false
		}
	}
	state.closeAll(scan)
	return scan.result()
}

// pythonState tient ce que l'analyseur doit se rappeler d'une ligne à l'autre.
type pythonState struct {
	// levels est la pile des indentations ouvertes, 0 au fond.
	levels []int
	// depth compte les parenthèses, crochets et accolades ouverts. Tant qu'il
	// n'est pas nul, un passage à la ligne ne commence pas de ligne logique.
	depth       int
	atLineStart bool
}

// openBlocks lit l'indentation d'une ligne et en tire les jetons de structure.
// Il rend vrai quand la ligne était vide ou ne portait qu'un commentaire : rien
// n'y est comparable, et son indentation ne dit rien — une ligne blanche au
// milieu d'un bloc ne le ferme pas.
func (state *pythonState) openBlocks(scan *scanner) bool {
	offset, line, column := scan.mark()
	indent := 0
	for !scan.done() {
		switch scan.peek() {
		case ' ':
			indent++
		case '\t':
			indent += TabWidth - indent%TabWidth
		default:
			goto measured
		}
		scan.advance()
	}
measured:
	if scan.done() || scan.peek() == '\n' || scan.has("#") {
		return false
	}
	state.atLineStart = false

	top := state.levels[len(state.levels)-1]
	switch {
	case indent > top:
		state.levels = append(state.levels, indent)
		scan.emitAt(Indent, OpenBlock, offset, line, column)
	case indent < top:
		// Plusieurs blocs peuvent se fermer d'un coup. Une indentation qui ne
		// retombe sur aucun niveau connu est une erreur de syntaxe ; on ferme
		// ce qu'on peut plutôt que d'abandonner le fichier.
		for len(state.levels) > 1 && state.levels[len(state.levels)-1] > indent {
			state.levels = state.levels[:len(state.levels)-1]
			scan.emitAt(Dedent, CloseBlock, offset, line, column)
		}
	}
	return false
}

// closeAll ferme les blocs restés ouverts à la fin du fichier.
func (state *pythonState) closeAll(scan *scanner) {
	offset, line, column := scan.mark()
	for len(state.levels) > 1 {
		state.levels = state.levels[:len(state.levels)-1]
		scan.emitAt(Dedent, CloseBlock, offset, line, column)
	}
}

// track suit les parenthèses ouvertes.
func (state *pythonState) track(char byte) {
	switch char {
	case '(', '[', '{':
		state.depth++
	case ')', ']', '}':
		if state.depth > 0 {
			state.depth--
		}
	}
}

// pythonPrefixes sont les lettres qui peuvent précéder une chaîne : « f" »,
// « rb'…' ». Sans elles, la lettre se lirait comme un nom, et « f"…" » donnerait
// « ID » puis « STR » là où il n'y a qu'une chaîne.
const pythonPrefixes = "rubfRUBF"

// readPrefixedQuote avale le préfixe d'une chaîne, s'il y en a un, et laisse la
// chaîne elle-même à l'analyseur ordinaire.
func (state *pythonState) readPrefixedQuote(scan *scanner) bool {
	length := 0
	for length < 2 && strings.IndexByte(pythonPrefixes, scan.at(length)) >= 0 {
		length++
	}
	if length == 0 {
		return false
	}
	next := scan.at(length)
	if next != '"' && next != '\'' {
		return false
	}
	scan.skip(length)
	return false
}
