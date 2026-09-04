package app

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/orgs"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Valeur réservée du menu des organisations.
const orgFreeEntry = "\x00saisir"

// orgAccess résume ce que le compte connecté peut faire dans une organisation.
type orgAccess = orgs.Access

// organizations liste les organisations du compte connecté, avec le rôle et le
// droit d'y créer des dépôts. La liste est mise en cache : elle ne change guère.
func (s *Session) organizations() []orgAccess {
	if s.orgAccesses != nil {
		return s.orgAccesses
	}

	var progress *ui.Progress
	accesses, err := orgs.List(s.Client, s.Cache, s.Viewer, s.Options.Jobs,
		func(done, total int, login string) {
			if progress == nil {
				progress = ui.NewProgress(s.Console, "Organisations", total)
			}
			progress.Update(done, login)
		})
	if progress != nil {
		progress.Finish("")
	}
	if err != nil {
		// Sans « read:org », GitHub ne révèle pas les adhésions.
		s.Console.Note("Liste des organisations indisponible : %v "+
			"(portée « read:org » absente ? gh auth refresh -s read:org).", err)
		return nil
	}
	if len(accesses) == 0 {
		return nil
	}
	s.orgAccesses = accesses
	return accesses
}

// accessFor retrouve ce que l'on sait d'une organisation déjà inventoriée.
func (s *Session) accessFor(org string) (orgAccess, bool) {
	return orgs.Find(s.orgAccesses, org)
}

// pickOrg propose les organisations du compte, ou demande un nom à défaut.
func (s *Session) pickOrg() (string, error) {
	accesses := s.organizations()
	if len(accesses) == 0 {
		return s.askOrgName()
	}

	options := make([]ui.Option, 0, len(accesses)+1)
	for _, access := range accesses {
		options = append(options, ui.Option{Value: access.Login, Label: access.Label()})
	}
	options = append(options, ui.Option{Value: orgFreeEntry, Label: "Saisir un autre nom…"})

	choice, err := s.Prompt.Choose("Organisation GitHub", options, s.defaultOrg(accesses))
	if err != nil {
		return "", err
	}
	if choice == orgFreeEntry {
		return s.askOrgName()
	}
	return choice, nil
}

// defaultOrg place le curseur sur l'organisation de la dernière fois.
func (s *Session) defaultOrg(accesses []orgAccess) string {
	for _, access := range accesses {
		if strings.EqualFold(access.Login, s.Settings.Org) {
			return access.Login
		}
	}
	if len(accesses) > 0 {
		return accesses[0].Login
	}
	return orgFreeEntry
}

// askOrgName demande un nom d'organisation au clavier.
func (s *Session) askOrgName() (string, error) {
	return s.Prompt.Ask(ui.Question{
		Title:   "Organisation GitHub",
		Default: s.Settings.Org,
		Validate: func(value string) (string, error) {
			return valid.Login(value, "Organisation")
		},
	})
}
