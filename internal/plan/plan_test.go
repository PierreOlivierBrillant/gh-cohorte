package plan_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

func reglages(pattern string) config.Settings {
	settings := config.Default()
	settings.Assignment = "tp1"
	settings.NamePattern = pattern
	return settings
}

func TestRenderTousLesChamps(t *testing.T) {
	personne := roster.Person{FullName: "Émilie Côté", Username: "emilie-cote"}
	cas := map[string]string{
		"{assignment}-{username}": "tp1-emilie-cote",
		"{name}":                  "emilie-cote",
		"{fullname}":              "Émilie Côté",
		"{first}.{last}":          "emilie.cote",
		"{assignment}-{index}":    "tp1-03",
	}
	for gabarit, attendu := range cas {
		if obtenu := plan.Render(gabarit, personne, "tp1", 3); obtenu != attendu {
			t.Errorf("Render(%q) = %q, attendu %q", gabarit, obtenu, attendu)
		}
	}
}

func TestRenderPrenomSeul(t *testing.T) {
	personne := roster.Person{FullName: "Prince", Username: "prince"}
	if obtenu := plan.Render("{first}|{last}", personne, "tp1", 1); obtenu != "prince|" {
		t.Errorf("Render = %q", obtenu)
	}
}

func TestValidatePattern(t *testing.T) {
	if _, err := plan.ValidatePattern("{assignment}-{username}", "Gabarit de nom", true); err != nil {
		t.Errorf("gabarit valide refusé : %v", err)
	}
	if _, err := plan.ValidatePattern("{assignment}-{inconnu}", "Gabarit de nom", true); err == nil {
		t.Error("un champ inconnu doit être refusé")
	} else if !strings.Contains(err.Error(), "{inconnu}") {
		t.Errorf("le message doit nommer le champ fautif : %v", err)
	}
	if _, err := plan.ValidatePattern("{assignment}", "Gabarit de nom", true); err == nil {
		t.Error("un gabarit non distinctif doit être refusé")
	}
	if _, err := plan.ValidatePattern("{assignment}", "Gabarit de description", false); err != nil {
		t.Errorf("une description non distinctive est permise : %v", err)
	}
	if _, err := plan.ValidatePattern("   ", "Gabarit de nom", true); err == nil {
		t.Error("un gabarit vide doit être refusé")
	}
}

func TestBuildNomsEtDescriptions(t *testing.T) {
	gens := []roster.Person{
		{FullName: "Émilie Côté", Username: "emilie-cote"},
		{FullName: "Jean-Luc Picard", Username: "jlpicard"},
	}
	items, err := plan.Build(gens, reglages("{assignment}-{username}"))
	if err != nil {
		t.Fatalf("Build : %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("%d dépôt(s) planifié(s)", len(items))
	}
	if items[0].Name != "tp1-emilie-cote" || items[1].Name != "tp1-jlpicard" {
		t.Errorf("noms = %q, %q", items[0].Name, items[1].Name)
	}
	if items[0].Description != "tp1 — Émilie Côté" {
		t.Errorf("description = %q", items[0].Description)
	}
}

func TestBuildRefuseLesCollisions(t *testing.T) {
	gens := []roster.Person{
		{FullName: "Jean Tremblay", Username: "jtremblay1"},
		{FullName: "Jean Tremblay", Username: "jtremblay2"},
	}
	_, err := plan.Build(gens, reglages("{assignment}-{name}"))
	if err == nil {
		t.Fatal("deux personnes visant le même dépôt doivent être refusées")
	}
	if !strings.Contains(err.Error(), "Collision") {
		t.Errorf("message inattendu : %v", err)
	}
}

func TestBuildRefuseUnNomDeDepotInvalide(t *testing.T) {
	gens := []roster.Person{{FullName: "Émilie Côté", Username: "emilie-cote"}}
	settings := reglages("{fullname} {username}")
	if _, err := plan.Build(gens, settings); err == nil {
		t.Error("un nom de dépôt avec espace doit être refusé")
	}
}

func TestBuildDescriptionTronquee(t *testing.T) {
	gens := []roster.Person{{FullName: strings.Repeat("é", 120), Username: "quelquun"}}
	settings := reglages("{assignment}-{username}")
	settings.DescriptionPattern = strings.Repeat("{fullname} ", 5)
	items, err := plan.Build(gens, settings)
	if err != nil {
		t.Fatalf("Build : %v", err)
	}
	if longueur := len([]rune(items[0].Description)); longueur != 350 {
		t.Errorf("description de %d caractères, attendu 350", longueur)
	}
}
