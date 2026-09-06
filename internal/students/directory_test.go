package students_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/students"
)

// college décrit deux sessions et deux cours autour des mêmes personnes :
// Émilie traverse les trois groupes, Picard n'a fait que l'automne, et Aminata
// n'a que la session d'hiver.
func college() ([]classroom.Classroom, []groups.RepoInfo) {
	defauts := classroom.DefaultsFrom(config.Default())
	cours := []classroom.Classroom{
		{
			Org: "acme", Session: "a26", Course: "5n6", Group: "01", Defaults: defauts,
			Students: []roster.Person{
				{FullName: "Jean-Luc Picard", Username: "jlpicard"},
				{FullName: "Émilie Côté", Username: "ecote"},
			},
		},
		{
			Org: "acme", Session: "a26", Course: "4w6", Group: "01", Defaults: defauts,
			Students: []roster.Person{{FullName: "Émilie Côté", Username: "ecote"}},
		},
		{
			Org: "acme", Session: "h27", Course: "5n6", Group: "02", Defaults: defauts,
			Students: []roster.Person{
				{FullName: "Émilie Côté", Username: "ecote"},
				// Le nom complet manque ici ; l'automne le connaît.
				{Username: "aminata-d"},
			},
		},
	}
	inventaire := []groups.RepoInfo{
		{Name: "a26.5n6.01.tp1.jean-luc-picard", PushedAt: "2026-09-01T10:00:00Z"},
		{Name: "a26.5n6.01.tp1.emilie-cote", PushedAt: "2026-09-20T10:00:00Z"},
		{Name: "a26.4w6.01.projet.emilie-cote", PushedAt: "2026-11-05T10:00:00Z"},
		{Name: "h27.5n6.02.tp1.emilie-cote", PushedAt: "2027-02-10T10:00:00Z"},
		// Un dépôt dont le dernier niveau ne désigne personne de connu.
		{Name: "h27.5n6.02.tp1.inconnu-quelconque", PushedAt: "2027-02-11T10:00:00Z"},
	}
	return cours, inventaire
}

func annuaire() []students.Row {
	return students.Directory(college())
}

func TestAnnuaireFondUnePersonneVueParPlusieursGroupes(t *testing.T) {
	lignes := annuaire()
	if comptes(lignes) != "aminata-d,ecote,jlpicard" {
		t.Fatalf("annuaire : %s", comptes(lignes))
	}

	var emilie students.Row
	for _, ligne := range lignes {
		if ligne.Username == "ecote" {
			emilie = ligne
		}
	}
	if len(emilie.Enrollments) != 3 || len(emilie.Repos) != 3 {
		t.Fatalf("Émilie : %d inscription(s), %d dépôt(s)", len(emilie.Enrollments), len(emilie.Repos))
	}
	// Le dernier envoi est le plus récent de tous ses groupes.
	if emilie.PushedAt != "2027-02-10" {
		t.Fatalf("dernier envoi d'Émilie : %q", emilie.PushedAt)
	}
	// Les cours suivis vont de la session la plus récente à la plus ancienne :
	// c'est ici qu'on en décide, pour que les trois interfaces s'accordent.
	places := make([]string, 0, 3)
	for _, inscription := range emilie.Enrollments {
		places = append(places, inscription.Scope)
	}
	if strings.Join(places, ",") != "h27.5n6.02,a26.4w6.01,a26.5n6.01" {
		t.Fatalf("ordre des cours suivis : %v", places)
	}

	// Deux groupes ont chacun leur « tp1 » : leur place les distingue.
	vues := map[string]bool{}
	for _, depot := range emilie.Repos {
		vues[depot.Scope] = true
	}
	if !vues["a26.5n6.01"] || !vues["h27.5n6.02"] {
		t.Fatalf("places des dépôts : %v", vues)
	}
}

// Un nom complet connu d'un seul groupe vaut pour toute la ligne : c'est le
// compte GitHub qui identifie, et il ne change pas d'une session à l'autre.
func TestAnnuaireRecupereUnNomCompletConnuAilleurs(t *testing.T) {
	cours, inventaire := college()
	cours[0].Students = append(cours[0].Students,
		roster.Person{FullName: "Aminata Diallo", Username: "aminata-d"})
	for _, ligne := range students.Directory(cours, inventaire) {
		if ligne.Username == "aminata-d" && ligne.FullName != "Aminata Diallo" {
			t.Fatalf("nom complet d'Aminata : %q", ligne.FullName)
		}
	}
}

func TestFiltrerParSessionEtParCours(t *testing.T) {
	lignes := annuaire()
	cas := []struct {
		session, cours string
		attendus       string
	}{
		{session: "a26", attendus: "ecote,jlpicard"},
		{session: "h27", attendus: "aminata-d,ecote"},
		{cours: "4w6", attendus: "ecote"},
		{cours: "5n6", attendus: "aminata-d,ecote,jlpicard"},
		// Les deux critères portent sur la même inscription : Picard a bien
		// fait 5N6, mais pas à l'hiver.
		{session: "h27", cours: "5n6", attendus: "aminata-d,ecote"},
		{session: "h27", cours: "4w6", attendus: ""},
		// La casse d'un sigle ne compte pas : « 4W6 » s'écrit ainsi.
		{cours: "4W6", attendus: "ecote"},
	}
	for _, essai := range cas {
		filtre := students.Filter{Session: essai.session, Course: essai.cours}
		retenues := students.Apply(lignes, filtre, students.ByName, false)
		if comptes(retenues) != essai.attendus {
			t.Fatalf("session %q, cours %q : %s (attendu %s)",
				essai.session, essai.cours, comptes(retenues), essai.attendus)
		}
	}
}

// Être inscrit suffit : quelqu'un qui n'a rien remis a suivi le cours.
func TestUneInscriptionSansDepotCompte(t *testing.T) {
	cours, inventaire := college()
	cours[1].Students = append(cours[1].Students,
		roster.Person{FullName: "Jean-Luc Picard", Username: "jlpicard"})
	lignes := students.Directory(cours, inventaire)
	retenues := students.Apply(lignes,
		students.Filter{Session: "a26", Course: "4w6"}, students.ByName, false)
	if comptes(retenues) != "ecote,jlpicard" {
		t.Fatalf("inscrits à 4W6 : %s", comptes(retenues))
	}
}

func TestSessionsEtCoursDeLAnnuaire(t *testing.T) {
	lignes := annuaire()
	sessions := make([]string, 0, 2)
	for _, session := range students.SessionsIn(lignes) {
		sessions = append(sessions, session.Short+" ("+session.Name+")")
	}
	// De la plus récente à la plus ancienne, comme partout ailleurs.
	if strings.Join(sessions, ", ") != "h27 (Hiver 2027), a26 (Automne 2026)" {
		t.Fatalf("sessions : %v", sessions)
	}

	if tous := students.CoursesIn(lignes, ""); strings.Join(tous, ",") != "4w6,5n6" {
		t.Fatalf("cours : %v", tous)
	}
	// Une session choisie ne propose que les cours qu'elle a portés.
	if hiver := students.CoursesIn(lignes, "h27"); strings.Join(hiver, ",") != "5n6" {
		t.Fatalf("cours de l'hiver : %v", hiver)
	}
}

// Un dépôt dont le dernier niveau ne désigne personne n'ajoute pas quelqu'un à
// l'annuaire — mais il ne disparaît pas non plus.
func TestDepotsSansEtudiantConnuSontComptes(t *testing.T) {
	cours, inventaire := college()
	if orphelins := students.Unmatched(cours, inventaire); orphelins != 1 {
		t.Fatalf("dépôts non rattachés : %d", orphelins)
	}
}
