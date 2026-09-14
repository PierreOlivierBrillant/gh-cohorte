package classroom_test

import (
	"strings"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
)

// remise compose ce qu'un historique dirait d'un dépôt.
func remise(commits int, dernier string, auteurs map[string]int) groups.Handin {
	return groups.Handin{Commits: commits, Last: dernier, Authors: auteurs}
}

// instant rend la date d'un commit, au format que GitHub emploie.
func instant(jour, heure string) string {
	moment, err := time.ParseInLocation("2006-01-02 15:04", jour+" "+heure, time.Local)
	if err != nil {
		panic(err)
	}
	return moment.UTC().Format(time.RFC3339)
}

// calendrier joue le registre de l'organisation : il ne retient des travaux que
// leur date, sous leur identifiant complet — c'est tout ce qu'un groupe lui
// demande.
type calendrier map[string]string

func (c calendrier) Due(assignmentID string) string {
	return c[strings.ToLower(strings.TrimSpace(assignmentID))]
}

// date rend la seule échéance qu'un changement de groupe veut écrire.
func date(lignes []classroom.Deadline, id string) (string, bool) {
	for _, ligne := range lignes {
		if strings.EqualFold(ligne.Assignment, id) {
			return ligne.Due, true
		}
	}
	return "", false
}

func TestDateCibleSeLitDepuisLeRegistre(t *testing.T) {
	cours := groupe("a26", "5n6", "01", personnes("Émilie Côté", "ecote")).
		Scheduling(calendrier{"a26.5n6.01.tp1": "2026-10-01"})

	if cours.DueOf("tp1") != "2026-10-01" {
		t.Errorf("DueOf(« tp1 ») = %q", cours.DueOf("tp1"))
	}
	// L'identifiant complet désigne le même travail que le nom court.
	if cours.DueOf("a26.5n6.01.tp1") != "2026-10-01" {
		t.Errorf("DueOf(identifiant complet) = %q", cours.DueOf("a26.5n6.01.tp1"))
	}
	if cours.DueOf("tp2") != "" {
		t.Errorf("un travail sans échéance en rend une : %q", cours.DueOf("tp2"))
	}
	// Sans registre, rien n'est affirmé : mieux vaut ne rien dire que de faire
	// croire à un travail sans échéance.
	if sans := groupe("a26", "5n6", "01", nil); sans.DueOf("tp1") != "" {
		t.Errorf("un groupe sans calendrier répond %q", sans.DueOf("tp1"))
	}
}

func TestDateCibleSeRangeSousLIdentifiantComplet(t *testing.T) {
	cours := groupe("a26", "5n6", "01", nil)

	lignes, err := cours.SetDue("tp1", "2026-10-01")
	if err != nil {
		t.Fatalf("SetDue : %v", err)
	}
	// Le nom court ne suffirait pas : le registre porte toute l'organisation.
	due, vise := date(lignes, "a26.5n6.01.tp1")
	if !vise || due != "2026-10-01" {
		t.Errorf("SetDue = %+v", lignes)
	}

	// Une date vide retire l'échéance : c'est la même décision dans l'autre sens.
	sans, err := cours.SetDue("tp1", "")
	if err != nil {
		t.Fatalf("SetDue : %v", err)
	}
	if due, vise := date(sans, "a26.5n6.01.tp1"); !vise || due != "" {
		t.Errorf("le retrait ne vise pas le travail : %+v", sans)
	}
}

func TestDateCibleRefuseCeQuiNEnEstPasUne(t *testing.T) {
	cours := groupe("a26", "5n6", "01", nil)
	if _, err := cours.SetDue("tp1", "le 1er octobre"); err == nil {
		t.Error("SetDue accepte « le 1er octobre »")
	}
	if _, err := cours.SetDue("", "2026-10-01"); err == nil {
		t.Error("SetDue accepte un travail sans nom")
	}
}

func TestDateCibleSuitLeTravailRenomme(t *testing.T) {
	cours := groupe("a26", "5n6", "01", nil).
		Scheduling(calendrier{"a26.5n6.01.tp1": "2026-10-01"})

	lignes := cours.RenameDue("tp1", "projet-final")
	if due, vise := date(lignes, "a26.5n6.01.projet-final"); !vise || due != "2026-10-01" {
		t.Errorf("le nouveau nom ne reçoit pas l'échéance : %+v", lignes)
	}
	if due, vise := date(lignes, "a26.5n6.01.tp1"); !vise || due != "" {
		t.Errorf("l'ancien nom garde l'échéance : %+v", lignes)
	}
	// Un travail sans échéance n'a rien à reporter.
	if reste := cours.RenameDue("tp2", "tp3"); len(reste) != 0 {
		t.Errorf("RenameDue = %+v pour un travail sans date", reste)
	}
}

func TestDateCibleSuitLeTravailDeplace(t *testing.T) {
	depart := groupe("a26", "5n6", "01", nil).
		Scheduling(calendrier{"a26.5n6.01.tp1": "2026-10-01"})
	arrivee := groupe("a26", "5n6", "02", nil)

	lignes := classroom.MoveDue(depart, arrivee,
		[]classroom.Relocation{{ID: "a26.5n6.01.tp1", Name: "tp1"}})
	if due, vise := date(lignes, "a26.5n6.02.tp1"); !vise || due != "2026-10-01" {
		t.Errorf("le groupe d'arrivée n'a pas l'échéance : %+v", lignes)
	}
	if due, vise := date(lignes, "a26.5n6.01.tp1"); !vise || due != "" {
		t.Errorf("le groupe de départ garde l'échéance : %+v", lignes)
	}
}

// --------------------------------------------------------------- les bilans

func TestUnCommitApresLaDateCibleEstEnRetard(t *testing.T) {
	cours := groupe("a26", "5n6", "01", personnes("Émilie Côté", "ecote")).
		Scheduling(calendrier{"a26.5n6.01.tp1": "2026-10-01"})
	repos := depots("a26.5n6.01.tp1.emilie-cote")
	remises := map[string]groups.Handin{
		"a26.5n6.01.tp1.emilie-cote": remise(4, instant("2026-10-02", "09:15"),
			map[string]int{"ecote": 4}),
	}

	bilans := cours.Reviews("a26.5n6.01.tp1", repos, nil, remises)
	if len(bilans) != 1 {
		t.Fatalf("%d bilan(s), attendu 1", len(bilans))
	}
	if !bilans[0].Late {
		t.Error("un commit du 2 octobre n'est pas vu en retard sur le 1er")
	}
	if bilans[0].Commits != 4 {
		t.Errorf("Commits = %d, attendu 4", bilans[0].Commits)
	}
	if bilans[0].Missing() {
		t.Errorf("l'étudiante qui a quatre commits est dite muette : %v", bilans[0].Silent)
	}
}

func TestUnCommitAvantLaDateCibleNEstPasEnRetard(t *testing.T) {
	cours := groupe("a26", "5n6", "01", personnes("Émilie Côté", "ecote")).
		Scheduling(calendrier{"a26.5n6.01.tp1": "2026-10-01"})
	repos := depots("a26.5n6.01.tp1.emilie-cote")
	remises := map[string]groups.Handin{
		// Le même jour, avant minuit : une date sans heure vaut la fin du jour.
		"a26.5n6.01.tp1.emilie-cote": remise(2, instant("2026-10-01", "23:30"),
			map[string]int{"ecote": 2}),
	}
	bilans := cours.Reviews("a26.5n6.01.tp1", repos, nil, remises)
	if bilans[0].Late {
		t.Error("un commit du 1er à 23 h 30 est vu en retard sur le 1er")
	}
}

func TestSansDateCibleRienNEstJamaisEnRetard(t *testing.T) {
	cours := groupe("a26", "5n6", "01", personnes("Émilie Côté", "ecote"))
	repos := depots("a26.5n6.01.tp1.emilie-cote")
	remises := map[string]groups.Handin{
		"a26.5n6.01.tp1.emilie-cote": remise(1, instant("2030-01-01", "12:00"),
			map[string]int{"ecote": 1}),
	}
	if bilans := cours.Reviews("a26.5n6.01.tp1", repos, nil, remises); bilans[0].Late {
		t.Error("un travail sans échéance a un retard")
	}
}

func TestUnEtudiantSansAucunCommitEstSignale(t *testing.T) {
	cours := groupe("a26", "5n6", "01", personnes("Émilie Côté", "ecote"))
	repos := depots("a26.5n6.01.tp1.emilie-cote")
	remises := map[string]groups.Handin{
		// Seul l'enseignant a écrit : les fichiers de départ, rien d'autre.
		"a26.5n6.01.tp1.emilie-cote": remise(1, instant("2026-09-01", "08:00"),
			map[string]int{"prof": 1}),
	}
	bilans := cours.Reviews("a26.5n6.01.tp1", repos, nil, remises)
	if !bilans[0].Missing() {
		t.Fatal("une étudiante sans aucun commit n'est pas signalée")
	}
	if bilans[0].Silent[0].Username != "ecote" {
		t.Errorf("Silent = %v", bilans[0].Silent)
	}
}

func TestUnCommitSansCompteEstRattacheParSonNom(t *testing.T) {
	cours := groupe("a26", "5n6", "01", personnes("Émilie Côté", "ecote"))
	repos := depots("a26.5n6.01.tp1.emilie-cote")
	// Une machine mal configurée : le commit porte un nom, aucun compte.
	muet := remise(3, instant("2026-09-20", "10:00"), nil)
	muet.Anonymous = []groups.Author{
		{Name: "Emilie Cote", Email: "emilie.cote@college.qc.ca", Commits: 3},
	}
	remises := map[string]groups.Handin{"a26.5n6.01.tp1.emilie-cote": muet}

	bilans := cours.Reviews("a26.5n6.01.tp1", repos, nil, remises)
	if bilans[0].Missing() {
		t.Errorf("un commit signé de son nom ne la compte pas : %v", bilans[0].Silent)
	}
}

func TestTousLesMembresMuetsDUneEquipeSontNommes(t *testing.T) {
	cours := groupe("a26", "5n6", "01",
		personnes("Émilie Côté", "ecote", "Jean-Luc Picard", "jlpicard"))
	repos := depots("a26.5n6.01.projet.eq1")
	equipes := []teams.Team{{
		Slug: "a26-5n6-01-eq1", Name: "a26.5n6.01.eq1", Short: "eq1",
		Members: []string{"ecote", "jlpicard"},
	}}
	remises := map[string]groups.Handin{
		"a26.5n6.01.projet.eq1": remise(5, instant("2026-11-01", "09:00"),
			map[string]int{"ecote": 5}),
	}

	bilans := cours.Reviews("a26.5n6.01.projet", repos, equipes, remises)
	if len(bilans) != 1 {
		t.Fatalf("%d bilan(s), attendu 1", len(bilans))
	}
	if len(bilans[0].Silent) != 1 || bilans[0].Silent[0].Username != "jlpicard" {
		t.Errorf("Silent = %v, attendu le seul @jlpicard", bilans[0].Silent)
	}
}

func TestUnDepotNonReleveNEstPasUnDepotVide(t *testing.T) {
	cours := groupe("a26", "5n6", "01",
		personnes("Émilie Côté", "ecote", "Jean-Luc Picard", "jlpicard")).
		Scheduling(calendrier{"a26.5n6.01.tp1": "2026-10-01"})
	repos := depots("a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.jean-luc-picard")
	// Un seul des deux historiques a été lu.
	remises := map[string]groups.Handin{
		"a26.5n6.01.tp1.emilie-cote": remise(3, instant("2026-10-03", "09:00"),
			map[string]int{"ecote": 3}),
	}

	bilans := cours.Reviews("a26.5n6.01.tp1", repos, nil, remises)
	if len(bilans) != 1 {
		t.Fatalf("%d bilan(s), attendu 1 : le dépôt non relevé n'a rien à dire", len(bilans))
	}

	travaux := cours.WithHandins(cours.Assignments(repos, nil), repos, nil, remises)
	if len(travaux) != 1 {
		t.Fatalf("%d travail(s), attendu 1", len(travaux))
	}
	if travaux[0].Seen != 1 {
		t.Errorf("Seen = %d, attendu 1 sur 2 dépôts", travaux[0].Seen)
	}
	if travaux[0].Commits != 3 {
		t.Errorf("Commits = %d, attendu 3", travaux[0].Commits)
	}
	if travaux[0].Late != 1 {
		t.Errorf("Late = %d, attendu 1", travaux[0].Late)
	}
	if travaux[0].Due != "2026-10-01" {
		t.Errorf("Due = %q", travaux[0].Due)
	}
}
