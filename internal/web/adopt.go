package web

import (
	"net/http"
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Adopter des dépôts que rien n'organise. La détection automatique propose ce
// qu'elle sait deviner ; quand elle ne devine rien — et beaucoup
// d'organisations n'ont jamais suivi de convention —, un gabarit écrit à la
// main dit où lire le travail et la personne. Cette route ne fait qu'essayer :
// rien n'est déclaré tant que le groupe n'est pas créé.

// matchRow est un dépôt lu par le gabarit.
type matchRow struct {
	Repo       string `json:"repo"`
	Assignment string `json:"assignment"`
	Student    string `json:"student"`
}

// handleMatchPattern confronte un gabarit aux dépôts de l'organisation.
func (s *Server) handleMatchPattern(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Pattern string `json:"pattern"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	gabarit, err := classroom.ParsePattern(body.Pattern)
	if err != nil {
		fail(writer, err)
		return
	}
	repos, source, err := s.repos(request.PathValue("org"),
		request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}

	noms := make([]string, 0, len(repos))
	for _, repo := range repos {
		noms = append(noms, repo.Name)
	}
	// Les noms s'éclairent les uns les autres : un travail reconnu ailleurs
	// tranche là où un nom seul resterait ambigu.
	decoupes := gabarit.Resolve(noms)

	lignes := make([]matchRow, 0, len(decoupes))
	travaux := map[string]bool{}
	etudiants := map[string]bool{}
	for _, decoupe := range decoupes {
		lignes = append(lignes, matchRow{
			Repo: decoupe.Repo, Assignment: decoupe.Assignment, Student: decoupe.Student,
		})
		if decoupe.Assignment != "" {
			travaux[decoupe.Assignment] = true
		}
		etudiants[decoupe.Student] = true
	}
	sort.Slice(lignes, func(i, j int) bool {
		return strings.ToLower(lignes[i].Repo) < strings.ToLower(lignes[j].Repo)
	})

	writeJSON(writer, http.StatusOK, map[string]any{
		"pattern": gabarit.String(), "prefix": gabarit.Prefix(),
		"rows": lignes, "matched": len(lignes), "total": len(repos),
		"assignments": triees(travaux), "students": triees(etudiants),
		"source": source,
	})
}

// triees rend un ensemble en liste ordonnée.
func triees(ensemble map[string]bool) []string {
	liste := make([]string, 0, len(ensemble))
	for valeur := range ensemble {
		liste = append(liste, valeur)
	}
	sort.Slice(liste, func(i, j int) bool {
		return strings.ToLower(liste[i]) < strings.ToLower(liste[j])
	})
	return liste
}

// handleMoveStudent déplace un étudiant d'un groupe vers un autre. Sa fiche
// suit ; ses dépôts, eux, ne bougent que si on le demande — les renommer est
// une écriture sur GitHub, pas un rangement local.
func (s *Server) handleMoveStudent(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Username string `json:"username"`
		// Target est la place du groupe d'arrivée.
		Target string `json:"target"`
		// Repos demande de renommer aussi les dépôts de la personne pour
		// qu'ils rejoignent le groupe d'arrivée.
		Repos bool `json:"repos"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}

	depart, err := s.place(request)
	if err != nil {
		fail(writer, err)
		return
	}
	arrivee, err := s.placeAt(body.Target)
	if err != nil {
		fail(writer, err)
		return
	}
	if classroom.NormalizeScope(depart.Scope()) == classroom.NormalizeScope(arrivee.Scope()) {
		fail(writer, valid.Errorf("Le groupe d'arrivée est celui de départ."))
		return
	}

	personne, trouvee := depart.Find(body.Username)
	if !trouvee {
		fail(writer, valid.Errorf(
			"@%s n'est pas dans « %s ».", body.Username, depart.Label()))
		return
	}
	if arrivee.Has(personne.Username) {
		fail(writer, valid.Errorf(
			"@%s est déjà dans « %s ».", personne.Username, arrivee.Label()))
		return
	}

	// Les dépôts sont relevés avant de toucher aux listes : après, le groupe
	// de départ ne reconnaîtrait plus les siens.
	var renommages []migrationRow
	if body.Repos {
		repos, _, err := s.repos(depart.Org, false)
		if err != nil {
			fail(writer, err)
			return
		}
		renommages, err = s.moveRows(depart, arrivee, personne, repos)
		if err != nil {
			fail(writer, err)
			return
		}
	}

	depart.Students = sansPersonne(depart.Students, personne.Username)
	arrivee.Students = append(append([]roster.Person(nil), arrivee.Students...), personne)
	if _, err := s.classrooms.Save(depart); err != nil {
		fail(writer, err)
		return
	}
	if _, err := s.classrooms.Save(arrivee); err != nil {
		fail(writer, err)
		return
	}

	if len(renommages) == 0 {
		writeJSON(writer, http.StatusOK, map[string]any{
			"moved": personne.Username, "target": arrivee.Label(), "renamed": 0,
		})
		return
	}

	label := "Déplacement de @" + personne.Username + " vers « " + arrivee.Label() + " »"
	job := s.jobs.Start("deplacement", label, func(job *Job) (any, error) {
		renommes, echecs := 0, 0
		for index, ligne := range renommages {
			if job.Canceled() {
				break
			}
			if _, err := s.deps.Client.RenameRepo(depart.Org, ligne.Repo, ligne.Target); err != nil {
				echecs++
				job.Line(ligne.Repo+" : échec — "+err.Error(),
					map[string]string{"status": "échec"})
			} else {
				renommes++
				job.Line(ligne.Repo+" → "+ligne.Target,
					map[string]string{"status": "mis à jour"})
			}
			job.Progress(index+1, len(renommages), ligne.Repo)
		}
		s.forget(depart.Org)
		return map[string]any{
			"moved": personne.Username, "target": arrivee.Label(),
			"renamed": renommes, "failed": echecs,
		}, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// moveRows compose le renommage des dépôts d'une personne qui change de groupe.
// Un groupe d'arrivée qui ne suit pas la nomenclature courante ne sait pas
// nommer : le déplacement se fait alors sans les dépôts.
func (s *Server) moveRows(depart, arrivee classroom.Classroom, personne roster.Person,
	repos []groups.RepoInfo) ([]migrationRow, error) {
	if arrivee.Legacy() {
		return nil, valid.Errorf(
			"« %s » suit une nomenclature dépassée : ses dépôts ne peuvent pas être "+
				"nommés. Renommez-les d'abord, ou déplacez l'étudiant sans ses dépôts.",
			arrivee.Label())
	}
	fragment, err := naming.Student(personne.FullName)
	if err != nil {
		return nil, valid.Errorf(
			"Le nom complet de @%s manque : sans lui, ses dépôts ne peuvent pas être "+
				"renommés.", personne.Username)
	}

	existants := map[string]bool{}
	for _, repo := range repos {
		existants[strings.ToLower(repo.Name)] = true
	}

	var lignes []migrationRow
	for _, travail := range depart.Assignments(repos) {
		nom, err := naming.Fragment(depart.ShortName(travail.ID), "Travail")
		if err != nil {
			continue
		}
		for _, repo := range depart.Repos(travail.ID, repos) {
			student, inscrit := depart.StudentOf(repo.Name)
			if !inscrit || !strings.EqualFold(student.Username, personne.Username) {
				continue
			}
			cible := naming.Compose(arrivee.Session, arrivee.Course, arrivee.Group, nom, fragment)
			if existants[strings.ToLower(cible)] {
				return nil, valid.Errorf("« %s » existe déjà.", cible)
			}
			lignes = append(lignes, migrationRow{
				Repo: repo.Name, Target: cible,
				Student: fragment, Username: personne.Username,
			})
		}
	}
	return lignes, nil
}

// sansPersonne retire un compte d'une liste d'étudiants.
func sansPersonne(liste []roster.Person, username string) []roster.Person {
	restants := make([]roster.Person, 0, len(liste))
	for _, personne := range liste {
		if strings.EqualFold(personne.Username, username) {
			continue
		}
		restants = append(restants, personne)
	}
	return restants
}
