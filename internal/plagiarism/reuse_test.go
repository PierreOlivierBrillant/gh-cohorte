package plagiarism_test

import (
	"sync"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plagiarism"
)

// compteur compte les archives réellement téléchargées : c'est la seule mesure
// qui dise si la reprise a servi à quelque chose.
type compteur struct {
	client corpus.Client
	mutex  sync.Mutex
	appels map[string]int
}

func (c *compteur) Archive(owner, repo, ref string) ([]byte, error) {
	c.mutex.Lock()
	c.appels[repo]++
	c.mutex.Unlock()
	return c.client.Archive(owner, repo, ref)
}

func (c *compteur) total() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	somme := 0
	for _, appels := range c.appels {
		somme += appels
	}
	return somme
}

func compte(client *ghapi.Client) *compteur {
	return &compteur{client: client, appels: map[string]int{}}
}

func cibleEnvoyeeLe(nom, envoi string) corpus.Target {
	cible := cible(nom)
	cible.PushedAt = envoi
	return cible
}

// Une session passée ne change plus : la retélécharger à chaque analyse
// coûterait le plus clair du temps pour recalculer des empreintes identiques.
func TestUneCopieQuiNAPasBougeNEstPasRetelechargee(t *testing.T) {
	state := fakegh.NewState()
	depot(state, "alice", map[string]string{"src/Solution.java": solution})
	depot(state, "bruno", map[string]string{"src/Solution.java": solution})
	compteurs := compte(client(t, state))

	premiere := plagiarism.Request{Targets: []corpus.Target{cible("alice"), cible("bruno")}}
	ancien, err := plagiarism.Run(compteurs, premiere, nil)
	if err != nil {
		t.Fatalf("première analyse : %v", err)
	}
	if compteurs.total() != 2 {
		t.Fatalf("archives téléchargées : %d", compteurs.total())
	}

	// Les deux copies datent d'avant l'analyse : rien n'a bougé depuis.
	avant := "2020-01-01T00:00:00Z"
	seconde := plagiarism.Request{Targets: []corpus.Target{
		cibleEnvoyeeLe("alice", avant), cibleEnvoyeeLe("bruno", avant),
	}}
	rapport, err := plagiarism.RunWith(compteurs, seconde,
		[]*plagiarism.Report{ancien}, nil)
	if err != nil {
		t.Fatalf("seconde analyse : %v", err)
	}
	if compteurs.total() != 2 {
		t.Fatalf("%d archives téléchargées : la reprise n'a pas servi", compteurs.total())
	}
	if rapport.Reused != 2 || rapport.Analyzed() != 2 {
		t.Fatalf("copies reprises : %d, analysées : %d", rapport.Reused, rapport.Analyzed())
	}
	// Et la mesure reste la même : reprendre un index ne doit rien changer au
	// résultat, sans quoi la reprise serait un piège.
	if match, trouve := rapport.Match("alice", "bruno"); !trouve || match.Similarity < 0.99 {
		t.Fatalf("mesure après reprise : %.2f (trouvée : %v)", match.Similarity, trouve)
	}
}

// L'inverse n'est pas vrai : une date absente ou postérieure ne prouve rien, et
// le doute doit faire retélécharger.
func TestUneCopieQuiAPuBougerEstRetelechargee(t *testing.T) {
	state := fakegh.NewState()
	depot(state, "alice", map[string]string{"src/Solution.java": solution})
	depot(state, "bruno", map[string]string{"src/Solution.java": solution})
	compteurs := compte(client(t, state))

	ancien, err := plagiarism.Run(compteurs,
		plagiarism.Request{Targets: []corpus.Target{cible("alice"), cible("bruno")}}, nil)
	if err != nil {
		t.Fatalf("première analyse : %v", err)
	}

	cas := map[string][]corpus.Target{
		"date absente": {cible("alice"), cible("bruno")},
		"envoi postérieur à l'analyse": {
			cibleEnvoyeeLe("alice", "2099-01-01T00:00:00Z"),
			cibleEnvoyeeLe("bruno", "2099-01-01T00:00:00Z"),
		},
	}
	for nom, cibles := range cas {
		avant := compteurs.total()
		rapport, err := plagiarism.RunWith(compteurs,
			plagiarism.Request{Targets: cibles}, []*plagiarism.Report{ancien}, nil)
		if err != nil {
			t.Fatalf("« %s » : %v", nom, err)
		}
		if rapport.Reused != 0 || compteurs.total() != avant+2 {
			t.Fatalf("« %s » : %d reprises, %d archives",
				nom, rapport.Reused, compteurs.total()-avant)
		}
	}
}

// Un rapport produit avec d'autres réglages n'est d'aucun secours : ses
// empreintes ont été calculées sur d'autres fichiers, et les mêler aux nôtres
// donnerait des mesures qui ne veulent rien dire.
func TestUnRapportDUnAutreReglageNEstPasRepris(t *testing.T) {
	state := fakegh.NewState()
	depot(state, "alice", map[string]string{"src/Solution.java": solution})
	depot(state, "bruno", map[string]string{"src/Solution.java": solution})
	compteurs := compte(client(t, state))

	ancien, err := plagiarism.Run(compteurs,
		plagiarism.Request{Targets: []corpus.Target{cible("alice"), cible("bruno")}}, nil)
	if err != nil {
		t.Fatalf("première analyse : %v", err)
	}

	avant := compteurs.total()
	autres := plagiarism.Request{
		Targets: []corpus.Target{
			cibleEnvoyeeLe("alice", "2020-01-01T00:00:00Z"),
			cibleEnvoyeeLe("bruno", "2020-01-01T00:00:00Z"),
		},
		Inspection: inspect.Settings{Languages: []string{"java"}},
		Kgram:      15,
	}
	rapport, err := plagiarism.RunWith(compteurs, autres, []*plagiarism.Report{ancien}, nil)
	if err != nil {
		t.Fatalf("analyse : %v", err)
	}
	if rapport.Reused != 0 || compteurs.total() != avant+2 {
		t.Fatalf("%d reprises pour un autre réglage", rapport.Reused)
	}
}
