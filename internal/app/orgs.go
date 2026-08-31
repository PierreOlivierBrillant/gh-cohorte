package app

import (
	"sort"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Valeur réservée du menu des organisations.
const orgFreeEntry = "\x00saisir"

// orgAccess résume ce que le compte connecté peut faire dans une organisation.
type orgAccess struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	// CanCreate dit si la création de dépôts y est possible ; Known dit si la
	// réponse est sûre — GitHub ne montre ce réglage qu'aux propriétaires.
	CanCreate bool `json:"can_create"`
	Known     bool `json:"known"`
}

// Label décrit l'organisation et ce qu'on peut y faire.
func (o orgAccess) Label() string {
	label := o.Login
	if o.Name != "" && !strings.EqualFold(o.Name, o.Login) {
		label += " — " + o.Name
	}
	switch {
	case o.Role == "admin":
		return label + "  · propriétaire"
	case o.CanCreate:
		return label + "  · membre, création autorisée"
	case o.Known:
		return label + "  · membre, création réservée aux propriétaires"
	default:
		return label + "  · membre"
	}
}

// rank classe les organisations : d'abord celles où tout est possible.
func (o orgAccess) rank() int {
	switch {
	case o.Role == "admin":
		return 0
	case o.CanCreate:
		return 1
	case !o.Known:
		return 2
	default:
		return 3
	}
}

// organizations liste les organisations du compte connecté, avec le rôle et le
// droit d'y créer des dépôts. La liste est mise en cache : elle ne change guère.
func (s *Session) organizations() []orgAccess {
	if s.orgAccesses != nil {
		return s.orgAccesses
	}
	key := cache.OrgsKey(s.Viewer)
	var cached []orgAccess
	if s.Cache.Get(key, cache.OrgsTTL, &cached) && len(cached) > 0 {
		s.orgAccesses = cached
		return cached
	}

	memberships, err := s.Client.ListOrgMemberships(nil)
	if err != nil || len(memberships) == 0 {
		if err != nil {
			// Sans « read:org », GitHub ne révèle pas les adhésions.
			s.Console.Note("Liste des organisations indisponible : %v "+
				"(portée « read:org » absente ? gh auth refresh -s read:org).", err)
		}
		return nil
	}

	progress := ui.NewProgress(s.Console, "Organisations", len(memberships))
	accesses := make([]orgAccess, len(memberships))
	queue := make(chan int)
	var group sync.WaitGroup
	var mutex sync.Mutex
	done := 0

	workers := s.Options.Jobs
	if workers > len(memberships) {
		workers = len(memberships)
	}
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for position := range queue {
				membership := memberships[position]
				accesses[position] = s.describeOrg(membership)
				mutex.Lock()
				done++
				progress.Update(done, membership.Organization.Login)
				mutex.Unlock()
			}
		}()
	}
	for position := range memberships {
		queue <- position
	}
	close(queue)
	group.Wait()
	progress.Finish("")

	sort.Slice(accesses, func(i, j int) bool {
		if accesses[i].rank() != accesses[j].rank() {
			return accesses[i].rank() < accesses[j].rank()
		}
		return strings.ToLower(accesses[i].Login) < strings.ToLower(accesses[j].Login)
	})
	s.Cache.Set(key, accesses)
	s.orgAccesses = accesses
	return accesses
}

// accessFor retrouve ce que l'on sait d'une organisation déjà inventoriée.
func (s *Session) accessFor(org string) (orgAccess, bool) {
	for _, access := range s.orgAccesses {
		if strings.EqualFold(access.Login, org) {
			return access, true
		}
	}
	return orgAccess{}, false
}

// describeOrg complète une adhésion par le nom de l'organisation et le droit
// d'y créer des dépôts.
func (s *Session) describeOrg(membership ghapi.Membership) orgAccess {
	access := orgAccess{Login: membership.Organization.Login, Role: membership.Role}
	// Un propriétaire peut toujours créer : inutile d'en demander plus.
	if access.Role == "admin" {
		access.CanCreate, access.Known = true, true
	}
	data, err := s.Client.GetRepoOwnerOrg(access.Login)
	if err != nil || data == nil {
		return access
	}
	access.Name = data.Name
	if data.MembersCanCreateRepositories != nil && access.Role != "admin" {
		access.CanCreate, access.Known = *data.MembersCanCreateRepositories, true
	}
	return access
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
