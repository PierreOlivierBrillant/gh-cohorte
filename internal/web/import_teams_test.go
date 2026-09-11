package web_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// Reprendre un travail fait en équipe : ce qui suit le préfixe nomme une
// équipe, et ce sont les accès du dépôt qui disent qui en est.

// equipeOrg monte une organisation où un travail d'équipe a été fait sans
// l'outil : deux dépôts, deux ou trois personnes dans chacun.
func equipeOrg() *fakegh.State {
	state := fakegh.NewState()
	state.Users["walid"] = "Walid Hakkani"
	state.Users["etienne-l"] = "Étienne Lyonnais"
	for nom, membres := range map[string][]string{
		"projet-alpha": {"emilie-cote", "jlpicard"},
		"projet-beta":  {"aminata-d", "walid", "etienne-l"},
	} {
		state.AddRepo("acme", nom, true)
		for _, compte := range membres {
			state.AddCollaborator("acme/"+nom, compte, "push")
		}
		// L'enseignant a accès à tout : il ne fait partie d'aucune équipe.
		state.AddCollaborator("acme/"+nom, "prof", "admin")
	}
	return state
}

// repriseEquipe est ce que l'aperçu d'une reprise en équipe rend.
type repriseEquipe struct {
	TeamWork bool   `json:"team_work"`
	Prefix   string `json:"prefix"`
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	Teams    []struct {
		Short   string   `json:"short"`
		Name    string   `json:"name"`
		Repo    string   `json:"repo"`
		Target  string   `json:"target"`
		Members []string `json:"members"`
		Exists  bool     `json:"exists"`
	} `json:"teams"`
	Moves []struct {
		Repo   string `json:"repo"`
		Target string `json:"target"`
	} `json:"moves"`
	Students []struct {
		FullName string `json:"full_name"`
		Username string `json:"username"`
	} `json:"students"`
	Silent []string `json:"silent"`
}

// apercuEquipe demande l'aperçu d'une reprise en équipe.
func (h *harnais) apercuEquipe(corps map[string]any) repriseEquipe {
	h.t.Helper()
	corps["teams"] = true
	var vue repriseEquipe
	h.json(http.MethodPost, "/api/orgs/acme/import/preview", corps, &vue)
	return vue
}

// L'aperçu lit l'équipe dans le nom, et ses membres dans les accès.
func TestRepriseEnEquipeLitLesMembresDansLesAcces(t *testing.T) {
	h := nouveau(t, equipeOrg())
	vue := h.apercuEquipe(map[string]any{
		"prefix": "projet", "name": "projet", "scope": "a26.5n6.01",
	})

	if !vue.TeamWork || len(vue.Teams) != 2 {
		t.Fatalf("deux équipes attendues : %+v", vue)
	}
	trouve := map[string][]string{}
	cibles := map[string]string{}
	for _, equipe := range vue.Teams {
		trouve[equipe.Short] = equipe.Members
		cibles[equipe.Short] = equipe.Target
		if equipe.Exists {
			t.Fatalf("aucune équipe n'existe encore : %+v", equipe)
		}
	}
	if strings.Join(trouve["alpha"], ",") != "emilie-cote,jlpicard" {
		t.Fatalf("membres d'alpha = %v", trouve["alpha"])
	}
	if strings.Join(trouve["beta"], ",") != "aminata-d,etienne-l,walid" {
		t.Fatalf("membres de beta = %v", trouve["beta"])
	}
	// L'enseignant a accès à tout : il n'est d'aucune équipe.
	for court, membres := range trouve {
		for _, compte := range membres {
			if compte == "prof" {
				t.Fatalf("l'enseignant ne devrait pas être dans « %s »", court)
			}
		}
	}
	// Le dépôt prend le nom de son équipe au dernier niveau.
	if cibles["alpha"] != "a26.5n6.01.projet.alpha" {
		t.Fatalf("cible d'alpha = %s", cibles["alpha"])
	}
	if len(vue.Moves) != 2 {
		t.Fatalf("deux renommages attendus : %+v", vue.Moves)
	}
	// Les membres deviennent les étudiants du groupe, nommés quand
	// l'organisation les connaît.
	if len(vue.Students) != 5 {
		t.Fatalf("cinq personnes attendues : %+v", vue.Students)
	}
}

// La reprise renomme les dépôts, crée les équipes, et leur partage le leur.
func TestRepriseEnEquipeReconstitueLesEquipes(t *testing.T) {
	h := nouveau(t, equipeOrg())
	bilan := h.travail(http.MethodPost, "/api/orgs/acme/import", map[string]any{
		"prefix": "projet", "name": "projet", "scope": "a26.5n6.01", "teams": true,
	})
	if bilan["status"] != "terminé" {
		t.Fatalf("reprise en échec : %v", bilan)
	}
	resultat, _ := bilan["result"].(map[string]any)
	if resultat == nil || resultat["renamed"].(float64) != 2 ||
		resultat["teams"].(float64) != 2 || resultat["shared"].(float64) != 2 {
		t.Fatalf("bilan inattendu : %v", resultat)
	}

	noms := h.depots()
	sort.Strings(noms)
	attendus := []string{"a26.5n6.01.projet.alpha", "a26.5n6.01.projet.beta"}
	if strings.Join(noms, ",") != strings.Join(attendus, ",") {
		t.Fatalf("dépôts renommés inattendus : %v", noms)
	}

	// Les équipes portent la place du groupe, et leurs membres viennent des
	// accès des dépôts.
	membres := h.State.TeamMembers("acme", fakegh.TeamSlug("a26.5n6.01.alpha"))
	if strings.Join(membres, ",") != "emilie-cote,jlpicard" {
		t.Fatalf("membres d'alpha = %v", membres)
	}
	// Chaque équipe a reçu son dépôt.
	partages := h.State.TeamRepoNames("acme", fakegh.TeamSlug("a26.5n6.01.beta"))
	if strings.Join(partages, ",") != "acme/a26.5n6.01.projet.beta" {
		t.Fatalf("dépôt partagé avec beta = %v", partages)
	}

	// Et le travail se relit comme s'il avait toujours été distribué ainsi.
	var fiche struct {
		Teams       int `json:"teams"`
		Assignments []struct {
			Name  string `json:"name"`
			Kind  string `json:"kind"`
			Teams int    `json:"teams"`
		} `json:"assignments"`
	}
	h.json(http.MethodGet, "/api/classrooms/a26.5n6.01?refresh=1", nil, &fiche)
	if fiche.Teams != 2 || len(fiche.Assignments) != 1 {
		t.Fatalf("le groupe devrait porter deux équipes et un travail : %+v", fiche)
	}
	if fiche.Assignments[0].Kind != "équipe" || fiche.Assignments[0].Teams != 2 {
		t.Fatalf("le travail devrait être d'équipe : %+v", fiche.Assignments[0])
	}
}

// Une équipe déjà là n'est pas recréée : la reprise la complète.
func TestRepriseEnEquipeCompleteUneEquipeDejaLa(t *testing.T) {
	state := equipeOrg()
	state.AddTeam("acme", "a26.5n6.01.alpha", "emilie-cote")
	h := nouveau(t, state)

	vue := h.apercuEquipe(map[string]any{
		"prefix": "projet", "scope": "a26.5n6.01",
	})
	for _, equipe := range vue.Teams {
		if equipe.Short != "alpha" {
			continue
		}
		if !equipe.Exists {
			t.Fatalf("alpha existe déjà : %+v", equipe)
		}
		if strings.Join(equipe.Members, ",") != "emilie-cote,jlpicard" {
			t.Fatalf("membres d'alpha = %v", equipe.Members)
		}
	}

	h.travail(http.MethodPost, "/api/orgs/acme/import", map[string]any{
		"prefix": "projet", "scope": "a26.5n6.01", "teams": true,
	})
	if noms := h.State.TeamNames("acme"); len(noms) != 4 {
		// « enseignants » et « direction » du décor, plus alpha et beta.
		t.Fatalf("une seule équipe devait naître : %v", noms)
	}
}

// Un dépôt dont les accès ne disent rien donne une équipe vide : le dépôt lui
// appartient, mais il reste à dire qui en est.
func TestRepriseEnEquipeSignaleUnDepotSansAcces(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "projet-alpha", true)
	h := nouveau(t, state)

	vue := h.apercuEquipe(map[string]any{"prefix": "projet", "scope": "a26.5n6.01"})
	if len(vue.Silent) != 1 || vue.Silent[0] != "projet-alpha" {
		t.Fatalf("le dépôt muet devrait être signalé : %+v", vue.Silent)
	}
	if len(vue.Teams) != 1 || len(vue.Teams[0].Members) != 0 {
		t.Fatalf("l'équipe devrait être vide : %+v", vue.Teams)
	}
}

// Deux dépôts qui donneraient la même équipe arrêtent la reprise : « alpha » et
// « Alpha » ne font qu'un nom sur GitHub.
func TestRepriseEnEquipeRefuseDeuxDepotsPourUneEquipe(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "projet-alpha", true)
	state.AddRepo("acme", "projet-Alpha", true)
	h := nouveau(t, state)

	reponse, contenu := h.requete(http.MethodPost, "/api/orgs/acme/import/preview",
		map[string]any{"prefix": "projet", "scope": "a26.5n6.01", "teams": true})
	if reponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("statut attendu 400, reçu %d — %s", reponse.StatusCode, contenu)
	}
	if !strings.Contains(string(contenu), "même équipe") {
		t.Fatalf("message peu explicite : %s", contenu)
	}
}

// La sélection vaut aussi pour une reprise en équipe : un dépôt d'essai reste
// où il est.
func TestRepriseEnEquipeRespecteLaSelection(t *testing.T) {
	h := nouveau(t, equipeOrg())
	vue := h.apercuEquipe(map[string]any{
		"prefix": "projet", "scope": "a26.5n6.01",
		"only": []string{"projet-alpha"},
	})
	if len(vue.Teams) != 1 || vue.Teams[0].Short != "alpha" {
		t.Fatalf("seule alpha devait être reprise : %+v", vue.Teams)
	}
	if len(vue.Moves) != 1 {
		t.Fatalf("un seul renommage attendu : %+v", vue.Moves)
	}
}
