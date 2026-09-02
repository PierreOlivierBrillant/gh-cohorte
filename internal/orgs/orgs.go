// Package orgs inventorie les organisations du compte connecté : nom affiché,
// rôle, et droit d'y créer des dépôts. L'inventaire est partagé par l'assistant
// du terminal et par l'interface web.
package orgs

import (
	"sort"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
)

// DefaultJobs est le nombre d'organisations inspectées de front.
const DefaultJobs = 4

// Access résume ce que le compte connecté peut faire dans une organisation.
type Access struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	// CanCreate dit si la création de dépôts y est possible ; Known dit si la
	// réponse est sûre — GitHub ne montre ce réglage qu'aux propriétaires.
	CanCreate bool `json:"can_create"`
	Known     bool `json:"known"`
}

// Label décrit l'organisation et ce qu'on peut y faire.
func (a Access) Label() string {
	label := a.Login
	if a.Name != "" && !strings.EqualFold(a.Name, a.Login) {
		label += " — " + a.Name
	}
	switch {
	case a.Role == "admin":
		return label + "  · propriétaire"
	case a.CanCreate:
		return label + "  · membre, création autorisée"
	case a.Known:
		return label + "  · membre, création réservée aux propriétaires"
	default:
		return label + "  · membre"
	}
}

// Rank classe les organisations : d'abord celles où tout est possible.
func (a Access) Rank() int {
	switch {
	case a.Role == "admin":
		return 0
	case a.CanCreate:
		return 1
	case !a.Known:
		return 2
	default:
		return 3
	}
}

// Find retrouve ce que l'on sait d'une organisation déjà inventoriée.
func Find(accesses []Access, org string) (Access, bool) {
	for _, access := range accesses {
		if strings.EqualFold(access.Login, org) {
			return access, true
		}
	}
	return Access{}, false
}

// List renvoie les organisations du compte, de la plus permissive à la moins.
// La liste est mise en cache : elle ne change guère. Une erreur signale des
// adhésions illisibles — sans « read:org », GitHub ne les révèle pas.
func List(client *ghapi.Client, store *cache.Cache, viewer string, jobs int,
	onProgress func(done, total int, login string)) ([]Access, error) {
	key := cache.OrgsKey(viewer)
	var cached []Access
	if store != nil && store.Get(key, cache.OrgsTTL, &cached) && len(cached) > 0 {
		return cached, nil
	}

	memberships, err := client.ListOrgMemberships(nil)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, nil
	}

	if jobs < 1 {
		jobs = DefaultJobs
	}
	if jobs > len(memberships) {
		jobs = len(memberships)
	}
	accesses := make([]Access, len(memberships))
	queue := make(chan int)
	var group sync.WaitGroup
	var mutex sync.Mutex
	done := 0

	for worker := 0; worker < jobs; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for position := range queue {
				membership := memberships[position]
				accesses[position] = describe(client, membership)
				mutex.Lock()
				done++
				if onProgress != nil {
					onProgress(done, len(memberships), membership.Organization.Login)
				}
				mutex.Unlock()
			}
		}()
	}
	for position := range memberships {
		queue <- position
	}
	close(queue)
	group.Wait()

	sort.Slice(accesses, func(i, j int) bool {
		if accesses[i].Rank() != accesses[j].Rank() {
			return accesses[i].Rank() < accesses[j].Rank()
		}
		return strings.ToLower(accesses[i].Login) < strings.ToLower(accesses[j].Login)
	})
	if store != nil {
		store.Set(key, accesses)
	}
	return accesses, nil
}

// describe complète une adhésion par le nom de l'organisation et le droit d'y
// créer des dépôts.
func describe(client *ghapi.Client, membership ghapi.Membership) Access {
	access := Access{Login: membership.Organization.Login, Role: membership.Role}
	// Un propriétaire peut toujours créer : inutile d'en demander plus.
	if access.Role == "admin" {
		access.CanCreate, access.Known = true, true
	}
	data, err := client.GetRepoOwnerOrg(access.Login)
	if err != nil || data == nil {
		return access
	}
	access.Name = data.Name
	if data.MembersCanCreateRepositories != nil && access.Role != "admin" {
		access.CanCreate, access.Known = *data.MembersCanCreateRepositories, true
	}
	return access
}
