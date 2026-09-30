package web_test

import (
	"net/http"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Un fichier que l'enseignant pousse dans le dépôt avance son « pushed_at » :
// sans l'historique, l'étudiante paraîtrait active le jour où le prof y a
// touché. Une fois l'historique relevé, le dernier envoi montré — et filtré —
// est le sien.
func TestLeDernierEnvoiIgnoreCeQueLEnseignantAPousse(t *testing.T) {
	state := fakegh.NewState()
	depot := state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	depot.PushedAt = "2025-09-12T16:00:00Z"
	depot.History = []fakegh.HistoryEntry{
		{At: "2025-09-12T16:00:00Z", Login: "prof"},
		{At: "2025-09-05T10:00:00Z", Login: "ecote"},
	}
	state.AddContributors("acme/a26.5n6.01.tp1.emilie-cote", "ecote", "prof")

	h := avantLeRegistre(t, state, cohorte("a26", "5n6", "01", "Émilie Côté", "ecote"))
	h.coopter("prof", true)

	envoi := func(requete string) (string, int) {
		var detail struct {
			Repos []struct {
				PushedAt string `json:"pushed_at"`
			} `json:"repos"`
		}
		h.json(http.MethodGet, "/api/classrooms/a26.5n6.01/assignments/tp1"+requete, nil, &detail)
		if len(detail.Repos) == 0 {
			return "", 0
		}
		return detail.Repos[0].PushedAt, len(detail.Repos)
	}

	// Avant le relevé, rien ne dit qui a poussé : la date est celle de GitHub.
	if date, _ := envoi(""); date != valid.Moment("2025-09-12T16:00:00Z") {
		t.Errorf("avant le relevé : %q", date)
	}

	h.travail(http.MethodPost, "/api/classrooms/a26.5n6.01/assignments/tp1/handins", nil)
	if date, _ := envoi(""); date != valid.Moment("2025-09-05T10:00:00Z") {
		t.Errorf("après le relevé : %q, attendu le dernier commit d'Émilie", date)
	}
	// Le filtre lit la même date : Émilie n'a rien envoyé après le 10.
	if _, nombre := envoi("?after=2025-09-10"); nombre != 0 {
		t.Errorf("le filtre retient Émilie pour le commit du prof")
	}
}
