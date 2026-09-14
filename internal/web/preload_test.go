package web_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// ligneDeTravail est ce qu'une page montre d'un dépôt : ce que son historique
// a livré, et ce qu'on sait de ses accès.
type ligneDeTravail struct {
	Name    string `json:"name"`
	Seen    bool   `json:"seen"`
	Commits int    `json:"commits"`
	Access  *struct {
		Collaborators []string `json:"collaborators"`
	} `json:"access"`
}

// travailOuvert rend les dépôts d'un travail, tels que la page les recevrait.
func (h *harnais) travailOuvert(place, nom string) []ligneDeTravail {
	h.t.Helper()
	var detail struct {
		Repos []ligneDeTravail `json:"repos"`
	}
	h.json(http.MethodGet, "/api/classrooms/"+place+"/assignments/"+nom, nil, &detail)
	return detail.Repos
}

// attendreLaPreparation laisse au préchargement le temps de remplir la page.
func (h *harnais) attendreLaPreparation(place, nom string) []ligneDeTravail {
	h.t.Helper()
	limite := time.Now().Add(15 * time.Second)
	for time.Now().Before(limite) {
		lignes := h.travailOuvert(place, nom)
		if len(lignes) > 0 && lignes[0].Seen && lignes[0].Access != nil {
			return lignes
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("« %s » n'a pas été préparé", nom)
	return nil
}

// Ouvrir la liste des groupes lance, en arrière-plan, les deux lectures chères
// des cours de la session la plus récente : l'écran du travail les montre alors
// sans qu'on ait rien eu à demander.
func TestLesCoursDeLaSessionEnCoursSePreparentEnArrierePlan(t *testing.T) {
	state := fakegh.NewState()
	recent := state.AddRepo("acme", "h27.5n6.01.tp1.emilie-cote", true)
	recent.History = []string{"2027-02-05T14:00:00Z", "2027-02-01T09:00:00Z"}
	state.AddCollaborator("acme/h27.5n6.01.tp1.emilie-cote", "ecote", "push")

	ancien := state.AddRepo("acme", "a26.5n6.01.tp1.jean-luc-picard", true)
	ancien.History = []string{"2026-10-05T14:00:00Z"}
	state.AddCollaborator("acme/a26.5n6.01.tp1.jean-luc-picard", "jlpicard", "push")

	h := avantLeRegistre(t, state,
		cohorte("h27", "5n6", "01", "Émilie Côté", "ecote"),
		cohorte("a26", "5n6", "01", "Jean-Luc Picard", "jlpicard"))

	// Rien n'a encore été regardé : la page le dirait.
	if lignes := h.travailOuvert("h27.5n6.01", "tp1"); len(lignes) != 1 ||
		lignes[0].Seen || lignes[0].Access != nil {
		t.Fatalf("avant toute lecture : %+v", lignes)
	}

	// Lister les groupes suffit : c'est le moment où l'on choisit, et le
	// préchargement part pendant ce temps-là.
	h.json(http.MethodGet, "/api/classrooms", nil, nil)

	lignes := h.attendreLaPreparation("h27.5n6.01", "tp1")
	if lignes[0].Commits != 2 {
		t.Fatalf("commits = %d : l'historique devait être relevé", lignes[0].Commits)
	}
	if len(lignes[0].Access.Collaborators) != 1 ||
		lignes[0].Access.Collaborators[0] != "ecote" {
		t.Fatalf("accès = %+v", lignes[0].Access)
	}

	// La session passée ne bouge plus : la préparer coûterait cher pour un
	// écran que personne ne rouvre.
	anciennes := h.travailOuvert("a26.5n6.01", "tp1")
	if len(anciennes) != 1 || anciennes[0].Seen || anciennes[0].Access != nil {
		t.Fatalf("une session passée a été préparée : %+v", anciennes)
	}
}

// « Inspecter les accès » reste un geste qui va voir maintenant : il relit tout,
// même ce qu'un préchargement avait déjà mis en mémoire.
func TestInspecterLesAccesRelitCeQuIlSait(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", "h27.5n6.01.tp1.emilie-cote", true)
	h := avantLeRegistre(t, state, cohorte("h27", "5n6", "01", "Émilie Côté", "ecote"))

	bilan := h.travail(http.MethodPost,
		"/api/classrooms/h27.5n6.01/assignments/tp1/access?refresh=1", nil)
	if bilan["status"] != "terminé" {
		t.Fatalf("inspection : %+v", bilan)
	}

	// L'accès arrive après la première inspection : sans relire, l'écran
	// montrerait encore un dépôt sans personne dessus.
	state.AddCollaborator("acme/h27.5n6.01.tp1.emilie-cote", "ecote", "push")
	bilan = h.travail(http.MethodPost,
		"/api/classrooms/h27.5n6.01/assignments/tp1/access?refresh=1", nil)
	resultats, _ := bilan["result"].([]any)
	if len(resultats) != 1 {
		t.Fatalf("résultats : %+v", bilan["result"])
	}
	premier, _ := resultats[0].(map[string]any)
	comptes, _ := premier["collaborators"].([]any)
	if len(comptes) != 1 || comptes[0] != "ecote" {
		t.Fatalf("collaborateurs = %+v", premier)
	}

	// Et ce qu'il a lu reste : la page rouverte le montre sans rien redemander.
	lignes := h.travailOuvert("h27.5n6.01", "tp1")
	if len(lignes) != 1 || lignes[0].Access == nil ||
		len(lignes[0].Access.Collaborators) != 1 {
		t.Fatalf("accès mémorisés : %+v", lignes)
	}
}
