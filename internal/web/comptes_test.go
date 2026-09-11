package web_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// Une personne, plusieurs comptes.
//
// Un étudiant travaille parfois sous deux comptes. Rien ne le déduit de son
// nom — deux étudiants peuvent s'appeler pareil, et les confondre leur donnerait
// un dépôt pour deux : c'est une décision, prise et écrite.

// rattacher déclare qu'un compte est celui d'une personne déjà inscrite.
func (h *harnais) rattacher(place, personne, compte string) (*http.Response, []byte) {
	h.t.Helper()
	return h.requete(http.MethodPost, "/api/classrooms/"+place+"/students/accounts",
		map[string]any{"username": personne, "account": compte})
}

func TestDeuxComptesNeFontQuUnePersonne(t *testing.T) {
	state := fakegh.NewState()
	state.Users["ecote"] = "Émilie Côté"
	h := nouveau(t, state)
	place := h.groupe("a26", "5n6", "01",
		"Émilie Côté", "emilie-cote", "Jean-Luc Picard", "jlpicard")

	if reponse, contenu := h.rattacher(place, "emilie-cote", "ecote"); reponse.StatusCode >= 300 {
		t.Fatalf("rattachement refusé : %d — %s", reponse.StatusCode, contenu)
	}

	// Un seul dépôt : c'est son nom qui le nomme, pas son compte.
	h.travail(http.MethodPost, "/api/classrooms/"+place+"/assignments",
		map[string]any{"name": "tp1"})
	noms := h.depots()
	sort.Strings(noms)
	attendus := []string{"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.jean-luc-picard"}
	if strings.Join(noms, ",") != strings.Join(attendus, ",") {
		t.Fatalf("un dépôt par personne attendu : %v", noms)
	}

	// Mais elle y est invitée sous ses deux comptes.
	invites := map[string]bool{}
	for _, invitation := range h.State.Invitations["acme/a26.5n6.01.tp1.emilie-cote"] {
		invites[invitation.Login] = true
	}
	if !invites["emilie-cote"] || !invites["ecote"] {
		t.Fatalf("les deux comptes devraient être invités : %v", invites)
	}
}

// Le rattachement retire le compte de la liste s'il y était inscrit à part :
// la même personne n'y figure pas deux fois.
func TestRattacherRetireLInscriptionSeparee(t *testing.T) {
	state := fakegh.NewState()
	state.Users["ecote"] = ""
	h := nouveau(t, state)
	place := h.groupe("a26", "5n6", "01",
		"Émilie Côté", "emilie-cote", "", "ecote")

	h.rattacher(place, "emilie-cote", "ecote")

	var fiche struct {
		Students []struct {
			FullName string   `json:"full_name"`
			Username string   `json:"username"`
			Also     []string `json:"also"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+place, nil, &fiche)
	if len(fiche.Students) != 1 {
		t.Fatalf("une seule personne attendue : %+v", fiche.Students)
	}
	if strings.Join(fiche.Students[0].Also, ",") != "ecote" {
		t.Fatalf("le second compte devrait être rattaché : %+v", fiche.Students[0])
	}
}

// Rien ne se déduit du nom : deux homonymes restent deux personnes, et la
// préparation les refuse — c'est ce qui les empêche de partager un dépôt.
func TestDeuxHomonymesRestentDeuxPersonnes(t *testing.T) {
	h := nouveau(t, nil)
	place := h.groupe("a26", "5n6", "01",
		"Jean Tremblay", "jtremblay", "Jean Tremblay", "jean-t")

	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+place+"/assignments/preview", map[string]any{"name": "tp1"})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut %d, attendu 400 — %s", reponse.StatusCode, contenu)
	}
	if !strings.Contains(string(contenu), "Jean Tremblay") {
		t.Fatalf("message peu explicite : %s", contenu)
	}
}

// Le dépôt arrivé sous l'un de ses comptes est le sien sous l'autre aussi.
func TestUnDepotSousLAutreCompteResteLeSien(t *testing.T) {
	state := fakegh.NewState()
	state.Users["ecote"] = "Émilie Côté"
	state.AddRepo("acme", "a26.5n6.01.tp1.ecote", true)
	h := nouveau(t, state)
	place := h.groupe("a26", "5n6", "01", "Émilie Côté", "emilie-cote")
	h.rattacher(place, "emilie-cote", "ecote")

	var liste struct {
		Students []struct {
			Username    string   `json:"username"`
			Accounts    []string `json:"accounts"`
			Assignments []struct {
				Repo string `json:"repo"`
			} `json:"assignments"`
		} `json:"students"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+place+"/students?refresh=1", nil, &liste)
	if len(liste.Students) != 1 {
		t.Fatalf("une seule personne attendue : %+v", liste.Students)
	}
	if strings.Join(liste.Students[0].Accounts, ",") != "emilie-cote,ecote" {
		t.Fatalf("ses deux comptes devraient être rendus : %+v", liste.Students[0])
	}
	if len(liste.Students[0].Assignments) != 1 {
		t.Fatalf("le dépôt sous son autre compte est le sien : %+v", liste.Students[0])
	}
}

// Une équipe nomme ses membres, y compris ceux que seul le registre connaît.
func TestUneEquipeNommeSesMembres(t *testing.T) {
	h, place := groupeAvecEquipes(t)
	var liste listeEquipes
	h.json(http.MethodGet, "/api/classrooms/"+place+"/teams", nil, &liste)
	for _, equipe := range liste.Teams {
		if equipe.Short != "eq1" {
			continue
		}
		noms := make([]string, 0, len(equipe.People))
		for _, personne := range equipe.People {
			noms = append(noms, personne.FullName)
		}
		sort.Strings(noms)
		if strings.Join(noms, ",") != "Jean-Luc Picard,Émilie Côté" {
			t.Fatalf("les membres devraient être nommés : %v", noms)
		}
	}
}
