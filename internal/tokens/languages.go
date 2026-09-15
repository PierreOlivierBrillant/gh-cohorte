package tokens

import (
	"path"
	"strings"
)

// Language est un langage reconnu : comment le nommer, à quelles extensions il
// répond, comment l'analyser, et avec quelles bornes de winnowing.
//
// Les bornes vivent ici parce qu'elles dépendent du langage et de rien d'autre.
// Un jeton de markdown est un mot ; un jeton de Java est une accolade ou un
// « ID ». Vingt-trois jetons de Java couvrent deux ou trois lignes ; vingt-trois
// mots de prose couvrent un paragraphe. La même borne ne peut pas servir aux
// deux, et l'imposer rendrait la prose aveugle ou le code bavard.
type Language struct {
	ID    string
	Label string
	// Extensions sont écrites en minuscules, le point compris.
	Extensions []string
	// Kgram est le nombre de jetons d'un k-gramme, Window la taille de la
	// fenêtre de winnowing. Ce sont des défauts : un réglage d'analyse les
	// remplace.
	Kgram  int
	Window int

	lex func(string) Result
}

// Bornes par défaut, reprises de Dolos pour le code. Elles ont fait leurs
// preuves sur des travaux d'étudiants, et il n'y a pas de raison de s'en
// écarter sans mesure à l'appui.
const (
	CodeKgram   = 23
	CodeWindow  = 17
	MarkupKgram = 15
	// MarkupWindow et ProseWindow sont plus petits : un fichier de balisage ou
	// de prose est court, et une grande fenêtre n'y retiendrait presque rien.
	MarkupWindow = 11
	ProseKgram   = 12
	ProseWindow  = 8
)

// languages énumère ce que l'outil sait lire. L'ordre est celui où les
// proposer : les langages de cours d'abord, le balisage ensuite.
var languages = []Language{
	{ID: "java", Label: "Java", Extensions: []string{".java"},
		Kgram: CodeKgram, Window: CodeWindow, lex: lexer(javaSyntax)},
	{ID: "kotlin", Label: "Kotlin", Extensions: []string{".kt", ".kts"},
		Kgram: CodeKgram, Window: CodeWindow, lex: lexer(kotlinSyntax)},
	{ID: "csharp", Label: "C#", Extensions: []string{".cs"},
		Kgram: CodeKgram, Window: CodeWindow, lex: lexer(csharpSyntax)},
	{ID: "dart", Label: "Dart", Extensions: []string{".dart"},
		Kgram: CodeKgram, Window: CodeWindow, lex: lexer(dartSyntax)},
	{ID: "python", Label: "Python", Extensions: []string{".py", ".pyw"},
		Kgram: CodeKgram, Window: CodeWindow, lex: lexPython},
	{ID: "javascript", Label: "JavaScript",
		Extensions: []string{".js", ".mjs", ".cjs", ".jsx"},
		Kgram:      CodeKgram, Window: CodeWindow, lex: lexer(javascriptSyntax)},
	{ID: "typescript", Label: "TypeScript",
		Extensions: []string{".ts", ".mts", ".cts"},
		Kgram:      CodeKgram, Window: CodeWindow, lex: lexer(typescriptSyntax)},
	{ID: "tsx", Label: "TSX", Extensions: []string{".tsx"},
		Kgram: CodeKgram, Window: CodeWindow, lex: lexer(typescriptSyntax)},
	{ID: "sql", Label: "SQL", Extensions: []string{".sql"},
		Kgram: CodeKgram, Window: CodeWindow, lex: lexer(sqlSyntax)},
	{ID: "css", Label: "CSS", Extensions: []string{".css", ".scss", ".sass", ".less"},
		Kgram: MarkupKgram, Window: MarkupWindow, lex: lexer(cssSyntax)},
	{ID: "html", Label: "HTML", Extensions: []string{".html", ".htm"},
		Kgram: MarkupKgram, Window: MarkupWindow, lex: lexHTML},
	{ID: "xml", Label: "XML", Extensions: []string{".xml", ".xsd", ".xsl", ".xslt"},
		Kgram: MarkupKgram, Window: MarkupWindow, lex: lexXML},
	{ID: "markdown", Label: "Markdown", Extensions: []string{".md", ".markdown"},
		Kgram: ProseKgram, Window: ProseWindow, lex: lexMarkdown},
}

// byID et byExtension sont construits une fois : la sélection des fichiers les
// interroge une fois par fichier, et il y en a des dizaines de milliers.
//
// Ils se remplissent dans « init » plutôt qu'à l'initialisation de la variable,
// parce qu'un analyseur peut en appeler un autre — le « script » d'une page est
// du JavaScript — et que la table des langages dépendrait alors d'elle-même.
var (
	byID        = map[string]Language{}
	byExtension = map[string]Language{}
)

func init() {
	for _, language := range languages {
		byID[language.ID] = language
		for _, extension := range language.Extensions {
			byExtension[extension] = language
		}
	}
}

// All énumère les langages reconnus, dans l'ordre où les proposer.
func All() []Language {
	all := make([]Language, len(languages))
	copy(all, languages)
	return all
}

// IDs rend les identifiants des langages reconnus.
func IDs() []string {
	ids := make([]string, 0, len(languages))
	for _, language := range languages {
		ids = append(ids, language.ID)
	}
	return ids
}

// Get retrouve un langage par son identifiant, insensible à la casse.
func Get(id string) (Language, bool) {
	language, known := byID[strings.ToLower(strings.TrimSpace(id))]
	return language, known
}

// Detect devine le langage d'un fichier par son extension.
//
// Le nom seul suffit : le contenu ne sert pas à trancher. Deviner un langage au
// contenu est possible mais se trompe, et une erreur ici n'est pas anodine —
// analyser du Java comme du SQL donne un flux qui ne ressemble à rien, sans
// qu'aucun message ne le dise. Une extension inconnue vaut mieux : le fichier
// est écarté, et le rapport le nomme.
func Detect(name string) (Language, bool) {
	language, known := byExtension[strings.ToLower(path.Ext(name))]
	return language, known
}

// ------------------------------------------------------- tables de syntaxe

// syntax décrit ce qu'un analyseur générique doit savoir d'un langage de la
// famille C : ses mots réservés, ses commentaires, ses chaînes.
//
// Neuf des treize langages tiennent dans cette table. Écrire neuf analyseurs
// distincts pour dire neuf fois la même chose serait neuf fois l'occasion de se
// tromper ; ce qui diffère vraiment — Python, le balisage, la prose — a son
// analyseur, et celui-là seulement.
type syntax struct {
	keywords map[string]bool
	// fold rend les mots-clés insensibles à la casse : « SELECT » et « select »
	// sont le même mot en SQL, et les distinguer casserait l'appariement entre
	// deux copies qui ne diffèrent que par le style d'écriture.
	fold bool
	// keepIdentifiers garde les noms tels quels au lieu de les réduire à « ID ».
	// C'est vrai du seul CSS : ses « identifiants » sont des noms de propriétés
	// — « display », « flex-direction » —, choisis par le langage et non par qui
	// écrit. Les effacer ferait de toute feuille de style la même feuille.
	keepIdentifiers bool
	lineComments    []string
	blockComments   [][2]string
	quotes          []quote
}

// quote est une forme de littéral textuel.
type quote struct {
	open, close string
	// escape dit que la barre oblique inverse protège le caractère suivant.
	escape bool
	// multiline autorise le passage à la ligne — les triples guillemets, les
	// gabarits entre accents graves.
	multiline bool
}

// Formes de chaînes communes à presque toute la famille C.
var cLikeQuotes = []quote{
	{open: `"`, close: `"`, escape: true},
	{open: `'`, close: `'`, escape: true},
}

var slashComments = []string{"//"}
var starComments = [][2]string{{"/*", "*/"}}

func words(list string) map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(list) {
		set[word] = true
	}
	return set
}

// reserved rend les mots-clés d'un langage, une fois retirés ceux qui n'en sont
// pas vraiment.
//
// Beaucoup de langages ont des mots-clés contextuels : « record » en Java,
// « value » en C#, « type » en TypeScript, « date » en SQL. Ils ne sont
// réservés qu'à un endroit précis de la grammaire, et partout ailleurs ce sont
// des noms ordinaires — des noms que les étudiants emploient beaucoup, parce
// que ce sont justement les mots qui décrivent bien une variable.
//
// Les garder comme mots-clés rendrait la détection dépendante du nom choisi :
// deux copies identiques dont l'une appelle sa variable « record » cesseraient
// de se ressembler. C'est exactement ce que la normalisation existe pour
// empêcher. Ils sont donc traités comme des noms, et rejoignent « ID ».
//
// Le sens inverse — garder un mot structurant comme « private » ou « suspend »,
// que personne n'emploie comme nom de variable — coûte, lui, une simple perte
// de finesse si l'on se trompe. Le doute penche donc du côté de ce qui ne peut
// pas nuire.
func reserved(all, contextual string) map[string]bool {
	set := words(all)
	for _, word := range strings.Fields(contextual) {
		delete(set, word)
	}
	return set
}

var javaSyntax = syntax{
	keywords: reserved(`abstract assert boolean break byte case catch char class
		const continue default do double else enum extends final finally float
		for goto if implements import instanceof int interface long native new
		package private protected public return short static strictfp super
		switch synchronized this throw throws transient try void volatile while
		var record sealed permits yield true false null`,
		`var record sealed permits yield`),
	lineComments:  slashComments,
	blockComments: starComments,
	quotes: append([]quote{
		{open: `"""`, close: `"""`, escape: true, multiline: true},
	}, cLikeQuotes...),
}

var kotlinSyntax = syntax{
	keywords: reserved(`as break class continue do else false for fun if in
		interface is null object package return super this throw true try
		typealias val var when while by catch constructor delegate dynamic
		field file finally get import init param property receiver set setparam
		where actual abstract annotation companion const crossinline data enum
		expect external final infix inline inner internal lateinit noinline open
		operator out override private protected public reified sealed suspend
		tailrec vararg`,
		`by delegate dynamic field file get set init param property receiver
		setparam where actual expect data value out open`),
	lineComments:  slashComments,
	blockComments: starComments,
	quotes: append([]quote{
		{open: `"""`, close: `"""`, multiline: true},
	}, cLikeQuotes...),
}

var csharpSyntax = syntax{
	keywords: reserved(`abstract as base bool break byte case catch char checked
		class const continue decimal default delegate do double else enum event
		explicit extern false finally fixed float for foreach goto if implicit
		in int interface internal is lock long namespace new null object
		operator out override params private protected public readonly ref
		return sbyte sealed short sizeof stackalloc static string struct switch
		this throw true try typeof uint ulong unchecked unsafe ushort using
		virtual void volatile while var async await dynamic nameof partial
		record init with when where yield get set value global`,
		`var value record get set dynamic nameof global init`),
	lineComments:  slashComments,
	blockComments: starComments,
	quotes: append([]quote{
		{open: `"""`, close: `"""`, multiline: true},
		{open: `@"`, close: `"`, multiline: true},
	}, cLikeQuotes...),
}

var dartSyntax = syntax{
	keywords: reserved(`abstract as assert async await break case catch class
		const continue covariant default deferred do dynamic else enum export
		extends extension external factory false final finally for get hide if
		implements import in interface is late library mixin new null on
		operator part required rethrow return set show static super switch sync
		this throw true try typedef var void while with yield`,
		`get set dynamic on part show hide sync late required covariant deferred`),
	lineComments:  slashComments,
	blockComments: starComments,
	quotes: append([]quote{
		{open: `"""`, close: `"""`, escape: true, multiline: true},
		{open: `'''`, close: `'''`, escape: true, multiline: true},
	}, cLikeQuotes...),
}

// javascriptKeywords sert aussi à TypeScript, qui n'en retire aucun.
const javascriptKeywords = `await break case catch class const continue
	debugger default delete do else enum export extends false finally for
	function if import in instanceof new null return super switch this throw
	true try typeof var void while with yield let static async of get set`

// javascriptSoft et typescriptSoft nomment ce que ces langages réservent sans
// l'interdire. La liste de TypeScript est longue parce que tout son vocabulaire
// de types — « type », « string », « number », « any » — est contextuel, et que
// ce sont les noms de variables les plus courants qui soient.
const javascriptSoft = `of get set from async static`

var javascriptSyntax = syntax{
	keywords:      reserved(javascriptKeywords, javascriptSoft),
	lineComments:  slashComments,
	blockComments: starComments,
	quotes: append([]quote{
		{open: "`", close: "`", escape: true, multiline: true},
	}, cLikeQuotes...),
}

var typescriptSyntax = syntax{
	keywords: reserved(javascriptKeywords+`
		abstract as asserts bigint boolean declare implements infer interface is
		keyof module namespace never number object private protected public
		readonly require string symbol type undefined unique unknown from
		satisfies override out accessor`,
		javascriptSoft+` type string number object boolean symbol any unknown
		never undefined module namespace require out accessor bigint is`),
	lineComments:  slashComments,
	blockComments: starComments,
	quotes: append([]quote{
		{open: "`", close: "`", escape: true, multiline: true},
	}, cLikeQuotes...),
}

var sqlSyntax = syntax{
	keywords: reserved(`select from where insert update delete create alter drop
		table view index trigger procedure function database schema join inner
		left right full outer cross on using group by order asc desc having
		limit offset fetch first rows only union all distinct as and or not null
		is in between like ilike exists case when then else end primary key
		foreign references unique check default constraint cascade restrict set
		values into begin commit rollback transaction with recursive returning
		int integer bigint smallint serial varchar char text boolean date time
		timestamp numeric decimal real double precision blob add column rename
		to if replace temporary grant revoke count sum avg min max coalesce`,
		`count sum avg min max coalesce date time text key value first to add
		replace database schema only rows`),
	fold:          true,
	lineComments:  []string{"--", "#"},
	blockComments: starComments,
	quotes: []quote{
		{open: `'`, close: `'`, multiline: true},
		{open: `"`, close: `"`, multiline: true},
	},
}

var cssSyntax = syntax{
	keepIdentifiers: true,
	lineComments:    slashComments,
	blockComments:   starComments,
	quotes:          cLikeQuotes,
}
