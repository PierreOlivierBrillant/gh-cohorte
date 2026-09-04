package classroom_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
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

func TestPlanDeRenommageRefuseSansNomComplet(t *testing.T) {
	inventaire := depots("a26.5n6.01.tp1.emilie-cote")
	cours := groupe("a26", "5n6", "01", cohorte)
	avant, _ := cours.Find("emilie-cote")

	_, err := classroom.PlanRenameStudent(cours, avant,
		roster.Person{Username: "emilie-cote"}, inventaire)
	if err == nil || !strings.Contains(err.Error(), "emilie-cote") {
		t.Fatalf("nom complet manquant attendu : %v", err)
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
