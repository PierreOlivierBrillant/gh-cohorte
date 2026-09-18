package web_test

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/runner"
)

// Inscrire une cohorte telle que le collège l'exporte.
//
// Un export de Léa ne porte aucun compte GitHub : un numéro d'étudiant, un nom,
// un code permanent. L'interface n'en retenait que les personnes déjà pourvues
// d'un compte — c'est-à-dire aucune — et refusait la liste entière : « Aucun
// étudiant dans la liste fournie », devant un fichier de vingt-trois noms. La
// lecture, elle, s'était bien passée ; c'est la façade qui jetait tout.

// omnivox écrit un export de Léa octet par octet : Windows-1252, séparateur
// « ; », champs blindés et fins de ligne CRLF. Jamais un vrai fichier dans le
// dépôt — il porterait des noms d'étudiants.
func omnivox(lignes ...string) []byte {
	var octets []byte
	for _, ligne := range lignes {
		for _, caractere := range ligne {
			if caractere < 0x100 {
				octets = append(octets, byte(caractere))
				continue
			}
			octets = append(octets, '?')
		}
		octets = append(octets, '\r', '\n')
	}
	return octets
}

var exportDeLea = omnivox(
	"Numéro d'étudiant;Nom de l'étudiant;Prénom;Code permanent",
	`="1680229";="Adam-Larocque";="Laurent";="ADAL01010101"`,
	`="1256639";="Rolleston-Chase";="Patrick";="ROLP02020202"`,
	`="2100123";="Côté";="Émilie";="COTE03030303"`,
)

// La lecture rend les personnes, et le dit, même sans un seul compte.
func TestLaLectureDUnExportDeLeaRendLesPersonnes(t *testing.T) {
	h := nouveau(t, fakegh.NewState())

	var lue struct {
		People []struct {
			FullName  string `json:"full_name"`
			Username  string `json:"username"`
			StudentID string `json:"student_id"`
		} `json:"people"`
		Issues []any `json:"issues"`
	}
	h.json(http.MethodPost, "/api/roster/parse",
		map[string]any{"content": exportDeLea}, &lue)

	if len(lue.Issues) != 0 {
		t.Fatalf("rien n'est fautif dans cet export : %v", lue.Issues)
	}
	if len(lue.People) != 3 {
		t.Fatalf("%d personne(s) rendues, 3 attendues : %+v", len(lue.People), lue.People)
	}
	if lue.People[0].FullName != "Laurent Adam-Larocque" ||
		lue.People[0].StudentID != "1680229" || lue.People[0].Username != "" {
		t.Errorf("première personne : %+v", lue.People[0])
	}
}

// Et le groupe les garde : c'est le geste de septembre.
func TestUnExportDeLeaInscritLaCohorteEntiere(t *testing.T) {
	h := nouveau(t, fakegh.NewState())
	place := h.groupe("a26", "5n6", "1010", "Émilie Côté", "ecote")

	var lue struct {
		People []map[string]any `json:"people"`
	}
	h.json(http.MethodPost, "/api/roster/parse",
		map[string]any{"content": exportDeLea}, &lue)

	reponse, contenu := h.requete(http.MethodPost,
		"/api/classrooms/"+place+"/students", map[string]any{"people": lue.People})
	if reponse.StatusCode >= 300 {
		t.Fatalf("inscription refusée : %d — %s", reponse.StatusCode, contenu)
	}

	var liste struct {
		Students []struct {
			FullName string `json:"full_name"`
		} `json:"students"`
		Total int `json:"total"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+place+"/students", nil, &liste)
	if liste.Total != 3 {
		t.Fatalf("%d étudiant(s) dans le groupe, 3 attendus : %+v",
			liste.Total, liste.Students)
	}
}

// Et la distribution suit : un dépôt par personne, nommé par son nom puisque
// c'est lui que la nomenclature porte — et le bilan dit sans détour que
// personne n'y a été invité, faute de compte. Une case vide se lirait comme un
// oubli d'affichage ; c'est un fait, et celui dont dépend l'accès.
func TestUnDepotEstCreePourQuiNAPasEncoreDeCompte(t *testing.T) {
	state := fakegh.NewState()
	state.Users["ecote"] = "Émilie Côté"
	state.Users["ladamlarocque"] = "Laurent Adam-Larocque"
	h := nouveau(t, state)
	place := h.groupe("a26", "5n6", "1010", "Émilie Côté", "ecote")

	var lue struct {
		People []map[string]any `json:"people"`
	}
	h.json(http.MethodPost, "/api/roster/parse",
		map[string]any{"content": exportDeLea}, &lue)
	h.json(http.MethodPost, "/api/classrooms/"+place+"/students",
		map[string]any{"people": lue.People}, nil)

	// Laurent a créé son compte depuis : on le lui rattache, et son matricule
	// est tout ce qu'on a pour le désigner.
	if reponse, contenu := h.rattacher(place, "1680229", "ladamlarocque"); reponse.StatusCode >= 300 {
		t.Fatalf("rattachement par matricule refusé : %d — %s", reponse.StatusCode, contenu)
	}

	rapport := h.travail(http.MethodPost, "/api/classrooms/"+place+"/assignments",
		map[string]any{"name": "tp1"})
	brut, err := json.Marshal(rapport)
	if err != nil {
		t.Fatalf("bilan illisible : %v", err)
	}
	bilan := string(brut)

	noms := h.depots()
	sort.Strings(noms)
	attendus := []string{
		"a26.5n6.1010.tp1.emilie-cote",
		"a26.5n6.1010.tp1.laurent-adam-larocque",
		"a26.5n6.1010.tp1.patrick-rolleston-chase",
	}
	if strings.Join(noms, ",") != strings.Join(attendus, ",") {
		t.Fatalf("un dépôt par personne attendu :\n  %v\n  %v", noms, attendus)
	}
	// Laurent, à qui l'on vient de rattacher un compte, est invité sur le sien.
	invites := h.State.Invitations["acme/a26.5n6.1010.tp1.laurent-adam-larocque"]
	if len(invites) != 1 || !strings.EqualFold(invites[0].Login, "ladamlarocque") {
		t.Errorf("Laurent devrait être invité sous son compte : %+v", invites)
	}
	// Patrick n'en a toujours pas : son dépôt existe, et personne n'y est
	// invité. C'est un fait, pas un échec.
	if invites := h.State.Invitations["acme/a26.5n6.1010.tp1.patrick-rolleston-chase"]; len(invites) != 0 {
		t.Errorf("personne n'était à inviter : %+v", invites)
	}
	// Et le bilan le dit, plutôt que de laisser la case vide : une case vide se
	// lit comme un oubli d'affichage, alors que c'est le fait dont dépend
	// l'accès au dépôt.
	if !strings.Contains(bilan, runner.CollaboratorUnknown) {
		t.Errorf("le bilan ne dit pas que personne n'était à inviter : %s", bilan)
	}
}
