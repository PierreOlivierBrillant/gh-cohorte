package classroom

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Cloisonner un groupe, c'est donner ses dépôts à une équipe et n'y mettre que
// ceux qui l'enseignent.
//
// Deux enseignants d'un même cours n'ont aucune raison de voir les étudiants de
// l'autre. Rien dans la nomenclature ne les sépare — « a26.5n6.01 » et
// « a26.5n6.02 » vivent dans la même organisation —, et rien dans le registre
// ne le peut : un fichier ne retient personne. Ce qui les sépare, ce sont les
// droits GitHub, et c'est donc là que le cloisonnement s'écrit.
//
// L'équipe enseignante du groupe reçoit ses dépôts ; ses membres les voient,
// les autres non. Cela suppose que les enseignants soient membres de
// l'organisation et non propriétaires : un propriétaire voit tout, et aucune
// équipe n'y changera rien. L'outil le dit plutôt que de laisser croire à un
// cloisonnement qui n'existerait pas.
//
// Le registre et l'équipe ne disent pas la même chose. Le registre dit qui
// enseigne ; l'équipe dit où. On ne peut inscrire dans l'équipe d'un groupe que
// quelqu'un que le registre déclare enseignant — c'est à cela que sert
// « is_teacher » —, mais c'est l'inscription, jamais le registre, qui ouvre
// l'accès.

// TeacherPermission est le droit donné à l'équipe enseignante sur les dépôts du
// groupe.
//
// C'est « admin » et non « push » : renommer un dépôt, le déplacer, en changer
// la visibilité ou le supprimer sont des choses que l'outil fait couramment, et
// « push » ne les permet pas. Un enseignant cloisonné qui ne pourrait rien
// faire de son propre groupe trouverait la moitié de l'outil cassée.
const TeacherPermission = "admin"

// TeacherTeamName compose le nom de l'équipe enseignante du groupe.
func (c Classroom) TeacherTeamName() string {
	return naming.TeacherTeamName(c.Session, c.Course, c.Group)
}

// TeacherTeam retrouve l'équipe enseignante du groupe parmi celles de
// l'organisation. Son absence n'est pas une panne : un groupe non cloisonné
// n'en a pas, et tout le reste fonctionne sans elle.
func (c Classroom) TeacherTeam(infos []teams.Info) (teams.Team, bool) {
	return teams.TeacherTeam(c.Session, c.Course, c.Group, infos)
}

// Teaching est l'état du cloisonnement d'un groupe, tel que les trois
// interfaces le montrent.
type Teaching struct {
	// Name est le nom complet de l'équipe, Slug son adresse sur GitHub. Le
	// slug est vide tant que l'équipe n'existe pas.
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
	// Exists dit que l'équipe est là. Sans elle, le groupe n'est pas cloisonné
	// et ses dépôts se lisent selon ce que l'organisation accorde d'office.
	Exists bool `json:"exists"`
	// Teachers nomme ceux qui l'enseignent, rangés par nom.
	Teachers []roster.Person `json:"teachers"`
	// Waiting nomme ceux qui ont été invités sans avoir encore accepté. Ils en
	// sont : ils ne l'ont pas encore su.
	Waiting []string `json:"waiting,omitempty"`
	// Repos compte les dépôts du groupe que l'équipe recevrait.
	Repos int `json:"repos"`
}

// Cloistered dit que le groupe est réellement cloisonné : l'équipe existe et
// quelqu'un y est. Une équipe vide n'enferme rien — elle retire simplement à
// tout le monde ce que personne n'avait.
func (t Teaching) Cloistered() bool { return t.Exists && len(t.Teachers) > 0 }

// TeachingOf décrit le cloisonnement d'un groupe. Les noms viennent du
// registre, qui est le seul à savoir qui se cache derrière un compte.
func (c Classroom) TeachingOf(infos []teams.Info, repos []groups.RepoInfo,
	names Names) Teaching {
	etat := Teaching{Name: c.TeacherTeamName(), Repos: len(c.Owned(repos))}
	equipe, existe := c.TeacherTeam(infos)
	if !existe {
		etat.Teachers = []roster.Person{}
		return etat
	}
	etat.Exists, etat.Slug, etat.Waiting = true, equipe.Slug, equipe.Pending

	etat.Teachers = make([]roster.Person, 0, len(equipe.Members))
	for _, compte := range equipe.Members {
		personne := roster.Person{Username: compte}
		if names != nil {
			if connue, trouve := names.Lookup(compte); trouve {
				personne.FullName = connue.FullName
			}
		}
		etat.Teachers = append(etat.Teachers, personne)
	}
	SortPeople(etat.Teachers)
	return etat
}

// TeachingStep est une écriture à faire sur GitHub pour cloisonner le groupe.
const (
	// CreateTeam crée l'équipe enseignante.
	CreateTeam = "création"
	// GrantRepo donne un dépôt du groupe à l'équipe.
	GrantRepo = "accès"
	// JoinTeam y inscrit quelqu'un, LeaveTeam l'en retire.
	JoinTeam  = "inscription"
	LeaveTeam = "retrait"
)

// TeachingStep est une écriture du plan de cloisonnement.
type TeachingStep struct {
	Kind string `json:"kind"`
	// Target est le compte pour une inscription ou un retrait, le dépôt pour
	// un accès, et le nom de l'équipe pour sa création.
	Target string `json:"target"`
	// Name nomme la personne quand on en connaît une, pour que le plan se
	// lise sans avoir à traduire les comptes.
	Name string `json:"name,omitempty"`
}

// TeachingPlan est ce que cloisonner ferait, avant de le faire.
//
// Comme partout ailleurs dans l'outil, il se montre avant de s'appliquer : ces
// écritures touchent à qui voit quoi, et personne ne devrait avoir à les
// découvrir après coup.
type TeachingPlan struct {
	Team        string         `json:"team"`
	Slug        string         `json:"slug,omitempty"`
	Description string         `json:"description"`
	Permission  string         `json:"permission"`
	Steps       []TeachingStep `json:"steps"`
	// Notice dit ce que le plan ne peut pas garantir — un propriétaire de
	// l'organisation voit les dépôts quoi qu'il arrive.
	Notice string `json:"notice,omitempty"`
}

// Empty dit qu'il n'y a rien à écrire.
func (p TeachingPlan) Empty() bool { return len(p.Steps) == 0 }

// PlanTeaching compose ce qu'il faut écrire pour que l'équipe enseignante du
// groupe soit exactement celle qu'on demande, et qu'elle ait ses dépôts.
//
// Les comptes demandés doivent être déclarés enseignants au registre. C'est le
// seul endroit où « is_teacher » a un effet, et il est indirect : il ne donne
// rien, il autorise à donner. Coopter quelqu'un puis l'inscrire ici sont deux
// gestes, et le second seul ouvre un accès.
func (c Classroom) PlanTeaching(infos []teams.Info, repos []groups.RepoInfo,
	wanted []string, teaching Teachers) (TeachingPlan, error) {
	comptes, err := cleanLogins(wanted)
	if err != nil {
		return TeachingPlan{}, err
	}
	if teaching != nil {
		for _, compte := range comptes {
			if !teaching.Teaches(compte) {
				return TeachingPlan{}, valid.Errorf(
					"@%s n'est pas déclaré enseignant : reconnaissez-le d'abord comme tel "+
						"depuis sa fiche, puis inscrivez-le à l'équipe du groupe.", compte)
			}
		}
	}

	plan := TeachingPlan{
		Team:        c.TeacherTeamName(),
		Description: teams.DescribeTeachers(c.Session, c.Course, c.Group),
		Permission:  TeacherPermission,
		Steps:       []TeachingStep{},
		Notice:      OwnersSeeAll,
	}
	equipe, existe := c.TeacherTeam(infos)
	if !existe {
		plan.Steps = append(plan.Steps, TeachingStep{Kind: CreateTeam, Target: plan.Team})
	} else {
		plan.Slug = equipe.Slug
	}

	// Le retrait précède l'inscription : si l'opération s'interrompt, mieux
	// vaut un accès de moins qu'un accès de trop.
	for _, membre := range equipe.Members {
		if !containsFold(comptes, membre) {
			plan.Steps = append(plan.Steps, TeachingStep{Kind: LeaveTeam, Target: membre})
		}
	}
	for _, compte := range comptes {
		if existe && equipe.Has(compte) {
			continue
		}
		plan.Steps = append(plan.Steps, TeachingStep{Kind: JoinTeam, Target: compte})
	}

	// L'accès se redonne sans condition : GitHub l'accepte deux fois sans
	// broncher, et le vérifier coûterait une requête par dépôt pour n'éviter
	// qu'une écriture sans effet.
	for _, depot := range c.Owned(repos) {
		plan.Steps = append(plan.Steps, TeachingStep{Kind: GrantRepo, Target: depot.Name})
	}
	return plan, nil
}

// OwnersSeeAll dit ce que le cloisonnement ne peut pas faire.
//
// Une équipe ouvre un accès ; elle n'en ferme aucun. Un propriétaire de
// l'organisation voit tous ses dépôts par construction, et une permission de
// base autre que « none » en ouvre d'office à tous ses membres. Taire cela
// laisserait croire à un cloisonnement qui n'existe pas — c'est précisément le
// genre de silence qui fait qu'on découvre trop tard qu'un collègue lisait les
// copies de nos étudiants.
const OwnersSeeAll = "L'équipe ouvre un accès, elle n'en ferme aucun : " +
	"un propriétaire de l'organisation voit tous ses dépôts quoi qu'il arrive. " +
	"Pour que le cloisonnement tienne, les enseignants doivent en être membres " +
	"et non propriétaires, et la permission de base doit être « none »."

// Teachers dit qui, dans l'organisation, est déclaré enseignant. Le registre le
// sait ; « classroom » n'a pas à savoir comment.
type Teachers interface {
	Teaches(username string) bool
}

// Staffing verse dans le groupe ce que le registre dit de qui enseigne. Comme
// « Scheduling », il ne retient rien : il branche le groupe sur le registre, et
// c'est le registre qui reste la source.
//
// Sans cela, un dépôt ne saurait pas distinguer ce qu'un étudiant y a remis de
// ce que son enseignant y a poussé — un gabarit, une correction —, et daterait
// la remise du jour où l'enseignant y a touché.
func (c Classroom) Staffing(enseignants Teachers) Classroom {
	c.enseignants = enseignants
	return c
}

// cleanLogins met des comptes en forme, les dédoublonne et les range. Deux fois
// le même compte n'est pas deux enseignants.
func cleanLogins(usernames []string) ([]string, error) {
	vus := map[string]bool{}
	propres := make([]string, 0, len(usernames))
	for _, brut := range usernames {
		if strings.TrimSpace(brut) == "" {
			continue
		}
		compte, err := valid.Login(brut, "Compte GitHub")
		if err != nil {
			return nil, err
		}
		if vus[strings.ToLower(compte)] {
			continue
		}
		vus[strings.ToLower(compte)] = true
		propres = append(propres, compte)
	}
	sort.Slice(propres, func(i, j int) bool {
		return strings.ToLower(propres[i]) < strings.ToLower(propres[j])
	})
	return propres, nil
}

// containsFold dit si un compte figure dans une liste, casse ignorée.
func containsFold(liste []string, valeur string) bool {
	for _, item := range liste {
		if strings.EqualFold(item, valeur) {
			return true
		}
	}
	return false
}
