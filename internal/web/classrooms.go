package web

import (
	"net/http"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/runner"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/starter"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// classroomPayload est un groupe accompagné de ce que les dépôts en disent.
type classroomPayload struct {
	classroom.Classroom
	// SessionName est le nom long de la session : « Automne 2026 ». Il ne sert
	// qu'à l'affichage, et vit dans le magasin plutôt que dans chaque groupe.
	SessionName string                 `json:"session_name,omitempty"`
	Assignments []classroom.Assignment `json:"assignments"`
	Source      string                 `json:"source,omitempty"`
}

// fiche habille un groupe de son nom de session.
func (s *Server) fiche(cours classroom.Classroom) classroomPayload {
	return classroomPayload{
		Classroom:   cours,
		SessionName: s.classrooms.SessionName(cours.Session),
		Assignments: []classroom.Assignment{},
	}
}

// handleClassrooms liste les groupes déclarés, avec le nombre de leurs travaux
// quand l'inventaire de leur organisation est joignable.
func (s *Server) handleClassrooms(writer http.ResponseWriter, _ *http.Request) {
	declares := s.classrooms.List()
	liste := make([]classroomPayload, 0, len(declares))
	for _, cours := range declares {
		fiche := s.fiche(cours)
		if repos, source, err := s.repos(cours.Org, false); err == nil {
			fiche.Assignments = cours.Assignments(repos)
			fiche.Source = source
		}
		liste = append(liste, fiche)
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"classrooms": liste, "sessions": s.classrooms.Sessions(),
	})
}

// classroomInput est ce que l'interface envoie pour déclarer ou modifier un groupe.
type classroomInput struct {
	Name    string `json:"name"`
	Org     string `json:"org"`
	Session string `json:"session"`
	Course  string `json:"course"`
	Group   string `json:"group"`
	// SessionName est le nom long de la session, retenu pour toutes celles qui
	// portent le même nom court.
	SessionName string `json:"session_name"`
	// Prefix n'est renseigné que pour adopter un groupe de l'ancienne
	// nomenclature, en attendant sa migration.
	Prefix string `json:"prefix"`
	// Pattern adopte des dépôts que rien n'organise, en disant comment lire
	// leurs noms : « projet-{assignment}-{student} ».
	Pattern    string             `json:"pattern"`
	Students   []roster.Person    `json:"students"`
	RosterPath string             `json:"roster_path"`
	Defaults   classroom.Defaults `json:"defaults"`
}

// handleCreateClassroom déclare un groupe. Rien n'est écrit sur GitHub : un
// groupe n'est qu'un préfixe, une liste d'étudiants et des réglages.
func (s *Server) handleCreateClassroom(writer http.ResponseWriter, request *http.Request) {
	var body classroomInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	cree, err := s.classrooms.Add(classroom.Classroom{
		Name: body.Name, Org: body.Org, Session: body.Session,
		Course: body.Course, Group: body.Group,
		LegacyPrefix: body.Prefix, LegacyPattern: body.Pattern,
		Students: body.Students, RosterPath: body.RosterPath,
		Defaults: s.defaultsOr(body.Defaults),
	})
	if err != nil {
		fail(writer, err)
		return
	}
	if strings.TrimSpace(body.SessionName) != "" && cree.Session != "" {
		if err := s.classrooms.SetSessionName(cree.Session, body.SessionName); err != nil {
			fail(writer, err)
			return
		}
	}
	writeJSON(writer, http.StatusCreated, s.fiche(cree))
}

// defaultsOr complète des réglages absents par ceux de la session.
func (s *Server) defaultsOr(defauts classroom.Defaults) classroom.Defaults {
	if strings.TrimSpace(defauts.Visibility) != "" {
		return defauts
	}
	return classroom.DefaultsFrom(s.Settings())
}

// handleClassroom ouvre un groupe : ses réglages, ses étudiants, ses travaux.
func (s *Server) handleClassroom(writer http.ResponseWriter, request *http.Request) {
	cours, ok := s.classrooms.Get(request.PathValue("id"))
	if !ok {
		fail(writer, valid.Errorf("Groupe inconnu."))
		return
	}
	repos, source, err := s.repos(cours.Org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}
	fiche := s.fiche(cours)
	fiche.Assignments = cours.Assignments(repos)
	fiche.Source = source
	writeJSON(writer, http.StatusOK, fiche)
}

// handleUpdateClassroom modifie le nom, le préfixe ou les réglages d'un groupe.
func (s *Server) handleUpdateClassroom(writer http.ResponseWriter, request *http.Request) {
	cours, ok := s.classrooms.Get(request.PathValue("id"))
	if !ok {
		fail(writer, valid.Errorf("Groupe inconnu."))
		return
	}
	var body classroomInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	cours.Name, cours.Org = body.Name, body.Org
	cours.Session, cours.Course, cours.Group = body.Session, body.Course, body.Group
	cours.LegacyPrefix, cours.LegacyPattern = body.Prefix, body.Pattern
	if strings.TrimSpace(body.Session) != "" {
		// Le groupe rejoint la nomenclature courante : ce qui décrivait son
		// ancienne façon de lire les dépôts n'a plus cours.
		cours.LegacyPrefix, cours.LegacyPattern = "", ""
	}
	cours.Defaults = s.defaultsOr(body.Defaults)
	if body.Students != nil {
		cours.Students = body.Students
	}
	if strings.TrimSpace(body.RosterPath) != "" {
		cours.RosterPath = body.RosterPath
	}

	modifie, err := s.classrooms.Update(cours)
	if err != nil {
		fail(writer, err)
		return
	}
	if modifie.Session != "" {
		if err := s.classrooms.SetSessionName(modifie.Session, body.SessionName); err != nil {
			fail(writer, err)
			return
		}
	}
	writeJSON(writer, http.StatusOK, s.fiche(modifie))
}

// handleDeleteClassroom retire un groupe de la liste, sans toucher aux dépôts.
func (s *Server) handleDeleteClassroom(writer http.ResponseWriter, request *http.Request) {
	if err := s.classrooms.Delete(request.PathValue("id")); err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{
		"message": "Groupe retiré de la liste. Aucun dépôt n'a été supprimé sur GitHub.",
	})
}

// ------------------------------------------------------------------ étudiants

// studentRow est une ligne de la liste des étudiants du groupe.
type studentRow struct {
	FullName    string              `json:"full_name"`
	Username    string              `json:"username"`
	Assignments []studentAssignment `json:"assignments"`
}

// studentAssignment dit où un étudiant a déjà un dépôt.
type studentAssignment struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Repo string `json:"repo"`
	URL  string `json:"url"`
}

// handleClassroomStudents croise les étudiants du groupe avec ses travaux :
// c'est l'équivalent du « a accepté le devoir » de GitHub Classroom, déduit des
// dépôts existants plutôt que d'une invitation.
func (s *Server) handleClassroomStudents(writer http.ResponseWriter, request *http.Request) {
	cours, ok := s.classrooms.Get(request.PathValue("id"))
	if !ok {
		fail(writer, valid.Errorf("Groupe inconnu."))
		return
	}
	repos, _, err := s.repos(cours.Org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}

	travaux := cours.Assignments(repos)
	// Les dépôts de chaque travail sont rattachés à leur étudiant une fois pour
	// toutes : la relecture d'un nom ne dépend plus du compte GitHub.
	parEtudiant := map[string][]studentAssignment{}
	for _, travail := range travaux {
		for _, repo := range cours.Repos(travail.ID, repos) {
			student, inscrit := cours.StudentOf(repo.Name)
			if !inscrit {
				continue
			}
			cle := strings.ToLower(student.Username)
			parEtudiant[cle] = append(parEtudiant[cle], studentAssignment{
				Name: travail.Name, ID: travail.ID, Repo: repo.Name,
				URL: s.urlOf(cours.Org, repo),
			})
		}
	}

	lignes := make([]studentRow, 0, len(cours.Students))
	for _, student := range cours.Students {
		travauxDeLEtudiant := parEtudiant[strings.ToLower(student.Username)]
		if travauxDeLEtudiant == nil {
			travauxDeLEtudiant = []studentAssignment{}
		}
		lignes = append(lignes, studentRow{
			FullName: student.FullName, Username: student.Username,
			Assignments: travauxDeLEtudiant,
		})
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"students": lignes, "assignments": travaux,
	})
}

// handleSetStudents remplace la liste des étudiants du groupe, depuis un fichier
// de la machine ou depuis une liste déjà lue.
func (s *Server) handleSetStudents(writer http.ResponseWriter, request *http.Request) {
	cours, ok := s.classrooms.Get(request.PathValue("id"))
	if !ok {
		fail(writer, valid.Errorf("Groupe inconnu."))
		return
	}
	var body struct {
		Path   string          `json:"path"`
		People []roster.Person `json:"people"`
		// Append ajoute à la liste au lieu de la remplacer.
		Append bool `json:"append"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}

	people := body.People
	var issues []roster.Issue
	if strings.TrimSpace(body.Path) != "" {
		liste, err := roster.Load(body.Path)
		if err != nil {
			fail(writer, err)
			return
		}
		people, issues = liste.People, liste.Issues
		if chemin, err := roster.ExpandPath(body.Path); err == nil {
			cours.RosterPath = chemin
		}
	}
	if len(people) == 0 && !body.Append {
		fail(writer, valid.Errorf("Aucun étudiant dans la liste fournie."))
		return
	}
	if body.Append {
		cours.Students = append(cours.Students, people...)
	} else {
		cours.Students = people
	}

	modifie, err := s.classrooms.Update(cours)
	if err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"classroom": s.fiche(modifie), "issues": issues,
	})
}

// handleResolveStudentNames retrouve les noms complets manquants et les retient.
func (s *Server) handleResolveStudentNames(writer http.ResponseWriter, request *http.Request) {
	cours, ok := s.classrooms.Get(request.PathValue("id"))
	if !ok {
		fail(writer, valid.Errorf("Groupe inconnu."))
		return
	}
	pairs := make([]identity.Pair, 0, len(cours.Students))
	for _, student := range cours.Students {
		if strings.TrimSpace(student.FullName) == "" {
			pairs = append(pairs, identity.Pair{Repo: student.Username, Login: student.Username})
		}
	}
	if len(pairs) == 0 {
		fail(writer, valid.Errorf("Tous les noms complets sont déjà connus."))
		return
	}

	resolver := s.resolver(cours.Org)
	total := len(pairs)
	job := s.jobs.Start("noms", "Noms complets de « "+cours.Name+" »", func(job *Job) (any, error) {
		noms := resolver.Resolve(pairs, true, func(done, _ int, login string) {
			job.Progress(done, total, "@"+login)
		})
		complets := 0
		for position, student := range cours.Students {
			if nom := noms[student.Username]; nom != "" {
				cours.Students[position].FullName = nom
				complets++
			}
		}
		modifie, err := s.classrooms.Update(cours)
		if err != nil {
			return nil, err
		}
		return map[string]any{"resolved": complets, "students": modifie.Students}, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// ------------------------------------------------------- détail d'un travail

// assignmentRepo est un dépôt du travail, tel que l'affiche l'interface.
type assignmentRepo struct {
	Name       string `json:"name"`
	Student    string `json:"student"`
	FullName   string `json:"full_name"`
	Username   string `json:"username"`
	Private    bool   `json:"private"`
	Visibility string `json:"visibility"`
	URL        string `json:"url"`
	PushedAt   string `json:"pushed_at"`
}

// assignmentOf résout le groupe et le travail désignés par l'adresse.
func (s *Server) assignmentOf(request *http.Request) (
	classroom.Classroom, string, []groups.RepoInfo, error) {
	cours, ok := s.classrooms.Get(request.PathValue("id"))
	if !ok {
		return cours, "", nil, valid.Errorf("Groupe inconnu.")
	}
	nom := strings.TrimSpace(request.PathValue("name"))
	if nom == "" {
		return cours, "", nil, valid.Errorf("Travail inconnu.")
	}
	repos, _, err := s.repos(cours.Org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		return cours, "", nil, err
	}
	return cours, cours.AssignmentID(nom), repos, nil
}

// handleAssignment renvoie les dépôts d'un travail, étudiant par étudiant.
func (s *Server) handleAssignment(writer http.ResponseWriter, request *http.Request) {
	cours, id, repos, err := s.assignmentOf(request)
	if err != nil {
		fail(writer, err)
		return
	}
	trouves := cours.Repos(id, repos)
	if len(trouves) == 0 {
		fail(writer, valid.Errorf("Aucun dépôt pour le travail « %s ».", cours.ShortName(id)))
		return
	}

	lignes := make([]assignmentRepo, 0, len(trouves))
	for _, repo := range trouves {
		ligne := assignmentRepo{
			Name: repo.Name, Student: repo.Suffix, Private: repo.Private,
			Visibility: repo.Visibility(), URL: s.urlOf(cours.Org, repo),
			PushedAt: repo.PushedAt,
		}
		if student, inscrit := cours.StudentOf(repo.Name); inscrit {
			ligne.FullName, ligne.Username = student.FullName, student.Username
		}
		lignes = append(lignes, ligne)
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"id": id, "name": cours.ShortName(id), "repos": lignes,
	})
}

// handleAssignmentAccess inspecte les accès de tous les dépôts d'un travail.
func (s *Server) handleAssignmentAccess(writer http.ResponseWriter, request *http.Request) {
	cours, id, repos, err := s.assignmentOf(request)
	if err != nil {
		fail(writer, err)
		return
	}
	trouves := cours.Repos(id, repos)

	job := s.jobs.Start("acces", "Accès des dépôts de « "+cours.ShortName(id)+" »",
		func(job *Job) (any, error) {
			found := make([]accessPayload, 0, len(trouves))
			for index, repo := range trouves {
				if job.Canceled() {
					return found, nil
				}
				payload, err := s.accessOf(cours.Org, repo.Name)
				if err != nil {
					return nil, err
				}
				found = append(found, payload)
				job.Progress(index+1, len(trouves), repo.Name)
				job.Line(repo.Name, payload)
			}
			return found, nil
		})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// ------------------------------------------------------------------- travaux

// assignmentInput décrit le travail à distribuer.
type assignmentInput struct {
	Name     string             `json:"name"`
	Settings classroom.Defaults `json:"settings"`
	// Usernames restreint la distribution ; vide, tout le groupe est servi.
	Usernames    []string `json:"usernames"`
	DryRun       bool     `json:"dry_run"`
	ForceStarter bool     `json:"force_starter"`
}

// prepare valide un travail et renvoie le groupe, les réglages, et les étudiants
// à servir — ceux qui ont déjà un dépôt pour ce travail étant écartés.
func (s *Server) prepare(request *http.Request, body assignmentInput) (
	classroom.Classroom, config.Settings, []roster.Person, []roster.Person, error) {
	var vide config.Settings
	cours, ok := s.classrooms.Get(request.PathValue("id"))
	if !ok {
		return cours, vide, nil, nil, valid.Errorf("Groupe inconnu.")
	}
	if cours.Legacy() {
		return cours, vide, nil, nil, valid.Errorf(
			"« %s » suit l'ancienne nomenclature. Migrez-le vers « cours%sgroupe » "+
				"avant de lui distribuer un travail.", cours.Name, naming.Separator)
	}
	// Le nom du dépôt contient désormais le nom de l'étudiant : sans lui, il n'y
	// a pas de dépôt à nommer.
	if incomplets := cours.MissingNames(); len(incomplets) > 0 {
		comptes := make([]string, 0, len(incomplets))
		for _, student := range incomplets {
			comptes = append(comptes, "@"+student.Username)
		}
		return cours, vide, nil, nil, valid.Errorf(
			"Nom complet manquant pour %s : le nom du dépôt en dépend. "+
				"Retrouvez les noms depuis l'onglet Étudiants.", strings.Join(comptes, ", "))
	}
	nom, err := naming.Fragment(body.Name, "Nom du travail")
	if err != nil {
		return cours, vide, nil, nil, err
	}

	cours.Defaults = s.defaultsOr(body.Settings)
	settings, err := normalize(cours.Settings(nom))
	if err != nil {
		return cours, vide, nil, nil, err
	}

	// Une sélection présente, fût-elle vide, reste une sélection : cocher
	// personne ne doit pas revenir à servir tout le groupe.
	restreint := body.Usernames != nil
	voulus := map[string]bool{}
	for _, login := range body.Usernames {
		voulus[strings.ToLower(login)] = true
	}
	repos, _, err := s.repos(cours.Org, false)
	if err != nil {
		return cours, vide, nil, nil, err
	}
	servis := cours.Served(settings.Assignment, repos)

	var aServir, dejaServis []roster.Person
	for _, student := range cours.Students {
		if restreint && !voulus[strings.ToLower(student.Username)] {
			continue
		}
		if servis[strings.ToLower(student.Username)] {
			dejaServis = append(dejaServis, student)
			continue
		}
		aServir = append(aServir, student)
	}
	return cours, settings, aServir, dejaServis, nil
}

// handlePreviewAssignment montre les dépôts qui seraient créés, sans rien écrire.
func (s *Server) handlePreviewAssignment(writer http.ResponseWriter, request *http.Request) {
	var body assignmentInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	cours, settings, aServir, dejaServis, err := s.prepare(request, body)
	if err != nil {
		fail(writer, err)
		return
	}
	items, err := plan.Build(aServir, settings)
	if err != nil && len(aServir) > 0 {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"assignment": settings.Assignment,
		"short_name": cours.ShortName(settings.Assignment),
		"items":      rows(items),
		"served":     dejaServis,
	})
}

// handleCreateAssignment distribue un travail aux étudiants du groupe.
func (s *Server) handleCreateAssignment(writer http.ResponseWriter, request *http.Request) {
	var body assignmentInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	cours, settings, aServir, dejaServis, err := s.prepare(request, body)
	if err != nil {
		fail(writer, err)
		return
	}
	if len(aServir) == 0 {
		fail(writer, valid.Errorf(
			"Rien à distribuer : tous les étudiants retenus ont déjà un dépôt pour ce travail."))
		return
	}
	items, err := plan.Build(aServir, settings)
	if err != nil {
		fail(writer, err)
		return
	}

	// Les fichiers de départ sont relus au lancement : le dossier a pu changer
	// depuis la prévisualisation.
	var bundle *starter.Bundle
	if strings.TrimSpace(settings.StarterDir) != "" {
		bundle, err = starter.Load(settings.StarterDir)
		if err != nil {
			fail(writer, err)
			return
		}
		settings.StarterDir = bundle.Root
	}

	// Le groupe retient les réglages du dernier travail distribué : le suivant
	// n'aura pas à les retaper.
	cours.Defaults.StarterDir = settings.StarterDir
	if _, err := s.classrooms.Update(cours); err != nil {
		fail(writer, err)
		return
	}

	label := "Distribution de « " + cours.ShortName(settings.Assignment) + " » à " +
		itoa(len(items)) + " étudiant(s)"
	if body.DryRun {
		label = "Simulation de « " + cours.ShortName(settings.Assignment) + " »"
	}
	job := s.jobs.Start("distribution", label, func(job *Job) (any, error) {
		executor := runner.New(s.deps.Client, settings, bundle)
		report, err := executor.Run(items, runner.Options{
			DryRun:       body.DryRun,
			ForceStarter: body.ForceStarter,
			OnProgress: func(index, total int, result runner.Result) {
				job.Progress(index, total, result.Repo)
				job.Line(result.Repo+" : "+result.Status, result)
			},
		})
		if err != nil {
			return nil, err
		}
		if !body.DryRun && report.Count(runner.Created) > 0 {
			s.forget(cours.Org)
		}

		bilan := map[string]any{
			"report": report, "assignment": settings.Assignment,
			"short_name": cours.ShortName(settings.Assignment),
			"created":    report.Count(runner.Created),
			"existing":   report.Count(runner.Existing),
			"failed":     len(report.Failures()),
			"dry_run":    body.DryRun,
			"skipped":    dejaServis,
		}
		if jsonPath, csvPath, err := report.Save(s.reportDir()); err != nil {
			job.Warn("Bilan non enregistré : " + err.Error())
		} else {
			bilan["json_path"], bilan["csv_path"] = jsonPath, csvPath
		}
		return bilan, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// ----------------------------------------------------------------- candidats

// handleCandidates propose des groupes à partir des dépôts déjà présents, pour
// qu'une organisation en cours d'année s'adopte sans rien renommer.
func (s *Server) handleCandidates(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	repos, source, err := s.repos(org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}

	// Les préfixes déjà couverts par un groupe ne sont plus à proposer.
	pris := map[string]bool{}
	for _, cours := range s.classrooms.List() {
		if strings.EqualFold(cours.Org, org) {
			pris[strings.ToLower(cours.Scope())] = true
		}
	}
	proposes := make([]classroom.Candidate, 0)
	for _, candidat := range classroom.Candidates(repos) {
		if pris[strings.ToLower(candidat.Prefix)] {
			continue
		}
		proposes = append(proposes, candidat)
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"candidates": proposes, "total": len(repos), "source": source,
	})
}
