package corpus_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/rules"
)

func inventaire(noms ...string) []groups.RepoInfo {
	liste := make([]groups.RepoInfo, 0, len(noms))
	for _, nom := range noms {
		liste = append(liste, groups.RepoInfo{Name: nom})
	}
	return liste
}

// college est un inventaire où le cours a changé de sigle en cours de route, et
// où le travail a changé de nom.
func college() []groups.RepoInfo {
	return inventaire(
		"a26.5n6.01.tp1.alice", "a26.5n6.01.tp1.bruno",
		"a26.5n6.02.tp1.claire", // même cours, autre groupe, même session
		"a26.5n6.01.tp2.alice",  // autre travail
		"a26.4w6.01.tp1.david",  // autre cours
		"h25.5m6.01.tp-1.emma",  // ancien sigle, ancien nom du travail
		"a24.5m6.03.tp1.farid",  // ancien sigle, autre session
		".cohorte",              // dépôt de service
		"notes-du-cours",        // hors nomenclature
	)
}

func equivalences() rules.Rules {
	regles, _ := rules.Rules{
		Courses:     []rules.Course{{ID: "prog3", Codes: []string{"5n6", "5m6"}}},
		Assignments: []rules.Alias{{ID: "tp1", Aliases: []string{"tp-1"}}},
	}.Validate()
	return regles
}

func ids(cibles []corpus.Target) []string {
	noms := make([]string, 0, len(cibles))
	for _, cible := range cibles {
		noms = append(noms, cible.ID)
	}
	return noms
}

func TestLaPorteeDeGroupeNeRetientQueCeGroupe(t *testing.T) {
	cibles, err := corpus.Select("acme", college(), "a26.5n6.01.tp1",
		corpus.ReachGroup, equivalences())
	if err != nil {
		t.Fatalf("sélection : %v", err)
	}
	if obtenu := strings.Join(ids(cibles), ","); obtenu != "a26.5n6.01.tp1.alice,a26.5n6.01.tp1.bruno" {
		t.Fatalf("copies retenues : %s", obtenu)
	}
	for _, cible := range cibles {
		if cible.Origin != "a26.5n6.01" || cible.Owner != "acme" {
			t.Fatalf("cible mal étiquetée : %+v", cible)
		}
	}
}

func TestLaPorteeDeCoursPrendLesAutresGroupesDeLaSession(t *testing.T) {
	cibles, err := corpus.Select("acme", college(), "a26.5n6.01.tp1",
		corpus.ReachCourse, equivalences())
	if err != nil {
		t.Fatalf("sélection : %v", err)
	}
	obtenu := strings.Join(ids(cibles), ",")
	if !strings.Contains(obtenu, "a26.5n6.02.tp1.claire") {
		t.Fatalf("l'autre groupe manque : %s", obtenu)
	}
	// Mais pas les autres sessions, ni les autres travaux, ni les autres cours.
	for _, absent := range []string{"h25.", "a24.", "tp2", "4w6"} {
		if strings.Contains(obtenu, absent) {
			t.Fatalf("« %s » n'aurait pas dû entrer : %s", absent, obtenu)
		}
	}
}

// C'est le piège qui justifie tout le paquet « rules » : le cours a changé de
// sigle, et sans équivalence les copies d'il y a deux ans sont invisibles.
func TestLaPorteeDAnneesSuitLeCoursQuiAChangeDeSigle(t *testing.T) {
	cibles, err := corpus.Select("acme", college(), "a26.5n6.01.tp1",
		corpus.ReachYears, equivalences())
	if err != nil {
		t.Fatalf("sélection : %v", err)
	}
	obtenu := strings.Join(ids(cibles), ",")
	for _, attendu := range []string{"h25.5m6.01.tp-1.emma", "a24.5m6.03.tp1.farid"} {
		if !strings.Contains(obtenu, attendu) {
			t.Fatalf("« %s » manque : %s", attendu, obtenu)
		}
	}
	if strings.Contains(obtenu, "4w6") || strings.Contains(obtenu, "tp2") {
		t.Fatalf("un autre cours ou un autre travail est entré : %s", obtenu)
	}
	if len(cibles) != 5 {
		t.Fatalf("copies retenues : %v", ids(cibles))
	}

	// Sans équivalence déclarée, l'ancien sigle reste invisible : c'est bien le
	// fichier de règles qui fait le travail, et non une devinette du code.
	sans, err := corpus.Select("acme", college(), "a26.5n6.01.tp1",
		corpus.ReachYears, rules.Rules{})
	if err != nil {
		t.Fatalf("sélection sans règles : %v", err)
	}
	if strings.Contains(strings.Join(ids(sans), ","), "5m6") {
		t.Fatalf("un sigle non déclaré est entré : %v", ids(sans))
	}
}

func TestLesPlacesSontRangeesDeLaPlusRecenteALaPlusAncienne(t *testing.T) {
	cibles, _ := corpus.Select("acme", college(), "a26.5n6.01.tp1",
		corpus.ReachYears, equivalences())
	places := corpus.Places(cibles)
	attendu := []string{"a26.5n6.01", "a26.5n6.02", "h25.5m6.01", "a24.5m6.03"}
	if strings.Join(places, ",") != strings.Join(attendu, ",") {
		t.Fatalf("places : %v", places)
	}
}

func TestUneSelectionTropMaigreEstRefuseeEnDisantLaPortee(t *testing.T) {
	seul := inventaire("a26.5n6.01.tp1.alice")
	_, err := corpus.Select("acme", seul, "a26.5n6.01.tp1", corpus.ReachGroup, rules.Rules{})
	if err == nil || !strings.Contains(err.Error(), "groupe") {
		t.Fatalf("le refus doit nommer la portée : %v", err)
	}
	_, err = corpus.Select("acme", college(), "tp1", corpus.ReachGroup, rules.Rules{})
	if err == nil || !strings.Contains(err.Error(), "a26.5n6.01.tp1") {
		t.Fatalf("un identifiant incomplet doit montrer la forme attendue : %v", err)
	}
}

func TestLesPorteesSeValident(t *testing.T) {
	if reach, err := corpus.ParseReach(""); err != nil || reach != corpus.ReachGroup {
		t.Fatalf("sans valeur, c'est le groupe : %q (%v)", reach, err)
	}
	if _, err := corpus.ParseReach("univers"); err == nil {
		t.Fatal("une portée inconnue doit être refusée")
	}
	for _, reach := range corpus.Reaches {
		if corpus.ReachLabels[reach] == "" {
			t.Fatalf("la portée « %s » n'est décrite nulle part", reach)
		}
		if lue, err := corpus.ParseReach(string(reach)); err != nil || lue != reach {
			t.Fatalf("« %s » ne se relit pas : %v", reach, err)
		}
	}
}
