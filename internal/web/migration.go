package web

import (
	"net/http"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Migrer un groupe, c'est renommer ses dépôts pour qu'ils suivent la
// nomenclature à quatre niveaux. GitHub garde une redirection depuis l'ancien
// nom : les clones et les liens déjà distribués continuent de fonctionner.

// migrationRow est un dépôt à renommer, ou la raison qui l'en empêche.
type migrationRow struct {
	Repo     string `json:"repo"`
	Target   string `json:"target,omitempty"`
	Student  string `json:"student,omitempty"`
	Username string `json:"username,omitempty"`
	Problem  string `json:"problem,omitempty"`
}

// Ready dit si la ligne peut être renommée.
func (r migrationRow) Ready() bool { return r.Problem == "" && r.Target != "" }

// migrationInput est ce que l'interface envoie pour préparer ou lancer une
// migration.
type migrationInput struct {
	Course string `json:"course"`
	Group  string `json:"group"`
	// SkipBlocked laisse en place les dépôts qui ne peuvent pas être renommés,
	// au lieu de refuser la migration entière.
	SkipBlocked bool `json:"skip_blocked"`
}

// migrationPlan compose le renommage de tous les dépôts d'un groupe hérité.
func (s *Server) migrationPlan(request *http.Request, body migrationInput) (
	classroom.Classroom, string, string, []migrationRow, error) {
	cours, ok := s.classrooms.Get(request.PathValue("id"))
	if !ok {
		return cours, "", "", nil, valid.Errorf("Groupe inconnu.")
	}
	if !cours.Legacy() {
		return cours, "", "", nil, valid.Errorf(
			"« %s » suit déjà la nomenclature à quatre niveaux.", cours.Name)
	}
	course, err := naming.Fragment(body.Course, "Cours")
	if err != nil {
		return cours, "", "", nil, err
	}
	group, err := naming.Fragment(body.Group, "Groupe")
	if err != nil {
		return cours, "", "", nil, err
	}

	repos, _, err := s.repos(cours.Org, false)
	if err != nil {
		return cours, "", "", nil, err
	}
	existants := map[string]bool{}
	for _, repo := range repos {
		existants[strings.ToLower(repo.Name)] = true
	}

	var lignes []migrationRow
	vises := map[string]string{} // nom visé → dépôt qui le vise déjà
	for _, travail := range cours.Assignments(repos) {
		nom, err := naming.Fragment(cours.ShortName(travail.ID), "Travail")
		if err != nil {
			nom = ""
		}
		for _, repo := range cours.Repos(travail.ID, repos) {
			ligne := migrationRow{Repo: repo.Name}
			student, inscrit := cours.StudentOf(repo.Name)
			switch {
			case nom == "":
				ligne.Problem = "nom de travail illisible"
			case !inscrit:
				ligne.Problem = "compte « " + repo.Suffix + " » absent de la liste du groupe"
			default:
				ligne.Username = student.Username
				fragment, err := naming.Student(student.FullName)
				if err != nil {
					ligne.Problem = "nom complet manquant pour @" + student.Username
					break
				}
				ligne.Student = fragment
				ligne.Target = naming.Compose(course, group, nom, fragment)
			}

			if ligne.Target != "" {
				cle := strings.ToLower(ligne.Target)
				switch {
				case vises[cle] != "":
					ligne.Problem = "même nom visé que « " + vises[cle] + " »"
					ligne.Target = ""
				case existants[cle]:
					ligne.Problem = "« " + ligne.Target + " » existe déjà"
					ligne.Target = ""
				default:
					vises[cle] = repo.Name
				}
			}
			lignes = append(lignes, ligne)
		}
	}
	return cours, course, group, lignes, nil
}

// handleMigrationPreview montre le renommage sans rien écrire.
func (s *Server) handleMigrationPreview(writer http.ResponseWriter, request *http.Request) {
	var body migrationInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	cours, course, group, lignes, err := s.migrationPlan(request, body)
	if err != nil {
		fail(writer, err)
		return
	}
	prets, bloques := 0, 0
	for _, ligne := range lignes {
		if ligne.Ready() {
			prets++
		} else {
			bloques++
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"prefix": cours.LegacyPrefix, "course": course, "group": group,
		"rows": lignes, "ready": prets, "blocked": bloques,
	})
}

// handleMigrationApply renomme les dépôts, puis fait passer le groupe à la
// nouvelle nomenclature.
func (s *Server) handleMigrationApply(writer http.ResponseWriter, request *http.Request) {
	var body migrationInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	cours, course, group, lignes, err := s.migrationPlan(request, body)
	if err != nil {
		fail(writer, err)
		return
	}

	var prets []migrationRow
	var bloques []migrationRow
	for _, ligne := range lignes {
		if ligne.Ready() {
			prets = append(prets, ligne)
		} else {
			bloques = append(bloques, ligne)
		}
	}
	if len(bloques) > 0 && !body.SkipBlocked {
		fail(writer, valid.Errorf(
			"%d dépôt(s) ne peuvent pas être renommés : corrigez la liste des étudiants, "+
				"ou acceptez de les laisser en place.", len(bloques)))
		return
	}
	if len(prets) == 0 {
		fail(writer, valid.Errorf("Aucun dépôt à renommer."))
		return
	}

	label := "Migration de « " + cours.Name + " » vers " + naming.Prefix(course, group)
	job := s.jobs.Start("migration", label, func(job *Job) (any, error) {
		renommes, echecs := 0, 0
		for index, ligne := range prets {
			if job.Canceled() {
				break
			}
			_, err := s.deps.Client.RenameRepo(cours.Org, ligne.Repo, ligne.Target)
			if err != nil {
				echecs++
				job.Line(ligne.Repo+" : échec — "+err.Error(),
					map[string]string{"status": "échec"})
			} else {
				renommes++
				job.Line(ligne.Repo+" → "+ligne.Target,
					map[string]string{"status": "mis à jour"})
			}
			job.Progress(index+1, len(prets), ligne.Repo)
		}
		for _, ligne := range bloques {
			job.Warn(ligne.Repo + " laissé en place : " + ligne.Problem)
		}
		s.forget(cours.Org)

		// Le groupe ne bascule que si tout ce qui devait être renommé l'a été :
		// sinon il cesserait de voir les dépôts restés en arrière.
		bascule := echecs == 0 && !job.Canceled()
		if bascule {
			cours.Course, cours.Group, cours.LegacyPrefix = course, group, ""
			if _, err := s.classrooms.Update(cours); err != nil {
				return nil, err
			}
		}
		return map[string]any{
			"renamed": renommes, "failed": echecs, "skipped": len(bloques),
			"switched": bascule, "course": course, "group": group,
		}, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}
