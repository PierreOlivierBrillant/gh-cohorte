package inspect_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
)

func sources(fichiers map[string]string) []inspect.Source {
	liste := make([]inspect.Source, 0, len(fichiers))
	for chemin, contenu := range fichiers {
		liste = append(liste, inspect.Source{Path: chemin, Content: []byte(contenu)})
	}
	return liste
}

func chemins(selection inspect.Selection) []string {
	noms := make([]string, 0, len(selection.Kept))
	for _, fichier := range selection.Kept {
		noms = append(noms, fichier.Path)
	}
	return noms
}

func ecarte(selection inspect.Selection, chemin string) (inspect.Skipped, bool) {
	for _, fichier := range selection.Skipped {
		if fichier.Path == chemin {
			return fichier, true
		}
	}
	return inspect.Skipped{}, false
}

func inspecteur(t *testing.T, settings inspect.Settings) *inspect.Inspector {
	t.Helper()
	inspector, err := inspect.New(settings, nil)
	if err != nil {
		t.Fatalf("réglage refusé : %v", err)
	}
	return inspector
}

// ------------------------------------------------------------------ motifs

func TestUnMotifSansBarreObliquePorteSurUnNom(t *testing.T) {
	accepte := []string{
		"node_modules/react/index.js", "app/node_modules/x.js", "node_modules/x",
	}
	for _, chemin := range accepte {
		if !inspect.Match("node_modules", chemin) {
			t.Fatalf("« node_modules » devrait écarter « %s »", chemin)
		}
	}
	if inspect.Match("node_modules", "src/node_modules_perso.js") {
		t.Fatal("« node_modules » ne doit pas répondre à un nom qui le contient")
	}
	if !inspect.Match("*.min.js", "public/js/jquery.min.js") {
		t.Fatal("« *.min.js » devrait répondre où qu'il soit")
	}
}

func TestUnMotifAvecCheminPorteSurLeCheminEntier(t *testing.T) {
	cas := map[string]bool{
		"app/**":           true,
		"app/**/*.tsx":     true,
		"src/**":           false,
		"**/page.tsx":      true,
		"app/page.tsx":     false,
		"app/*/page.tsx":   true,
		"**/dashboard/**":  true,
		"app/dashboard/**": true,
	}
	const chemin = "app/dashboard/page.tsx"
	for motif, attendu := range cas {
		if obtenu := inspect.Match(motif, chemin); obtenu != attendu {
			t.Fatalf("« %s » contre « %s » : %v, attendu %v",
				motif, chemin, obtenu, attendu)
		}
	}
}

// ------------------------------------------------------ exclusions d'office

func TestCeQuiEstEcarteDOfficeLEstAvantToutLeReste(t *testing.T) {
	selection := inspecteur(t, inspect.Settings{}).Select(sources(map[string]string{
		"depot/src/Main.java":                 "class Main { }",
		"depot/node_modules/react/index.js":   "module.exports = 1;",
		"depot/package-lock.json":             `{"lockfileVersion": 3}`,
		"depot/public/js/jquery.min.js":       "!function(){}();",
		"depot/lib/modele.g.dart":             "// engendré",
		"depot/target/classes/Main.class":     "\x00\x01binaire",
		"depot/assets/logo.ico":               "peu importe",
		"depot/lib/Widget.designer.cs":        "// engendré",
		"depot/.git/config":                   "[core]",
		"depot/android/app/build/rapport.xml": "<x/>",
	}))
	if noms := chemins(selection); len(noms) != 1 || noms[0] != "src/Main.java" {
		t.Fatalf("fichiers retenus : %v", noms)
	}
	// Le motif qui a décidé doit être rendu : sans lui, corriger un profil
	// relève de la divination.
	if fichier, trouve := ecarte(selection, "node_modules/react/index.js"); !trouve ||
		fichier.Reason != inspect.Excluded || fichier.Rule != "node_modules" {
		t.Fatalf("écartement de node_modules : %+v", fichier)
	}
}

func TestUnBinaireEstReconnuParSonContenuMemeSansExtension(t *testing.T) {
	selection := inspecteur(t, inspect.Settings{}).Select(sources(map[string]string{
		"depot/donnees.sql":  "SELECT 1;\x00\x00\x00 binaire déguisé",
		"depot/schema.sql":   "CREATE TABLE etudiants (id INT);",
		"depot/vide.java":    "",
		"depot/notes.txt":    "un fichier texte sans langage connu",
		"depot/Correct.java": "class Correct { }",
	}))
	if noms := chemins(selection); len(noms) != 2 {
		t.Fatalf("fichiers retenus : %v", noms)
	}
	for chemin, attendu := range map[string]string{
		"donnees.sql": inspect.IsBinary,
		"vide.java":   inspect.IsEmpty,
		"notes.txt":   inspect.UnknownTongue,
	} {
		if fichier, trouve := ecarte(selection, chemin); !trouve || fichier.Reason != attendu {
			t.Fatalf("« %s » : %+v, attendu %q", chemin, fichier, attendu)
		}
	}
}

// ------------------------------------------------------- racine du projet

// C'est le piège que tout le reste doit absorber : le même travail, rangé
// autrement. Un profil qui dit « app/ » doit trouver les deux.
func TestDeuxStructuresDifferentesDonnentLesMemesChemins(t *testing.T) {
	inspector := inspecteur(t, inspect.Settings{Profile: "next"})

	plate := inspector.Select(sources(map[string]string{
		"acme-tp1-alice-abc123/app/page.tsx":        "export default function Page() { return null; }",
		"acme-tp1-alice-abc123/app/layout.tsx":      "export default function Layout() { return null; }",
		"acme-tp1-alice-abc123/package.json":        `{"name":"tp1"}`,
		"acme-tp1-alice-abc123/README.md":           "# TP1",
		"acme-tp1-alice-abc123/node_modules/x/i.js": "1",
	}))
	rangee := inspector.Select(sources(map[string]string{
		"acme-tp1-bruno-def456/tp1/mon_tp/app/page.tsx":   "export default function Page() { return null; }",
		"acme-tp1-bruno-def456/tp1/mon_tp/app/layout.tsx": "export default function Layout() { return null; }",
		"acme-tp1-bruno-def456/tp1/mon_tp/package.json":   `{"name":"tp1"}`,
		"acme-tp1-bruno-def456/README.md":                 "# TP1",
	}))

	attendu := []string{"app/layout.tsx", "app/page.tsx"}
	if got := chemins(plate); strings.Join(got, ",") != strings.Join(attendu, ",") {
		t.Fatalf("structure plate : %v", got)
	}
	if got := chemins(rangee); strings.Join(got, ",") != strings.Join(attendu, ",") {
		t.Fatalf("structure rangée : %v", got)
	}
	if rangee.Root != "acme-tp1-bruno-def456/tp1/mon_tp" {
		t.Fatalf("racine devinée : %q", rangee.Root)
	}
}

func TestLaDescenteSArreteLaOuLeProjetCommence(t *testing.T) {
	cas := []struct {
		nom     string
		chemins []string
		racine  string
	}{
		{"préfixe d'archive seul", []string{
			"acme-depot-sha/src/Main.java", "acme-depot-sha/pom.xml",
		}, "acme-depot-sha"},
		{"un seul dossier d'emballage", []string{
			"acme-depot-sha/tp1/src/Main.java", "acme-depot-sha/tp1/pom.xml",
		}, "acme-depot-sha/tp1"},
		{"un README ne retient pas la descente", []string{
			"acme-depot-sha/README.md", "acme-depot-sha/tp1/pom.xml",
			"acme-depot-sha/tp1/src/Main.java",
		}, "acme-depot-sha/tp1"},
		{"du code retient la descente", []string{
			"acme-depot-sha/Main.java", "acme-depot-sha/tp1/Autre.java",
		}, "acme-depot-sha"},
		{"deux dossiers retiennent la descente", []string{
			"acme-depot-sha/client/index.ts", "acme-depot-sha/serveur/index.ts",
		}, "acme-depot-sha"},
		{"la descente est bornée", []string{
			"a/b/c/d/e/f/g/Main.java",
		}, "a/b/c/d"},
	}
	for _, cas := range cas {
		if racine := inspect.Root(cas.chemins); racine != cas.racine {
			t.Fatalf("%s : racine %q, attendue %q", cas.nom, racine, cas.racine)
		}
	}
}

// Descendre ne doit rien perdre : un README resté au-dessus de la racine garde
// son chemin et reste comparable.
func TestDescendreNePerdPasCeQuiEstAuDessus(t *testing.T) {
	selection := inspecteur(t, inspect.Settings{}).Select(sources(map[string]string{
		"depot/README.md":          "# Consignes du travail",
		"depot/tp1/pom.xml":        "<project/>",
		"depot/tp1/src/Main.java":  "class Main { }",
		"depot/tp1/notes/lisez.md": "# Notes",
	}))
	noms := strings.Join(chemins(selection), ",")
	if !strings.Contains(noms, "README.md") || !strings.Contains(noms, "src/Main.java") {
		t.Fatalf("fichiers retenus : %s (racine %q)", noms, selection.Root)
	}
}

// ------------------------------------------------------------------ profils

func TestUnProfilRestreintLesCheminsEtLesLangages(t *testing.T) {
	fichiers := map[string]string{
		"depot/app/page.tsx":           "export default function Page() { return null; }",
		"depot/app/style.css":          ".x { color: red; }",
		"depot/app/page.test.tsx":      "test('x', () => {});",
		"depot/scripts/outil.py":       "print('x')",
		"depot/public/index.html":      "<html></html>",
		"depot/documentation/guide.md": "# Guide",
	}
	selection := inspecteur(t, inspect.Settings{Profile: "next"}).Select(sources(fichiers))
	if noms := strings.Join(chemins(selection), ","); noms != "app/page.tsx,app/style.css" {
		t.Fatalf("profil Next.js : %s", noms)
	}
	if fichier, _ := ecarte(selection, "scripts/outil.py"); fichier.Reason != inspect.OtherTongue {
		t.Fatalf("un Python doit être écarté comme langage : %+v", fichier)
	}
	if fichier, _ := ecarte(selection, "app/page.test.tsx"); fichier.Reason != inspect.OutsideProfile {
		t.Fatalf("un test doit être écarté par le profil : %+v", fichier)
	}

	// Sans profil, tout ce qui est d'un langage connu est retenu.
	tout := inspecteur(t, inspect.Settings{}).Select(sources(fichiers))
	if len(tout.Kept) != len(fichiers) {
		t.Fatalf("sans profil : %v", chemins(tout))
	}
}

func TestLesLangagesDeLAnalyseRemplacentCeuxDuProfil(t *testing.T) {
	selection := inspecteur(t, inspect.Settings{
		Profile: "next", Languages: []string{"css"},
	}).Select(sources(map[string]string{
		"depot/app/page.tsx":  "export default function Page() { return null; }",
		"depot/app/style.css": ".x { color: red; }",
	}))
	if noms := chemins(selection); len(noms) != 1 || noms[0] != "app/style.css" {
		t.Fatalf("langages imposés : %v", noms)
	}
}

func TestUneExclusionPonctuelleSeDistingueDuProfil(t *testing.T) {
	selection := inspecteur(t, inspect.Settings{
		Exclude: []string{"*.sql"},
	}).Select(sources(map[string]string{
		"depot/schema.sql": "CREATE TABLE x (id INT);",
		"depot/Main.java":  "class Main { }",
	}))
	if fichier, _ := ecarte(selection, "schema.sql"); fichier.Reason != inspect.Requested ||
		fichier.Rule != "*.sql" {
		t.Fatalf("exclusion ponctuelle : %+v", fichier)
	}
}

func TestUnReglageImpossibleEstRefuseAvantToutTravail(t *testing.T) {
	if _, err := inspect.New(inspect.Settings{Profile: "nextjs"}, nil); err == nil {
		t.Fatal("un profil inconnu doit être refusé")
	}
	_, err := inspect.New(inspect.Settings{Languages: []string{"java", "cobol"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "cobol") {
		t.Fatalf("un langage inconnu doit être refusé en le nommant : %v", err)
	}
}

func TestUnProfilDeLOrganisationRemplaceCeluiDOffice(t *testing.T) {
	declare := []inspect.Profile{
		{ID: "next", Label: "Next.js maison", Include: []string{"src/**"}},
		{ID: "maison", Label: "Profil maison", Include: []string{"noyau/**"}},
	}
	catalogue := inspect.Catalog(declare)
	profil, err := inspect.FindProfile(catalogue, "next")
	if err != nil || profil.Label != "Next.js maison" {
		t.Fatalf("profil remplacé : %+v (%v)", profil, err)
	}
	if _, err := inspect.FindProfile(catalogue, "maison"); err != nil {
		t.Fatalf("profil ajouté : %v", err)
	}
	if len(catalogue) != len(inspect.BuiltIn)+1 {
		t.Fatalf("catalogue de %d profils", len(catalogue))
	}
}

func TestLesMotifsEcartesSontComptesParMotif(t *testing.T) {
	selection := inspecteur(t, inspect.Settings{}).Select(sources(map[string]string{
		"depot/node_modules/a/i.js": "1",
		"depot/node_modules/b/i.js": "1",
		"depot/logo.ico":            "x",
		"depot/Main.java":           "class Main { }",
	}))
	if selection.Reasons[inspect.Excluded] != 2 || selection.Reasons[inspect.IsBinary] != 1 {
		t.Fatalf("décompte par motif : %+v", selection.Reasons)
	}
	if motifs := selection.Summary(); len(motifs) == 0 || motifs[0] != inspect.Excluded {
		t.Fatalf("motif le plus fréquent : %v", motifs)
	}
}

// Un projet Android mêle trois façons d'écrire la même application — Java,
// Kotlin, Compose — et les range toutes au même endroit. Le profil doit les
// prendre ensemble, et laisser dehors ce que l'outillage engendre.
func TestLeProfilAndroidPrendJavaKotlinEtCompose(t *testing.T) {
	selection := inspecteur(t, inspect.Settings{Profile: "android"}).Select(sources(map[string]string{
		// Du Java, du Kotlin et du Compose : tous sous « java/ », parce que
		// c'est là qu'Android Studio les met.
		"depot/app/src/main/java/ca/acme/MainActivity.kt": "class MainActivity : AppCompatActivity() { }",
		"depot/app/src/main/java/ca/acme/Legacy.java":     "public class Legacy { }",
		"depot/app/src/main/java/ca/acme/ui/Ecran.kt":     "@Composable fun Ecran() { Text(\"bonjour\") }",
		"depot/app/src/main/kotlin/ca/acme/Modele.kt":     "data class Modele(val nom: String)",
		"depot/app/src/main/res/layout/activity_main.xml": "<LinearLayout></LinearLayout>",
		"depot/app/src/main/AndroidManifest.xml":          "<manifest></manifest>",
		"depot/app/src/test/java/ca/acme/ModeleTest.kt":   "class ModeleTest { }",
		// Un second module : le module ne s'appelle pas toujours « app ».
		"depot/coeur/src/main/kotlin/ca/acme/Calcul.kt": "object Calcul { }",

		// Ce que l'outillage engendre ou que personne n'écrit à la main.
		"depot/app/build.gradle.kts":                      "plugins { id(\"com.android.application\") }",
		"depot/app/src/main/res/values/colors.xml":        "<resources></resources>",
		"depot/app/src/main/res/drawable/ic_launcher.xml": "<vector></vector>",
		"depot/app/build/generated/R.java":                "public final class R { }",
		"depot/gradle/wrapper/gradle-wrapper.properties":  "distributionUrl=x",
	}))

	attendus := []string{
		"app/src/main/AndroidManifest.xml",
		"app/src/main/java/ca/acme/Legacy.java",
		"app/src/main/java/ca/acme/MainActivity.kt",
		"app/src/main/java/ca/acme/ui/Ecran.kt",
		"app/src/main/kotlin/ca/acme/Modele.kt",
		"app/src/main/res/layout/activity_main.xml",
		"app/src/test/java/ca/acme/ModeleTest.kt",
		"coeur/src/main/kotlin/ca/acme/Calcul.kt",
	}
	if obtenu := strings.Join(chemins(selection), ","); obtenu != strings.Join(attendus, ",") {
		t.Fatalf("profil Android :\n%v\nattendu :\n%v", chemins(selection), attendus)
	}
	for _, dehors := range []string{
		"app/build.gradle.kts", "app/src/main/res/values/colors.xml",
		"app/src/main/res/drawable/ic_launcher.xml",
	} {
		if _, ecarte := ecarte(selection, dehors); !ecarte {
			t.Fatalf("« %s » aurait dû être écarté", dehors)
		}
	}
}
