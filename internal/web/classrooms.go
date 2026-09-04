package web

import (
	"net/http"
	"sort"
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
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/students"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// classroomPayload est un groupe accompagné de ce que les dépôts en disent.
// Tout ce qu'il porte se lit ailleurs : sa place et ses travaux dans les noms
// de dépôts, le nom de sa session dans son nom court. Le fichier local n'ajoute
// que la liste des étudiants et les réglages des prochains travaux.
type classroomPayload struct {
	classroom.Classroom
	// Scope désigne le groupe : c'est par là que l'interface le rouvre.
	Scope string `json:"scope"`
	// Label et SessionName se déduisent de la place ; ils ne sont pas retenus.
	Label       string                 `json:"label"`
	SessionName string                 `json:"session_name,omitempty"`
	Assignments []classroom.Assignment `json:"assignments"`
	Source      string                 `json:"source,omitempty"`
	// Known dit qu'une liste d'étudiants et des réglages sont retenus pour ce
	// groupe ; sinon, il n'existe que par ses dépôts.
	Known bool `json:"known"`
}

// fiche habille un groupe de ce que sa place laisse déduire.
func (s *Server) fiche(cours classroom.Classroom) classroomPayload {
	_, connu := s.classrooms.Find(cours.Org, cours.Scope())
	return classroomPayload{
		Classroom:   cours,
		Scope:       cours.Scope(),
		Label:       cours.Label(),
		SessionName: cours.SessionName(),
		Assignments: []classroom.Assignment{},
		Known:       connu,
	}
}

// handleClassrooms liste les groupes de l'organisation : ceux que les dépôts
// dessinent, et ceux qu'on a déclarés sans qu'un travail n'ait encore été
// distribué.
func (s *Server) handleClassrooms(writer http.ResponseWriter, request *http.Request) {
	org := s.org()
	repos, source, err := s.repos(org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}

	sessions := map[string]bool{}
	visibles := s.visibles(org, repos)
	liste := make([]classroomPayload, 0, len(visibles))
	for _, cours := range visibles {
		fiche := s.fiche(cours)
		fiche.Assignments = cours.Assignments(repos)
		fiche.Source = source
		liste = append(liste, fiche)
		if cours.Session != "" {
			sessions[cours.Session] = true
		}
	}

	courtes := make([]classroom.Session, 0, len(sessions))
	for court := range sessions {
		courtes = append(courtes, classroom.Session{
			Short: court, Name: classroom.SessionName(court),
		})
	}
	sort.Slice(courtes, func(i, j int) bool {
		return strings.ToLower(courtes[i].Short) < strings.ToLower(courtes[j].Short)
	})

	writeJSON(writer, http.StatusOK, map[string]any{
		"classrooms": liste, "sessions": courtes, "org": org,
	})
}

// classroomInput est ce que l'interface envoie pour déclarer un groupe ou
// changer ce qu'on retient de lui. Ni nom d'affichage, ni nom de session : ils
// se déduisent de la place, et les inventer localement ne les rendrait vrais
// que sur cette machine.
type classroomInput struct {
	Session string `json:"session"`
	Course  string `json:"course"`
	Group   string `json:"group"`
	// Prefix n'est renseigné que pour adopter un groupe d'une nomenclature
	// dépassée, en attendant son renommage.
	Prefix string `json:"prefix"`
	// Pattern adopte des dépôts que rien n'organise, en disant comment lire
	// leurs noms : « projet-{assignment}-{student} ».
	Pattern    string             `json:"pattern"`
	Students   []roster.Person    `json:"students"`
	RosterPath string             `json:"roster_path"`
	Defaults   classroom.Defaults `json:"defaults"`
}

// classroom compose le groupe décrit par une requête.
func (s *Server) fromInput(body classroomInput) classroom.Classroom {
	return classroom.Classroom{
		Org: s.org(), Session: body.Session, Course: body.Course, Group: body.Group,
		LegacyPrefix: body.Prefix, LegacyPattern: body.Pattern,
		Students: body.Students, RosterPath: body.RosterPath,
		Defaults: s.defaultsOr(body.Defaults),
	}
}

// handleCreateClassroom retient une liste et des réglages pour une place. Rien
// n'est écrit sur GitHub : le groupe existera vraiment quand ses dépôts
// existeront.
func (s *Server) handleCreateClassroom(writer http.ResponseWriter, request *http.Request) {
	var body classroomInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	cree, err := s.classrooms.Save(s.fromInput(body))
	if err != nil {
		fail(writer, err)
		return
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
	cours, err := s.place(request)
	if err != nil {
		fail(writer, err)
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

// handleUpdateClassroom change ce qu'on retient d'un groupe : sa liste et ses
// réglages. Sa place, elle, ne se change pas ici — il faudrait renommer ses
// dépôts, ce que fait « migration ».
func (s *Server) handleUpdateClassroom(writer http.ResponseWriter, request *http.Request) {
	cours, err := s.place(request)
	if err != nil {
		fail(writer, err)
		return
	}
	var body classroomInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	cours.Defaults = s.defaultsOr(body.Defaults)
	if body.Students != nil {
		cours.Students = body.Students
	}
	if strings.TrimSpace(body.RosterPath) != "" {
		cours.RosterPath = body.RosterPath
	}

	modifie, err := s.classrooms.Save(cours)
	if err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, s.fiche(modifie))
}

// handleForgetClassroom oublie ce qu'on retenait d'un groupe. Aucun dépôt n'est
// touché : s'il en reste, le groupe continue d'exister et de s'afficher — sans
// sa liste ni ses réglages.
func (s *Server) handleForgetClassroom(writer http.ResponseWriter, request *http.Request) {
	if err := s.classrooms.Forget(s.org(), request.PathValue("scope")); err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{
		"message": "Liste et réglages oubliés. Aucun dépôt n'a été supprimé sur GitHub.",
	})
}

// ------------------------------------------------------------------ étudiants

// studentRow est une ligne de la liste des étudiants du groupe.
type studentRow struct {
	FullName    string              `json:"full_name"`
	Username    string              `json:"username"`
	Assignments []studentAssignment `json:"assignments"`
	// PushedAt est le dernier envoi de la personne, tous travaux confondus.
	PushedAt string `json:"pushed_at,omitempty"`
}

// studentAssignment dit où un étudiant a déjà un dépôt.
type studentAssignment struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Repo     string `json:"repo"`
	URL      string `json:"url"`
	PushedAt string `json:"pushed_at,omitempty"`
}

// studentQuery lit les critères de tri et de filtre passés dans l'adresse. Ce
// qu'ils veulent dire est décidé dans « students » : l'interface ne fait ici
// que transmettre ce qu'on lui a demandé.
func studentQuery(request *http.Request) (students.Filter, students.Key, bool, error) {
	valeurs := request.URL.Query()
	filtre, err := students.Filter{
		Text:         valeurs.Get("q"),
		Name:         valeurs.Get("name"),
		Username:     valeurs.Get("username"),
		Assignment:   valeurs.Get("assignment"),
		PushedAfter:  valeurs.Get("after"),
		PushedBefore: valeurs.Get("before"),
		Activity:     students.Activity(valeurs.Get("activity")),
	}.Validate()
	if err != nil {
		return filtre, students.ByName, false, err
	}
	tri, err := students.ParseKey(valeurs.Get("sort"))
	if err != nil {
		return filtre, tri, false, err
	}
	return filtre, tri, valeurs.Get("desc") == "1", nil
}

// handleClassroomStudents croise les étudiants du groupe avec ses travaux :
// c'est l'équivalent du « a accepté le devoir » de GitHub Classroom, déduit des
// dépôts existants plutôt que d'une invitation. La liste se filtre et se trie
// ici plutôt qu'au navigateur, pour que « avant le 1er octobre » veuille dire
// la même chose partout.
func (s *Server) handleClassroomStudents(writer http.ResponseWriter, request *http.Request) {
	cours, err := s.place(request)
	if err != nil {
		fail(writer, err)
		return
	}
	filtre, tri, decroissant, err := studentQuery(request)
	if err != nil {
		fail(writer, err)
		return
	}
	repos, _, err := s.repos(cours.Org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}

	toutes := students.Build(cours, repos)
	retenues := students.Apply(toutes, filtre, tri, decroissant)

	// Les noms complets manquants se comptent sur le groupe entier : le
	// bouton qui les retrouve n'a pas à dépendre de ce qui est affiché.
	manquants := 0
	for _, ligne := range toutes {
		if ligne.FullName == "" {
			manquants++
		}
	}

	lignes := make([]studentRow, 0, len(retenues))
	for _, ligne := range retenues {
		travaux := make([]studentAssignment, 0, len(ligne.Repos))
		for _, depot := range ligne.Repos {
			travaux = append(travaux, studentAssignment{
				Name: depot.Assignment, ID: depot.ID, Repo: depot.Name,
				URL:      s.urlOf(cours.Org, groups.Repo{Name: depot.Name, URL: depot.URL}),
				PushedAt: depot.PushedAt,
			})
		}
		lignes = append(lignes, studentRow{
			FullName: ligne.FullName, Username: ligne.Username,
			Assignments: travaux, PushedAt: ligne.PushedAt,
		})
	}

	writeJSON(writer, http.StatusOK, map[string]any{
		"students": lignes, "assignments": cours.Assignments(repos),
		// Le total dit combien le filtre a écarté : sans lui, une liste vide ne
		// distinguerait pas un groupe vide d'un critère trop étroit.
		"total": len(toutes), "shown": len(lignes), "missing_names": manquants,
	})
}

// handleSetStudents remplace la liste des étudiants du groupe, depuis un fichier
// de la machine ou depuis une liste déjà lue.
func (s *Server) handleSetStudents(writer http.ResponseWriter, request *http.Request) {
	cours, err := s.place(request)
	if err != nil {
		fail(writer, err)
		return
	}
	var body struct {
		Path   string          `json:"path"`
		People []roster.Person `json:"people"`
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
	if len(people) == 0 {
		fail(writer, valid.Errorf("Aucun étudiant dans la liste fournie."))
		return
	}
	cours.Students = people

	modifie, err := s.classrooms.Save(cours)
	if err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"classroom": s.fiche(modifie), "issues": issues,
	})
}

// handleAddStudent ajoute une personne à la liste du groupe, sans toucher au
// reste. Remplacer la liste entière pour une inscription tardive obligeait à
// avoir le fichier sous la main ; ici, deux champs suffisent.
//
// Les travaux qu'on lui désigne lui sont remis dans la foulée, aux réglages que
// le groupe retient — ceux-là mêmes qui ont servi à ses camarades. Sans eux,
// une arrivée en cours de session demandait de revenir distribuer travail par
// travail.
func (s *Server) handleAddStudent(writer http.ResponseWriter, request *http.Request) {
	cours, err := s.place(request)
	if err != nil {
		fail(writer, err)
		return
	}
	var body struct {
		FullName string `json:"full_name"`
		Username string `json:"username"`
		// Assignments désigne les travaux dont le dépôt doit lui être créé.
		Assignments []string `json:"assignments"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	augmente, err := cours.Add(roster.Person{FullName: body.FullName, Username: body.Username})
	if err != nil {
		fail(writer, err)
		return
	}
	// Un compte qui n'existe pas sur GitHub ne sert à rien dans une liste :
	// aucun dépôt ne pourra lui être remis.
	personne := augmente.Students[len(augmente.Students)-1]
	if existe, err := s.deps.Client.UserExists(personne.Username); err == nil && !existe {
		fail(writer, valid.Errorf("Le compte « %s » n'existe pas sur GitHub.", personne.Username))
		return
	}

	// Tout est préparé avant d'écrire quoi que ce soit : un travail qu'on ne
	// saurait pas nommer refuse l'ajout entier plutôt que de laisser la
	// personne inscrite à moitié servie.
	remises, err := s.remises(augmente, personne, body.Assignments)
	if err != nil {
		fail(writer, err)
		return
	}

	modifie, err := s.classrooms.Save(augmente)
	if err != nil {
		fail(writer, err)
		return
	}
	if len(remises) == 0 {
		writeJSON(writer, http.StatusOK, map[string]any{
			"classroom": s.fiche(modifie), "student": personne, "created": 0,
		})
		return
	}

	noms := make([]string, 0, len(remises))
	for _, remise := range remises {
		noms = append(noms, remise.Name)
	}
	label := "Dépôts de @" + personne.Username + " — " + strings.Join(noms, ", ")
	job := s.jobs.Start("distribution", label, func(job *Job) (any, error) {
		crees, existants, echecs := 0, 0, 0
		for index, remise := range remises {
			if job.Canceled() {
				break
			}
			executor := runner.New(s.deps.Client, remise.Settings, remise.Bundle)
			report, err := executor.Run(remise.Items, runner.Options{
				OnProgress: func(_, _ int, result runner.Result) {
					job.Line(result.Repo+" : "+result.Status, result)
				},
			})
			if err != nil {
				return nil, err
			}
			crees += report.Count(runner.Created)
			existants += report.Count(runner.Existing)
			echecs += len(report.Failures())
			job.Progress(index+1, len(remises), remise.Name)
		}
		if crees > 0 {
			s.forget(cours.Org)
		}
		return map[string]any{
			"student": personne, "created": crees,
			"existing": existants, "failed": echecs,
		}, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// remise est un travail à remettre à une personne : ses réglages, le dépôt à
// créer, et les fichiers de départ qui l'accompagnent.
type remise struct {
	Name     string
	Settings config.Settings
	Items    []plan.PlannedRepo
	Bundle   *starter.Bundle
}

// remises compose ce qu'il faut créer pour donner à une personne les dépôts des
// travaux désignés. Rien n'est écrit ici : un réglage devenu faux — un dossier
// de fichiers de départ disparu depuis la dernière distribution — refuse la
// remise entière au lieu de l'interrompre à mi-chemin.
func (s *Server) remises(cours classroom.Classroom, personne roster.Person,
	noms []string) ([]remise, error) {
	if len(noms) == 0 {
		return nil, nil
	}
	if cours.Legacy() {
		return nil, valid.Errorf(
			"« %s » suit une nomenclature dépassée : ses dépôts ne peuvent pas être nommés. "+
				"Renommez-les d'abord, ou ajoutez la personne sans ses dépôts.", cours.Label())
	}

	// Le nom du dépôt vient du nom complet : sans lui, il n'y a rien à nommer.
	// Le dire ici évite de laisser l'échec surgir du fond du plan.
	if _, err := naming.Student(personne.FullName); err != nil {
		return nil, valid.Errorf(
			"Le nom complet de @%s manque : sans lui, ses dépôts ne peuvent pas être nommés.",
			personne.Username)
	}

	vus := map[string]bool{}
	preparees := make([]remise, 0, len(noms))
	for _, brut := range noms {
		nom, err := naming.Fragment(brut, "Nom du travail")
		if err != nil {
			return nil, err
		}
		if vus[strings.ToLower(nom)] {
			continue
		}
		vus[strings.ToLower(nom)] = true

		settings, err := normalize(cours.Settings(nom))
		if err != nil {
			return nil, err
		}
		items, err := plan.Build([]roster.Person{personne}, settings)
		if err != nil {
			return nil, err
		}
		var bundle *starter.Bundle
		if strings.TrimSpace(settings.StarterDir) != "" {
			if bundle, err = starter.Load(settings.StarterDir); err != nil {
				return nil, valid.Errorf(
					"Fichiers de départ de « %s » : %v Corrigez le dossier dans les réglages "+
						"du groupe, ou ajoutez la personne sans ce travail.", nom, err)
			}
			settings.StarterDir = bundle.Root
		}
		preparees = append(preparees, remise{
			Name: nom, Settings: settings, Items: items, Bundle: bundle,
		})
	}
	return preparees, nil
}

// handleResolveStudentNames retrouve les noms complets manquants et les retient.
func (s *Server) handleResolveStudentNames(writer http.ResponseWriter, request *http.Request) {
	cours, err := s.place(request)
	if err != nil {
		fail(writer, err)
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
	job := s.jobs.Start("noms", "Noms complets de « "+cours.Label()+" »", func(job *Job) (any, error) {
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
		modifie, err := s.classrooms.Save(cours)
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
	cours, err := s.place(request)
	if err != nil {
		return cours, "", nil, err
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
	cours, err := s.place(request)
	if err != nil {
		return cours, vide, nil, nil, err
	}
	if cours.Legacy() {
		return cours, vide, nil, nil, valid.Errorf(
			"« %s » suit une nomenclature dépassée. Renommez ses dépôts en "+
				"« session%[2]scours%[2]sgroupe » avant de lui distribuer un travail.",
			cours.Label(), naming.Separator)
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
	if _, err := s.classrooms.Save(cours); err != nil {
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
	for _, cours := range s.visibles(org, repos) {
		pris[classroom.NormalizeScope(cours.Scope())] = true
	}
	proposes := make([]classroom.Candidate, 0)
	for _, candidat := range classroom.Candidates(repos) {
		if pris[classroom.NormalizeScope(candidat.Prefix)] {
			continue
		}
		proposes = append(proposes, candidat)
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"candidates": proposes, "total": len(repos), "source": source,
	})
}
