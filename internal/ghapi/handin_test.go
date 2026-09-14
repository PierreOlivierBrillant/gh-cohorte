package ghapi_test

import (
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

func TestHandinCompteLesCommitsEtLeursAuteurs(t *testing.T) {
	state := fakegh.NewState()
	depot := state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	// Du plus récent au plus ancien, comme GitHub les rend.
	depot.History = []string{
		"2026-10-02T09:15:00Z", "2026-09-28T18:00:00Z", "2026-09-01T08:00:00Z",
	}
	state.AddContributors("acme/a26.5n6.01.tp1.emilie-cote", "ecote", "ecote", "prof")

	c, _ := client(t, state)
	remise, err := c.Handin("acme", "a26.5n6.01.tp1.emilie-cote")
	if err != nil {
		t.Fatalf("Handin : %v", err)
	}
	if remise.Commits != 3 {
		t.Errorf("Commits = %d, attendu 3", remise.Commits)
	}
	if remise.Last != "2026-10-02T09:15:00Z" {
		t.Errorf("Last = %q", remise.Last)
	}
	if remise.Authors["ecote"] != 2 || remise.Authors["prof"] != 1 {
		t.Errorf("Authors = %v", remise.Authors)
	}
	if remise.By([]string{"ECOTE"}) != 2 {
		t.Errorf("By(« ECOTE ») = %d : la casse d'un compte ne compte pas", remise.By([]string{"ECOTE"}))
	}
}

func TestHandinReleveLesAuteursSansCompte(t *testing.T) {
	state := fakegh.NewState()
	depot := state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	depot.History = []string{"2026-09-20T10:00:00Z"}
	state.AddAnonymous("acme/a26.5n6.01.tp1.jean-luc-picard", fakegh.AnonymousAuthor{
		Name: "Jean-Luc Picard", Email: "jlp@enterprise.fed", Commits: 1,
	})

	c, _ := client(t, state)
	remise, err := c.Handin("acme", "a26.5n6.01.tp1.jean-luc-picard")
	if err != nil {
		t.Fatalf("Handin : %v", err)
	}
	if len(remise.Anonymous) != 1 || remise.Anonymous[0].Commits != 1 {
		t.Fatalf("Anonymous = %v", remise.Anonymous)
	}
	if !remise.Signed("Jean-Luc Picard") {
		t.Error("un commit signé de son nom ne lui est pas rattaché")
	}
	if remise.Signed("Émilie Côté") {
		t.Error("un commit est rattaché à quelqu'un qui ne l'a pas signé")
	}
}

func TestHandinDUnDepotVideNEstPasUneErreur(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "a26.5n6.01.tp1.personne", true)

	c, _ := client(t, state)
	remise, err := c.Handin("acme", "a26.5n6.01.tp1.personne")
	if err != nil {
		t.Fatalf("Handin : %v", err)
	}
	if !remise.Empty() || remise.Last != "" {
		t.Errorf("un dépôt vide raconte quelque chose : %+v", remise)
	}
}
