package teams

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Composer une équipe, c'est une suite d'écritures sur GitHub. Elles sont
// toutes préparées avant que la première ne parte : une composition
// impossible — quelqu'un qu'on inscrirait deux fois, une équipe qui n'existe
// pas — est refusée en entier plutôt qu'appliquée à moitié.
//
// Un étudiant n'appartient qu'à une équipe par groupe. Ce n'est pas GitHub qui
// l'impose — rien ne l'empêcherait d'être dans deux — mais l'outil : « le dépôt
// de l'équipe d'Émilie » ne voudrait rien dire autrement. L'inscrire quelque
// part le retire donc de là où il était.

// Nature d'une écriture à faire sur GitHub.
const (
	Join  = "inscription"
	Leave = "retrait"
)

// Step est une écriture à faire sur GitHub pour composer les équipes.
type Step struct {
	Kind     string `json:"kind"`
	Slug     string `json:"slug"`
	Team     string `json:"team"`
	Username string `json:"username"`
}

// PlanAssign compose l'inscription de personnes dans une équipe du groupe.
// Celles qui étaient dans une autre équipe la quittent : le retrait précède
// toujours l'inscription, pour qu'aucune ne se retrouve dans deux équipes si
// l'opération s'interrompt.
func PlanAssign(list []Team, target string, usernames []string) ([]Step, error) {
	arrivee, trouvee := Find(list, target)
	if !trouvee {
		return nil, valid.Errorf("Aucune équipe « %s » dans ce groupe.", strings.TrimSpace(target))
	}
	demandes, err := logins(usernames)
	if err != nil {
		return nil, err
	}
	if len(demandes) == 0 {
		return nil, valid.Errorf("Aucune personne à inscrire.")
	}

	var etapes []Step
	for _, username := range demandes {
		if arrivee.Has(username) {
			return nil, valid.Errorf("@%s est déjà dans « %s ».", username, arrivee.Label())
		}
		if depart, membre := Of(list, username); membre {
			etapes = append(etapes, Step{
				Kind: Leave, Slug: depart.Slug, Team: depart.Short, Username: username,
			})
		}
		etapes = append(etapes, Step{
			Kind: Join, Slug: arrivee.Slug, Team: arrivee.Short, Username: username,
		})
	}
	return etapes, nil
}

// PlanRemove compose le retrait de personnes d'une équipe. Elles restent dans
// le groupe : ne plus être dans une équipe n'est pas en être exclu.
func PlanRemove(list []Team, target string, usernames []string) ([]Step, error) {
	equipe, trouvee := Find(list, target)
	if !trouvee {
		return nil, valid.Errorf("Aucune équipe « %s » dans ce groupe.", strings.TrimSpace(target))
	}
	demandes, err := logins(usernames)
	if err != nil {
		return nil, err
	}
	if len(demandes) == 0 {
		return nil, valid.Errorf("Aucune personne à retirer.")
	}

	etapes := make([]Step, 0, len(demandes))
	for _, username := range demandes {
		if !equipe.Has(username) {
			return nil, valid.Errorf("@%s n'est pas dans « %s ».", username, equipe.Label())
		}
		etapes = append(etapes, Step{
			Kind: Leave, Slug: equipe.Slug, Team: equipe.Short, Username: username,
		})
	}
	return etapes, nil
}

// PlanCompose amène une équipe à la composition exacte donnée : ceux qui n'y
// sont pas encore y entrent, ceux qui n'y sont plus attendus en sortent. C'est
// ce qui permet de décrire une équipe d'un trait, sans avoir à dire ce qui a
// changé depuis la dernière fois.
func PlanCompose(list []Team, target string, usernames []string) ([]Step, error) {
	equipe, trouvee := Find(list, target)
	if !trouvee {
		return nil, valid.Errorf("Aucune équipe « %s » dans ce groupe.", strings.TrimSpace(target))
	}
	voulus, err := logins(usernames)
	if err != nil {
		return nil, err
	}

	attendus := map[string]bool{}
	for _, username := range voulus {
		attendus[strings.ToLower(username)] = true
	}

	var etapes []Step
	partants := append([]string(nil), equipe.Members...)
	sort.Slice(partants, func(i, j int) bool {
		return strings.ToLower(partants[i]) < strings.ToLower(partants[j])
	})
	for _, membre := range partants {
		if attendus[strings.ToLower(membre)] {
			continue
		}
		etapes = append(etapes, Step{
			Kind: Leave, Slug: equipe.Slug, Team: equipe.Short, Username: membre,
		})
	}
	for _, username := range voulus {
		if equipe.Has(username) {
			continue
		}
		if depart, membre := Of(list, username); membre {
			etapes = append(etapes, Step{
				Kind: Leave, Slug: depart.Slug, Team: depart.Short, Username: username,
			})
		}
		etapes = append(etapes, Step{
			Kind: Join, Slug: equipe.Slug, Team: equipe.Short, Username: username,
		})
	}
	return etapes, nil
}

// logins met des comptes GitHub en forme, sans doublon et dans l'ordre donné.
func logins(usernames []string) ([]string, error) {
	vus := map[string]bool{}
	propres := make([]string, 0, len(usernames))
	for _, brut := range usernames {
		if strings.TrimSpace(brut) == "" {
			continue
		}
		username, err := valid.Login(brut, "Compte GitHub")
		if err != nil {
			return nil, err
		}
		if vus[strings.ToLower(username)] {
			continue
		}
		vus[strings.ToLower(username)] = true
		propres = append(propres, username)
	}
	return propres, nil
}

// ------------------------------------------------------------ nommer, adopter

// Available refuse un nom court déjà pris dans le groupe. GitHub le refuserait
// aussi — un « slug » ne sert qu'une fois par organisation —, mais son message
// parlerait d'un nom que personne n'a écrit.
func Available(list []Team, short string) error {
	if _, pris := Find(list, short); pris {
		return valid.Errorf("Une équipe « %s » existe déjà dans ce groupe.", short)
	}
	return nil
}

// PlanRename dit le nom complet que prendra une équipe renommée, et refuse un
// nom déjà occupé. Le nom court est ce qui change ; la place, elle, ne bouge
// pas — une équipe ne change pas de groupe.
func PlanRename(list []Team, from, to string) (Team, string, error) {
	equipe, trouvee := Find(list, from)
	if !trouvee {
		return Team{}, "", valid.Errorf("Aucune équipe « %s » dans ce groupe.",
			strings.TrimSpace(from))
	}
	short, err := ShortName(to)
	if err != nil {
		return Team{}, "", err
	}
	if strings.EqualFold(short, equipe.Short) {
		return Team{}, "", valid.Errorf("« %s » porte déjà ce nom.", equipe.Label())
	}
	if err := Available(list, short); err != nil {
		return Team{}, "", err
	}
	return equipe, naming.TeamName(equipe.Session, equipe.Course, equipe.Group, short), nil
}

// PlanAdopt dit le nom qu'une équipe déjà présente dans l'organisation prendra
// en rejoignant un groupe. Adopter, c'est renommer : l'équipe garde ses
// membres, son historique et ses accès, et devient lisible pour l'outil.
//
// Une équipe qui relève déjà d'un groupe n'est pas à adopter : la faire changer
// de groupe d'un renommage laisserait ses dépôts derrière elle.
func PlanAdopt(list []Team, infos []Info, slug, short, session, course, group string) (
	Info, string, error) {
	source, trouvee := bySlug(infos, slug)
	if !trouvee {
		return Info{}, "", valid.Errorf("Aucune équipe « %s » dans l'organisation.",
			strings.TrimSpace(slug))
	}
	if _, deja := Read(source); deja {
		return Info{}, "", valid.Errorf(
			"« %s » relève déjà d'un groupe : elle ne peut pas en changer.", source.Name)
	}
	nom, err := ShortName(short)
	if err != nil {
		return Info{}, "", err
	}
	if err := Available(list, nom); err != nil {
		return Info{}, "", err
	}
	return source, naming.TeamName(session, course, group, nom), nil
}

// bySlug retrouve une équipe de l'organisation par son adresse GitHub.
func bySlug(infos []Info, slug string) (Info, bool) {
	wanted := strings.ToLower(strings.TrimSpace(slug))
	for _, info := range infos {
		if strings.ToLower(info.Slug) == wanted {
			return info, true
		}
	}
	return Info{}, false
}
