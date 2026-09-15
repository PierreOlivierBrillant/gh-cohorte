package users_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/users"
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

func annuaire() []users.Row {
	cours, inventaire := college()
	return users.Directory(cours, inventaire, nil, nil, nil)
}

func TestAnnuaireFondUnePersonneVueParPlusieursGroupes(t *testing.T) {
	lignes := annuaire()
	if comptes(lignes) != "aminata-d,ecote,jlpicard" {
		t.Fatalf("annuaire : %s", comptes(lignes))
	}

	var emilie users.Row
	for _, ligne := range lignes {
		if ligne.Username == "ecote" {
			emilie = ligne
		}
	}
	if len(emilie.Enrollments) != 3 || len(emilie.Repos) != 3 {
		t.Fatalf("Émilie : %d inscription(s), %d dépôt(s)", len(emilie.Enrollments), len(emilie.Repos))
	}
	// Le dernier envoi est le plus récent de tous ses groupes.
	if emilie.PushedAt != envoi("2027-02-10T10:00:00Z") {
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
	for _, ligne := range users.Directory(cours, inventaire, nil, nil, nil) {
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
		filtre := users.Filter{Session: essai.session, Course: essai.cours}
		retenues := users.Apply(lignes, filtre, users.ByName, false)
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
	lignes := users.Directory(cours, inventaire, nil, nil, nil)
	retenues := users.Apply(lignes,
		users.Filter{Session: "a26", Course: "4w6"}, users.ByName, false)
	if comptes(retenues) != "ecote,jlpicard" {
		t.Fatalf("inscrits à 4W6 : %s", comptes(retenues))
	}
}

func TestSessionsEtCoursDeLAnnuaire(t *testing.T) {
	lignes := annuaire()
	sessions := make([]string, 0, 2)
	for _, session := range users.SessionsIn(lignes) {
		sessions = append(sessions, session.Short+" ("+session.Name+")")
	}
	// De la plus récente à la plus ancienne, comme partout ailleurs.
	if strings.Join(sessions, ", ") != "h27 (Hiver 2027), a26 (Automne 2026)" {
		t.Fatalf("sessions : %v", sessions)
	}

	if tous := users.CoursesIn(lignes, ""); strings.Join(tous, ",") != "4w6,5n6" {
		t.Fatalf("cours : %v", tous)
	}
	// Une session choisie ne propose que les cours qu'elle a portés.
	if hiver := users.CoursesIn(lignes, "h27"); strings.Join(hiver, ",") != "5n6" {
		t.Fatalf("cours de l'hiver : %v", hiver)
	}
}

// Un dépôt dont le dernier niveau ne désigne personne n'ajoute pas quelqu'un à
// l'annuaire — mais il ne disparaît pas non plus.
func TestDepotsSansEtudiantConnuSontComptes(t *testing.T) {
	cours, inventaire := college()
	if orphelins := users.Unmatched(cours, inventaire, nil); orphelins != 1 {
		t.Fatalf("dépôts non rattachés : %d", orphelins)
	}
}

// Les comptes d'une personne sont ceux que tous ses groupes lui connaissent.
// Un groupe où elle n'en a déclaré qu'un ne doit pas faire oublier le second
// qu'un autre a retenu — sinon la moitié de son travail semble venir d'ailleurs.
func TestLAnnuaireReunitLesComptesDeTousLesGroupes(t *testing.T) {
	cours, inventaire := college()
	// Le matricule réunit ses lignes ; un seul groupe connaît son autre compte.
	for index := range cours {
		for rang := range cours[index].Students {
			if cours[index].Students[rang].Username == "ecote" {
				cours[index].Students[rang].StudentID = "2100123"
			}
		}
	}
	cours[0].Students[1].Also = []string{"emilie-perso"}

	for _, ligne := range users.Directory(cours, inventaire, nil, nil, nil) {
		if ligne.Username != "ecote" {
			continue
		}
		if len(ligne.Accounts) != 2 {
			t.Fatalf("comptes = %v", ligne.Accounts)
		}
		if ligne.StudentID != "2100123" {
			t.Errorf("matricule = %q", ligne.StudentID)
		}
		return
	}
	t.Fatal("Émilie est absente de l'annuaire")
}

// Un enseignant n'est sur aucune liste de classe : sans les équipes
// enseignantes et le registre, l'annuaire ne le verrait jamais — et « ne
// montrer que les enseignants » ne montrerait presque personne.
func TestLAnnuaireFaitEntrerLesEnseignants(t *testing.T) {
	cours, inventaire := college()
	infos := []teams.Info{{
		Slug: "a26-5n6-01-enseignants", Name: "a26.5n6.01.enseignants",
		Members: []string{"kjaneway"},
	}}
	lignes := users.Directory(cours, inventaire, nil, infos, connu())

	var janeway users.Row
	for _, ligne := range lignes {
		if ligne.Username == "kjaneway" {
			janeway = ligne
		}
	}
	if janeway.Username == "" {
		t.Fatalf("l'enseignante est absente : %s", comptes(lignes))
	}
	if !janeway.IsTeacher || janeway.Role() != users.AsTeacher {
		t.Errorf("rôle = %q", janeway.Role())
	}
	if janeway.FullName != "Kathryn Janeway" {
		t.Errorf("nom = %q", janeway.FullName)
	}
	// Le cours qu'elle donne figure comme tel, et ne lui prête aucun dépôt.
	if len(janeway.Enrollments) != 1 || !janeway.Enrollments[0].Teaching() {
		t.Fatalf("inscriptions = %+v", janeway.Enrollments)
	}
	if len(janeway.Repos) != 0 {
		t.Errorf("un enseignant ne rend rien : %+v", janeway.Repos)
	}
	// Les étudiants restent étudiants.
	for _, ligne := range lignes {
		if ligne.Username != "kjaneway" && ligne.IsTeacher {
			t.Errorf("@%s ne devrait pas être enseignant", ligne.Username)
		}
	}
}

// L'équipe prouve ce que le registre déclare : qui est dans l'équipe
// enseignante d'un groupe a l'accès à ses dépôts, et le montrer étudiant serait
// faux — même si personne ne l'a coopté.
func TestUnMembreDEquipeEnseignanteEstUnEnseignant(t *testing.T) {
	cours, inventaire := college()
	infos := []teams.Info{{
		Slug: "a26-5n6-01-enseignants", Name: "a26.5n6.01.enseignants",
		// Personne ne l'a déclaré au registre : seule l'équipe le dit.
		Members: []string{"jlpicard"},
	}}
	for _, ligne := range users.Directory(cours, inventaire, nil, infos, connu()) {
		if ligne.Username != "jlpicard" {
			continue
		}
		if !ligne.IsTeacher {
			t.Fatal("un membre de l'équipe enseignante est un enseignant")
		}
		// Il garde ses propres cours suivis, et gagne celui qu'il donne.
		suivis, donnes := 0, 0
		for _, inscription := range ligne.Enrollments {
			if inscription.Teaching() {
				donnes++
			} else {
				suivis++
			}
		}
		if suivis != 1 || donnes != 1 {
			t.Fatalf("suivis = %d, donnés = %d", suivis, donnes)
		}
		return
	}
	t.Fatal("Picard est absent de l'annuaire")
}

// Le filtre par rôle retient les uns ou les autres, jamais les deux.
func TestLeFiltreParRole(t *testing.T) {
	cours, inventaire := college()
	infos := []teams.Info{{
		Slug: "a26-5n6-01-enseignants", Name: "a26.5n6.01.enseignants",
		Members: []string{"kjaneway"},
	}}
	lignes := users.Directory(cours, inventaire, nil, infos, connu())

	profs := users.Apply(lignes, users.Filter{Role: users.OnlyTeachers}, users.ByName, false)
	if comptes(profs) != "kjaneway" {
		t.Fatalf("enseignants = %s", comptes(profs))
	}
	etudiants := users.Apply(lignes, users.Filter{Role: users.OnlyStudents}, users.ByName, false)
	if comptes(etudiants) != "aminata-d,ecote,jlpicard" {
		t.Fatalf("étudiants = %s", comptes(etudiants))
	}
	// Sans rôle demandé, tout le monde passe.
	tous := users.Apply(lignes, users.Filter{}, users.ByName, false)
	if len(tous) != len(lignes) {
		t.Errorf("%d sur %d", len(tous), len(lignes))
	}
}

// Le rôle se tape sans accent aussi bien qu'avec : « étudiant » se saisit
// rarement accentué au terminal.
func TestLeRoleSeLitAvecOuSansAccent(t *testing.T) {
	for _, saisie := range []string{"étudiant", "etudiant", "ÉTUDIANT"} {
		if role, err := users.ParseRole(saisie); err != nil || role != users.OnlyStudents {
			t.Errorf("« %s » = %q, %v", saisie, role, err)
		}
	}
	if role, err := users.ParseRole(""); err != nil || role != users.AnyRole {
		t.Errorf("vide = %q, %v", role, err)
	}
	if _, err := users.ParseRole("popularite"); err == nil {
		t.Error("un rôle inconnu doit être refusé")
	}
}
