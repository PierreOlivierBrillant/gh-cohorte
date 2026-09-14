package users_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/users"
)

// registre de poche : ce que l'organisation sait des comptes.
type registre struct {
	noms       map[string]string
	enseignent map[string]bool
}

func (r registre) Name(username string) string { return r.noms[strings.ToLower(username)] }
func (r registre) Teaches(username string) bool {
	return r.enseignent[strings.ToLower(username)]
}
func (r registre) Knows(username string) bool {
	_, connu := r.noms[strings.ToLower(username)]
	return connu
}

func (r registre) Teachers() []registry.User {
	fiches := make([]registry.User, 0)
	for compte, enseigne := range r.enseignent {
		if enseigne {
			fiches = append(fiches, registry.User{
				Username: compte, FullName: r.noms[compte], IsTeacher: true})
		}
	}
	return fiches
}

func connu() registre {
	return registre{
		noms: map[string]string{
			"ecote": "Émilie Côté", "jlpicard": "Jean-Luc Picard",
			"aminata-d": "Aminata Diallo", "kjaneway": "Kathryn Janeway",
		},
		enseignent: map[string]bool{"kjaneway": true},
	}
}

// etapes abrège la lecture d'une chronologie.
func etapes(fiche users.Profile) string {
	rendus := make([]string, 0, len(fiche.Timeline))
	for _, etape := range fiche.Timeline {
		rendus = append(rendus, etape.Scope+":"+etape.Role)
	}
	return strings.Join(rendus, ",")
}

// La chronologie va du plus récent au plus ancien : c'est l'ordre où l'on
// cherche quelqu'un, et il est décidé dans le domaine pour que les trois
// interfaces montrent le même.
func TestLaChronologieVaDuPlusRecentAuPlusAncien(t *testing.T) {
	cours, inventaire := college()
	fiche := users.ProfileOf(cours, inventaire, nil, nil, connu(), "ecote")

	if etapes(fiche) != "h27.5n6.02:étudiant,a26.4w6.01:étudiant,a26.5n6.01:étudiant" {
		t.Fatalf("chronologie = %s", etapes(fiche))
	}
	if fiche.FullName != "Émilie Côté" || fiche.Courses != 3 || fiche.Taught != 0 {
		t.Errorf("fiche = %+v", fiche)
	}
	if fiche.Repos != 3 || fiche.PushedAt != "2027-02-10" {
		t.Errorf("dépôts = %d, dernier envoi = %q", fiche.Repos, fiche.PushedAt)
	}
	// Chaque étape porte de quoi se lire sans traduction : la session en
	// toutes lettres, le cours, le groupe.
	if fiche.Timeline[0].SessionName == "" || fiche.Timeline[0].Group != "02" {
		t.Errorf("étape = %+v", fiche.Timeline[0])
	}
}

// Les dépôts d'un cours sont rangés sous lui, du plus récemment touché au plus
// ancien — c'est ce qu'on vient voir.
func TestChaqueEtapePorteSesDepots(t *testing.T) {
	cours, inventaire := college()
	// Un second travail dans le même groupe, touché avant le premier.
	inventaire = append(inventaire, groups.RepoInfo{
		Name: "a26.5n6.01.tp2.emilie-cote", PushedAt: "2026-10-30T10:00:00Z",
	})
	fiche := users.ProfileOf(cours, inventaire, nil, nil, connu(), "ecote")

	var automne users.Step
	for _, etape := range fiche.Timeline {
		if etape.Scope == "a26.5n6.01" {
			automne = etape
		}
	}
	if len(automne.Assignments) != 2 {
		t.Fatalf("travaux = %+v", automne.Assignments)
	}
	if automne.Assignments[0].Name != "tp2" || automne.Assignments[1].Name != "tp1" {
		t.Errorf("ordre des travaux = %+v", automne.Assignments)
	}
	if automne.Assignments[0].Repo != "a26.5n6.01.tp2.emilie-cote" {
		t.Errorf("dépôt = %q", automne.Assignments[0].Repo)
	}
	if automne.PushedAt != "2026-10-30" || automne.Silent {
		t.Errorf("étape = %+v", automne)
	}
}

// Avoir un dépôt et n'y avoir rien envoyé n'est pas n'avoir aucun dépôt : les
// deux se remarquent autrement, et la fiche doit les distinguer.
func TestUnCoursMuetSeSignale(t *testing.T) {
	cours, _ := college()
	muet := []groups.RepoInfo{{Name: "a26.5n6.01.tp1.jean-luc-picard"}}
	fiche := users.ProfileOf(cours, muet, nil, nil, connu(), "jlpicard")

	if len(fiche.Timeline) != 1 {
		t.Fatalf("chronologie = %s", etapes(fiche))
	}
	etape := fiche.Timeline[0]
	if !etape.Silent {
		t.Error("un dépôt sans envoi rend le cours muet")
	}
	if len(etape.Assignments) != 1 || etape.PushedAt != "" {
		t.Errorf("étape = %+v", etape)
	}
}

// Un enseignant n'est inscrit nulle part : ses cours se lisent dans les
// équipes enseignantes des groupes. C'est ce qui permet de chercher ce qu'un
// collègue a déjà donné.
func TestLesCoursDonnesFigurentDansLaChronologie(t *testing.T) {
	cours, inventaire := college()
	infos := []teams.Info{
		{Slug: "a26-5n6-01-enseignants", Name: "a26.5n6.01.enseignants",
			Members: []string{"kjaneway"}},
		{Slug: "h27-5n6-02-enseignants", Name: "h27.5n6.02.enseignants",
			Members: []string{"kjaneway"}},
	}
	fiche := users.ProfileOf(cours, inventaire, nil, infos, connu(), "kjaneway")

	if etapes(fiche) != "h27.5n6.02:enseignant,a26.5n6.01:enseignant" {
		t.Fatalf("chronologie = %s", etapes(fiche))
	}
	if fiche.Taught != 2 || fiche.Courses != 0 {
		t.Errorf("donnés = %d, suivis = %d", fiche.Taught, fiche.Courses)
	}
	if !fiche.IsTeacher || fiche.Role != users.AsTeacher {
		t.Errorf("rôle = %q", fiche.Role)
	}
	// Un enseignant ne rend rien : ses étapes ne portent pas de dépôts.
	for _, etape := range fiche.Timeline {
		if len(etape.Assignments) != 0 {
			t.Errorf("%s : travaux = %+v", etape.Scope, etape.Assignments)
		}
	}
}

// Quelqu'un peut avoir suivi un cours et en donner un autre. La chronologie
// mêle les deux et dit lequel est lequel.
func TestUneMemePersonnePeutAvoirSuiviEtDonne(t *testing.T) {
	cours, inventaire := college()
	infos := []teams.Info{{
		Slug: "h27-5n6-02-enseignants", Name: "h27.5n6.02.enseignants",
		Members: []string{"jlpicard"},
	}}
	fiche := users.ProfileOf(cours, inventaire, nil, infos, connu(), "jlpicard")

	if etapes(fiche) != "h27.5n6.02:enseignant,a26.5n6.01:étudiant" {
		t.Fatalf("chronologie = %s", etapes(fiche))
	}
	if fiche.Taught != 1 || fiche.Courses != 1 {
		t.Errorf("donnés = %d, suivis = %d", fiche.Taught, fiche.Courses)
	}
}

// Une personne travaille parfois sous deux comptes. Ils mènent tous à la même
// fiche : elle n'en a qu'une.
func TestTousSesComptesMenentALaMemeFiche(t *testing.T) {
	cours, inventaire := college()
	cours[0].Students[1].Also = []string{"emilie-perso"}
	cours[0].Students[1].StudentID = "2100123"

	parLeSien := users.ProfileOf(cours, inventaire, nil, nil, connu(), "ecote")
	parLAutre := users.ProfileOf(cours, inventaire, nil, nil, connu(), "emilie-perso")

	if parLAutre.Username != parLeSien.Username {
		t.Fatalf("comptes = %q et %q", parLeSien.Username, parLAutre.Username)
	}
	if len(parLAutre.Accounts) != 2 {
		t.Errorf("comptes de la personne = %v", parLAutre.Accounts)
	}
	// Le matricule est ce qui a réuni ces deux comptes : la fiche le porte.
	if parLAutre.StudentID != "2100123" {
		t.Errorf("matricule = %q", parLAutre.StudentID)
	}
	if etapes(parLAutre) != etapes(parLeSien) {
		t.Errorf("chronologies : %s / %s", etapes(parLeSien), etapes(parLAutre))
	}
}

// Un compte que rien ne connaît rend une fiche vide plutôt qu'une page
// absente : le compte existe sur GitHub, il n'a simplement rien fait ici.
func TestUnCompteInconnuRendUneFicheVide(t *testing.T) {
	cours, inventaire := college()
	fiche := users.ProfileOf(cours, inventaire, nil, nil, connu(), "personne")

	if fiche.Known {
		t.Error("le registre ne connaît pas ce compte")
	}
	if len(fiche.Timeline) != 0 || fiche.Repos != 0 {
		t.Errorf("fiche = %+v", fiche)
	}
	// Son compte reste celui qu'on a demandé : c'est par lui qu'on ouvre son
	// profil GitHub.
	if fiche.Username != "personne" || strings.Join(fiche.Accounts, ",") != "personne" {
		t.Errorf("comptes = %v", fiche.Accounts)
	}
	if fiche.Role != users.AsStudent {
		t.Errorf("rôle = %q", fiche.Role)
	}
}
