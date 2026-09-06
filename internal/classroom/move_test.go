package classroom_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// ------------------------------------------------------- renommer une fiche

func TestRenommerUnEtudiantNeTouchePasAuxAutres(t *testing.T) {
	cours := groupe("a26", "5n6", "01", cohorte)

	modifie, err := cours.Rename("emilie-cote",
		roster.Person{FullName: "Émilie Côté-Tremblay", Username: "emilie-cote"})
	if err != nil {
		t.Fatalf("renommage refusé : %v", err)
	}
	if len(modifie.Students) != len(cohorte) {
		t.Fatalf("liste après renommage : %+v", modifie.Students)
	}
	corrigee, inscrite := modifie.Find("emilie-cote")
	if !inscrite || corrigee.FullName != "Émilie Côté-Tremblay" {
		t.Fatalf("fiche corrigée : %+v (%v)", corrigee, inscrite)
	}
	// La liste d'origine n'a pas bougé : le groupe est une valeur, pas une
	// référence partagée.
	ancienne, _ := cours.Find("emilie-cote")
	if ancienne.FullName != "Émilie Côté" {
		t.Fatalf("la liste d'origine a été modifiée : %+v", ancienne)
	}
	if voisin, _ := modifie.Find("jlpicard"); voisin.FullName != "Jean-Luc Picard" {
		t.Fatalf("un voisin a changé : %+v", voisin)
	}
}

func TestRenommerChangeLeCompte(t *testing.T) {
	cours := groupe("a26", "5n6", "01", cohorte)

	modifie, err := cours.Rename("jlpicard",
		roster.Person{FullName: "Jean-Luc Picard", Username: "jl-picard"})
	if err != nil {
		t.Fatalf("renommage refusé : %v", err)
	}
	if _, encore := modifie.Find("jlpicard"); encore {
		t.Fatal("l'ancien compte est resté dans la liste")
	}
	if _, inscrit := modifie.Find("jl-picard"); !inscrit {
		t.Fatalf("nouveau compte absent : %+v", modifie.Students)
	}
}

func TestRenommerRefuseUnCompteDejaPris(t *testing.T) {
	cours := groupe("a26", "5n6", "01", cohorte)

	_, err := cours.Rename("emilie-cote",
		roster.Person{FullName: "Émilie Côté", Username: "jlpicard"})
	if err == nil || !strings.Contains(err.Error(), "jlpicard") {
		t.Fatalf("erreur attendue sur le compte déjà pris : %v", err)
	}
}

func TestRenommerRefuseQuelquUnQuiNEstPasLa(t *testing.T) {
	cours := groupe("a26", "5n6", "01", cohorte)

	if _, err := cours.Rename("inconnu",
		roster.Person{FullName: "Personne", Username: "inconnu"}); err == nil {
		t.Fatal("renommer un absent aurait dû être refusé")
	}
}

// ------------------------------------------------- renommer aussi ses dépôts

func TestPlanDeRenommageSuitLeNomCorrige(t *testing.T) {
	inventaire := depots(
		"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp2.emilie-cote",
		"a26.5n6.01.tp1.jean-luc-picard",
	)
	cours := groupe("a26", "5n6", "01", cohorte)
	avant, _ := cours.Find("emilie-cote")

	lignes, err := classroom.PlanRenameStudent(cours, avant,
		roster.Person{FullName: "Émilie Côté-Tremblay", Username: "emilie-cote"}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(lignes) != 2 {
		t.Fatalf("dépôts à renommer : %+v", lignes)
	}
	cibles := map[string]string{}
	for _, ligne := range lignes {
		cibles[ligne.Repo] = ligne.Target
	}
	if cibles["a26.5n6.01.tp1.emilie-cote"] != "a26.5n6.01.tp1.emilie-cote-tremblay" ||
		cibles["a26.5n6.01.tp2.emilie-cote"] != "a26.5n6.01.tp2.emilie-cote-tremblay" {
		t.Fatalf("cibles composées : %v", cibles)
	}
}

// Changer le seul compte ne touche pas aux noms de dépôts : ils portent le nom
// de la personne, pas son compte.
func TestPlanDeRenommageVideQuandLeNomNeChangePas(t *testing.T) {
	inventaire := depots("a26.5n6.01.tp1.emilie-cote")
	cours := groupe("a26", "5n6", "01", cohorte)
	avant, _ := cours.Find("emilie-cote")

	lignes, err := classroom.PlanRenameStudent(cours, avant,
		roster.Person{FullName: "Émilie Côté", Username: "e-cote"}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(lignes) != 0 {
		t.Fatalf("rien n'aurait dû être renommé : %+v", lignes)
	}
}

func TestPlanDeRenommageRefuseUneCollision(t *testing.T) {
	inventaire := depots(
		"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.jean-luc-picard",
	)
	cours := groupe("a26", "5n6", "01", cohorte)
	avant, _ := cours.Find("emilie-cote")

	_, err := classroom.PlanRenameStudent(cours, avant,
		roster.Person{FullName: "Jean-Luc Picard", Username: "emilie-cote"}, inventaire)
	if err == nil || !strings.Contains(err.Error(), "a26.5n6.01.tp1.jean-luc-picard") {
		t.Fatalf("collision attendue : %v", err)
	}
}

// Sans nom complet, il n'y a rien à écrire au dernier niveau : le dépôt garde
// le sien plutôt que de refuser le renommage. C'est la même règle qui permet de
// déplacer un travail dont on ne connaît encore personne.
func TestPlanDeRenommageGardeLeFragmentSansNomComplet(t *testing.T) {
	inventaire := depots("a26.5n6.01.tp1.emilie-cote")
	cours := groupe("a26", "5n6", "01", cohorte)
	avant, _ := cours.Find("emilie-cote")

	lignes, err := classroom.PlanRenameStudent(cours, avant,
		roster.Person{Username: "emilie-cote"}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(lignes) != 0 {
		t.Fatalf("le dépôt aurait dû garder son nom : %+v", lignes)
	}
}

// Un groupe resté à l'ancienne nomenclature ne sait pas nommer un dépôt : le
// renommage se fait alors sans eux.
func TestPlanDeRenommageRefuseUnGroupeHerite(t *testing.T) {
	inventaire := depots("tp1-emilie-cote")
	cours := heritage("tp1", "emilie-cote")
	avant, _ := cours.Find("emilie-cote")

	if _, err := classroom.PlanRenameStudent(cours, avant,
		roster.Person{FullName: "Émilie Côté", Username: "emilie-cote"},
		inventaire); err == nil {
		t.Fatal("un groupe hérité aurait dû refuser le renommage de ses dépôts")
	}
}

// ------------------------------------------------------ déplacer un travail

// adopte déclare un groupe lu par gabarit, comme l'adoption en produit : des
// comptes GitHub, aucun nom complet.
func adopte(gabarit string, comptes ...string) classroom.Classroom {
	cours := classroom.Classroom{
		Org: "acme", LegacyPattern: gabarit,
		Students: classroom.StudentsOf(comptes),
		Defaults: classroom.DefaultsFrom(config.Default()),
	}
	valide, err := cours.Validate()
	if err != nil {
		panic(err)
	}
	return valide
}

// Le cas qui motive tout : un préfixe fourre-tout, aucun nom complet connu, et
// un travail à en sortir. Les dépôts arrivent à la bonne place en gardant leur
// dernier niveau.
func TestDeplacerUnTravailGardeLeFragmentInconnu(t *testing.T) {
	inventaire := depots(
		"travail-de-tp1-jlpicard", "travail-de-tp1-aminata-d",
		"travail-de-tp2-emilie-cote",
	)
	depart := adopte("travail-de-{assignment}-{student}",
		"jlpicard", "aminata-d", "emilie-cote")
	arrivee := groupe("a26", "5n6", "01", nil)

	lignes, err := classroom.PlanMoveAssignments(depart, arrivee,
		[]classroom.Relocation{{ID: "tp1"}}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	cibles := map[string]string{}
	for _, ligne := range lignes {
		cibles[ligne.Repo] = ligne.Target
	}
	if len(cibles) != 2 ||
		cibles["travail-de-tp1-jlpicard"] != "a26.5n6.01.tp1.jlpicard" ||
		cibles["travail-de-tp1-aminata-d"] != "a26.5n6.01.tp1.aminata-d" {
		t.Fatalf("cibles composées : %v", cibles)
	}
}

// Le travail peut prendre un nom au passage : c'est le seul moment où corriger
// « travail-de » ne coûte rien.
func TestDeplacerUnTravailLeRenomme(t *testing.T) {
	inventaire := depots("travail-de-jlpicard", "travail-de-emilie-cote")
	depart := adopte("travail-de-{student}", "jlpicard", "emilie-cote")
	arrivee := groupe("h27", "420", "02", nil)

	lignes, err := classroom.PlanMoveAssignments(depart, arrivee,
		[]classroom.Relocation{{ID: depart.Label(), Name: "Travail de session"}},
		inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(lignes) != 2 {
		t.Fatalf("dépôts à déplacer : %+v", lignes)
	}
	for _, ligne := range lignes {
		if !strings.HasPrefix(ligne.Target, "h27.420.02.travail-de-session.") {
			t.Fatalf("cible composée : %q", ligne.Target)
		}
	}
}

// Quand le nom complet est connu, le déplacement en profite : le dépôt arrive
// nommé comme la nomenclature le veut.
func TestDeplacerUnTravailEcritLeNomConnu(t *testing.T) {
	inventaire := depots("travail-de-tp1-jlpicard")
	depart := adopte("travail-de-{assignment}-{student}", "jlpicard")
	depart, err := depart.Rename("jlpicard",
		roster.Person{FullName: "Jean-Luc Picard", Username: "jlpicard"})
	if err != nil {
		t.Fatalf("fiche corrigée : %v", err)
	}
	arrivee := groupe("a26", "5n6", "01", nil)

	lignes, err := classroom.PlanMoveAssignments(depart, arrivee,
		[]classroom.Relocation{{ID: "tp1"}}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(lignes) != 1 || lignes[0].Target != "a26.5n6.01.tp1.jean-luc-picard" {
		t.Fatalf("cible composée : %+v", lignes)
	}
	if lignes[0].Username != "jlpicard" {
		t.Fatalf("étudiant rattaché : %+v", lignes[0])
	}
}

func TestDeplacerUnTravailRefuseUneCollision(t *testing.T) {
	inventaire := depots("travail-de-tp1-jlpicard", "a26.5n6.01.tp1.jlpicard")
	depart := adopte("travail-de-{assignment}-{student}", "jlpicard")
	arrivee := groupe("a26", "5n6", "01", nil)

	_, err := classroom.PlanMoveAssignments(depart, arrivee,
		[]classroom.Relocation{{ID: "tp1"}}, inventaire)
	if err == nil || !strings.Contains(err.Error(), "a26.5n6.01.tp1.jlpicard") {
		t.Fatalf("collision attendue : %v", err)
	}
}

func TestDeplacerUnTravailRefuseUneArriveeHeritee(t *testing.T) {
	inventaire := depots("travail-de-tp1-jlpicard")
	depart := adopte("travail-de-{assignment}-{student}", "jlpicard")

	_, err := classroom.PlanMoveAssignments(depart, heritage("vieux", "jlpicard"),
		[]classroom.Relocation{{ID: "tp1"}}, inventaire)
	if err == nil || !strings.Contains(err.Error(), "nomenclature") {
		t.Fatalf("arrivée héritée attendue : %v", err)
	}
}

// Les fiches suivent les dépôts qui partent ; celles dont il reste un dépôt au
// départ restent inscrites des deux côtés.
func TestLesFichesSuiventLeTravailDeplace(t *testing.T) {
	inventaire := depots(
		"travail-de-tp1-jlpicard", "travail-de-tp1-aminata-d",
		"travail-de-tp2-jlpicard",
	)
	depart := adopte("travail-de-{assignment}-{student}", "jlpicard", "aminata-d")
	arrivee := groupe("a26", "5n6", "01", nil)

	lignes, err := classroom.PlanMoveAssignments(depart, arrivee,
		[]classroom.Relocation{{ID: "tp1"}}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	suivent, quittent := classroom.Followers(depart, lignes, inventaire)
	if len(suivent) != 2 {
		t.Fatalf("personnes qui suivent : %+v", suivent)
	}
	if len(quittent) != 1 || quittent[0].Username != "aminata-d" {
		t.Fatalf("personnes qui quittent : %+v", quittent)
	}
}

// Un dépôt arrivé sous son compte GitHub reste rattaché à son étudiant : c'est
// ce qui permet de le renommer une fois le nom complet retrouvé.
func TestUnDepotNommeParLeCompteResteRattache(t *testing.T) {
	inventaire := depots("a26.5n6.01.tp1.jlpicard")
	cours := groupe("a26", "5n6", "01", classroom.StudentsOf([]string{"jlpicard"}))

	student, inscrit := cours.StudentOf("a26.5n6.01.tp1.jlpicard")
	if !inscrit || student.Username != "jlpicard" {
		t.Fatalf("étudiant rattaché : %+v (%v)", student, inscrit)
	}
	avant, _ := cours.Find("jlpicard")
	lignes, err := classroom.PlanRenameStudent(cours, avant,
		roster.Person{FullName: "Jean-Luc Picard", Username: "jlpicard"}, inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(lignes) != 1 || lignes[0].Target != "a26.5n6.01.tp1.jean-luc-picard" {
		t.Fatalf("cible composée : %+v", lignes)
	}
}

// ------------------------------------------------------ renommer un travail

// Le cas courant : un travail mal nommé, et rien d'autre à changer. Seul le
// niveau du travail bouge dans le nom de chaque dépôt.
func TestRenommerUnTravailNeTouchePasAuReste(t *testing.T) {
	inventaire := depots(
		"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.jlpicard",
		"a26.5n6.01.tp2.jlpicard",
	)
	cours := groupe("a26", "5n6", "01", cohorte)

	lignes, err := classroom.PlanRenameAssignment(cours, "a26.5n6.01.tp1",
		"projet-final", inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	cibles := map[string]string{}
	for _, ligne := range lignes {
		cibles[ligne.Repo] = ligne.Target
	}
	if len(cibles) != 2 ||
		cibles["a26.5n6.01.tp1.emilie-cote"] != "a26.5n6.01.projet-final.emilie-cote" ||
		cibles["a26.5n6.01.tp1.jlpicard"] != "a26.5n6.01.projet-final.jlpicard" {
		t.Fatalf("cibles composées : %v", cibles)
	}
}

// Le nom saisi passe par la nomenclature : « Projet final » ne peut pas entrer
// tel quel dans un nom de dépôt.
func TestRenommerUnTravailMetLeNomEnForme(t *testing.T) {
	inventaire := depots("a26.5n6.01.tp1.jlpicard")
	cours := groupe("a26", "5n6", "01", nil)

	lignes, err := classroom.PlanRenameAssignment(cours, "a26.5n6.01.tp1",
		"Projet final", inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(lignes) != 1 || lignes[0].Target != "a26.5n6.01.projet-final.jlpicard" {
		t.Fatalf("cible composée : %+v", lignes)
	}
}

// On renomme le travail, pas les personnes : un dépôt qui porte encore un compte
// GitHub le garde, même quand le nom complet est désormais connu. Le corriger a
// sa propre opération, et mêler les deux rendrait le renommage illisible.
func TestRenommerUnTravailGardeLeNiveauDeLEtudiant(t *testing.T) {
	inventaire := depots("a26.5n6.01.tp1.jlpicard")
	cours := groupe("a26", "5n6", "01",
		personnes("Jean-Luc Picard", "jlpicard"))

	lignes, err := classroom.PlanRenameAssignment(cours, "a26.5n6.01.tp1",
		"tp2", inventaire)
	if err != nil {
		t.Fatalf("plan refusé : %v", err)
	}
	if len(lignes) != 1 || lignes[0].Target != "a26.5n6.01.tp2.jlpicard" {
		t.Fatalf("cible composée : %+v", lignes)
	}
}

// Renommer vers le nom déjà porté ne fait rien : le dire vaut mieux que de
// lancer une opération vide.
func TestRenommerUnTravailRefuseLeMemeNom(t *testing.T) {
	inventaire := depots("a26.5n6.01.tp1.jlpicard")
	cours := groupe("a26", "5n6", "01", nil)

	_, err := classroom.PlanRenameAssignment(cours, "a26.5n6.01.tp1", "tp1", inventaire)
	if err == nil || !strings.Contains(err.Error(), "porte déjà ce nom") {
		t.Fatalf("refus attendu : %v", err)
	}
}

// Un nom déjà pris par un autre travail refuse l'opération entière : la moitié
// des dépôts renommés laisserait deux travaux là où il n'y en avait qu'un.
func TestRenommerUnTravailRefuseUneCollision(t *testing.T) {
	inventaire := depots("a26.5n6.01.tp1.jlpicard", "a26.5n6.01.tp2.jlpicard")
	cours := groupe("a26", "5n6", "01", nil)

	_, err := classroom.PlanRenameAssignment(cours, "a26.5n6.01.tp1", "tp2", inventaire)
	if err == nil || !strings.Contains(err.Error(), "existe déjà") {
		t.Fatalf("collision attendue : %v", err)
	}
}

// Un travail sans dépôt n'existe pas : il n'y a rien à renommer.
func TestRenommerUnTravailInconnu(t *testing.T) {
	inventaire := depots("a26.5n6.01.tp1.jlpicard")
	cours := groupe("a26", "5n6", "01", nil)

	_, err := classroom.PlanRenameAssignment(cours, "a26.5n6.01.tp9", "tp2", inventaire)
	if err == nil || !strings.Contains(err.Error(), "Aucun dépôt") {
		t.Fatalf("refus attendu : %v", err)
	}
}

// Un groupe resté à l'ancienne nomenclature ne sait pas nommer : le renommage
// est refusé avant toute écriture, et le refus dit par où passer.
func TestRenommerUnTravailRefuseUnGroupeHerite(t *testing.T) {
	inventaire := depots("vieux-tp1-jlpicard")
	cours := heritage("vieux-tp1", "jlpicard")

	_, err := classroom.PlanRenameAssignment(cours, "vieux-tp1", "tp2", inventaire)
	if err == nil || !strings.Contains(err.Error(), "nomenclature") {
		t.Fatalf("refus attendu : %v", err)
	}
}
