package identity

import (
	"sort"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
)

// À qui un dépôt appartient-il ? Son nom le dit mal : « kickmyb-firebase-alice »
// se découpe aussi bien en « kickmyb » et « firebase-alice », et le compte qu'on
// en tire alors n'existe pas. Les accès, eux, ne se devinent pas — la personne
// qui travaille dans un dépôt y a été ajoutée, et c'est cette liste-là qui fait
// foi.
//
// L'invitation compte autant que l'accès établi : un étudiant qui n'a pas encore
// cliqué sur le courriel de GitHub n'est pas collaborateur, et son dépôt est
// pourtant bien le sien.

// Owner est ce qu'on a appris d'un dépôt.
type Owner struct {
	// Login est le compte de la personne à qui le dépôt appartient. Vide quand
	// les accès n'ont pas tranché.
	Login string
	// Access nomme tous les comptes qui y ont accès, l'enseignant compris.
	// C'est ce que le cache retient, et ce qu'une revue peut montrer.
	Access []string
}

// Owners rend, pour chaque dépôt, le compte que ses accès désignent.
//
// Le viewer — celui qui se sert de l'outil — est écarté d'office : il a accès à
// tout, et n'est donc l'indice de rien. Les dépôts déjà connus ne coûtent aucun
// appel ; les autres sont demandés en parallèle, comme les profils.
func (r *Resolver) Owners(org string, repos []string, viewer string,
	onProgress func(done, total int, repo string)) map[string]Owner {
	trouves := make(map[string]Owner, len(repos))
	var manquants []string
	for _, repo := range repos {
		var acces []string
		if r.store.Get(cache.AccessKey(org, repo), cache.AccessTTL, &acces) {
			trouves[repo] = decider(repo, acces, viewer)
			continue
		}
		manquants = append(manquants, repo)
	}
	if len(manquants) == 0 || r.client == nil {
		return trouves
	}

	workers := r.jobs
	if workers > len(manquants) {
		workers = len(manquants)
	}
	type resultat struct {
		repo  string
		acces []string
	}
	file := make(chan string)
	sorties := make(chan resultat)
	var groupe sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		groupe.Add(1)
		go func() {
			defer groupe.Done()
			for repo := range file {
				sorties <- resultat{repo: repo, acces: r.accessOf(org, repo)}
			}
		}()
	}
	go func() {
		for _, repo := range manquants {
			file <- repo
		}
		close(file)
		groupe.Wait()
		close(sorties)
	}()

	appris := map[string]any{}
	faits := 0
	for item := range sorties {
		faits++
		trouves[item.repo] = decider(item.repo, item.acces, viewer)
		// Un dépôt sans accès direct est mémorisé aussi : c'est une réponse,
		// et y revenir coûterait le même appel pour la même chose.
		appris[cache.AccessKey(org, item.repo)] = item.acces
		if onProgress != nil {
			onProgress(faits, len(manquants), item.repo)
		}
	}
	r.store.SetMany(appris)
	return trouves
}

// accessOf relève qui a accès à un dépôt : les collaborateurs directs, et les
// personnes invitées qui n'ont pas encore accepté. Une erreur ne rend rien —
// un dépôt dont on ne peut pas lire les accès se lira par son nom, comme avant.
func (r *Resolver) accessOf(org, repo string) []string {
	vus := map[string]bool{}
	comptes := make([]string, 0, 2)
	ajouter := func(login string) {
		login = strings.TrimSpace(login)
		if login == "" || vus[strings.ToLower(login)] {
			return
		}
		vus[strings.ToLower(login)] = true
		comptes = append(comptes, login)
	}

	if collaborateurs, err := r.client.ListCollaborators(org, repo); err == nil {
		for _, personne := range collaborateurs {
			ajouter(personne.Login)
		}
	}
	if invitations, err := r.client.ListInvitations(org, repo); err == nil {
		for _, invitation := range invitations {
			ajouter(invitation.Invitee.Login)
		}
	}
	sort.Slice(comptes, func(i, j int) bool {
		return strings.ToLower(comptes[i]) < strings.ToLower(comptes[j])
	})
	return comptes
}

// decider choisit le compte que les accès désignent, une fois le viewer écarté.
func decider(repo string, acces []string, viewer string) Owner {
	candidats := make([]string, 0, len(acces))
	for _, compte := range acces {
		if viewer != "" && strings.EqualFold(compte, viewer) {
			continue
		}
		candidats = append(candidats, compte)
	}
	login, _ := groups.Owner(repo, candidats)
	return Owner{Login: login, Access: acces}
}
