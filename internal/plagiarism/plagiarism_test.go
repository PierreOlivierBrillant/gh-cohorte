package plagiarism_test

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plagiarism"
)

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

    public int taille() {
        return articles.size();
    }
}
`

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

const annexe = `
public class Annexe {
    public String decrire(int[] valeurs) {
        StringBuilder texte = new StringBuilder();
        texte.append("Le tableau contient ");
        texte.append(valeurs.length);
        texte.append(" valeurs, dont la première est ");
        if (valeurs.length > 0) {
            texte.append(valeurs[0]);
        } else {
            texte.append("aucune");
        }
        texte.append(".");
        return texte.toString();
    }
}
`

const autre = `
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

func client(t *testing.T, state *fakegh.State) *ghapi.Client {
	t.Helper()
	serveur := fakegh.New(state)
	t.Cleanup(serveur.Close)
	c, err := ghapi.New(ghapi.Options{
		Host: "127.0.0.1", Token: "jeton", BaseURL: serveur.URL(),
		Sleep: func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("client : %v", err)
	}
	return c
}

func depot(state *fakegh.State, nom string, fichiers map[string]string) {
	state.AddRepo("acme", nom, true)
	state.SeedCommit("acme/"+nom, fichiers, "main")
}

func cible(nom string) corpus.Target {
	return corpus.Target{ID: nom, Label: nom, Origin: "01", Owner: "acme", Repo: nom}
}

// cohorte monte un groupe : deux copies identiques, une différente, toutes
// parties du même gabarit.
func cohorte(t *testing.T) *ghapi.Client {
	t.Helper()
	state := fakegh.NewState()
	depot(state, "modele", map[string]string{"src/Inventaire.java": gabarit})
	depot(state, "alice", map[string]string{
		"src/Inventaire.java": gabarit, "src/Solution.java": solution,
	})
	depot(state, "bruno", map[string]string{
		"src/Inventaire.java": gabarit, "src/Solution.java": solution,
	})
	depot(state, "claire", map[string]string{
		"src/Inventaire.java": gabarit, "src/Travail.java": autre,
	})
	// David a recopié la solution, puis écrit son annexe à lui : sa paire avec
	// Alice a donc des fichiers reliés et d'autres qui ne le sont pas.
	depot(state, "david", map[string]string{
		"src/Inventaire.java": gabarit, "src/Solution.java": solution,
		"src/Annexe.java": annexe,
	})
	return client(t, state)
}

// demande est la configuration normale : on désigne le gabarit distribué,
// puisque l'outil sait lequel il a distribué. Sans lui, sur un groupe de cette
// taille, les quatre copies se ressembleraient par leur seul squelette commun
// — c'est précisément ce que l'épreuve du gabarit montre plus bas.
func demande() plagiarism.Request {
	return plagiarism.Request{
		Assignment: "a26.5n6.01.tp1", Org: "acme",
		Targets: []corpus.Target{
			cible("alice"), cible("bruno"), cible("claire"), cible("david"),
		},
		Baseline: []corpus.Target{cible("modele")},
	}
}

// ------------------------------------------------------------------ analyse

func TestUneAnalyseRendUnRapportComplet(t *testing.T) {
	rapport, err := plagiarism.Run(cohorte(t), demande(), nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	if rapport.Version != plagiarism.Version || rapport.CreatedAt == "" {
		t.Fatalf("rapport mal formé : %+v", rapport.Version)
	}
	// L'avertissement voyage avec le rapport : relu dans six mois ou transmis
	// à un collègue, il doit porter le sien.
	if rapport.Disclaimer != plagiarism.Disclaimer {
		t.Fatal("le rapport doit porter l'avertissement")
	}
	if rapport.Profile.ID != inspect.AllProfile {
		t.Fatalf("profil appliqué : %q", rapport.Profile.ID)
	}
	if rapport.Analyzed() != 4 || len(rapport.Problems) != 0 {
		t.Fatalf("copies analysées : %d, soucis : %+v", rapport.Analyzed(), rapport.Problems)
	}

	match, trouve := rapport.Match("alice", "bruno")
	if !trouve || match.Similarity < 0.99 {
		t.Fatalf("deux copies identiques : %.2f (trouvée : %v)", match.Similarity, trouve)
	}
	// Trois copies sur quatre partagent la même solution. C'est une fraude à
	// trois, pas un squelette commun : la règle de bruit ne doit pas l'effacer
	// sous prétexte que soixante-quinze pour cent des copies la portent.
	for _, autre := range []string{"bruno", "david"} {
		if match, trouve := rapport.Match("alice", autre); !trouve || match.Similarity < 0.5 {
			t.Fatalf("« alice » et « %s » : %.2f (trouvée : %v)",
				autre, match.Similarity, trouve)
		}
	}
	// Claire a résolu le même exercice autrement : elle ne doit pas remonter.
	if match, trouve := rapport.Match("alice", "claire"); trouve && match.Similarity > 0.3 {
		t.Fatalf("une solution différente remonte à %.2f", match.Similarity)
	}

	// Le commit archivé est noté : c'est ce qui permet de rouvrir la paire
	// plus tard sur exactement le même état.
	if rapport.Commit("alice") == "" {
		t.Fatal("le commit archivé doit être noté")
	}
}

// Sans le retrait du gabarit, toutes les copies se ressemblent et le rapport
// est inutilisable. C'est ce que l'outil sait faire et qu'un comparateur
// générique ne peut pas : il connaît le modèle qu'il a distribué.
//
// L'épreuve porte sur deux copies seulement, et c'est délibéré. À trois copies
// et plus, le garde-fou statistique écarte déjà le gabarit tout seul — il est
// chez tout le monde —, et l'effet du retrait explicite ne se verrait pas. À
// deux, ce garde-fou ne s'applique pas : « chez tout le monde » voudrait dire
// « chez les deux », c'est-à-dire exactement ce qu'on cherche.
func TestLeGabaritDistribueEstRetireQuandOnLeDesigne(t *testing.T) {
	paire := plagiarism.Request{
		Assignment: "a26.5n6.01.tp1", Org: "acme",
		Targets: []corpus.Target{cible("alice"), cible("claire")},
	}

	sans, err := plagiarism.Run(cohorte(t), paire, nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	brut, trouve := sans.Match("alice", "claire")
	if !trouve || brut.Similarity < 0.5 {
		t.Fatalf("sans retrait, le gabarit commun devrait les rapprocher : %.2f", brut.Similarity)
	}

	paire.Baseline = []corpus.Target{cible("modele")}
	avec, err := plagiarism.Run(cohorte(t), paire, nil)
	if err != nil {
		t.Fatalf("analyse avec gabarit : %v", err)
	}
	if avec.Result.IgnoredBaseline == 0 {
		t.Fatal("aucune empreinte de gabarit écartée")
	}
	if net, trouve := avec.Match("alice", "claire"); trouve && net.Similarity >= brut.Similarity {
		t.Fatalf("le gabarit n'a rien changé : %.2f puis %.2f", brut.Similarity, net.Similarity)
	}

	// Et il ne doit pas effacer ce qu'on cherche : deux copies identiques
	// restent identiques une fois le gabarit retiré.
	vraies := plagiarism.Request{
		Targets:  []corpus.Target{cible("alice"), cible("bruno")},
		Baseline: []corpus.Target{cible("modele")},
	}
	rapport, err := plagiarism.Run(cohorte(t), vraies, nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	if match, _ := rapport.Match("alice", "bruno"); match.Similarity < 0.99 {
		t.Fatalf("le gabarit a effacé une vraie copie : %.2f", match.Similarity)
	}
}

func TestUnGabaritIllisibleNArretePasLAnalyse(t *testing.T) {
	requete := demande()
	requete.Baseline = []corpus.Target{cible("inexistant")}
	rapport, err := plagiarism.Run(cohorte(t), requete, nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	if rapport.Analyzed() != 4 {
		t.Fatalf("copies analysées : %d", rapport.Analyzed())
	}
	// Mais cela se dit : sans le retrait, tous les scores sont gonflés de la
	// même quantité, et l'enseignant doit savoir qu'il les lit ainsi.
	trouve := false
	for _, souci := range rapport.Problems {
		if souci.ID == "inexistant" {
			trouve = true
		}
	}
	if !trouve {
		t.Fatalf("le gabarit illisible doit être signalé : %+v", rapport.Problems)
	}
}

func TestUneDemandeImpossibleEstRefuseeAvantTousTelechargement(t *testing.T) {
	cas := map[string]plagiarism.Request{
		"une seule copie":   {Targets: []corpus.Target{cible("alice")}},
		"bruit hors bornes": {Targets: []corpus.Target{cible("a"), cible("b")}, Noise: 2},
		"seuil hors bornes": {Targets: []corpus.Target{cible("a"), cible("b")}, MinSimilarity: 1.5},
		"bornes négatives":  {Targets: []corpus.Target{cible("a"), cible("b")}, Kgram: -1},
		"profil inconnu":    {Targets: []corpus.Target{cible("a"), cible("b")}, Inspection: inspect.Settings{Profile: "nextjs"}},
	}
	for nom, requete := range cas {
		if err := requete.Validate(); err == nil {
			t.Fatalf("« %s » aurait dû être refusée", nom)
		}
	}
}

// ------------------------------------------------------------------ fichier

func TestUnRapportSEcritEtSeRelit(t *testing.T) {
	rapport, err := plagiarism.Run(cohorte(t), demande(), nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	dossier := t.TempDir()
	cheminJSON, cheminCSV, err := rapport.Save(dossier)
	if err != nil {
		t.Fatalf("écriture : %v", err)
	}

	info, err := os.Stat(cheminJSON)
	if err != nil {
		t.Fatalf("fichier : %v", err)
	}
	// Des données d'étudiants : lisibles par leur propriétaire, personne d'autre.
	if mode := info.Mode().Perm(); mode != 0o600 && mode != 0o666 {
		t.Fatalf("permissions du rapport : %v", mode)
	}

	relu, err := plagiarism.Load(cheminJSON)
	if err != nil {
		t.Fatalf("relecture : %v", err)
	}
	if relu.Analyzed() != rapport.Analyzed() ||
		len(relu.Result.Matches) != len(rapport.Result.Matches) {
		t.Fatalf("rapport relu différent : %d copies, %d paires",
			relu.Analyzed(), len(relu.Result.Matches))
	}
	// L'index est gardé : rouvrir un rapport ou déplacer un seuil ne doit pas
	// demander de retélécharger trois cents dépôts.
	if len(relu.Index.Works) == 0 || relu.Index.Works[0].PrintCount() == 0 {
		t.Fatal("l'index doit survivre à l'écriture")
	}

	contenu, err := os.ReadFile(cheminCSV)
	if err != nil {
		t.Fatalf("CSV : %v", err)
	}
	lignes, err := csv.NewReader(strings.NewReader(string(contenu))).ReadAll()
	if err != nil {
		t.Fatalf("CSV illisible : %v", err)
	}
	if len(lignes) < 2 || lignes[0][0] != "copie_a" || len(lignes[0]) != 10 {
		t.Fatalf("en-tête du CSV : %v", lignes[0])
	}
	if !strings.Contains(cheminJSON, filepath.Join(dossier, plagiarism.Dir)) {
		t.Fatalf("emplacement du rapport : %q", cheminJSON)
	}

	// Les rapports se listent du plus récent au plus ancien.
	if trouves := plagiarism.List(dossier); len(trouves) != 1 || trouves[0] != cheminJSON {
		t.Fatalf("rapports listés : %v", trouves)
	}
}

func TestUnRapportIncomprehensibleEstRefuseAvecUnMotif(t *testing.T) {
	dossier := t.TempDir()
	futur := filepath.Join(dossier, "futur.json")
	os.WriteFile(futur, []byte(`{"version": 99}`), 0o600)
	if _, err := plagiarism.Load(futur); err == nil ||
		!strings.Contains(err.Error(), "upgrade") {
		t.Fatalf("un rapport d'une version future doit proposer la mise à jour : %v", err)
	}

	autre := filepath.Join(dossier, "autre.json")
	os.WriteFile(autre, []byte(`{"quelque": "chose"}`), 0o600)
	if _, err := plagiarism.Load(autre); err == nil {
		t.Fatal("un fichier qui n'est pas un rapport doit être refusé")
	}
	if _, err := plagiarism.Load(filepath.Join(dossier, "absent.json")); err == nil {
		t.Fatal("un rapport absent doit être refusé")
	}
}

// ------------------------------------------------------------------ paires

// C'est la propriété qui rend un rapport vérifiable : ce qui est surligné doit
// être réellement commun aux deux fichiers.
func TestLesFragmentsSurlignesSontVraimentCommuns(t *testing.T) {
	serveur := cohorte(t)
	rapport, err := plagiarism.Run(serveur, demande(), nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}

	paire, err := plagiarism.OpenPair(serveur, rapport, "alice", "bruno")
	if err != nil {
		t.Fatalf("ouverture de la paire : %v", err)
	}
	vue, err := paire.File("src/Solution.java", "src/Solution.java")
	if err != nil {
		t.Fatalf("vue de fichier : %v", err)
	}
	if len(vue.Spans) == 0 {
		t.Fatal("aucun fragment situé")
	}
	if !strings.Contains(vue.Left.Text, "plusGrand") || vue.Left.Lines < 10 {
		t.Fatalf("le texte du fichier doit être rendu : %d lignes", vue.Left.Lines)
	}
	// Le flux de jetons aussi : c'est lui qui rend limpide ce que le moteur a
	// comparé, et pourquoi renommer une variable n'aurait rien changé.
	if len(vue.Left.Tokens) == 0 || vue.Left.Tokens[0].Text == "" {
		t.Fatal("le flux de jetons doit être rendu")
	}

	for _, span := range vue.Spans {
		gauche := vue.Left.Text[span.LeftStart:span.LeftEnd]
		droite := vue.Right.Text[span.RightStart:span.RightEnd]
		if gauche != droite {
			t.Fatalf("fragment surligné différent des deux côtés :\n%q\n%q", gauche, droite)
		}
		if span.LeftFrom < 1 || span.LeftTo > vue.Left.Lines ||
			span.LeftFrom > span.LeftTo {
			t.Fatalf("lignes du fragment hors du fichier : %+v", span)
		}
		if span.Tokens <= 0 {
			t.Fatalf("fragment vide : %+v", span)
		}
	}
}

func TestLaVueDeProjetMontreCeQuiEstRelieEtCeQuiNeLEstPas(t *testing.T) {
	serveur := cohorte(t)
	rapport, err := plagiarism.Run(serveur, demande(), nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	paire, err := plagiarism.OpenPair(serveur, rapport, "alice", "david")
	if err != nil {
		t.Fatalf("ouverture : %v", err)
	}

	vue := paire.Project(rapport)
	if vue.Left.ID != "alice" || vue.Right.ID != "david" {
		t.Fatalf("côtés : %+v / %+v", vue.Left, vue.Right)
	}
	relies := map[string]bool{}
	for _, lien := range vue.Links {
		relies[lien.LeftPath] = true
		if lien.LeftPath == lien.RightPath && !lien.SameName {
			t.Fatalf("même chemin non signalé : %+v", lien)
		}
	}
	// La solution recopiée est reliée ; l'annexe que David a écrite lui-même
	// ne l'est pas, et c'est cette asymétrie qui permet de juger.
	if !relies["src/Solution.java"] {
		t.Fatalf("la solution recopiée doit être reliée : %+v", vue.Links)
	}
	// Et chaque fichier sans correspondant dit pourquoi : « aucun passage
	// commun » est un résultat, « trop court » est un aveu.
	seul, trouve := seulement(vue.RightOnly, "src/Annexe.java")
	if !trouve || seul.Reason != plagiarism.NothingShared {
		t.Fatalf("fichiers de droite sans lien : %+v", vue.RightOnly)
	}
}

func TestOuvrirUnePaireQuiNExistePasEstRefuse(t *testing.T) {
	serveur := cohorte(t)
	rapport, err := plagiarism.Run(serveur, demande(), nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	if _, err := plagiarism.OpenPair(serveur, rapport, "alice", "inconnu"); err == nil {
		t.Fatal("une paire absente du rapport doit être refusée")
	}
	paire, err := plagiarism.OpenPair(serveur, rapport, "alice", "bruno")
	if err != nil {
		t.Fatalf("ouverture : %v", err)
	}
	if _, err := paire.File("src/Absent.java", "src/Solution.java"); err == nil {
		t.Fatal("deux fichiers sans fragment commun doivent être refusés")
	}
}

func contient(liste []string, valeur string) bool {
	for _, element := range liste {
		if element == valeur {
			return true
		}
	}
	return false
}

func seulement(liste []plagiarism.Lonely, chemin string) (plagiarism.Lonely, bool) {
	for _, element := range liste {
		if element.Path == chemin {
			return element, true
		}
	}
	return plagiarism.Lonely{}, false
}

// Un fichier retenu par le profil mais trop court pour former un k-gramme ne
// doit pas disparaître du rapport : il a été retenu, on croirait l'avoir
// comparé. Il paraît donc parmi les fichiers sans correspondant, avec sa raison.
func TestUnFichierTropCourtResteVisibleAvecSaRaison(t *testing.T) {
	state := fakegh.NewState()
	for _, nom := range []string{"alice", "bruno"} {
		depot(state, nom, map[string]string{
			"src/Solution.java": solution,
			"src/Barils.java":   "class Barils { }",
		})
	}
	serveur := client(t, state)
	requete := plagiarism.Request{
		Targets: []corpus.Target{cible("alice"), cible("bruno")},
	}
	rapport, err := plagiarism.Run(serveur, requete, nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	paire, err := plagiarism.OpenPair(serveur, rapport, "alice", "bruno")
	if err != nil {
		t.Fatalf("ouverture : %v", err)
	}
	seul, trouve := seulement(paire.Project(rapport).LeftOnly, "src/Barils.java")
	if !trouve || seul.Reason != plagiarism.TooShortToCompare {
		t.Fatalf("fichiers sans correspondant : %+v", paire.Project(rapport).LeftOnly)
	}
}
