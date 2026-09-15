package tokens_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/tokens"
)

// textes rend le flux comparé, pour le lire d'un coup d'œil dans un message
// d'échec.
func textes(result tokens.Result) []string {
	flux := make([]string, 0, len(result.Tokens))
	for _, jeton := range result.Tokens {
		flux = append(flux, jeton.Text)
	}
	return flux
}

func flux(t *testing.T, langage, source string) string {
	t.Helper()
	return strings.Join(textes(tokens.Lex(langage, []byte(source))), " ")
}

// C'est la propriété qui justifie tout le paquet : renommer ses variables ne
// change rien au flux comparé. Sans elle, le maquillage le moins coûteux qui
// soit suffirait à passer au travers.
func TestRenommerNeChangeRienAuFlux(t *testing.T) {
	original := `
		int total = 0;
		for (int index = 0; index < liste.length; index++) {
			total += liste[index];
		}`
	maquille := `
		int somme = 0;
		for (int i = 0; i < tableau.length; i++) {
			somme += tableau[i];
		}`
	if premier, second := flux(t, "java", original), flux(t, "java", maquille); premier != second {
		t.Fatalf("les deux flux diffèrent :\n%s\n%s", premier, second)
	}
}

func TestLesLitterauxSontEffaces(t *testing.T) {
	obtenu := flux(t, "java", `String message = "bonjour"; int âge = 42;`)
	attendu := "ID ID = STR ; int ID = NUM ;"
	if obtenu != attendu {
		t.Fatalf("flux : %q, attendu %q", obtenu, attendu)
	}
}

// Les commentaires sortent du flux : deux copies qui ne diffèrent que par leurs
// commentaires doivent rester appariées. Mais ils ne sont pas jetés — un
// commentaire identique est un signal à lui seul.
func TestLesCommentairesSortentDuFluxSansEtreJetes(t *testing.T) {
	source := "// Calcule la moyenne\nint x = 1; /* et voilà */\n"
	result := tokens.Lex("java", []byte(source))

	if obtenu := strings.Join(textes(result), " "); obtenu != "int ID = NUM ;" {
		t.Fatalf("flux : %q", obtenu)
	}
	if len(result.Comments) != 2 {
		t.Fatalf("commentaires relevés : %+v", result.Comments)
	}
	if result.Comments[0].Text != "calcule la moyenne" {
		t.Fatalf("commentaire réduit : %q", result.Comments[0].Text)
	}
	if result.Comments[0].Line != 1 || result.Comments[1].Line != 2 {
		t.Fatalf("lignes des commentaires : %+v", result.Comments)
	}
}

// Le même commentaire recopié ailleurs, réindenté et redécoré, doit se
// reconnaître : c'est la réduction qui le permet.
func TestUnCommentaireSeReconnaitMalgreSaDecoration(t *testing.T) {
	premier := tokens.Lex("java", []byte("/** Vérifie   le solde */\n"))
	second := tokens.Lex("java", []byte("      // Vérifie le solde\n"))
	if premier.Comments[0].Text != second.Comments[0].Text {
		t.Fatalf("%q ≠ %q", premier.Comments[0].Text, second.Comments[0].Text)
	}
}

// Sans positions exactes, on sait que deux fichiers se ressemblent sans pouvoir
// le montrer — et un rapport invérifiable ne vaut rien.
func TestLesPositionsSurviventAuxAccents(t *testing.T) {
	result := tokens.Lex("java", []byte("int é = 1;\nint b = 2;\n"))
	// « é » tient sur deux octets : la colonne doit compter des caractères.
	egal := result.Tokens[2]
	if egal.Text != "=" || egal.Line != 1 || egal.Column != 7 {
		t.Fatalf("jeton « = » : %+v", egal)
	}
	deuxieme := result.Tokens[5]
	if deuxieme.Text != "int" || deuxieme.Line != 2 || deuxieme.Column != 1 {
		t.Fatalf("« int » de la deuxième ligne : %+v", deuxieme)
	}
}

// Une source invalide — un guillemet oublié, un commentaire jamais refermé —
// est le cas courant dans un travail d'étudiant, pas l'exception. L'analyse
// doit finir et rendre ce qu'elle peut.
func TestUneSourceInvalideNAvalePasToutLeFichier(t *testing.T) {
	obtenu := flux(t, "java", "String x = \"jamais fermée\nint y = 2;\n")
	if !strings.Contains(obtenu, "int") {
		t.Fatalf("la chaîne ouverte a avalé la suite : %q", obtenu)
	}

	result := tokens.Lex("java", []byte("/* jamais refermé\nint y = 2;\n"))
	if len(result.Tokens) != 0 {
		t.Fatalf("un commentaire ouvert doit courir jusqu'au bout : %v", textes(result))
	}
}

func TestSQLIgnoreLaCasseDesMotsCles(t *testing.T) {
	haut := flux(t, "sql", "SELECT nom FROM Etudiants WHERE id = 3;")
	bas := flux(t, "sql", "select nom from Etudiants where id = 3;")
	if haut != bas {
		t.Fatalf("%q ≠ %q", haut, bas)
	}
	if !strings.HasPrefix(haut, "select ID from ID where ID = NUM") {
		t.Fatalf("flux SQL : %q", haut)
	}
}

// En CSS, les « identifiants » viennent du langage — « display », « flex ». Les
// effacer ferait de toute feuille de style la même. Ce que l'auteur choisit,
// lui, doit disparaître : le nom d'une classe ou d'une propriété personnalisée.
func TestCSSGardeLesProprietesEtEffaceLesNomsChoisis(t *testing.T) {
	obtenu := flux(t, "css", ".ma-classe { display: flex; margin-left: 4px; }")
	attendu := ". ID { display : flex ; margin-left : NUM ; }"
	if obtenu != attendu {
		t.Fatalf("flux : %q, attendu %q", obtenu, attendu)
	}

	custom := flux(t, "css", ":root { --couleur-titre: #fff; }")
	if !strings.Contains(custom, "ID :") {
		t.Fatalf("une propriété personnalisée doit être effacée : %q", custom)
	}
	if negatif := flux(t, "css", "p { margin: -10px; }"); !strings.Contains(negatif, "NUM") {
		t.Fatalf("une valeur négative est un nombre : %q", negatif)
	}
}

// Python n'a pas d'accolades : sans jetons d'indentation, deux structures
// différentes donneraient le même flux.
func TestPythonDistingueDeuxStructuresParLIndentation(t *testing.T) {
	imbrique := "for a in b:\n    for c in d:\n        print(c)\n"
	enchaine := "for a in b:\n    print(a)\nfor c in d:\n    print(c)\n"
	if flux(t, "python", imbrique) == flux(t, "python", enchaine) {
		t.Fatal("deux structures différentes donnent le même flux")
	}
	if obtenu := flux(t, "python", imbrique); !strings.Contains(obtenu, "⇥ for") {
		t.Fatalf("aucun bloc ouvert : %q", obtenu)
	}
}

func TestPythonIgnoreLesLignesVidesEtLesPrefixesDeChaine(t *testing.T) {
	avec := flux(t, "python", "def f():\n    x = 1\n\n    y = 2\n")
	sans := flux(t, "python", "def f():\n    x = 1\n    y = 2\n")
	if avec != sans {
		t.Fatalf("une ligne blanche ne ferme pas un bloc :\n%s\n%s", avec, sans)
	}
	if obtenu := flux(t, "python", "x = f\"bonjour {nom}\"\n"); obtenu != "ID = STR" {
		t.Fatalf("préfixe de chaîne : %q", obtenu)
	}
}

// Une valeur d'attribut change d'un travail à l'autre sans rien dire ; le nom
// de la balise et celui de l'attribut, eux, sont la structure de la page.
func TestHTMLGardeLaStructureEtEffaceLesValeurs(t *testing.T) {
	obtenu := flux(t, "html", `<div class="carte"><p>Bonjour tout le monde</p></div>`)
	attendu := "<div class STR <p bonjour tout le monde </p </div"
	if obtenu != attendu {
		t.Fatalf("flux : %q, attendu %q", obtenu, attendu)
	}
}

// Un script posé dans une page est du JavaScript, pas du balisage. Le lire
// comme une suite de mots le rendrait méconnaissable.
func TestHTMLAnalyseLesScriptsCommeDuCode(t *testing.T) {
	result := tokens.Lex("html", []byte("<script>\nconst total = 1 + 2;\n</script>\n"))
	obtenu := strings.Join(textes(result), " ")
	if !strings.Contains(obtenu, "const ID = NUM + NUM ;") {
		t.Fatalf("le script n'a pas été analysé comme du code : %q", obtenu)
	}
	// Recousu à sa place : le « const » est bien sur la deuxième ligne.
	for _, jeton := range result.Tokens {
		if jeton.Text == "const" && jeton.Line != 2 {
			t.Fatalf("position du script mal recousue : %+v", jeton)
		}
	}
}

func TestMarkdownCompareDesMotsEtAnalyseLesBlocsDeCode(t *testing.T) {
	obtenu := flux(t, "markdown", "# Titre du travail\n\nBonjour, tout le monde.\n")
	if obtenu != "titre du travail bonjour tout le monde" {
		t.Fatalf("flux : %q", obtenu)
	}

	source := "Exemple :\n\n```java\nint total = 0;\n```\n\nFin.\n"
	result := tokens.Lex("markdown", []byte(source))
	rendu := strings.Join(textes(result), " ")
	if !strings.Contains(rendu, "int ID = NUM ;") {
		t.Fatalf("le bloc de code n'a pas été analysé comme du Java : %q", rendu)
	}
	if !strings.HasSuffix(rendu, "fin") {
		t.Fatalf("la prose qui suit le bloc a disparu : %q", rendu)
	}
	for _, jeton := range result.Tokens {
		if jeton.Text == "int" && jeton.Line != 4 {
			t.Fatalf("position du bloc mal recousue : %+v", jeton)
		}
	}
}

func TestMarkdownLaisseEnProseUnBlocSansLangageConnu(t *testing.T) {
	rendu := flux(t, "markdown", "```rust\nfn main() {}\n```\n")
	if !strings.Contains(rendu, "fn") || !strings.Contains(rendu, "main") {
		t.Fatalf("le bloc devait rester de la prose : %q", rendu)
	}
}

func TestDetectionParExtension(t *testing.T) {
	cas := map[string]string{
		"src/Main.java":       "java",
		"lib/widget.dart":     "dart",
		"app/page.tsx":        "tsx",
		"Program.CS":          "csharp",
		"docs/README.md":      "markdown",
		"styles/app.scss":     "css",
		"schema.sql":          "sql",
		"pages/index.html":    "html",
		"pom.xml":             "xml",
		"scripts/outil.py":    "python",
		"src/index.mjs":       "javascript",
		"src/types.ts":        "typescript",
		"app/Modele.kt":       "kotlin",
		"images/logo.ico":     "",
		"Makefile":            "",
		"main.go":             "",
		"assets/police.woff2": "",
	}
	for chemin, attendu := range cas {
		langage, reconnu := tokens.Detect(chemin)
		if attendu == "" {
			if reconnu {
				t.Fatalf("« %s » a été reconnu comme %q", chemin, langage.ID)
			}
			continue
		}
		if !reconnu || langage.ID != attendu {
			t.Fatalf("« %s » → %q, attendu %q", chemin, langage.ID, attendu)
		}
	}
}

func TestUnLangageInconnuNeProduitRien(t *testing.T) {
	if result := tokens.Lex("cobol", []byte("DISPLAY 'x'.")); !result.Empty() {
		t.Fatalf("un langage inconnu a produit des jetons : %v", textes(result))
	}
}

// Les bornes de winnowing dépendent du langage : vingt-trois mots de prose
// couvrent un paragraphe là où vingt-trois jetons de Java couvrent deux lignes.
func TestChaqueLangageAPorteSesBornes(t *testing.T) {
	for _, langage := range tokens.All() {
		if langage.Kgram < 2 || langage.Window < 1 {
			t.Fatalf("%s : bornes invalides (k=%d, w=%d)",
				langage.ID, langage.Kgram, langage.Window)
		}
		if len(langage.Extensions) == 0 || langage.Label == "" {
			t.Fatalf("%s : langage incomplet", langage.ID)
		}
	}
	if prose, _ := tokens.Get("markdown"); prose.Kgram >= tokens.CodeKgram {
		t.Fatalf("la prose doit avoir une borne plus courte que le code : %d", prose.Kgram)
	}
}
