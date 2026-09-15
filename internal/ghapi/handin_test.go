package ghapi_test

import (
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

func TestHandinCompteLesCommitsEtLeursAuteurs(t *testing.T) {
	state := fakegh.NewState()
	depot := state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	// Du plus récent au plus ancien, comme GitHub les rend.
	depot.History = fakegh.Commits(
		"2026-10-02T09:15:00Z", "2026-09-28T18:00:00Z", "2026-09-01T08:00:00Z",
	)
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
	depot.History = fakegh.Commits("2026-09-20T10:00:00Z")
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

// Dater une remise demande de savoir qui a commis quand, et pas seulement quand
// le dépôt a bougé pour la dernière fois : c'est ce qui permet ensuite d'écarter
// ce qu'un enseignant y a poussé.
func TestHandinDateLeDernierCommitDeChaqueAuteur(t *testing.T) {
	state := fakegh.NewState()
	depot := state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	depot.History = []fakegh.HistoryEntry{
		{At: "2026-10-05T11:00:00Z", Login: "github-classroom[bot]"},
		{At: "2026-10-03T16:00:00Z", Login: "prof"},
		{At: "2026-09-30T20:45:00Z", Login: "ecote"},
		{At: "2026-09-28T18:00:00Z", Login: "ECote"},
	}
	state.AddContributors("acme/a26.5n6.01.tp1.emilie-cote", "ecote", "ecote", "prof")

	c, _ := client(t, state)
	remise, err := c.Handin("acme", "a26.5n6.01.tp1.emilie-cote")
	if err != nil {
		t.Fatalf("Handin : %v", err)
	}
	if remise.Commits != 4 {
		t.Errorf("Commits = %d, attendu 4", remise.Commits)
	}
	if remise.LastBy["ecote"] != "2026-09-30T20:45:00Z" {
		t.Errorf("dernier commit d'Émilie = %q", remise.LastBy["ecote"])
	}
	if remise.LastBy["prof"] != "2026-10-03T16:00:00Z" {
		t.Errorf("dernier commit de l'enseignant = %q", remise.LastBy["prof"])
	}
	// Un robot écrit lui aussi : ce qu'il a fait ne date la remise de personne.
	if _, present := remise.LastBy["github-classroom[bot]"]; present {
		t.Errorf("un robot date une remise : %v", remise.LastBy)
	}
	// Écarter l'enseignant rend la remise à sa date.
	if dernier := remise.LastBut(func(login string) bool { return login == "prof" }); dernier !=
		"2026-09-30T20:45:00Z" {
		t.Errorf("LastBut = %q", dernier)
	}
}

// Un commit qu'aucune adresse ne rattache à un compte date quand même une
// remise : c'est celui d'un étudiant dont la machine est mal configurée, et le
// taire le ferait passer pour muet.
func TestUnCommitSansCompteDateQuandMemeLaRemise(t *testing.T) {
	state := fakegh.NewState()
	depot := state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	depot.History = []fakegh.HistoryEntry{
		{At: "2026-09-20T10:00:00Z"},
		{At: "2026-09-18T10:00:00Z", Login: "prof"},
	}

	c, _ := client(t, state)
	remise, err := c.Handin("acme", "a26.5n6.01.tp1.jean-luc-picard")
	if err != nil {
		t.Fatalf("Handin : %v", err)
	}
	if remise.LastBy[""] != "2026-09-20T10:00:00Z" {
		t.Errorf("le commit sans compte n'est pas daté : %v", remise.LastBy)
	}
	if dernier := remise.LastBut(func(login string) bool { return login == "prof" }); dernier !=
		"2026-09-20T10:00:00Z" {
		t.Errorf("LastBut = %q", dernier)
	}
}
