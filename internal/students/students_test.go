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

// cohorte déclare le groupe qui sert de décor à ces tests : trois personnes,
// deux travaux, et des envois à des dates différentes.
func cohorte() (classroom.Classroom, []groups.RepoInfo) {
	cours := classroom.Classroom{
		Org: "acme", Session: "a26", Course: "5n6", Group: "01",
		Students: []roster.Person{
			{FullName: "Jean-Luc Picard", Username: "jlpicard"},
			{FullName: "Émilie Côté", Username: "ecote"},
			{FullName: "Aminata Diallo", Username: "aminata-d"},
		},
		Defaults: classroom.DefaultsFrom(config.Default()),
	}
	inventaire := []groups.RepoInfo{
		{Name: "a26.5n6.01.tp1.jean-luc-picard", PushedAt: "2026-09-01T10:00:00Z"},
		{Name: "a26.5n6.01.tp2.jean-luc-picard", PushedAt: "2026-10-15T10:00:00Z"},
		{Name: "a26.5n6.01.tp1.emilie-cote", PushedAt: "2026-09-20T10:00:00Z"},
		// Aminata a un dépôt, mais n'y a jamais rien envoyé.
		{Name: "a26.5n6.01.tp1.aminata-diallo"},
	}
	return cours, inventaire
}

// comptes rend les comptes d'une liste, dans l'ordre où elle les donne.
func comptes(lignes []students.Row) string {
	noms := make([]string, 0, len(lignes))
	for _, ligne := range lignes {
		noms = append(noms, ligne.Username)
	}
	return strings.Join(noms, ",")
}

func TestLigneRetientLePlusRecentEnvoi(t *testing.T) {
	lignes := students.Build(cohorte())
	trouve := map[string]students.Row{}
	for _, ligne := range lignes {
		trouve[ligne.Username] = ligne
	}
	if picard := trouve["jlpicard"]; picard.PushedAt != "2026-10-15" || len(picard.Repos) != 2 {
		t.Fatalf("Picard : %+v", picard)
	}
	// Un dépôt sans envoi ne donne pas de date : c'est ce qui distingue « muet »
	// de « n'a rien ».
	if aminata := trouve["aminata-d"]; aminata.PushedAt != "" || len(aminata.Repos) != 1 {
		t.Fatalf("Aminata : %+v", aminata)
	}
}

func TestRechercheIgnoreCasseEtAccents(t *testing.T) {
	lignes := students.Build(cohorte())
	for _, cherche := range []string{"cote", "CÔTÉ", "Émilie"} {
		retenues := students.Apply(lignes, students.Filter{Text: cherche}, students.ByName, false)
		if comptes(retenues) != "ecote" {
			t.Fatalf("« %s » : %s", cherche, comptes(retenues))
		}
	}
	// Le compte se cherche à part du nom.
	retenues := students.Apply(lignes, students.Filter{Username: "picard"}, students.ByName, false)
	if comptes(retenues) != "jlpicard" {
		t.Fatalf("par compte : %s", comptes(retenues))
	}
	if comptes(students.Apply(lignes,
		students.Filter{Name: "picard"}, students.ByName, false)) != "jlpicard" {
		t.Fatal("« picard » est aussi dans le nom complet")
	}
}

func TestBornesDuDernierEnvoi(t *testing.T) {
	lignes := students.Build(cohorte())

	apres := students.Apply(lignes,
		students.Filter{PushedAfter: "2026-10-01"}, students.ByName, false)
	if comptes(apres) != "jlpicard" {
		t.Fatalf("après le 1er octobre : %s", comptes(apres))
	}
	avant := students.Apply(lignes,
		students.Filter{PushedBefore: "2026-09-30"}, students.ByName, false)
	if comptes(avant) != "ecote" {
		t.Fatalf("avant le 30 septembre : %s", comptes(avant))
	}
	// Sans date connue, une personne n'est ni avant ni après : elle se retrouve
	// par « muet », pas par une borne.
	muets := students.Apply(lignes,
		students.Filter{Activity: students.Silent}, students.ByName, false)
	if comptes(muets) != "aminata-d" {
		t.Fatalf("muets : %s", comptes(muets))
	}
}

func TestActiviteEtTravail(t *testing.T) {
	cours, inventaire := cohorte()
	cours.Students = append(cours.Students,
		roster.Person{FullName: "Zoé Tremblay", Username: "ztremblay"})
	lignes := students.Build(cours, inventaire)

	sans := students.Apply(lignes,
		students.Filter{Activity: students.WithoutRepos}, students.ByName, false)
	if comptes(sans) != "ztremblay" {
		t.Fatalf("sans dépôt : %s", comptes(sans))
	}
	avec := students.Apply(lignes,
		students.Filter{Activity: students.WithRepos}, students.ByUsername, false)
	if comptes(avec) != "aminata-d,ecote,jlpicard" {
		t.Fatalf("avec dépôt : %s", comptes(avec))
	}
	tp2 := students.Apply(lignes,
		students.Filter{Assignment: "tp2"}, students.ByName, false)
	if comptes(tp2) != "jlpicard" {
		t.Fatalf("tp2 : %s", comptes(tp2))
	}
}

func TestTriParNomCompteEtEnvoi(t *testing.T) {
	lignes := students.Build(cohorte())

	// Les accents ne dispersent pas l'ordre : « Émilie » se range avec les E.
	if ordre := comptes(students.Apply(lignes, students.Filter{},
		students.ByName, false)); ordre != "aminata-d,ecote,jlpicard" {
		t.Fatalf("par nom : %s", ordre)
	}
	if ordre := comptes(students.Apply(lignes, students.Filter{},
		students.ByUsername, true)); ordre != "jlpicard,ecote,aminata-d" {
		t.Fatalf("par compte, décroissant : %s", ordre)
	}
	// Sans date, Aminata se range avec les plus anciens : en queue d'un tri
	// décroissant, en tête d'un tri croissant.
	if ordre := comptes(students.Apply(lignes, students.Filter{},
		students.ByPushed, true)); ordre != "jlpicard,ecote,aminata-d" {
		t.Fatalf("par envoi, décroissant : %s", ordre)
	}
	if ordre := comptes(students.Apply(lignes, students.Filter{},
		students.ByPushed, false)); ordre != "aminata-d,ecote,jlpicard" {
		t.Fatalf("par envoi, croissant : %s", ordre)
	}
}

func TestFiltreRefuseCeQuIlNePeutPasAppliquer(t *testing.T) {
	if _, err := (students.Filter{PushedAfter: "1er octobre"}).Validate(); err == nil {
		t.Fatal("une date qui n'en est pas une doit être refusée")
	}
	if _, err := (students.Filter{
		PushedAfter: "2026-10-01", PushedBefore: "2026-09-01"}).Validate(); err == nil {
		t.Fatal("des bornes inversées doivent être refusées")
	}
	if _, err := (students.Filter{Activity: "bavard"}).Validate(); err == nil {
		t.Fatal("une activité inconnue doit être refusée")
	}
	if _, err := students.ParseKey("popularite"); err == nil {
		t.Fatal("un tri inconnu doit être refusé")
	}
	// Ce qui est accepté revient normalisé.
	filtre, err := (students.Filter{
		PushedAfter: "2026-10-01", Activity: "AVEC", Text: "  Côté "}).Validate()
	if err != nil || filtre.Activity != students.WithRepos || filtre.Text != "Côté" {
		t.Fatalf("normalisation : %+v (%v)", filtre, err)
	}
}

func TestLignesDUnGroupeLuParPrefixe(t *testing.T) {
	group := groups.Build("tp1", []groups.RepoInfo{
		{Name: "tp1-jlpicard", PushedAt: "2026-09-01T10:00:00Z"},
		{Name: "tp1-ecote"},
	})
	lignes := students.FromGroup(group, map[string]string{"tp1-jlpicard": "Jean-Luc Picard"})
	if ordre := comptes(students.Apply(lignes, students.Filter{},
		students.ByPushed, true)); ordre != "jlpicard,ecote" {
		t.Fatalf("par envoi : %s", ordre)
	}
	// Le nom complet vient d'ailleurs que du dépôt ; sans lui, seul le suffixe
	// permet de retrouver la personne.
	if retenues := students.Apply(lignes, students.Filter{Text: "picard"},
		students.ByName, false); comptes(retenues) != "jlpicard" {
		t.Fatalf("recherche : %s", comptes(retenues))
	}
}
