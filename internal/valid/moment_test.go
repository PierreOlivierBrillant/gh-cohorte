package valid_test

import (
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// GitHub date tout en UTC. Une remise du soir se lirait au lendemain si on
// l'affichait telle quelle : c'est l'heure de la machine qui compte, celle où
// l'échéance a été annoncée à la classe.
func TestUnInstantSeLitALHeureDeLaMachine(t *testing.T) {
	brut := time.Date(2026, 8, 21, 22, 32, 11, 0, time.UTC)
	attendu := brut.Local().Format("2006-01-02 15:04")
	if obtenu := valid.Moment(brut.Format(time.RFC3339)); obtenu != attendu {
		t.Errorf("Moment = %q, attendu %q", obtenu, attendu)
	}
}

func TestUnInstantAbsentResteAbsent(t *testing.T) {
	if obtenu := valid.Moment("  "); obtenu != "" {
		t.Errorf("Moment = %q : un dépôt qui n'a rien reçu n'a pas de date", obtenu)
	}
}

// Une forme inattendue s'affiche sans être belle, ce qui vaut mieux que de la
// faire disparaître.
func TestUneFormeIllisibleEstRendueTelleQuelle(t *testing.T) {
	if obtenu := valid.Moment("jamais"); obtenu != "jamais" {
		t.Errorf("Moment = %q", obtenu)
	}
}

// Les bornes d'un filtre portent sur la journée : l'heure de l'envoi n'y change
// rien, sans quoi « avant le 1er octobre » écarterait le 1er octobre lui-même.
func TestLeJourSeDetacheDeLHeure(t *testing.T) {
	if obtenu := valid.Day("2026-10-01 14:32"); obtenu != "2026-10-01" {
		t.Errorf("Day = %q", obtenu)
	}
	if obtenu := valid.Day("2026-10-01"); obtenu != "2026-10-01" {
		t.Errorf("Day = %q", obtenu)
	}
	if obtenu := valid.Day(""); obtenu != "" {
		t.Errorf("Day = %q", obtenu)
	}
}
