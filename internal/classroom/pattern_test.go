package classroom_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
)

func gabarit(t *testing.T, source string) classroom.Pattern {
	t.Helper()
	compile, err := classroom.ParsePattern(source)
	if err != nil {
		t.Fatalf("gabarit %q : %v", source, err)
	}
	return compile
}

func TestGabaritRefuseCeQuIlNeSaitPasLire(t *testing.T) {
	cas := map[string]string{
		"":                                    "vide",
		"projet-{assignment}":                 "{student}",
		"{student}-{student}":                 "{student}",
		"{assignment}-{assignment}-{student}": "{assignment}",
		"{assignment}{student}":               "séparer",
		"{cours}-{student}":                   "Champ inconnu",
	}
	for source, attendu := range cas {
		_, err := classroom.ParsePattern(source)
		if err == nil {
			t.Errorf("%q accepté", source)
			continue
		}
		if !strings.Contains(err.Error(), attendu) {
			t.Errorf("%q → %v, attendu un message parlant de %q", source, err, attendu)
		}
	}
}

func TestGabaritDecoupeAuPlusLong(t *testing.T) {
	compile := gabarit(t, "{assignment}-{student}")
	// Le travail est glouton : « travail-de-session » n'est pas coupé en deux.
	travail, etudiant, ok := compile.Match("a26-5n6-travail-de-session-jlpicard")
	if !ok || travail != "a26-5n6-travail-de-session" || etudiant != "jlpicard" {
		t.Fatalf("découpe : %q / %q (%v)", travail, etudiant, ok)
	}
}

func TestGabaritExactQuandLaPersonneEstConnue(t *testing.T) {
	compile := gabarit(t, "projet-{assignment}-{student}")
	// Sans rien savoir, la découpe la plus longue se trompe : le nom de la
	// personne contient un tiret.
	travail, etudiant, _ := compile.Match("projet-tp1-emilie-cote")
	if travail != "tp1-emilie" || etudiant != "cote" {
		t.Fatalf("découpe gloutonne : %q / %q", travail, etudiant)
	}
	// La liste du groupe lève l'ambiguïté.
	exact, ok := compile.MatchFor("projet-tp1-emilie-cote", "emilie-cote")
	if !ok || exact != "tp1" {
		t.Fatalf("découpe exacte : %q (%v)", exact, ok)
	}
	if _, ok := compile.MatchFor("projet-tp1-emilie-cote", "jlpicard"); ok {
		t.Fatal("une autre personne ne devrait pas correspondre")
	}
}

func TestGabaritEclaireLesNomsLesUnsParLesAutres(t *testing.T) {
	compile := gabarit(t, "projet-{assignment}-{student}")
	decoupes := compile.Resolve([]string{
		"projet-tp1-jlpicard",    // sans ambiguïté : tp1 / jlpicard
		"projet-tp1-emilie-cote", // deux lectures possibles
		"projet-tp2-jlpicard",    // sans ambiguïté : tp2 / jlpicard
		"angular-tp1-jlpicard",   // hors gabarit
	})
	if len(decoupes) != 3 {
		t.Fatalf("découpes : %+v", decoupes)
	}
	trouve := map[string]string{}
	for _, decoupe := range decoupes {
		trouve[decoupe.Repo] = decoupe.Assignment + "/" + decoupe.Student
	}
	// « tp1 » se reconnaît grâce au dépôt voisin, malgré le tiret du nom.
	if trouve["projet-tp1-emilie-cote"] != "tp1/emilie-cote" {
		t.Fatalf("découpes : %v", trouve)
	}
}

func TestGabaritSansTravail(t *testing.T) {
	compile := gabarit(t, "kickmyb-{student}")
	_, etudiant, ok := compile.Match("kickmyb-equipe-3")
	if !ok || etudiant != "equipe-3" {
		t.Fatalf("découpe : %q (%v)", etudiant, ok)
	}
	if _, _, ok := compile.Match("angular-equipe-3"); ok {
		t.Fatal("un dépôt hors gabarit ne devrait pas correspondre")
	}
}

func TestGroupeAdopteParGabarit(t *testing.T) {
	cours := classroom.Classroom{
		Org: "acme", Name: "Projets", LegacyPattern: "projet-{assignment}-{student}",
		Students: personnes("Émilie Côté", "emilie-cote", "Jean-Luc Picard", "jlpicard"),
		Defaults: classroom.DefaultsFrom(config.Default()),
	}
	valide, err := cours.Validate()
	if err != nil {
		t.Fatalf("validation : %v", err)
	}
	if !valide.Legacy() {
		t.Fatal("un groupe adopté par gabarit reste à migrer")
	}

	inventaire := depots(
		"projet-tp1-emilie-cote", "projet-tp1-jlpicard",
		"projet-tp2-emilie-cote", "projet-tp2-visiteur",
		"angular-tp1-emilie-cote", // hors gabarit
	)
	travaux := valide.Assignments(inventaire)
	if len(travaux) != 2 {
		t.Fatalf("travaux : %v", noms(travaux))
	}
	for _, travail := range travaux {
		switch travail.Name {
		case "tp1":
			if travail.Repos != 2 || travail.Students != 2 {
				t.Fatalf("tp1 : %+v", travail)
			}
		case "tp2":
			// « visiteur » n'est pas de la liste : son dépôt compte à part.
			if travail.Repos != 2 || travail.Students != 1 || travail.Others != 1 {
				t.Fatalf("tp2 : %+v", travail)
			}
		default:
			t.Fatalf("travail inattendu : %q", travail.Name)
		}
	}

	student, inscrit := valide.StudentOf("projet-tp1-emilie-cote")
	if !inscrit || student.Username != "emilie-cote" {
		t.Fatalf("étudiant : %+v (%v)", student, inscrit)
	}
	if depots := valide.Repos("tp1", inventaire); len(depots) != 2 {
		t.Fatalf("dépôts de tp1 : %+v", depots)
	}
	if servis := valide.Served("tp1", inventaire); len(servis) != 2 {
		t.Fatalf("servis : %+v", servis)
	}
}
