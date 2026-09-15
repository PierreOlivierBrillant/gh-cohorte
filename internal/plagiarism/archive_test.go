package plagiarism_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plagiarism"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// nommee attache une identité à une cible : c'est elle que l'anonymisation
// efface.
func nommee(nom, complet, compte string) corpus.Target {
	cible := cible(nom)
	cible.Person = roster.Person{FullName: complet, Username: compte}
	return cible
}

// groupeDeLEnseignante monte le groupe de celle qui exporte.
func groupeDeLEnseignante(t *testing.T) (*ghapi.Client, plagiarism.Request) {
	t.Helper()
	state := fakegh.NewState()
	for _, nom := range []string{"emilie-cote", "jlpicard"} {
		state.AddRepo("acme", nom, true)
		state.SeedCommit("acme/"+nom, map[string]string{
			"src/Solution.java": "// Travail d'Émilie Côté\n" + solution,
			"README.md":         "# Rendu de Émilie Côté (@emilie-cote)\n",
		}, "main")
	}
	emilie := nommee("emilie-cote", "Émilie Côté", "emilie-cote")
	emilie.HandedIn = "2026-10-01T09:00:00Z"
	return client(t, state), plagiarism.Request{
		Assignment: "a26.5n6.01.tp1", Org: "acme",
		Targets: []corpus.Target{
			emilie, nommee("jlpicard", "Jean-Luc Picard", "jlpicard"),
		},
	}
}

// C'est le chemin pour un collègue d'une autre organisation : les copies
// voyagent, les noms restent.
func TestUnEnvoiAnonymiseTraverseEtSeCompare(t *testing.T) {
	serveur, requete := groupeDeLEnseignante(t)

	envoi, err := plagiarism.Export(serveur, requete, anonymize.Options{Seed: 11}, nil)
	if err != nil {
		t.Fatalf("export : %v", err)
	}
	if envoi.Copies != 2 || len(envoi.Problems) != 0 {
		t.Fatalf("envoi : %d copies, %+v", envoi.Copies, envoi.Problems)
	}
	if len(envoi.Bundle.Table.Tokens) != 2 {
		t.Fatalf("table : %+v", envoi.Bundle.Table.Tokens)
	}
	// La table porte ce que l'archive tait : le nom, le compte, la provenance,
	// et la date de remise quand on la connaît — c'est elle qui répond à
	// « lequel des deux a remis en premier ».
	for _, token := range envoi.Bundle.Table.Tokens {
		if token.FullName == "" || token.Username == "" {
			t.Fatalf("entrée incomplète : %+v", token)
		}
		if token.Work == "emilie-cote" && token.HandedIn != "2026-10-01T09:00:00Z" {
			t.Fatalf("date de remise : %+v", token)
		}
	}
	// Ce qui est parti ne nomme personne.
	if strings.Contains(string(envoi.Bundle.Zip), "Côté") ||
		strings.Contains(string(envoi.Bundle.Zip), "emilie") {
		t.Fatal("un nom se trouve dans l'archive")
	}

	chemin := filepath.Join(t.TempDir(), "travaux.zip")
	if err := os.WriteFile(chemin, envoi.Bundle.Zip, 0o600); err != nil {
		t.Fatalf("écriture : %v", err)
	}

	// Le collègue, dans une autre organisation, avec sa propre copie.
	autre := fakegh.NewState()
	depot(autre, "sa-copie", map[string]string{"src/Solution.java": solution})
	depot(autre, "son-autre-copie", map[string]string{"src/Travail.java": autre_()})
	chezLui := client(t, autre)

	rapport, err := plagiarism.Run(chezLui, plagiarism.Request{
		Assignment: "tp1-chez-lui",
		Targets:    []corpus.Target{cible("sa-copie"), cible("son-autre-copie")},
		Archives:   []string{chemin},
	}, nil)
	if err != nil {
		t.Fatalf("analyse chez le collègue : %v", err)
	}
	if rapport.Received != 2 {
		t.Fatalf("copies reçues : %d (%+v)", rapport.Received, rapport.Problems)
	}

	// Sa copie ressemble à l'une des nôtres, et il le voit sans savoir de qui.
	trouve := false
	for _, match := range rapport.Result.Matches {
		venuDAilleurs := strings.HasPrefix(match.Left, "sa-copie") !=
			strings.HasPrefix(match.Right, "sa-copie")
		if venuDAilleurs && match.Similarity > 0.9 {
			trouve = true
			if strings.Contains(match.LeftName+match.RightName, "Côté") {
				t.Fatalf("un nom a traversé : %+v", match)
			}
		}
	}
	if !trouve {
		t.Fatalf("la copie reçue n'a pas été appariée : %+v", rapport.Result.Matches)
	}

	// Et le rapport dit d'où elles viennent, sans dire qui elles sont.
	for _, work := range rapport.Index.Works {
		if strings.HasPrefix(work.ID, "sa-") || strings.HasPrefix(work.ID, "son-") {
			continue
		}
		if work.Origin == "" {
			t.Fatalf("une copie reçue sans provenance : %+v", work)
		}
	}
}

// Une archive illisible ne doit pas emporter le reste de l'analyse.
func TestUneArchiveIllisibleDevientUnMotif(t *testing.T) {
	state := fakegh.NewState()
	depot(state, "alice", map[string]string{"src/Solution.java": solution})
	depot(state, "bruno", map[string]string{"src/Solution.java": solution})

	chemin := filepath.Join(t.TempDir(), "casse.zip")
	os.WriteFile(chemin, []byte("ceci n'est pas une archive"), 0o600)

	rapport, err := plagiarism.Run(client(t, state), plagiarism.Request{
		Targets:  []corpus.Target{cible("alice"), cible("bruno")},
		Archives: []string{chemin},
	}, nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	if rapport.Analyzed() != 2 {
		t.Fatalf("le reste de l'analyse doit survivre : %d copies", rapport.Analyzed())
	}
	trouve := false
	for _, souci := range rapport.Problems {
		if souci.Reason == corpus.Broken {
			trouve = true
		}
	}
	if !trouve {
		t.Fatalf("l'archive illisible doit être un motif : %+v", rapport.Problems)
	}
}

// Une archive absente se dit avant le premier téléchargement, et non au milieu.
func TestUneArchiveAbsenteEstRefuseeDAvance(t *testing.T) {
	requete := plagiarism.Request{
		Targets:  []corpus.Target{cible("alice"), cible("bruno")},
		Archives: []string{filepath.Join(t.TempDir(), "absente.zip")},
	}
	if err := requete.Validate(); err == nil ||
		!strings.Contains(err.Error(), "absente.zip") {
		t.Fatalf("le refus doit nommer l'archive : %v", err)
	}
}

// Une seule archive suffit : comparer ses copies à celles d'un collègue est une
// demande aussi valable que comparer deux dépôts.
func TestUneArchiveSeuleCompteCommeDesCopies(t *testing.T) {
	requete := plagiarism.Request{
		Targets:  []corpus.Target{cible("alice")},
		Archives: []string{"quelque-chose.zip"},
	}
	// Le compte est bon ; c'est le fichier absent qui fait échouer, et il est
	// nommé.
	err := requete.Validate()
	if err == nil || strings.Contains(err.Error(), "au moins deux") {
		t.Fatalf("une cible et une archive font deux : %v", err)
	}
}

// autre_ est une solution différente, pour que tout ne se ressemble pas.
func autre_() string { return autre }
