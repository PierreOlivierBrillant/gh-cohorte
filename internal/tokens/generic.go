package tokens

import "strings"

// lexer fabrique l'analyseur d'un langage de la famille C à partir de sa table
// de syntaxe.
//
// L'ordre des essais n'est pas indifférent et ne peut pas l'être : « // » doit
// être reconnu avant « / », « -- » avant « - », « """ » avant « " ». Chaque
// forme est donc essayée de la plus longue à la plus courte, et les tables
// rangent déjà leurs variantes dans cet ordre.
func lexer(rules syntax) func(string) Result {
	return func(source string) Result {
		scan := newScanner(source)
		for !scan.done() {
			switch {
			case isSpace(scan.peek()):
				scan.advance()
			case rules.readComment(scan):
			case rules.readQuote(scan):
			case rules.readNumber(scan):
			case rules.readWord(scan):
			default:
				rules.readPunctuation(scan)
			}
		}
		return scan.result()
	}
}

// readComment avale un commentaire et le range de côté. Il rend faux si la
// source ne commence pas par un commentaire, pour que l'appelant essaie autre
// chose.
func (rules syntax) readComment(scan *scanner) bool {
	for _, marker := range rules.lineComments {
		if scan.has(marker) {
			line := scan.line
			start := scan.offset + len(marker)
			for !scan.done() && scan.peek() != '\n' {
				scan.advance()
			}
			scan.comment(scan.source[start:scan.offset], line)
			return true
		}
	}
	for _, pair := range rules.blockComments {
		if !scan.has(pair[0]) {
			continue
		}
		line := scan.line
		scan.skip(len(pair[0]))
		start := scan.offset
		for !scan.done() && !scan.has(pair[1]) {
			scan.advance()
		}
		end := scan.offset
		// Un commentaire jamais refermé court jusqu'à la fin du fichier. C'est
		// une source invalide, mais elle existe — une accolade oubliée dans un
		// travail d'étudiant est le cas courant, pas l'exception —, et il vaut
		// mieux l'analyser au mieux que refuser tout le fichier.
		scan.skip(len(pair[1]))
		scan.comment(scan.source[start:end], line)
		return true
	}
	return false
}

// readQuote avale un littéral textuel. Son contenu n'est pas comparé : deux
// copies qui ne diffèrent que par leurs messages restent appariées, et c'est
// voulu — changer ses libellés est le maquillage le moins coûteux qui soit.
func (rules syntax) readQuote(scan *scanner) bool {
	for _, form := range rules.quotes {
		if !scan.has(form.open) {
			continue
		}
		offset, line, column := scan.mark()
		scan.skip(len(form.open))
		for !scan.done() {
			if form.escape && scan.peek() == '\\' {
				scan.skip(2)
				continue
			}
			if scan.has(form.close) {
				scan.skip(len(form.close))
				break
			}
			// Une chaîne d'une seule ligne qui rencontre une fin de ligne n'est
			// pas fermée : s'arrêter là évite d'avaler tout le reste du fichier
			// à cause d'un guillemet oublié.
			if !form.multiline && scan.peek() == '\n' {
				break
			}
			scan.advance()
		}
		scan.emit(Text, AnyText, offset, line, column)
		return true
	}
	return false
}

// readNumber avale un littéral numérique, suffixes et séparateurs compris —
// « 0xFF », « 1_000_000 », « 3.14f », « 1e-9 », « 10px ».
func (rules syntax) readNumber(scan *scanner) bool {
	if !rules.startsNumber(scan) {
		return false
	}
	offset, line, column := scan.mark()
	if scan.peek() == '-' || scan.peek() == '.' {
		scan.advance()
	}
	for !scan.done() {
		char := scan.peek()
		// L'exposant emporte son signe : sans cela « 1e-9 » se couperait en
		// « 1e », « - » et « 9 ».
		if (char == 'e' || char == 'E') && (scan.at(1) == '+' || scan.at(1) == '-') &&
			isDigit(scan.at(2)) {
			scan.skip(2)
			continue
		}
		if !isDigit(char) && !isIdentifierPart(char) && char != '.' {
			break
		}
		// Un point suivi d'autre chose qu'un chiffre termine le nombre : c'est
		// l'accès à un membre, « 1.toString() » en Kotlin.
		if char == '.' && !isDigit(scan.at(1)) {
			break
		}
		scan.advance()
	}
	scan.emit(Number, AnyNumber, offset, line, column)
	return true
}

// startsNumber dit si la source commence par un nombre. Le signe n'en fait
// partie que là où le tiret n'est pas un opérateur — en CSS, « -10px » est une
// valeur, pas une soustraction.
func (rules syntax) startsNumber(scan *scanner) bool {
	char := scan.peek()
	switch {
	case isDigit(char):
		return true
	case char == '.':
		return isDigit(scan.at(1))
	case rules.keepIdentifiers && char == '-':
		return isDigit(scan.at(1)) || (scan.at(1) == '.' && isDigit(scan.at(2)))
	}
	return false
}

// readWord avale un mot : mot-clé du langage, ou nom choisi par qui écrit.
func (rules syntax) readWord(scan *scanner) bool {
	if !rules.startsWord(scan.peek()) {
		return false
	}
	offset, line, column := scan.mark()
	for !scan.done() && rules.continuesWord(scan.peek()) {
		scan.advance()
	}
	word := scan.source[offset:scan.offset]
	lowered := strings.ToLower(word)

	if rules.fold {
		word = lowered
	}
	if rules.keywords[word] {
		scan.emit(Keyword, word, offset, line, column)
		return true
	}
	if rules.keepIdentifiers && !rules.chosenName(scan, lowered, offset) {
		scan.emit(Identifier, lowered, offset, line, column)
		return true
	}
	scan.emit(Identifier, AnyIdentifier, offset, line, column)
	return true
}

// startsWord et continuesWord disent ce qui compose un nom. Le tiret en fait
// partie en CSS, où « flex-direction » est un seul mot : le couper en trois
// ferait de chaque feuille de style une suite de fragments interchangeables.
func (rules syntax) startsWord(char byte) bool {
	return isIdentifierStart(char) || (rules.keepIdentifiers && char == '-')
}

func (rules syntax) continuesWord(char byte) bool {
	return isIdentifierPart(char) || (rules.keepIdentifiers && char == '-')
}

// chosenName dit qu'un nom a été choisi par qui écrit plutôt que par le
// langage, et doit donc être effacé.
//
// La question ne se pose que là où les noms sont gardés, c'est-à-dire en CSS.
// « display » ou « grid-template-columns » viennent du langage et doivent
// rester ; le nom d'une classe, d'un identifiant ou d'une propriété
// personnalisée vient de l'auteur, et le garder reviendrait à comparer des
// noms au lieu de comparer des feuilles de style.
func (rules syntax) chosenName(scan *scanner, word string, offset int) bool {
	if strings.HasPrefix(word, "--") {
		return true
	}
	previous := scan.lastToken()
	return previous != nil && previous.Kind == Punctuation && previous.End == offset &&
		(previous.Text == "." || previous.Text == "#")
}

// readPunctuation avale un caractère d'opérateur ou de ponctuation.
//
// Un caractère par jeton, jamais deux : « => » devient « = » puis « > ». C'est
// plus grossier qu'une table d'opérateurs, mais cela ne perd rien — les deux
// côtés d'une comparaison sont découpés de la même façon —, cela vaut pour
// treize langages sans en connaître aucun, et cela n'a aucun cas particulier à
// se tromper.
func (rules syntax) readPunctuation(scan *scanner) {
	offset, line, column := scan.mark()
	scan.advance()
	scan.emit(Punctuation, scan.source[offset:scan.offset], offset, line, column)
}

// lastToken rend le dernier jeton émis, ou nil.
func (s *scanner) lastToken() *Token {
	if len(s.tokens) == 0 {
		return nil
	}
	return &s.tokens[len(s.tokens)-1]
}
