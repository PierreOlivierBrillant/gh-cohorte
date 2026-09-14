package web

import (
	"net/http"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/users"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// L'annuaire regarde l'organisation entière plutôt qu'un groupe : c'est la
// seule vue où « qui a suivi quoi » se lit d'un coup, une personne par ligne et
// ses sessions en face. Tout ce qu'il montre est calculé dans « students » —
// l'interface ne fait que transmettre les critères et rendre les lignes.

// directoryRow est un étudiant de l'organisation, avec les cours qu'il a suivis.
type directoryRow struct {
	FullName    string                `json:"full_name"`
	Username    string                `json:"username"`
	Enrollments []directoryEnrollment `json:"enrollments"`
	// Repos est le nombre de dépôts de la personne, tous groupes confondus.
	Repos    int    `json:"repos"`
	PushedAt string `json:"pushed_at,omitempty"`
}

// directoryEnrollment est un cours suivi, avec les dépôts qu'il a laissés.
type directoryEnrollment struct {
	Scope       string              `json:"scope"`
	Session     string              `json:"session,omitempty"`
	SessionName string              `json:"session_name,omitempty"`
	Course      string              `json:"course,omitempty"`
	Group       string              `json:"group,omitempty"`
	Label       string              `json:"label"`
	Assignments []studentAssignment `json:"assignments"`
	PushedAt    string              `json:"pushed_at,omitempty"`
}

// directoryQuery lit les critères de l'annuaire. Ce sont ceux de la liste d'un
// groupe, plus la session et le cours — les deux seuls qu'un groupe seul ne
// pouvait pas poser.
func directoryQuery(request *http.Request) (users.Filter, users.Key, bool, error) {
	valeurs := request.URL.Query()
	filtre, err := users.Filter{
		Text:         valeurs.Get("q"),
		Session:      valeurs.Get("session"),
		Course:       valeurs.Get("course"),
		PushedAfter:  valeurs.Get("after"),
		PushedBefore: valeurs.Get("before"),
		Activity:     users.Activity(valeurs.Get("activity")),
	}.Validate()
	if err != nil {
		return filtre, users.ByName, false, err
	}
	tri, err := users.ParseKey(valeurs.Get("sort"))
	if err != nil {
		return filtre, tri, false, err
	}
	return filtre, tri, valeurs.Get("desc") == "1", nil
}

// handleDirectory dresse l'annuaire des étudiants de l'organisation : une ligne
// par personne, tous groupes confondus, avec ce qu'elle a suivi.
func (s *Server) handleDirectory(writer http.ResponseWriter, request *http.Request) {
	org := s.org()
	filtre, tri, decroissant, err := directoryQuery(request)
	if err != nil {
		fail(writer, err)
		return
	}
	repos, source, err := s.repos(org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}

	visibles := s.visibles(org, repos)
	// Les équipes disent lesquels des dépôts appartiennent à une équipe plutôt
	// qu'à personne : sans elles, l'annuaire les compterait orphelins.
	infos, _ := s.orgTeams(org, false)
	equipes := teamsOfAll(visibles, infos)
	toutes := users.Directory(visibles, repos, equipes)
	retenues := users.Apply(toutes, filtre, tri, decroissant)

	lignes := make([]directoryRow, 0, len(retenues))
	for _, ligne := range retenues {
		lignes = append(lignes, s.directoryRow(org, ligne))
	}

	// Les sessions et les cours proposés viennent de l'annuaire entier, pas de
	// ce qui reste affiché : un filtre ne doit pas retirer de la liste ce qui
	// permettrait d'en sortir.
	writeJSON(writer, http.StatusOK, map[string]any{
		"students": lignes,
		"sessions": users.SessionsIn(toutes),
		"courses":  users.CoursesIn(toutes, filtre.Session),
		"total":    len(toutes), "shown": len(lignes),
		// Les dépôts que personne ne réclame : sans eux, une liste incomplète
		// se lirait comme si elle était entière.
		"unmatched": users.Unmatched(visibles, repos, equipes),
		"org":       org, "source": source,
	})
}

// directoryRow rassemble les dépôts d'une personne sous le groupe d'où ils
// viennent : deux groupes peuvent avoir chacun leur « tp1 ».
func (s *Server) directoryRow(org string, ligne users.Row) directoryRow {
	parPlace := map[string][]studentAssignment{}
	for _, depot := range ligne.Repos {
		parPlace[depot.Scope] = append(parPlace[depot.Scope], studentAssignment{
			Name: depot.Assignment, ID: depot.ID, Repo: depot.Name,
			URL:      s.urlOf(org, groups.Repo{Name: depot.Name, URL: depot.URL}),
			PushedAt: depot.PushedAt,
		})
	}

	inscriptions := make([]directoryEnrollment, 0, len(ligne.Enrollments))
	for _, inscription := range ligne.Enrollments {
		travaux := parPlace[inscription.Scope]
		if travaux == nil {
			travaux = []studentAssignment{}
		}
		inscriptions = append(inscriptions, directoryEnrollment{
			Scope: inscription.Scope, Session: inscription.Session,
			SessionName: inscription.SessionName, Course: inscription.Course,
			Group: inscription.Group, Label: inscription.Label,
			Assignments: travaux, PushedAt: inscription.PushedAt,
		})
	}
	return directoryRow{
		FullName: ligne.FullName, Username: ligne.Username,
		Enrollments: inscriptions, Repos: len(ligne.Repos), PushedAt: ligne.PushedAt,
	}
}

// teamsOfAll rassemble les équipes de plusieurs groupes : l'annuaire regarde
// l'organisation entière, pas un groupe à la fois.
func teamsOfAll(courses []classroom.Classroom, infos []teams.Info) []teams.Team {
	toutes := make([]teams.Team, 0, len(infos))
	for _, cours := range courses {
		toutes = append(toutes, cours.Teams(infos)...)
	}
	return toutes
}

// ------------------------------------------------------------------ la fiche

// La fiche d'un utilisateur est la seule vue centrée sur une personne. Tout ce
// qu'elle montre est calculé dans « users » — l'interface ne fait que nommer le
// compte et rendre ce qui revient.

// handleUser dresse la fiche d'un compte : son rôle, ses comptes, et la
// chronologie de son passage dans l'organisation.
func (s *Server) handleUser(writer http.ResponseWriter, request *http.Request) {
	compte, err := valid.Login(request.PathValue("account"), "Compte GitHub")
	if err != nil {
		fail(writer, err)
		return
	}
	org := s.org()
	repos, source, err := s.repos(org, request.URL.Query().Get("refresh") == "1")
	if err != nil {
		fail(writer, err)
		return
	}

	visibles := s.visibles(org, repos)
	infos, _ := s.orgTeams(org, false)
	set, avis := s.names(org)
	fiche := users.ProfileOf(visibles, repos, teamsOfAll(visibles, infos), infos, set, compte)

	// Les adresses des dépôts se composent ici : le domaine ne connaît pas
	// l'hôte, qui n'est pas toujours github.com.
	for etape := range fiche.Timeline {
		for travail := range fiche.Timeline[etape].Assignments {
			depot := &fiche.Timeline[etape].Assignments[travail]
			depot.URL = s.urlOf(org, groups.Repo{Name: depot.Repo, URL: depot.URL})
		}
	}

	writeJSON(writer, http.StatusOK, map[string]any{
		"user": fiche, "org": org, "source": source, "notice": avis,
		// Qui regarde décide de ce que la fiche propose : seul un enseignant
		// coopte, et la page n'a pas à deviner la règle.
		//
		// Le décompte des enseignants va avec : tant que l'organisation n'en a
		// aucun, quelqu'un doit pouvoir commencer, et une page qui cacherait le
		// bouton ne laisserait aucun chemin pour le faire.
		"viewer": s.deps.Viewer, "viewer_teaches": set.Teaches(s.deps.Viewer),
		"teachers": len(set.Teachers()), "host": s.hostName(),
	})
}

// handleUserRole coopte un utilisateur comme enseignant, ou l'en défait.
//
// Seul un enseignant coopte. Ce n'est pas cette vérification qui protège le
// registre — un étudiant n'a jamais eu le droit d'écrire dans « .cohorte », et
// GitHub le lui refuserait bien avant nous. Elle est là pour que le refus soit
// dit dans les mots de l'outil plutôt qu'en HTTP 404, et pour qu'un enseignant
// qui se retire ne le fasse pas par inadvertance.
func (s *Server) handleUserRole(writer http.ResponseWriter, request *http.Request) {
	compte, err := valid.Login(request.PathValue("account"), "Compte GitHub")
	if err != nil {
		fail(writer, err)
		return
	}
	var body struct {
		IsTeacher bool `json:"is_teacher"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}

	org := s.org()
	set, _ := s.names(org)
	if !set.Teaches(s.deps.Viewer) && set.Len() > 0 && anyTeacher(set) {
		fail(writer, valid.Errorf(
			"Seul un enseignant peut en reconnaître un autre. @%s n'est pas déclaré "+
				"enseignant dans « %s ».", s.deps.Viewer, org))
		return
	}
	// Se retirer soi-même le dernier rôle d'enseignant laisserait
	// l'organisation sans personne pour en coopter : plus aucune fiche ne
	// pourrait rendre le sien à quiconque.
	if !body.IsTeacher && lastTeacher(set, compte) {
		fail(writer, valid.Errorf(
			"@%s est le seul enseignant de « %s » : lui retirer son rôle ne laisserait "+
				"personne pour en reconnaître un autre. Reconnaissez d'abord un collègue.",
			compte, org))
		return
	}

	change := registry.SetRole(compte, body.IsTeacher)
	// Un enseignant ne figure sur aucune liste de classe : le registre ne le
	// connaît pas, et on ne peut pas donner un rôle à qui n'y est pas. Le
	// coopter, c'est donc d'abord l'y faire entrer.
	//
	// Son compte est vérifié sur GitHub au passage : une faute de frappe
	// créerait sinon une fiche que rien ne désigne, et qui traînerait dans le
	// registre sans qu'on sache jamais de qui il s'agissait.
	if _, connu := set.Find(compte); !connu {
		appris, err := s.newcomer(compte)
		if err != nil {
			fail(writer, err)
			return
		}
		change.Learn = append(change.Learn, appris)
	}

	publie, err := s.registryOf(org).Apply(change)
	if err != nil {
		fail(writer, err)
		return
	}
	fiche, _ := publie.Find(compte)
	writeJSON(writer, http.StatusOK, map[string]any{
		"username": compte, "is_teacher": fiche.IsTeacher, "role": fiche.Role(),
	})
}

// newcomer compose la fiche de quelqu'un que le registre ne connaît pas encore,
// après avoir vérifié que son compte existe.
//
// Le nom vient de son profil public quand il en porte un. À défaut, la fiche
// n'en a pas : le compte suffit à la désigner, et un enseignant ne nomme aucun
// dépôt — c'est le nom d'un étudiant qui en nomme un.
func (s *Server) newcomer(username string) (registry.User, error) {
	profil, err := s.deps.Client.GetUser(username)
	if err != nil {
		return registry.User{}, err
	}
	if profil == nil {
		return registry.User{}, valid.Errorf(
			"Le compte @%s n'existe pas sur %s.", username, s.hostName())
	}
	fiche := registry.User{Username: username}
	if nom := strings.TrimSpace(profil.Name); nom != "" {
		fiche.FullName = nom
	}
	return fiche, nil
}

// handleUserName donne son nom complet à quelqu'un qui n'en a pas.
//
// Le nom vit au registre, pas dans un groupe : c'est une propriété de la
// personne, et la lui donner depuis sa fiche doit valoir partout — y compris
// pour quelqu'un qu'aucun groupe déclaré ici ne connaît.
//
// Cela ne renomme aucun dépôt. Le slug que le nouveau nom produit s'ajoute à
// ceux que la personne portait déjà, et les dépôts créés sous l'ancien restent
// les siens : c'est l'invariant du registre, et c'est ce qui rend ce geste sans
// danger. Renommer les dépôts est une autre opération, offerte là où on les
// voit — dans la liste du groupe.
func (s *Server) handleUserName(writer http.ResponseWriter, request *http.Request) {
	compte, err := valid.Login(request.PathValue("account"), "Compte GitHub")
	if err != nil {
		fail(writer, err)
		return
	}
	var body struct {
		FullName string `json:"full_name"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	nom, err := valid.FullName(body.FullName)
	if err != nil {
		fail(writer, err)
		return
	}

	org := s.org()
	set, _ := s.names(org)
	// Un compte que rien ne connaît est vérifié sur GitHub avant d'entrer au
	// registre : une faute de frappe y laisserait sinon une fiche que rien ne
	// désigne.
	//
	// « Rien », c'est ni le registre ni aucune liste de groupe. Quelqu'un
	// qu'une liste déclare est déjà quelqu'un : le renvoyer vers GitHub
	// n'apprendrait rien, et rendrait le nommage impossible dès que GitHub ne
	// répond pas — alors que c'est justement une correction qu'on fait hors
	// ligne, liste en main.
	if _, connu := set.Find(compte); !connu && !s.declared(org, compte) {
		if _, err := s.newcomer(compte); err != nil {
			fail(writer, err)
			return
		}
	}

	publie, err := s.registryOf(org).Apply(registry.Change{
		Learn:  []registry.User{registry.From(roster.Person{FullName: nom, Username: compte})},
		Reason: "Nomme @" + compte + " : " + nom,
	})
	if err != nil {
		fail(writer, err)
		return
	}
	// Rien à invalider : le magasin du registre rescelle ce qu'il vient
	// d'écrire, et la prochaine lecture en part.
	fiche, _ := publie.Find(compte)
	writeJSON(writer, http.StatusOK, map[string]any{
		"username": compte, "full_name": fiche.FullName,
	})
}

// declared dit qu'une liste de groupe de ce poste connaît déjà ce compte.
func (s *Server) declared(org, username string) bool {
	for _, personne := range s.classrooms.People(org) {
		if personne.Owns(username) {
			return true
		}
	}
	return false
}

// anyTeacher dit que l'organisation a déjà au moins un enseignant. Tant qu'elle
// n'en a aucun, le premier venu peut se déclarer : quelqu'un doit pouvoir
// commencer, et celui qui a le droit d'écrire dans « .cohorte » est justement
// celui à qui ce droit a été donné.
func anyTeacher(set *registry.Set) bool { return len(set.Teachers()) > 0 }

// lastTeacher dit que retirer ce compte laisserait l'organisation sans
// enseignant.
func lastTeacher(set *registry.Set, username string) bool {
	enseignants := set.Teachers()
	return len(enseignants) == 1 && strings.EqualFold(enseignants[0].Username, username)
}

// hostName rend l'hôte GitHub de la session, pour que la page compose les
// adresses de profils. Il n'est pas toujours « github.com ».
func (s *Server) hostName() string {
	if host := strings.TrimSpace(s.deps.Host); host != "" {
		return host
	}
	return "github.com"
}
