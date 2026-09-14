package classroom

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
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
	// Members sont les comptes qui composeront l'équipe, et Sources dit d'où
	// chacun vient — de l'équipe GitHub du dépôt, de ses accès, ou de ses
	// commits. Une composition devinée doit pouvoir être démentie, et pour cela
	// il faut voir sur quoi elle repose.
	Members []string          `json:"members"`
	Sources map[string]string `json:"sources,omitempty"`
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
	// membres de toutes les équipes, réunis et nommés.
	Students []roster.Person `json:"students"`
	// Pairings dit, compte par compte, qui a été reconnu et pourquoi. Les
	// équipes disent qui a fait le travail ; elles ne disent pas son nom, et
	// c'est la liste du groupe qui le donne.
	Pairings []roster.Pairing `json:"pairings"`
	// Unmatched nomme les comptes qu'aucune personne de la liste ne nomme. Ils
	// rejoignent quand même le groupe : leur équipe les y a mis, et leur nom
	// se corrige ensuite.
	Unmatched []string `json:"unmatched"`
	// Absent nomme les personnes de la liste qu'aucune équipe ne réclame.
	Absent []string `json:"absent"`
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
	// Members donne, pour un nom de dépôt, les comptes qui l'ont fait, et ce
	// qui les y a fait reconnaître. L'enseignant en a déjà été retiré — il a
	// accès à tout, et n'est donc l'indice de rien.
	Members map[string]identity.Crew
	// Chosen impose la composition d'une équipe, par son nom court. C'est le
	// dernier mot : ce qui a été corrigé à l'écran ne se redevine pas.
	Chosen map[string][]string
	// Known nomme les comptes que l'organisation connaît déjà, pour que les
	// personnes inscrites au groupe le soient sous leur nom.
	Known map[string]string
	// Entries est la liste du groupe. Les équipes disent qui a fait le
	// travail ; c'est elle qui dit comment ces gens s'appellent.
	Entries []roster.Entry
	// Profiles associe un compte au nom affiché de son profil GitHub : l'indice
	// le plus sûr après le numéro d'étudiant. Il peut être nil.
	Profiles map[string]string
	// Guess autorise le rapprochement des comptes que la liste ne nomme pas.
	// Une fois qu'on a corrigé un rapprochement à l'écran, non : le jugement
	// rendu doit tenir, y compris quand il consiste à ne rapprocher personne.
	Guess bool
	// Existing sont les équipes que le groupe a déjà : une reprise ne recrée
	// pas ce qui est là, elle le complète.
	Existing []teams.Team
}

// Sources d'un membre que « identity » ne donne pas : ce qu'une équipe déjà
// déclarée retient, et ce qu'une main a tranché.
const (
	FromTeamItself = "équipe du groupe"
	ChosenByHand   = "choisi"
)

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

	// Les compositions tranchées à l'écran arrivent telles qu'on les a écrites :
	// c'est le nom court, mis en forme, qui les retrouve.
	choisis := make(map[string][]string, len(demande.Chosen))
	for court, voulus := range demande.Chosen {
		if nom, err := teams.ShortName(court); err == nil {
			choisis[strings.ToLower(nom)] = voulus
		}
	}

	plan := TeamImport{
		TeamWork: true, Prefix: groupe.Prefix, Name: nom, Scope: arrivee.Scope(),
		Teams: make([]ImportedTeam, 0, groupe.Len()),
	}
	vues := map[string]string{} // nom court → dépôt qui l'a déjà pris
	depots := make([]groups.Repo, 0, groupe.Len())
	courts := make([]string, 0, groupe.Len())
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
		depots = append(depots, depot)
		courts = append(courts, court)
	}

	// Une personne n'est que d'une équipe à la fois : ce qui a été tranché à
	// l'écran est donc composé d'abord, et ce qui est deviné ne vient pas le
	// défaire en réclamant quelqu'un qui a déjà sa place.
	ordre := make([]int, 0, len(depots))
	for rang, court := range courts {
		if _, decide := choisis[strings.ToLower(court)]; decide {
			ordre = append(ordre, rang)
		}
	}
	for rang, court := range courts {
		if _, decide := choisis[strings.ToLower(court)]; !decide {
			ordre = append(ordre, rang)
		}
	}

	membres := map[string]bool{}
	places := map[string]bool{} // comptes déjà rangés dans une équipe
	composees := make(map[int]ImportedTeam, len(depots))
	for _, rang := range ordre {
		depot, court := depots[rang], courts[rang]
		equipe := ImportedTeam{
			Short: court, Name: arrivee.TeamName(court), Repo: depot.Name,
			Target: cible(arrivee, nom, court), Sources: map[string]string{},
		}
		if _, existe := teams.Find(demande.Existing, court); existe {
			equipe.Exists = true
		}
		ajouter := func(compte, source string) {
			if compte = strings.TrimSpace(compte); compte == "" {
				return
			}
			if places[strings.ToLower(compte)] {
				return
			}
			places[strings.ToLower(compte)] = true
			equipe.Members = append(equipe.Members, compte)
			equipe.Sources[compte] = source
		}

		// Une composition arrêtée à l'écran a le dernier mot : la redeviner
		// déferait ce qu'on vient de décider, y compris quand cela consiste à
		// n'y mettre personne.
		if voulus, decide := choisis[strings.ToLower(court)]; decide {
			for _, compte := range voulus {
				ajouter(compte, ChosenByHand)
			}
		} else {
			if deja, existe := teams.Find(demande.Existing, court); existe {
				// L'équipe est déjà là : ses membres restent, ceux du dépôt s'y
				// ajoutent. Une reprise ne défait pas ce qui a été composé.
				for _, compte := range deja.Members {
					ajouter(compte, FromTeamItself)
				}
			}
			for _, membre := range demande.Members[depot.Name].Members {
				ajouter(membre.Login, membre.Source)
			}
		}
		for _, compte := range equipe.Members {
			membres[strings.ToLower(compte)] = true
		}
		composees[rang] = equipe
	}

	// Rendues dans l'ordre des dépôts, et non dans celui où elles ont été
	// composées : l'écran doit les lire comme il les a montrées.
	for rang := range depots {
		equipe := composees[rang]
		if len(equipe.Members) == 0 {
			plan.Silent = append(plan.Silent, equipe.Repo)
		}
		plan.Teams = append(plan.Teams, equipe)
	}

	// Les équipes disent qui a fait le travail ; la liste dit comment ces gens
	// s'appellent. C'est le même rapprochement que pour un travail individuel,
	// appliqué aux membres plutôt qu'aux propriétaires des dépôts.
	logins := make([]string, 0, len(membres))
	for _, equipe := range plan.Teams {
		logins = append(logins, equipe.Members...)
	}
	plan.Pairings = pair(demande.Entries, logins,
		demande.Profiles, demande.Known, demande.Guess)

	nommes := map[string]bool{}
	vus := map[string]bool{}
	people := make([]roster.Person, 0, len(logins))
	for _, trouve := range plan.Pairings {
		if !trouve.Found() {
			plan.Unmatched = append(plan.Unmatched, trouve.Login)
			// Un compte sans nom rejoint quand même le groupe : son équipe l'y
			// a mis, et le taire le ferait disparaître de la liste.
			people = append(people, roster.Person{Username: trouve.Login})
			continue
		}
		nommes[strings.ToLower(trouve.Login)] = true
		retenir(vus, trouve.Entry.FullName)
		people = append(people, roster.Person{
			FullName: trouve.Entry.FullName, Username: trouve.Login,
			StudentID: trouve.Entry.StudentID,
		})
	}
	for _, entree := range demande.Entries {
		// Comparé comme les noms le sont partout ailleurs : une accentuation
		// ou une casse ne fait pas une personne de plus.
		if cle := valid.Slugify(entree.FullName); cle == "" || !vus[cle] {
			plan.Absent = append(plan.Absent, entree.FullName)
		}
	}
	plan.Students = dedupe(people)
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

func contient(liste []string, valeur string) bool {
	for _, item := range liste {
		if strings.EqualFold(item, valeur) {
			return true
		}
	}
	return false
}
