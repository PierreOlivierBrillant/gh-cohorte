package web_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// travailAuxQuatreInvitations monte un travail où chaque étudiant en est à un
// point différent : Émilie est entrée dans son dépôt, Jean-Luc a encore le
// temps de répondre, le délai d'Aminata est passé, et Bruno n'a pas
// d'invitation du tout — il l'a refusée, ou elle a été annulée.
func travailAuxQuatreInvitations(t *testing.T) *harnais {
	t.Helper()
	state := fakegh.NewState()
	state.Users["btanguay"] = "Bruno Tanguay"

	state.AddRepo("acme", "a26.5n6.01.tp1.emilie-cote", true)
	state.AddCollaborator("acme/a26.5n6.01.tp1.emilie-cote", "emilie-cote", "push")

	state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	state.Invite("acme/a26.5n6.01.tp1.jean-luc-picard", "jlpicard", "push")

	state.AddRepo("acme", "a26.5n6.01.tp1.aminata-diallo", true)
	state.Invite("acme/a26.5n6.01.tp1.aminata-diallo", "aminata-d", "pull")
	state.ExpireInvitations("acme/a26.5n6.01.tp1.aminata-diallo")

	state.AddRepo("acme", "a26.5n6.01.tp1.bruno-tanguay", true)

	return avantLeRegistre(t, state, cohorte("a26", "5n6", "01",
		"Émilie Côté", "emilie-cote", "Jean-Luc Picard", "jlpicard",
		"Aminata Diallo", "aminata-d", "Bruno Tanguay", "btanguay"))
}

// ligneDInvitation est ce que la page reçoit de l'invitation d'un dépôt.
type ligneDInvitation struct {
	Invitation string `json:"invitation"`
	Invitable  bool   `json:"invitable"`
}

func (h *harnais) invitationsDuTravail() map[string]ligneDInvitation {
	h.t.Helper()
	var detail struct {
		Repos []struct {
			Name string `json:"name"`
			ligneDInvitation
		} `json:"repos"`
	}
	h.json(http.MethodGet, "/api/classrooms/a26.5n6.01/assignments/tp1", nil, &detail)
	lignes := map[string]ligneDInvitation{}
	for _, repo := range detail.Repos {
		lignes[repo.Name] = repo.ligneDInvitation
	}
	return lignes
}

func (h *harnais) inspecterLesAcces() {
	h.t.Helper()
	h.travail(http.MethodPost, "/api/classrooms/a26.5n6.01/assignments/tp1/access?refresh=1", nil)
}

// L'état de l'invitation ne se devine pas : tant que les accès n'ont pas été
// lus, la page ne prétend rien ; une fois lus, chaque dépôt dit où en est la
// personne qu'il vise, et seuls ceux qu'un envoi débloquerait le proposent.
func TestLEtatDeLInvitationParaitUneFoisLesAccesInspectes(t *testing.T) {
	h := travailAuxQuatreInvitations(t)

	for nom, ligne := range h.invitationsDuTravail() {
		if ligne.Invitation != "" || ligne.Invitable {
			t.Errorf("%s dit %+v avant toute inspection", nom, ligne)
		}
	}

	h.inspecterLesAcces()
	lignes := h.invitationsDuTravail()
	attendus := map[string]ligneDInvitation{
		"a26.5n6.01.tp1.emilie-cote":     {"acceptée", false},
		"a26.5n6.01.tp1.jean-luc-picard": {"en attente", false},
		"a26.5n6.01.tp1.aminata-diallo":  {"expirée", true},
		"a26.5n6.01.tp1.bruno-tanguay":   {"sans invitation", true},
	}
	for nom, attendu := range attendus {
		if lignes[nom] != attendu {
			t.Errorf("%s : %+v, attendu %+v", nom, lignes[nom], attendu)
		}
	}
}

// Le bouton d'une ligne fait partir ce qui manque : une première invitation à
// qui n'en a pas, au droit du groupe ; une neuve à la place d'une expirée, au
// droit qu'elle promettait. La ligne repasse « en attente ».
func TestLeBoutonDUneLigneEnvoieCeQuiManque(t *testing.T) {
	h := travailAuxQuatreInvitations(t)
	h.inspecterLesAcces()
	ancienne := h.State.Invitations["acme/a26.5n6.01.tp1.aminata-diallo"][0].ID

	var reponse struct {
		Message string `json:"message"`
	}
	h.json(http.MethodPost,
		"/api/classrooms/a26.5n6.01/assignments/tp1/repos/a26.5n6.01.tp1.bruno-tanguay/invitations",
		nil, &reponse)
	if reponse.Message != "Invitation envoyée à @btanguay (push)." {
		t.Errorf("message : %q", reponse.Message)
	}
	h.json(http.MethodPost,
		"/api/classrooms/a26.5n6.01/assignments/tp1/repos/a26.5n6.01.tp1.aminata-diallo/invitations",
		nil, &reponse)
	if reponse.Message != "Nouvelle invitation envoyée à @aminata-d (pull)." {
		t.Errorf("message : %q", reponse.Message)
	}

	if invitations := h.State.Invitations["acme/a26.5n6.01.tp1.bruno-tanguay"]; len(invitations) != 1 ||
		invitations[0].Login != "btanguay" || invitations[0].Permission != "push" {
		t.Errorf("Bruno n'a pas reçu sa première invitation : %+v", invitations)
	}
	if invitations := h.State.Invitations["acme/a26.5n6.01.tp1.aminata-diallo"]; len(invitations) != 1 ||
		invitations[0].ID == ancienne || invitations[0].Expired || invitations[0].Permission != "pull" {
		t.Errorf("l'invitation d'Aminata n'a pas été renouvelée : %+v", invitations)
	}

	// L'envoi a relu les accès des dépôts touchés : la page rouverte montre
	// l'invitation neuve sans qu'il faille réinspecter.
	lignes := h.invitationsDuTravail()
	for _, nom := range []string{"a26.5n6.01.tp1.bruno-tanguay", "a26.5n6.01.tp1.aminata-diallo"} {
		if lignes[nom] != (ligneDInvitation{"en attente", false}) {
			t.Errorf("après l'envoi, %s dit %+v", nom, lignes[nom])
		}
	}
}

// Une invitation encore valable ne se double pas : la ligne n'a rien à
// envoyer, et le serveur le dit plutôt que de faire partir un second courriel.
func TestUneInvitationValableNeSeDoublePas(t *testing.T) {
	h := travailAuxQuatreInvitations(t)
	attente := h.State.Invitations["acme/a26.5n6.01.tp1.jean-luc-picard"][0].ID
	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/a26.5n6.01/assignments/tp1/repos/a26.5n6.01.tp1.jean-luc-picard/invitations",
		nil)
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400 — %s", reponse.StatusCode, contenu)
	}
	if !strings.Contains(string(contenu), "Rien à envoyer") {
		t.Errorf("message : %s", contenu)
	}
	if invitations := h.State.Invitations["acme/a26.5n6.01.tp1.jean-luc-picard"]; len(invitations) != 1 ||
		invitations[0].ID != attente {
		t.Errorf("l'invitation valable a été touchée : %+v", invitations)
	}
}

// Le panneau d'accès renvoie une invitation désignée : une invitation qui
// n'existe plus — acceptée ou annulée entre-temps — ne se renvoie pas.
func TestRenvoyerUneInvitationDisparueEstRefuse(t *testing.T) {
	h := travailAuxQuatreInvitations(t)
	reponse, contenu := h.requete(http.MethodPost,
		"/api/orgs/acme/repos/a26.5n6.01.tp1.aminata-diallo/invitations/9999/resend", nil)
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400 — %s", reponse.StatusCode, contenu)
	}
	if !strings.Contains(string(contenu), "n'existe plus") {
		t.Errorf("message : %s", contenu)
	}
}

// L'envoi en série relit les accès, remplace l'invitation expirée et invite
// qui n'en a pas ; celle qui attend encore reste telle quelle.
func TestEnvoyerLesInvitationsManquantesDUnTravail(t *testing.T) {
	h := travailAuxQuatreInvitations(t)
	attente := h.State.Invitations["acme/a26.5n6.01.tp1.jean-luc-picard"][0].ID

	bilan := h.travail(http.MethodPost, "/api/classrooms/a26.5n6.01/assignments/tp1/invitations", nil)
	envois, _ := bilan["result"].([]any)
	depots := map[string]bool{}
	for _, brut := range envois {
		envoi, _ := brut.(map[string]any)
		if envoi["error"] != nil {
			t.Errorf("échec : %+v", envoi)
		}
		depots[envoi["repo"].(string)] = true
	}
	if len(depots) != 2 || !depots["a26.5n6.01.tp1.aminata-diallo"] ||
		!depots["a26.5n6.01.tp1.bruno-tanguay"] {
		t.Fatalf("envois : %+v", bilan)
	}

	if invitations := h.State.Invitations["acme/a26.5n6.01.tp1.aminata-diallo"]; len(invitations) != 1 ||
		invitations[0].Expired {
		t.Errorf("l'invitation d'Aminata n'a pas été renouvelée : %+v", invitations)
	}
	if invitations := h.State.Invitations["acme/a26.5n6.01.tp1.bruno-tanguay"]; len(invitations) != 1 ||
		invitations[0].Login != "btanguay" {
		t.Errorf("Bruno n'a pas reçu sa première invitation : %+v", invitations)
	}
	// Les lignes touchées disent l'invitation neuve, pas un dépôt qu'on
	// n'aurait jamais regardé.
	lignes := h.invitationsDuTravail()
	for _, nom := range []string{"a26.5n6.01.tp1.bruno-tanguay", "a26.5n6.01.tp1.aminata-diallo"} {
		if lignes[nom] != (ligneDInvitation{"en attente", false}) {
			t.Errorf("après l'envoi, %s dit %+v", nom, lignes[nom])
		}
	}
	if invitations := h.State.Invitations["acme/a26.5n6.01.tp1.jean-luc-picard"]; len(invitations) != 1 ||
		invitations[0].ID != attente {
		t.Errorf("l'invitation encore valable a été touchée : %+v", invitations)
	}
}
