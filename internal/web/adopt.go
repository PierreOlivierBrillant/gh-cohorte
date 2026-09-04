package web

import (
	"net/http"
	"sort"
	"strconv"
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

// movePlace décrit un groupe d'arrivée qui n'existe pas encore : le
// déplacement le déclare au passage, plutôt que d'obliger à le créer d'abord
// puis à revenir.
type movePlace struct {
	Session string `json:"session"`
	Course  string `json:"course"`
	Group   string `json:"group"`
}

// moveInput est ce que l'interface envoie pour déplacer des étudiants.
type moveInput struct {
	// Username désigne une personne seule ; Usernames, plusieurs. Les deux se
	// cumulent : l'interface envoie l'un ou l'autre selon ce qui est coché.
	Username  string   `json:"username"`
	Usernames []string `json:"usernames"`
	// Target est la place d'un groupe qui existe déjà.
	Target string `json:"target"`
	// NewGroup, à sa place, décrit un groupe à déclarer pour l'occasion.
	NewGroup *movePlace `json:"new_group"`
	// Repos demande de renommer aussi les dépôts des personnes déplacées pour
	// qu'ils rejoignent le groupe d'arrivée.
	Repos bool `json:"repos"`
}

// handleMoveStudent déplace une ou plusieurs personnes d'un groupe vers un
// autre — existant, ou déclaré pour l'occasion. Leurs fiches suivent ; leurs
// dépôts, eux, ne bougent que si on le demande.
func (s *Server) handleMoveStudent(writer http.ResponseWriter, request *http.Request) {
	var body moveInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}

	depart, err := s.place(request)
	if err != nil {
		fail(writer, err)
		return
	}
	repos, _, err := s.repos(depart.Org, false)
	if err != nil {
		fail(writer, err)
		return
	}
	arrivee, neuf, err := s.moveTarget(body, repos)
	if err != nil {
		fail(writer, err)
		return
	}
	if classroom.NormalizeScope(depart.Scope()) == classroom.NormalizeScope(arrivee.Scope()) {
		fail(writer, valid.Errorf("Le groupe d'arrivée est celui de départ."))
		return
	}

	personnes, err := movers(depart, arrivee, body)
	if err != nil {
		fail(writer, err)
		return
	}

	// Les dépôts sont relevés avant de toucher aux listes : après, le groupe
	// de départ ne reconnaîtrait plus les siens.
	var renommages []classroom.Move
	if body.Repos {
		renommages, err = classroom.PlanMove(depart, arrivee, personnes, repos)
		if err != nil {
			fail(writer, err)
			return
		}
	}

	if _, err := s.classrooms.Save(depart.Without(comptes(personnes)...)); err != nil {
		fail(writer, err)
		return
	}
	if _, err := s.classrooms.Save(arrivee.With(personnes...)); err != nil {
		fail(writer, err)
		return
	}

	bilan := map[string]any{
		"moved": comptes(personnes), "count": len(personnes),
		"target": arrivee.Label(), "target_scope": arrivee.Scope(),
		"created": neuf, "renamed": 0,
	}
	if len(renommages) == 0 {
		writeJSON(writer, http.StatusOK, bilan)
		return
	}

	label := "Déplacement de " + personnesEnMots(personnes) +
		" vers « " + arrivee.Label() + " »"
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
		bilan["renamed"], bilan["failed"] = renommes, echecs
		return bilan, nil
	})
	writeJSON(writer, http.StatusAccepted, job.State())
}

// moveTarget résout le groupe d'arrivée : une place déjà occupée, ou une place
// libre que le déplacement déclare. Déclarer sur une place occupée serait une
// fusion déguisée : elle est refusée, la place se choisit alors dans la liste.
func (s *Server) moveTarget(body moveInput, repos []groups.RepoInfo) (
	classroom.Classroom, bool, error) {
	if body.NewGroup == nil {
		arrivee, err := s.placeAt(body.Target)
		return arrivee, false, err
	}
	session, err := naming.Fragment(body.NewGroup.Session, "Session")
	if err != nil {
		return classroom.Classroom{}, false, err
	}
	course, err := naming.Fragment(body.NewGroup.Course, "Cours")
	if err != nil {
		return classroom.Classroom{}, false, err
	}
	group, err := naming.Fragment(body.NewGroup.Group, "Groupe")
	if err != nil {
		return classroom.Classroom{}, false, err
	}
	place := naming.Prefix(session, course, group)
	for _, connu := range s.visibles(s.org(), repos) {
		if classroom.NormalizeScope(connu.Scope()) == classroom.NormalizeScope(place) {
			return classroom.Classroom{}, false, valid.Errorf(
				"« %s » existe déjà : choisissez-le dans la liste des groupes.", place)
		}
	}
	arrivee, err := classroom.AtScope(s.org(), place, classroom.DefaultsFrom(s.Settings()))
	return arrivee, true, err
}

// movers rassemble les personnes à déplacer, en refusant d'emblée celles que le
// groupe de départ ne connaît pas et celles que l'arrivée connaît déjà.
func movers(depart, arrivee classroom.Classroom, body moveInput) ([]roster.Person, error) {
	demandes := append([]string(nil), body.Usernames...)
	if strings.TrimSpace(body.Username) != "" {
		demandes = append(demandes, body.Username)
	}

	vus := map[string]bool{}
	personnes := make([]roster.Person, 0, len(demandes))
	for _, demande := range demandes {
		username, err := valid.Login(demande, "Compte GitHub")
		if err != nil {
			return nil, err
		}
		if vus[strings.ToLower(username)] {
			continue
		}
		vus[strings.ToLower(username)] = true

		personne, trouvee := depart.Find(username)
		if !trouvee {
			return nil, valid.Errorf("@%s n'est pas dans « %s ».", username, depart.Label())
		}
		if arrivee.Has(personne.Username) {
			return nil, valid.Errorf("@%s est déjà dans « %s ».",
				personne.Username, arrivee.Label())
		}
		personnes = append(personnes, personne)
	}
	if len(personnes) == 0 {
		return nil, valid.Errorf("Aucune personne à déplacer.")
	}
	return personnes, nil
}

// comptes rend les comptes GitHub d'une liste de personnes.
func comptes(personnes []roster.Person) []string {
	liste := make([]string, 0, len(personnes))
	for _, personne := range personnes {
		liste = append(liste, personne.Username)
	}
	return liste
}

// personnesEnMots nomme ce qu'un déplacement emporte, pour le titre du journal.
func personnesEnMots(personnes []roster.Person) string {
	if len(personnes) == 1 {
		return "@" + personnes[0].Username
	}
	return strconv.Itoa(len(personnes)) + " étudiants"
}
