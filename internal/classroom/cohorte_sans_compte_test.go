package classroom_test

import (
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// Le matricule désigne une personne aussi bien que son compte.
//
// La liste d'un groupe était réduite à ceux dont on connaissait déjà le compte
// GitHub, et cela se faisait à l'enregistrement, en silence : une cohorte
// importée du collège — qui n'en porte aucun — disparaissait entre le moment où
// on la déposait et celui où on la relisait.
func TestUneCohorteSansCompteSurvitALEnregistrement(t *testing.T) {
	cours := classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "1010",
		Students: []roster.Person{
			{FullName: "Laurent Adam-Larocque", StudentID: "1680229"},
			{FullName: "Patrick Rolleston-Chase", StudentID: "1256639"},
			{FullName: "Émilie Côté", Username: "ecote", StudentID: "2100123"},
		},
	}
	valide, err := cours.Validate()
	if err != nil {
		t.Fatalf("Validate : %v", err)
	}
	if len(valide.Students) != 3 {
		t.Fatalf("%d étudiant(s) retenus, 3 attendus : %+v",
			len(valide.Students), valide.Students)
	}
}

// Deux lignes d'un même matricule sont deux désignations d'une seule personne.
//
// Elles survivent toutes deux à l'enregistrement — « dedupe » range des
// désignations, non des gens —, et c'est « Identities » qui les réunit, comme
// il le fait depuis que le matricule est la clé d'un étudiant. Écrire la
// réunion ici aussi en ferait deux endroits libres de diverger.
func TestLeMatriculeReunitDeuxLignesEnUnePersonne(t *testing.T) {
	cours := classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "1010",
		Students: []roster.Person{
			{FullName: "Laurent Adam-Larocque", StudentID: "1680229"},
			{Username: "ladamlarocque", StudentID: "1680229"},
		},
	}
	valide, err := cours.Validate()
	if err != nil {
		t.Fatalf("Validate : %v", err)
	}
	if len(valide.Students) != 2 {
		t.Fatalf("%d ligne(s) retenues, 2 attendues : %+v",
			len(valide.Students), valide.Students)
	}
	identites := valide.Identities()
	if len(identites) != 1 {
		t.Fatalf("%d personne(s), une seule attendue : %+v", len(identites), identites)
	}
	if identites[0].FullName != "Laurent Adam-Larocque" {
		t.Errorf("le nom s'est perdu : %+v", identites[0])
	}
	if !identites[0].Has("ladamlarocque") {
		t.Errorf("le compte de la seconde ligne n'a pas rejoint la personne : %+v",
			identites[0])
	}
}

// Ni compte ni matricule : rien ne désigne cette personne, et deux homonymes y
// seraient confondus. Elle est écartée, comme avant.
func TestUnePersonneQueRienNeDesigneEstEcartee(t *testing.T) {
	cours := classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "1010",
		Students: []roster.Person{
			{FullName: "Sans rien"},
			{FullName: "Émilie Côté", Username: "ecote"},
		},
	}
	valide, err := cours.Validate()
	if err != nil {
		t.Fatalf("Validate : %v", err)
	}
	if len(valide.Students) != 1 || valide.Students[0].Username != "ecote" {
		t.Fatalf("liste retenue : %+v", valide.Students)
	}
}
