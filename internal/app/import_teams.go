package app

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
)

// Reprendre un travail d'équipe au terminal. Le parcours est celui de la
// reprise ordinaire — même choix de travail, mêmes dépôts retenus, même place
// d'arrivée — et s'en écarte là où il le doit : rien n'est rapproché d'une
// liste, puisque le dernier niveau du nom désigne une équipe.

// membres rend, pour chaque dépôt, les comptes qui y ont accès : dans un
// travail d'équipe, c'est l'équipe elle-même. L'enseignant en est écarté — il a
// accès à tout, et n'est donc l'indice de rien.
func (i *importSession) membres(prefixe string, seulement []string,
	repos []groups.RepoInfo) map[string][]string {
	noms := aLire(prefixe, seulement, repos)
	if len(noms) == 0 {
		return nil
	}
	resolveur := identity.New(i.session.Client, i.session.Cache, i.session.Options.Jobs)
	spin := ui.NewSpinner(i.session.Console, "Accès aux dépôts…")
	spin.Start()
	trouves := resolveur.Owners(i.org, noms, i.session.Viewer, nil)
	spin.Stop()

	equipes := make(map[string][]string, len(trouves))
	for nom, proprietaire := range trouves {
		comptes := make([]string, 0, len(proprietaire.Access))
		for _, compte := range proprietaire.Access {
			if compte == "" || strings.EqualFold(compte, i.session.Viewer) {
				continue
			}
			comptes = append(comptes, compte)
		}
		equipes[nom] = comptes
	}
	return equipes
}

// aLire nomme les dépôts d'un préfixe dont les accès sont à lire.
func aLire(prefixe string, seulement []string, repos []groups.RepoInfo) []string {
	groupe := groups.Build(prefixe, repos)
	if groupe.Len() == 0 {
		return nil
	}
	gardes := map[string]bool{}
	for _, nom := range seulement {
		gardes[strings.ToLower(strings.TrimSpace(nom))] = true
	}
	noms := make([]string, 0, groupe.Len())
	for _, depot := range groupe.Repos {
		if len(gardes) > 0 && !gardes[strings.ToLower(depot.Name)] {
			continue
		}
		noms = append(noms, depot.Name)
	}
	return noms
}

// enEquipe déroule la reprise d'un travail d'équipe, une fois le travail, les
// dépôts et la place choisis.
func (i *importSession) enEquipe(arrivee classroom.Classroom, prefixe, nom string,
	repos []groups.RepoInfo) (int, error) {
	console := i.session.Console

	infos, err := i.session.Client.LoadOrgTeams(i.org, i.session.Options.Jobs)
	if err != nil {
		return ExitFailure, err
	}
	plan, err := classroom.PlanTeamImport(arrivee, classroom.TeamImportRequest{
		Prefix: prefixe, Name: nom, Only: i.retenus,
		Members:  i.membres(prefixe, i.retenus, repos),
		Known:    i.connus(),
		Existing: arrivee.Teams(infos),
	}, repos)
	if err != nil {
		return ExitValidation, err
	}
	i.montrerEquipes(plan)

	if i.session.Options.DryRun {
		console.Blank()
		console.Note("Simulation : rien n'a été écrit.")
		return ExitOK, nil
	}
	if !i.session.Options.Yes {
		suite, err := i.session.Prompt.Confirm(plural(
			"Renommer %d dépôt(s), composer leurs équipes et leur en donner l'accès ?",
			len(plan.Moves)), false)
		if err != nil {
			return ExitOK, err
		}
		if !suite {
			console.Warning("Annulé : rien n'a été renommé.")
			return ExitAborted, nil
		}
	}
	return i.appliquerEquipes(arrivee, plan)
}

// montrerEquipes récapitule la reprise avant toute écriture.
func (i *importSession) montrerEquipes(plan classroom.TeamImport) {
	console := i.session.Console
	console.Heading("Reprise en équipe de « " + plan.Prefix + " »")

	lignes := make([][]string, 0, len(plan.Teams))
	for _, equipe := range plan.Teams {
		etat := "à créer"
		if equipe.Exists {
			etat = "déjà là"
		}
		lignes = append(lignes, []string{
			equipe.Short, equipe.Repo + " → " + equipe.Target,
			membresEnMots(equipe.Members), etat,
		})
	}
	console.Table([]string{"Équipe", "Dépôt", "Membres", "État"}, lignes, 50)
	console.Printf("  %s personne(s) rejoindront « %s ».",
		console.OK(itoa(len(plan.Students))), plan.Scope)
	if len(plan.Silent) > 0 {
		console.Warning("Aucun accès sur %s : leur équipe naîtra vide.",
			strings.Join(plan.Silent, ", "))
	}
}

// appliquerEquipes écrit la reprise : les dépôts d'abord, les équipes ensuite,
// le partage en dernier. Un renommage qui échoue arrête tout ; une équipe qui
// échoue laisse des dépôts bien nommés, que « --team-share » achèvera.
func (i *importSession) appliquerEquipes(arrivee classroom.Classroom,
	plan classroom.TeamImport) (int, error) {
	console := i.session.Console

	// Le registre passe en premier : un nom qui n'y monterait pas ne serait
	// connu que de ce poste.
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

	var connu []groups.RepoInfo
	if i.session.Cache.Get(cache.ReposKey(i.org), cache.ReposTTL, &connu) && len(connu) > 0 {
		i.session.Cache.Set(cache.ReposKey(i.org), groups.WithRenamed(connu, suivis))
	}
	if echecs > 0 {
		console.Warning("%d dépôt(s) repris, %d en échec. "+
			"Les équipes n'ont pas été composées.", renommes, echecs)
		return ExitFailure, nil
	}

	faites, partages, ratees := i.composerEquipes(arrivee, plan)

	store := classroom.Open(classroom.PathNextTo(i.session.ConfigFile))
	if _, err := store.Save(arrivee.With(plan.Students...)); err != nil {
		return ExitFailure, err
	}
	if ratees > 0 {
		console.Warning("%d équipe(s) composée(s), %d en échec. "+
			"Relancez avec « --team-share » pour achever le partage.", faites, ratees)
		return ExitFailure, nil
	}
	console.Success("%d dépôt(s) repris dans « %s », %d équipe(s) composée(s), "+
		"%d partage(s).", renommes, arrivee.Scope(), faites, partages)
	return ExitOK, nil
}

// composerEquipes crée les équipes, y inscrit leurs membres, et leur partage
// leur dépôt.
func (i *importSession) composerEquipes(arrivee classroom.Classroom,
	plan classroom.TeamImport) (faites, partages, ratees int) {
	console := i.session.Console
	droit := arrivee.Settings(plan.Name).Permission

	for _, equipe := range plan.Teams {
		slug, err := i.assurerEquipe(arrivee, equipe)
		if err != nil {
			ratees++
			console.Failure("équipe %s : %v", equipe.Short, err)
			continue
		}
		faites++
		console.Printf("  %s équipe %s — %s", console.OK("✓"), equipe.Short,
			membresEnMots(equipe.Members))

		if err := i.session.Client.GrantTeamRepo(
			arrivee.Org, slug, arrivee.Org, equipe.Target, droit); err != nil {
			console.Failure("%s : partage impossible — %v", equipe.Target, err)
			continue
		}
		partages++
	}
	return faites, partages, ratees
}

// assurerEquipe crée l'équipe si elle manque, puis la compose. Elle rend
// l'adresse GitHub de l'équipe, celle par laquelle on lui partage un dépôt.
func (i *importSession) assurerEquipe(arrivee classroom.Classroom,
	equipe classroom.ImportedTeam) (string, error) {
	infos, err := i.session.Client.LoadOrgTeams(arrivee.Org, i.session.Options.Jobs)
	if err != nil {
		return "", err
	}
	presentes := arrivee.Teams(infos)
	trouvee, existe := teams.Find(presentes, equipe.Short)
	if !existe {
		cree, err := i.session.Client.CreateTeam(arrivee.Org, equipe.Name,
			teams.Describe(arrivee.Session, arrivee.Course, arrivee.Group, equipe.Short),
			teams.Privacy)
		if err != nil {
			return "", err
		}
		trouvee, _ = teams.Read(*cree)
		presentes = append(presentes, trouvee)
	}

	etapes, err := teams.PlanCompose(presentes, equipe.Short, equipe.Members)
	if err != nil {
		return trouvee.Slug, err
	}
	for _, etape := range etapes {
		if etape.Kind == teams.Join {
			err = i.session.Client.AddTeamMember(arrivee.Org, etape.Slug,
				etape.Username, teams.MemberRole)
		} else {
			err = i.session.Client.RemoveTeamMember(arrivee.Org, etape.Slug, etape.Username)
		}
		if err != nil {
			return trouvee.Slug, err
		}
	}
	return trouvee.Slug, nil
}

// membresEnMots dit la composition d'une équipe.
func membresEnMots(membres []string) string {
	if len(membres) == 0 {
		return "aucun membre connu"
	}
	comptes := make([]string, 0, len(membres))
	for _, compte := range membres {
		comptes = append(comptes, "@"+compte)
	}
	return strings.Join(comptes, ", ")
}
