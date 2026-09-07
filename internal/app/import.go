package app

import (
	"sort"
	"strings"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/complete"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Reprendre des dépôts qu'une autre convention a nommés — ceux de GitHub
// Classroom, « travail-compte ».
//
// L'écran suit l'ordre des questions : quels travaux sont là, qui est derrière
// chaque compte, où cela doit arriver. Rien n'est écrit avant le récapitulatif
// et l'accord.

// importSession tient l'écran d'importation.
type importSession struct {
	session *Session
	org     string
	// proprietaires dit, pour un dépôt, le compte GitHub que ses accès
	// désignent. Relevé une fois le travail choisi, il sert à chaque plan
	// refait : un rapprochement corrigé ne doit pas rendre son compte au nom.
	proprietaires map[string]string
}

// importRepos déroule l'importation, de la découverte au renommage.
func (s *Session) importRepos() (int, error) {
	ecran := &importSession{session: s, org: s.Settings.Org}
	return ecran.run()
}

func (i *importSession) run() (int, error) {
	console := i.session.Console
	console.Heading("Reprendre des dépôts de « " + i.org + " »")

	repos, err := i.session.orgRepos(i.org, false)
	if err != nil {
		return ExitFailure, err
	}
	dehors := classroom.ForeignOf(repos)
	if len(dehors.Repos) == 0 {
		console.Note("Tous les dépôts de « %s » suivent déjà la nomenclature.", i.org)
		return ExitOK, nil
	}

	prefixe, err := i.choisirTravail(dehors)
	if err != nil || prefixe == "" {
		return ExitOK, err
	}
	entrees, fichier, err := i.charger()
	if err != nil {
		return ExitOK, err
	}
	debut := classroom.AssignmentStart(prefixe, repos, func(depot string) (time.Time, error) {
		return i.session.Client.FirstCommit(i.org, depot)
	})
	place, err := i.choisirPlace(classroom.GuessPlace(fichier, entrees, debut))
	if err != nil {
		return ExitOK, err
	}
	nom := strings.TrimSpace(i.session.Options.RenameTo)
	if nom == "" {
		nom = prefixe
	}

	arrivee, err := classroom.AtScope(i.org, place,
		classroom.DefaultsFrom(i.session.Settings))
	if err != nil {
		return ExitValidation, err
	}
	// Qui a accès à quoi se lit avant le reste : c'est ce qui dit le compte de
	// chaque dépôt, et donc où finit le travail dans son nom.
	i.proprietaires = i.acces(prefixe, repos)
	plan, err := classroom.PlanImport(arrivee, classroom.ImportRequest{
		Prefix: prefixe, Name: nom, Entries: entrees,
		Profiles: i.profils(prefixe, repos), Guess: true,
		NamedOnly: i.session.Options.NamedOnly, Owners: i.proprietaires,
		Known: i.connus(),
	}, repos)
	if err != nil {
		return ExitValidation, err
	}
	if plan.Divided() {
		choisi, err := i.choisirParmiLesCaches(plan)
		if err != nil || choisi == "" {
			return ExitOK, err
		}
		if strings.TrimSpace(i.session.Options.RenameTo) == "" {
			nom = choisi
		}
		plan, err = classroom.PlanImport(arrivee, classroom.ImportRequest{
			Prefix: choisi, Name: nom, Entries: entrees,
			Profiles: i.profils(choisi, repos), Guess: true,
			NamedOnly: i.session.Options.NamedOnly, Owners: i.proprietaires,
			Known: i.connus(),
		}, repos)
		if err != nil {
			return ExitValidation, err
		}
	}
	i.montrer(plan)

	// Une ressemblance douteuse se tranche ici, avant que quoi que ce soit ne
	// soit écrit : c'est le moment où cela ne coûte rien.
	if i.session.Interactive() && !i.session.Options.Yes {
		corrige, err := i.corriger(arrivee, plan, repos)
		if err != nil {
			return ExitOK, err
		}
		plan = corrige
	}

	if i.session.Options.DryRun {
		console.Blank()
		console.Note("Simulation : rien n'a été écrit.")
		return ExitOK, nil
	}
	if !i.session.Options.Yes {
		suite, err := i.session.Prompt.Confirm(plural(
			"Renommer %d dépôt(s) et déclarer le groupe ?", len(plan.Moves)), false)
		if err != nil {
			return ExitOK, err
		}
		if !suite {
			console.Warning("Annulé : rien n'a été renommé.")
			return ExitAborted, nil
		}
	}
	return i.appliquer(arrivee, plan)
}

// choisirTravail propose les travaux devinés, ou retient celui qu'on a nommé.
func (i *importSession) choisirTravail(dehors classroom.Foreign) (string, error) {
	console := i.session.Console
	if demande := strings.TrimSpace(i.session.Options.Import); demande != "" {
		return demande, nil
	}

	console.Printf("  %s dépôt(s) hors nomenclature.", console.Info(itoa(len(dehors.Repos))))
	if len(dehors.Assignments) == 0 {
		return "", valid.Errorf(
			"Aucun préfixe commun dans ces dépôts : nommez-le avec --import.")
	}
	rows := make([][]string, 0, len(dehors.Assignments))
	for _, travail := range dehors.Assignments {
		rows = append(rows, []string{travail.Prefix, itoa(travail.Count) + " dépôt(s)"})
	}
	console.Table([]string{"Travail", "Dépôts"}, rows, 15)

	if !i.session.Interactive() {
		return "", valid.Errorf("Travail manquant : passez --import en mode non interactif.")
	}
	options := make([]ui.Option, 0, len(dehors.Assignments)+1)
	for _, travail := range dehors.Assignments {
		options = append(options, ui.Option{
			Value: travail.Prefix,
			Label: travail.Prefix + " — " + itoa(travail.Count) + " dépôt(s)",
		})
	}
	options = append(options, ui.Option{Value: "", Label: "Revenir"})
	return i.session.Prompt.Choose("Travail à reprendre", options, "")
}

// connus rend le nom complet des comptes que l'organisation sait déjà nommer :
// son registre d'abord, puis les groupes déjà déclarés sur ce poste. Un compte
// qui s'y trouve n'a pas à repasser par un rapprochement — la réponse est
// écrite, et c'est autant de questions en moins.
func (i *importSession) connus() map[string]string {
	noms := map[string]string{}
	registre, _ := i.session.names(i.org)
	for _, etudiant := range registre.All() {
		if nom := strings.TrimSpace(etudiant.FullName); nom != "" {
			noms[strings.ToLower(etudiant.Username)] = nom
		}
	}
	// Ce que le poste retient et que le registre ignore encore vaut aussi :
	// une reprise faite avant la publication en est pleine.
	store := classroom.Open(classroom.PathNextTo(i.session.ConfigFile))
	for _, personne := range store.People(i.org) {
		compte := strings.ToLower(strings.TrimSpace(personne.Username))
		if compte == "" || strings.TrimSpace(personne.FullName) == "" {
			continue
		}
		if _, deja := noms[compte]; !deja {
			noms[compte] = personne.FullName
		}
	}
	return noms
}

// acces relève, pour les dépôts d'un préfixe, le compte GitHub que leurs accès
// désignent. C'est un appel par dépôt la première fois, et rien ensuite : le
// cache les retient d'une reprise à l'autre.
func (i *importSession) acces(prefixe string, repos []groups.RepoInfo) map[string]string {
	groupe := groups.Build(prefixe, repos)
	if groupe.Len() == 0 {
		return nil
	}
	noms := make([]string, 0, groupe.Len())
	for _, depot := range groupe.Repos {
		noms = append(noms, depot.Name)
	}
	resolveur := identity.New(i.session.Client, i.session.Cache, i.session.Options.Jobs)
	spin := ui.NewSpinner(i.session.Console, "Accès aux dépôts…")
	spin.Start()
	trouves := resolveur.Owners(i.org, noms, i.session.Viewer, nil)
	spin.Stop()

	comptes := make(map[string]string, len(trouves))
	for nom, proprietaire := range trouves {
		if proprietaire.Login != "" {
			comptes[nom] = proprietaire.Login
		}
	}
	return comptes
}

// choisirParmiLesCaches tranche quand le préfixe deviné couvrait plusieurs
// travaux. « kickmyb » n'en est pas un : « kickmyb-firebase » et
// « kickmyb-android » en sont deux, et se reprennent l'un après l'autre.
func (i *importSession) choisirParmiLesCaches(plan classroom.Import) (string, error) {
	console := i.session.Console
	console.Blank()
	console.Warning("« %s » n'est pas un travail : les accès aux dépôts en révèlent %d.",
		plan.Prefix, len(plan.Splits))
	rows := make([][]string, 0, len(plan.Splits))
	for _, travail := range plan.Splits {
		rows = append(rows, []string{travail.Prefix, itoa(travail.Count) + " dépôt(s)"})
	}
	console.Table([]string{"Travail", "Dépôts"}, rows, 15)

	if !i.session.Interactive() {
		return "", valid.Errorf(
			"Plusieurs travaux sous « %s » : nommez celui à reprendre avec --import.",
			plan.Prefix)
	}
	options := make([]ui.Option, 0, len(plan.Splits)+1)
	for _, travail := range plan.Splits {
		options = append(options, ui.Option{
			Value: travail.Prefix,
			Label: travail.Prefix + " — " + itoa(travail.Count) + " dépôt(s)",
		})
	}
	options = append(options, ui.Option{Value: "", Label: "Revenir"})
	return i.session.Prompt.Choose("Travail à reprendre", options, "")
}

// charger lit la liste des étudiants, et explique où la prendre. Le chemin est
// rendu avec elle : le nom du fichier dit le cours et le groupe.
func (i *importSession) charger() ([]roster.Entry, string, error) {
	console := i.session.Console
	chemin := strings.TrimSpace(i.session.Options.Roster)
	if chemin == "" {
		if !i.session.Interactive() {
			return nil, "", valid.Errorf(
				"Liste manquante : passez --roster en mode non interactif.")
		}
		console.Blank()
		console.Heading("Liste des étudiants")
		for _, ligne := range strings.Split(roster.OmnivoxHelp, "\n") {
			console.Note("%s", ligne)
		}
		reponse, err := i.session.Prompt.Ask(ui.Question{
			Title:    "Chemin du fichier",
			Default:  i.session.Settings.RosterPath,
			Complete: complete.Path,
		})
		if err != nil {
			return nil, "", err
		}
		chemin = reponse
	}
	liste, err := roster.Load(chemin)
	if err != nil {
		return nil, "", err
	}
	for _, souci := range liste.Issues {
		console.Warning("Ligne %d : %s", souci.Line, souci.Message)
	}
	if len(liste.Entries) == 0 {
		return nil, "", valid.Errorf("Aucun étudiant dans « %s ».", chemin)
	}
	console.Printf("  %s étudiant(s) lus.", console.OK(itoa(len(liste.Entries))))
	return liste.Entries, chemin, nil
}

// choisirPlace demande où les dépôts doivent arriver, en proposant ce qui a pu
// être deviné : le cours dans le nom du fichier, le groupe dans sa colonne, la
// session dans le premier commit du travail. Rien n'est imposé — la proposition
// s'efface d'un caractère.
func (i *importSession) choisirPlace(devinee classroom.Place) (string, error) {
	if place := strings.TrimSpace(i.session.Options.Into); place != "" {
		return place, nil
	}
	if !i.session.Interactive() {
		return "", valid.Errorf("Place manquante : passez --into en mode non interactif.")
	}
	console := i.session.Console
	if len(devinee.Groups) > 0 {
		console.Warning("La liste mêle les groupes %s : indiquez celui qui reçoit ces dépôts.",
			strings.Join(devinee.Groups, ", "))
	}
	return i.session.Prompt.Ask(ui.Question{
		Title:   "Place d'arrivée, par exemple « a26.5n6.1030 »",
		Default: devinee.Scope(),
		Validate: func(valeur string) (string, error) {
			return naming.Path(valeur, "Place")
		},
	})
}

// profils demande à GitHub le nom affiché des comptes qu'on va rapprocher.
// C'est l'indice le plus sûr après le numéro d'étudiant, et il ne coûte qu'une
// requête par compte inconnu.
func (i *importSession) profils(prefixe string, repos []groups.RepoInfo) map[string]string {
	groupe := groups.Build(prefixe, repos)
	pairs := make([]identity.Pair, 0, groupe.Len())
	for _, depot := range groupe.Repos {
		// Le compte des accès d'abord : demander le profil de ce que le nom
		// portait — « firebase-Walid7Akk » — ne ramènerait rien.
		compte := depot.Suffix
		if login := i.proprietaires[depot.Name]; login != "" {
			compte = login
		}
		pairs = append(pairs, identity.Pair{Repo: compte, Login: compte})
	}
	if len(pairs) == 0 {
		return nil
	}
	resolveur := identity.New(i.session.Client, i.session.Cache, i.session.Options.Jobs)
	spin := ui.NewSpinner(i.session.Console, "Noms des profils GitHub…")
	spin.Start()
	noms := resolveur.Resolve(pairs, true, nil)
	spin.Stop()

	profils := map[string]string{}
	for compte, nom := range noms {
		if nom != "" {
			profils[strings.ToLower(compte)] = nom
		}
	}
	return profils
}

// montrer écrit ce que l'importation ferait.
func (i *importSession) montrer(plan classroom.Import) {
	console := i.session.Console
	console.Blank()
	console.Heading("Rapprochement des comptes")

	rows := make([][]string, 0, len(plan.Pairings))
	for _, trouve := range plan.Pairings {
		nom, raison := trouve.Entry.FullName, trouve.Reason
		switch {
		case trouve.Ambiguous:
			nom = console.Warn("à trancher : " + strings.Join(trouve.Rivals, ", "))
		case !trouve.Found():
			nom, raison = console.Warn("personne trouvée"), ""
		case trouve.Score < roster.Probable:
			nom = console.Warn(nom + " ?")
		}
		rows = append(rows, []string{"@" + trouve.Login, nom, raison})
	}
	console.Table([]string{"Compte", "Étudiant", "Reconnu par"}, rows, 0)

	console.Blank()
	console.Heading("Renommage")
	moves := make([][]string, 0, len(plan.Moves))
	for _, ligne := range plan.Moves {
		moves = append(moves, []string{ligne.Repo, "→ " + ligne.Target})
	}
	console.Table([]string{"Dépôt", "Deviendra"}, moves, 20)

	if len(plan.Unmatched) > 0 {
		sort := "repris sous le compte qu'ils portent, faute d'un nom"
		if plan.NamedOnly {
			sort = "laissés où ils sont"
		}
		console.Blank()
		console.Printf("  %s : %s",
			console.Warn(plural("%d dépôt(s) sans étudiant connu", len(plan.Unmatched))),
			console.Dim(sort))
	}
	if len(plan.Absent) > 0 {
		console.Blank()
		console.Printf("  %s : %s",
			console.Warn(plural("%d étudiant(s) sans dépôt pour ce travail", len(plan.Absent))),
			console.Dim(strings.Join(plan.Absent, ", ")))
	}
	// Le compte des autres vient de leurs accès : c'est le seul qui puisse
	// encore être faux.
	if len(plan.Unconfirmed) > 0 {
		console.Blank()
		console.Printf("  %s : %s",
			console.Warn(plural("%d dépôt(s) ne donnent accès à personne", len(plan.Unconfirmed))),
			console.Dim("leur compte est celui que leur nom porte"))
	}
}

// appliquer renomme, déclare le groupe, et confie les noms au registre.
func (i *importSession) appliquer(arrivee classroom.Classroom,
	plan classroom.Import) (int, error) {
	console := i.session.Console

	// Le registre passe en premier : un nom qui n'y monterait pas ne serait
	// connu que de ce poste, et c'est ce qu'on veut cesser.
	if err := i.session.apprendre(i.org, plan.Students); err != nil {
		return ExitFailure, err
	}

	progress := ui.NewProgress(console, "Renommage", len(plan.Moves))
	renommes, echecs := 0, 0
	var suivis []groups.Renamed
	for index, ligne := range plan.Moves {
		apres, err := i.session.Client.RenameRepo(i.org, ligne.Repo, ligne.Target)
		if err != nil {
			progress.Clear()
			console.Failure("%s : %v", ligne.Repo, err)
			echecs++
		} else {
			renommes++
			suivis = append(suivis, groups.Renamed{Before: ligne.Repo, After: apres.Info()})
		}
		progress.Update(index+1, ligne.Repo)
	}
	progress.Finish("")

	// L'inventaire suit les renommages plutôt que d'être relu : c'est « groups »
	// qui décide de ce qu'un renommage lui fait, et les deux interfaces s'y
	// tiennent.
	var connu []groups.RepoInfo
	if i.session.Cache.Get(cache.ReposKey(i.org), cache.ReposTTL, &connu) && len(connu) > 0 {
		i.session.Cache.Set(cache.ReposKey(i.org), groups.WithRenamed(connu, suivis))
	}

	store := classroom.Open(classroom.PathNextTo(i.session.ConfigFile))
	if _, err := store.Save(arrivee.With(plan.Students...)); err != nil {
		return ExitFailure, err
	}

	if echecs > 0 {
		console.Warning("%d dépôt(s) repris, %d en échec.", renommes, echecs)
		return ExitFailure, nil
	}
	console.Success("%d dépôt(s) repris dans « %s ».", renommes, arrivee.Scope())
	return ExitOK, nil
}

// --- corriger un rapprochement

// personneRetenue marque le choix de ne rapprocher personne. Une valeur vide
// dirait « revenir », et ce n'est pas la même décision.
const personneRetenue = "\x00personne"

// corriger laisse trancher à la main ce que le rapprochement a deviné.
//
// C'est le même jugement que l'interface web rend au même moment. Sans lui, le
// terminal ne saurait que tout accepter ou tout refuser, alors qu'une seule
// ressemblance douteuse suffit à gâcher un import.
func (i *importSession) corriger(arrivee classroom.Classroom, plan classroom.Import,
	repos []groups.RepoInfo) (classroom.Import, error) {
	console := i.session.Console

	// Tout ce que la liste portait : ceux qu'un dépôt a trouvés, et ceux
	// qu'aucun ne concerne. Les deux ensemble font le groupe.
	noms := append([]string{}, plan.Absent...)
	retenus := map[string]string{}
	for _, trouve := range plan.Pairings {
		if trouve.Found() {
			noms = append(noms, trouve.Entry.FullName)
			retenus[trouve.Login] = trouve.Entry.FullName
		}
	}
	sort.Strings(noms)

	for {
		suite, err := i.session.Prompt.Confirm("Corriger un rapprochement ?", false)
		if err != nil {
			return plan, err
		}
		if !suite {
			return i.laisserLesInconnus(arrivee, plan, noms, retenus, repos)
		}
		login, err := i.session.Prompt.Choose("Compte à corriger",
			comptesACorriger(plan, retenus), "")
		if err != nil || login == "" {
			return plan, err
		}
		nom, err := i.session.Prompt.Choose("Étudiant pour @"+login,
			etudiantsLibres(noms, retenus, login), retenus[login])
		if err != nil {
			return plan, err
		}
		if nom == "" {
			continue
		}

		delete(retenus, login)
		if nom != personneRetenue {
			// Un nom ne peut désigner qu'un compte : le donner ici le retire
			// de là où il était.
			for autre, porte := range retenus {
				if porte == nom {
					delete(retenus, autre)
				}
			}
			retenus[login] = nom
		}

		// Le plan est refait sans rien redeviner : le jugement rendu doit
		// tenir, y compris quand il consiste à ne rapprocher personne.
		refait, err := classroom.PlanImport(arrivee, classroom.ImportRequest{
			Prefix: plan.Prefix, Name: plan.Name,
			Entries: entreesRetenues(noms, retenus), NamedOnly: plan.NamedOnly,
			Owners: i.proprietaires,
		}, repos)
		if err != nil {
			return plan, err
		}
		plan = refait
		console.Blank()
		i.montrer(plan)
	}
}

// laisserLesInconnus propose de laisser derrière les dépôts dont personne n'a
// été reconnu. Sans cela ils entrent dans la nomenclature sous le compte qu'ils
// portent : un dernier niveau qui n'est pas un nom. Les deux se défendent, et
// la question ne se pose que lorsqu'il en reste.
func (i *importSession) laisserLesInconnus(arrivee classroom.Classroom,
	plan classroom.Import, noms []string, retenus map[string]string,
	repos []groups.RepoInfo) (classroom.Import, error) {
	if len(plan.Unmatched) == 0 || plan.NamedOnly {
		return plan, nil
	}
	laisser, err := i.session.Prompt.Confirm(plural(
		"Laisser où ils sont les %d dépôt(s) dont l'étudiant est inconnu ?",
		len(plan.Unmatched)), false)
	if err != nil || !laisser {
		return plan, err
	}
	refait, err := classroom.PlanImport(arrivee, classroom.ImportRequest{
		Prefix: plan.Prefix, Name: plan.Name,
		Entries: entreesRetenues(noms, retenus), NamedOnly: true,
		Owners: i.proprietaires,
	}, repos)
	if err != nil {
		return plan, err
	}
	i.session.Console.Blank()
	i.montrer(refait)
	return refait, nil
}

// comptesACorriger énumère les dépôts et ce à quoi ils mènent pour l'instant.
func comptesACorriger(plan classroom.Import, retenus map[string]string) []ui.Option {
	options := make([]ui.Option, 0, len(plan.Pairings)+1)
	for _, trouve := range plan.Pairings {
		nom := retenus[trouve.Login]
		if nom == "" {
			nom = "personne"
		}
		options = append(options, ui.Option{
			Value: trouve.Login, Label: "@" + trouve.Login + " → " + nom,
		})
	}
	return append(options, ui.Option{Value: "", Label: "Terminer"})
}

// etudiantsLibres ne propose que ce qui reste : une personne déjà donnée à un
// autre compte ne peut pas l'être deux fois.
func etudiantsLibres(noms []string, retenus map[string]string, login string) []ui.Option {
	pris := map[string]bool{}
	for autre, nom := range retenus {
		if autre != login {
			pris[nom] = true
		}
	}
	options := []ui.Option{{Value: personneRetenue, Label: "— personne —"}}
	for _, nom := range noms {
		if !pris[nom] {
			options = append(options, ui.Option{Value: nom, Label: nom})
		}
	}
	return append(options, ui.Option{Value: "", Label: "Revenir"})
}

// entreesRetenues rend la liste telle que les corrections l'ont laissée : un
// nom par personne, et le compte qu'on lui a donné.
func entreesRetenues(noms []string, retenus map[string]string) []roster.Entry {
	parNom := map[string]string{}
	for login, nom := range retenus {
		parNom[nom] = login
	}
	liste := make([]roster.Entry, 0, len(noms))
	for _, nom := range noms {
		liste = append(liste, roster.Entry{FullName: nom, Username: parNom[nom]})
	}
	return liste
}
