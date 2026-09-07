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
	// Unmatched nomme les dépôts dont le compte n'a mené à personne.
	Unmatched []string `json:"unmatched"`
	// NamedOnly dit ce qu'il advient d'eux : laissés où ils sont, ou repris
	// sous le compte qu'ils portent faute d'un nom à leur donner.
	NamedOnly bool `json:"named_only"`
	// Absent nomme les personnes de la liste qu'aucun dépôt ne concerne.
	Absent []string `json:"absent"`
	// Splits nomme les travaux distincts que les accès ont révélés sous le
	// préfixe demandé. Un seul dans le cas ordinaire ; plusieurs veut dire que
	// le préfixe est un fourre-tout — « kickmyb » pour « kickmyb-firebase » et
	// « kickmyb-android » —, qu'il n'y a rien à reprendre tel quel, et qu'il
	// faut choisir lequel.
	Splits []groups.Detected `json:"splits"`
	// Unconfirmed nomme les dépôts dont aucun accès n'a désigné la personne :
	// leur compte est celui que le nom porte, faute de mieux, et c'est le seul
	// endroit où il peut encore être faux.
	Unconfirmed []string `json:"unconfirmed"`
}

// Divided dit que le préfixe demandé couvre plusieurs travaux : il y a une
// question à poser avant de pouvoir écrire quoi que ce soit.
func (i Import) Divided() bool { return len(i.Splits) > 1 }

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
	// Owners donne, pour un nom de dépôt, le compte GitHub que ses accès
	// désignent. C'est la seule source sûre : un nom de dépôt ne dit pas où
	// finit le travail, et le découper au jugé invente des comptes qui
	// n'existent pas. Ce que la carte ne dit pas retombe sur le nom, faute de
	// mieux — voir « Unconfirmed ».
	Owners map[string]string
	// Guess autorise le rapprochement des comptes que la liste ne nomme pas.
	//
	// Une fois qu'on a corrigé un rapprochement à l'écran, non : le jugement
	// rendu doit tenir, y compris quand il consiste à ne rapprocher personne.
	// Redeviner alors déferait ce qu'on vient de décider.
	Guess bool
	// NamedOnly laisse où ils sont les dépôts dont on ne connaît pas la
	// personne, plutôt que de les reprendre sous le compte qu'ils portent.
	//
	// Les deux se défendent. Reprendre le compte garde le travail entier et
	// laisse corriger le nom plus tard ; laisser derrière évite d'inscrire
	// dans la nomenclature un dernier niveau qui n'est pas un nom. C'est à
	// qui importe de trancher.
	NamedOnly bool
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

	// Les accès disent qui est derrière chaque dépôt ; le nom, lui, ne dit
	// alors plus que le travail. Un préfixe qui en cache plusieurs se voit ici,
	// et nulle part ailleurs.
	lus := lire(groupe, demande.Owners)
	travaux := parTravail(lus, groupe.Prefix)
	if len(travaux) > 1 {
		return Import{Prefix: groupe.Prefix, Name: name, Scope: arrivee.Scope(),
			Splits: travaux, NamedOnly: demande.NamedOnly}, nil
	}
	travail := travaux[0].Prefix
	// Le nom laissé tel quel suit le travail que les accès ont révélé : qui a
	// choisi « kickmyb » sans y toucher visait « kickmyb-firebase », le seul
	// travail qui s'y trouvait. Un nom tapé, lui, est un choix, et il tient.
	if strings.TrimSpace(name) == "" || strings.EqualFold(strings.TrimSpace(name), prefix) {
		name = travail
	}
	groupe = groups.Group{Prefix: travail, Repos: comptes(lus)}

	logins := make([]string, 0, groupe.Len())
	for _, depot := range groupe.Repos {
		logins = append(logins, depot.Suffix)
	}
	rapprochements := pair(entries, logins, demande.Profiles, demande.Guess)

	plan := Import{Prefix: groupe.Prefix, Name: name, Scope: arrivee.Scope(),
		Pairings: rapprochements, NamedOnly: demande.NamedOnly,
		Splits: travaux, Unconfirmed: sansAcces(lus)}
	connus := make([]roster.Person, 0, len(rapprochements))
	vus := map[string]bool{}
	nommes := map[string]bool{}
	for _, trouve := range rapprochements {
		if !trouve.Found() {
			plan.Unmatched = append(plan.Unmatched, trouve.Login)
			continue
		}
		nommes[strings.ToLower(trouve.Login)] = true
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

	lignes, err := PlanRelocate(arrivee.With(connus...), name,
		aReprendre(groupe.Repos, nommes, demande.NamedOnly), connus, repos)
	if err != nil {
		return plan, err
	}
	plan.Moves = lignes
	return plan, nil
}

// depotLu est un dépôt du travail, tel que ses accès l'éclairent : le compte de
// la personne, le travail que son nom porte une fois ce compte retiré, et si
// tout cela vient des accès ou seulement du nom.
type depotLu struct {
	repo    groups.Repo
	login   string
	travail string
	sur     bool
}

// lire relit les dépôts d'un préfixe à la lumière des accès. Sans accès connu,
// le nom reste seul juge — c'est ce que faisait l'outil avant de savoir les
// lire, et ce qu'il continue de faire pour un dépôt auquel personne n'est
// rattaché.
func lire(groupe groups.Group, proprietaires map[string]string) []depotLu {
	lus := make([]depotLu, 0, groupe.Len())
	for _, depot := range groupe.Repos {
		lu := depotLu{repo: depot, login: depot.Suffix, travail: groupe.Prefix}
		if login := strings.TrimSpace(proprietaires[depot.Name]); login != "" {
			lu.login, lu.sur = login, true
			if travail, coupe := groups.Split(depot.Name, login); coupe {
				lu.travail = travail
			}
		}
		lus = append(lus, lu)
	}
	return lus
}

// parTravail range les dépôts par le travail que leur nom porte, du plus fourni
// au moins fourni. Un préfixe ordinaire n'en donne qu'un ; un fourre-tout en
// donne autant qu'il en cache.
func parTravail(lus []depotLu, defaut string) []groups.Detected {
	comptes := map[string]int{}
	tels := map[string]string{} // minuscules → travail tel qu'il s'écrit
	for _, lu := range lus {
		nom := lu.travail
		if strings.TrimSpace(nom) == "" {
			nom = defaut
		}
		cle := strings.ToLower(nom)
		comptes[cle]++
		if _, deja := tels[cle]; !deja {
			tels[cle] = nom
		}
	}
	travaux := make([]groups.Detected, 0, len(comptes))
	for cle, combien := range comptes {
		travaux = append(travaux, groups.Detected{Prefix: tels[cle], Count: combien})
	}
	sort.Slice(travaux, func(i, j int) bool {
		if travaux[i].Count != travaux[j].Count {
			return travaux[i].Count > travaux[j].Count
		}
		return travaux[i].Prefix < travaux[j].Prefix
	})
	return travaux
}

// comptes rend les dépôts avec, pour dernier niveau, le compte que les accès
// désignent. Tout ce qui suit — le rapprochement, le renommage, le repli quand
// personne n'est connu — s'appuie sur ce champ, et n'a donc rien à savoir des
// accès.
func comptes(lus []depotLu) []groups.Repo {
	depots := make([]groups.Repo, 0, len(lus))
	for _, lu := range lus {
		depot := lu.repo
		depot.Suffix = lu.login
		depots = append(depots, depot)
	}
	return depots
}

// sansAcces nomme les dépôts dont le compte n'a pas été confirmé.
func sansAcces(lus []depotLu) []string {
	var noms []string
	for _, lu := range lus {
		if !lu.sur {
			noms = append(noms, lu.repo.Name)
		}
	}
	return noms
}

// aReprendre retient les dépôts qui seront renommés. Sans NamedOnly ils le sont
// tous, celui dont on ignore la personne compris : il garde alors le compte
// qu'il porte comme dernier niveau, faute d'un nom à lui donner.
func aReprendre(depots []groups.Repo, nommes map[string]bool, nommesSeulement bool) []groups.Repo {
	if !nommesSeulement {
		return depots
	}
	retenus := make([]groups.Repo, 0, len(depots))
	for _, depot := range depots {
		if nommes[strings.ToLower(depot.Suffix)] {
			retenus = append(retenus, depot)
		}
	}
	return retenus
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
