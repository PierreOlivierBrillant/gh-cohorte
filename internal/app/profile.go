package app

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/users"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// La fiche d'un utilisateur au terminal, c'est la même chose qu'au navigateur :
// le rôle, les comptes, et la chronologie du passage — cours suivis et cours
// donnés mêlés, du plus récent au plus ancien. Rien n'en est décidé ici :
// « users » la dresse, et le terminal l'écrit.

// showProfile écrit la fiche d'un compte, puis propose de le coopter.
func (s *Session) showProfile(account string) (int, error) {
	compte, err := valid.Login(account, "Compte GitHub")
	if err != nil {
		return ExitValidation, err
	}
	org := s.Settings.Org
	// Nommer avant de montrer : la fiche dira alors le nom qu'on vient de
	// donner, plutôt que d'afficher « nom inconnu » juste au-dessus.
	if voulu := strings.TrimSpace(s.Options.FullName); voulu != "" {
		if code, err := s.nameUser(org, compte, voulu); err != nil {
			return code, err
		}
	}
	fiche, set, err := s.profileOf(org, compte)
	if err != nil {
		return ExitFailure, err
	}
	s.printProfile(org, fiche)

	// Un drapeau tranche sans rien demander ; sans lui, la question se pose à
	// qui peut y répondre.
	if s.Options.TeacherSet {
		return s.setRole(org, set, fiche, s.Options.Teacher)
	}
	if !s.Interactive() {
		return ExitOK, nil
	}
	// Un nom manquant se règle d'abord : sans lui, aucun travail ne peut être
	// distribué à cette personne, et c'est plus urgent qu'un rôle.
	if fiche.FullName == "" {
		if code, err := s.askName(org, fiche); err != nil || code != ExitOK {
			return code, err
		}
		if fiche, set, err = s.profileOf(org, compte); err != nil {
			return ExitFailure, err
		}
	}
	return s.askRole(org, set, fiche)
}

// askName propose de nommer quelqu'un qui n'a pas de nom complet.
func (s *Session) askName(org string, fiche users.Profile) (int, error) {
	s.Console.Warning("@%s n'a pas de nom complet : c'est lui qui nomme ses "+
		"dépôts, et sans lui aucun travail ne peut lui être distribué.", fiche.Username)
	voulu, err := s.Prompt.Ask(ui.Question{
		Title:      "Nom complet (vide pour laisser ainsi)",
		AllowEmpty: true,
	})
	if err != nil || strings.TrimSpace(voulu) == "" {
		return ExitOK, err
	}
	return s.nameUser(org, fiche.Username, voulu)
}

// nameUser écrit le nom complet d'un compte au registre.
//
// Le nom vit au registre, pas dans un groupe : c'est une propriété de la
// personne, et vaut donc pour tous ses cours. Aucun dépôt n'est renommé — le
// slug que le nouveau nom produit s'ajoute à ceux que la personne portait, et
// les dépôts créés sous l'ancien restent les siens.
func (s *Session) nameUser(org, account, wanted string) (int, error) {
	nom, err := valid.FullName(wanted)
	if err != nil {
		return ExitValidation, err
	}
	set, _ := s.names(org)
	// Un compte que rien ne connaît — ni le registre, ni aucune liste de ce
	// poste — est vérifié sur GitHub avant d'entrer au registre : une faute de
	// frappe y laisserait sinon une fiche que rien ne désigne.
	if _, connu := set.Find(account); !connu && !s.declared(org, account) {
		profil, err := s.Client.GetUser(account)
		if err != nil {
			return ExitFailure, err
		}
		if profil == nil {
			return ExitValidation, valid.Errorf(
				"Le compte @%s n'existe pas sur %s.", account, s.host())
		}
	}

	if _, err := s.registryOf(org).Apply(registry.Change{
		Learn:  []registry.User{registry.From(roster.Person{FullName: nom, Username: account})},
		Reason: "Nomme @" + account + " : " + nom,
	}); err != nil {
		return ExitFailure, err
	}
	s.Console.Success("@%s s'appelle « %s ».", account, nom)
	return ExitOK, nil
}

// declared dit qu'une liste de groupe de ce poste connaît déjà ce compte.
func (s *Session) declared(org, account string) bool {
	for _, personne := range s.groupStore().People(org) {
		if personne.Owns(account) {
			return true
		}
	}
	return false
}

// profileOf dresse la fiche d'un compte, avec le registre qui l'a nommée.
func (s *Session) profileOf(org, account string) (users.Profile, *registry.Set, error) {
	repos, err := s.orgRepos(org, false)
	if err != nil {
		return users.Profile{}, nil, err
	}
	store := classroom.Open(classroom.PathNextTo(s.ConfigFile))
	set, avis := s.names(org)
	if avis != "" {
		s.Console.Print(s.Console.Warn(avis))
	}
	visibles := store.Visible(org, repos, classroom.DefaultsFrom(s.Settings), set)

	infos, _ := s.Client.LoadOrgTeams(org, s.Options.Jobs)
	equipes := make([]teams.Team, 0, len(infos))
	for _, cours := range visibles {
		equipes = append(equipes, cours.Teams(infos)...)
	}
	return users.ProfileOf(visibles, repos, equipes, infos, set, account), set, nil
}

// printProfile écrit la fiche : l'identité, puis la chronologie.
func (s *Session) printProfile(org string, fiche users.Profile) {
	console := s.Console
	nom := fiche.FullName
	if nom == "" {
		nom = console.Dim("nom inconnu")
	}
	console.Heading(nom + " — @" + fiche.Username)

	console.Printf("  %s %s", console.Dim(pad("Rôle", 18)), fiche.Role)
	console.Printf("  %s %s", console.Dim(pad("Comptes", 18)),
		"@"+strings.Join(fiche.Accounts, "  @"))
	if fiche.StudentID != "" {
		console.Printf("  %s %s", console.Dim(pad("Matricule", 18)), fiche.StudentID)
	}
	envoi := fiche.PushedAt
	if envoi == "" {
		envoi = console.Dim("jamais")
	}
	console.Printf("  %s %s", console.Dim(pad("Dernier envoi", 18)), envoi)
	if !fiche.Known {
		console.Note("Le registre de « %s » ne connaît pas @%s : le compte existe sur "+
			"GitHub, il n'a simplement rien fait ici.", org, fiche.Username)
	}

	if len(fiche.Timeline) == 0 {
		console.Warning("Aucun cours suivi ni donné dans « %s ».", org)
		return
	}
	console.Heading("Son passage dans l'organisation")
	rows := make([][]string, 0, len(fiche.Timeline))
	for _, etape := range fiche.Timeline {
		session := etape.SessionName
		if session == "" {
			session = etape.Session
		}
		rows = append(rows, []string{
			session, strings.ToUpper(etape.Course), etape.Group,
			etape.Role, etape.Scope, stepDetail(console, etape),
		})
	}
	console.Table(
		[]string{"Session", "Cours", "Groupe", "Rôle", "Place", "Ce qu'il en reste"},
		rows, 46)
}

// stepDetail dit ce qu'une étape a laissé. « Muet » n'est pas « aucun dépôt » :
// le dépôt existe, il n'a simplement jamais rien reçu.
func stepDetail(console *ui.Console, etape users.Step) string {
	if etape.Teaching() {
		return console.Dim("a donné ce cours")
	}
	if len(etape.Assignments) == 0 {
		return console.Dim("aucun dépôt")
	}
	noms := make([]string, 0, len(etape.Assignments))
	for _, travail := range etape.Assignments {
		noms = append(noms, travail.Name)
	}
	suite := strings.Join(noms, ", ")
	if etape.Silent {
		return suite + console.Dim(" — aucun envoi")
	}
	return suite + console.Dim(" — "+etape.PushedAt)
}

// askRole propose de reconnaître quelqu'un comme enseignant, ou de l'en
// défaire. Elle n'est offerte qu'à un enseignant — sauf tant que
// l'organisation n'en a aucun : le premier doit bien pouvoir se déclarer.
func (s *Session) askRole(org string, set *registry.Set, fiche users.Profile) (int, error) {
	if !set.Teaches(s.Viewer) && len(set.Teachers()) > 0 {
		return ExitOK, nil
	}
	question := "Reconnaître @" + fiche.Username + " comme enseignant ?"
	if fiche.IsTeacher {
		question = "Retirer son rôle d'enseignant à @" + fiche.Username + " ?"
	}
	if len(set.Teachers()) == 0 {
		s.Console.Note("Aucun enseignant n'est encore reconnu dans « %s » : "+
			"le premier se déclare, les suivants sont cooptés.", org)
	}
	voulu, err := s.Prompt.Confirm(question, false)
	if err != nil || !voulu {
		return ExitOK, err
	}
	return s.setRole(org, set, fiche, !fiche.IsTeacher)
}

// setRole écrit le rôle au registre.
//
// Les refus sont ceux du navigateur, dits ici dans les mêmes mots : ce n'est
// pas eux qui protègent le registre — un étudiant n'a jamais eu le droit d'y
// écrire — mais ils évitent un geste qui échouerait plus loin, en HTTP.
func (s *Session) setRole(org string, set *registry.Set,
	fiche users.Profile, teacher bool) (int, error) {
	enseignants := set.Teachers()
	if !set.Teaches(s.Viewer) && len(enseignants) > 0 {
		return ExitValidation, valid.Errorf(
			"Seul un enseignant peut en reconnaître un autre. @%s n'est pas déclaré "+
				"enseignant dans « %s ».", s.Viewer, org)
	}
	if !teacher && len(enseignants) == 1 &&
		strings.EqualFold(enseignants[0].Username, fiche.Username) {
		return ExitValidation, valid.Errorf(
			"@%s est le seul enseignant de « %s » : lui retirer son rôle ne laisserait "+
				"personne pour en reconnaître un autre. Reconnaissez d'abord un collègue.",
			fiche.Username, org)
	}
	if fiche.IsTeacher == teacher {
		s.Console.Note("@%s est déjà %s.", fiche.Username, fiche.Role)
		return ExitOK, nil
	}

	change := registry.SetRole(fiche.Username, teacher)
	// Un enseignant ne figure sur aucune liste de classe : le registre ne le
	// connaît pas, et on ne donne pas un rôle à qui n'y est pas. Son compte est
	// vérifié au passage, faute de quoi une faute de frappe laisserait une
	// fiche que rien ne désigne.
	if !fiche.Known {
		profil, err := s.Client.GetUser(fiche.Username)
		if err != nil {
			return ExitFailure, err
		}
		if profil == nil {
			return ExitValidation, valid.Errorf(
				"Le compte @%s n'existe pas sur %s.", fiche.Username, s.host())
		}
		appris := registry.User{Username: fiche.Username}
		if nom := strings.TrimSpace(profil.Name); nom != "" {
			appris.FullName = nom
		}
		change.Learn = append(change.Learn, appris)
	}

	publie, err := s.registryOf(org).Apply(change)
	if err != nil {
		return ExitFailure, err
	}
	inscrite, _ := publie.Find(fiche.Username)
	s.Console.Success("@%s est désormais %s.", fiche.Username, inscrite.Role())
	if teacher {
		s.Console.Note("Cela ne lui ouvre aucun dépôt : inscrivez-le à l'équipe " +
			"enseignante d'un groupe pour lui en donner l'accès.")
	}
	return ExitOK, nil
}

// host rend l'hôte GitHub de la session, pour les messages.
func (s *Session) host() string {
	if host := strings.TrimSpace(s.Options.Host); host != "" {
		return host
	}
	return "github.com"
}
