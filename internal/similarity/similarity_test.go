package similarity_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/similarity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/tokens"
)

// copie fabrique une copie d'index à partir de fichiers sources.
func copie(t *testing.T, id, origine string, fichiers map[string]string) similarity.Work {
	t.Helper()
	work := similarity.Work{ID: id, Origin: origine}
	for chemin, source := range fichiers {
		langage, reconnu := tokens.Detect(chemin)
		if !reconnu {
			t.Fatalf("« %s » : extension non reconnue", chemin)
		}
		analyse := tokens.Lex(langage.ID, []byte(source))
		file, court := similarity.Analyze(chemin, analyse, 0, 0)
		if court != nil {
			t.Fatalf("« %s » : trop court pour être comparé (%d jetons, k=%d)",
				chemin, court.Tokens, court.Kgram)
		}
		work.Files = append(work.Files, file)
		work.Extras.Comments = append(work.Extras.Comments, commentaires(analyse)...)
	}
	return work
}

func commentaires(result tokens.Result) []string {
	textes := make([]string, 0, len(result.Comments))
	for _, note := range result.Comments {
		textes = append(textes, note.Text)
	}
	return textes
}

// synthetique fabrique une copie à partir d'un flux de jetons écrit à la main.
//
// Certaines épreuves portent sur une règle de décompte — ce qui est écarté
// comme banal, où tombe le seuil — et non sur l'analyse d'un langage. Les
// mener sur du vrai Java reviendrait à mesurer deux choses à la fois, et à ne
// plus savoir laquelle a cassé quand l'épreuve échoue.
func synthetique(t *testing.T, id string, mots []string) similarity.Work {
	t.Helper()
	flux := make([]tokens.Token, 0, len(mots))
	for rang, mot := range mots {
		flux = append(flux, tokens.Token{Text: mot, Line: rang + 1, Column: 1})
	}
	file, court := similarity.Analyze("flux.java",
		tokens.Result{Language: "java", Tokens: flux}, 0, 0)
	if court != nil {
		t.Fatalf("flux de %d jetons : trop court pour k=%d", court.Tokens, court.Kgram)
	}
	return similarity.Work{ID: id, Files: []similarity.File{file}}
}

// bloc fabrique une suite de jetons propre à un nom donné.
func bloc(nom string, taille int) []string {
	mots := make([]string, 0, taille)
	for rang := 0; rang < taille; rang++ {
		mots = append(mots, fmt.Sprintf("%s-%d", nom, rang))
	}
	return mots
}

func trouver(rapport similarity.Report, gauche, droite string) (similarity.Match, bool) {
	for _, match := range rapport.Matches {
		if (match.Left == gauche && match.Right == droite) ||
			(match.Left == droite && match.Right == gauche) {
			return match, true
		}
	}
	return similarity.Match{}, false
}

// --------------------------------------------------------------- les sources
//
// Les sources d'épreuve sont longues à dessein. Sur un fichier de cinquante
// jetons, k vaut vingt-trois : quelques empreintes en tout, et le moindre écart
// en emporte la moitié. Les mesures n'y veulent rien dire, et un seuil calé
// dessus ne dirait rien non plus. Les vrais travaux font des centaines de
// lignes ; les épreuves doivent leur ressembler.

const gabarit = `
package tp1;

import java.util.ArrayList;
import java.util.List;

public class Inventaire {
    private final List<String> articles = new ArrayList<>();

    public void ajouter(String article) {
        if (article == null || article.isEmpty()) {
            throw new IllegalArgumentException("article vide");
        }
        articles.add(article);
    }

    public boolean contient(String article) {
        return articles.contains(article);
    }

    public void retirer(String article) {
        if (!articles.contains(article)) {
            throw new IllegalStateException("article absent");
        }
        articles.remove(article);
    }

    public int taille() {
        return articles.size();
    }
}
`

// methodes porte les blocs dont les copies d'épreuve sont faites. Les fabriquer
// à partir d'une même liste plutôt que de recopier trois longues sources à la
// main garantit que « renommée » et « réordonnée » sont bien le même programme
// renommé et réordonné, et non deux textes qui se ressemblent.
var methodes = []string{
	`
    public int[] statistiques(int[] valeurs) {
        if (valeurs.length == 0) {
            throw new IllegalArgumentException("tableau vide");
        }
        int plusGrand = valeurs[0];
        int plusPetit = valeurs[0];
        int total = 0;
        for (int index = 0; index < valeurs.length; index++) {
            int valeur = valeurs[index];
            if (valeur > plusGrand) {
                plusGrand = valeur;
            }
            if (valeur < plusPetit) {
                plusPetit = valeur;
            }
            total = total + valeur;
        }
        int moyenne = total / valeurs.length;
        return new int[] { plusPetit, plusGrand, total, moyenne };
    }
`,
	`
    public void trier(int[] valeurs) {
        for (int passage = 0; passage < valeurs.length - 1; passage++) {
            boolean echange = false;
            for (int position = 0; position < valeurs.length - passage - 1; position++) {
                if (valeurs[position] > valeurs[position + 1]) {
                    int temporaire = valeurs[position];
                    valeurs[position] = valeurs[position + 1];
                    valeurs[position + 1] = temporaire;
                    echange = true;
                }
            }
            if (!echange) {
                return;
            }
        }
    }
`,
	`
    public int rechercher(int[] valeurs, int cible) {
        int debut = 0;
        int fin = valeurs.length - 1;
        while (debut <= fin) {
            int milieu = debut + (fin - debut) / 2;
            if (valeurs[milieu] == cible) {
                return milieu;
            }
            if (valeurs[milieu] < cible) {
                debut = milieu + 1;
            } else {
                fin = milieu - 1;
            }
        }
        return -1;
    }
`,
	`
    public int[] fusionner(int[] gauche, int[] droite) {
        int[] resultat = new int[gauche.length + droite.length];
        int rangGauche = 0;
        int rangDroite = 0;
        int sortie = 0;
        while (rangGauche < gauche.length && rangDroite < droite.length) {
            if (gauche[rangGauche] <= droite[rangDroite]) {
                resultat[sortie] = gauche[rangGauche];
                rangGauche = rangGauche + 1;
            } else {
                resultat[sortie] = droite[rangDroite];
                rangDroite = rangDroite + 1;
            }
            sortie = sortie + 1;
        }
        while (rangGauche < gauche.length) {
            resultat[sortie] = gauche[rangGauche];
            rangGauche = rangGauche + 1;
            sortie = sortie + 1;
        }
        while (rangDroite < droite.length) {
            resultat[sortie] = droite[rangDroite];
            rangDroite = rangDroite + 1;
            sortie = sortie + 1;
        }
        return resultat;
    }
`,
}

func classe(nom string, corps []string) string {
	return "public class " + nom + " {\n" + strings.Join(corps, "\n") + "\n}\n"
}

var original = classe("Solution", methodes)

// reordonne est le même programme, méthodes interverties.
var reordonne = classe("Solution",
	[]string{methodes[2], methodes[0], methodes[3], methodes[1]})

// renomme est le même programme, tous ses noms changés.
var renomme = strings.NewReplacer(
	"Solution", "Devoir", "statistiques", "bilan", "trier", "ordonner",
	"rechercher", "localiser", "fusionner", "assembler", "valeurs", "tableau",
	"plusGrand", "record", "plusPetit", "minimum", "total", "cumul",
	"index", "i", "valeur", "element", "moyenne", "centre", "passage", "tour",
	"position", "curseur", "temporaire", "garde", "echange", "permute",
	"debut", "bas", "fin", "haut", "milieu", "pivot", "cible", "recherche",
	"gauche", "premier", "droite", "second", "resultat", "sortie2",
	"rangGauche", "tetePremier", "rangDroite", "teteSecond", "sortie", "ecriture",
).Replace(original)

// honnete résout les mêmes problèmes autrement : c'est le faux positif qu'il ne
// faut pas remonter.
const honnete = `
import java.util.Arrays;
import java.util.stream.IntStream;

public class Travail {
    public int[] statistiques(int[] entrees) {
        IntSummaryStatistics bilan = Arrays.stream(entrees).summaryStatistics();
        return new int[] {
            bilan.getMin(), bilan.getMax(), (int) bilan.getSum(), (int) bilan.getAverage()
        };
    }

    public void trier(int[] entrees) {
        Arrays.sort(entrees);
    }

    public int rechercher(int[] entrees, int cible) {
        return Arrays.binarySearch(entrees, cible);
    }

    public int[] fusionner(int[] gauche, int[] droite) {
        return IntStream.concat(Arrays.stream(gauche), Arrays.stream(droite))
            .sorted()
            .toArray();
    }
}
`

// ------------------------------------------------------------------ épreuves

func TestUneCopieExacteSeVoitEntierement(t *testing.T) {
	corpus := similarity.Corpus{Works: []similarity.Work{
		copie(t, "alice", "01", map[string]string{"Solution.java": original}),
		copie(t, "bruno", "01", map[string]string{"Solution.java": original}),
	}}
	rapport := similarity.Compare(corpus, similarity.Options{})
	match, trouve := trouver(rapport, "alice", "bruno")
	if !trouve {
		t.Fatal("la paire n'a pas été relevée")
	}
	if match.Similarity < 0.99 {
		t.Fatalf("similarité d'une copie exacte : %.2f", match.Similarity)
	}
}

// La propriété qui justifie la normalisation : renommer ne sauve pas.
func TestRenommerNeSauvePas(t *testing.T) {
	corpus := similarity.Corpus{Works: []similarity.Work{
		copie(t, "alice", "01", map[string]string{"Solution.java": original}),
		copie(t, "bruno", "01", map[string]string{"Devoir.java": renomme}),
	}}
	rapport := similarity.Compare(corpus, similarity.Options{})
	match, trouve := trouver(rapport, "alice", "bruno")
	// Le flux comparé est identique au jeton près : la similarité doit valoir
	// un. Rien de moins ne serait acceptable — ce serait dire qu'un nom de
	// variable a pesé sur la mesure.
	if !trouve || match.Similarity < 0.999 {
		t.Fatalf("similarité après renommage : %.3f (trouvée : %v)", match.Similarity, trouve)
	}
}

// La propriété qui justifie le winnowing : rien ne dépend de la position, donc
// intervertir deux méthodes ne change presque rien.
func TestIntervertirDeuxMethodesNeSauvePasNonPlus(t *testing.T) {
	corpus := similarity.Corpus{Works: []similarity.Work{
		copie(t, "alice", "01", map[string]string{"Solution.java": original}),
		copie(t, "bruno", "01", map[string]string{"Solution.java": reordonne}),
	}}
	rapport := similarity.Compare(corpus, similarity.Options{})
	match, trouve := trouver(rapport, "alice", "bruno")
	if !trouve || match.Similarity < 0.8 {
		t.Fatalf("similarité après réordonnancement : %.2f (trouvée : %v)",
			match.Similarity, trouve)
	}
}

// Deux solutions honnêtes d'un même exercice ne doivent pas remonter. C'est le
// faux positif qu'on cherche à éviter, et il compte autant que la détection.
func TestDeuxSolutionsHonnetesNeRemontentPas(t *testing.T) {
	corpus := similarity.Corpus{Works: []similarity.Work{
		copie(t, "alice", "01", map[string]string{"Solution.java": original}),
		copie(t, "claire", "01", map[string]string{"Travail.java": honnete}),
	}}
	rapport := similarity.Compare(corpus, similarity.Options{})
	if match, trouve := trouver(rapport, "alice", "claire"); trouve && match.Similarity > 0.3 {
		t.Fatalf("deux solutions différentes se ressemblent à %.2f", match.Similarity)
	}
}

// Le gabarit distribué est commun à toutes les copies sans que personne n'ait
// rien copié. Sans son retrait, tout le monde remonte et le rapport est
// inutilisable.
func TestLeGabaritDistribueEstRetireDeLaComparaison(t *testing.T) {
	avec := func(nom, propre string) similarity.Work {
		return copie(t, nom, "01", map[string]string{
			"Inventaire.java": gabarit, "Solution.java": propre,
		})
	}
	corpus := similarity.Corpus{Works: []similarity.Work{
		avec("alice", original), avec("claire", honnete),
	}}

	sans := similarity.Compare(corpus, similarity.Options{})
	brut, _ := trouver(sans, "alice", "claire")

	modele := copie(t, "modele", "", map[string]string{"Inventaire.java": gabarit})
	filtre := similarity.Compare(corpus, similarity.Options{
		Baseline: similarity.Baseline(modele),
	})
	if filtre.IgnoredBaseline == 0 {
		t.Fatal("aucune empreinte de gabarit écartée")
	}
	net, trouve := trouver(filtre, "alice", "claire")
	if trouve && net.Similarity >= brut.Similarity {
		t.Fatalf("le gabarit n'a rien changé : %.2f puis %.2f",
			brut.Similarity, net.Similarity)
	}
}

// Le garde-fou statistique fait le même travail sans qu'on ait rien à désigner.
//
// L'épreuve est menée sur des flux fabriqués plutôt que sur du Java : ce qui se
// vérifie ici est une règle de décompte, et la fabriquer à la main permet de
// dire exactement ce que chaque copie partage avec les autres.
func TestUneEmpreinteTropRepandueEstEcartee(t *testing.T) {
	commun := bloc("commun", 120)
	works := make([]similarity.Work, 0, 12)
	for numero := 0; numero < 12; numero++ {
		propre := bloc(fmt.Sprintf("propre%d", numero), 120)
		works = append(works, synthetique(t, fmt.Sprintf("etudiant%d", numero),
			append(append([]string{}, commun...), propre...)))
	}
	rapport := similarity.Compare(similarity.Corpus{Works: works}, similarity.Options{})
	if rapport.IgnoredCommon == 0 {
		t.Fatal("aucune empreinte écartée pour cause de banalité")
	}
	for _, match := range rapport.Matches {
		t.Fatalf("douze copies qui ne partagent que du commun se ressemblent : %+v", match)
	}
}

// Sur un petit corpus, « présent chez trop de monde » ne veut rien dire : trois
// copies identiques sur quatre sont exactement ce qu'on cherche, pas du bruit.
func TestLaRegleDeBruitNeSAppliquePasAUnPetitCorpus(t *testing.T) {
	corpus := similarity.Corpus{Works: []similarity.Work{
		copie(t, "alice", "01", map[string]string{"Solution.java": original}),
		copie(t, "bruno", "01", map[string]string{"Solution.java": original}),
	}}
	rapport := similarity.Compare(corpus, similarity.Options{})
	if rapport.IgnoredCommon != 0 {
		t.Fatalf("%d empreintes écartées à tort", rapport.IgnoredCommon)
	}
}

// Une courte copie entièrement recopiée dans une longue a une similarité
// faible et une couverture totale. Ne lire que la similarité la laisserait
// passer.
func TestUneCourteCopieEntierementRecopieeSeVoitParLaCouverture(t *testing.T) {
	longue := original + "\n" + strings.ReplaceAll(honnete, "Travail", "Annexe") +
		"\n" + strings.ReplaceAll(gabarit, "Inventaire", "Autre")
	corpus := similarity.Corpus{Works: []similarity.Work{
		copie(t, "courte", "01", map[string]string{"Solution.java": original}),
		copie(t, "longue", "01", map[string]string{"Tout.java": longue}),
	}}
	rapport := similarity.Compare(corpus, similarity.Options{})
	match, trouve := trouver(rapport, "courte", "longue")
	if !trouve {
		t.Fatal("la paire n'a pas été relevée")
	}
	courte := match.LeftCoverage
	if match.Left != "courte" {
		courte = match.RightCoverage
	}
	if courte < 0.9 {
		t.Fatalf("couverture de la courte copie : %.2f", courte)
	}
	if match.Similarity > courte {
		t.Fatalf("similarité %.2f et couverture %.2f : la mesure ne dit rien",
			match.Similarity, courte)
	}
}

// Un score sans fragment n'est pas vérifiable. Les positions rendues doivent
// désigner le vrai passage commun.
func TestLesFragmentsDesignentLePassageCommun(t *testing.T) {
	corpus := similarity.Corpus{Works: []similarity.Work{
		copie(t, "alice", "01", map[string]string{"Solution.java": original}),
		copie(t, "bruno", "01", map[string]string{"Solution.java": original}),
	}}
	rapport := similarity.Compare(corpus, similarity.Options{})
	match, _ := trouver(rapport, "alice", "bruno")
	if len(match.Files) == 0 || len(match.Files[0].Fragments) == 0 {
		t.Fatalf("aucun fragment : %+v", match.Files)
	}
	fichier := match.Files[0]
	if fichier.LeftPath != "Solution.java" || fichier.RightPath != "Solution.java" {
		t.Fatalf("fichiers appariés : %s / %s", fichier.LeftPath, fichier.RightPath)
	}
	if fichier.LongestFragment < fichier.LeftTokens/2 {
		t.Fatalf("plus long fragment de %d jetons sur %d",
			fichier.LongestFragment, fichier.LeftTokens)
	}
	for _, fragment := range fichier.Fragments {
		if fragment.Length() <= 0 || fragment.LeftEnd > fichier.LeftTokens {
			t.Fatalf("fragment hors du fichier : %+v", fragment)
		}
	}

	// Traduit en lignes, le fragment doit tomber dans le fichier.
	flux := tokens.Lex("java", []byte(original))
	debut, fin := similarity.Lines(flux.Tokens, fichier.Fragments[0].LeftStart,
		fichier.Fragments[0].LeftEnd)
	if debut < 1 || fin < debut || fin > strings.Count(original, "\n")+1 {
		t.Fatalf("lignes du fragment : %d à %d", debut, fin)
	}
}

// La garantie du winnowing : deux fichiers qui partagent un passage assez long
// retiennent forcément une empreinte commune dedans, où qu'il se trouve.
func TestUnPassageAssezLongDonneToujoursUneEmpreinteCommune(t *testing.T) {
	commun := make([]tokens.Token, 0, 64)
	for index := 0; index < tokens.CodeKgram+tokens.CodeWindow; index++ {
		commun = append(commun, tokens.Token{Text: fmt.Sprintf("m%d", index)})
	}
	prefixe := func(taille int) []tokens.Token {
		flux := make([]tokens.Token, 0, taille+len(commun))
		for index := 0; index < taille; index++ {
			flux = append(flux, tokens.Token{Text: fmt.Sprintf("p%d-%d", taille, index)})
		}
		return append(flux, commun...)
	}
	for _, decalage := range []int{0, 1, 7, 40, 133} {
		gauche := similarity.Fingerprints(prefixe(3), tokens.CodeKgram, tokens.CodeWindow)
		droite := similarity.Fingerprints(prefixe(decalage), tokens.CodeKgram, tokens.CodeWindow)
		if !partagent(gauche, droite) {
			t.Fatalf("décalage de %d jetons : aucune empreinte commune", decalage)
		}
	}
}

func partagent(gauche, droite []similarity.Print) bool {
	connus := map[uint64]bool{}
	for _, print := range gauche {
		connus[print.Hash] = true
	}
	for _, print := range droite {
		if connus[print.Hash] {
			return true
		}
	}
	return false
}

func TestUnFichierTropCourtEstSignaleEtNonCompare(t *testing.T) {
	analyse := tokens.Lex("java", []byte("int x = 1;"))
	file, court := similarity.Analyze("Court.java", analyse, 0, 0)
	if court == nil {
		t.Fatal("un fichier de cinq jetons devrait être signalé trop court")
	}
	if court.Kgram != tokens.CodeKgram || len(file.Prints) != 0 {
		t.Fatalf("fichier trop court mal rendu : %+v / %+v", court, file)
	}
}

// Le seuil doit tomber entre les deux populations, pas au milieu de l'une
// d'elles.
//
// Le corpus d'épreuve a la forme qu'ont les vrais : une masse de copies
// honnêtes qui se ressemblent un peu — les mêmes consignes, les mêmes
// tournures — et une poignée de copies identiques à l'écart. Sans la première,
// il n'y aurait rien à séparer, et proposer un seuil n'aurait aucun sens.
func TestLeSeuilSepareLesDeuxPopulations(t *testing.T) {
	const honnetes = 16
	works := make([]similarity.Work, 0, honnetes+4)

	// Chaque copie honnête partage un bloc avec sa voisine, et rien de plus :
	// de quoi peupler le bas de la distribution sans qu'aucun bloc ne devienne
	// assez répandu pour être écarté comme banal.
	for numero := 0; numero < honnetes; numero++ {
		flux := append(bloc(fmt.Sprintf("voisin%d", numero), 80),
			bloc(fmt.Sprintf("voisin%d", numero+1), 80)...)
		flux = append(flux, bloc(fmt.Sprintf("propre%d", numero), 260)...)
		works = append(works, synthetique(t, fmt.Sprintf("honnete%d", numero), flux))
	}
	recopie := bloc("recopie", 420)
	for numero := 0; numero < 4; numero++ {
		works = append(works, synthetique(t, fmt.Sprintf("copieur%d", numero), recopie))
	}

	rapport := similarity.Compare(similarity.Corpus{Works: works}, similarity.Options{})
	if rapport.Threshold <= 0 {
		t.Fatalf("aucun seuil proposé sur %d paires", rapport.Compared)
	}
	for _, match := range rapport.Matches {
		copieurs := strings.HasPrefix(match.Left, "copieur") &&
			strings.HasPrefix(match.Right, "copieur")
		if copieurs && match.Similarity < rapport.Threshold {
			t.Fatalf("deux copies identiques sous le seuil %.2f : %.2f",
				rapport.Threshold, match.Similarity)
		}
		if !copieurs && match.Similarity >= rapport.Threshold {
			t.Fatalf("deux copies honnêtes au-dessus du seuil %.2f : %.2f (%s, %s)",
				rapport.Threshold, match.Similarity, match.Left, match.Right)
		}
	}

	total := 0
	for _, tranche := range rapport.Histogram {
		total += tranche.Count
	}
	if total != rapport.Compared {
		t.Fatalf("histogramme : %d paires pour %d mesurées", total, rapport.Compared)
	}
}

func TestUneMemeSignatureDansDeuxCopiesEstUnSignal(t *testing.T) {
	alice := copie(t, "alice", "01", map[string]string{"Solution.java": original})
	bruno := copie(t, "bruno", "01", map[string]string{"Travail.java": honnete})
	alice.Extras.Signature = "7f3a2c91"
	bruno.Extras.Signature = "7f3a2c91"

	rapport := similarity.Compare(similarity.Corpus{
		Works: []similarity.Work{alice, bruno},
	}, similarity.Options{})
	if len(rapport.Signals) != 1 || rapport.Signals[0].Kind != similarity.SharedSignature {
		t.Fatalf("signaux relevés : %+v", rapport.Signals)
	}
	if len(rapport.Signals[0].Works) != 2 {
		t.Fatalf("copies concernées : %+v", rapport.Signals[0].Works)
	}
}

func TestUnLongCommentaireIdentiqueEstUnSignal(t *testing.T) {
	note := "// On additionne toutes les valeurs du tableau une par une\n"
	corpus := similarity.Corpus{Works: []similarity.Work{
		copie(t, "alice", "01", map[string]string{"Solution.java": note + original}),
		copie(t, "bruno", "01", map[string]string{"Travail.java": note + honnete}),
		copie(t, "claire", "01", map[string]string{"Autre.java": gabarit}),
	}}
	rapport := similarity.Compare(corpus, similarity.Options{})
	for _, signal := range rapport.Signals {
		if signal.Kind == similarity.SharedComment && len(signal.Works) == 2 {
			return
		}
	}
	t.Fatalf("le commentaire partagé n'a pas été relevé : %+v", rapport.Signals)
}
