package valid_test

import (
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Une date seule désigne la fin de la journée : « remis le 1er octobre » veut
// dire avant que le 1er octobre ne soit fini, pas avant qu'il ne commence.
func TestUneDateSansHeureVautLaFinDuJour(t *testing.T) {
	echeance, err := valid.ParseDue("2026-10-01")
	if err != nil {
		t.Fatalf("ParseDue : %v", err)
	}
	attendu := time.Date(2026, 10, 1, 23, 59, 59, 0, time.Local)
	if !echeance.Equal(attendu) {
		t.Errorf("ParseDue(« 2026-10-01 ») = %s, attendu %s", echeance, attendu)
	}
}

func TestUneHeureDonneeEstGardee(t *testing.T) {
	echeance, err := valid.ParseDue("2026-10-01T13:30")
	if err != nil {
		t.Fatalf("ParseDue : %v", err)
	}
	if echeance.Hour() != 13 || echeance.Minute() != 30 {
		t.Errorf("ParseDue(« 2026-10-01T13:30 ») = %s, heure perdue", echeance)
	}
	// Le fuseau est celui de la machine : c'est dans celui-là que l'échéance a
	// été annoncée à la classe.
	if _, decalage := echeance.Zone(); decalage != offsetLocal(2026, 10, 1) {
		t.Errorf("ParseDue rend %s, hors du fuseau local", echeance)
	}
}

func offsetLocal(annee int, mois time.Month, jour int) int {
	_, decalage := time.Date(annee, mois, jour, 13, 30, 0, 0, time.Local).Zone()
	return decalage
}

func TestUneDateCibleSeMetEnFormeOuSeRefuse(t *testing.T) {
	for _, cas := range []struct{ saisie, attendu string }{
		{"2026-10-01", "2026-10-01"},
		{"  2026-10-01  ", "2026-10-01"},
		{"2026-10-01T23:59", "2026-10-01T23:59"},
		// Une valeur vide reste vide : c'est ainsi qu'on retire une date.
		{"", ""},
		{"   ", ""},
	} {
		normale, err := valid.NormalizeDue(cas.saisie)
		if err != nil || normale != cas.attendu {
			t.Errorf("NormalizeDue(%q) = %q, %v — attendu %q",
				cas.saisie, normale, err, cas.attendu)
		}
	}

	for _, saisie := range []string{
		"le 1er octobre", "2026-13-40", "01/10/2026", "2026-10-01 23:59", "2026",
	} {
		if _, err := valid.NormalizeDue(saisie); err == nil {
			t.Errorf("NormalizeDue accepte %q", saisie)
		}
		if _, err := valid.ParseDue(saisie); err == nil {
			t.Errorf("ParseDue accepte %q", saisie)
		}
	}
}

// Une date vide n'est pas une erreur : c'est un travail sans échéance, et rien
// n'y sera jamais dit en retard.
func TestUneDateVideNEstPasUneErreur(t *testing.T) {
	echeance, err := valid.ParseDue("")
	if err != nil {
		t.Fatalf("ParseDue : %v", err)
	}
	if !echeance.IsZero() {
		t.Errorf("ParseDue(« ») = %s", echeance)
	}
}
