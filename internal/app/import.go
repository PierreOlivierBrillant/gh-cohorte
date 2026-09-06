package app

import (
	"strings"

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
	entrees, err := i.charger()
	if err != nil {
		return ExitOK, err
	}
	place, err := i.choisirPlace()
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
	plan, err := classroom.PlanImport(arrivee, prefixe, nom, entrees,
		i.profils(prefixe, repos), repos)
	if err != nil {
		return ExitValidation, err
	}
	i.montrer(plan)

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

// charger lit la liste des étudiants, et explique où la prendre.
func (i *importSession) charger() ([]roster.Entry, error) {
	console := i.session.Console
	chemin := strings.TrimSpace(i.session.Options.Roster)
	if chemin == "" {
		if !i.session.Interactive() {
			return nil, valid.Errorf(
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
			return nil, err
		}
		chemin = reponse
	}
	liste, err := roster.Load(chemin)
	if err != nil {
		return nil, err
	}
	for _, souci := range liste.Issues {
		console.Warning("Ligne %d : %s", souci.Line, souci.Message)
	}
	if len(liste.Entries) == 0 {
		return nil, valid.Errorf("Aucun étudiant dans « %s ».", chemin)
	}
	console.Printf("  %s étudiant(s) lus.", console.OK(itoa(len(liste.Entries))))
	return liste.Entries, nil
}

// choisirPlace demande où les dépôts doivent arriver.
func (i *importSession) choisirPlace() (string, error) {
	if place := strings.TrimSpace(i.session.Options.Into); place != "" {
		return place, nil
	}
	if !i.session.Interactive() {
		return "", valid.Errorf("Place manquante : passez --into en mode non interactif.")
	}
	return i.session.Prompt.Ask(ui.Question{
		Title: "Place d'arrivée, par exemple « a26.5n6.1030 »",
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
		pairs = append(pairs, identity.Pair{Repo: depot.Suffix, Login: depot.Suffix})
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

	if len(plan.Absent) > 0 {
		console.Blank()
		console.Printf("  %s : %s",
			console.Warn(plural("%d étudiant(s) sans dépôt pour ce travail", len(plan.Absent))),
			console.Dim(strings.Join(plan.Absent, ", ")))
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
