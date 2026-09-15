package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plagiarism"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

const gabaritTP = `
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

    public int taille() {
        return articles.size();
    }
}
`

const solutionTP = `
public class Solution {
    public int somme(int[] valeurs) {
        int total = 0;
        for (int index = 0; index < valeurs.length; index++) {
            total = total + valeurs[index];
        }
        return total;
    }

    public int maximum(int[] valeurs) {
        int plusGrand = valeurs[0];
        for (int index = 1; index < valeurs.length; index++) {
            if (valeurs[index] > plusGrand) {
                plusGrand = valeurs[index];
            }
        }
        return plusGrand;
    }
}
`

const autreTP = `
import java.util.Arrays;

public class Travail {
    public int somme(int[] entrees) {
        return Arrays.stream(entrees).sum();
    }

    public int maximum(int[] entrees) {
        return Arrays.stream(entrees).max().orElseThrow();
    }
}
`

// groupeRemis monte un travail remis par trois personnes : deux copies
// identiques, une différente, toutes parties du même gabarit.
func groupeRemis(t *testing.T) *harnais {
	t.Helper()
	state := fakegh.NewState()
	state.AddRepo("acme", "modele-tp1", true)
	state.SeedCommit("acme/modele-tp1",
		map[string]string{"src/Inventaire.java": gabaritTP}, "main")

	copies := map[string]string{
		"a26.5n6.01.tp1.emilie-cote":     solutionTP,
		"a26.5n6.01.tp1.jean-luc-picard": solutionTP,
		"a26.5n6.01.tp1.aminata-diallo":  autreTP,
	}
	for nom, source := range copies {
		state.AddRepo("acme", nom, true)
		state.SeedCommit("acme/"+nom, map[string]string{
			"src/Inventaire.java": gabaritTP,
			"src/Solution.java":   source,
			"assets/logo.ico":     "\x00\x00 image",
		}, "main")
	}

	h := nouveau(t, state)
	defauts := classroom.DefaultsFrom(config.Default())
	defauts.Template = "acme/modele-tp1"
	store := classroom.Open(classroom.PathNextTo(h.Reglages))
	cours := classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "01",
		Defaults: defauts,
		Students: []roster.Person{
			{FullName: "Émilie Côté", Username: "emilie-cote"},
			{FullName: "Jean-Luc Picard", Username: "jlpicard"},
			{FullName: "Aminata Diallo", Username: "aminata-d"},
		},
	}
	if _, err := store.Save(cours); err != nil {
		t.Fatalf("déclaration : %v", err)
	}
	return h
}

func TestDrapeauPlagiarismCompareLesCopiesEtEcritSonRapport(t *testing.T) {
	h := groupeRemis(t)
	h.Options.Plagiarism = true
	h.Options.Manage = "a26.5n6.01.tp1"
	h.Options.ManageRequested = true
	h.Options.Yes = true

	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}

	// L'avertissement est dit, et il n'est pas raccourci : c'est le même texte
	// que le navigateur affiche, décidé dans le domaine.
	h.contient("n'est pas un détecteur de plagiat")
	// Les deux copies identiques sont appariées, et nommées par leur nom
	// complet plutôt que par le slug du dépôt.
	h.contient("Émilie Côté", "Jean-Luc Picard", "Paires, du plus suspect")
	// Le gabarit distribué est retiré de lui-même : l'outil sait lequel il a
	// déposé.
	h.contient("venant du gabarit distribué")
	// La vue de comparaison n'existe qu'au navigateur : le trou est dit plutôt
	// que laissé en silence.
	h.contient("n'a pas d'équivalent au terminal")

	// Le rapport est un fichier, et l'assistant dit où il est.
	rapports := plagiarism.List(h.Rapports)
	if len(rapports) != 1 {
		t.Fatalf("rapports écrits : %v", rapports)
	}
	rapport, err := plagiarism.Load(rapports[0])
	if err != nil {
		t.Fatalf("relecture : %v", err)
	}
	if rapport.Analyzed() != 3 {
		t.Fatalf("copies analysées : %d", rapport.Analyzed())
	}
	match, trouve := rapport.Match(
		"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.jean-luc-picard")
	if !trouve || match.Similarity < 0.99 {
		t.Fatalf("deux copies identiques : %.2f (trouvée : %v)", match.Similarity, trouve)
	}
	// Aminata a résolu l'exercice autrement : elle ne doit pas remonter.
	if match, trouve := rapport.Match(
		"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.aminata-diallo"); trouve &&
		match.Similarity > 0.3 {
		t.Fatalf("une solution différente remonte à %.2f", match.Similarity)
	}

	// Le CSV est écrit à côté, pour un tableur ou un dossier.
	csv := strings.TrimSuffix(rapports[0], ".json") + ".csv"
	if _, err := os.Stat(csv); err != nil {
		t.Fatalf("CSV des paires : %v", err)
	}
	if !strings.Contains(rapports[0], filepath.Join(h.Rapports, plagiarism.Dir)) {
		t.Fatalf("emplacement du rapport : %q", rapports[0])
	}
}

func TestDrapeauPlagiarismRefuseUnProfilInconnuSansRienTelecharger(t *testing.T) {
	h := groupeRemis(t)
	h.Options.Plagiarism = true
	h.Options.Manage = "a26.5n6.01.tp1"
	h.Options.ManageRequested = true
	h.Options.Profile = "nextjs"
	h.Options.Yes = true

	if code := h.muet(); code != app.ExitValidation {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("nextjs")
	if len(plagiarism.List(h.Rapports)) != 0 {
		t.Fatal("un réglage refusé ne doit rien écrire")
	}
}

// Un langage retiré doit vraiment l'être : c'est le remède que l'estimation
// propose quand l'analyse est trop lourde, et il ne servirait à rien s'il ne
// changeait pas le corpus.
func TestDrapeauLanguagesRestreintCeQuiEstCompare(t *testing.T) {
	h := groupeRemis(t)
	h.Options.Plagiarism = true
	h.Options.Manage = "a26.5n6.01.tp1"
	h.Options.ManageRequested = true
	h.Options.Languages = "sql"
	h.Options.Yes = true

	// Aucun fichier SQL dans ces dépôts : toutes les copies sont écartées, et
	// l'analyse doit le dire plutôt que de rendre un rapport vide sans motif.
	if code := h.muet(); code != app.ExitValidation {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("aucun fichier retenu")
}

// Le gabarit de passe automatisée se dépose sans jeton ni réseau : c'est un
// fichier à écrire, et rien d'autre.
func TestLeGabaritDePasseSeDeposeEtSExplique(t *testing.T) {
	h := nouveau(t, nil)
	chemin := filepath.Join(t.TempDir(), "plagiat.yml")
	h.Options.EmitWorkflowSet = true
	h.Options.EmitWorkflow = chemin

	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	contenu, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatalf("gabarit : %v", err)
	}
	// Ce qu'il doit dire avant d'être utilisé : le jeton qu'il demande, et la
	// protection sans laquelle il annule le cloisonnement.
	for _, attendu := range []string{
		"COHORTE_TOKEN", "protégé en écriture", "--publish-index",
		"ni code ni nom",
	} {
		if !strings.Contains(string(contenu), attendu) {
			t.Fatalf("le gabarit doit dire « %s » :\n%s", attendu, contenu)
		}
	}
	// Un rapport porte des noms : la variante qui le rend est commentée, et
	// prévient.
	if !strings.Contains(string(contenu), "# - name: Comparer") {
		t.Fatalf("la variante doit rester commentée :\n%s", contenu)
	}

	// Un fichier déjà présent n'est pas écrasé : il a peut-être été retouché.
	if code := h.muet(); code != app.ExitValidation {
		t.Fatalf("un gabarit existant doit être refusé (code %d)", code)
	}
	h.contient("existe déjà")
}
