package tokens

import "strings"

// Le markdown est de la prose, et se compare comme telle : un jeton par mot,
// sans la ponctuation. Ce qui s'y recopie est un paragraphe, une consigne
// reformulée, un rapport ; le nombre de virgules n'en dit rien.
//
// Les blocs de code clôturés font exception. Un extrait posé dans un README est
// du code, pas de la prose, et le lire comme une suite de mots le rendrait
// méconnaissable — « for » et « ID » y pèseraient autant que « le » et « la ».
// Quand la clôture annonce un langage connu, le bloc est donc analysé avec cet
// analyseur-là, puis recousu à sa place.

func lexMarkdown(source string) Result {
	scan := newScanner(source)
	for !scan.done() {
		switch {
		case scan.has("<!--"):
			readMarkupComment(scan)
		case openingFence(scan) != "":
			readFence(scan)
		default:
			readProse(scan, func() bool {
				return scan.has("<!--") || openingFence(scan) != ""
			})
		}
	}
	return scan.result()
}

// MaxFenceIndent est le décalage qu'une clôture peut avoir sans cesser d'en
// être une. Au-delà de trois espaces, c'est un bloc indenté, pas une clôture :
// c'est la règle de CommonMark, et s'en écarter ferait avaler la moitié d'un
// fichier au premier extrait mal aligné.
const MaxFenceIndent = 3

// openingFence rend la clôture qui commence ici — « ``` » ou « ~~~ » —, ou une
// chaîne vide. Elle n'existe qu'en début de ligne.
func openingFence(scan *scanner) string {
	if !atLineStart(scan) {
		return ""
	}
	rest := scan.source[scan.offset:]
	indent := 0
	for indent < len(rest) && rest[indent] == ' ' && indent <= MaxFenceIndent {
		indent++
	}
	if indent > MaxFenceIndent {
		return ""
	}
	return fenceAt(rest[indent:])
}

// fenceAt rend la suite de caractères de clôture en tête d'une ligne.
func fenceAt(line string) string {
	for _, char := range []byte{'`', '~'} {
		length := 0
		for length < len(line) && line[length] == char {
			length++
		}
		if length >= 3 {
			return line[:length]
		}
	}
	return ""
}

// atLineStart dit que le curseur est au début d'une ligne.
func atLineStart(scan *scanner) bool {
	return scan.offset == 0 || scan.source[scan.offset-1] == '\n'
}

// readFence avale un bloc de code clôturé.
func readFence(scan *scanner) {
	fence := openingFence(scan)
	// L'en-tête de clôture porte le langage : « ```kotlin ». Ce qui suit le
	// premier mot — « ```js title="x" » — ne le nomme pas et ne sert à rien ici.
	header := lineAt(scan.source, scan.offset)
	info := strings.TrimSpace(strings.TrimLeft(header, " `~"))
	language, _, _ := strings.Cut(info, " ")
	skipLine(scan)

	offset, line := scan.offset, scan.line
	end := closingFence(scan.source, scan.offset, fence)

	if inner, known := Get(language); known {
		content := scan.source[offset:end]
		spliced := inner.lex(content)
		spliced.Language = inner.ID
		scan.splice(spliced, offset, line)
		for scan.offset < end {
			scan.advance()
		}
	} else {
		// Langage inconnu ou absent : le bloc reste de la prose. C'est moins
		// fin, mais cela n'invente rien — analyser du Rust avec la table du
		// Java produirait un flux qui ne veut rien dire.
		readProse(scan, func() bool { return scan.offset >= end })
	}
	if !scan.done() {
		skipLine(scan)
	}
}

// closingFence rend l'endroit où le contenu d'un bloc s'arrête : le début de la
// ligne qui le referme, ou la fin du fichier quand rien ne le referme.
func closingFence(source string, from int, fence string) int {
	for offset := from; offset < len(source); {
		line := lineAt(source, offset)
		trimmed := strings.TrimLeft(line, " ")
		if len(line)-len(trimmed) <= MaxFenceIndent {
			if closing := fenceAt(trimmed); len(closing) >= len(fence) &&
				closing[0] == fence[0] {
				return offset
			}
		}
		next := strings.IndexByte(source[offset:], '\n')
		if next < 0 {
			return len(source)
		}
		offset += next + 1
	}
	return len(source)
}

// lineAt rend la ligne qui commence à cet endroit, sans son saut de ligne.
func lineAt(source string, offset int) string {
	rest := source[offset:]
	if end := strings.IndexByte(rest, '\n'); end >= 0 {
		return strings.TrimSuffix(rest[:end], "\r")
	}
	return rest
}

// skipLine avance jusqu'au début de la ligne suivante.
func skipLine(scan *scanner) {
	for !scan.done() && scan.peek() != '\n' {
		scan.advance()
	}
	scan.advance()
}
