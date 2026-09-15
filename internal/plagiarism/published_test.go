package plagiarism_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/exchange"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plagiarism"
)

// publies est un accès aux index publiés, monté à la main : ce qui s'éprouve
// ici est le dépistage, pas la lecture d'un dépôt.
type publies map[string]exchange.Published

func (p publies) Read(assignment string) (exchange.Published, bool, error) {
	published, trouve := p[assignment]
	return published, trouve, nil
}

// indexDuCollegue produit l'index publié d'un groupe voisin.
func indexDuCollegue(t *testing.T) exchange.Published {
	t.Helper()
	state := fakegh.NewState()
	for _, nom := range []string{"son-eleve", "son-autre-eleve"} {
		depot(state, nom, map[string]string{"src/Solution.java": solution})
	}
	requete := plagiarism.Request{
		Assignment: "a26.5n6.02.tp1", Org: "acme",
		Targets: []corpus.Target{
			nommee("son-eleve", "Maude Tremblay", "mtremblay"),
			nommee("son-autre-eleve", "Karim Belkacem", "kbelkacem"),
		},
	}
	rapport, err := plagiarism.Run(client(t, state), requete, nil)
	if err != nil {
		t.Fatalf("analyse du collègue : %v", err)
	}
	publie, table, err := plagiarism.Publishable(rapport, "collegue", "groupe 02",
		anonymize.Options{Seed: 3})
	if err != nil {
		t.Fatalf("publication : %v", err)
	}
	if len(table.Tokens) != 2 {
		t.Fatalf("table du collègue : %+v", table.Tokens)
	}
	return publie
}

// C'est ce qui rend le dépistage possible sans percer le cloisonnement : des
// hachés traversent, pas du code.
func TestUnIndexPublieNePorteNiCodeNiNom(t *testing.T) {
	publie := indexDuCollegue(t)
	contenu, err := exchange.EncodePublished(publie)
	if err != nil {
		t.Fatalf("écriture : %v", err)
	}
	for _, interdit := range []string{
		"Maude", "Tremblay", "mtremblay", "Karim", "Belkacem",
		"son-eleve", "class Solution", "plusGrand",
	} {
		if strings.Contains(string(contenu), interdit) {
			t.Fatalf("« %s » se trouve dans l'index publié", interdit)
		}
	}
	// Les chemins de fichiers restent : ils disent où un fragment se trouve, ce
	// qui sert à juger, et « src/Solution.java » ne nomme personne.
	if !strings.Contains(string(contenu), "src/Solution.java") {
		t.Fatalf("les chemins doivent rester : %s", contenu)
	}
	if publie.Copies() != 2 || publie.Prints() == 0 {
		t.Fatalf("index : %d copies, %d empreintes", publie.Copies(), publie.Prints())
	}
	if publie.Teacher != "collegue" || publie.Origin != "groupe 02" {
		t.Fatalf("il faut savoir à qui s'adresser : %+v", publie.Teacher)
	}
}

// Le dépistage : on mesure contre les copies d'un collègue sans en lire une.
func TestUnePaireSeVoitSansLireLeCodeDuCollegue(t *testing.T) {
	publie := indexDuCollegue(t)

	state := fakegh.NewState()
	depot(state, "ma-copie", map[string]string{"src/Solution.java": solution})
	depot(state, "mon-autre-copie", map[string]string{"src/Travail.java": autre})

	rapport, err := plagiarism.RunFrom(plagiarism.Sources{
		Client:  client(t, state),
		Indexes: publies{publie.Assignment: publie},
	}, plagiarism.Request{
		Assignment: "a26.5n6.01.tp1",
		Targets:    []corpus.Target{cible("ma-copie"), cible("mon-autre-copie")},
		Indexes:    []string{publie.Assignment},
	}, nil)
	if err != nil {
		t.Fatalf("dépistage : %v", err)
	}
	if rapport.Screened != 2 {
		t.Fatalf("copies dépistées : %d (%+v)", rapport.Screened, rapport.Problems)
	}

	croise := 0
	for _, match := range rapport.Result.Matches {
		dAilleurs := strings.HasPrefix(match.Left, "m") != strings.HasPrefix(match.Right, "m")
		if dAilleurs && match.Similarity > 0.9 {
			croise++
			// Ce qui vient d'en face ne porte pas de nom : seule sa provenance
			// est connue.
			if strings.Contains(match.LeftName+match.RightName, "Tremblay") {
				t.Fatalf("un nom a traversé : %+v", match)
			}
		}
	}
	if croise == 0 {
		t.Fatalf("aucune paire croisée : %+v", rapport.Result.Matches)
	}

	// La provenance suit : c'est ce qui permet de dire « ces deux-là ne
	// viennent même pas du même groupe ».
	for _, work := range rapport.Index.Works {
		if strings.HasPrefix(work.ID, "m") {
			continue
		}
		if work.Origin != "groupe 02" {
			t.Fatalf("provenance d'une copie dépistée : %+v", work)
		}
	}
}

// Un index calculé autrement n'est pas mêlé à moitié : il est écarté entier,
// avec une raison qui dit quoi aligner.
func TestUnIndexAuxAutresBornesEstEcarteEnDisantPourquoi(t *testing.T) {
	publie := indexDuCollegue(t)

	state := fakegh.NewState()
	depot(state, "ma-copie", map[string]string{"src/Solution.java": solution})
	depot(state, "mon-autre-copie", map[string]string{"src/Travail.java": autre})

	rapport, err := plagiarism.RunFrom(plagiarism.Sources{
		Client:  client(t, state),
		Indexes: publies{publie.Assignment: publie},
	}, plagiarism.Request{
		Targets: []corpus.Target{cible("ma-copie"), cible("mon-autre-copie")},
		Indexes: []string{publie.Assignment},
		Kgram:   15, // des bornes différentes de celles du collègue
	}, nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	if rapport.Screened != 0 {
		t.Fatalf("un index incomparable a été mêlé : %d copies", rapport.Screened)
	}
	trouve := false
	for _, souci := range rapport.Problems {
		if souci.Reason == plagiarism.IndexMismatched &&
			strings.Contains(souci.Detail, "k-grammes") {
			trouve = true
		}
	}
	if !trouve {
		t.Fatalf("l'écart doit être nommé : %+v", rapport.Problems)
	}
	// Et le reste de l'analyse survit.
	if rapport.Analyzed() != 2 {
		t.Fatalf("copies analysées : %d", rapport.Analyzed())
	}
}

// Un index qu'on demande et que personne n'a publié appelle une demande, pas
// une panne : la différence est ce qu'on dit à l'enseignant.
func TestUnIndexAbsentSeDitSansArreterLAnalyse(t *testing.T) {
	state := fakegh.NewState()
	depot(state, "ma-copie", map[string]string{"src/Solution.java": solution})
	depot(state, "mon-autre-copie", map[string]string{"src/Travail.java": autre})

	rapport, err := plagiarism.RunFrom(plagiarism.Sources{
		Client: client(t, state), Indexes: publies{},
	}, plagiarism.Request{
		Targets: []corpus.Target{cible("ma-copie"), cible("mon-autre-copie")},
		Indexes: []string{"a26.5n6.02.tp1"},
	}, nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	if rapport.Analyzed() != 2 {
		t.Fatalf("copies analysées : %d", rapport.Analyzed())
	}
	trouve := false
	for _, souci := range rapport.Problems {
		if souci.Reason == plagiarism.NoIndex &&
			strings.Contains(souci.Detail, "demandez") {
			trouve = true
		}
	}
	if !trouve {
		t.Fatalf("l'absence doit appeler une demande : %+v", rapport.Problems)
	}
}

// On ne republie pas l'index d'un collègue : cela redistribuerait ce qu'on nous
// a confié, et sous nos jetons, ce qui brouillerait la piste jusqu'à son
// propriétaire.
func TestOnNeRepubliePasCeQuiVientDAilleurs(t *testing.T) {
	publie := indexDuCollegue(t)

	state := fakegh.NewState()
	depot(state, "ma-copie", map[string]string{"src/Solution.java": solution})
	depot(state, "mon-autre-copie", map[string]string{"src/Travail.java": autre})
	rapport, err := plagiarism.RunFrom(plagiarism.Sources{
		Client: client(t, state), Indexes: publies{publie.Assignment: publie},
	}, plagiarism.Request{
		Assignment: "a26.5n6.01.tp1",
		Targets:    []corpus.Target{cible("ma-copie"), cible("mon-autre-copie")},
		Indexes:    []string{publie.Assignment},
	}, nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}

	mien, _, err := plagiarism.Publishable(rapport, "prof", "groupe 01",
		anonymize.Options{Seed: 5})
	if err != nil {
		t.Fatalf("publication : %v", err)
	}
	// Seules mes deux copies, pas les quatre.
	if mien.Copies() != 2 {
		t.Fatalf("copies publiées : %d", mien.Copies())
	}
}
