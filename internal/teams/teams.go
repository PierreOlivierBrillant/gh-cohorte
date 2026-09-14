// Package teams tient la notion d'équipe : un sous-ensemble des étudiants d'un
// groupe, à qui l'on distribue un travail commun.
//
// Une équipe est une vraie équipe d'organisation GitHub — c'est ce que faisait
// GitHub Classroom, et c'est ce qui rend le reste simple : l'accès au dépôt est
// accordé à l'équipe, pas à chacun de ses membres. Déplacer quelqu'un d'une
// équipe à l'autre lui retire donc l'accès aux dépôts de l'ancienne et lui
// donne celui des nouveaux, sans qu'aucun dépôt n'ait à être touché.
//
// Comme pour les dépôts, c'est le nom qui porte tout :
//
//	session . cours . groupe . équipe
//	a26.5n6.01.eq1
//
// La place du groupe y figure parce qu'une équipe appartient à l'organisation
// entière : sans elle, deux groupes ne pourraient pas avoir chacun leur
// « eq1 ». La description de l'équipe, elle, ne dit rien que le nom ne dise
// déjà — elle est là pour qui lit la page de l'équipe sur GitHub, pas pour
// l'outil.
//
// Les étudiants y sont inscrits comme simples membres, jamais comme
// responsables : un membre ne peut ni renommer son équipe, ni en changer la
// description, ni en faire sortir quelqu'un.
//
// Un groupe porte en plus une équipe d'un autre genre : « a26.5n6.01.enseignants ».
// Elle ne reçoit pas de travail — elle reçoit l'accès à tous les dépôts du
// groupe, et c'est elle qui cloisonne. Deux enseignants d'un même cours ont
// chacun leur groupe et chacun leur équipe ; celui qui n'est pas dans l'équipe
// de l'autre ne voit pas ses étudiants. Le registre dit qui enseigne, l'équipe
// dit où : c'est l'équipe qui décide de l'accès, jamais le registre.
package teams

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Privacy est la visibilité donnée aux équipes créées par l'outil. « closed »
// les rend visibles aux membres de l'organisation : un étudiant voit l'équipe
// dont il fait partie, et ses coéquipiers.
const Privacy = "closed"

// MemberRole est le rôle donné aux étudiants. Un « member » ne peut ni
// renommer l'équipe, ni la décrire, ni en modifier la composition : c'est ce
// qui fait que le nom de l'équipe reste ce que l'enseignant en a fait.
const MemberRole = "member"

// Info retient d'une équipe GitHub ce que l'outil en lit. Les membres viennent
// d'un autre point de l'API : ils sont renseignés à part.
type Info struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Members     []string `json:"members"`
	// Pending nomme ceux qui ont été invités sans avoir encore accepté.
	// Inscrire dans une équipe quelqu'un qui n'est pas membre de
	// l'organisation l'y invite plutôt que de l'y mettre : il en fait partie
	// pour l'outil, et le taire ferait croire que rien ne s'est passé.
	Pending []string `json:"pending,omitempty"`
}

// Team est une équipe d'un groupe : une place dans la nomenclature, un nom
// court, et des comptes GitHub.
type Team struct {
	// Slug est l'adresse de l'équipe sur GitHub ; c'est par lui que l'API la
	// désigne. Il ne se lit pas : GitHub y remplace le point par un tiret.
	Slug string `json:"slug"`
	// Name est le nom complet, « a26.5n6.01.eq1 ».
	Name string `json:"name"`
	// Short est le nom court, « eq1 » : celui qu'on lit et qu'on écrit.
	Short       string   `json:"short"`
	Session     string   `json:"session"`
	Course      string   `json:"course"`
	Group       string   `json:"group"`
	Description string   `json:"description"`
	Members     []string `json:"members"`
	// Pending nomme ceux qui n'ont pas encore accepté leur invitation.
	Pending []string `json:"pending,omitempty"`
}

// Waiting dit qu'un compte a été invité sans avoir encore accepté.
func (t Team) Waiting(username string) bool {
	for _, compte := range t.Pending {
		if strings.EqualFold(compte, username) {
			return true
		}
	}
	return false
}

// Scope rend la place du groupe auquel l'équipe appartient.
func (t Team) Scope() string { return naming.Prefix(t.Session, t.Course, t.Group) }

// Label nomme l'équipe pour l'affichage.
func (t Team) Label() string { return "Équipe " + t.Short }

// Has dit si un compte GitHub fait partie de l'équipe.
func (t Team) Has(username string) bool {
	for _, member := range t.Members {
		if strings.EqualFold(member, username) {
			return true
		}
	}
	return false
}

// Read reconnaît une équipe de la nomenclature. Une équipe dont le nom ne la
// suit pas existe dans l'organisation sans relever d'aucun groupe : elle n'est
// pas une erreur, seulement quelque chose que l'outil ne gère pas encore.
func Read(info Info) (Team, bool) {
	parts, reconnu := naming.ParseTeam(info.Name)
	if !reconnu {
		return Team{}, false
	}
	return Team{
		Slug: info.Slug, Name: strings.TrimSpace(info.Name), Short: parts.Team,
		Session: parts.Session, Course: parts.Course, Group: parts.Group,
		Description: info.Description, Members: info.Members,
		Pending: info.Pending,
	}, true
}

// In retient les équipes d'étudiants d'un groupe, classées par nom court.
//
// L'équipe enseignante en est écartée. Elle porte le même genre de nom et vit
// dans la même organisation, mais elle n'est pas une équipe du groupe au sens
// où l'entend le reste de l'outil : on ne lui distribue pas de travail, elle
// ne compte pas dans « combien d'équipes », et personne n'y est placé depuis
// la liste. « TeacherTeam » est le seul chemin qui y mène.
func In(session, course, group string, infos []Info) []Team {
	retenues := make([]Team, 0, len(infos))
	for _, info := range infos {
		equipe, reconnue := Read(info)
		if !reconnue || equipe.Teaching() {
			continue
		}
		if !naming.TeamBelongs(naming.TeamParts{
			Session: equipe.Session, Course: equipe.Course, Group: equipe.Group,
		}, session, course, group) {
			continue
		}
		retenues = append(retenues, equipe)
	}
	sort.Slice(retenues, func(i, j int) bool {
		return strings.ToLower(retenues[i].Short) < strings.ToLower(retenues[j].Short)
	})
	return retenues
}

// Loose renvoie les équipes de l'organisation qui ne relèvent d'aucun groupe :
// celles qu'on peut adopter. Elles sont classées par nom.
func Loose(infos []Info) []Info {
	restantes := make([]Info, 0, len(infos))
	for _, info := range infos {
		if _, reconnue := Read(info); reconnue {
			continue
		}
		restantes = append(restantes, info)
	}
	sort.Slice(restantes, func(i, j int) bool {
		return strings.ToLower(restantes[i].Name) < strings.ToLower(restantes[j].Name)
	})
	return restantes
}

// Find retrouve une équipe par son nom court, sans tenir compte de la casse.
func Find(list []Team, short string) (Team, bool) {
	wanted := strings.ToLower(strings.TrimSpace(short))
	for _, equipe := range list {
		if strings.ToLower(equipe.Short) == wanted {
			return equipe, true
		}
	}
	return Team{}, false
}

// Of retrouve l'équipe d'un étudiant. Une personne n'appartient qu'à une équipe
// par groupe : c'est ce que « Assign » garantit.
func Of(list []Team, username string) (Team, bool) {
	for _, equipe := range list {
		if equipe.Has(username) {
			return equipe, true
		}
	}
	return Team{}, false
}

// Names rend les noms courts d'une liste d'équipes.
func Names(list []Team) []string {
	noms := make([]string, 0, len(list))
	for _, equipe := range list {
		noms = append(noms, equipe.Short)
	}
	return noms
}

// ShortName valide le nom court d'une équipe d'étudiants. Il est slugifié
// comme les autres niveaux du nom : le point y reste réservé à la séparation.
//
// « enseignants » est refusé : c'est le nom court de l'équipe enseignante du
// groupe, et une équipe d'étudiants qui le porterait lui prendrait sa place —
// l'organisation n'accepte qu'un nom d'équipe donné. Le dire au moment où l'on
// nomme vaut mieux qu'un échec de GitHub quelques écrans plus loin.
func ShortName(value string) (string, error) {
	short, err := naming.Fragment(value, "Nom de l'équipe")
	if err != nil {
		return "", err
	}
	if strings.EqualFold(short, naming.TeacherTeam) {
		return "", valid.Errorf(
			"Nom de l'équipe : « %s » est réservé à l'équipe enseignante du groupe.",
			naming.TeacherTeam)
	}
	return short, nil
}

// --------------------------------------------------- équipe enseignante

// Teaching dit que l'équipe est celle qui enseigne le groupe, et non une
// équipe d'étudiants.
func (t Team) Teaching() bool {
	return naming.IsTeacherTeam(naming.TeamParts{Team: t.Short})
}

// TeacherTeam retrouve l'équipe enseignante d'un groupe parmi celles de
// l'organisation. Son absence n'est pas une panne : un groupe créé avant que
// l'outil ne sache les cloisonner n'en a pas, et tout ce qui le concerne
// continue de fonctionner — sans cloisonnement, simplement.
func TeacherTeam(session, course, group string, infos []Info) (Team, bool) {
	for _, info := range infos {
		equipe, reconnue := Read(info)
		if !reconnue || !equipe.Teaching() {
			continue
		}
		if naming.TeamBelongs(naming.TeamParts{
			Session: equipe.Session, Course: equipe.Course, Group: equipe.Group,
		}, session, course, group) {
			return equipe, true
		}
	}
	return Team{}, false
}

// DescribeTeachers compose la description de l'équipe enseignante sur GitHub.
func DescribeTeachers(session, course, group string) string {
	return "Enseignants du groupe " + group + ", " + strings.ToUpper(course) +
		", " + naming.SessionLabel(session) + " — accès à ses dépôts (gh cohorte)"
}

// Describe compose la description déposée sur GitHub. Elle ne sert qu'à qui lit
// la page de l'équipe : l'outil, lui, ne se fie qu'au nom. Un membre ne peut
// pas la modifier, mais un propriétaire de l'organisation le peut — la faire
// porter une vérité serait donc lui demander plus qu'elle ne peut tenir.
func Describe(session, course, group, short string) string {
	return "Équipe " + short + " — groupe " + group + ", " +
		strings.ToUpper(course) + ", " + naming.SessionLabel(session) + " (gh cohorte)"
}
