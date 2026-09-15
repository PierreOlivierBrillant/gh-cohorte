package plagiarism_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/exchange"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plagiarism"
)

// publieUnIndex monte un groupe, publie son index, et écrit la table à côté —
// exactement ce que fait une publication.
func publieUnIndex(t *testing.T) (*ghapi.Client, plagiarism.Request, exchange.Published, string) {
	t.Helper()
	state := fakegh.NewState()
	for _, nom := range []string{"emilie-cote", "jlpicard"} {
		depot(state, nom, map[string]string{
			"src/Solution.java": "// Travail d'Émilie Côté\n" + solution,
		})
	}
	serveur := client(t, state)
	requete := plagiarism.Request{
		Assignment: "a26.5n6.02.tp1", Org: "acme",
		Targets: []corpus.Target{
			nommee("emilie-cote", "Émilie Côté", "emilie-cote"),
			nommee("jlpicard", "Jean-Luc Picard", "jlpicard"),
		},
	}
	rapport, err := plagiarism.Run(serveur, requete, nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	publie, table, err := plagiarism.Publishable(rapport, "collegue", "groupe 02",
		anonymize.Options{Seed: 17})
	if err != nil {
		t.Fatalf("publication : %v", err)
	}

	// La table est écrite là où la publication l'écrit : c'est elle qu'on
	// retrouvera pour savoir quelle copie un jeton désigne.
	bilans := t.TempDir()
	dossier := filepath.Join(bilans, plagiarism.Dir)
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		t.Fatal(err)
	}
	contenu, err := os.ReadFile(ecrireTableDeTest(t, dossier, rapport.Basename(), table))
	if err != nil || len(contenu) == 0 {
		t.Fatalf("table : %v", err)
	}
	return serveur, requete, publie, bilans
}

func ecrireTableDeTest(t *testing.T, dossier, base string, table anonymize.Table) string {
	t.Helper()
	chemin := filepath.Join(dossier, base+plagiarism.IndexTableSuffix)
	payload, err := jsonDe(table)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chemin, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return chemin
}

// La copie repart sous le jeton qu'elle portait : le demandeur a mesuré
// celui-là, et une copie arrivant sous un autre serait, pour lui, une copie
// qu'il n'a jamais vue.
func TestAccorderUneDemandeEnvoieLaSeuleCopieVisee(t *testing.T) {
	serveur, requete, publie, bilans := publieUnIndex(t)
	vise := publie.Corpus.Works[0].ID

	trouvee, err := plagiarism.LocateToken(bilans, publie.Assignment, vise)
	if err != nil {
		t.Fatalf("recherche du jeton : %v", err)
	}
	if trouvee.Work != "emilie-cote" && trouvee.Work != "jlpicard" {
		t.Fatalf("copie désignée : %+v", trouvee)
	}

	demande := exchange.Ask{
		ID: "K7DM2X", From: "prof", To: "collegue",
		Assignment: publie.Assignment, Token: vise, Similarity: 0.94,
	}
	envoi, err := plagiarism.Grant(serveur, demande, trouvee, requete,
		anonymize.Options{Seed: 23})
	if err != nil {
		t.Fatalf("levée du voile : %v", err)
	}

	// Une seule copie, et c'est celle qui était demandée.
	if envoi.Copies != 1 || len(envoi.Bundle.Table.Tokens) != 1 {
		t.Fatalf("envoi : %d copie(s), %+v", envoi.Copies, envoi.Bundle.Table.Tokens)
	}
	if envoi.Bundle.Table.Tokens[0].Base != vise {
		t.Fatalf("le jeton doit être celui de l'index : %q contre %q",
			envoi.Bundle.Table.Tokens[0].Base, vise)
	}
	if envoi.Bundle.Table.Tokens[0].Work != trouvee.Work {
		t.Fatalf("copie envoyée : %+v", envoi.Bundle.Table.Tokens[0])
	}

	// Et rien de ce qui part ne nomme personne.
	recu, err := anonymize.Import(envoi.Bundle.Zip)
	if err != nil {
		t.Fatalf("relecture de l'envoi : %v", err)
	}
	if len(recu.Copies) != 1 || recu.Copies[0].Work != vise {
		t.Fatalf("copies reçues : %+v", recu.Copies)
	}
	if strings.Contains(string(envoi.Bundle.Zip), "Côté") ||
		strings.Contains(string(envoi.Bundle.Zip), "Picard") {
		t.Fatal("un nom se trouve dans l'envoi")
	}
}

// Sans la table, personne ne peut dire quelle copie un jeton désignait. L'outil
// le dit plutôt que d'envoyer la mauvaise.
func TestSansLaTableLeJetonNeDesignePlusRien(t *testing.T) {
	_, _, publie, _ := publieUnIndex(t)
	vide := t.TempDir()

	_, err := plagiarism.LocateToken(vide, publie.Assignment, publie.Corpus.Works[0].ID)
	if err == nil || !strings.Contains(err.Error(), "table") {
		t.Fatalf("l'absence de table doit être dite : %v", err)
	}

	_, _, publie2, bilans := publieUnIndex(t)
	if _, err := plagiarism.LocateToken(bilans, publie2.Assignment, "ZZZZZZ"); err == nil {
		t.Fatal("un jeton inconnu ne doit désigner aucune copie")
	}
	// Et une table d'un autre travail ne répond pas pour celui-ci.
	if _, err := plagiarism.LocateToken(bilans, "a26.5n6.99.tp9",
		publie2.Corpus.Works[0].ID); err == nil {
		t.Fatal("la table d'un autre travail ne doit pas répondre")
	}
}

// Une demande qui ne se tient pas est refusée à l'écriture.
func TestUneDemandeIncoherenteEstRefusee(t *testing.T) {
	cas := map[string]exchange.Ask{
		"sans identifiant":  {From: "prof", To: "collegue", Assignment: "a26.5n6.02.tp1", Token: "K7DM2X"},
		"travail incomplet": {ID: "AB12CD", From: "prof", To: "collegue", Assignment: "tp1", Token: "K7DM2X"},
		"sans destinataire": {ID: "AB12CD", From: "prof", Assignment: "a26.5n6.02.tp1", Token: "K7DM2X"},
		"à soi-même":        {ID: "AB12CD", From: "prof", To: "prof", Assignment: "a26.5n6.02.tp1", Token: "K7DM2X"},
		"sans copie visée":  {ID: "AB12CD", From: "prof", To: "collegue", Assignment: "a26.5n6.02.tp1"},
		"état inconnu":      {ID: "AB12CD", From: "prof", To: "collegue", Assignment: "a26.5n6.02.tp1", Token: "K7DM2X", State: "peut-être"},
	}
	for nom, demande := range cas {
		if _, err := (exchange.Asks{Asks: []exchange.Ask{demande}}).Validate(); err == nil {
			t.Fatalf("« %s » aurait dû être refusée", nom)
		}
	}
}

func jsonDe(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}
