// Package tokens réduit un fichier source à un flux de jetons comparables.
//
// Comparer deux fichiers caractère par caractère ne dit rien : renommer ses
// variables, réindenter, changer les guillemets suffit à tout brouiller. Ce
// qu'on compare ici est un flux de jetons normalisés — tout identifiant devient
// « ID », tout nombre « NUM », toute chaîne « STR », et seuls les mots-clés et
// la ponctuation gardent leur forme. Deux copies qui ne diffèrent que par les
// noms donnent alors exactement le même flux, et le maquillage le plus courant
// ne sert plus à rien.
//
// Les analyseurs sont écrits à la main, en Go pur. Un arbre syntaxique — ce que
// Dolos tire de tree-sitter — serait un peu plus fin, mais tree-sitter impose
// cgo, et l'extension est distribuée précompilée pour six cibles : elle doit se
// construire sans compilateur C. Le winnowing travaille de toute façon sur un
// flux de jetons, jamais sur l'arbre.
//
// Les commentaires sont retirés du flux et rendus à part. Ce n'est pas du
// mépris : deux commentaires identiques, fautes de frappe comprises, sont
// souvent plus parlants qu'un score. Les noyer dans le flux les dilue ; les en
// sortir en fait un signal à eux seuls.
package tokens

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind dit ce qu'un jeton était avant d'être normalisé. Le flux comparé ne
// retient que Text ; Kind sert à l'afficher — c'est lui qui colore le mode
// « jetons » de la vue de comparaison, où l'on montre ce que le moteur a
// réellement comparé.
type Kind uint8

// Les natures de jetons. Toutes n'existent pas dans tous les langages : un
// fichier Java n'a ni Word ni Tag, un markdown n'a que des Word.
const (
	// Keyword est un mot réservé du langage : il garde sa forme.
	Keyword Kind = iota
	// Identifier est un nom choisi par qui écrit. Il devient « ID » : c'est
	// cette normalisation, et elle seule, qui rend le renommage inopérant.
	Identifier
	// Number et Text sont des littéraux, réduits à « NUM » et « STR ». Deux
	// copies qui ne diffèrent que par une constante restent appariées.
	Number
	Text
	// Punctuation est un caractère de ponctuation ou d'opérateur, gardé tel
	// quel, un caractère par jeton.
	Punctuation
	// Word est un mot de prose : markdown, ou le texte entre deux balises.
	Word
	// Tag est un nom de balise, Attribute un nom d'attribut.
	Tag
	Attribute
	// Indent et Dedent portent la structure de Python, où l'indentation tient
	// lieu d'accolades. Sans eux, deux fonctions imbriquées autrement
	// donneraient le même flux.
	Indent
	Dedent
)

// Formes normalisées des jetons dont le contenu ne compte pas.
const (
	AnyIdentifier = "ID"
	AnyNumber     = "NUM"
	AnyText       = "STR"
	OpenBlock     = "⇥"
	CloseBlock    = "⇤"
)

// Token est un jeton du flux comparé.
//
// Text est ce qui est haché ; le reste sert à le remontrer. Les positions sont
// gardées pour une seule raison, mais elle est décisive : sans elles, on sait
// que deux fichiers partagent un fragment sans pouvoir le montrer, et un
// rapport qu'on ne peut pas vérifier ne vaut rien.
type Token struct {
	Kind Kind   `json:"kind"`
	Text string `json:"text"`
	// Line et Column sont comptées à partir de 1, la colonne en runes : c'est
	// ce qu'attend un éditeur, et ce que la vue de comparaison affiche.
	Line   int `json:"line"`
	Column int `json:"column"`
	// Start et End bornent le jeton dans le fichier, en octets.
	Start int `json:"start"`
	End   int `json:"end"`
}

// Comment est un commentaire, mis de côté plutôt que comparé.
type Comment struct {
	// Text est réduit : minuscules, espaces resserrés, marqueurs retirés. Deux
	// commentaires qui ne diffèrent que par leur indentation sont le même.
	Text string `json:"text"`
	Line int    `json:"line"`
}

// Result est ce qu'un fichier donne une fois analysé.
type Result struct {
	Language string    `json:"language"`
	Tokens   []Token   `json:"tokens"`
	Comments []Comment `json:"comments"`
}

// Empty dit qu'il n'y a rien à comparer dans ce fichier.
func (r Result) Empty() bool { return len(r.Tokens) == 0 }

// Lex analyse un contenu dans le langage donné. Un langage inconnu ne produit
// rien plutôt qu'une erreur : le choix des fichiers se fait ailleurs, et un
// appelant qui insiste doit obtenir un résultat vide, pas une panne.
func Lex(language string, content []byte) Result {
	lang, known := Get(language)
	if !known {
		return Result{}
	}
	result := lang.lex(string(content))
	result.Language = lang.ID
	return result
}

// ------------------------------------------------------------------ scanner

// scanner parcourt une source en tenant la ligne et la colonne à jour. Tous les
// analyseurs s'appuient dessus : c'est là, et là seulement, que les positions
// se calculent, si bien qu'aucun analyseur ne peut se tromper de compte.
type scanner struct {
	source string
	// offset est la position courante en octets, line et column ce qu'elle
	// vaut pour un humain.
	offset int
	line   int
	column int

	tokens   []Token
	comments []Comment
}

func newScanner(source string) *scanner {
	return &scanner{source: source, line: 1, column: 1}
}

// done dit que la source est épuisée.
func (s *scanner) done() bool { return s.offset >= len(s.source) }

// peek rend l'octet courant, ou 0 à la fin. Travailler en octets convient :
// tout ce qu'on cherche — guillemets, barres obliques, accolades — est en ASCII,
// et un octet de continuation UTF-8 ne peut pas être confondu avec eux.
func (s *scanner) peek() byte {
	if s.done() {
		return 0
	}
	return s.source[s.offset]
}

// at rend l'octet situé n positions plus loin, ou 0.
func (s *scanner) at(n int) byte {
	if s.offset+n >= len(s.source) {
		return 0
	}
	return s.source[s.offset+n]
}

// has dit si la source continue par ce préfixe.
func (s *scanner) has(prefix string) bool {
	return strings.HasPrefix(s.source[s.offset:], prefix)
}

// advance avance d'un octet en tenant les positions. Seule une rune entière
// fait avancer la colonne : les octets de continuation d'un caractère accentué
// ne comptent pas pour un caractère.
func (s *scanner) advance() {
	if s.done() {
		return
	}
	char := s.source[s.offset]
	s.offset++
	switch {
	case char == '\n':
		s.line++
		s.column = 1
	case char < utf8.RuneSelf || char >= 0xC0:
		// Début de rune : un caractère de plus sur la ligne.
		s.column++
	}
}

// skip avance de n octets.
func (s *scanner) skip(n int) {
	for i := 0; i < n && !s.done(); i++ {
		s.advance()
	}
}

// mark retient la position courante, pour la poser sur le jeton qu'on est en
// train de lire.
func (s *scanner) mark() (offset, line, column int) {
	return s.offset, s.line, s.column
}

// emit ajoute un jeton lu depuis la position retenue.
func (s *scanner) emit(kind Kind, text string, offset, line, column int) {
	s.tokens = append(s.tokens, Token{
		Kind: kind, Text: text,
		Line: line, Column: column, Start: offset, End: s.offset,
	})
}

// emitAt ajoute un jeton sans étendue — c'est le cas d'INDENT et de DEDENT, qui
// ne recouvrent aucun caractère.
func (s *scanner) emitAt(kind Kind, text string, offset, line, column int) {
	s.tokens = append(s.tokens, Token{
		Kind: kind, Text: text,
		Line: line, Column: column, Start: offset, End: offset,
	})
}

// comment range un commentaire de côté, réduit.
func (s *scanner) comment(raw string, line int) {
	if text := reduce(raw); text != "" {
		s.comments = append(s.comments, Comment{Text: text, Line: line})
	}
}

// result rend ce qui a été lu.
func (s *scanner) result() Result {
	return Result{Tokens: s.tokens, Comments: s.comments}
}

// splice recopie le résultat d'un analyseur imbriqué — le contenu d'un bloc de
// code en markdown, d'un « script » en HTML — en replaçant ses positions dans
// le fichier qui le contient.
//
// Le contenu imbriqué commence toujours en début de ligne : la colonne du
// premier jeton se reporte donc telle quelle, sans décalage à corriger.
func (s *scanner) splice(inner Result, offset, line int) {
	for _, token := range inner.Tokens {
		token.Line += line - 1
		token.Start += offset
		token.End += offset
		s.tokens = append(s.tokens, token)
	}
	for _, note := range inner.Comments {
		note.Line += line - 1
		s.comments = append(s.comments, note)
	}
}

// ------------------------------------------------------------------ outils

// reduce met un commentaire à plat : minuscules, espaces resserrés, marqueurs
// de décoration retirés. C'est ce qui permet de reconnaître le même commentaire
// recopié à un autre niveau d'indentation, ou avec une étoile en plus.
func reduce(raw string) string {
	var builder strings.Builder
	space := true
	for _, char := range strings.ToLower(raw) {
		switch {
		case unicode.IsSpace(char):
			if !space {
				builder.WriteByte(' ')
				space = true
			}
		case char == '*' || char == '/' || char == '#' || char == '-':
			// Décoration de bordure : « /** », « *** », « --- ». Seuls les
			// caractères isolés en tête ou en queue sont concernés, et les
			// couper partout donne le même résultat sans avoir à les situer.
			continue
		default:
			builder.WriteRune(char)
			space = false
		}
	}
	return strings.TrimSpace(builder.String())
}

// isIdentifierStart et isIdentifierPart disent ce qui peut composer un nom.
//
// Tout ce qui dépasse l'ASCII y est admis : « prénom » est un identifiant
// valide en Kotlin, en C# comme en Python, et l'écarter couperait un jeton en
// deux au milieu d'un mot — de quoi désaligner tout le reste du fichier.
func isIdentifierStart(char byte) bool {
	return char == '_' || char == '$' || char >= utf8.RuneSelf ||
		(char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

func isIdentifierPart(char byte) bool {
	return isIdentifierStart(char) || (char >= '0' && char <= '9')
}

func isDigit(char byte) bool { return char >= '0' && char <= '9' }

// isSpace ne reconnaît que les blancs ASCII : ce sont les seuls qu'un octet
// isolé peut désigner sans risque au milieu d'une source UTF-8.
func isSpace(char byte) bool {
	return char == ' ' || char == '\t' || char == '\r' || char == '\n' ||
		char == '\v' || char == '\f'
}
