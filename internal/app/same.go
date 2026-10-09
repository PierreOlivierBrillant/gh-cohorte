package app

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-milou/internal/brand"
	"github.com/PierreOlivierBrillant/gh-milou/internal/registry"
	"github.com/PierreOlivierBrillant/gh-milou/internal/ui"
	"github.com/PierreOlivierBrillant/gh-milou/internal/users"
)

// Une même personne travaille parfois sous deux comptes — celui d'une session
// et celui de la suivante. Les réunir se décide dans « users » : qui peut, ce
// qui est refusé, ce qui l'emporte. Le terminal recueille les deux comptes, dit
// ce que la réunion fera, et l'écrit au registre — par les drapeaux, depuis la
// fiche, ou depuis le menu de l'annuaire.

// sameFromFlags réunit ou sépare le compte de « --user » comme les drapeaux le
// demandent. « fini » dit qu'il n'y a rien à montrer de plus : un aperçu
// demandé par « --dry-run » s'arrête là.
func (s *Session) sameFromFlags(org, account string) (fini bool, code int, err error) {
	vue, err := s.orgView(org, false)
	if err != nil {
		return true, ExitFailure, err
	}
	if s.Options.Separate {
		separation, err := users.PlanSplit(vue.rows(), vue.set, s.Viewer, org, account)
		if err != nil {
			return true, ExitValidation, err
		}
		if s.Options.DryRun {
			s.printSplitting(separation)
			s.Console.Note("Rien n'a été écrit (--dry-run).")
			return true, ExitOK, nil
		}
		return false, s.applySplit(org, separation), nil
	}
	// Un compte vu dans l'historique d'un dépôt n'est pas une faute de frappe :
	// la réunion l'accepte même s'il n'est d'aucun groupe ni du registre.
	lignes := users.Witness(vue.rows(), s.histoires(org, vue.repos), account, s.Options.SameAs)
	reunion, err := users.PlanJoin(lignes, vue.set, s.Viewer, org, account, s.Options.SameAs)
	if err != nil {
		return true, ExitValidation, err
	}
	s.printJoining(reunion)
	if s.Options.DryRun {
		s.Console.Note("Rien n'a été écrit (--dry-run).")
		return true, ExitOK, nil
	}
	return false, s.applyJoin(org, reunion), nil
}

// askSame propose, depuis la fiche, de réunir la personne à un autre compte ou
// d'en séparer un. Elle n'est offerte qu'à qui peut le décider.
func (s *Session) askSame(org string, fiche users.Profile) (int, error) {
	vue, err := s.orgView(org, false)
	if err != nil {
		return ExitFailure, err
	}
	if !users.MayDecide(vue.set, s.Viewer) {
		return ExitOK, nil
	}
	options := ui.Options(
		"rien", "Rien",
		"reunir", "Réunir @"+fiche.Username+" à un autre compte de la même personne")
	if len(fiche.Joined) > 1 {
		options = append(options, ui.Option{
			Value: "separer", Label: "Séparer l'un de ses comptes, réunis à tort"})
	}
	choix, err := s.Prompt.Choose("Ses comptes", options, "rien")
	if err != nil {
		return ExitOK, err
	}
	switch choix {
	case "reunir":
		_, err = s.chooseAndJoin(org, vue.rows(), vue.set, fiche.Username)
	case "separer":
		_, err = s.chooseAndSplit(org, vue.rows(), vue.set, fiche.Joined)
	}
	return ExitOK, err
}

// same mène le geste depuis le menu de l'annuaire : choisir la personne, puis
// le reste. Il dit si le registre a bougé, pour que l'annuaire se redresse.
func (d *directorySession) same(reunir bool) (bool, error) {
	console := d.session.Console
	if !users.MayDecide(d.set, d.session.Viewer) {
		console.Failure("Seul un enseignant peut réunir ou séparer deux comptes. @%s "+
			"n'est pas déclaré enseignant dans « %s ».", d.session.Viewer, d.org)
		return false, nil
	}
	visibles := users.Apply(d.rows, d.filter, d.sortKey, d.sortDesc)
	if !reunir {
		// Seul ce que le registre réunit se sépare ici : deux comptes qu'une
		// liste de groupe déclare se corrigent depuis ce groupe.
		reunies := make([]users.Row, 0)
		for _, ligne := range visibles {
			if len(d.set.Accounts(ligne.Username)) > 1 {
				reunies = append(reunies, ligne)
			}
		}
		visibles = reunies
	}
	if len(visibles) == 0 {
		if reunir {
			console.Warning("Personne à réunir.")
		} else {
			console.Warning("Aucun compte n'est réuni à un autre au registre de « %s ».", d.org)
		}
		return false, nil
	}
	options := make([]ui.Option, 0, len(visibles))
	for _, ligne := range visibles {
		options = append(options, ui.Option{Value: ligne.Username, Label: intitule(ligne.FullName,
			ligne.Accounts)})
	}
	compte, err := d.session.Prompt.Choose("Qui ?", options, options[0].Value)
	if err != nil {
		return false, err
	}
	if reunir {
		return d.session.chooseAndJoin(d.org, d.rows, d.set, compte)
	}
	return d.session.chooseAndSplit(d.org, d.rows, d.set, d.set.Accounts(compte))
}

// chooseAndJoin demande l'autre compte et lequel des deux désigne la personne,
// dit ce que la réunion fera, et l'écrit une fois confirmée.
func (s *Session) chooseAndJoin(org string, rows []users.Row, set *registry.Set,
	account string) (bool, error) {
	console := s.Console
	candidats := users.Candidates(rows, account)
	if len(candidats) == 0 {
		console.Warning("Personne d'autre n'est connu dans « %s ».", org)
		return false, nil
	}
	// Les homonymes passent d'abord, et le disent : une suggestion, jamais une
	// décision — deux personnes peuvent s'appeler pareil.
	options := make([]ui.Option, 0, len(candidats))
	for _, candidat := range candidats {
		libelle := intitule(candidat.FullName, candidat.Accounts)
		if candidat.SameName {
			libelle += "  — même nom"
		}
		options = append(options, ui.Option{Value: candidat.Username, Label: libelle})
	}
	console.Note("Rien ne se déduit d'un nom : deux personnes peuvent s'appeler pareil, " +
		"et les réunir leur donnerait un seul dépôt pour deux.")
	autre, err := s.Prompt.Choose("Autre compte de @"+account, options, options[0].Value)
	if err != nil {
		return false, err
	}
	garde, err := s.Prompt.Choose("Lequel la désigne ensuite ?", ui.Options(
		autre, "@"+autre+" — son nom et son matricule l'emportent",
		account, "@"+account+" — les siens l'emportent"), autre)
	if err != nil {
		return false, err
	}
	rejoint := account
	if strings.EqualFold(garde, account) {
		rejoint = autre
	}

	reunion, err := users.PlanJoin(rows, set, s.Viewer, org, rejoint, garde)
	if err != nil {
		console.Failure("%v", err)
		return false, nil
	}
	s.printJoining(reunion)
	confirme, err := s.Prompt.Confirm("Réunir @"+reunion.Account+" à @"+reunion.Principal+" ?", false)
	if err != nil || !confirme {
		return false, err
	}
	return s.applyJoin(org, reunion) == ExitOK, nil
}

// chooseAndSplit demande lequel des comptes réunis séparer, et le sépare une
// fois confirmé. Celui qui ne désigne pas la personne est proposé d'abord :
// c'est le plus souvent celui qu'on a réuni à tort.
func (s *Session) chooseAndSplit(org string, rows []users.Row, set *registry.Set,
	reunis []string) (bool, error) {
	if len(reunis) < 2 {
		s.Console.Warning("Aucun de ces comptes n'est réuni à un autre au registre.")
		return false, nil
	}
	ordre := append(append([]string(nil), reunis[1:]...), reunis[0])
	options := make([]ui.Option, 0, len(ordre))
	for _, compte := range ordre {
		options = append(options, ui.Option{Value: compte, Label: "@" + compte})
	}
	compte, err := s.Prompt.Choose("Compte à séparer", options, options[0].Value)
	if err != nil {
		return false, err
	}
	separation, err := users.PlanSplit(rows, set, s.Viewer, org, compte)
	if err != nil {
		s.Console.Failure("%v", err)
		return false, nil
	}
	s.printSplitting(separation)
	confirme, err := s.Prompt.Confirm("Séparer @"+compte+" ?", false)
	if err != nil || !confirme {
		return false, err
	}
	return s.applySplit(org, separation) == ExitOK, nil
}

// printJoining dit ce qu'une réunion fera, avant qu'elle ne s'écrive.
func (s *Session) printJoining(reunion users.Joining) {
	console := s.Console
	console.Heading("Réunir @" + reunion.Account + " à @" + reunion.Principal)
	vide := func(valeur string) string {
		if valeur == "" {
			return console.Dim("aucun")
		}
		return valeur
	}
	console.Printf("  %s %s", console.Dim(pad("Nom complet", 18)), vide(reunion.FullName))
	console.Printf("  %s %s", console.Dim(pad("Matricule", 18)), vide(reunion.StudentID))
	console.Printf("  %s %s", console.Dim(pad("Rôle", 18)), reunion.Role)
	console.Printf("  %s %s", console.Dim(pad("Comptes", 18)),
		"@"+strings.Join(reunion.Accounts, "  @"))
	console.Note("@%s la désigne désormais. Aucun dépôt n'est renommé ni retiré, et "+
		"aucune liste de groupe n'est touchée : elle sera invitée sous ses deux comptes "+
		"aux prochains travaux.", reunion.Principal)
}

// printSplitting dit ce qu'une séparation fera.
func (s *Session) printSplitting(separation users.Splitting) {
	s.Console.Heading("Séparer @" + separation.Account)
	s.Console.Note("@%s redevient une personne à lui seul, avec le nom et le matricule "+
		"que sa fiche portait ; @%s restent ensemble. Ses dépôts restent les siens, et "+
		"aucune liste n'est touchée.", separation.Account,
		strings.Join(separation.Remaining, ", @"))
}

// applyJoin écrit la réunion au registre.
func (s *Session) applyJoin(org string, reunion users.Joining) int {
	if _, err := s.registryOf(org).Apply(reunion.Change); err != nil {
		s.Console.Failure("%v", err)
		return ExitFailure
	}
	s.Console.Success("@%s et @%s sont désormais une même personne.",
		reunion.Account, reunion.Principal)
	s.Console.Note("Pour défaire : %s --user %s --separate", brand.Command, reunion.Account)
	return ExitOK
}

// applySplit écrit la séparation au registre.
func (s *Session) applySplit(org string, separation users.Splitting) int {
	if _, err := s.registryOf(org).Apply(separation.Change); err != nil {
		s.Console.Failure("%v", err)
		return ExitFailure
	}
	s.Console.Success("@%s est de nouveau une personne à lui seul.", separation.Account)
	return ExitOK
}

// intitule nomme une personne dans une liste de choix : son nom, et tous ses
// comptes — c'est en les voyant qu'on reconnaît quelqu'un qui en a deux.
func intitule(nom string, comptes []string) string {
	if nom == "" {
		nom = "nom inconnu"
	}
	return nom + "  (@" + strings.Join(comptes, ", @") + ")"
}
