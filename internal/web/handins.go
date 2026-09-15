package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Une date cible, et ce que les dépôts en font.
//
// Deux choses se passent ici, et elles ne se ressemblent pas. Fixer une date
// n'écrit rien sur GitHub : c'est une ligne du fichier des groupes, comme la
// liste des étudiants. Relever les historiques, en revanche, coûte deux
// requêtes par dépôt — un travail distribué à trente personnes en demande
// soixante. C'est donc un travail de fond, comme l'inspection des accès, et son
// résultat est mémorisé : l'écran qu'on rouvre montre ses pastilles sans rien
// redemander.

// deadlineInput est la date cible qu'on fixe, change ou retire. Une valeur vide
// retire l'échéance : c'est la même décision prise dans l'autre sens.
type deadlineInput struct {
	Due string `json:"due"`
}

// handleSetDeadline fixe la date cible d'un travail. Elle monte au registre de
// l'organisation, où vivent déjà les noms que les dépôts ne disent pas : la
// date vaut alors pour l'équipe entière, et suit d'un poste à l'autre.
func (s *Server) handleSetDeadline(writer http.ResponseWriter, request *http.Request) {
	cours, err := s.place(request)
	if err != nil {
		fail(writer, err)
		return
	}
	nom := strings.TrimSpace(request.PathValue("name"))
	if nom == "" {
		fail(writer, valid.Errorf("Travail inconnu."))
		return
	}
	var body deadlineInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	lignes, err := cours.SetDue(nom, body.Due)
	if err != nil {
		fail(writer, err)
		return
	}
	set, err := s.registryOf(cours.Org).Apply(echeances(lignes))
	if err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"name": cours.ShortName(nom), "due": set.Due(cours.AssignmentID(nom)),
	})
}

// remisesConnues rend ce qu'on sait déjà des historiques, sans rien demander à
// GitHub. Les dépôts qu'on n'a pas encore relevés sont simplement absents : un
// écran doit pouvoir distinguer « rien remis » de « pas encore regardé ».
func (s *Server) remisesConnues(org string, noms []string) map[string]groups.Handin {
	if len(noms) == 0 {
		return nil
	}
	return s.resolver(org).Handins(org, noms, identity.Cached, nil)
}

// jusqua lit ce que l'adresse demande : « refresh=1 » oublie ce qu'on savait,
// sans quoi le relevé se contente d'aller chercher ce qui manque.
func jusqua(request *http.Request) identity.Reading {
	if request.URL.Query().Get("refresh") == "1" {
		return identity.Refresh
	}
	return identity.Fetch
}

// nomsDeDepots rend les noms d'une liste de dépôts.
func nomsDeDepots(depots []groups.Repo) []string {
	noms := make([]string, 0, len(depots))
	for _, depot := range depots {
		noms = append(noms, depot.Name)
	}
	return noms
}

// reposDuGroupe rassemble les dépôts de tous les travaux d'un groupe.
func reposDuGroupe(cours classroom.Classroom, repos []groups.RepoInfo,
	travaux []classroom.Assignment) []string {
	noms := make([]string, 0, len(repos))
	for _, travail := range travaux {
		noms = append(noms, nomsDeDepots(cours.Repos(travail.ID, repos))...)
	}
	return noms
}

// releve compose le travail de fond qui lit les historiques. Il sert aux deux
// portées — un travail, ou le groupe entier — parce que c'est le même geste :
// seule la liste des dépôts change.
func (s *Server) releve(cours classroom.Classroom, titre string, noms []string,
	jusqua identity.Reading, resultat func(map[string]groups.Handin) any) *Job {
	return s.jobs.Start("remises", titre, func(job *Job) (any, error) {
		remises := s.resolver(cours.Org).Handins(cours.Org, noms, jusqua,
			func(done, total int, repo string) {
				job.Progress(done, total, repo)
			})
		if job.Canceled() {
			return resultat(remises), nil
		}
		// Un dépôt illisible — un jeton sans droit dessus — n'est pas relevé.
		// Le taire ferait croire qu'il est vide.
		if manquants := len(noms) - len(remises); manquants > 0 {
			job.Warn(fmt.Sprintf("%d dépôt(s) n'ont pas pu être lus : leur historique "+
				"reste inconnu.", manquants))
		}
		s.pourquoiRien(cours, remises, job)
		return resultat(remises), nil
	})
}

// pourquoiRien va chercher les accès des dépôts qui n'ont rien reçu.
//
// Un dépôt vide pose une question de plus que les autres : la personne a-t-elle
// seulement accepté son invitation ? Sans réponse, on lui reprocherait un
// silence qu'elle n'a pas choisi. Ceux qui ont reçu quelque chose n'en ont pas
// besoin — la question ne se pose plus — et leurs accès coûteraient deux
// requêtes pour rien.
func (s *Server) pourquoiRien(cours classroom.Classroom,
	remises map[string]groups.Handin, job *Job) {
	muets := make([]string, 0, len(remises))
	for repo, remise := range remises {
		if cours.HandedIn(remise) == "" {
			muets = append(muets, repo)
		}
	}
	if len(muets) == 0 || job.Canceled() {
		return
	}
	s.resolver(cours.Org).Accesses(cours.Org, muets, identity.Fetch,
		func(done, total int, repo string) { job.Progress(done, total, repo) })
}

// handleAssignmentHandins relève les historiques des dépôts d'un travail et
// confronte chacun à la date cible et aux personnes qu'il vise.
func (s *Server) handleAssignmentHandins(writer http.ResponseWriter, request *http.Request) {
	cours, id, repos, err := s.assignmentOf(request)
	if err != nil {
		fail(writer, err)
		return
	}
	equipes, err := s.teamsIn(cours)
	if err != nil {
		fail(writer, err)
		return
	}
	depots := cours.Repos(id, repos)
	if len(depots) == 0 {
		fail(writer, valid.Errorf("Aucun dépôt pour le travail « %s ».", cours.ShortName(id)))
		return
	}
	job := s.releve(cours, "Remises de « "+cours.ShortName(id)+" »",
		nomsDeDepots(depots), jusqua(request),
		func(remises map[string]groups.Handin) any {
			return cours.Reviews(id, repos, equipes, remises)
		})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// handleClassroomHandins relève les historiques de tous les dépôts du groupe et
// rend les travaux complétés : c'est ce qui allume les pastilles de la liste.
func (s *Server) handleClassroomHandins(writer http.ResponseWriter, request *http.Request) {
	cours, err := s.place(request)
	if err != nil {
		fail(writer, err)
		return
	}
	repos, _, err := s.repos(cours.Org, false)
	if err != nil {
		fail(writer, err)
		return
	}
	cours = s.enrichi(cours, repos)
	equipes, err := s.teamsIn(cours)
	if err != nil {
		fail(writer, err)
		return
	}
	travaux := cours.Assignments(repos, equipes)
	noms := reposDuGroupe(cours, repos, travaux)
	if len(noms) == 0 {
		fail(writer, valid.Errorf("Aucun dépôt dans « %s ».", cours.Label()))
		return
	}
	job := s.releve(cours, "Remises de « "+cours.Label()+" »", noms, jusqua(request),
		func(remises map[string]groups.Handin) any {
			return map[string]any{
				"assignments": cours.WithHandins(travaux, repos, equipes, remises),
			}
		})
	writeJSON(writer, http.StatusAccepted, job.State())
}
