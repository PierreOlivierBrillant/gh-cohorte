package corpus_test

import (
	"strings"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/signature"
)

const solution = `
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

func client(t *testing.T, state *fakegh.State) *ghapi.Client {
	t.Helper()
	serveur := fakegh.New(state)
	t.Cleanup(serveur.Close)
	c, err := ghapi.New(ghapi.Options{
		Host: "127.0.0.1", Token: "jeton-de-test", BaseURL: serveur.URL(),
		Sleep: func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("client : %v", err)
	}
	return c
}

func depot(state *fakegh.State, nom string, fichiers map[string]string) {
	state.AddRepo("acme", nom, true)
	if len(fichiers) > 0 {
		state.SeedCommit("acme/"+nom, fichiers, "main")
	}
}

func cible(nom string) corpus.Target {
	return corpus.Target{ID: nom, Label: nom, Origin: "01", Owner: "acme", Repo: nom}
}

func options(t *testing.T, settings inspect.Settings) corpus.Options {
	t.Helper()
	inspector, err := inspect.New(settings, nil)
	if err != nil {
		t.Fatalf("réglage : %v", err)
	}
	return corpus.Options{Inspector: inspector, Jobs: 2}
}

func probleme(result corpus.Result, id string) (corpus.Problem, bool) {
	for _, souci := range result.Problems {
		if souci.ID == id {
			return souci, true
		}
	}
	return corpus.Problem{}, false
}

// ------------------------------------------------------------------ untar

func TestUntarRendLesFichiersEtLeCommit(t *testing.T) {
	archive, err := fakegh.Tarball("acme-tp1-alice-3f9c2ab", map[string]string{
		"src/Main.java": "class Main { }",
		"README.md":     "# TP1",
	})
	if err != nil {
		t.Fatalf("archive : %v", err)
	}
	sources, prefixe, err := corpus.Untar(archive)
	if err != nil {
		t.Fatalf("dépaquetage : %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("fichiers dépaquetés : %d", len(sources))
	}
	if prefixe != "acme-tp1-alice-3f9c2ab" {
		t.Fatalf("préfixe : %q", prefixe)
	}
	// Le commit archivé se lit dans le préfixe : c'est ce qui permet de
	// retélécharger exactement la même chose plus tard.
	if corpus.Commit(prefixe) != "3f9c2ab" {
		t.Fatalf("commit : %q", corpus.Commit(prefixe))
	}
	for _, source := range sources {
		if !strings.HasPrefix(source.Path, "acme-tp1-alice-3f9c2ab/") {
			t.Fatalf("chemin sans préfixe : %q", source.Path)
		}
	}
}

func TestUntarRefuseUneArchiveQuiNEnEstPasUne(t *testing.T) {
	if _, _, err := corpus.Untar([]byte("ceci n'est pas une archive")); err == nil {
		t.Fatal("une archive illisible doit être refusée")
	}
}

// ------------------------------------------------------------- constitution

func TestBuildConstruitUnIndexSansContenu(t *testing.T) {
	state := fakegh.NewState()
	depot(state, "a26.5n6.01.tp1.alice", map[string]string{"src/Solution.java": solution})
	depot(state, "a26.5n6.01.tp1.bruno", map[string]string{"src/Solution.java": solution})

	result := corpus.Build(client(t, state),
		[]corpus.Target{cible("a26.5n6.01.tp1.alice"), cible("a26.5n6.01.tp1.bruno")},
		options(t, inspect.Settings{}), nil)

	if len(result.Problems) != 0 {
		t.Fatalf("soucis : %+v", result.Problems)
	}
	if len(result.Corpus.Works) != 2 {
		t.Fatalf("copies : %d", len(result.Corpus.Works))
	}
	for _, work := range result.Corpus.Works {
		if work.PrintCount() == 0 || len(work.Files) != 1 {
			t.Fatalf("copie mal construite : %+v", work)
		}
		if work.Files[0].Path != "src/Solution.java" {
			t.Fatalf("chemin normalisé : %q", work.Files[0].Path)
		}
	}
	// Le préfixe de l'archive a bien été retiré, et le commit noté.
	for _, inspected := range result.Inspected {
		if inspected.Commit == "" || !strings.HasPrefix(inspected.Root, "acme-") {
			t.Fatalf("inspection : %+v", inspected)
		}
	}
	fichiers, octets, jetons, empreintes := result.Totals()
	if fichiers != 2 || octets == 0 || jetons == 0 || empreintes == 0 {
		t.Fatalf("totaux : %d %d %d %d", fichiers, octets, jetons, empreintes)
	}
}

// Un rapport qui passe des dépôts sous silence laisse croire qu'ils ont été
// regardés. Chaque cas doit donc porter un motif nommé.
func TestChaqueDepotNonAnalyseEstNommeAvecSonMotif(t *testing.T) {
	state := fakegh.NewState()
	depot(state, "vide", nil)
	depot(state, "interdit", map[string]string{"src/Solution.java": solution})
	depot(state, "binaire", map[string]string{"logo.ico": "\x00\x00 image", "notes.txt": "texte"})
	depot(state, "minuscule", map[string]string{"src/Petit.java": "class P { }"})
	depot(state, "bon", map[string]string{"src/Solution.java": solution})
	state.FailOn["GET /repos/acme/interdit/tarball"] = fakegh.Failure{
		Status: 403, Message: "Forbidden",
	}

	result := corpus.Build(client(t, state), []corpus.Target{
		cible("vide"), cible("interdit"), cible("binaire"),
		cible("minuscule"), cible("bon"),
	}, options(t, inspect.Settings{}), nil)

	if len(result.Corpus.Works) != 1 || result.Corpus.Works[0].ID != "bon" {
		t.Fatalf("copies retenues : %+v", result.Corpus.Works)
	}
	attendus := map[string]string{
		"vide":      corpus.NoCommit,
		"interdit":  corpus.Denied,
		"binaire":   corpus.NothingKept,
		"minuscule": corpus.TooShort,
	}
	for id, motif := range attendus {
		souci, trouve := probleme(result, id)
		if !trouve || souci.Reason != motif {
			t.Fatalf("« %s » : %+v, motif attendu %q", id, souci, motif)
		}
		if souci.Detail == "" {
			t.Fatalf("« %s » : motif sans détail", id)
		}
	}
	// Quand rien n'est retenu, il faut pouvoir voir fichier par fichier
	// pourquoi — c'est le seul cas où le décompte ne suffit pas.
	if souci, _ := probleme(result, "binaire"); len(souci.Skipped) != 2 {
		t.Fatalf("détail de l'écartement : %+v", souci.Skipped)
	}
}

func TestUnProfilQuiNeTrouveRienLeDitEtNommeLaRacine(t *testing.T) {
	state := fakegh.NewState()
	depot(state, "alice", map[string]string{
		"tp1/mon_tp/src/Solution.java": solution,
		"tp1/mon_tp/pom.xml":           "<project/>",
	})
	result := corpus.Build(client(t, state), []corpus.Target{cible("alice")},
		options(t, inspect.Settings{Profile: "next"}), nil)

	souci, trouve := probleme(result, "alice")
	if !trouve || souci.Reason != corpus.NothingKept {
		t.Fatalf("souci : %+v", souci)
	}
	if !strings.Contains(souci.Detail, "tp1/mon_tp") {
		t.Fatalf("la racine retenue doit être dite : %q", souci.Detail)
	}
	// Le motif majoritaire est dit, et c'est le vrai : un profil Next.js sur un
	// projet Java écarte sur le langage bien avant d'écarter sur le chemin.
	if !strings.Contains(souci.Detail, inspect.OtherTongue) {
		t.Fatalf("le motif majoritaire doit être dit : %q", souci.Detail)
	}
}

func TestLeProgresEstRapporteUneFoisParDepot(t *testing.T) {
	state := fakegh.NewState()
	cibles := make([]corpus.Target, 0, 5)
	for _, nom := range []string{"a", "b", "c", "d", "e"} {
		depot(state, nom, map[string]string{"src/Solution.java": solution})
		cibles = append(cibles, cible(nom))
	}
	vus := make(chan string, len(cibles))
	result := corpus.Build(client(t, state), cibles, options(t, inspect.Settings{}),
		func(done, total int, id string) {
			if total != len(cibles) || done < 1 || done > total {
				t.Errorf("avancement : %d/%d", done, total)
			}
			vus <- id
		})
	close(vus)
	if len(vus) != len(cibles) || len(result.Corpus.Works) != len(cibles) {
		t.Fatalf("%d avancements pour %d copies", len(vus), len(result.Corpus.Works))
	}
}

// ------------------------------------------------------------- estimation

func TestLEstimationExtrapoleCeQuElleAMesure(t *testing.T) {
	state := fakegh.NewState()
	cibles := make([]corpus.Target, 0, 12)
	for _, nom := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		depot(state, nom, map[string]string{
			"src/Solution.java": solution,
			"README.md":         strings.Repeat("Des consignes recopiées partout. ", 40),
		})
		cibles = append(cibles, cible(nom))
	}

	apercu := corpus.Sample(client(t, state), cibles, options(t, inspect.Settings{}), 3)
	if apercu.Estimate.Sampled != 3 || apercu.Estimate.Repos != 12 {
		t.Fatalf("échantillon : %+v", apercu.Estimate)
	}
	if apercu.Estimate.Pairs != 66 {
		t.Fatalf("paires prévues : %d", apercu.Estimate.Pairs)
	}
	if apercu.Estimate.Tokens == 0 || apercu.Estimate.Memory == 0 {
		t.Fatalf("estimation vide : %+v", apercu.Estimate)
	}
	// L'aperçu montre aussi ce que le profil retient, fichier par fichier.
	if len(apercu.Samples) != 3 || len(apercu.Samples[0].Kept) != 2 {
		t.Fatalf("aperçu : %+v", apercu.Samples)
	}
	for _, echantillon := range apercu.Samples {
		for _, kept := range echantillon.Kept {
			if kept.Content != nil {
				t.Fatal("l'aperçu ne doit pas porter le contenu des fichiers")
			}
		}
	}

	// Le markdown pèse ici plus que le code : le remède doit le proposer, et
	// le chiffrer.
	var retirer *corpus.Remedy
	for index, remede := range apercu.Estimate.Remedies {
		if strings.Contains(remede.Label, "Markdown") {
			retirer = &apercu.Estimate.Remedies[index]
		}
	}
	if retirer == nil {
		t.Fatalf("aucun remède ne propose de retirer le Markdown : %+v",
			apercu.Estimate.Remedies)
	}
	if retirer.Saves < 0.15 || !strings.Contains(retirer.Detail, "%") {
		t.Fatalf("remède non chiffré : %+v", retirer)
	}
	if len(retirer.Languages) == 0 {
		t.Fatalf("le remède doit dire quel réglage appliquer : %+v", retirer)
	}
}

func TestUnBudgetDepasseEstDitAvecSonChiffre(t *testing.T) {
	estimation := corpus.Estimate{Memory: 9 << 30, Seconds: 45 * 60}
	verdict := estimation.Against(corpus.RunnerBudget)
	if verdict.Fits {
		t.Fatal("neuf gigaoctets ne tiennent pas dans sept")
	}
	if len(verdict.Warnings) != 2 {
		t.Fatalf("avertissements : %+v", verdict.Warnings)
	}
	for _, avertissement := range verdict.Warnings {
		if !strings.ContainsAny(avertissement, "0123456789") {
			t.Fatalf("avertissement sans chiffre : %q", avertissement)
		}
	}
	petite := corpus.Estimate{Memory: 1 << 30, Seconds: 30}
	if !petite.Against(corpus.RunnerBudget).Fits {
		t.Fatal("une petite analyse doit tenir")
	}
}

func TestLesPoidsEtLesDureesSeLisent(t *testing.T) {
	cas := map[int64]string{512: "512 o", 4 << 10: "4 Ko", 3 << 20: "3 Mo", 2 << 30: "2.0 Go"}
	for octets, attendu := range cas {
		if obtenu := corpus.Bytes(octets); obtenu != attendu {
			t.Fatalf("%d octets : %q, attendu %q", octets, obtenu, attendu)
		}
	}
	durees := map[float64]string{0.5: "moins d'une seconde", 42: "42 s", 300: "5 min", 7200: "2.0 h"}
	for secondes, attendu := range durees {
		if obtenu := corpus.Duration(secondes); obtenu != attendu {
			t.Fatalf("%.0f s : %q, attendu %q", secondes, obtenu, attendu)
		}
	}
}

// La marque invisible est cherchée dans tout ce que l'inspection a retenu, et
// non dans le seul README : un étudiant qui déplace un fichier ne la fait pas
// disparaître.
func TestUneMarqueInvisibleEstRelevee(t *testing.T) {
	token, err := signature.New()
	if err != nil {
		t.Fatalf("tirage : %v", err)
	}
	signe := string(signature.Sign([]byte("# Travail pratique 1\n\nConsignes.\n"), token))

	state := fakegh.NewState()
	depot(state, "alice", map[string]string{
		"README.md": signe, "src/Solution.java": solution,
	})
	depot(state, "bruno", map[string]string{"src/Solution.java": solution})

	result := corpus.Build(client(t, state),
		[]corpus.Target{cible("alice"), cible("bruno")},
		options(t, inspect.Settings{}), nil)

	for _, work := range result.Corpus.Works {
		if work.ID == "alice" {
			if work.Extras.Signature != signature.Text(token) {
				t.Fatalf("marque relevée : %q, attendue %q",
					work.Extras.Signature, signature.Text(token))
			}
			continue
		}
		// Une copie sans marque n'en invente pas : son absence ne prouve rien,
		// mais elle ne doit pas être confondue avec une marque vide partagée.
		if work.Extras.Signature != "" {
			t.Fatalf("une marque est apparue de nulle part : %+v", work)
		}
	}
	for _, inspected := range result.Inspected {
		if inspected.ID == "alice" && !inspected.Signed {
			t.Fatalf("la copie signée doit être rapportée comme telle : %+v", inspected)
		}
	}

	// Et la marque n'entre pas dans la mesure : des blancs ne produisent aucun
	// jeton, et deux copies ne diffèrent pas parce que l'une est signée.
	sans := corpus.Build(client(t, state), []corpus.Target{cible("bruno")},
		options(t, inspect.Settings{}), nil)
	avec, sansMarque := 0, 0
	for _, work := range result.Corpus.Works {
		if work.ID == "alice" {
			avec = work.PrintCount()
		}
	}
	for _, work := range sans.Corpus.Works {
		sansMarque = work.PrintCount()
	}
	if avec < sansMarque {
		t.Fatalf("la marque a coûté des empreintes : %d contre %d", avec, sansMarque)
	}
}
