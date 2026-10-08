package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/PierreOlivierBrillant/gh-nestor/internal/broadcast"
	"github.com/PierreOlivierBrillant/gh-nestor/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-nestor/internal/identity"
	"github.com/PierreOlivierBrillant/gh-nestor/internal/valid"
)

// Déposer un fichier dans tous les dépôts d'un travail, depuis le navigateur.
// Ce qui décide vit dans « broadcast » : les mêmes champs, les mêmes refus et
// le même sort pour un fichier déjà présent qu'au terminal.

// demandeDeDepot est ce que la page envoie.
type demandeDeDepot struct {
	// Path est le chemin du fichier sur la machine ; Content ses octets quand
	// il a été déposé dans la page, qui n'en donne alors que le nom (Name).
	Path    string `json:"path"`
	Content []byte `json:"content"`
	Name    string `json:"name"`
	// Target est le chemin dans le dépôt ; vide, c'est le nom du fichier.
	Target    string `json:"target"`
	Message   string `json:"message"`
	Raw       bool   `json:"raw"`
	Overwrite bool   `json:"overwrite"`
	DryRun    bool   `json:"dry_run"`
}

// planDeDepot lit la demande et remplit le gabarit pour chaque dépôt du
// travail désigné par l'adresse.
func (s *Server) planDeDepot(request *http.Request) (
	classroom.Classroom, string, demandeDeDepot, *broadcast.Plan, error) {
	var demande demandeDeDepot
	cours, id, repos, err := s.assignmentOf(request)
	if err != nil {
		return cours, id, demande, nil, err
	}
	if err := decode(request, &demande); err != nil {
		return cours, id, demande, nil, err
	}
	source := broadcast.Source{Name: strings.TrimSpace(demande.Name), Content: demande.Content}
	if len(demande.Content) == 0 {
		if strings.TrimSpace(demande.Path) == "" {
			return cours, id, demande, nil, valid.Errorf("Choisissez le fichier à déposer.")
		}
		if source, err = broadcast.Load(demande.Path); err != nil {
			return cours, id, demande, nil, err
		}
	}
	cible := strings.TrimSpace(demande.Target)
	if cible == "" {
		cible = source.Name
	}
	equipes, err := s.teamsIn(cours)
	if err != nil {
		return cours, id, demande, nil, err
	}
	depots := cours.Repos(id, repos)
	if len(depots) == 0 {
		return cours, id, demande, nil,
			valid.Errorf("Aucun dépôt pour le travail « %s ».", cours.ShortName(id))
	}
	contexte, destinataires := broadcast.ForClassroom(cours, id, equipes, depots, time.Now())
	plan, err := broadcast.Prepare(broadcast.Request{
		Content: source.Content, Path: cible, Message: demande.Message, Raw: demande.Raw,
	}, contexte, destinataires)
	return cours, id, demande, plan, err
}

// handleBroadcastFields rend les champs du gabarit, pour que la page les
// nomme comme le terminal.
func (s *Server) handleBroadcastFields(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, broadcast.Fields)
}

// handleFilePreview montre, sans rien demander à GitHub, ce que le premier
// dépôt recevrait et ce que le gabarit risque de mal faire.
func (s *Server) handleFilePreview(writer http.ResponseWriter, request *http.Request) {
	_, _, _, plan, err := s.planDeDepot(request)
	if err != nil {
		fail(writer, err)
		return
	}
	premier := plan.Items[0]
	apercu := map[string]any{
		"repo": premier.Repo, "recipient": premier.Label(), "path": premier.Path,
		"message": premier.Message,
	}
	if broadcast.IsText(premier.Content) {
		apercu["content"] = string(premier.Content)
	}
	var vide map[string]any
	for _, item := range plan.Items {
		if len(item.Empty) > 0 {
			vide = map[string]any{"repo": item.Repo, "fields": item.Empty}
			break
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"count": len(plan.Items), "templated": plan.Templated,
		"used": plan.Used, "unknown": plan.Unknown,
		"incomplete": plan.Incomplete(), "empty_example": vide,
		"sample": apercu,
	})
}

// handleFilePush dépose le fichier dans chaque dépôt du travail — ou, en
// simulation, dit ce que chacun deviendrait sans rien y écrire.
func (s *Server) handleFilePush(writer http.ResponseWriter, request *http.Request) {
	cours, id, demande, plan, err := s.planDeDepot(request)
	if err != nil {
		fail(writer, err)
		return
	}
	titre := "Dépôt de « " + plan.Items[0].Path + " » dans « " + cours.ShortName(id) + " »"
	if demande.DryRun {
		titre = "Simulation — " + titre
	}
	job := s.jobs.Start("fichier", titre, func(job *Job) (any, error) {
		resultats := broadcast.Run(s.deps.Client, cours.Org, plan, broadcast.Options{
			Overwrite: demande.Overwrite, DryRun: demande.DryRun,
			OnResult: func(done, total int, result broadcast.Result) {
				job.Line(result.Repo+" : "+result.Summary(),
					map[string]string{"status": string(result.Status)})
				job.Progress(done, total, result.Repo)
			},
		})
		// L'historique des dépôts écrits est relu aussitôt : sans lui, leur
		// dernier envoi serait celui du fichier qu'on vient d'y déposer.
		if ecrits := broadcast.Written(resultats); len(ecrits) > 0 && !demande.DryRun {
			s.resolver(cours.Org).Handins(cours.Org, ecrits, identity.Refresh,
				func(done, total int, repo string) { job.Progress(done, total, repo) })
		}
		return resultats, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}
