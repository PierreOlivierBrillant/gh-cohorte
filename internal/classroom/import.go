package classroom

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Reprendre des dépôts que GitHub Classroom a nommés.
//
// Ils s'appellent « travail-compte » : un préfixe commun à toute la cohorte,
// puis le compte GitHub de la personne. Rien d'autre. Ni session, ni cours, ni
// groupe, et surtout pas le nom de la personne — ce que la nomenclature à cinq
// niveaux met au dernier rang.
//
// Les importer, c'est donc répondre à trois questions dans l'ordre : quels
// travaux ces dépôts dessinent-ils, qui est derrière chaque compte, et quel nom
// chaque dépôt prendra. La première se lit dans les préfixes ; la deuxième vient
// de la liste du groupe, rapprochée si elle ne dit pas les comptes ; la
// troisième est le renommage ordinaire, celui qui déplace déjà un travail d'une
// place à l'autre.

// Foreign décrit ce qu'une organisation porte hors de la nomenclature.
type Foreign struct {
	// Repos sont les dépôts dont le nom ne se lit pas en cinq niveaux.
	Repos []string `json:"repos"`
	// Assignments sont les travaux que leurs préfixes dessinent, du plus
	// répandu au moins répandu.
	Assignments []groups.Detected `json:"assignments"`
}

// ForeignOf relève ce qu'une organisation porte hors nomenclature.
//
// Un dépôt de service n'en fait pas partie : il a déjà été écarté de
// l'inventaire, et n'appartient à personne.
func ForeignOf(repos []groups.RepoInfo) Foreign {
	noms := make([]string, 0, len(repos))
	for _, repo := range repos {
		if _, reconnu := naming.Parse(repo.Name); reconnu {
			continue
		}
		noms = append(noms, repo.Name)
	}
	sort.Strings(noms)
	return Foreign{Repos: noms, Assignments: groups.Detect(noms, 2)}
}

// Import est ce qu'une importation ferait, avant qu'elle ne le fasse.
type Import struct {
	// Prefix est le travail tel que les dépôts le portent, Name celui qu'il
	// prendra à l'arrivée.
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
	// Scope est la place d'arrivée.
	Scope string `json:"scope"`
	// Pairings dit, compte par compte, qui a été reconnu et pourquoi.
	Pairings []roster.Pairing `json:"pairings"`
	// Moves est le renommage lui-même.
	Moves []Move `json:"moves"`
	// Students sont les personnes que l'importation inscrira au groupe.
	Students []roster.Person `json:"students"`
	// Unmatched nomme les dépôts dont le compte n'a mené à personne : ils
	// resteront où ils sont.
	Unmatched []string `json:"unmatched"`
	// Absent nomme les personnes de la liste qu'aucun dépôt ne concerne.
	Absent []string `json:"absent"`
}

// Ready dit qu'il y a quelque chose à écrire.
func (i Import) Ready() bool { return len(i.Moves) > 0 }

// PlanImport compose l'importation d'un travail : le rapprochement d'abord, le
// renommage ensuite.
//
// La liste tranche quand elle porte les comptes : on ne rapproche que ce qu'elle
// laisse en blanc. Les profils GitHub — compte vers nom affiché — aident le
// rapprochement quand on les connaît, et peuvent être nils.
func PlanImport(arrivee Classroom, prefix, name string, entries []roster.Entry,
	profiles map[string]string, repos []groups.RepoInfo) (Import, error) {
	groupe := groups.Build(prefix, repos)
	if groupe.Len() == 0 {
		return Import{}, valid.Errorf("Aucun dépôt ne commence par « %s ».", prefix)
	}
	if strings.TrimSpace(name) == "" {
		name = prefix
	}

	comptes := make([]string, 0, groupe.Len())
	for _, depot := range groupe.Repos {
		comptes = append(comptes, depot.Suffix)
	}
	rapprochements := pair(entries, comptes, profiles)

	plan := Import{Prefix: groupe.Prefix, Name: name, Scope: arrivee.Scope(),
		Pairings: rapprochements}
	connus := make([]roster.Person, 0, len(rapprochements))
	vus := map[string]bool{}
	for _, trouve := range rapprochements {
		if !trouve.Found() {
			plan.Unmatched = append(plan.Unmatched, trouve.Login)
			continue
		}
		vus[strings.ToLower(trouve.Entry.FullName)] = true
		// Le compte vient du dépôt, le nom de la liste : c'est ce couple que
		// le renommage et le registre attendent.
		connus = append(connus, roster.Person{
			FullName: trouve.Entry.FullName, Username: trouve.Login,
		})
	}
	for _, entree := range entries {
		if !vus[strings.ToLower(entree.FullName)] {
			plan.Absent = append(plan.Absent, entree.FullName)
		}
	}
	plan.Students = connus

	lignes, err := PlanRelocate(arrivee.With(connus...), name, groupe.Repos, connus, repos)
	if err != nil {
		return plan, err
	}
	plan.Moves = lignes
	return plan, nil
}

// pair rapproche les comptes des personnes. Ce que la liste dit explicitement
// n'est jamais deviné : seuls les comptes qu'elle laisse en blanc passent par
// le rapprochement, et les personnes déjà prises n'y sont plus candidates.
func pair(entries []roster.Entry, logins []string,
	profiles map[string]string) []roster.Pairing {
	parCompte := map[string]roster.Entry{}
	for _, entree := range entries {
		if compte := strings.ToLower(strings.TrimSpace(entree.Username)); compte != "" {
			parCompte[compte] = entree
		}
	}

	rapprochements := make([]roster.Pairing, 0, len(logins))
	var reste []string
	for _, login := range logins {
		if entree, dite := parCompte[strings.ToLower(login)]; dite {
			rapprochements = append(rapprochements, roster.Pairing{
				Login: login, Entry: entree, Score: 100,
				Reason: "compte donné par la liste",
			})
			continue
		}
		reste = append(reste, login)
	}
	if len(reste) == 0 {
		return rapprochements
	}

	libres := make([]roster.Entry, 0, len(entries))
	for _, entree := range entries {
		if strings.TrimSpace(entree.Username) == "" {
			libres = append(libres, entree)
		}
	}
	devines := roster.Match(libres, reste, profiles)

	// L'ordre rendu suit celui des dépôts, pour que la revue se lise dans le
	// même ordre que la liste des dépôts qu'on est en train de regarder.
	parLogin := map[string]roster.Pairing{}
	for _, trouve := range append(rapprochements, devines...) {
		parLogin[trouve.Login] = trouve
	}
	ordonnes := make([]roster.Pairing, 0, len(logins))
	for _, login := range logins {
		ordonnes = append(ordonnes, parLogin[login])
	}
	return ordonnes
}
