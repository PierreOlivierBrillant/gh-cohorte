package web

import (
	"net/http"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Reprendre un travail fait en équipe. Le parcours est celui de la reprise
// ordinaire — mêmes écrans, mêmes routes — et ne s'en écarte qu'ici : ce qui
// suit le préfixe nomme une équipe, les accès du dépôt disent qui en est, et
// rien n'est rapproché d'une liste.
//
// Les écritures partent dans un ordre qui laisse toujours quelque chose de
// rattrapable : les dépôts d'abord, les équipes ensuite, le partage en dernier.
// Un renommage qui échoue arrête tout ; une équipe qui échoue laisse des dépôts
// correctement nommés, que « Partager avec les équipes » achèvera.

// teamImportPlan compose la reprise d'un travail d'équipe.
func (s *Server) teamImportPlan(org string, body importInput) (
	classroom.TeamImport, classroom.Classroom, error) {
	var vide classroom.Classroom
	arrivee, err := classroom.AtScope(org, body.Scope, classroom.DefaultsFrom(s.Settings()))
	if err != nil {
		return classroom.TeamImport{}, vide, err
	}
	repos, _, err := s.repos(org, false)
	if err != nil {
		return classroom.TeamImport{}, vide, err
	}
	equipes, err := s.teamsIn(arrivee)
	if err != nil {
		return classroom.TeamImport{}, vide, err
	}
	entrees, deviner, err := s.entries(body)
	if err != nil {
		return classroom.TeamImport{}, vide, err
	}
	membres := s.membres(org, body.Prefix, body.Only, repos)
	demande := classroom.TeamImportRequest{
		Prefix: body.Prefix, Name: body.Name, Only: body.Only,
		Members:  membres,
		Known:    s.connus(org),
		Existing: equipes,
		Chosen:   body.Crews,
		Entries:  entrees,
		Guess:    deviner,
	}
	if deviner {
		// Les profils GitHub ne servent qu'à la première lecture : après une
		// correction, plus rien n'est deviné.
		demande.Profiles = s.profilsDesMembres(org, membres)
	}
	plan, err := classroom.PlanTeamImport(arrivee, demande, repos)
	return plan, arrivee, err
}

// profilsDesMembres retrouve le nom affiché du profil GitHub de chaque membre.
// C'est l'indice le plus sûr après le numéro d'étudiant, et le seul dont on
// dispose quand la liste ne porte aucun compte.
func (s *Server) profilsDesMembres(org string,
	equipes map[string]identity.Crew) map[string]string {
	vus := map[string]bool{}
	pairs := make([]identity.Pair, 0, len(equipes))
	for _, crew := range equipes {
		for _, membre := range crew.Members {
			if vus[strings.ToLower(membre.Login)] {
				continue
			}
			vus[strings.ToLower(membre.Login)] = true
			// Le compte sert de clé comme de question : c'est son profil qu'on
			// demande, et c'est par lui que le rapprochement le retrouve.
			pairs = append(pairs, identity.Pair{Repo: membre.Login, Login: membre.Login})
		}
	}
	if len(pairs) == 0 {
		return nil
	}
	profils := map[string]string{}
	for compte, nom := range s.resolver(org).Resolve(pairs, true, nil) {
		if nom != "" {
			profils[strings.ToLower(compte)] = nom
		}
	}
	return profils
}

// handleTeamImport reprend un travail d'équipe : les dépôts changent de nom,
// les équipes sont reconstituées, et chacune reçoit le sien.
func (s *Server) handleTeamImport(writer http.ResponseWriter, org string, body importInput) {
	plan, arrivee, err := s.teamImportPlan(org, body)
	if err != nil {
		fail(writer, err)
		return
	}
	if !plan.Ready() {
		fail(writer, valid.Errorf("Aucun dépôt à reprendre pour « %s ».", body.Prefix))
		return
	}
	// Le registre passe en premier : un nom qui n'y monterait pas ne serait
	// connu que de ce poste.
	if err := s.apprendre(org, plan.Students...); err != nil {
		fail(writer, err)
		return
	}

	label := "Reprise en équipe de « " + plan.Prefix + " » vers " + arrivee.Scope()
	job := s.jobs.Start("importation", label, func(job *Job) (any, error) {
		renommes, echecs := 0, 0
		var suivis []groups.Renamed
		for index, ligne := range plan.Moves {
			if job.Canceled() {
				break
			}
			apres, err := s.deps.Client.RenameRepo(org, ligne.Repo, ligne.Target)
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
			job.Progress(index+1, len(plan.Moves), ligne.Repo)
		}
		s.renamed(org, suivis)

		bilan := map[string]any{
			"team_work": true, "renamed": renommes, "failed": echecs,
			"scope": arrivee.Scope(), "students": len(plan.Students),
			"teams": 0, "shared": 0,
		}
		if echecs > 0 || job.Canceled() {
			job.Warn("Les équipes n'ont pas été créées : tous les dépôts n'ont pas suivi.")
			return bilan, nil
		}

		faites, partages, ratees := s.reconstituer(job, arrivee, plan)
		bilan["teams"], bilan["shared"], bilan["team_failed"] = faites, partages, ratees

		enregistre, err := s.classrooms.Save(arrivee.With(plan.Students...))
		if err != nil {
			return nil, err
		}
		bilan["scope"] = enregistre.Scope()
		return bilan, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// reconstituer crée les équipes, y inscrit leurs membres, et leur partage leur
// dépôt. Une équipe en échec n'arrête pas les suivantes : le travail est déjà
// bien nommé, et ce qui manque se rattrape.
func (s *Server) reconstituer(job *Job, arrivee classroom.Classroom,
	plan classroom.TeamImport) (faites, partages, ratees int) {
	droit := arrivee.Settings(plan.Name).Permission
	for index, equipe := range plan.Teams {
		if job.Canceled() {
			break
		}
		slug, err := s.assurerEquipe(arrivee, equipe)
		if err != nil {
			ratees++
			job.Line("équipe "+equipe.Short+" : échec — "+err.Error(),
				map[string]string{"status": "échec"})
			continue
		}
		faites++
		job.Line("équipe "+equipe.Short+" — "+membresEnMots(equipe.Members),
			map[string]string{"status": "composée"})

		if err := s.deps.Client.GrantTeamRepo(
			arrivee.Org, slug, arrivee.Org, equipe.Target, droit); err != nil {
			job.Line(equipe.Target+" : partage impossible — "+err.Error(),
				map[string]string{"status": "échec"})
			continue
		}
		partages++
		job.Progress(index+1, len(plan.Teams), equipe.Short)
	}
	s.forgetTeams(arrivee.Org)
	return faites, partages, ratees
}

// assurerEquipe crée l'équipe si elle manque, puis y inscrit ses membres. Elle
// rend l'adresse GitHub de l'équipe, celle par laquelle on lui partage un dépôt.
func (s *Server) assurerEquipe(arrivee classroom.Classroom,
	equipe classroom.ImportedTeam) (string, error) {
	infos, err := s.orgTeams(arrivee.Org, true)
	if err != nil {
		return "", err
	}
	presentes := arrivee.Teams(infos)
	trouvee, existe := teams.Find(presentes, equipe.Short)
	if !existe {
		cree, err := s.deps.Client.CreateTeam(arrivee.Org, equipe.Name,
			teams.Describe(arrivee.Session, arrivee.Course, arrivee.Group, equipe.Short),
			teams.Privacy)
		if err != nil {
			return "", err
		}
		trouvee, _ = teams.Read(*cree)
		presentes = append(presentes, trouvee)
	}

	// Composer plutôt qu'inscrire : la reprise dit ce que l'équipe est, et
	// relancer la même reprise ne doit rien changer de plus.
	etapes, err := teams.PlanCompose(presentes, equipe.Short, equipe.Members)
	if err != nil {
		return trouvee.Slug, err
	}
	if _, echecs := s.applyTeamSteps(arrivee.Org, etapes); len(echecs) > 0 {
		return trouvee.Slug, valid.Errorf("%s", strings.Join(echecs, " ; "))
	}
	return trouvee.Slug, nil
}

// membresEnMots dit la composition d'une équipe pour le journal.
func membresEnMots(membres []string) string {
	if len(membres) == 0 {
		return "aucun membre connu"
	}
	comptes := make([]string, 0, len(membres))
	for _, compte := range membres {
		comptes = append(comptes, "@"+compte)
	}
	return strings.Join(comptes, ", ")
}
