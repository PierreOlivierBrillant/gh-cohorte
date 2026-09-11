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
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/students"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
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
	// Teams compte les équipes du groupe ; elles vivent sur GitHub, pas ici.
	Teams  int    `json:"teams"`
	Source string `json:"source,omitempty"`
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

	// Les équipes disent lesquels des travaux sont d'équipe ; leur absence
	// n'empêche rien — le groupe n'a alors que des travaux individuels.
	infos, _ := s.orgTeams(org, false)

	visibles := s.visibles(org, repos)
	liste := make([]classroomPayload, 0, len(visibles))
	courts := make([]string, 0, len(visibles))
	for _, cours := range visibles {
		equipes := cours.Teams(infos)
		fiche := s.fiche(cours)
		fiche.Assignments = cours.Assignments(repos, equipes)
		fiche.Teams = len(equipes)
		fiche.Source = source
		liste = append(liste, fiche)
		courts = append(courts, cours.Session)
	}

	writeJSON(writer, http.StatusOK, map[string]any{
		"classrooms": liste, "sessions": classroom.SessionsOf(courts), "org": org,
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
	depart := s.fromInput(body)
	if err := s.apprendre(depart.Org, depart.Students...); err != nil {
		fail(writer, err)
		return
	}
	cree, err := s.classrooms.Save(depart)
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
	cours = s.enrichi(cours, repos)
	equipes, err := s.teamsIn(cours)
	if err != nil {
		fail(writer, err)
		return
	}
	fiche := s.fiche(cours)
	fiche.Assignments = cours.Assignments(repos, equipes)
	fiche.Teams = len(equipes)
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
	FullName string `json:"full_name"`
	Username string `json:"username"`
	// Accounts porte tous les comptes de la personne ; le premier est celui
	// qui la désigne.
	Accounts []string `json:"accounts,omitempty"`
	// Team nomme l'équipe de la personne dans ce groupe ; vide si elle n'en a
	// aucune.
	Team        string              `json:"team,omitempty"`
	Assignments []studentAssignment `json:"assignments"`
	// PushedAt est le dernier envoi de la personne, tous travaux confondus.
	PushedAt string `json:"pushed_at,omitempty"`
}

// studentAssignment dit où un étudiant a déjà un dépôt.
type studentAssignment struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Repo string `json:"repo"`
	URL  string `json:"url"`
	// Team nomme l'équipe à qui le dépôt appartient ; vide pour un travail
	// individuel.
	Team     string `json:"team,omitempty"`
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
	cours = s.enrichi(cours, repos)
	equipes, err := s.teamsIn(cours)
	if err != nil {
		fail(writer, err)
		return
	}

	toutes := students.Build(cours, repos, equipes)
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
				Team:     depot.Team,
				PushedAt: depot.PushedAt,
			})
		}
		equipe := ""
		for _, compte := range ligne.Accounts {
			if sienne, membre := teams.Of(equipes, compte); membre {
				equipe = sienne.Short
				break
			}
		}
		lignes = append(lignes, studentRow{
			FullName: ligne.FullName, Username: ligne.Username,
			Accounts: ligne.Accounts, Team: equipe,
			Assignments: travaux, PushedAt: ligne.PushedAt,
		})
	}

	writeJSON(writer, http.StatusOK, map[string]any{
		"students": lignes, "assignments": cours.Assignments(repos, equipes),
		"team_names": teamNames(equipes),
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
	if err := s.apprendre(cours.Org, people...); err != nil {
		fail(writer, err)
		return
	}

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
	if err := s.apprendre(cours.Org, personne); err != nil {
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
			// Une création oblige à relire : le dépôt neuf n'est pas dans
			// l'inventaire, et sa date de dernier envoi ne s'invente pas.
			s.forget(cours.Org)
		}
		return map[string]any{
			"student": personne, "created": crees,
			"existing": existants, "failed": echecs,
		}, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// handleAttachAccount rattache un second compte GitHub à une personne déjà
// inscrite. Une même personne travaille parfois sous deux comptes ; sans le
// dire, ils passeraient pour deux étudiants du même nom — ce que la préparation
// refuse, et à raison : deux homonymes réels ne peuvent pas partager un dépôt.
//
// Rien ne se déduit du nom, ici ou ailleurs : c'est une décision, et c'est
// pourquoi elle se prend à la main.
func (s *Server) handleAttachAccount(writer http.ResponseWriter, request *http.Request) {
	cours, err := s.place(request)
	if err != nil {
		fail(writer, err)
		return
	}
	var body struct {
		// Username désigne la personne, Account le compte à lui rattacher.
		Username string `json:"username"`
		Account  string `json:"account"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	compte, err := valid.Login(body.Account, "Compte GitHub")
	if err != nil {
		fail(writer, err)
		return
	}
	personne, inscrite := cours.Find(body.Username)
	if !inscrite {
		fail(writer, valid.Errorf("@%s n'est pas dans « %s ».",
			strings.TrimSpace(body.Username), cours.Label()))
		return
	}
	if personne.Owns(compte) {
		fail(writer, valid.Errorf("@%s est déjà un compte de %s.",
			compte, personne.FullName))
		return
	}
	// Un compte qui n'existe pas sur GitHub ne sert à rien dans une liste.
	if existe, err := s.deps.Client.UserExists(compte); err == nil && !existe {
		fail(writer, valid.Errorf("Le compte « %s » n'existe pas sur GitHub.", compte))
		return
	}

	augmente := personne
	augmente.Also = append(append([]string(nil), personne.Also...), compte)
	// Le compte pouvait être inscrit à part : le rattacher le retire de là,
	// sans quoi la même personne y figurerait deux fois.
	modifie, err := cours.Without(compte).Rename(personne.Username, augmente)
	if err != nil {
		fail(writer, err)
		return
	}
	if err := s.apprendre(cours.Org, roster.Person{
		FullName: personne.FullName, Username: compte,
	}); err != nil {
		fail(writer, err)
		return
	}
	enregistre, err := s.classrooms.Save(modifie)
	if err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"classroom": s.fiche(enregistre), "student": augmente, "account": compte,
	})
}

// handleRenameStudent corrige la fiche d'une personne — son nom complet, son
// compte GitHub, ou les deux — sans toucher au reste de la liste. Un prénom mal
// orthographié obligeait jusqu'ici à remplacer la liste entière, donc celle de
// tout le monde, et à avoir le fichier sous la main.
//
// Le nom complet est le dernier niveau du nom des dépôts : le corriger laisse
// ceux déjà créés sous l'ancien. Les renommer est une écriture sur GitHub, elle
// ne part donc que si on la demande.
func (s *Server) handleRenameStudent(writer http.ResponseWriter, request *http.Request) {
	cours, err := s.place(request)
	if err != nil {
		fail(writer, err)
		return
	}
	var body struct {
		// Username désigne la personne telle qu'elle est encore inscrite.
		Username string `json:"username"`
		// FullName et NewUsername sont ce qu'elle devient ; un compte laissé
		// vide reste celui qu'elle avait.
		FullName    string `json:"full_name"`
		NewUsername string `json:"new_username"`
		// Repos demande de renommer aussi ses dépôts pour qu'ils portent son
		// nouveau nom.
		Repos bool `json:"repos"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}

	// Les dépôts sont relevés avant même de chercher la personne : celle que
	// seuls ses dépôts révèlent — un groupe qu'on n'a pas déclaré ici — doit
	// pouvoir être nommée comme les autres, et c'est justement celle à qui il
	// manque un nom.
	repos, _, err := s.repos(cours.Org, false)
	if err != nil {
		fail(writer, err)
		return
	}
	cours = s.enrichi(cours, repos)

	avant, inscrit := cours.Find(body.Username)
	if !inscrit {
		fail(writer, valid.Errorf("@%s n'est pas dans « %s ».",
			strings.TrimSpace(body.Username), cours.Label()))
		return
	}
	compte := strings.TrimSpace(body.NewUsername)
	if compte == "" {
		compte = avant.Username
	}
	modifie, err := cours.Rename(avant.Username,
		roster.Person{FullName: body.FullName, Username: compte})
	if err != nil {
		fail(writer, err)
		return
	}
	apres, _ := modifie.Find(compte)

	// Un compte qui n'existe pas sur GitHub ne sert à rien dans une liste :
	// aucun dépôt ne pourra lui être remis. Celui qui ne change pas a déjà été
	// vérifié à l'inscription.
	if !strings.EqualFold(apres.Username, avant.Username) {
		if existe, err := s.deps.Client.UserExists(apres.Username); err == nil && !existe {
			fail(writer, valid.Errorf("Le compte « %s » n'existe pas sur GitHub.", apres.Username))
			return
		}
	}

	// Le plan se compose en entier avant la première écriture, et sur le groupe
	// tel qu'il est encore : c'est l'ancien nom qui retrouve ses dépôts.
	var renommages []classroom.Move
	if body.Repos {
		if renommages, err = classroom.PlanRenameStudent(cours, avant, apres, repos); err != nil {
			fail(writer, err)
			return
		}
	}

	// Le registre retient le nouveau nom sans oublier l'ancien slug : les
	// dépôts déjà créés restent rattachés à leur personne, qu'on les renomme
	// ou non.
	if err := s.apprendre(cours.Org, apres); err != nil {
		fail(writer, err)
		return
	}

	enregistre, err := s.classrooms.Save(modifie)
	if err != nil {
		fail(writer, err)
		return
	}

	bilan := map[string]any{
		"classroom": s.fiche(enregistre), "student": apres, "previous": avant,
		"renamed": 0,
	}
	if len(renommages) == 0 {
		writeJSON(writer, http.StatusOK, bilan)
		return
	}

	label := "Dépôts de @" + apres.Username + " au nom de " + apres.FullName
	job := s.jobs.Start("renommage", label, func(job *Job) (any, error) {
		renommes, echecs := 0, 0
		var suivis []groups.Renamed
		for index, ligne := range renommages {
			if job.Canceled() {
				break
			}
			apres, err := s.deps.Client.RenameRepo(cours.Org, ligne.Repo, ligne.Target)
			if err != nil {
				echecs++
				job.Line(ligne.Repo+" : échec — "+err.Error(),
					map[string]string{"status": "échec"})
			} else {
				renommes++
				suivis = append(suivis, groups.Renamed{Before: ligne.Repo, After: apres.Info()})
				job.Line(ligne.Repo+" → "+ligne.Target,
					map[string]string{"status": "mis à jour"})
			}
			job.Progress(index+1, len(renommages), ligne.Repo)
		}
		s.renamed(cours.Org, suivis)
		bilan["renamed"], bilan["failed"] = renommes, echecs
		return bilan, nil
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
	// Le bouton compte les noms manquants sur la liste que les dépôts
	// complètent ; les retrouver doit porter sur la même liste, sans quoi il
	// annoncerait des noms qu'il ne chercherait jamais.
	repos, _, err := s.repos(cours.Org, false)
	if err != nil {
		fail(writer, err)
		return
	}
	cours = s.enrichi(cours, repos)

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
		var retrouvees []roster.Person
		for position, student := range cours.Students {
			if nom := noms[student.Username]; nom != "" {
				cours.Students[position].FullName = nom
				retrouvees = append(retrouvees, cours.Students[position])
				complets++
			}
		}
		// Un nom retrouvé une fois vaut pour toute l'organisation : le mettre au
		// registre évite de le rechercher groupe après groupe.
		if err := s.apprendre(cours.Org, retrouvees...); err != nil {
			return nil, err
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
	Name     string `json:"name"`
	Student  string `json:"student"`
	FullName string `json:"full_name"`
	Username string `json:"username"`
	// Team nomme l'équipe destinataire, et Members ses membres : un dépôt
	// d'équipe ne porte le nom de personne, il faut donc dire qui il concerne —
	// et le dire par leur nom, pas par leur seul compte.
	Team       string          `json:"team,omitempty"`
	Members    []roster.Person `json:"members,omitempty"`
	Private    bool            `json:"private"`
	Visibility string          `json:"visibility"`
	URL        string          `json:"url"`
	PushedAt   string          `json:"pushed_at"`
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
	cours = s.enrichi(cours, repos)
	return cours, cours.AssignmentID(nom), repos, nil
}

// handleAssignment renvoie les dépôts d'un travail, étudiant par étudiant.
//
// La liste se filtre et se trie comme celle des étudiants, et par le même
// paquet : un travail, c'est un dépôt par personne — exactement ce que
// l'assistant du terminal montre d'un préfixe. « Avant le 1er octobre » y veut
// donc dire la même chose qu'ailleurs.
func (s *Server) handleAssignment(writer http.ResponseWriter, request *http.Request) {
	cours, id, repos, err := s.assignmentOf(request)
	if err != nil {
		fail(writer, err)
		return
	}
	filtre, tri, decroissant, err := studentQuery(request)
	if err != nil {
		fail(writer, err)
		return
	}
	equipes, err := s.teamsIn(cours)
	if err != nil {
		fail(writer, err)
		return
	}
	trouves := cours.Repos(id, repos)
	if len(trouves) == 0 {
		fail(writer, valid.Errorf("Aucun dépôt pour le travail « %s ».", cours.ShortName(id)))
		return
	}

	// Le nom complet ne se lit pas dans le dépôt : il vient de la liste du
	// groupe, dépôt par dépôt, comme le résolveur le fournit au terminal.
	noms := make(map[string]string, len(trouves))
	parNom := make(map[string]groups.Repo, len(trouves))
	tous := make([]string, 0, len(trouves))
	for _, repo := range trouves {
		parNom[repo.Name] = repo
		tous = append(tous, repo.Name)
		if equipe, appartient := cours.TeamOf(repo.Name, equipes); appartient {
			// Un dépôt d'équipe ne porte le nom de personne : c'est celui de
			// l'équipe qui sert à le chercher et à l'ordonner.
			noms[repo.Name] = equipe.Label()
			continue
		}
		if student, inscrit := cours.StudentOf(repo.Name); inscrit {
			noms[repo.Name] = student.FullName
		}
	}
	retenues := students.Apply(
		students.FromGroup(groups.Group{Prefix: cours.ShortName(id), Repos: trouves}, noms),
		filtre, tri, decroissant)

	lignes := make([]assignmentRepo, 0, len(retenues))
	for _, retenue := range retenues {
		repo := parNom[retenue.Repos[0].Name]
		ligne := assignmentRepo{
			Name: repo.Name, Student: repo.Suffix, Private: repo.Private,
			Visibility: repo.Visibility(), URL: s.urlOf(cours.Org, repo),
			PushedAt: repo.PushedAt,
		}
		if equipe, appartient := cours.TeamOf(repo.Name, equipes); appartient {
			ligne.FullName, ligne.Team = equipe.Label(), equipe.Short
			ligne.Members = cours.Members(equipe)
		} else if student, inscrit := cours.StudentOf(repo.Name); inscrit {
			ligne.FullName, ligne.Username = student.FullName, student.Username
		}
		lignes = append(lignes, ligne)
	}
	nature := classroom.Individual
	for _, travail := range cours.Assignments(repos, equipes) {
		if strings.EqualFold(travail.ID, id) {
			nature = travail.Kind
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"id": id, "name": cours.ShortName(id), "repos": lignes, "kind": nature,
		// Le total dit combien le filtre a écarté ; « names » nomme tous les
		// dépôts du travail, filtrés compris — ce qu'on cache à l'écran ne sort
		// pas du travail pour autant.
		"shown": len(lignes), "total": len(trouves), "names": tous,
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
	Usernames []string `json:"usernames"`
	// Teams demande un travail d'équipe : un dépôt par équipe, nommé d'après
	// elle, et partagé avec elle plutôt qu'avec chacun de ses membres.
	Teams bool `json:"teams"`
	// TeamNames restreint la distribution à certaines équipes ; vide, toutes
	// celles du groupe sont servies. C'est ce qui permet de créer les dépôts
	// d'un travail d'équipe par petits lots plutôt que tous d'un coup.
	TeamNames    []string `json:"team_names"`
	DryRun       bool     `json:"dry_run"`
	ForceStarter bool     `json:"force_starter"`
}

// distribution est ce qu'un travail va produire : des dépôts pour des
// personnes, ou des dépôts pour des équipes. Les deux suivent le même chemin —
// mêmes réglages, même plan, même exécuteur — et ne se distinguent qu'ici.
type distribution struct {
	Cours    classroom.Classroom
	Settings config.Settings
	ForTeams bool

	People []roster.Person
	Teams  []teams.Team
	// Skipped nomme ce qui a déjà un dépôt pour ce travail.
	SkippedPeople []roster.Person
	SkippedTeams  []teams.Team
}

// Count dit combien de dépôts la distribution créerait.
func (d distribution) Count() int {
	if d.ForTeams {
		return len(d.Teams)
	}
	return len(d.People)
}

// Items compose le plan des dépôts à créer.
func (d distribution) Items() ([]plan.PlannedRepo, error) {
	if d.ForTeams {
		return plan.BuildTeams(classroom.TeamTargets(d.Teams), d.Settings)
	}
	// Une personne qui travaille sous deux comptes n'a qu'un dépôt : c'est son
	// nom qui le nomme, et elle y est invitée sous chacun d'eux.
	comptes := d.Cours.Accounts()
	cibles := make([]plan.Target, 0, len(d.People))
	for _, person := range d.People {
		cibles = append(cibles, plan.Target{
			Person: person, Accounts: comptes[strings.ToLower(person.Username)],
		})
	}
	return plan.BuildFor(cibles, d.Settings)
}

// Skipped nomme, pour le bilan, ce qui avait déjà un dépôt.
func (d distribution) Skipped() any {
	if d.ForTeams {
		return teamNames(d.SkippedTeams)
	}
	return d.SkippedPeople
}

// prepare valide un travail et compose sa distribution : les réglages, et qui
// reste à servir — ceux qui ont déjà un dépôt pour ce travail étant écartés.
func (s *Server) prepare(request *http.Request, body assignmentInput) (distribution, error) {
	var vide distribution
	cours, err := s.place(request)
	if err != nil {
		return vide, err
	}
	nom, err := naming.Fragment(body.Name, "Nom du travail")
	if err != nil {
		return vide, err
	}
	cours.Defaults = s.defaultsOr(body.Settings)
	settings, err := normalize(cours.Settings(nom))
	if err != nil {
		return vide, err
	}
	repos, _, err := s.repos(cours.Org, false)
	if err != nil {
		return vide, err
	}
	cours = s.enrichi(cours, repos)
	if body.Teams {
		return s.prepareTeams(cours, settings, body, repos)
	}

	// Le nom du dépôt contient le nom de l'étudiant : sans lui, il n'y a pas de
	// dépôt à nommer. Un travail d'équipe, lui, s'en passe — c'est l'équipe qui
	// nomme le dépôt.
	if incomplets := cours.MissingNames(); len(incomplets) > 0 {
		comptes := make([]string, 0, len(incomplets))
		for _, student := range incomplets {
			comptes = append(comptes, "@"+student.Username)
		}
		return vide, valid.Errorf(
			"Nom complet manquant pour %s : le nom du dépôt en dépend. "+
				"Retrouvez les noms depuis l'onglet Étudiants.", strings.Join(comptes, ", "))
	}

	// Une sélection présente, fût-elle vide, reste une sélection : cocher
	// personne ne doit pas revenir à servir tout le groupe.
	restreint := body.Usernames != nil
	voulus := map[string]bool{}
	for _, login := range body.Usernames {
		voulus[strings.ToLower(login)] = true
	}
	servis := cours.Served(settings.Assignment, repos)

	// La distribution va aux personnes, non aux comptes : deux comptes d'un
	// même nom n'ont qu'un dépôt, et cocher l'un revient à cocher la personne.
	partage := distribution{Cours: cours, Settings: settings}
	for _, identite := range cours.Identities() {
		if restreint && !voulue(identite, voulus) {
			continue
		}
		if servis[strings.ToLower(identite.Username())] {
			partage.SkippedPeople = append(partage.SkippedPeople, identite.Person())
			continue
		}
		partage.People = append(partage.People, identite.Person())
	}
	return partage, nil
}

// voulue dit qu'une personne a été retenue, sous l'un quelconque de ses comptes.
func voulue(identite classroom.Identity, voulus map[string]bool) bool {
	for _, compte := range identite.Accounts {
		if voulus[strings.ToLower(compte)] {
			return true
		}
	}
	return false
}

// prepareTeams compose la distribution d'un travail d'équipe : un dépôt par
// équipe retenue. Rien n'oblige à toutes les servir d'un coup — c'est même le
// cas courant, une équipe se formant parfois après les autres.
func (s *Server) prepareTeams(cours classroom.Classroom, settings config.Settings,
	body assignmentInput, repos []groups.RepoInfo) (distribution, error) {
	var vide distribution
	equipes, err := s.teamsIn(cours)
	if err != nil {
		return vide, err
	}
	if len(equipes) == 0 {
		return vide, valid.Errorf(
			"« %s » n'a aucune équipe : créez-en avant de distribuer un travail d'équipe.",
			cours.Label())
	}

	// Une sélection présente, fût-elle vide, reste une sélection.
	restreint := body.TeamNames != nil
	voulues := map[string]bool{}
	for _, nom := range body.TeamNames {
		court, err := teams.ShortName(nom)
		if err != nil {
			return vide, err
		}
		if _, connue := teams.Find(equipes, court); !connue {
			return vide, valid.Errorf("Aucune équipe « %s » dans « %s ».", court, cours.Label())
		}
		voulues[strings.ToLower(court)] = true
	}
	servies := cours.ServedTeams(settings.Assignment, repos, equipes)

	partage := distribution{Cours: cours, Settings: settings, ForTeams: true}
	for _, equipe := range equipes {
		if restreint && !voulues[strings.ToLower(equipe.Short)] {
			continue
		}
		if servies[strings.ToLower(equipe.Short)] {
			partage.SkippedTeams = append(partage.SkippedTeams, equipe)
			continue
		}
		partage.Teams = append(partage.Teams, equipe)
	}
	return partage, nil
}

// handlePreviewAssignment montre les dépôts qui seraient créés, sans rien écrire.
func (s *Server) handlePreviewAssignment(writer http.ResponseWriter, request *http.Request) {
	var body assignmentInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	partage, err := s.prepare(request, body)
	if err != nil {
		fail(writer, err)
		return
	}
	items, err := partage.Items()
	if err != nil && partage.Count() > 0 {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"assignment": partage.Settings.Assignment,
		"short_name": partage.Cours.ShortName(partage.Settings.Assignment),
		"items":      rows(items),
		"served":     partage.Skipped(),
		"teams":      partage.ForTeams,
	})
}

// handleCreateAssignment distribue un travail aux étudiants du groupe.
func (s *Server) handleCreateAssignment(writer http.ResponseWriter, request *http.Request) {
	var body assignmentInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	partage, err := s.prepare(request, body)
	if err != nil {
		fail(writer, err)
		return
	}
	if partage.Count() == 0 {
		fail(writer, valid.Errorf("Rien à distribuer : %s retenu%s a déjà un dépôt pour ce travail.",
			destinataires(partage.ForTeams), plurielDes(partage.ForTeams)))
		return
	}
	cours, settings := partage.Cours, partage.Settings
	items, err := partage.Items()
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

	quoi := " étudiant(s)"
	if partage.ForTeams {
		quoi = " équipe(s)"
	}
	label := "Distribution de « " + cours.ShortName(settings.Assignment) + " » à " +
		itoa(len(items)) + quoi
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
			// Voir « updateInventory » : une création se relit, un renommage se suit.
			s.forget(cours.Org)
		}

		bilan := map[string]any{
			"report": report, "assignment": settings.Assignment,
			"short_name": cours.ShortName(settings.Assignment),
			"created":    report.Count(runner.Created),
			"existing":   report.Count(runner.Existing),
			"failed":     len(report.Failures()),
			"dry_run":    body.DryRun,
			"skipped":    partage.Skipped(),
			"teams":      partage.ForTeams,
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

// destinataires nomme, au singulier collectif, à qui un travail s'adresse.
func destinataires(equipes bool) string {
	if equipes {
		return "toutes les équipes"
	}
	return "tous les étudiants"
}

func plurielDes(equipes bool) string {
	if equipes {
		return "es"
	}
	return "s"
}

// ----------------------------------------------------------------- candidats
