package web

import (
	"net/http"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Renommer un travail, sans le déplacer. Un travail mal nommé — « tp1 » pour ce
// qui est devenu le projet final, une faute de frappe distribuée à trente
// personnes — n'avait jusqu'ici qu'une issue : le déplacer vers un autre groupe
// pour profiter du nom qu'on y choisit. C'est beaucoup demander pour corriger
// un mot.
//
// Le nom du travail est un niveau du nom de chaque dépôt : le corriger, c'est
// les renommer tous. GitHub garde une redirection depuis chaque ancien nom, si
// bien que les clones et les liens déjà distribués continuent de fonctionner.

// renameInput est ce que l'interface envoie pour renommer un travail.
type renameInput struct {
	// ID désigne le travail tel qu'il est encore nommé, Name le nom qu'il prend.
	ID   string `json:"id"`
	Name string `json:"name"`
}

// renommage compose le renommage demandé, sans rien écrire. Le nom rendu est
// celui que porteront les dépôts — la nomenclature le met en forme —, et non
// celui qui a été tapé : les deux diffèrent dès qu'on saisit « Projet final ».
func (s *Server) renommage(request *http.Request, body renameInput) (
	classroom.Classroom, string, []classroom.Move, error) {
	cours, err := s.place(request)
	if err != nil {
		return cours, "", nil, err
	}
	if strings.TrimSpace(body.ID) == "" {
		return cours, "", nil, valid.Errorf("Aucun travail à renommer.")
	}
	nom, err := naming.Fragment(body.Name, "Travail")
	if err != nil {
		return cours, "", nil, err
	}
	repos, _, err := s.repos(cours.Org, false)
	if err != nil {
		return cours, "", nil, err
	}
	lignes, err := classroom.PlanRenameAssignment(cours, body.ID, nom, repos)
	if err != nil {
		return cours, "", nil, err
	}
	return cours, nom, lignes, nil
}

// handleRenameAssignmentPreview montre le renommage sans rien écrire.
func (s *Server) handleRenameAssignmentPreview(writer http.ResponseWriter, request *http.Request) {
	var body renameInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	cours, nom, lignes, err := s.renommage(request, body)
	if err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"rows": lignes, "ready": len(lignes),
		"previous": cours.ShortName(body.ID),
		"name":     nom, "id": cours.AssignmentID(nom),
	})
}

// handleRenameAssignment renomme les dépôts du travail. Rien d'autre n'est à
// mettre à jour : un travail n'est pas une fiche enregistrée quelque part, il
// est l'ensemble de ses dépôts — les renommer suffit à le renommer.
func (s *Server) handleRenameAssignment(writer http.ResponseWriter, request *http.Request) {
	var body renameInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	cours, apres, lignes, err := s.renommage(request, body)
	if err != nil {
		fail(writer, err)
		return
	}

	avant := cours.ShortName(body.ID)
	label := "Renommage de « " + avant + " » en « " + apres + " »"
	job := s.jobs.Start("renommage", label, func(job *Job) (any, error) {
		renommes, echecs := 0, 0
		for index, ligne := range lignes {
			if job.Canceled() {
				break
			}
			if _, err := s.deps.Client.RenameRepo(
				cours.Org, ligne.Repo, ligne.Target); err != nil {
				echecs++
				job.Line(ligne.Repo+" : échec — "+err.Error(),
					map[string]string{"status": "échec"})
			} else {
				renommes++
				job.Line(ligne.Repo+" → "+ligne.Target,
					map[string]string{"status": "mis à jour"})
			}
			job.Progress(index+1, len(lignes), ligne.Repo)
		}
		s.forget(cours.Org)

		// Un renommage à moitié fait laisse deux travaux là où il n'y en avait
		// qu'un : le dire vaut mieux que de le taire.
		if echecs > 0 || job.Canceled() {
			job.Warn("Les dépôts restés en arrière portent encore « " + avant +
				" » : relancez le renommage pour les rattraper.")
		}
		return map[string]any{
			"renamed": renommes, "failed": echecs,
			"name": apres, "previous": avant, "id": cours.AssignmentID(apres),
		}, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}
