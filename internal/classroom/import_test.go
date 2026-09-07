package classroom_test

import (
	"strings"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

func inscrits() []roster.Entry {
	return []roster.Entry{
		{FullName: "Laurent Adam-Larocque", StudentID: "1680229"},
		{FullName: "Félix Bourassa", StudentID: "2143020"},
		{FullName: "Étienne Lyonnais", StudentID: "1983429"},
	}
}

func arrivee() classroom.Classroom {
	cours, err := classroom.AtScope("acme", "a26.5n6.1030",
		classroom.DefaultsFrom(config.Default()))
	if err != nil {
		panic(err)
	}
	return cours
}

// Ce que l'organisation porte hors nomenclature, et les travaux que ses
// préfixes dessinent.
func TestDepotsHorsNomenclatureEtLeursTravaux(t *testing.T) {
	inventaire := depots(
		"tp1-ladamlarocque", "tp1-felixb", "tp1-lyonnais",
		"projet-final-ladamlarocque", "projet-final-felixb",
		"a26.5n6.1030.tp2.laurent-adam-larocque", // déjà à sa place
	)
	dehors := classroom.ForeignOf(inventaire)
	if len(dehors.Repos) != 5 {
		t.Fatalf("dépôts hors nomenclature : %v", dehors.Repos)
	}
	for _, nom := range dehors.Repos {
		if strings.HasPrefix(nom, "a26.") {
			t.Fatalf("un dépôt déjà à sa place a été compté : %v", dehors.Repos)
		}
	}
	prefixes := map[string]int{}
	for _, travail := range dehors.Assignments {
		prefixes[travail.Prefix] = travail.Count
	}
	if prefixes["tp1"] != 3 || prefixes["projet-final"] != 2 {
		t.Fatalf("travaux devinés : %+v", dehors.Assignments)
	}
}

// Le cas complet : une liste d'Omnivox sans comptes, des dépôts nommés par
// GitHub Classroom, et un travail à faire entrer dans la nomenclature.
func TestImportRapprocheEtRenomme(t *testing.T) {
	inventaire := depots("tp1-ladamlarocque", "tp1-felixbourassa", "tp1-lyonnais")

	plan, err := classroom.PlanImport(arrivee(), classroom.ImportRequest{
		Prefix: "tp1", Name: "tp1", Entries: inscrits(), Guess: true}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(plan.Moves) != 3 || len(plan.Unmatched) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	cibles := map[string]string{}
	for _, ligne := range plan.Moves {
		cibles[ligne.Repo] = ligne.Target
	}
	if cibles["tp1-ladamlarocque"] != "a26.5n6.1030.tp1.laurent-adam-larocque" {
		t.Fatalf("cibles = %v", cibles)
	}
	if cibles["tp1-lyonnais"] != "a26.5n6.1030.tp1.etienne-lyonnais" {
		t.Fatalf("cibles = %v", cibles)
	}
	// Le compte vient du dépôt, le nom de la liste : c'est ce couple que le
	// registre attend.
	if len(plan.Students) != 3 {
		t.Fatalf("personnes inscrites = %+v", plan.Students)
	}
	for _, personne := range plan.Students {
		if personne.Username == "" || personne.FullName == "" {
			t.Fatalf("personne incomplète : %+v", personne)
		}
	}
}

// Le travail peut prendre un autre nom au passage : c'est le seul moment où
// corriger « tp1-final-v2 » ne coûte rien.
func TestImportPeutRenommerLeTravail(t *testing.T) {
	inventaire := depots("projet-final-lyonnais")
	plan, err := classroom.PlanImport(arrivee(), classroom.ImportRequest{
		Prefix: "projet-final", Name: "Projet final", Entries: inscrits(), Guess: true},
		inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(plan.Moves) != 1 ||
		plan.Moves[0].Target != "a26.5n6.1030.projet-final.etienne-lyonnais" {
		t.Fatalf("plan = %+v", plan.Moves)
	}
}

// Quand la liste porte les comptes, rien n'est deviné.
func TestUneListeQuiDitLesComptesNeSeDevinePas(t *testing.T) {
	inventaire := depots("tp1-xkcd42")
	dits := []roster.Entry{
		{FullName: "Étienne Lyonnais", StudentID: "1983429", Username: "xkcd42"},
		{FullName: "Félix Bourassa", StudentID: "2143020"},
	}
	plan, err := classroom.PlanImport(arrivee(), classroom.ImportRequest{
		Prefix: "tp1", Name: "tp1", Entries: dits, Guess: true}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(plan.Pairings) != 1 || plan.Pairings[0].Entry.FullName != "Étienne Lyonnais" {
		t.Fatalf("rapprochements = %+v", plan.Pairings)
	}
	if !strings.Contains(plan.Pairings[0].Reason, "liste") {
		t.Fatalf("raison = %q", plan.Pairings[0].Reason)
	}
	// Félix n'a pas de dépôt pour ce travail : le dire évite de croire la
	// liste entière importée.
	if len(plan.Absent) != 1 || plan.Absent[0] != "Félix Bourassa" {
		t.Fatalf("absents = %v", plan.Absent)
	}
}

// Un dépôt dont le compte ne mène à personne garde ce compte comme dernier
// niveau : on ne peut pas lui inventer un nom, et le laisser derrière ferait un
// travail à moitié importé. Le plan le nomme, pour qu'on puisse le corriger.
func TestUnDepotSansPersonneGardeSonCompte(t *testing.T) {
	inventaire := depots("tp1-lyonnais", "tp1-visiteur-anonyme-42")
	plan, err := classroom.PlanImport(arrivee(), classroom.ImportRequest{
		Prefix: "tp1", Name: "tp1", Entries: inscrits(), Guess: true}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(plan.Unmatched) != 1 || plan.Unmatched[0] != "visiteur-anonyme-42" {
		t.Fatalf("dépôts sans personne = %v", plan.Unmatched)
	}
	// Il figure quand même au renommage, sous le nom qu'il porte : le laisser
	// derrière ferait un travail à moitié importé.
	if len(plan.Moves) != 2 {
		t.Fatalf("renommages = %+v", plan.Moves)
	}
	for _, ligne := range plan.Moves {
		if ligne.Repo == "tp1-visiteur-anonyme-42" &&
			ligne.Target != "a26.5n6.1030.tp1.visiteur-anonyme-42" {
			t.Fatalf("le dépôt inconnu devait garder son dernier niveau : %+v", ligne)
		}
	}
}

// Un préfixe qui ne désigne aucun dépôt se refuse plutôt que de rendre un plan
// vide qu'on prendrait pour un succès.
func TestUnPrefixeSansDepotEstRefuse(t *testing.T) {
	if _, err := classroom.PlanImport(arrivee(), classroom.ImportRequest{
		Prefix: "absent", Name: "tp1", Entries: inscrits(), Guess: true},
		depots("tp1-lyonnais")); err == nil {
		t.Fatal("un préfixe sans dépôt doit être refusé")
	}
}

// Sans rapprochement, les comptes que la liste ne nomme pas restent sans
// réponse : c'est ce qu'on a décidé à l'écran, et redeviner le déferait.
func TestSansRapprochementRienNEstDevine(t *testing.T) {
	inventaire := depots("tp1-ladamlarocque", "tp1-lyonnais")
	corrige := []roster.Entry{
		{FullName: "Étienne Lyonnais", Username: "tp1-inexistant"},
	}
	plan, err := classroom.PlanImport(arrivee(), classroom.ImportRequest{
		Prefix: "tp1", Name: "tp1", Entries: corrige}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(plan.Unmatched) != 2 {
		t.Fatalf("des comptes ont été devinés : %+v", plan.Pairings)
	}
	// L'ordre des dépôts est tenu même quand rien n'est reconnu.
	if len(plan.Pairings) != 2 || plan.Pairings[0].Login != "ladamlarocque" {
		t.Fatalf("rapprochements = %+v", plan.Pairings)
	}
}

// La place d'arrivée se devine : le cours dans le nom du fichier, le groupe
// dans la colonne qui le porte, la session dans le premier commit du travail.
func TestLaPlaceDArriveeSeDevine(t *testing.T) {
	entrees := []roster.Entry{
		{FullName: "Étienne Lyonnais", Group: "1040"},
		{FullName: "Mei Chen", Group: "1040"},
	}
	premier := time.Date(2026, time.September, 14, 8, 0, 0, 0, time.UTC)
	place := classroom.GuessPlace("ListeEtudiants_cours4203N5EM_gr1040.csv", entrees, premier)
	if place.Scope() != "a26.3N5.1040" {
		t.Fatalf("place = %+v", place)
	}
}

// Les mois du printemps sont ceux de l'hiver : « p26 » se compose et se lit,
// mais aucune date ne le désigne, alors la devinette ne le propose jamais.
func TestLePrintempsNeSeDevinePas(t *testing.T) {
	saisons := map[time.Month]string{
		time.January: "h26", time.April: "h26", time.May: "h26",
		time.June: "e26", time.July: "e26",
		time.August: "a26", time.December: "a26",
	}
	for mois, attendu := range saisons {
		moment := time.Date(2026, mois, 15, 0, 0, 0, 0, time.UTC)
		if devinee := classroom.GuessPlace("", nil, moment); devinee.Session != attendu {
			t.Fatalf("%s → %q, attendu %q", mois, devinee.Session, attendu)
		}
	}
}

// Ce qui n'a pas pu être deviné reste vide : inventer une place ferait
// renommer des dépôts au mauvais endroit.
func TestCeQuiNeSeDevinePasResteVide(t *testing.T) {
	place := classroom.GuessPlace("cohorte.csv", nil, time.Time{})
	if place.Session != "" || place.Course != "" || place.Group != "" {
		t.Fatalf("place inventée : %+v", place)
	}
	if place.Scope() != "" {
		t.Fatalf("une place à trous a produit une portée : %q", place.Scope())
	}
}

// La recherche de la date s'arrête après quelques dépôts : si les premiers sont
// vides, personne n'a encore rien remis, et interroger tout le groupe ne
// changerait rien qu'au temps d'attente.
func TestLaDateSeChercheDansQuelquesDepotsSeulement(t *testing.T) {
	var noms []string
	inventaire := depots("tp1-a", "tp1-b", "tp1-c", "tp1-d", "tp1-e", "tp1-f", "tp1-g")
	debut := classroom.AssignmentStart("tp1", inventaire, func(depot string) (time.Time, error) {
		noms = append(noms, depot)
		return time.Time{}, nil
	})
	if !debut.IsZero() {
		t.Fatalf("une date est sortie de dépôts vides : %s", debut)
	}
	if len(noms) != 5 {
		t.Fatalf("%d dépôts interrogés : %v", len(noms), noms)
	}
}

// Le premier dépôt qui répond suffit : les suivants ne sont pas dérangés.
func TestLaDateSArreteAuPremierDepotQuiRepond(t *testing.T) {
	var demandes int
	attendu := time.Date(2027, time.February, 3, 10, 0, 0, 0, time.UTC)
	debut := classroom.AssignmentStart("tp1", depots("tp1-a", "tp1-b", "tp1-c"),
		func(string) (time.Time, error) {
			demandes++
			return attendu, nil
		})
	if !debut.Equal(attendu) || demandes != 1 {
		t.Fatalf("début = %s après %d demande(s)", debut, demandes)
	}
}
