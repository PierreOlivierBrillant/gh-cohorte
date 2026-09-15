package tokens

import "strings"

// Le balisage ne se compare pas comme du code. Ce qui s'y recopie, ce sont les
// balises et leur emboîtement, les noms d'attributs, et le texte visible ; les
// valeurs d'attributs — une couleur, une largeur, une adresse — changent d'un
// travail à l'autre sans rien dire. L'analyseur garde donc les premiers et
// efface les secondes, comme il efface les chaînes d'un langage de code.
//
// En HTML, un « script » ou un « style » n'est pas du balisage : c'est du
// JavaScript ou du CSS posé au milieu. Il est analysé comme tel et recousu dans
// le flux à sa place, sans quoi une fonction recopiée dans une page passerait
// pour une suite de mots.

func lexHTML(source string) Result { return lexMarkup(source, true) }

func lexXML(source string) Result { return lexMarkup(source, false) }

func lexMarkup(source string, html bool) Result {
	scan := newScanner(source)
	for !scan.done() {
		switch {
		case scan.has("<!--"):
			readMarkupComment(scan)
		case scan.peek() == '<' && startsTag(scan.at(1)):
			readTag(scan, html)
		default:
			// Tout le reste est du texte, « a < b » compris : un chevron que
			// ne suit pas un nom de balise n'ouvre rien.
			readProse(scan, func() bool {
				return scan.peek() == '<' && (startsTag(scan.at(1)) || scan.has("<!--"))
			})
		}
	}
	return scan.result()
}

// startsTag dit qu'un chevron ouvre bien une balise.
func startsTag(char byte) bool {
	return isIdentifierStart(char) || char == '/' || char == '!' || char == '?'
}

func readMarkupComment(scan *scanner) {
	line := scan.line
	scan.skip(len("<!--"))
	start := scan.offset
	for !scan.done() && !scan.has("-->") {
		scan.advance()
	}
	end := scan.offset
	scan.skip(len("-->"))
	scan.comment(scan.source[start:end], line)
}

// readTag avale une balise entière, ses attributs compris.
func readTag(scan *scanner, html bool) {
	offset, line, column := scan.mark()
	scan.advance() // <

	// Une déclaration ou une instruction de traitement — « <!DOCTYPE », « <?xml »
	// — ne porte ni structure ni contenu : un seul jeton suffit à dire qu'elle
	// était là.
	if scan.peek() == '!' || scan.peek() == '?' {
		for !scan.done() && scan.peek() != '>' {
			scan.advance()
		}
		scan.advance()
		scan.emit(Tag, "<!", offset, line, column)
		return
	}

	closing := scan.peek() == '/'
	if closing {
		scan.advance()
	}
	name := readMarkupName(scan)
	marker := "<" + name
	if closing {
		marker = "</" + name
	}
	scan.emit(Tag, marker, offset, line, column)

	readAttributes(scan)
	selfClosing := scan.peek() == '/'
	if selfClosing {
		scan.advance()
	}
	if scan.peek() == '>' {
		scan.advance()
	}

	if html && !closing && !selfClosing {
		readEmbedded(scan, name)
	}
}

// readMarkupName lit un nom de balise ou d'attribut. Le tiret et les deux-points
// en font partie : « data-role » et « xsl:template » sont des noms entiers.
func readMarkupName(scan *scanner) string {
	start := scan.offset
	for !scan.done() {
		char := scan.peek()
		if !isIdentifierPart(char) && char != '-' && char != ':' && char != '.' {
			break
		}
		scan.advance()
	}
	return strings.ToLower(scan.source[start:scan.offset])
}

// readAttributes lit les attributs jusqu'à la fermeture de la balise.
func readAttributes(scan *scanner) {
	for !scan.done() {
		for !scan.done() && isSpace(scan.peek()) {
			scan.advance()
		}
		char := scan.peek()
		if char == 0 || char == '>' || char == '/' {
			return
		}
		if !isIdentifierStart(char) {
			// Un caractère inattendu : on l'avale plutôt que de boucler sur
			// place. Le balisage d'un travail d'étudiant n'est pas toujours
			// valide, et l'analyse doit tout de même finir.
			scan.advance()
			continue
		}
		offset, line, column := scan.mark()
		name := readMarkupName(scan)
		scan.emit(Attribute, name, offset, line, column)

		for !scan.done() && isSpace(scan.peek()) {
			scan.advance()
		}
		if scan.peek() != '=' {
			continue
		}
		scan.advance()
		for !scan.done() && isSpace(scan.peek()) {
			scan.advance()
		}
		readAttributeValue(scan)
	}
}

// readAttributeValue avale la valeur d'un attribut, entre guillemets ou nue.
func readAttributeValue(scan *scanner) {
	offset, line, column := scan.mark()
	switch quoted := scan.peek(); quoted {
	case '"', '\'':
		scan.advance()
		for !scan.done() && scan.peek() != quoted {
			scan.advance()
		}
		scan.advance()
	default:
		for !scan.done() && !isSpace(scan.peek()) &&
			scan.peek() != '>' && scan.peek() != '/' {
			scan.advance()
		}
	}
	scan.emit(Text, AnyText, offset, line, column)
}

// embedded dit quel langage se cache derrière une balise de page.
var embedded = map[string]string{"script": "javascript", "style": "css"}

// readEmbedded analyse le contenu d'un « script » ou d'un « style » avec son
// propre langage, puis recoud le résultat à sa place dans le flux.
func readEmbedded(scan *scanner, name string) {
	language, nested := embedded[name]
	if !nested {
		return
	}
	offset, line := scan.offset, scan.line
	closing := "</" + name
	for !scan.done() && !hasFold(scan.source[scan.offset:], closing) {
		scan.advance()
	}
	inner := Lex(language, []byte(scan.source[offset:scan.offset]))
	scan.splice(inner, offset, line)
}

// hasFold dit si une source commence par un préfixe, sans égard à la casse :
// « </SCRIPT> » ferme aussi bien que « </script> ».
func hasFold(source, prefix string) bool {
	if len(source) < len(prefix) {
		return false
	}
	return strings.EqualFold(source[:len(prefix)], prefix)
}

// readProse avale du texte jusqu'à ce que stop soit vrai, en émettant un jeton
// par mot. La ponctuation est écartée : deux copies qui ne diffèrent que par
// leurs virgules sont la même copie.
func readProse(scan *scanner, stop func() bool) {
	start, line, column := -1, 0, 0
	flush := func() {
		if start < 0 {
			return
		}
		word := strings.ToLower(scan.source[start:scan.offset])
		scan.tokens = append(scan.tokens, Token{
			Kind: Word, Text: word,
			Line: line, Column: column, Start: start, End: scan.offset,
		})
		start = -1
	}
	for !scan.done() && !stop() {
		if isIdentifierPart(scan.peek()) {
			if start < 0 {
				start, line, column = scan.mark()
			}
		} else {
			flush()
		}
		scan.advance()
	}
	flush()
}
