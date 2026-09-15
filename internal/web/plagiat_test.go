package web_test

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/exchange"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

const gabaritJava = `
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

const solutionJava = `
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

const autreJava = `
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

// groupeAvecCopies monte un groupe dont trois personnes ont remis : deux copies
// identiques et une différente, toutes parties du même gabarit.
func groupeAvecCopies(t *testing.T) (*harnais, string) {
	t.Helper()
	state := fakegh.NewState()

	modele := state.AddRepo("acme", "modele-tp1", true)
	_ = modele
	state.SeedCommit("acme/modele-tp1",
		map[string]string{"src/Inventaire.java": gabaritJava}, "main")

	copies := map[string]string{
		"a26.5n6.01.tp1.alice-martin":  solutionJava,
		"a26.5n6.01.tp1.bruno-tanguay": solutionJava,
		"a26.5n6.01.tp1.claire-otis":   autreJava,
	}
	for nom, source := range copies {
		state.AddRepo("acme", nom, true)
		state.SeedCommit("acme/"+nom, map[string]string{
			"src/Inventaire.java": gabaritJava,
			"src/Solution.java":   source,
			"assets/logo.ico":     "\x00\x00 image",
			"README.md":           "# Travail pratique 1",
		}, "main")
	}

	cours := classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "01",
		Students: []roster.Person{
			{FullName: "Alice Martin", Username: "alice"},
			{FullName: "Bruno Tanguay", Username: "bruno"},
			{FullName: "Claire Otis", Username: "claire"},
		},
		Defaults: classroom.Defaults{Template: "acme/modele-tp1"},
	}
	return avantLeRegistre(t, state, cours), "a26.5n6.01"
}

func TestPlagiatOffreSesProfilsEtSesLangages(t *testing.T) {
	h := nouveau(t, nil)
	var reponse struct {
		Disclaimer string `json:"disclaimer"`
		Profiles   []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Note  string `json:"note"`
		} `json:"profiles"`
		Languages []struct {
			ID    string `json:"id"`
			Kgram int    `json:"kgram"`
		} `json:"languages"`
		Excluded map[string][]string `json:"excluded"`
	}
	h.json(http.MethodGet, "/api/plagiat/options", nil, &reponse)

	// L'avertissement vient du domaine : il ne doit pas exister en trois
	// versions, une par interface.
	if !strings.Contains(reponse.Disclaimer, "n'est pas un détecteur de plagiat") {
		t.Fatalf("avertissement : %q", reponse.Disclaimer)
	}
	if len(reponse.Profiles) < 5 || reponse.Profiles[0].ID != "tout" {
		t.Fatalf("profils : %+v", reponse.Profiles)
	}
	for _, profil := range reponse.Profiles {
		if profil.Label == "" || profil.Note == "" {
			t.Fatalf("un profil sans explication ne sera pas utilisé : %+v", profil)
		}
	}
	if len(reponse.Languages) != 13 {
		t.Fatalf("langages offerts : %d", len(reponse.Languages))
	}
	// La page doit pouvoir dire ce qui est écarté d'office : sans cela,
	// personne ne comprend pourquoi un fichier manque.
	if len(reponse.Excluded["directories"]) == 0 || len(reponse.Excluded["binary"]) == 0 {
		t.Fatalf("exclusions annoncées : %+v", reponse.Excluded)
	}
}

// L'aperçu montre ce qui entre et ce que ça coûtera, avant de lancer quoi que
// ce soit : une analyse à l'aveugle sur cinq ans de dépôts se termine mal.
func TestPlagiatMontreCeQuiEntreAvantDeLancer(t *testing.T) {
	h, scope := groupeAvecCopies(t)
	var apercu struct {
		Repos    int    `json:"repos"`
		Baseline string `json:"baseline"`
		Profile  struct {
			ID string `json:"id"`
		} `json:"profile"`
		Samples []struct {
			Repo    string                            `json:"repo"`
			Root    string                            `json:"root"`
			Kept    []struct{ Path, Language string } `json:"kept"`
			Skipped []struct{ Path, Reason string }   `json:"skipped"`
		} `json:"samples"`
		Estimate struct {
			Repos    int   `json:"repos"`
			Pairs    int   `json:"pairs"`
			Tokens   int64 `json:"tokens"`
			Fits     bool  `json:"fits"`
			Remedies []struct {
				Label string  `json:"label"`
				Saves float64 `json:"saves"`
			} `json:"remedies"`
		} `json:"estimate"`
	}
	h.json(http.MethodPost, "/api/classrooms/"+scope+"/assignments/tp1/plagiat/preview",
		map[string]any{"profile": "tout"}, &apercu)

	if apercu.Repos != 3 || apercu.Estimate.Pairs != 3 {
		t.Fatalf("aperçu : %d dépôts, %d paires", apercu.Repos, apercu.Estimate.Pairs)
	}
	if apercu.Baseline != "acme/modele-tp1" {
		t.Fatalf("le gabarit distribué doit être nommé : %q", apercu.Baseline)
	}
	if len(apercu.Samples) == 0 {
		t.Fatal("aucun dépôt échantillonné")
	}
	echantillon := apercu.Samples[0]
	// Le README, le gabarit et la solution ; l'image est écartée.
	if len(echantillon.Kept) != 3 {
		t.Fatalf("fichiers retenus : %+v", echantillon.Kept)
	}
	// Le fichier binaire est écarté avec son motif : on doit pouvoir dire
	// pourquoi il manque plutôt que de le laisser disparaître.
	trouve := false
	for _, ecarte := range echantillon.Skipped {
		if strings.HasSuffix(ecarte.Path, ".ico") && ecarte.Reason != "" {
			trouve = true
		}
	}
	if !trouve {
		t.Fatalf("écartements annoncés : %+v", echantillon.Skipped)
	}
	if !apercu.Estimate.Fits || apercu.Estimate.Tokens == 0 {
		t.Fatalf("estimation : %+v", apercu.Estimate)
	}
	// Le Markdown pèse assez pour qu'un remède chiffré le propose.
	if len(apercu.Estimate.Remedies) == 0 {
		t.Fatal("aucun remède proposé")
	}
	for _, remede := range apercu.Estimate.Remedies {
		if remede.Label == "" || remede.Saves <= 0 {
			t.Fatalf("remède non chiffré : %+v", remede)
		}
	}
}

func TestPlagiatAnalyseEtRelitSonRapport(t *testing.T) {
	h, scope := groupeAvecCopies(t)
	bilan := h.travail(http.MethodPost,
		"/api/classrooms/"+scope+"/assignments/tp1/plagiat",
		map[string]any{"profile": "tout", "baseline": true})

	if bilan["status"] != "terminé" {
		t.Fatalf("analyse : %+v", bilan)
	}
	resultat, _ := bilan["result"].(map[string]any)
	nom, _ := resultat["report"].(string)
	if nom == "" {
		t.Fatalf("aucun rapport produit : %+v", resultat)
	}
	if works, _ := resultat["works"].(float64); works != 3 {
		t.Fatalf("copies analysées : %+v", resultat["works"])
	}

	// Le rapport se relit sans rien relancer.
	var rapport struct {
		Assignment string              `json:"assignment"`
		Disclaimer string              `json:"disclaimer"`
		Profile    struct{ ID string } `json:"profile"`
		Works      []struct {
			ID     string   `json:"id"`
			Label  string   `json:"label"`
			Origin string   `json:"origin"`
			Files  []string `json:"files"`
		} `json:"works"`
		Result struct {
			Threshold float64 `json:"threshold"`
			Matches   []struct {
				Left       string  `json:"left"`
				Right      string  `json:"right"`
				LeftName   string  `json:"left_name"`
				Similarity float64 `json:"similarity"`
			} `json:"matches"`
			IgnoredBaseline int `json:"ignored_baseline"`
		} `json:"result"`
		Problems []struct{ Reason string } `json:"problems"`
	}
	h.json(http.MethodGet, "/api/plagiat/reports/"+nom, nil, &rapport)

	if rapport.Assignment != "a26.5n6.01.tp1" || rapport.Disclaimer == "" {
		t.Fatalf("rapport : %+v", rapport.Assignment)
	}
	if rapport.Result.IgnoredBaseline == 0 {
		t.Fatal("le gabarit distribué devait être écarté")
	}
	if len(rapport.Works) != 3 {
		t.Fatalf("copies : %+v", rapport.Works)
	}
	// Le rapport parle en noms complets, pas en slugs : un rapport qu'il faut
	// traduire ligne à ligne ne sera pas lu.
	for _, work := range rapport.Works {
		if work.Label == "" || work.Origin != "a26.5n6.01" {
			t.Fatalf("copie mal étiquetée : %+v", work)
		}
	}

	var paire struct{ gauche, droite string }
	for _, match := range rapport.Result.Matches {
		if match.Similarity > 0.9 {
			paire.gauche, paire.droite = match.Left, match.Right
		}
		if match.LeftName == "" {
			t.Fatalf("paire sans nom complet : %+v", match)
		}
	}
	if paire.gauche == "" {
		t.Fatalf("les deux copies identiques n'ont pas été appariées : %+v",
			rapport.Result.Matches)
	}

	// La liste des rapports le retrouve.
	var liste struct {
		Reports []struct {
			Name       string `json:"name"`
			Assignment string `json:"assignment"`
			Works      int    `json:"works"`
		} `json:"reports"`
	}
	h.json(http.MethodGet, "/api/plagiat/reports", nil, &liste)
	if len(liste.Reports) != 1 || liste.Reports[0].Name != nom {
		t.Fatalf("rapports listés : %+v", liste.Reports)
	}

	verifierPaire(t, h, nom, paire.gauche, paire.droite)
}

// verifierPaire ouvre les deux vues de comparaison et vérifie que ce qui est
// surligné est réellement commun aux deux fichiers.
func verifierPaire(t *testing.T, h *harnais, rapport, gauche, droite string) {
	t.Helper()
	var projet struct {
		Match struct {
			Similarity float64 `json:"similarity"`
		} `json:"match"`
		Project struct {
			Left  struct{ ID, Commit string } `json:"left"`
			Right struct{ ID, Commit string } `json:"right"`
			Links []struct {
				LeftPath        string `json:"left_path"`
				RightPath       string `json:"right_path"`
				LongestFragment int    `json:"longest_fragment"`
				SameName        bool   `json:"same_name"`
			} `json:"links"`
		} `json:"project"`
	}
	h.json(http.MethodPost, "/api/plagiat/reports/"+rapport+"/pair",
		map[string]any{"left": gauche, "right": droite}, &projet)

	if projet.Project.Left.Commit == "" {
		t.Fatal("la vue de projet doit dire sur quel commit elle porte")
	}
	if len(projet.Project.Links) == 0 {
		t.Fatalf("aucun fichier relié : %+v", projet.Project)
	}
	lien := projet.Project.Links[0]
	if lien.LongestFragment <= 0 || !lien.SameName {
		t.Fatalf("lien : %+v", lien)
	}

	var fichier struct {
		Left struct {
			Path   string `json:"path"`
			Text   string `json:"text"`
			Lines  int    `json:"lines"`
			Tokens []struct {
				Text string `json:"text"`
			} `json:"tokens"`
		} `json:"left"`
		Right struct {
			Text string `json:"text"`
		} `json:"right"`
		Spans []struct {
			LeftStart  int `json:"left_start"`
			LeftEnd    int `json:"left_end"`
			RightStart int `json:"right_start"`
			RightEnd   int `json:"right_end"`
			LeftFrom   int `json:"left_from"`
			LeftTo     int `json:"left_to"`
		} `json:"spans"`
	}
	h.json(http.MethodPost, "/api/plagiat/reports/"+rapport+"/file",
		map[string]any{
			"left": gauche, "right": droite,
			"left_path": lien.LeftPath, "right_path": lien.RightPath,
		}, &fichier)

	if fichier.Left.Text == "" || len(fichier.Spans) == 0 {
		t.Fatalf("vue de fichier : %d lignes, %d fragments",
			fichier.Left.Lines, len(fichier.Spans))
	}
	// Le flux de jetons est rendu : c'est lui qui montre ce que le moteur a
	// réellement comparé, et pourquoi renommer une variable n'y change rien.
	identifiants := 0
	for _, jeton := range fichier.Left.Tokens {
		if jeton.Text == "ID" {
			identifiants++
		}
	}
	if identifiants == 0 {
		t.Fatal("le flux de jetons doit montrer les identifiants normalisés")
	}
	// Et ce qui est surligné est vraiment commun aux deux côtés.
	for _, span := range fichier.Spans {
		if span.LeftStart >= span.LeftEnd || span.LeftEnd > len(fichier.Left.Text) {
			t.Fatalf("fragment hors du fichier : %+v", span)
		}
		if fichier.Left.Text[span.LeftStart:span.LeftEnd] !=
			fichier.Right.Text[span.RightStart:span.RightEnd] {
			t.Fatalf("fragment surligné différent des deux côtés : %+v", span)
		}
		if span.LeftFrom < 1 || span.LeftTo > fichier.Left.Lines {
			t.Fatalf("lignes du fragment : %+v", span)
		}
	}
}

// Le nom d'un rapport vient de l'adresse : il ne doit désigner qu'un fichier du
// dossier des rapports, jamais un chemin composé par quelqu'un d'autre.
func TestPlagiatRefuseUnNomDeRapportComposé(t *testing.T) {
	h := nouveau(t, nil)
	for _, nom := range []string{"..%2f..%2fetc%2fpasswd", "..", "sous%2fdossier"} {
		reponse, _ := h.requete(http.MethodGet, "/api/plagiat/reports/"+nom, nil)
		if reponse.StatusCode < 400 {
			t.Fatalf("« %s » a été accepté (statut %d)", nom, reponse.StatusCode)
		}
	}
}

func TestPlagiatRefuseUnTravailSansAssezDeCopies(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.alice-martin", true)
	state.SeedCommit("acme/a26.5n6.01.tp1.alice-martin",
		map[string]string{"src/Solution.java": solutionJava}, "main")
	h := avantLeRegistre(t, state, classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "01",
		Students: []roster.Person{{FullName: "Alice Martin", Username: "alice"}},
	})

	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/a26.5n6.01/assignments/tp1/plagiat",
		map[string]any{"profile": "tout"})
	if reponse.StatusCode < 400 {
		t.Fatalf("une seule copie a été acceptée : %s", contenu)
	}
	// Le refus dit ce qui manque, et sous quelle portée : c'est souvent la
	// portée qu'il faut élargir, pas la liste qu'il faut corriger.
	if !strings.Contains(string(contenu), "au moins deux") ||
		!strings.Contains(string(contenu), "groupe") {
		t.Fatalf("le refus doit dire ce qui manque : %s", contenu)
	}
}

func TestPlagiatRefuseUnProfilInconnuAvantDeTelecharger(t *testing.T) {
	h, scope := groupeAvecCopies(t)
	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+scope+"/assignments/tp1/plagiat/preview",
		map[string]any{"profile": "nextjs"})
	if reponse.StatusCode < 400 || !strings.Contains(string(contenu), "nextjs") {
		t.Fatalf("profil inconnu : statut %d — %s", reponse.StatusCode, contenu)
	}
}

// ------------------------------------------------------ règles partagées

// Les équivalences de sigles ne sont pas un réglage : ce sont des faits que le
// département connaît et que l'outil ne peut pas deviner. Elles vivent dans le
// registre, déclarées une fois pour toute l'équipe.
func TestLesReglesSeDeclarentEtSePartagent(t *testing.T) {
	h, _ := groupeAvecCopies(t)

	var vide struct {
		Declared struct {
			Courses []struct{ ID string } `json:"courses"`
		} `json:"declared"`
		Override bool `json:"override"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/rules", nil, &vide)
	if len(vide.Declared.Courses) != 0 || vide.Override {
		t.Fatalf("règles de départ : %+v", vide)
	}

	var ecrites struct {
		Declared struct {
			Courses []struct {
				ID    string   `json:"id"`
				Label string   `json:"label"`
				Codes []string `json:"codes"`
			} `json:"courses"`
			Assignments []struct {
				ID      string   `json:"id"`
				Aliases []string `json:"aliases"`
			} `json:"assignments"`
		} `json:"declared"`
	}
	h.json(http.MethodPut, "/api/orgs/acme/rules", map[string]any{
		"courses": []map[string]any{
			{"id": "prog3", "label": "Programmation 3", "codes": []string{"5N6", "5M6"}},
		},
		"assignments": []map[string]any{
			{"id": "tp1", "aliases": []string{"TP-1"}},
		},
	}, &ecrites)

	if len(ecrites.Declared.Courses) != 1 ||
		len(ecrites.Declared.Courses[0].Codes) != 2 {
		t.Fatalf("cours déclarés : %+v", ecrites.Declared.Courses)
	}
	// Les sigles sont mis en forme par le domaine : « 5N6 » sur un plan de
	// cours et « 5n6 » dans un nom de dépôt sont le même sigle.
	if ecrites.Declared.Courses[0].Codes[0] != "5n6" {
		t.Fatalf("sigles mis en forme : %+v", ecrites.Declared.Courses[0].Codes)
	}

	// Et elles se relisent : elles sont dans le registre, pas dans cette page.
	var relues struct {
		Declared struct {
			Courses []struct{ ID string } `json:"courses"`
		} `json:"declared"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/rules", nil, &relues)
	if len(relues.Declared.Courses) != 1 {
		t.Fatalf("règles relues : %+v", relues)
	}

	// Les profils offerts à l'analyse s'enrichissent des règles : une addition
	// au registre paraît dans les menus sans qu'on touche au navigateur.
	h.json(http.MethodPut, "/api/orgs/acme/rules", map[string]any{
		"courses": []map[string]any{{"id": "prog3", "codes": []string{"5n6"}}},
		"profiles": []map[string]any{
			{"id": "maison", "label": "Profil maison", "include": []string{"noyau/**"}},
		},
	}, nil)
	var options struct {
		Profiles []struct{ ID string } `json:"profiles"`
	}
	h.json(http.MethodGet, "/api/plagiat/options", nil, &options)
	trouve := false
	for _, profil := range options.Profiles {
		if profil.ID == "maison" {
			trouve = true
		}
	}
	if !trouve {
		t.Fatalf("le profil déclaré doit paraître dans les options : %+v", options.Profiles)
	}
}

// Un sigle qui désignerait deux cours rendrait la question sans réponse : le
// refus vient du domaine, et il nomme le sigle fautif.
func TestDesReglesAmbiguesSontRefusees(t *testing.T) {
	h, _ := groupeAvecCopies(t)
	reponse, contenu := h.requete(http.MethodPut, "/api/orgs/acme/rules", map[string]any{
		"courses": []map[string]any{
			{"id": "prog3", "codes": []string{"5n6"}},
			{"id": "prog4", "codes": []string{"5n6"}},
		},
	})
	if reponse.StatusCode < 400 || !strings.Contains(string(contenu), "5n6") {
		t.Fatalf("l'ambiguïté doit être refusée en nommant le sigle : %s", contenu)
	}
}

// La portée décide de ce qui entre dans le corpus, et les équivalences de
// sigles décident de ce que « le même cours » veut dire.
func TestLaPorteeDAnneesRamasseLesAnciensSigles(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "modele-tp1", true)
	state.SeedCommit("acme/modele-tp1",
		map[string]string{"src/Inventaire.java": gabaritJava}, "main")

	// Cette session, sous le sigle courant ; et il y a deux ans, sous l'ancien.
	copies := map[string]string{
		"a26.5n6.01.tp1.alice-martin":  solutionJava,
		"a26.5n6.01.tp1.bruno-tanguay": autreJava,
		"h24.5m6.02.tp-1.ancien-eleve": solutionJava,
	}
	for nom, source := range copies {
		state.AddRepo("acme", nom, true)
		state.SeedCommit("acme/"+nom, map[string]string{
			"src/Inventaire.java": gabaritJava, "src/Solution.java": source,
		}, "main")
	}
	h := avantLeRegistre(t, state, classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "01",
		Students: []roster.Person{
			{FullName: "Alice Martin", Username: "alice"},
			{FullName: "Bruno Tanguay", Username: "bruno"},
		},
		Defaults: classroom.Defaults{Template: "acme/modele-tp1"},
	})

	// Sans équivalence déclarée, l'ancien sigle reste invisible.
	var etroit struct {
		Repos  int      `json:"repos"`
		Places []string `json:"places"`
	}
	h.json(http.MethodPost,
		"/api/classrooms/a26.5n6.01/assignments/tp1/plagiat/preview",
		map[string]any{"reach": "annees"}, &etroit)
	if etroit.Repos != 2 {
		t.Fatalf("sans règles : %d copies, places %v", etroit.Repos, etroit.Places)
	}

	h.json(http.MethodPut, "/api/orgs/acme/rules", map[string]any{
		"courses":     []map[string]any{{"id": "prog3", "codes": []string{"5n6", "5m6"}}},
		"assignments": []map[string]any{{"id": "tp1", "aliases": []string{"tp-1"}}},
	}, nil)

	var large struct {
		Repos  int      `json:"repos"`
		Places []string `json:"places"`
	}
	h.json(http.MethodPost,
		"/api/classrooms/a26.5n6.01/assignments/tp1/plagiat/preview",
		map[string]any{"reach": "annees"}, &large)
	if large.Repos != 3 {
		t.Fatalf("avec règles : %d copies, places %v", large.Repos, large.Places)
	}
	if len(large.Places) != 2 || large.Places[0] != "a26.5n6.01" {
		t.Fatalf("places, de la plus récente à la plus ancienne : %v", large.Places)
	}

	// Et l'analyse apparie bien la copie d'il y a deux ans.
	bilan := h.travail(http.MethodPost,
		"/api/classrooms/a26.5n6.01/assignments/tp1/plagiat",
		map[string]any{"reach": "annees", "baseline": true})
	resultat, _ := bilan["result"].(map[string]any)
	nom, _ := resultat["report"].(string)
	if nom == "" {
		t.Fatalf("analyse : %+v", bilan)
	}

	var rapport struct {
		Result struct {
			Matches []struct {
				Left        string  `json:"left"`
				Right       string  `json:"right"`
				LeftOrigin  string  `json:"left_origin"`
				RightOrigin string  `json:"right_origin"`
				Similarity  float64 `json:"similarity"`
			} `json:"matches"`
		} `json:"result"`
	}
	h.json(http.MethodGet, "/api/plagiat/reports/"+nom, nil, &rapport)

	trouve := false
	for _, match := range rapport.Result.Matches {
		if match.LeftOrigin != match.RightOrigin && match.Similarity > 0.9 {
			trouve = true
		}
	}
	if !trouve {
		t.Fatalf("la copie d'il y a deux ans n'a pas été appariée : %+v",
			rapport.Result.Matches)
	}
}

// ---------------------------------------------------- envoi anonymisé

// Deux fichiers sortent, et ils ne partent pas ensemble : l'archive, qui ne
// nomme personne, et la table, qui reste.
func TestUnEnvoiAnonymiseEcritLArchiveEtGardeLaTable(t *testing.T) {
	h, scope := groupeAvecCopies(t)
	destination := filepath.Join(t.TempDir(), "envoi.zip")

	bilan := h.travail(http.MethodPost,
		"/api/classrooms/"+scope+"/assignments/tp1/plagiat/export",
		map[string]any{"destination": destination, "profile": "tout"})
	if bilan["status"] != "terminé" {
		t.Fatalf("envoi : %+v", bilan)
	}
	resultat, _ := bilan["result"].(map[string]any)
	if copies, _ := resultat["copies"].(float64); copies != 3 {
		t.Fatalf("copies anonymisées : %+v", resultat["copies"])
	}

	archive, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("archive : %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("archive illisible : %v", err)
	}
	var tout strings.Builder
	for _, entry := range reader.File {
		tout.WriteString(entry.Name)
		fichier, err := entry.Open()
		if err != nil {
			t.Fatalf("lecture : %v", err)
		}
		io.Copy(&tout, fichier)
		fichier.Close()
	}
	for _, interdit := range []string{
		"Alice", "Martin", "alice-martin", "Bruno", "Tanguay", "Claire", "Otis",
	} {
		if strings.Contains(tout.String(), interdit) {
			t.Fatalf("« %s » se trouve dans l'archive", interdit)
		}
	}

	// La table est à côté, et elle porte les noms que l'archive tait.
	cheminCSV, _ := resultat["table_csv"].(string)
	if cheminCSV == "" || strings.Contains(cheminCSV, ".zip") {
		t.Fatalf("table : %q", cheminCSV)
	}
	table, err := os.ReadFile(cheminCSV)
	if err != nil {
		t.Fatalf("table : %v", err)
	}
	if !strings.Contains(string(table), "Alice Martin") {
		t.Fatalf("la table doit porter les noms : %s", table)
	}
	// Et le contrôle rend ce qui a résisté, pour être regardé avant l'envoi.
	if _, présent := resultat["residues"]; !présent {
		t.Fatalf("le contrôle doit être rendu : %+v", resultat)
	}
}

// Une archive reçue entre dans l'analyse par le même chemin que les dépôts.
func TestDesCopiesRecuesEntrentDansLAnalyse(t *testing.T) {
	h, scope := groupeAvecCopies(t)
	destination := filepath.Join(t.TempDir(), "recu.zip")
	h.travail(http.MethodPost,
		"/api/classrooms/"+scope+"/assignments/tp1/plagiat/export",
		map[string]any{"destination": destination, "profile": "tout"})

	// Les mêmes copies, reçues d'ailleurs : chacune doit retrouver son double.
	bilan := h.travail(http.MethodPost,
		"/api/classrooms/"+scope+"/assignments/tp1/plagiat",
		map[string]any{"profile": "tout", "baseline": true,
			"archives": []string{destination}})
	resultat, _ := bilan["result"].(map[string]any)
	if recues, _ := resultat["received"].(float64); recues != 3 {
		t.Fatalf("copies reçues : %+v (%+v)", resultat["received"], bilan)
	}

	nom, _ := resultat["report"].(string)
	var rapport struct {
		Result struct {
			Matches []struct {
				Left       string  `json:"left"`
				Right      string  `json:"right"`
				Similarity float64 `json:"similarity"`
			} `json:"matches"`
		} `json:"result"`
		Works []struct {
			ID     string `json:"id"`
			Label  string `json:"label"`
			Origin string `json:"origin"`
		} `json:"works"`
	}
	h.json(http.MethodGet, "/api/plagiat/reports/"+nom, nil, &rapport)

	croise := 0
	for _, match := range rapport.Result.Matches {
		venuDAilleurs := strings.HasPrefix(match.Left, "a26.") !=
			strings.HasPrefix(match.Right, "a26.")
		if venuDAilleurs && match.Similarity > 0.9 {
			croise++
		}
	}
	if croise == 0 {
		t.Fatalf("aucune copie reçue n'a été appariée : %+v", rapport.Result.Matches)
	}
	// Et les copies reçues ne nomment personne.
	for _, work := range rapport.Works {
		if strings.HasPrefix(work.ID, "a26.") {
			continue
		}
		if strings.Contains(work.Label, "Alice") || strings.Contains(work.Label, "Martin") {
			t.Fatalf("un nom a traversé : %+v", work)
		}
	}
}

// ------------------------------------------------- publication et catalogue

// Ce qui traverse le cloisonnement, ce sont des hachés. Un collègue mesure ses
// copies contre les nôtres sans en lire une ligne.
func TestPublierUnIndexNeFaitSortirNiCodeNiNom(t *testing.T) {
	h, scope := groupeAvecCopies(t)

	bilan := h.travail(http.MethodPost,
		"/api/classrooms/"+scope+"/assignments/tp1/plagiat",
		map[string]any{"profile": "tout", "baseline": true})
	resultat, _ := bilan["result"].(map[string]any)
	nom, _ := resultat["report"].(string)

	var publie struct {
		Assignment string `json:"assignment"`
		Copies     int    `json:"copies"`
		Prints     int    `json:"prints"`
		Origin     string `json:"origin"`
		Indexed    bool   `json:"indexed"`
		Table      string `json:"table"`
	}
	h.json(http.MethodPost, "/api/plagiat/reports/"+nom+"/publish",
		map[string]any{"index": true}, &publie)

	if publie.Copies != 3 || publie.Prints == 0 || !publie.Indexed {
		t.Fatalf("publication : %+v", publie)
	}
	if publie.Assignment != "a26.5n6.01.tp1" || publie.Origin != "a26.5n6.01" {
		t.Fatalf("index publié : %+v", publie)
	}

	// L'index est dans le dépôt de service, et il ne dit rien de personne.
	fichiers := h.State.Files("acme/"+exchange.IndexRepo, exchange.IndexBranch)
	contenu, present := fichiers[exchange.IndexPath("a26.5n6.01.tp1")]
	if !present {
		t.Fatalf("aucun index écrit : %v", fichiers)
	}
	for _, interdit := range []string{
		"Alice", "Martin", "alice-martin", "Bruno", "Claire", "class Solution",
		"plusGrand", "a26.5n6.01.tp1.",
	} {
		if strings.Contains(contenu, interdit) {
			t.Fatalf("« %s » se trouve dans l'index publié", interdit)
		}
	}
	// Mais les chemins restent : ils disent où un fragment se trouve.
	if !strings.Contains(contenu, "src/Solution.java") {
		t.Fatalf("les chemins doivent rester : %s", contenu)
	}
	// Et la table, qui relie les jetons aux personnes, est restée ici.
	if publie.Table == "" {
		t.Fatal("la table de correspondance doit être écrite sur ce poste")
	}
	table, err := os.ReadFile(publie.Table)
	if err != nil || !strings.Contains(string(table), "Alice Martin") {
		t.Fatalf("table : %v (%s)", err, table)
	}

	// Le catalogue annonce le travail, sans nommer d'étudiant.
	var catalogue struct {
		Teaching []struct {
			ID      string `json:"id"`
			Teacher string `json:"teacher"`
			Copies  int    `json:"copies"`
			Indexed bool   `json:"indexed"`
			Mine    bool   `json:"mine"`
		} `json:"teaching"`
		Teachers []string `json:"teachers"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/catalog", nil, &catalogue)
	if len(catalogue.Teaching) != 1 {
		t.Fatalf("catalogue : %+v", catalogue.Teaching)
	}
	ligne := catalogue.Teaching[0]
	if ligne.ID != "a26.5n6.01.tp1" || ligne.Copies != 3 || !ligne.Indexed || !ligne.Mine {
		t.Fatalf("ligne du catalogue : %+v", ligne)
	}
	if len(catalogue.Teachers) != 1 {
		t.Fatalf("enseignants : %v", catalogue.Teachers)
	}

	// La fiche du compte le montre aussi : c'est par là qu'on demande à
	// comparer.
	var profil struct {
		Given []struct {
			Assignment string `json:"assignment"`
		} `json:"given"`
	}
	h.json(http.MethodGet, "/api/users/"+h.State.Viewer, nil, &profil)
	if len(profil.Given) != 1 || profil.Given[0].Assignment != "tp1" {
		t.Fatalf("travaux annoncés sur la fiche : %+v", profil.Given)
	}
}

// Annoncer sans publier : le travail existe, mais il faudra une demande pour
// le comparer.
func TestOnPeutAnnoncerUnTravailSansEnPublierLIndex(t *testing.T) {
	h, scope := groupeAvecCopies(t)
	bilan := h.travail(http.MethodPost,
		"/api/classrooms/"+scope+"/assignments/tp1/plagiat",
		map[string]any{"profile": "tout"})
	resultat, _ := bilan["result"].(map[string]any)
	nom, _ := resultat["report"].(string)

	var annonce struct {
		Indexed bool `json:"indexed"`
	}
	h.json(http.MethodPost, "/api/plagiat/reports/"+nom+"/publish",
		map[string]any{"index": false}, &annonce)
	if annonce.Indexed {
		t.Fatal("l'index ne devait pas être publié")
	}
	if fichiers := h.State.Files("acme/"+exchange.IndexRepo, exchange.IndexBranch); len(fichiers) != 0 {
		t.Fatalf("rien ne devait être écrit : %v", fichiers)
	}

	var catalogue struct {
		Teaching []struct {
			Indexed bool `json:"indexed"`
		} `json:"teaching"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/catalog", nil, &catalogue)
	if len(catalogue.Teaching) != 1 || catalogue.Teaching[0].Indexed {
		t.Fatalf("catalogue : %+v", catalogue.Teaching)
	}
}

// -------------------------------------------------- demandes et levée du voile

// Le voile se lève sur un geste, jamais tout seul : une demande nommée, une
// décision, et l'envoi d'une seule copie — celle qui était demandée.
func TestUneDemandeSeDeposeSeTrancheEtProduitUnEnvoi(t *testing.T) {
	h, scope := groupeAvecCopies(t)

	// Le travail est analysé, son index publié, son annonce faite.
	bilan := h.travail(http.MethodPost,
		"/api/classrooms/"+scope+"/assignments/tp1/plagiat",
		map[string]any{"profile": "tout", "baseline": true})
	resultat, _ := bilan["result"].(map[string]any)
	nom, _ := resultat["report"].(string)

	var publie struct {
		Assignment string `json:"assignment"`
	}
	h.json(http.MethodPost, "/api/plagiat/reports/"+nom+"/publish",
		map[string]any{"index": true}, &publie)

	// Le jeton d'une des copies, tel qu'un collègue le verrait.
	fichiers := h.State.Files("acme/"+exchange.IndexRepo, exchange.IndexBranch)
	index, err := exchange.DecodePublished([]byte(fichiers[exchange.IndexPath(publie.Assignment)]))
	if err != nil {
		t.Fatalf("index publié : %v", err)
	}
	vise := index.Corpus.Works[0].ID

	// Une demande s'adresse à quelqu'un d'autre : ici, le compte connecté est
	// celui qui a publié, donc il ne peut pas se demander à lui-même.
	reponse, contenu := h.requete(http.MethodPost, "/api/orgs/acme/asks",
		map[string]any{"assignment": publie.Assignment, "token": vise})
	if reponse.StatusCode < 400 || !strings.Contains(string(contenu), "soi-même") {
		t.Fatalf("on ne se demande pas à soi-même : %s", contenu)
	}

	// La demande vient donc d'ailleurs : on l'écrit au registre comme le ferait
	// le collègue.
	depose := exchange.Ask{
		ID: "K7DM2X", From: "collegue", To: h.State.Viewer,
		Assignment: publie.Assignment, Token: vise, Similarity: 0.94,
		Note: "Une de mes copies lui ressemble beaucoup.",
	}
	if err := h.Serveur.RegistryApply("acme", registry.AskFor(depose)); err != nil {
		t.Fatalf("dépôt de la demande : %v", err)
	}

	var demandes struct {
		Received []struct {
			ID         string  `json:"id"`
			From       string  `json:"from"`
			Assignment string  `json:"assignment"`
			Token      string  `json:"token"`
			Similarity float64 `json:"similarity"`
			State      string  `json:"state"`
		} `json:"received"`
		Waiting int `json:"waiting"`
	}
	h.json(http.MethodGet, "/api/orgs/acme/asks", nil, &demandes)
	if demandes.Waiting != 1 || len(demandes.Received) != 1 {
		t.Fatalf("demandes reçues : %+v", demandes)
	}
	if demandes.Received[0].Token != vise || demandes.Received[0].Similarity != 0.94 {
		t.Fatalf("demande : %+v", demandes.Received[0])
	}

	// Accorder prépare l'envoi de cette seule copie, sous le même jeton.
	destination := filepath.Join(t.TempDir(), "accorde.zip")
	var accorde struct {
		State  string `json:"state"`
		Copies int    `json:"copies"`
		Path   string `json:"path"`
		Table  string `json:"table_csv"`
	}
	h.json(http.MethodPost, "/api/orgs/acme/asks/K7DM2X/grant",
		map[string]any{"destination": destination}, &accorde)
	if accorde.State != exchange.AskGranted || accorde.Copies != 1 {
		t.Fatalf("levée du voile : %+v", accorde)
	}

	archive, err := os.ReadFile(accorde.Path)
	if err != nil {
		t.Fatalf("archive : %v", err)
	}
	recu, err := anonymize.Import(archive)
	if err != nil {
		t.Fatalf("relecture : %v", err)
	}
	if len(recu.Copies) != 1 || recu.Copies[0].Work != vise {
		t.Fatalf("une seule copie, sous le jeton demandé : %+v", recu.Copies)
	}
	if strings.Contains(string(archive), "Martin") ||
		strings.Contains(string(archive), "Tanguay") {
		t.Fatal("un nom se trouve dans l'envoi")
	}

	// La demande est tranchée, et ne se tranche pas deux fois.
	reponse, contenu = h.requete(http.MethodPost, "/api/orgs/acme/asks/K7DM2X/grant",
		map[string]any{"destination": destination})
	if reponse.StatusCode < 400 || !strings.Contains(string(contenu), "accordée") {
		t.Fatalf("une demande tranchée ne se retranche pas : %s", contenu)
	}
}

// Une demande adressée à quelqu'un d'autre ne se tranche pas par mégarde.
func TestOnNeTranchePasLaDemandeDUnAutre(t *testing.T) {
	h, _ := groupeAvecCopies(t)
	autre := exchange.Ask{
		ID: "ZZ99ZZ", From: "collegue", To: "quelquun-dautre",
		Assignment: "a26.5n6.02.tp1", Token: "K7DM2X",
	}
	if err := h.Serveur.RegistryApply("acme", registry.AskFor(autre)); err != nil {
		t.Fatalf("dépôt : %v", err)
	}
	reponse, contenu := h.requete(http.MethodPost, "/api/orgs/acme/asks/ZZ99ZZ/deny",
		map[string]any{"reason": "non"})
	if reponse.StatusCode < 400 || !strings.Contains(string(contenu), "s'adresse à") {
		t.Fatalf("la demande d'un autre doit être refusée : %s", contenu)
	}
}
