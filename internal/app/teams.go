package app

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Les équipes au terminal. L'assistant travaille par préfixe et ignore la
// notion de groupe ; une équipe, elle, appartient à un groupe. Le préfixe fait
// donc office de place : « --manage a26.5n6.01 » désigne le groupe, et c'est
// sur lui que portent les drapeaux d'équipe.
//
// Rien n'est décidé ici : ce qui est permis, ce qui se heurte et ce qui doit
// précéder quoi vit dans « teams ». Ce fichier ne fait que poser les questions
// et rendre compte.

// desk rassemble ce qu'il faut pour agir sur les équipes d'un groupe.
type desk struct {
	session *Session
	cours   classroom.Classroom
	// list ne retient que les équipes du groupe ; infos, toutes celles de
	// l'organisation — l'adoption a besoin de voir les autres.
	list  []teams.Team
	infos []teams.Info
}

// teamDesk résout le groupe désigné par une place et lit ses équipes.
func (s *Session) teamDesk(place string) (*desk, error) {
	place = strings.TrimSpace(place)
	if place == "" {
		return nil, valid.Errorf(
			"Aucun groupe indiqué : passez « --manage session%[1]scours%[1]sgroupe ».",
			naming.Separator)
	}
	if len(strings.Split(place, naming.Separator)) != naming.Levels-2 {
		return nil, valid.Errorf(
			"« %s » ne désigne pas un groupe. Les équipes appartiennent à un groupe, "+
				"nommé « session%[2]scours%[2]sgroupe » — « a26.5n6.01 » par exemple.",
			place, naming.Separator)
	}
	cours, err := classroom.AtScope(s.Settings.Org, place,
		classroom.DefaultsFrom(s.Settings))
	if err != nil {
		return nil, err
	}
	bureau := &desk{session: s, cours: cours}
	return bureau, bureau.reload()
}

// reload relit les équipes de l'organisation. Elles ne sont pas mises en cache
// au terminal : une exécution est courte, et une composition périmée
// tromperait plus qu'elle ne ferait gagner.
func (d *desk) reload() error {
	var infos []teams.Info
	var err error
	ui.Await(d.session.Console, "Lecture des équipes de "+d.cours.Org+"…", func() {
		infos, err = d.session.Client.LoadOrgTeams(d.cours.Org, d.session.Options.Jobs, nil)
	})
	if err != nil {
		return err
	}
	d.list = d.cours.Teams(infos)
	d.infos = infos
	return nil
}

// show dresse la liste des équipes du groupe et de ceux qui n'en ont aucune.
func (d *desk) show() {
	console := d.session.Console
	console.Heading("Équipes de « " + d.cours.Label() + " »")
	if len(d.list) == 0 {
		console.Note("Aucune équipe. Créez-en une avec « --team NOM --team-members COMPTES ».")
		return
	}
	lignes := make([][]string, 0, len(d.list))
	for _, fiche := range d.cours.Describe(d.list) {
		membres := make([]string, 0, len(fiche.Members))
		for _, personne := range fiche.People {
			membres = append(membres, "@"+personne.Username)
		}
		for _, etranger := range fiche.Strangers {
			membres = append(membres, "@"+etranger+" (hors liste)")
		}
		if len(membres) == 0 {
			membres = append(membres, "—")
		}
		lignes = append(lignes, []string{fiche.Short, fiche.Name, strings.Join(membres, ", ")})
	}
	console.Table([]string{"Équipe", "Nom sur GitHub", "Membres"}, lignes, 50)

	if orphelins := d.cours.Unassigned(d.list); len(orphelins) > 0 {
		noms := make([]string, 0, len(orphelins))
		for _, personne := range orphelins {
			noms = append(noms, "@"+personne.Username)
		}
		console.Note("Sans équipe : %s", strings.Join(noms, ", "))
	}
}

// only rend l'unique équipe visée, et refuse une demande ambiguë.
func (d *desk) only() (string, error) {
	demandees := d.session.Options.Team
	if len(demandees) != 1 {
		return "", valid.Errorf(
			"Une seule équipe à la fois : passez « --team NOM ».")
	}
	return teams.ShortName(demandees[0])
}

// create crée une équipe du groupe et lui donne sa composition.
func (d *desk) create(short string, membres []string) error {
	if err := teams.Available(d.list, short); err != nil {
		return err
	}
	comptes, err := d.members(membres)
	if err != nil {
		return err
	}
	console := d.session.Console
	var err2 error
	ui.Await(console, "Création de l'équipe "+d.cours.TeamName(short)+"…", func() {
		_, err2 = d.session.Client.CreateTeam(d.cours.Org, d.cours.TeamName(short),
			teams.Describe(d.cours.Session, d.cours.Course, d.cours.Group, short),
			teams.Privacy)
	})
	if err2 != nil {
		return err2
	}
	console.Success("Équipe « %s » créée.", short)
	if err := d.reload(); err != nil {
		return err
	}
	if len(comptes) == 0 {
		return nil
	}
	return d.apply(teams.PlanAssign(d.list, short, comptes))
}

// compose donne à une équipe la composition exacte demandée, en la créant si
// elle n'existe pas encore : décrire une équipe suffit à la faire exister.
func (d *desk) compose(short string, membres []string) error {
	if _, existe := teams.Find(d.list, short); !existe {
		return d.create(short, membres)
	}
	comptes, err := d.members(membres)
	if err != nil {
		return err
	}
	return d.apply(teams.PlanCompose(d.list, short, comptes))
}

// assign inscrit des personnes dans une équipe ; elles quittent la leur.
func (d *desk) assign(short string, membres []string) error {
	comptes, err := d.members(membres)
	if err != nil {
		return err
	}
	return d.apply(teams.PlanAssign(d.list, short, comptes))
}

// remove retire des personnes de leur équipe.
func (d *desk) remove(short string, membres []string) error {
	comptes, err := d.members(membres)
	if err != nil {
		return err
	}
	return d.apply(teams.PlanRemove(d.list, short, comptes))
}

// rename renomme une équipe du groupe.
func (d *desk) rename(short, cible string) error {
	equipe, nom, err := teams.PlanRename(d.list, short, cible)
	if err != nil {
		return err
	}
	parts, _ := teams.Read(teams.Info{Name: nom})
	ui.Await(d.session.Console, "Renommage de « "+equipe.Name+" »…", func() {
		_, err = d.session.Client.UpdateTeam(d.cours.Org, equipe.Slug, nom,
			teams.Describe(d.cours.Session, d.cours.Course, d.cours.Group, parts.Short))
	})
	if err != nil {
		return err
	}
	d.session.Console.Success("« %s » devient « %s ».", equipe.Short, parts.Short)
	return d.reload()
}

// adopt fait entrer dans le groupe une équipe déjà présente dans
// l'organisation. Elle garde ses membres, ses accès et son histoire.
func (d *desk) adopt(slug, short string) error {
	source, nom, err := teams.PlanAdopt(d.list, d.infos, slug, short,
		d.cours.Session, d.cours.Course, d.cours.Group)
	if err != nil {
		return err
	}
	parts, _ := teams.Read(teams.Info{Name: nom})
	ui.Await(d.session.Console, "Adoption de « "+source.Name+" »…", func() {
		_, err = d.session.Client.UpdateTeam(d.cours.Org, source.Slug, nom,
			teams.Describe(d.cours.Session, d.cours.Course, d.cours.Group, parts.Short))
	})
	if err != nil {
		return err
	}
	d.session.Console.Success("« %s » rejoint « %s » sous le nom « %s ».",
		source.Name, d.cours.Label(), parts.Short)
	return d.reload()
}

// drop supprime une équipe. Ses dépôts restent : seul l'accès disparaît.
func (d *desk) drop(short string) error {
	equipe, trouvee := teams.Find(d.list, short)
	if !trouvee {
		return valid.Errorf("Aucune équipe « %s » dans ce groupe.", short)
	}
	if !d.session.Options.Yes {
		confirme, err := d.session.Prompt.Confirm(
			"Supprimer « "+equipe.Name+" » ? Ses dépôts resteront sur GitHub.", false)
		if err != nil {
			return err
		}
		if !confirme {
			d.session.Console.Warning("Annulé : l'équipe est intacte.")
			return nil
		}
	}
	var err error
	ui.Await(d.session.Console, "Suppression de « "+equipe.Name+" »…", func() {
		err = d.session.Client.DeleteTeam(d.cours.Org, equipe.Slug)
	})
	if err != nil {
		return err
	}
	d.session.Console.Success(
		"« %s » supprimée. Ses dépôts restent ; seul l'accès qu'elle donnait a disparu.",
		equipe.Name)
	return d.reload()
}

// apply exécute un plan de composition et rend compte étape par étape.
func (d *desk) apply(etapes []teams.Step, err error) error {
	if err != nil {
		return err
	}
	console := d.session.Console
	for _, etape := range etapes {
		if etape.Kind == teams.Join {
			err = d.session.Client.AddTeamMember(d.cours.Org, etape.Slug,
				etape.Username, teams.MemberRole)
		} else {
			err = d.session.Client.RemoveTeamMember(d.cours.Org, etape.Slug, etape.Username)
		}
		if err != nil {
			console.Failure("@%s : %s impossible — %v", etape.Username, etape.Kind, err)
			continue
		}
		console.Printf("  %s @%s %s « %s »", console.OK("✓"), etape.Username,
			verbe(etape.Kind), etape.Team)
	}
	return d.reload()
}

func verbe(kind string) string {
	if kind == teams.Join {
		return "rejoint"
	}
	return "quitte"
}

// members vérifie que les comptes donnés sont des étudiants du groupe. Un
// groupe dont la liste n'est pas retenue localement n'en connaît aucun : les
// comptes sont alors pris tels quels, car rien ne permet de les démentir.
func (d *desk) members(usernames []string) ([]string, error) {
	propres := make([]string, 0, len(usernames))
	for _, brut := range usernames {
		username, err := valid.Login(brut, "Compte GitHub")
		if err != nil {
			return nil, err
		}
		if len(d.cours.Students) > 0 {
			personne, inscrit := d.cours.Find(username)
			if !inscrit {
				return nil, valid.Errorf(
					"@%s n'est pas dans « %s » : inscrivez-le au groupe d'abord.",
					username, d.cours.Label())
			}
			username = personne.Username
		}
		propres = append(propres, username)
	}
	return propres, nil
}

// ---------------------------------------------------------------- inventaire

// orgRepos charge les dépôts de l'organisation : cache, puis API. C'est le même
// chemin que celui du mode gestion, dit une fois de plus parce qu'une session
// d'équipe n'ouvre pas de groupe de dépôts.
func (s *Session) orgRepos() ([]groups.RepoInfo, error) {
	key := cache.ReposKey(s.Settings.Org)
	var cached []groups.RepoInfo
	if s.Cache.Get(key, cache.ReposTTL, &cached) && len(cached) > 0 {
		return cached, nil
	}
	spin := ui.NewSpinner(s.Console, "Chargement des dépôts de "+s.Settings.Org+"…")
	spin.Start()
	repos, err := s.Client.ListOrgRepos(s.Settings.Org, func(total int) {
		spin.Detail(itoa(total) + " lus")
	})
	spin.Stop()
	if err != nil {
		return nil, err
	}
	s.Cache.Set(key, repos)
	return repos, nil
}

// ------------------------------------------------------------ ligne de commande

// wantsTeams dit si les drapeaux demandent une opération sur les équipes.
func (o *Options) wantsTeams() bool {
	return o.Teams || len(o.Team) > 0 || o.TeamMembersOn || len(o.TeamAdd) > 0 ||
		len(o.TeamRemove) > 0 || o.TeamRename != "" || o.TeamAdopt != "" || o.TeamDelete
}

// teamsMode tient les équipes d'un groupe : ce que les drapeaux demandent, ou,
// à défaut, ce que le menu propose.
func (s *Session) teamsMode() (int, error) {
	place := strings.TrimSpace(s.Options.Manage)
	if place == "" {
		chosen, err := s.askPlace()
		if err != nil {
			return ExitOK, err
		}
		place = chosen
	}
	return s.manageTeams(place)
}

// askPlace fait choisir le groupe dont on veut tenir les équipes, parmi ceux
// que les dépôts dessinent. Un nom peut toujours être saisi à la place : un
// groupe sans aucun dépôt n'apparaît nulle part, et c'est souvent celui qu'on
// prépare.
func (s *Session) askPlace() (string, error) {
	if !s.Interactive() {
		return s.require("", "--manage a26.5n6.01", "Groupe")
	}
	repos, err := s.orgRepos()
	if err != nil {
		return "", err
	}
	places := classroom.Places(repos)
	if len(places) == 0 {
		return s.Prompt.Ask(ui.Question{
			Title: "Place du groupe (« a26.5n6.01 ») :",
		})
	}
	const libre = "\x00libre"
	choix := make([]string, 0, 2*len(places)+2)
	for _, place := range places {
		choix = append(choix, place, place)
	}
	choix = append(choix, libre, "Saisir une autre place…")
	answer, err := s.Prompt.Choose("Quel groupe ?", ui.Options(choix...), places[0])
	if err != nil {
		return "", err
	}
	if answer != libre {
		return answer, nil
	}
	return s.Prompt.Ask(ui.Question{Title: "Place du groupe (« a26.5n6.01 ») :"})
}

// manageTeams applique les drapeaux d'équipe à un groupe, puis rend la main :
// une commande scriptée fait ce qu'on lui demande et s'en va.
func (s *Session) manageTeams(place string) (int, error) {
	bureau, err := s.teamDesk(place)
	if err != nil {
		return ExitOK, err
	}
	options := s.Options

	// Adopter et créer nomment l'équipe ; les autres la retrouvent.
	switch {
	case options.TeamAdopt != "":
		short, err := bureau.only()
		if err != nil {
			return ExitOK, err
		}
		if err := bureau.adopt(options.TeamAdopt, short); err != nil {
			return ExitOK, err
		}
	case options.TeamDelete:
		short, err := bureau.only()
		if err != nil {
			return ExitOK, err
		}
		if err := bureau.drop(short); err != nil {
			return ExitOK, err
		}
	case options.TeamRename != "":
		short, err := bureau.only()
		if err != nil {
			return ExitOK, err
		}
		if err := bureau.rename(short, options.TeamRename); err != nil {
			return ExitOK, err
		}
	case options.TeamMembersOn:
		short, err := bureau.only()
		if err != nil {
			return ExitOK, err
		}
		if err := bureau.compose(short, options.TeamMembers); err != nil {
			return ExitOK, err
		}
	case len(options.TeamAdd) > 0 || len(options.TeamRemove) > 0:
		short, err := bureau.only()
		if err != nil {
			return ExitOK, err
		}
		if len(options.TeamAdd) > 0 {
			if err := bureau.assign(short, options.TeamAdd); err != nil {
				return ExitOK, err
			}
		}
		if len(options.TeamRemove) > 0 {
			if err := bureau.remove(short, options.TeamRemove); err != nil {
				return ExitOK, err
			}
		}
	case !options.Teams && s.Interactive():
		// « --team eq1 » seul ne dit pas quoi en faire : le menu le demande.
		return s.teamMenu(bureau)
	}

	bureau.show()
	return ExitOK, nil
}

// ------------------------------------------------------------------- menu

var teamMenuOptions = ui.Options(
	"composer", "Composer une équipe (la crée au besoin)",
	"deplacer", "Déplacer un étudiant vers une équipe",
	"retirer", "Retirer un étudiant de son équipe",
	"renommer", "Renommer une équipe",
	"adopter", "Adopter une équipe existante de l'organisation",
	"supprimer", "Supprimer une équipe",
	"recharger", "Recharger la liste",
	"quitter", "Revenir",
)

// teamMenu tient les équipes d'un groupe au terminal, à la main.
func (s *Session) teamMenu(bureau *desk) (int, error) {
	for {
		bureau.show()
		action, err := s.Prompt.Choose("Que faire des équipes ?", teamMenuOptions, "quitter")
		if err != nil {
			return ExitOK, err
		}
		if action == "quitter" {
			return ExitOK, nil
		}
		if err := s.dispatchTeam(bureau, action); err != nil {
			if valid.IsValidation(err) || ghapi.IsGitHub(err) {
				s.Console.Failure("%v", err)
				continue
			}
			return ExitOK, err
		}
	}
}

func (s *Session) dispatchTeam(bureau *desk, action string) error {
	switch action {
	case "recharger":
		return bureau.reload()
	case "composer":
		nom, err := s.askTeamName("Nom de l'équipe (« eq1 ») :", "")
		if err != nil {
			return err
		}
		membres, err := s.askLogins("Comptes GitHub de l'équipe, séparés par des virgules :",
			bureau.membersOf(nom))
		if err != nil {
			return err
		}
		return bureau.compose(nom, membres)
	case "deplacer":
		nom, err := s.pickTeam(bureau, "Vers quelle équipe ?")
		if err != nil {
			return err
		}
		membres, err := s.askLogins("Comptes GitHub à y inscrire :", "")
		if err != nil {
			return err
		}
		return bureau.assign(nom, membres)
	case "retirer":
		nom, err := s.pickTeam(bureau, "De quelle équipe ?")
		if err != nil {
			return err
		}
		membres, err := s.askLogins("Comptes GitHub à en retirer :", bureau.membersOf(nom))
		if err != nil {
			return err
		}
		return bureau.remove(nom, membres)
	case "renommer":
		nom, err := s.pickTeam(bureau, "Quelle équipe renommer ?")
		if err != nil {
			return err
		}
		cible, err := s.askTeamName("Nouveau nom :", "")
		if err != nil {
			return err
		}
		return bureau.rename(nom, cible)
	case "adopter":
		libres := teams.Loose(bureau.infos)
		if len(libres) == 0 {
			return valid.Errorf("Aucune équipe de l'organisation n'est libre d'un groupe.")
		}
		choix := make([]string, 0, 2*len(libres))
		for _, info := range libres {
			choix = append(choix, info.Slug, info.Name+" ("+info.Slug+")")
		}
		slug, err := s.Prompt.Choose("Quelle équipe adopter ?", ui.Options(choix...), libres[0].Slug)
		if err != nil {
			return err
		}
		nom, err := s.askTeamName("Sous quel nom dans « "+bureau.cours.Label()+" » ?", "")
		if err != nil {
			return err
		}
		return bureau.adopt(slug, nom)
	case "supprimer":
		nom, err := s.pickTeam(bureau, "Quelle équipe supprimer ?")
		if err != nil {
			return err
		}
		return bureau.drop(nom)
	}
	return nil
}

// membersOf rend la composition d'une équipe, prête à être proposée en réponse.
func (d *desk) membersOf(short string) string {
	equipe, trouvee := teams.Find(d.list, short)
	if !trouvee {
		return ""
	}
	membres := append([]string(nil), equipe.Members...)
	sort.Strings(membres)
	return strings.Join(membres, ", ")
}

// pickTeam fait choisir une équipe du groupe.
func (s *Session) pickTeam(bureau *desk, question string) (string, error) {
	if len(bureau.list) == 0 {
		return "", valid.Errorf("Ce groupe n'a aucune équipe.")
	}
	choix := make([]string, 0, 2*len(bureau.list))
	for _, equipe := range bureau.list {
		choix = append(choix, equipe.Short, equipe.Short+" — "+bureau.membersOf(equipe.Short))
	}
	return s.Prompt.Choose(question, ui.Options(choix...), bureau.list[0].Short)
}

func (s *Session) askTeamName(title, defaultValue string) (string, error) {
	answer, err := s.Prompt.Ask(ui.Question{Title: title, Default: defaultValue})
	if err != nil {
		return "", err
	}
	return teams.ShortName(answer)
}

func (s *Session) askLogins(title, defaultValue string) ([]string, error) {
	answer, err := s.Prompt.Ask(ui.Question{
		Title: title, Default: defaultValue, AllowEmpty: true,
	})
	if err != nil {
		return nil, err
	}
	return splitList(answer), nil
}

// ------------------------------------------------------ distribution d'équipe

// createTeams distribue un travail aux équipes du groupe : un dépôt par équipe,
// nommé d'après elle et partagé avec elle. Aucune liste d'étudiants n'est
// nécessaire — les équipes sont sur GitHub, et elles disent qui est servi.
func (s *Session) createTeams() (int, error) {
	place, travail, err := splitAssignment(s.Options.Assignment)
	if err != nil {
		return ExitValidation, err
	}
	bureau, err := s.teamDesk(place)
	if err != nil {
		return ExitOK, err
	}
	if len(bureau.list) == 0 {
		return ExitValidation, valid.Errorf(
			"« %s » n'a aucune équipe : créez-en avant de distribuer un travail d'équipe.",
			bureau.cours.Label())
	}
	if s.Options.TeamShare {
		return s.shareTeamRepos(bureau, travail)
	}

	retenues, err := bureau.wanted(s.Options.Team)
	if err != nil {
		return ExitValidation, err
	}
	if err := s.configure(); err != nil {
		return ExitOK, err
	}
	// « configure » slugifie l'identifiant reçu, point compris : la
	// nomenclature le rétablit. Et le gabarit de nom n'est pas négociable pour
	// un travail d'équipe — c'est l'équipe qui occupe le dernier niveau, là où
	// un travail individuel met l'étudiant.
	s.Settings.Assignment = bureau.cours.AssignmentID(travail)
	s.Settings.NamePattern = classroom.NamePattern

	items, err := plan.BuildTeams(classroom.TeamTargets(retenues), s.Settings)
	if err != nil {
		return ExitOK, err
	}
	s.summarizeTeams(items, retenues)

	if !s.Options.Yes && !s.Options.DryRun {
		confirmed, err := s.Prompt.Confirm(
			plural("Confirmer et créer %d dépôt(s) d'équipe dans « "+s.Settings.Org+" » ?",
				len(items)), false)
		if err != nil {
			return ExitOK, err
		}
		if !confirmed {
			s.Console.Warning("Annulé : rien n'a été créé.")
			return ExitAborted, nil
		}
	}
	return s.execute(items)
}

// shareTeamRepos redonne à chaque équipe l'accès au dépôt qui porte son nom.
// C'est ce qui achève l'adoption d'un travail fait en équipe avant l'outil :
// les dépôts sont déjà là et bien nommés, mais rien ne les a jamais partagés.
func (s *Session) shareTeamRepos(bureau *desk, travail string) (int, error) {
	repos, err := s.orgRepos()
	if err != nil {
		return ExitOK, err
	}
	id := bureau.cours.AssignmentID(travail)
	droit := bureau.cours.Settings(travail).Permission

	s.Console.Heading("Partage de « " + travail + " » avec les équipes")
	faits, echecs := 0, 0
	for _, depot := range bureau.cours.Repos(id, repos) {
		equipe, appartient := bureau.cours.TeamOf(depot.Name, bureau.list)
		if !appartient {
			continue
		}
		if s.Options.DryRun {
			s.Console.Printf("  %s %s → équipe %s (%s)", s.Console.Dim("·"),
				depot.Name, equipe.Short, droit)
			faits++
			continue
		}
		if err := s.Client.AddTeamRepo(bureau.cours.Org, equipe.Slug,
			bureau.cours.Org, depot.Name, droit); err != nil {
			echecs++
			s.Console.Failure("%s : %v", depot.Name, err)
			continue
		}
		faits++
		s.Console.Printf("  %s %s → équipe %s (%s)", s.Console.OK("✓"),
			depot.Name, equipe.Short, droit)
	}
	if faits == 0 && echecs == 0 {
		return ExitValidation, valid.Errorf(
			"Aucun dépôt de « %s » ne porte le nom d'une équipe de « %s ».",
			travail, bureau.cours.Label())
	}
	if echecs > 0 {
		return ExitFailure, nil
	}
	s.Console.Success("%d dépôt(s) partagé(s) avec leur équipe.", faits)
	return ExitOK, nil
}

// wanted retient les équipes demandées ; sans demande, toutes celles du groupe.
func (d *desk) wanted(noms []string) ([]teams.Team, error) {
	if len(noms) == 0 {
		return d.list, nil
	}
	retenues := make([]teams.Team, 0, len(noms))
	for _, brut := range noms {
		short, err := teams.ShortName(brut)
		if err != nil {
			return nil, err
		}
		equipe, trouvee := teams.Find(d.list, short)
		if !trouvee {
			return nil, valid.Errorf("Aucune équipe « %s » dans « %s ».",
				short, d.cours.Label())
		}
		retenues = append(retenues, equipe)
	}
	return retenues, nil
}

// summarizeTeams récapitule une distribution d'équipe avant toute écriture.
func (s *Session) summarizeTeams(items []plan.PlannedRepo, retenues []teams.Team) {
	s.summarizeCommon(items, "Équipes à servir")
	membres := map[string][]string{}
	for _, equipe := range retenues {
		comptes := make([]string, 0, len(equipe.Members))
		for _, membre := range equipe.Members {
			comptes = append(comptes, "@"+membre)
		}
		membres[equipe.Short] = comptes
	}
	preview := make([][]string, 0, len(items))
	for _, item := range items {
		preview = append(preview, []string{
			item.Name, item.Recipient(), strings.Join(membres[item.Team], ", "),
		})
	}
	s.Console.Table([]string{"Dépôt", "Équipe", "Membres"}, preview, 20)
}

// splitAssignment sépare la place du groupe et le nom du travail dans un
// identifiant complet : « a26.5n6.01.tp1 » donne « a26.5n6.01 » et « tp1 ».
func splitAssignment(id string) (string, string, error) {
	niveaux := strings.Split(strings.TrimSpace(id), naming.Separator)
	if len(niveaux) != naming.TeamLevels {
		return "", "", valid.Errorf(
			"Travail d'équipe : « %s » doit nommer un groupe et un travail — "+
				"« session%[2]scours%[2]sgroupe%[2]stravail », « a26.5n6.01.tp1 » par exemple.",
			strings.TrimSpace(id), naming.Separator)
	}
	return strings.Join(niveaux[:3], naming.Separator), niveaux[3], nil
}
