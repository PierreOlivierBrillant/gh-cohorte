package rules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/rules"
)

func prog3() rules.Rules {
	return rules.Rules{
		Courses: []rules.Course{
			{ID: "prog3", Label: "Programmation 3", Codes: []string{"5N6", "5M6"}},
			{ID: "bd", Label: "Bases de données", Codes: []string{"4W6"}},
		},
		Assignments: []rules.Alias{{ID: "tp1", Aliases: []string{"tp-1", "travail1"}}},
	}
}

// C'est le piège que ce paquet existe pour absorber : un cours qui change de
// sigle, et trois ans de copies qui cessent d'être retrouvées.
func TestUnSigleChangeSansQueLeCoursChange(t *testing.T) {
	regles, err := prog3().Validate()
	if err != nil {
		t.Fatalf("règles refusées : %v", err)
	}
	for _, sigle := range []string{"5n6", "5M6", " 5N6 "} {
		famille := regles.SameCourse(sigle)
		if len(famille) != 2 || !contient(famille, "5n6") || !contient(famille, "5m6") {
			t.Fatalf("« %s » → %v", sigle, famille)
		}
	}
	if label := regles.CourseLabel("5m6"); label != "Programmation 3" {
		t.Fatalf("nom long du cours : %q", label)
	}
}

// Ne rien déclarer doit revenir à comparer un cours avec lui seul : c'est le
// comportement qu'on attend quand on n'a rien dit.
func TestUnSigleInconnuNAQueLuiMeme(t *testing.T) {
	var vides rules.Rules
	if famille := vides.SameCourse("7X9"); len(famille) != 1 || famille[0] != "7x9" {
		t.Fatalf("famille : %v", famille)
	}
	if famille := vides.SameAssignment("TP2"); len(famille) != 1 || famille[0] != "tp2" {
		t.Fatalf("famille : %v", famille)
	}
	if label := vides.CourseLabel("7x9"); label != "" {
		t.Fatalf("un cours non déclaré n'a pas de nom long : %q", label)
	}
}

func TestUnTravailSeRetrouveSousSesAutresNoms(t *testing.T) {
	regles, _ := prog3().Validate()
	for _, nom := range []string{"tp1", "tp-1", "travail1"} {
		famille := regles.SameAssignment(nom)
		if len(famille) != 3 || famille[0] != "tp1" {
			t.Fatalf("« %s » → %v", nom, famille)
		}
	}
}

// Un sigle qui appartiendrait à deux cours rendrait la question sans réponse :
// selon l'ordre de lecture, une analyse ramènerait les copies d'un programme ou
// d'un autre. Mieux vaut refuser à l'écriture.
func TestUnSigleNePeutPasDesignerDeuxCours(t *testing.T) {
	ambigues := rules.Rules{Courses: []rules.Course{
		{ID: "prog3", Codes: []string{"5n6"}},
		{ID: "prog4", Codes: []string{"5n6", "6n6"}},
	}}
	_, err := ambigues.Validate()
	if err == nil || !strings.Contains(err.Error(), "5n6") {
		t.Fatalf("l'ambiguïté doit être refusée en nommant le sigle : %v", err)
	}
}

func TestDesReglesIncompletesSontRefusees(t *testing.T) {
	cas := map[string]rules.Rules{
		"cours sans identifiant":  {Courses: []rules.Course{{Codes: []string{"5n6"}}}},
		"cours sans sigle":        {Courses: []rules.Course{{ID: "prog3"}}},
		"travail sans nom":        {Assignments: []rules.Alias{{Aliases: []string{"tp-1"}}}},
		"profil sans identifiant": {Profiles: []inspect.Profile{{Label: "Maison"}}},
		"part hors bornes":        {Defaults: rules.Defaults{Noise: 1.5}},
	}
	for nom, regles := range cas {
		if _, err := regles.Validate(); err == nil {
			t.Fatalf("« %s » aurait dû être refusé", nom)
		}
	}
}

// Un fichier passé en argument corrige le registre pour une analyse, sans le
// réécrire : à identifiant égal, c'est lui qui gagne.
func TestUnFichierSurchargeLesReglesDeLOrganisation(t *testing.T) {
	organisation, _ := prog3().Validate()
	surcharge, _ := rules.Rules{
		Courses: []rules.Course{
			{ID: "prog3", Label: "Programmation 3 (révisé)", Codes: []string{"5n6", "5m6", "5p6"}},
			{ID: "web", Codes: []string{"3k6"}},
		},
		Defaults: rules.Defaults{Noise: 0.5},
	}.Validate()

	fusion, err := organisation.Merge(surcharge).Validate()
	if err != nil {
		t.Fatalf("fusion refusée : %v", err)
	}
	if famille := fusion.SameCourse("5p6"); len(famille) != 3 {
		t.Fatalf("le cours remplacé : %v", famille)
	}
	// Ce que la surcharge ne nomme pas survit.
	if famille := fusion.SameCourse("4w6"); len(famille) != 1 {
		t.Fatalf("le cours non remplacé : %v", famille)
	}
	if famille := fusion.SameAssignment("tp-1"); len(famille) != 3 {
		t.Fatalf("les travaux survivent : %v", famille)
	}
	if famille := fusion.SameCourse("3k6"); len(famille) != 1 || famille[0] != "3k6" {
		t.Fatalf("le cours ajouté : %v", famille)
	}
	if fusion.Defaults.Noise != 0.5 {
		t.Fatalf("les bornes surchargées : %+v", fusion.Defaults)
	}
}

func TestLesReglesSEcriventEtSeRelisent(t *testing.T) {
	regles, _ := prog3().Validate()
	regles.Profiles = []inspect.Profile{
		{ID: "maison", Label: "Profil maison", Include: []string{"noyau/**"}},
	}
	contenu, err := rules.Encode(regles)
	if err != nil {
		t.Fatalf("écriture : %v", err)
	}
	relues, soucis := rules.Decode(contenu)
	if len(soucis) != 0 {
		t.Fatalf("relecture : %v", soucis)
	}
	if len(relues.SameCourse("5m6")) != 2 || len(relues.Profiles) != 1 {
		t.Fatalf("règles relues : %+v", relues)
	}
	// Les profils déclarés s'ajoutent à ceux que l'outil connaît.
	catalogue := inspect.Catalog(relues.Profiles)
	if _, err := inspect.FindProfile(catalogue, "maison"); err != nil {
		t.Fatalf("profil déclaré : %v", err)
	}
}

// Le fichier se modifie à la main sur github.com : une virgule de trop ne doit
// pas priver toute l'organisation de ses règles sans le dire.
func TestDesReglesIllisiblesSeSignalent(t *testing.T) {
	if _, soucis := rules.Decode([]byte("{ ceci n'est pas du JSON")); len(soucis) == 0 {
		t.Fatal("un fichier illisible doit se signaler")
	}
	_, soucis := rules.Decode([]byte(`{"version": 99}`))
	if len(soucis) == 0 || !strings.Contains(soucis[0], "upgrade") {
		t.Fatalf("une version future doit proposer la mise à jour : %v", soucis)
	}
}

func TestUnFichierDeReglesSeCharge(t *testing.T) {
	contenu, _ := rules.Encode(prog3())
	chemin := filepath.Join(t.TempDir(), "regles.json")
	if err := os.WriteFile(chemin, contenu, 0o600); err != nil {
		t.Fatalf("écriture : %v", err)
	}
	chargees, err := rules.Load(chemin)
	if err != nil {
		t.Fatalf("chargement : %v", err)
	}
	if len(chargees.SameCourse("5n6")) != 2 {
		t.Fatalf("règles chargées : %+v", chargees)
	}
	// Sans chemin, il n'y a rien à charger : ce n'est pas une erreur.
	if vides, err := rules.Load(""); err != nil || !vides.Empty() {
		t.Fatalf("chemin vide : %+v (%v)", vides, err)
	}
	if _, err := rules.Load(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("un fichier absent doit être refusé")
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
