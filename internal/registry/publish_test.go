package registry_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// Le cas ordinaire : un poste qui a des années de noms, un registre vide.
func TestPublierUnRegistreVideVerseTout(t *testing.T) {
	plan := registry.Plan(registry.Empty(), []roster.Person{
		personne("Émilie Côté", "ecote"),
		personne("Jean-Luc Picard", "jlpicard"),
	})
	if len(plan.New) != 2 || plan.Known != 0 || len(plan.Renamed) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Empty() || plan.Count() != 2 {
		t.Fatalf("Count = %d, Empty = %v", plan.Count(), plan.Empty())
	}
	// Le slug de chaque nom monte avec lui.
	if len(plan.New[0].Slugs) != 1 || plan.New[0].Slugs[0] != "emilie-cote" {
		t.Fatalf("slugs = %v", plan.New[0].Slugs)
	}
}

// Publier deux fois de suite ne fait rien la seconde : la publication est un
// aperçu de la différence, pas un versement aveugle.
func TestPublierDeuxFoisNeChangeRien(t *testing.T) {
	gens := []roster.Person{personne("Émilie Côté", "ecote")}
	set, _, err := appliquer(t, registry.Empty(), registry.Plan(registry.Empty(), gens).Apply(false))
	if err != nil {
		t.Fatal(err)
	}
	second := registry.Plan(set, gens)
	if !second.Empty() || second.Known != 1 {
		t.Fatalf("plan = %+v", second)
	}
}

// Le registre a pu être corrigé par quelqu'un d'autre. Le désaccord se montre ;
// par défaut, c'est le registre qui garde son nom.
func TestUnDesaccordSeMontreEtLeRegistreGardeSonNom(t *testing.T) {
	set, _, _ := appliquer(t, registry.Empty(),
		registry.Learn(personne("Émilie Côté", "ecote")))

	plan := registry.Plan(set, []roster.Person{personne("Emilie Cote", "ecote")})
	if len(plan.Renamed) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	desaccord := plan.Renamed[0]
	if desaccord.Registry != "Émilie Côté" || desaccord.Local != "Emilie Cote" {
		t.Fatalf("désaccord = %+v", desaccord)
	}

	garde, _, err := set.With(plan.Apply(false), "2026-09-06")
	if err != nil {
		t.Fatal(err)
	}
	if garde.Name("ecote") != "Émilie Côté" {
		t.Fatalf("nom conservé = %q", garde.Name("ecote"))
	}
	// Le nom écarté a tout de même nommé des dépôts : son slug monte.
	fiche, _ := garde.Find("ecote")
	if len(fiche.Slugs) == 0 {
		t.Fatal("aucun slug retenu")
	}
}

// On peut demander l'inverse : que le poste l'emporte.
func TestOnPeutFaireGagnerLePoste(t *testing.T) {
	set, _, _ := appliquer(t, registry.Empty(),
		registry.Learn(personne("Emilie Cote", "ecote")))

	plan := registry.Plan(set, []roster.Person{personne("Émilie Côté", "ecote")})
	repris, _, err := set.With(plan.Apply(true), "2026-09-06")
	if err != nil {
		t.Fatal(err)
	}
	if repris.Name("ecote") != "Émilie Côté" {
		t.Fatalf("nom repris = %q", repris.Name("ecote"))
	}
}

// C'est le dédoublonnage demandé : le même compte, nommé de deux façons dans
// deux groupes. La publication tranche — le premier rencontré —, mais elle le
// dit, et garde les deux slugs.
func TestUnCompteNommeDeuxFoisEstSignale(t *testing.T) {
	plan := registry.Plan(registry.Empty(), []roster.Person{
		personne("Émilie Côté", "ecote"),
		personne("Emlie Côté", "ECote"),
	})
	if len(plan.New) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Ambiguous) != 1 {
		t.Fatalf("ambiguïtés = %+v", plan.Ambiguous)
	}
	ambigu := plan.Ambiguous[0]
	if ambigu.Chosen != "Émilie Côté" || len(ambigu.Names) != 2 {
		t.Fatalf("ambiguïté = %+v", ambigu)
	}
	// Les deux slugs montent : aucun dépôt ne se détache, quel que soit le
	// nom retenu.
	if len(plan.New[0].Slugs) != 2 {
		t.Fatalf("slugs = %v", plan.New[0].Slugs)
	}
}

// Un compte dont personne ne connaît le nom ne se publie pas : une fiche sans
// nom n'apprendrait rien à qui la lit.
func TestUnCompteSansNomEstSignaleSansEtrePublie(t *testing.T) {
	plan := registry.Plan(registry.Empty(), []roster.Person{
		personne("", "inconnu"),
		personne("Émilie Côté", "ecote"),
	})
	if len(plan.New) != 1 || plan.New[0].Username != "ecote" {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Nameless) != 1 || plan.Nameless[0] != "inconnu" {
		t.Fatalf("sans nom = %v", plan.Nameless)
	}
}

// L'aperçu doit être stable : deux fois de suite, il dit la même chose.
func TestLApercuEstStable(t *testing.T) {
	gens := []roster.Person{
		personne("Jean-Luc Picard", "jlpicard"),
		personne("Émilie Côté", "ecote"),
		personne("Emlie Côté", "ecote"),
	}
	premier := registry.Plan(registry.Empty(), gens)
	second := registry.Plan(registry.Empty(), gens)
	if premier.Count() != second.Count() || len(premier.Ambiguous) != len(second.Ambiguous) {
		t.Fatalf("aperçus différents : %+v / %+v", premier, second)
	}
	for index := range premier.New {
		if premier.New[index].Username != second.New[index].Username {
			t.Fatalf("ordre instable : %+v / %+v", premier.New, second.New)
		}
	}
	// L'ordre donné est celui rendu : le premier compte reste le premier.
	if premier.New[0].Username != "jlpicard" {
		t.Fatalf("ordre = %+v", premier.New)
	}
}

// Le message du commit se lit : il dit ce qui a été publié.
func TestLeMessageDuCommitSeLit(t *testing.T) {
	plan := registry.Plan(registry.Empty(), []roster.Person{personne("Émilie Côté", "ecote")})
	message := plan.Apply(false).Reason
	if !strings.Contains(message, "1 étudiant") || strings.Contains(message, "1 étudiants") {
		t.Fatalf("message = %q", message)
	}
}
