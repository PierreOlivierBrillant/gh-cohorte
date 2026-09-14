package classroom_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
)

// avecEquipes décrit un groupe de trois personnes, deux équipes, et deux
// travaux : « tp1 » individuel, « projet » en équipe.
func avecEquipes() (classroom.Classroom, []groups.RepoInfo, []teams.Team) {
	cours := classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "01",
		Students: []roster.Person{
			{FullName: "Émilie Côté", Username: "emilie-cote"},
			{FullName: "Jean-Luc Picard", Username: "jlpicard"},
			{FullName: "Aminata Diallo", Username: "aminata-d"},
		},
	}
	inventaire := []groups.RepoInfo{
		{Name: "a26.5n6.01.tp1.emilie-cote", PushedAt: "2026-09-01T10:00:00Z"},
		{Name: "a26.5n6.01.tp1.jean-luc-picard"},
		{Name: "a26.5n6.01.projet.eq1", PushedAt: "2026-09-05T10:00:00Z"},
		{Name: "a26.5n6.01.projet.eq2"},
	}
	equipes := cours.Teams([]teams.Info{
		{Slug: "a26-5n6-01-eq1", Name: "a26.5n6.01.eq1",
			Members: []string{"emilie-cote", "jlpicard"}},
		{Slug: "a26-5n6-01-eq2", Name: "a26.5n6.01.eq2", Members: []string{"aminata-d"}},
	})
	return cours, inventaire, equipes
}

func trouver(travaux []classroom.Assignment, nom string) (classroom.Assignment, bool) {
	for _, travail := range travaux {
		if travail.Name == nom {
			return travail, true
		}
	}
	return classroom.Assignment{}, false
}

// La nature d'un travail ne se déclare nulle part : elle se lit dans le dernier
// niveau du nom de ses dépôts, qui nomme une équipe du groupe ou un étudiant.
func TestLaNatureDUnTravailSeLitDansLeNomDesDepots(t *testing.T) {
	cours, inventaire, equipes := avecEquipes()
	travaux := cours.Assignments(inventaire, equipes)

	solo, trouve := trouver(travaux, "tp1")
	if !trouve || solo.Kind != classroom.Individual || solo.Teams != 0 {
		t.Fatalf("« tp1 » devrait être individuel : %+v", solo)
	}
	if solo.Students != 2 {
		t.Fatalf("« tp1 » devrait compter deux étudiants servis : %+v", solo)
	}

	equipe, trouve := trouver(travaux, "projet")
	if !trouve || equipe.Kind != classroom.TeamWork || !equipe.ForTeams() {
		t.Fatalf("« projet » devrait être un travail d'équipe : %+v", equipe)
	}
	if equipe.Teams != 2 || equipe.Others != 0 {
		t.Fatalf("les deux dépôts devraient être reconnus comme ceux d'équipes : %+v", equipe)
	}
}

// Sans les équipes, les mêmes dépôts ne se rattachent à personne : c'est bien
// GitHub qui dit ce qu'ils sont, pas un fichier local.
func TestSansLesEquipesUnTravailDEquipeResteInconnu(t *testing.T) {
	cours, inventaire, _ := avecEquipes()
	travaux := cours.Assignments(inventaire, nil)
	projet, trouve := trouver(travaux, "projet")
	if !trouve {
		t.Fatal("le travail devrait rester visible")
	}
	if projet.Kind != classroom.Individual || projet.Others != 2 {
		t.Fatalf("sans équipes, ses dépôts sont hors liste : %+v", projet)
	}
}

func TestServedTeamsEcarteCeQuiEstDejaDistribue(t *testing.T) {
	cours, inventaire, equipes := avecEquipes()
	servies := cours.ServedTeams("a26.5n6.01.projet", inventaire, equipes)
	if !servies["eq1"] || !servies["eq2"] {
		t.Fatalf("les deux équipes ont un dépôt : %v", servies)
	}
	// Un travail que personne n'a encore n'a servi aucune équipe.
	if len(cours.ServedTeams("a26.5n6.01.tp2", inventaire, equipes)) != 0 {
		t.Fatal("aucune équipe ne devrait être servie pour un travail sans dépôt")
	}
}

func TestTeamOfRattacheUnDepotASonEquipe(t *testing.T) {
	cours, _, equipes := avecEquipes()
	equipe, appartient := cours.TeamOf("a26.5n6.01.projet.eq1", equipes)
	if !appartient || equipe.Short != "eq1" {
		t.Fatalf("le dépôt devrait revenir à eq1 : %v %+v", appartient, equipe)
	}
	if _, appartient := cours.TeamOf("a26.5n6.01.tp1.emilie-cote", equipes); appartient {
		t.Fatal("un dépôt d'étudiant n'appartient à aucune équipe")
	}
}

// Une équipe qui change de groupe emporte ses dépôts et ceux de ses membres,
// et rien d'autre : ce qui appartient au reste du groupe y reste.
func TestPlanMoveTeamEmporteLEquipeEtSesMembres(t *testing.T) {
	cours, inventaire, equipes := avecEquipes()
	arrivee := classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "02",
	}
	membres := cours.TeamMovers(equipes[0])
	if len(membres) != 2 {
		t.Fatalf("eq1 emmène deux personnes : %+v", membres)
	}

	lignes, err := classroom.PlanMoveTeam(cours, arrivee, equipes[0], membres, inventaire)
	if err != nil {
		t.Fatalf("PlanMoveTeam : %v", err)
	}
	obtenu := map[string]string{}
	for _, ligne := range lignes {
		obtenu[ligne.Repo] = ligne.Target
	}
	attendu := map[string]string{
		// Le dépôt de l'équipe garde son nom court et change de place.
		"a26.5n6.01.projet.eq1": "a26.5n6.02.projet.eq1",
		// Ceux de ses membres suivent, sous le fragment qui les nomme.
		"a26.5n6.01.tp1.emilie-cote":     "a26.5n6.02.tp1.emilie-cote",
		"a26.5n6.01.tp1.jean-luc-picard": "a26.5n6.02.tp1.jean-luc-picard",
	}
	if len(obtenu) != len(attendu) {
		t.Fatalf("%d renommage(s) : %v", len(obtenu), obtenu)
	}
	for depot, cible := range attendu {
		if obtenu[depot] != cible {
			t.Errorf("« %s » → %q, attendu %q", depot, obtenu[depot], cible)
		}
	}
}

// Un nom qui existe déjà à l'arrivée refuse le plan entier, avant le premier
// renommage : une collision découverte à mi-chemin laisserait l'équipe à cheval
// sur deux groupes.
func TestPlanMoveTeamRefuseUneCollision(t *testing.T) {
	cours, inventaire, equipes := avecEquipes()
	arrivee := classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "02",
	}
	inventaire = append(inventaire,
		groups.RepoInfo{Name: "a26.5n6.02.projet.eq1"})
	_, err := classroom.PlanMoveTeam(cours, arrivee, equipes[0],
		cours.TeamMovers(equipes[0]), inventaire)
	if err == nil {
		t.Fatal("un nom déjà pris à l'arrivée doit être refusé")
	}
	if !strings.Contains(err.Error(), "a26.5n6.02.projet.eq1") {
		t.Errorf("le refus doit nommer le dépôt fautif : %v", err)
	}
}

// Les dépôts d'une équipe sont ceux que son nom termine, tous travaux
// confondus. Un dépôt d'un autre groupe, ou d'une autre équipe, n'est pas le
// sien — même si les deux équipes portent le même nom court.
func TestTeamReposNeRetientQueLesDepotsDeLEquipe(t *testing.T) {
	cours, inventaire, equipes := avecEquipes()
	inventaire = append(inventaire,
		groups.RepoInfo{Name: "a26.5n6.01.tp2.eq1"},
		groups.RepoInfo{Name: "a26.5n6.02.projet.eq1"},
	)
	sien := cours.TeamRepos(equipes[0], inventaire)
	if len(sien) != 2 || sien[0] != "a26.5n6.01.projet.eq1" || sien[1] != "a26.5n6.01.tp2.eq1" {
		t.Fatalf("dépôts de eq1 = %v", sien)
	}
	if reste := cours.TeamRepos(equipes[1], inventaire); len(reste) != 1 {
		t.Fatalf("dépôts de eq2 = %v", reste)
	}
}

func TestUnaffectesNommeCeuxQueAucuneEquipeNAccueille(t *testing.T) {
	cours, _, equipes := avecEquipes()
	if restants := cours.Unassigned(equipes); len(restants) != 0 {
		t.Fatalf("les trois étudiants ont une équipe : %v", restants)
	}
	cours.Students = append(cours.Students, roster.Person{
		FullName: "Nouvelle Venue", Username: "nouvelle",
	})
	restants := cours.Unassigned(equipes)
	if len(restants) != 1 || restants[0].Username != "nouvelle" {
		t.Fatalf("la nouvelle venue devrait être seule sans équipe : %v", restants)
	}
}

// Un membre que la liste du groupe ne connaît pas a l'accès sans être inscrit
// nulle part : le taire ferait croire l'équipe plus petite qu'elle n'est.
func TestDescribeSepareLesMembresConnusDesEtrangers(t *testing.T) {
	cours, _, _ := avecEquipes()
	equipes := cours.Teams([]teams.Info{{
		Slug: "a26-5n6-01-eq1", Name: "a26.5n6.01.eq1",
		Members: []string{"emilie-cote", "intrus"},
	}})
	fiches := cours.Describe(equipes)
	if len(fiches) != 1 {
		t.Fatalf("une fiche attendue, %d", len(fiches))
	}
	if len(fiches[0].People) != 1 || fiches[0].People[0].Username != "emilie-cote" {
		t.Fatalf("membres reconnus inattendus : %v", fiches[0].People)
	}
	if len(fiches[0].Strangers) != 1 || fiches[0].Strangers[0].Username != "intrus" {
		t.Fatalf("l'intrus devrait être signalé : %v", fiches[0].Strangers)
	}
}

// Une équipe d'un autre groupe ne se rattache pas à celui-ci : c'est la place
// inscrite dans son nom qui les distingue.
func TestUneEquipeDUnAutreGroupeNestPasRetenue(t *testing.T) {
	cours, _, _ := avecEquipes()
	equipes := cours.Teams([]teams.Info{
		{Slug: "a26-5n6-02-eq1", Name: "a26.5n6.02.eq1"},
		{Slug: "les-anciens", Name: "Les anciens"},
	})
	if len(equipes) != 0 {
		t.Fatalf("aucune ne devrait lui être rattachée : %v", teams.Names(equipes))
	}
}
