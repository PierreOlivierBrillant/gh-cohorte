package groups_test

import (
	"reflect"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
)

func TestDetectPrefixesLesPlusGeneraux(t *testing.T) {
	noms := []string{
		"tp1-emilie-cote", "tp1-jlpicard", "tp1-aminata-d",
		"projet-final-emilie-cote", "projet-final-jlpicard",
		"notes-personnelles",
	}
	detectes := groups.Detect(noms, 2)
	attendu := []groups.Detected{
		{Prefix: "tp1", Count: 3},
		{Prefix: "projet-final", Count: 2},
	}
	if !reflect.DeepEqual(detectes, attendu) {
		t.Fatalf("Detect = %+v, attendu %+v", detectes, attendu)
	}
}

func TestDetectIgnoreLesGroupesTropPetits(t *testing.T) {
	if detectes := groups.Detect([]string{"tp1-seul", "autre-chose"}, 2); len(detectes) != 0 {
		t.Fatalf("Detect = %+v, aucun groupe attendu", detectes)
	}
}

func TestDetectEtendAuPlusLongPrefixeCommun(t *testing.T) {
	noms := []string{"projet-final-a", "projet-final-b", "projet-final-c"}
	detectes := groups.Detect(noms, 2)
	if len(detectes) != 1 || detectes[0].Prefix != "projet-final" || detectes[0].Count != 3 {
		t.Fatalf("Detect = %+v", detectes)
	}
}

func TestDetectDeuxTravauxDuMemePrefixe(t *testing.T) {
	noms := []string{"tp1-a", "tp1-b", "tp2-a", "tp2-b"}
	detectes := groups.Detect(noms, 2)
	if len(detectes) != 2 {
		t.Fatalf("Detect = %+v", detectes)
	}
	vus := map[string]int{}
	for _, item := range detectes {
		vus[item.Prefix] = item.Count
	}
	if vus["tp1"] != 2 || vus["tp2"] != 2 {
		t.Errorf("groupes = %+v", vus)
	}
}

func TestBuildGroupe(t *testing.T) {
	repos := []groups.RepoInfo{
		{Name: "tp1-jlpicard", Private: true, HTMLURL: "https://github.com/acme/tp1-jlpicard", PushedAt: "2026-08-19T10:11:12Z"},
		{Name: "tp1-emilie-cote", Private: false, HTMLURL: "https://github.com/acme/tp1-emilie-cote"},
		{Name: "autre-depot", Private: true},
		{Name: "tp1", Private: true},
	}
	groupe := groups.Build("TP1", repos)
	if groupe.Prefix != "tp1" || groupe.Len() != 2 {
		t.Fatalf("groupe = %+v", groupe)
	}
	if groupe.Repos[0].Name != "tp1-emilie-cote" {
		t.Errorf("les dépôts doivent être triés : %+v", groupe.Repos)
	}
	if groupe.Repos[0].Suffix != "emilie-cote" || groupe.Repos[0].Visibility() != "public" {
		t.Errorf("premier dépôt = %+v", groupe.Repos[0])
	}
	if groupe.Repos[1].PushedAt != "2026-08-19" {
		t.Errorf("date = %q, attendu 2026-08-19", groupe.Repos[1].PushedAt)
	}
	if _, index, ok := groupe.Find("JLPICARD"); !ok || index != 1 {
		t.Errorf("Find par suffixe = %d, %v", index, ok)
	}
	if _, _, ok := groupe.Find("tp1-inconnu"); ok {
		t.Error("Find doit échouer sur un nom absent")
	}
	if suffixes := groupe.Suffixes(); !suffixes["jlpicard"] || len(suffixes) != 2 {
		t.Errorf("suffixes = %+v", suffixes)
	}
}

func TestBuildPrefixeVide(t *testing.T) {
	if groupe := groups.Build("  ", []groups.RepoInfo{{Name: "tp1-a"}}); groupe.Len() != 0 {
		t.Errorf("un préfixe vide ne doit rien retenir : %+v", groupe)
	}
}

func TestParseSelection(t *testing.T) {
	noms := []string{"tp1-a", "tp1-b", "tp1-c", "tp1-d", "tp1-e"}
	resolve := func(token string) int {
		for index, nom := range noms {
			if nom == token {
				return index
			}
		}
		return -1
	}
	cas := map[string][]int{
		"":            {0, 1, 2, 3, 4},
		"tous":        {0, 1, 2, 3, 4},
		"ALL":         {0, 1, 2, 3, 4},
		"1,3":         {0, 2},
		"2-4":         {1, 2, 3},
		"4-2":         {1, 2, 3},
		"1, 3-4":      {0, 2, 3},
		"tp1-c":       {2},
		"1,3-4,tp1-e": {0, 2, 3, 4},
		"2 2 2":       {1},
	}
	for entree, attendu := range cas {
		obtenu, err := groups.ParseSelection(entree, len(noms), resolve)
		if err != nil {
			t.Errorf("ParseSelection(%q) : %v", entree, err)
			continue
		}
		if !reflect.DeepEqual(obtenu, attendu) {
			t.Errorf("ParseSelection(%q) = %v, attendu %v", entree, obtenu, attendu)
		}
	}
}

func TestParseSelectionInvalide(t *testing.T) {
	for _, entree := range []string{"0", "6", "1-9", "inconnu", "-3"} {
		if _, err := groups.ParseSelection(entree, 5, func(string) int { return -1 }); err == nil {
			t.Errorf("ParseSelection(%q) devrait échouer", entree)
		}
	}
}

func TestDetectListeVideOuUnique(t *testing.T) {
	if detectes := groups.Detect(nil, 2); len(detectes) != 0 {
		t.Errorf("Detect(nil) = %+v", detectes)
	}
	if detectes := groups.Detect([]string{"tp1-seul"}, 2); len(detectes) != 0 {
		t.Errorf("un seul dépôt ne fait pas un groupe : %+v", detectes)
	}
	if detectes := groups.Detect([]string{"sansseparateur", "autre"}, 2); len(detectes) != 0 {
		t.Errorf("des noms sans séparateur ne font pas un groupe : %+v", detectes)
	}
}

func TestParseSelectionListeVide(t *testing.T) {
	if indices, err := groups.ParseSelection("tous", 0, nil); err != nil || len(indices) != 0 {
		t.Errorf("ParseSelection = %v, %v", indices, err)
	}
}
