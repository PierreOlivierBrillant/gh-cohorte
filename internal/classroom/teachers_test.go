package classroom_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
)

// enseignants est un registre de poche : il dit qui l'est, rien de plus.
type enseignants map[string]bool

func (e enseignants) Teaches(username string) bool {
	return e[strings.ToLower(username)]
}

// aCloisonner décrit un groupe et ses dépôts, sans équipe enseignante encore.
func aCloisonner() (classroom.Classroom, []groups.RepoInfo) {
	cours := classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "01",
		Students: []roster.Person{
			{FullName: "Émilie Côté", Username: "emilie-cote"},
			{FullName: "Jean-Luc Picard", Username: "jlpicard"},
		},
	}
	return cours, []groups.RepoInfo{
		{Name: "a26.5n6.01.tp1.emilie-cote"},
		{Name: "a26.5n6.01.tp1.jean-luc-picard"},
		{Name: "a26.5n6.01.tp2.emilie-cote"},
		// Le groupe voisin : il n'est pas du nôtre, et rien ne doit le toucher.
		{Name: "a26.5n6.02.tp1.aminata-diallo"},
	}
}

// Cloisonner un groupe neuf crée son équipe et lui donne ses dépôts — les
// siens seulement : le groupe voisin appartient à quelqu'un d'autre, et c'est
// tout l'objet du cloisonnement.
func TestCloisonnerCreeLEquipeEtNAccordeQueSesDepots(t *testing.T) {
	cours, depots := aCloisonner()
	plan, err := cours.PlanTeaching(nil, depots,
		[]string{"kjaneway"}, enseignants{"kjaneway": true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Team != "a26.5n6.01.enseignants" {
		t.Fatalf("équipe = %q", plan.Team)
	}
	if plan.Permission != classroom.TeacherPermission {
		t.Errorf("droit = %q", plan.Permission)
	}

	var creations, inscriptions, acces []string
	for _, etape := range plan.Steps {
		switch etape.Kind {
		case classroom.CreateTeam:
			creations = append(creations, etape.Target)
		case classroom.JoinTeam:
			inscriptions = append(inscriptions, etape.Target)
		case classroom.GrantRepo:
			acces = append(acces, etape.Target)
		default:
			t.Errorf("étape inattendue : %+v", etape)
		}
	}
	if len(creations) != 1 || creations[0] != "a26.5n6.01.enseignants" {
		t.Errorf("créations = %v", creations)
	}
	if strings.Join(inscriptions, ",") != "kjaneway" {
		t.Errorf("inscriptions = %v", inscriptions)
	}
	if strings.Join(acces, ",") !=
		"a26.5n6.01.tp1.emilie-cote,a26.5n6.01.tp1.jean-luc-picard,a26.5n6.01.tp2.emilie-cote" {
		t.Errorf("accès = %v", acces)
	}
	// Le plan dit ce qu'il ne peut pas garantir : un propriétaire voit tout.
	if plan.Notice == "" {
		t.Error("le plan doit dire ce que l'équipe ne ferme pas")
	}
}

// Le registre autorise, l'équipe donne. Quelqu'un que le registre ne déclare
// pas enseignant ne peut pas être inscrit — c'est le seul effet d'is_teacher.
func TestOnNInscritQueQuelquunDeclareEnseignant(t *testing.T) {
	cours, depots := aCloisonner()
	_, err := cours.PlanTeaching(nil, depots,
		[]string{"emilie-cote"}, enseignants{"kjaneway": true})
	if err == nil {
		t.Fatal("inscrire un étudiant à l'équipe enseignante doit être refusé")
	}
	if !strings.Contains(err.Error(), "emilie-cote") {
		t.Errorf("le refus doit nommer le compte : %v", err)
	}
}

// Un enseignant qu'on retire quitte l'équipe, et le retrait précède
// l'inscription : mieux vaut un accès de moins qu'un accès de trop si
// l'opération s'interrompt.
func TestLeRetraitPrecedeLInscription(t *testing.T) {
	cours, depots := aCloisonner()
	infos := []teams.Info{{
		Slug: "a26-5n6-01-enseignants", Name: "a26.5n6.01.enseignants",
		Members: []string{"partant"},
	}}
	plan, err := cours.PlanTeaching(infos, depots,
		[]string{"arrivant"}, enseignants{"arrivant": true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Slug != "a26-5n6-01-enseignants" {
		t.Errorf("slug = %q", plan.Slug)
	}
	var ordre []string
	for _, etape := range plan.Steps {
		if etape.Kind == classroom.JoinTeam || etape.Kind == classroom.LeaveTeam {
			ordre = append(ordre, etape.Kind+":"+etape.Target)
		}
		if etape.Kind == classroom.CreateTeam {
			t.Error("l'équipe existe : elle ne doit pas être recréée")
		}
	}
	if strings.Join(ordre, ",") != "retrait:partant,inscription:arrivant" {
		t.Fatalf("ordre = %v", ordre)
	}
}

// Rejouer le même cloisonnement n'inscrit ni ne retire personne : seuls les
// accès sont redonnés, et GitHub les accepte deux fois sans broncher.
func TestRejouerNeBougePasLaComposition(t *testing.T) {
	cours, depots := aCloisonner()
	infos := []teams.Info{{
		Slug: "a26-5n6-01-enseignants", Name: "a26.5n6.01.enseignants",
		Members: []string{"kjaneway"},
	}}
	plan, err := cours.PlanTeaching(infos, depots,
		[]string{"kjaneway"}, enseignants{"kjaneway": true})
	if err != nil {
		t.Fatal(err)
	}
	for _, etape := range plan.Steps {
		if etape.Kind != classroom.GrantRepo {
			t.Errorf("étape inattendue : %+v", etape)
		}
	}
}

// L'équipe enseignante n'est pas une équipe du groupe : on ne lui distribue
// rien, et elle ne doit pas se compter parmi les équipes d'étudiants.
func TestLEquipeEnseignanteNEstPasUneEquipeDuGroupe(t *testing.T) {
	cours, _ := aCloisonner()
	infos := []teams.Info{
		{Slug: "a26-5n6-01-eq1", Name: "a26.5n6.01.eq1", Members: []string{"emilie-cote"}},
		{Slug: "a26-5n6-01-enseignants", Name: "a26.5n6.01.enseignants",
			Members: []string{"kjaneway"}},
	}
	equipes := cours.Teams(infos)
	if len(equipes) != 1 || equipes[0].Short != "eq1" {
		t.Fatalf("équipes du groupe = %+v", equipes)
	}
	if _, trouvee := cours.TeacherTeam(infos); !trouvee {
		t.Error("l'équipe enseignante doit se retrouver par son propre chemin")
	}
	// Et son nom court est réservé : aucune équipe d'étudiants ne le prend.
	if _, err := teams.ShortName("enseignants"); err == nil {
		t.Error("« enseignants » doit être refusé comme nom d'équipe d'étudiants")
	}
}

// Un groupe sans équipe enseignante n'est pas cloisonné, et une équipe vide
// n'enferme rien : le dire évite de croire à une protection qui n'existe pas.
func TestUnGroupeSansEquipeNEstPasCloisonne(t *testing.T) {
	cours, depots := aCloisonner()
	etat := cours.TeachingOf(nil, depots, nil)
	if etat.Exists || etat.Cloistered() {
		t.Fatalf("état = %+v", etat)
	}
	if etat.Repos != 3 {
		t.Errorf("dépôts du groupe = %d", etat.Repos)
	}

	vide := []teams.Info{{Slug: "a26-5n6-01-enseignants", Name: "a26.5n6.01.enseignants"}}
	if cours.TeachingOf(vide, depots, nil).Cloistered() {
		t.Error("une équipe vide ne cloisonne rien")
	}

	garnie := []teams.Info{{
		Slug: "a26-5n6-01-enseignants", Name: "a26.5n6.01.enseignants",
		Members: []string{"kjaneway"},
	}}
	etat = cours.TeachingOf(garnie, depots, annuaire{"kjaneway": {FullName: "Kathryn Janeway", Username: "kjaneway"}})
	if !etat.Cloistered() {
		t.Fatal("une équipe garnie cloisonne")
	}
	if len(etat.Teachers) != 1 || etat.Teachers[0].FullName != "Kathryn Janeway" {
		t.Errorf("enseignants = %+v", etat.Teachers)
	}
}
