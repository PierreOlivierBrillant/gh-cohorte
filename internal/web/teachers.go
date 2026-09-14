package web

import (
	"net/http"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Cloisonner un groupe touche à qui voit quoi. Comme tout ce qui écrit dans cet
// outil, l'opération montre d'abord ce qu'elle ferait : ces écritures ouvrent
// et ferment des accès aux copies d'étudiants, et personne ne devrait avoir à
// les découvrir après coup.

// teachingRendu est ce que l'aperçu et l'application renvoient tous deux.
type teachingRendu struct {
	Scope string             `json:"scope"`
	State classroom.Teaching `json:"state"`
	// Candidates énumère les enseignants de l'organisation : ceux qu'on peut
	// inscrire à l'équipe du groupe. On ne peut inscrire que quelqu'un que le
	// registre déclare enseignant, et la page n'a pas à le deviner.
	Candidates []teacherChoice `json:"candidates"`
	// Notice dit ce que l'équipe ne peut pas fermer.
	Notice string `json:"notice,omitempty"`
	// Exposure reprend l'avertissement du registre sur la permission de base :
	// c'est elle, et non l'équipe, qui décide de ce qui reste fermé.
	Exposure string `json:"exposure,omitempty"`
}

// teacherChoice est un enseignant qu'on peut inscrire à l'équipe d'un groupe.
type teacherChoice struct {
	Username string `json:"username"`
	FullName string `json:"full_name,omitempty"`
	// Member dit qu'il est déjà de l'équipe de ce groupe.
	Member bool `json:"member"`
}

// teachersOfOrg énumère les enseignants déclarés, en disant lesquels sont déjà
// de l'équipe du groupe.
func teachersOfOrg(set *registry.Set, equipe teams.Team, existe bool) []teacherChoice {
	choix := make([]teacherChoice, 0)
	for _, fiche := range set.Teachers() {
		choix = append(choix, teacherChoice{
			Username: fiche.Username, FullName: fiche.FullName,
			Member: existe && equipe.Has(fiche.Username),
		})
	}
	return choix
}

// teacherGrant rend l'équipe enseignante d'un groupe sous la forme que le
// runner attend : le slug, et le droit à lui donner.
//
// Un groupe non cloisonné n'en a pas, et le runner n'accorde alors rien. Une
// lecture qui échoue ne fait pas échouer la distribution : mieux vaut un dépôt
// créé sans l'accès de l'équipe — que « cloisonner » redonnera — qu'aucun
// dépôt du tout.
func (s *Server) teacherGrant(cours classroom.Classroom) (string, string) {
	infos, err := s.orgTeams(cours.Org, false)
	if err != nil {
		return "", ""
	}
	equipe, existe := cours.TeacherTeam(infos)
	if !existe {
		return "", ""
	}
	return equipe.Slug, classroom.TeacherPermission
}

// handleTeaching décrit le cloisonnement d'un groupe : son équipe enseignante,
// qui en est, et qui pourrait en être.
func (s *Server) handleTeaching(writer http.ResponseWriter, request *http.Request) {
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
	infos, err := s.orgTeams(cours.Org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}
	set, _ := s.names(cours.Org)
	equipe, existe := cours.TeacherTeam(infos)

	writeJSON(writer, http.StatusOK, teachingRendu{
		Scope:      cours.Scope(),
		State:      cours.TeachingOf(infos, repos, set),
		Candidates: teachersOfOrg(set, equipe, existe),
		Notice:     classroom.OwnersSeeAll,
		Exposure:   s.registryOf(cours.Org).Exposure(),
	})
}

// handleTeachingPreview montre ce que cloisonner ferait, sans rien écrire.
func (s *Server) handleTeachingPreview(writer http.ResponseWriter, request *http.Request) {
	cours, plan, err := s.planTeaching(request)
	if err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"scope": cours.Scope(), "plan": plan,
	})
}

// handleTeachingApply écrit ce que l'aperçu a montré.
//
// L'ordre du plan est tenu : les retraits d'abord, puis les inscriptions, puis
// les accès. Si l'opération s'interrompt, mieux vaut un accès de moins qu'un
// accès de trop.
func (s *Server) handleTeachingApply(writer http.ResponseWriter, request *http.Request) {
	cours, plan, err := s.planTeaching(request)
	if err != nil {
		fail(writer, err)
		return
	}
	if plan.Empty() {
		fail(writer, valid.Errorf(
			"Rien à faire : l'équipe enseignante de « %s » est déjà celle-là.", cours.Scope()))
		return
	}

	job := s.jobs.Start("cloisonnement",
		"Équipe enseignante de « "+cours.Scope()+" »", func(job *Job) (any, error) {
			slug := plan.Slug
			for index, etape := range plan.Steps {
				if job.Canceled() {
					break
				}
				if slug, err = s.applyTeachingStep(cours, slug, plan, etape); err != nil {
					return nil, err
				}
				job.Progress(index+1, len(plan.Steps), etape.Kind+" · "+etape.Target)
				job.Line(etape.Kind+" : "+etape.Target, etape)
			}
			// Les équipes de l'organisation ont bougé : ce qu'on en retenait
			// ne vaut plus.
			s.forgetTeams(cours.Org)
			return map[string]any{"team": plan.Team, "slug": slug}, nil
		})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// applyTeachingStep exécute une étape du plan et rend le slug de l'équipe, qui
// n'est connu qu'une fois celle-ci créée.
func (s *Server) applyTeachingStep(cours classroom.Classroom, slug string,
	plan classroom.TeachingPlan, etape classroom.TeachingStep) (string, error) {
	client := s.deps.Client
	switch etape.Kind {
	case classroom.CreateTeam:
		info, err := client.CreateTeam(cours.Org, plan.Team, plan.Description, teams.Privacy)
		if err != nil {
			return slug, err
		}
		return info.Slug, nil
	case classroom.JoinTeam:
		// Un enseignant est responsable de son équipe : contrairement aux
		// étudiants, il doit pouvoir y inscrire un collègue sans passer par
		// un propriétaire de l'organisation.
		return slug, client.AddTeamMember(cours.Org, slug, etape.Target, "maintainer")
	case classroom.LeaveTeam:
		return slug, client.RemoveTeamMember(cours.Org, slug, etape.Target)
	case classroom.GrantRepo:
		return slug, client.GrantTeamRepo(
			cours.Org, slug, cours.Org, etape.Target, plan.Permission)
	}
	return slug, valid.Errorf("Étape inconnue : « %s ».", etape.Kind)
}

// planTeaching lit la composition demandée et compose le plan.
func (s *Server) planTeaching(request *http.Request) (
	classroom.Classroom, classroom.TeachingPlan, error) {
	cours, err := s.place(request)
	if err != nil {
		return cours, classroom.TeachingPlan{}, err
	}
	var body struct {
		Teachers []string `json:"teachers"`
	}
	if err := decode(request, &body); err != nil {
		return cours, classroom.TeachingPlan{}, err
	}
	repos, _, err := s.repos(cours.Org, false)
	if err != nil {
		return cours, classroom.TeachingPlan{}, err
	}
	infos, err := s.orgTeams(cours.Org, false)
	if err != nil {
		return cours, classroom.TeachingPlan{}, err
	}

	set, _ := s.names(cours.Org)
	// Celui qui cloisonne doit rester dans l'équipe : s'en retirer lui ferait
	// perdre l'accès au groupe qu'il vient de cloisonner, et il n'aurait plus
	// aucun chemin pour y revenir.
	if len(body.Teachers) > 0 && set.Teaches(s.deps.Viewer) &&
		!containsFold(body.Teachers, s.deps.Viewer) {
		return cours, classroom.TeachingPlan{}, valid.Errorf(
			"@%s ne figure pas dans la composition demandée : vous perdriez l'accès à "+
				"« %s » sans pouvoir y revenir. Ajoutez-vous, ou faites-le faire par un "+
				"collègue déjà inscrit.", s.deps.Viewer, cours.Scope())
	}
	plan, err := cours.PlanTeaching(infos, repos, body.Teachers, set)
	return cours, plan, err
}

// containsFold dit si un compte figure dans une liste, casse ignorée.
func containsFold(liste []string, valeur string) bool {
	for _, item := range liste {
		if strings.EqualFold(strings.TrimSpace(item), valeur) {
			return true
		}
	}
	return false
}
