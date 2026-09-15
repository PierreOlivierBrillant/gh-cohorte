package groups_test

import (
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
)

// prof écarte l'enseignant, comme le registre le ferait.
func prof(login string) bool { return login == "prof" }

func TestLaRemiseSeDateHorsDesCommitsEcartes(t *testing.T) {
	remise := groups.Handin{
		Commits: 3,
		Last:    "2026-10-03T16:00:00Z",
		LastBy: map[string]string{
			"ecote": "2026-09-30T20:45:00Z",
			"prof":  "2026-10-03T16:00:00Z",
		},
	}
	if dernier := remise.LastBut(prof); dernier != "2026-09-30T20:45:00Z" {
		t.Errorf("LastBut = %q", dernier)
	}
	// Sans personne à écarter, c'est le dernier commit tout court.
	if dernier := remise.LastBut(nil); dernier != "2026-10-03T16:00:00Z" {
		t.Errorf("LastBut(nil) = %q", dernier)
	}
}

// Un dépôt où l'on n'a écarté que ce qui s'y trouvait n'a rien reçu : c'est une
// réponse, pas un manque d'information.
func TestUnDepotOuToutEstEcarteNaPasDeRemise(t *testing.T) {
	remise := groups.Handin{
		Commits: 1,
		Last:    "2026-10-03T16:00:00Z",
		LastBy:  map[string]string{"prof": "2026-10-03T16:00:00Z"},
	}
	if dernier := remise.LastBut(prof); dernier != "" {
		t.Errorf("LastBut = %q, attendu aucune remise", dernier)
	}
}

// Un relevé d'une forme antérieure ne porte pas ses auteurs. La date du dernier
// commit est alors tout ce qu'on sait, et mieux vaut une date trop tardive que
// pas de date du tout.
func TestUnReleveSansAuteursRendLaDateQuIlPorte(t *testing.T) {
	remise := groups.Handin{Commits: 2, Last: "2026-10-03T16:00:00Z"}
	if dernier := remise.LastBut(prof); dernier != "2026-10-03T16:00:00Z" {
		t.Errorf("LastBut = %q", dernier)
	}
}
