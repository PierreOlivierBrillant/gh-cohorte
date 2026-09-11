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

// Reprendre un travail fait en équipe.
//
// C'est la même reprise qu'un travail individuel, en plus simple : le dernier
// niveau du nom ne cache personne. « projet-alpha » se lit sans deviner —
// « alpha » est l'équipe, et tout ce qui précède est le travail. Il n'y a donc
// rien à rapprocher d'une liste : le nom ne prétend pas désigner quelqu'un.
//
// Ce sont les accès qui disent qui est dans l'équipe. Une personne ajoutée à un
// dépôt d'équipe y a été ajoutée parce qu'elle en fait partie ; c'est la seule
// source, et elle ne se devine pas. L'équipe est ensuite créée sur GitHub sous
// la nomenclature — « a26.5n6.01.alpha » —, ses membres y sont inscrits, et le
// dépôt lui est partagé. Après quoi le travail se lit comme s'il avait toujours
// été distribué par l'outil.

// ImportedTeam est une équipe telle que la reprise la reconstituera.
type ImportedTeam struct {
	// Short est le nom court, tiré de ce qui suit le préfixe ; Name le nom
	// complet qu'elle portera sur GitHub.
	Short string `json:"short"`
	Name  string `json:"name"`
	// Repo est le dépôt tel qu'il s'appelle, Target tel qu'il s'appellera.
	Repo   string `json:"repo"`
	Target string `json:"target"`
	// Members sont les comptes que les accès du dépôt désignent.
	Members []string `json:"members"`
	// Exists dit que l'équipe est déjà là : la reprise la complétera plutôt
	// que de la créer.
	Exists bool `json:"exists"`
}

// TeamImport est ce qu'une reprise en équipe ferait, avant qu'elle ne le fasse.
type TeamImport struct {
	// TeamWork ne varie pas : il dit à l'interface de quelle reprise il s'agit.
	TeamWork bool   `json:"team_work"`
	Prefix   string `json:"prefix"`
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	// Teams décrit chaque équipe reconstituée, une par dépôt.
	Teams []ImportedTeam `json:"teams"`
	// Moves est le renommage lui-même.
	Moves []Move `json:"moves"`
	// Students sont les personnes que la reprise inscrira au groupe : les
	// membres de toutes les équipes, réunis.
	Students []roster.Person `json:"students"`
	// Silent nomme les dépôts dont les accès ne désignent personne. L'équipe
	// sera créée vide : le dépôt lui appartiendra, mais il faudra dire qui en
	// est.
	Silent []string `json:"silent"`
}

// Ready dit qu'il y a quelque chose à écrire.
func (t TeamImport) Ready() bool { return len(t.Moves) > 0 }

// TeamImportRequest décrit ce qu'on veut reprendre en équipe.
type TeamImportRequest struct {
	// Prefix est le travail tel que les dépôts le portent, Name celui qu'il
	// prendra à l'arrivée.
	Prefix string
	Name   string
	// Only restreint la reprise aux dépôts nommés. Vide, le travail est repris
	// entier.
	Only []string
	// Members donne, pour un nom de dépôt, les comptes qui y ont accès : ce
	// sont eux, l'équipe. L'enseignant en a déjà été retiré — il a accès à
	// tout, et n'est donc l'indice de rien.
	Members map[string][]string
	// Known nomme les comptes que l'organisation connaît déjà, pour que les
	// personnes inscrites au groupe le soient sous leur nom.
	Known map[string]string
	// Existing sont les équipes que le groupe a déjà : une reprise ne recrée
	// pas ce qui est là, elle le complète.
	Existing []teams.Team
}

// PlanTeamImport compose la reprise d'un travail d'équipe : une équipe par
// dépôt, le renommage, et les personnes que cela inscrit au groupe.
func PlanTeamImport(arrivee Classroom, demande TeamImportRequest,
	repos []groups.RepoInfo) (TeamImport, error) {
	groupe := groups.Build(demande.Prefix, repos)
	if groupe.Len() == 0 {
		return TeamImport{}, valid.Errorf("Aucun dépôt ne commence par « %s ».", demande.Prefix)
	}
	if groupe = retenus(groupe, demande.Only); groupe.Len() == 0 {
		return TeamImport{}, valid.Errorf(
			"Aucun dépôt retenu : la sélection ne garde rien de « %s ».", demande.Prefix)
	}

	nom := strings.TrimSpace(demande.Name)
	if nom == "" {
		nom = groupe.Prefix
	}
	nom, err := teamAssignmentName(nom)
	if err != nil {
		return TeamImport{}, err
	}

	plan := TeamImport{
		TeamWork: true, Prefix: groupe.Prefix, Name: nom, Scope: arrivee.Scope(),
		Teams: make([]ImportedTeam, 0, groupe.Len()),
	}
	vues := map[string]string{} // nom court → dépôt qui l'a déjà pris
	membres := map[string]bool{}
	for _, depot := range groupe.Repos {
		court, err := teams.ShortName(depot.Suffix)
		if err != nil {
			return TeamImport{}, valid.Errorf(
				"« %s » : ce qui suit « %s » ne peut pas nommer une équipe. %v",
				depot.Name, groupe.Prefix, err)
		}
		if precedent, pris := vues[strings.ToLower(court)]; pris {
			return TeamImport{}, valid.Errorf(
				"« %s » et « %s » désignent la même équipe « %s ».",
				precedent, depot.Name, court)
		}
		vues[strings.ToLower(court)] = depot.Name

		equipe := ImportedTeam{
			Short: court, Name: arrivee.TeamName(court), Repo: depot.Name,
			Target: cible(arrivee, nom, court),
		}
		if deja, existe := teams.Find(demande.Existing, court); existe {
			// L'équipe est déjà là : ses membres restent, ceux du dépôt s'y
			// ajoutent. Une reprise ne défait pas ce qui a été composé.
			equipe.Exists, equipe.Members = true, deja.Members
		}
		for _, compte := range demande.Members[depot.Name] {
			if compte = strings.TrimSpace(compte); compte == "" {
				continue
			}
			if !contient(equipe.Members, compte) {
				equipe.Members = append(equipe.Members, compte)
			}
		}
		sort.Slice(equipe.Members, func(i, j int) bool {
			return strings.ToLower(equipe.Members[i]) < strings.ToLower(equipe.Members[j])
		})
		if len(equipe.Members) == 0 {
			plan.Silent = append(plan.Silent, depot.Name)
		}
		for _, compte := range equipe.Members {
			membres[strings.ToLower(compte)] = true
		}
		plan.Teams = append(plan.Teams, equipe)
	}

	plan.Students = inscrits(membres, demande.Known)
	// Le renommage est celui de n'importe quel déplacement : sans personne à
	// reconnaître, chaque dépôt garde le dernier niveau de son nom — c'est-à-
	// dire le nom de son équipe, slugifié comme elle.
	lignes, err := PlanRelocate(arrivee, nom, groupe.Repos, nil, repos)
	if err != nil {
		return plan, err
	}
	plan.Moves = lignes
	return plan, nil
}

// cible compose le nom d'arrivée d'un dépôt d'équipe. Il double « viser » —
// mais l'aperçu doit dire dès maintenant où chaque dépôt ira, équipe par
// équipe, et le plan de renommage ne se lit pas par dépôt.
func cible(arrivee Classroom, travail, equipe string) string {
	return naming.AssignmentID(arrivee.Session, arrivee.Course, arrivee.Group, travail) +
		naming.Separator + equipe
}

// teamAssignmentName valide le nom que le travail prendra. Il traverse les
// mêmes règles qu'ailleurs : un niveau du nom d'un dépôt, rien de plus.
func teamAssignmentName(value string) (string, error) {
	return valid.SlugFragment(value, "Nom du travail")
}

// inscrits rend les membres sous forme de personnes, nommées quand
// l'organisation les connaît. Un compte qu'elle ne nomme pas est inscrit quand
// même : son nom viendra du registre, ou d'une correction.
func inscrits(comptes map[string]bool, connus map[string]string) []roster.Person {
	logins := make([]string, 0, len(comptes))
	for compte := range comptes {
		logins = append(logins, compte)
	}
	sort.Strings(logins)
	people := make([]roster.Person, 0, len(logins))
	for _, login := range logins {
		people = append(people, roster.Person{
			FullName: connus[strings.ToLower(login)], Username: login,
		})
	}
	return dedupe(people)
}

func contient(liste []string, valeur string) bool {
	for _, item := range liste {
		if strings.EqualFold(item, valeur) {
			return true
		}
	}
	return false
}
