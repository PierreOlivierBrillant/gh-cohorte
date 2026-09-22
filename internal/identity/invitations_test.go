package identity_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
)

// Un accès établi l'emporte sur tout, une invitation encore valable sur une
// expirée : l'état dit si la personne est entrée, et sinon s'il est encore
// temps de le faire.
func TestLEtatDUneInvitation(t *testing.T) {
	expiree := identity.Invitation{ID: 1, Login: "ecote-2", Expired: true}
	valable := identity.Invitation{ID: 2, Login: "ecote"}
	cas := []struct {
		nom     string
		acces   identity.Access
		comptes []string
		attendu identity.InvitationState
		choisie int64
	}{
		{"entrée sous un compte, expirée sous l'autre",
			identity.Access{Collaborators: []string{"Ecote"},
				Invitations: []identity.Invitation{expiree}},
			[]string{"ecote", "ecote-2"}, identity.InvitationAccepted, 0},
		{"une invitation valable suffit",
			identity.Access{Invitations: []identity.Invitation{expiree, valable}},
			[]string{"ecote", "ecote-2"}, identity.InvitationPending, 2},
		{"seule l'expirée reste",
			identity.Access{Invitations: []identity.Invitation{expiree}},
			[]string{"ecote-2"}, identity.InvitationExpired, 1},
		{"l'invitation d'un autre ne compte pas",
			identity.Access{Invitations: []identity.Invitation{expiree}},
			[]string{"jlpicard"}, identity.InvitationNone, 0},
		{"sans personne visée, tout le monde compte",
			identity.Access{Invitations: []identity.Invitation{expiree}},
			nil, identity.InvitationExpired, 1},
	}
	for _, c := range cas {
		etat, invitation := c.acces.InvitationOf(c.comptes)
		if etat != c.attendu || invitation.ID != c.choisie {
			t.Errorf("%s : %q, invitation %d — attendu %q, invitation %d",
				c.nom, etat, invitation.ID, c.attendu, c.choisie)
		}
	}
}

// Le renvoi garde le droit que la première invitation promettait : GitHub le
// nomme « read » dans une invitation, et « pull » à l'ajout d'un collaborateur.
func TestRenvoyerGardeLeDroitPromis(t *testing.T) {
	client, serveur := monter(t)
	serveur.State.AddRepo("acme", "tp1-aminata-d", true)
	serveur.State.Invite("acme/tp1-aminata-d", "aminata-d", "pull")
	serveur.State.ExpireInvitations("acme/tp1-aminata-d")
	ancienne := serveur.State.Invitations["acme/tp1-aminata-d"][0].ID

	resolveur := identity.New(client, cache.NewIn(t.TempDir(), true), 4)
	renvoi, err := resolveur.Resend("acme", "tp1-aminata-d", ancienne)
	if err != nil {
		t.Fatalf("renvoi : %v", err)
	}
	if renvoi.Invitation.Permission != "pull" {
		t.Errorf("droit renvoyé = %q, attendu « pull »", renvoi.Invitation.Permission)
	}
	invitations := serveur.State.Invitations["acme/tp1-aminata-d"]
	if len(invitations) != 1 || invitations[0].ID == ancienne || invitations[0].Expired ||
		invitations[0].Permission != "pull" {
		t.Fatalf("invitations = %+v", invitations)
	}
}

// Ce qu'il faut envoyer suit l'état : rien à qui est entré ou attend encore,
// une neuve à la place d'une invitation expirée, et une première à chacun des
// comptes de qui n'a rien — jamais invité, invitation refusée ou annulée.
func TestCeQuIlFautEnvoyer(t *testing.T) {
	expiree := identity.Invitation{ID: 1, Login: "ecote", Permission: "pull", Expired: true}
	cas := []struct {
		nom     string
		acces   identity.Access
		comptes []string
		attendu []string
	}{
		{"entrée", identity.Access{Collaborators: []string{"ecote"}},
			[]string{"ecote"}, nil},
		{"attend encore", identity.Access{Invitations: []identity.Invitation{{ID: 2, Login: "ecote"}}},
			[]string{"ecote"}, nil},
		{"expirée", identity.Access{Invitations: []identity.Invitation{expiree}},
			[]string{"ecote"}, []string{"renvoi @ecote pull"}},
		{"sans invitation, sous ses deux comptes", identity.Access{},
			[]string{"ecote", "emilie-perso"},
			[]string{"première @ecote push", "première @emilie-perso push"}},
		// L'invitation d'un autre ne dit rien d'elle : elle n'en a pas.
		{"l'invitation expirée d'un autre", identity.Access{Invitations: []identity.Invitation{expiree}},
			[]string{"jlpicard"}, []string{"première @jlpicard push"}},
		// Sans personne visée, on ne sait pas qui inviter pour la première fois.
		{"personne de visé", identity.Access{}, nil, nil},
		{"personne de visé, une expirée", identity.Access{Invitations: []identity.Invitation{expiree}},
			nil, []string{"renvoi @ecote pull"}},
	}
	for _, c := range cas {
		var obtenus []string
		for _, envoi := range c.acces.Dispatches(c.comptes, "push") {
			geste := "renvoi"
			if envoi.First() {
				geste = "première"
			}
			obtenus = append(obtenus, geste+" @"+envoi.Invitation.Login+" "+envoi.Invitation.Permission)
		}
		if strings.Join(obtenus, ", ") != strings.Join(c.attendu, ", ") {
			t.Errorf("%s : %v, attendu %v", c.nom, obtenus, c.attendu)
		}
	}
}

// Une première invitation n'annule rien : elle part, au droit donné.
func TestUnePremiereInvitationPart(t *testing.T) {
	client, serveur := monter(t)
	serveur.State.AddRepo("acme", "tp1-aminata-d", true)

	resolveur := identity.New(client, cache.NewIn(t.TempDir(), true), 4)
	lus := resolveur.Accesses("acme", []string{"tp1-aminata-d"}, identity.Refresh, nil)
	envois := lus["tp1-aminata-d"].Dispatches([]string{"aminata-d"}, "pull")
	faits := resolveur.Send("acme", envois, nil)
	if len(faits) != 1 || faits[0].Err() != nil {
		t.Fatalf("envois = %+v", faits)
	}
	if faits[0].Summary() != "Invitation envoyée à @aminata-d (pull)." {
		t.Errorf("résumé : %q", faits[0].Summary())
	}
	invitations := serveur.State.Invitations["acme/tp1-aminata-d"]
	if len(invitations) != 1 || invitations[0].Login != "aminata-d" || invitations[0].Permission != "pull" {
		t.Fatalf("invitations = %+v", invitations)
	}
	if serveur.State.CallCount("DELETE /repos/acme/tp1-aminata-d/invitations") != 0 {
		t.Error("une première invitation a annulé quelque chose")
	}
}

// Si la nouvelle invitation ne part pas, l'ancienne est déjà annulée : le
// bilan doit le dire, pour qu'on invite la personne à la main.
func TestUnRenvoiQuiEchoueDitCeQuIlADefait(t *testing.T) {
	client, serveur := monter(t)
	serveur.State.AddRepo("acme", "tp1-aminata-d", true)
	serveur.State.Invite("acme/tp1-aminata-d", "aminata-d", "push")
	serveur.State.ExpireInvitations("acme/tp1-aminata-d")
	serveur.State.FailOn["PUT /repos/acme/tp1-aminata-d/collaborators/aminata-d"] =
		fakegh.Failure{Status: 422, Message: "Validation Failed"}

	resolveur := identity.New(client, cache.NewIn(t.TempDir(), true), 4)
	lus := resolveur.Accesses("acme", []string{"tp1-aminata-d"}, identity.Refresh, nil)
	envois := lus["tp1-aminata-d"].Dispatches([]string{"aminata-d"}, "push")
	if len(envois) != 1 || envois[0].First() {
		t.Fatalf("envois = %+v", envois)
	}
	faits := resolveur.Send("acme", envois, nil)
	if len(faits) != 1 || !strings.Contains(faits[0].Error, "ancienne invitation est annulée") {
		t.Fatalf("bilan = %+v", faits)
	}
	if faits[0].Err() == nil || faits[0].Summary() != faits[0].Error {
		t.Errorf("l'échec se tait : %q, %v", faits[0].Summary(), faits[0].Err())
	}
}
