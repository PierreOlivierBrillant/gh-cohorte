package classroom

import (
	"sort"
	"strings"
	"time"

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

// ImportRequest décrit ce qu'on veut reprendre.
type ImportRequest struct {
	// Prefix est le travail tel que les dépôts le portent, Name celui qu'il
	// prendra à l'arrivée.
	Prefix string
	Name   string
	// Entries est la liste du groupe. Une entrée qui porte un compte le dit ;
	// les autres restent à rapprocher.
	Entries []roster.Entry
	// Profiles associe un compte au nom affiché de son profil GitHub. C'est
	// l'indice le plus sûr après le numéro d'étudiant ; il peut être nil.
	Profiles map[string]string
	// Guess autorise le rapprochement des comptes que la liste ne nomme pas.
	//
	// Une fois qu'on a corrigé un rapprochement à l'écran, non : le jugement
	// rendu doit tenir, y compris quand il consiste à ne rapprocher personne.
	// Redeviner alors déferait ce qu'on vient de décider.
	Guess bool
}

// PlanImport compose l'importation d'un travail : le rapprochement d'abord, le
// renommage ensuite.
func PlanImport(arrivee Classroom, demande ImportRequest,
	repos []groups.RepoInfo) (Import, error) {
	prefix, name, entries := demande.Prefix, demande.Name, demande.Entries
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
	rapprochements := pair(entries, comptes, demande.Profiles, demande.Guess)

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
	profiles map[string]string, guess bool) []roster.Pairing {
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
	if len(reste) == 0 || !guess {
		// Sans rapprochement, les comptes que la liste ne nomme pas restent
		// sans réponse : c'est ce que la personne a décidé.
		for _, login := range reste {
			rapprochements = append(rapprochements, roster.Pairing{Login: login})
		}
		return ordonner(rapprochements, logins)
	}

	libres := make([]roster.Entry, 0, len(entries))
	for _, entree := range entries {
		if strings.TrimSpace(entree.Username) == "" {
			libres = append(libres, entree)
		}
	}
	devines := roster.Match(libres, reste, profiles)

	return ordonner(append(rapprochements, devines...), logins)
}

// ordonner range les rapprochements dans l'ordre des dépôts, pour que la revue
// se lise dans le même ordre que ce qu'on est en train de regarder.
func ordonner(rapprochements []roster.Pairing, logins []string) []roster.Pairing {
	parLogin := map[string]roster.Pairing{}
	for _, trouve := range rapprochements {
		parLogin[trouve.Login] = trouve
	}
	ordonnes := make([]roster.Pairing, 0, len(logins))
	for _, login := range logins {
		ordonnes = append(ordonnes, parLogin[login])
	}
	return ordonnes
}

// --- deviner la place d'arrivée

// Trois questions, trois sources. Le cours est dans le nom du fichier
// d'Omnivox, le groupe dans sa colonne « Groupe », et la session dans la date
// du premier commit d'un dépôt : un travail se fait pendant la session où on
// l'a donné, et son plus vieux commit tombe dedans.
//
// Aucune des trois n'est sûre, et c'est pourquoi elles ne font que préremplir :
// tout reste modifiable, et ce qui n'a pas pu être deviné reste vide plutôt que
// d'être inventé.

// Place est la place d'arrivée telle qu'on peut la deviner.
type Place struct {
	Session string `json:"session"`
	Course  string `json:"course"`
	Group   string `json:"group"`
	// Groups nomme les groupes quand la liste en mêle plusieurs : aucun ne
	// peut alors être choisi à la place de quelqu'un.
	Groups []string `json:"groups,omitempty"`
}

// GuessPlace devine la place d'arrivée depuis la liste, son nom de fichier, et
// la date du premier commit du travail. Une date nulle ne dit rien de la
// session : le champ reste vide.
func GuessPlace(filename string, entries []roster.Entry, premier time.Time) Place {
	indices := roster.HintsFrom(filename, entries)
	devinee := Place{Course: indices.Course, Group: indices.Group, Groups: indices.Groups}
	if !premier.IsZero() {
		devinee.Session = naming.SessionAt(premier)
	}
	return devinee
}

// depotsInterroges borne la recherche de la date : si les cinq premiers dépôts
// d'un travail sont vides, c'est que personne n'a encore rien remis, et
// interroger les trente-cinq autres ne changerait rien qu'au temps d'attente.
const depotsInterroges = 5

// FirstCommitLookup rend la date du premier commit d'un dépôt, ou une date
// nulle s'il n'en a pas.
type FirstCommitLookup func(repo string) (time.Time, error)

// AssignmentStart cherche quand un travail a commencé : la date du plus vieux
// commit qu'un de ses dépôts porte. Un seul suffit — ils ont tous été donnés le
// même jour, et une session dure des mois.
func AssignmentStart(prefix string, repos []groups.RepoInfo, lire FirstCommitLookup) time.Time {
	groupe := groups.Build(prefix, repos)
	for index, depot := range groupe.Repos {
		if index >= depotsInterroges {
			break
		}
		if moment, err := lire(depot.Name); err == nil && !moment.IsZero() {
			return moment
		}
	}
	return time.Time{}
}

// Scope rend la place sous la forme « a26.5n6.1030 », ou rien s'il manque un
// niveau : une place à trous ne désigne aucun dépôt.
func (p Place) Scope() string {
	if p.Session == "" || p.Course == "" || p.Group == "" {
		return ""
	}
	return naming.Prefix(p.Session, p.Course, p.Group)
}
