package users

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
)

// La fiche d'un utilisateur répond à une question que ni la liste d'un groupe
// ni l'annuaire ne posent : qu'est-ce que cette personne a fait ici, depuis le
// début ?
//
// L'annuaire donne une ligne par personne et ses cours en jetons ; il répond à
// « qui a suivi quoi » pour tout le monde à la fois. La fiche prend une seule
// personne et déroule son passage dans l'organisation, du plus récent au plus
// ancien : les cours suivis, ceux donnés, les dépôts laissés à chaque fois.
//
// Elle mêle les deux rôles à dessein. Chercher les cours qu'un collègue a déjà
// donnés et retrouver ceux qu'un étudiant a suivis sont la même question posée
// à deux personnes différentes, et une seule page doit y répondre.

// Profile est tout ce que l'organisation sait d'une personne.
type Profile struct {
	FullName string `json:"full_name"`
	// Username est le compte qui la désigne ; Accounts les porte tous. Une
	// personne travaille parfois sous deux comptes — celui du collège et le
	// sien —, et ils mènent tous à son profil GitHub.
	Username  string   `json:"username"`
	Accounts  []string `json:"accounts"`
	StudentID string   `json:"student_id,omitempty"`
	// IsTeacher est ce que le registre déclare, Role le mot qui le dit.
	IsTeacher bool   `json:"is_teacher"`
	Role      string `json:"role"`
	// Known dit que le registre connaît ce compte. Un compte qu'il ignore a pu
	// laisser des dépôts sans que personne ne l'ait jamais nommé.
	Known bool `json:"known"`
	// Timeline déroule son passage, de la session la plus récente à la plus
	// ancienne. L'ordre est décidé ici pour que le terminal et le navigateur
	// montrent le même.
	Timeline []Step `json:"timeline"`
	// Repos compte ses dépôts, Courses ses cours, Taught ceux qu'elle a
	// donnés. PushedAt est son plus récent envoi, vide s'il n'y en a eu aucun.
	Repos    int    `json:"repos"`
	Courses  int    `json:"courses"`
	Taught   int    `json:"taught"`
	PushedAt string `json:"pushed_at,omitempty"`
}

// Les rôles tenus dans un cours. Ce ne sont pas ceux du registre : on peut
// être enseignant et avoir suivi un cours comme étudiant, et la chronologie
// doit pouvoir le dire.
const (
	AsStudent = "étudiant"
	AsTeacher = "enseignant"
)

// Step est un cours, vu de la personne.
type Step struct {
	Scope       string `json:"scope"`
	Session     string `json:"session,omitempty"`
	SessionName string `json:"session_name,omitempty"`
	Course      string `json:"course,omitempty"`
	Group       string `json:"group,omitempty"`
	Label       string `json:"label"`
	// Role dit ce qu'elle y a été.
	Role string `json:"role"`
	// Assignments sont les dépôts qu'elle y a laissés, du plus récemment
	// touché au plus ancien. Un enseignant n'en a pas : il n'en rend pas.
	Assignments []Work `json:"assignments"`
	// PushedAt est son plus récent envoi dans ce cours.
	PushedAt string `json:"pushed_at,omitempty"`
	// Silent dit qu'elle y a des dépôts sans avoir jamais rien envoyé. Ce
	// n'est pas « aucun dépôt » : c'est un dépôt resté vide, et cela se
	// remarque autrement.
	Silent bool `json:"silent"`
}

// Teaching dit que la personne a donné ce cours plutôt que de l'avoir suivi.
func (s Step) Teaching() bool { return s.Role == AsTeacher }

// Work est un dépôt rendu.
type Work struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Repo     string `json:"repo"`
	URL      string `json:"url"`
	PushedAt string `json:"pushed_at,omitempty"`
	// Team nomme l'équipe à qui le dépôt appartient ; vide pour un travail
	// individuel.
	Team string `json:"team,omitempty"`
}

// Registry dit ce que le registre de l'organisation sait d'un compte. La fiche
// en a besoin pour son nom et son rôle ; elle n'a pas à savoir d'où ils
// viennent.
type Registry interface {
	// Name rend le nom complet d'un compte, vide s'il est inconnu.
	Name(username string) string
	// Teaches dit si le compte est déclaré enseignant.
	Teaches(username string) bool
	// Knows dit si le registre connaît le compte.
	Knows(username string) bool
	// Teachers énumère ceux qui enseignent. L'annuaire en a besoin : ils ne
	// figurent sur aucune liste de classe, et rien d'autre ne les y ferait
	// entrer.
	Teachers() []registry.User
}

// ProfileOf dresse la fiche d'un compte.
//
// Les cours donnés y figurent au même titre que les cours suivis. C'est ce qui
// permet de chercher ce qu'un collègue a déjà enseigné, et c'est aussi pourquoi
// la fiche ne peut pas se contenter des lignes de l'annuaire : celles-ci ne
// connaissent que les inscriptions, et un enseignant n'est inscrit nulle part.
//
// Un compte que rien ne connaît rend quand même une fiche, vide. C'est plus
// honnête qu'une page absente : le compte existe sur GitHub, il n'a simplement
// rien fait ici.
func ProfileOf(courses []classroom.Classroom, repos []groups.RepoInfo,
	equipes []teams.Team, infos []teams.Info, known Registry, account string) Profile {
	compte := strings.TrimSpace(account)
	fiche := Profile{
		Username: compte, Accounts: []string{}, Timeline: []Step{},
		Role: AsStudent,
	}
	if compte == "" {
		return fiche
	}
	if known != nil {
		fiche.FullName = known.Name(compte)
		fiche.IsTeacher = known.Teaches(compte)
		fiche.Known = known.Knows(compte)
		if fiche.IsTeacher {
			fiche.Role = AsTeacher
		}
	}

	// Tout vient de l'annuaire : c'est lui qui réunit les comptes d'une même
	// personne, rattache ses dépôts, et sait lire dans les équipes les cours
	// qu'elle a donnés. Le refaire ici ferait deux vérités d'une seule.
	for _, ligne := range Directory(courses, repos, equipes, infos, known) {
		if !owns(ligne, compte) {
			continue
		}
		fiche.FullName = firstNonEmpty(ligne.FullName, fiche.FullName)
		fiche.Username = firstNonEmpty(ligne.Username, fiche.Username)
		fiche.Accounts = ligne.Accounts
		fiche.StudentID = ligne.StudentID
		fiche.PushedAt = ligne.PushedAt
		fiche.Repos = len(ligne.Repos)
		if ligne.IsTeacher {
			fiche.IsTeacher, fiche.Role = true, AsTeacher
		}
		for _, inscription := range ligne.Enrollments {
			fiche.Timeline = append(fiche.Timeline, stepOf(inscription, ligne.Repos))
		}
		break
	}
	if len(fiche.Accounts) == 0 {
		fiche.Accounts = []string{compte}
	}

	sortSteps(fiche.Timeline)
	for _, etape := range fiche.Timeline {
		if etape.Teaching() {
			fiche.Taught++
			continue
		}
		fiche.Courses++
	}
	return fiche
}

// stepOf compose l'étape d'un cours. Un cours donné ne porte pas de dépôts :
// un enseignant n'en rend pas, et ceux du groupe sont ceux de ses étudiants.
func stepOf(inscription Enrollment, tous []Repo) Step {
	etape := Step{
		Scope: inscription.Scope, Session: inscription.Session,
		SessionName: inscription.SessionName, Course: inscription.Course,
		Group: inscription.Group, Label: inscription.Label,
		Role: inscription.Role, PushedAt: inscription.PushedAt,
		Assignments: []Work{},
	}
	if etape.Teaching() {
		return etape
	}
	for _, depot := range tous {
		if !strings.EqualFold(depot.Scope, inscription.Scope) {
			continue
		}
		etape.Assignments = append(etape.Assignments, Work{
			Name: depot.Assignment, ID: depot.ID, Repo: depot.Name,
			URL: depot.URL, PushedAt: depot.PushedAt, Team: depot.Team,
		})
	}
	// Le plus récemment touché d'abord : c'est ce qu'on vient voir.
	sort.SliceStable(etape.Assignments, func(i, j int) bool {
		if etape.Assignments[i].PushedAt != etape.Assignments[j].PushedAt {
			return etape.Assignments[i].PushedAt > etape.Assignments[j].PushedAt
		}
		return strings.ToLower(etape.Assignments[i].Name) <
			strings.ToLower(etape.Assignments[j].Name)
	})
	// Avoir des dépôts et n'avoir rien envoyé n'est pas n'avoir aucun dépôt.
	etape.Silent = len(etape.Assignments) > 0 && etape.PushedAt == ""
	return etape
}

// sortSteps range la chronologie de la session la plus récente à la plus
// ancienne — le même ordre que l'annuaire, décidé au même endroit.
func sortSteps(etapes []Step) {
	sort.SliceStable(etapes, func(i, j int) bool {
		if rang := naming.CompareSessions(etapes[i].Session, etapes[j].Session); rang != 0 {
			return rang < 0
		}
		if etapes[i].Course != etapes[j].Course {
			return strings.ToLower(etapes[i].Course) < strings.ToLower(etapes[j].Course)
		}
		return strings.ToLower(etapes[i].Group) < strings.ToLower(etapes[j].Group)
	})
}

// owns dit si une ligne de l'annuaire est celle de ce compte. Tous ses comptes
// y mènent : une personne qui travaille sous deux comptes n'a qu'une fiche.
func owns(ligne Row, account string) bool {
	if strings.EqualFold(ligne.Username, account) {
		return true
	}
	for _, compte := range ligne.Accounts {
		if strings.EqualFold(compte, account) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
