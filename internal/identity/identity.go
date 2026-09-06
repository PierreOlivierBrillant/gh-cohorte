// Package identity retrouve le nom complet de la personne derrière un compte
// GitHub, quand personne ne l'a jamais donné : le cache local d'abord, le
// profil GitHub ensuite.
//
// Ce n'est plus la source des noms — le registre de l'organisation l'est. Il ne
// sert qu'à combler ce que le registre ignore : un compte adopté depuis des
// dépôts hérités, que rien n'a encore nommé.
package identity

import (
	"os"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
)

// DefaultJobs borne les profils demandés de front.
const DefaultJobs = 8

// Pair associe un nom de dépôt au compte GitHub qu'il concerne.
type Pair struct {
	Repo  string
	Login string
}

// Resolver retrouve les noms complets et mémorise ce qu'il apprend.
type Resolver struct {
	client *ghapi.Client
	store  *cache.Cache
	jobs   int

	mutex   sync.Mutex
	byRepo  map[string]string
	byLogin map[string]string
}

// New construit un résolveur ; client peut être nil pour s'en tenir au cache.
func New(client *ghapi.Client, store *cache.Cache, jobs int) *Resolver {
	if jobs < 1 {
		jobs = DefaultJobs
	}
	if store == nil {
		store = cache.NewIn(os.TempDir(), false)
	}
	return &Resolver{
		client: client, store: store, jobs: jobs,
		byRepo: map[string]string{}, byLogin: map[string]string{},
	}
}

// Known renvoie un nom déjà connu sans le moindre appel réseau.
func (r *Resolver) Known(repoName, login string) (string, bool) {
	r.mutex.Lock()
	if found, ok := r.byRepo[strings.ToLower(repoName)]; ok {
		r.mutex.Unlock()
		return found, true
	}
	if found, ok := r.byLogin[strings.ToLower(login)]; ok {
		r.mutex.Unlock()
		return found, true
	}
	r.mutex.Unlock()

	var cached string
	if r.store.Get(cache.ProfileKey(login), cache.ProfileTTL, &cached) {
		return cached, true
	}
	return "", false
}

// Missing renvoie les couples dont le nom n'est pas encore connu localement.
func (r *Resolver) Missing(pairs []Pair) []Pair {
	var missing []Pair
	for _, pair := range pairs {
		if _, found := r.Known(pair.Repo, pair.Login); !found {
			missing = append(missing, pair)
		}
	}
	return missing
}

// Resolve renvoie « nom de dépôt → nom complet » ; une valeur vide signale un
// inconnu. Avec fetch à faux, aucun appel réseau n'est fait.
func (r *Resolver) Resolve(pairs []Pair, fetch bool, onProgress func(done, total int, repo string)) map[string]string {
	names := make(map[string]string, len(pairs))
	var missing []Pair
	for _, pair := range pairs {
		if found, ok := r.Known(pair.Repo, pair.Login); ok {
			names[pair.Repo] = found
			continue
		}
		names[pair.Repo] = ""
		missing = append(missing, pair)
	}
	if len(missing) == 0 || !fetch || r.client == nil {
		return names
	}

	workers := r.jobs
	if workers > len(missing) {
		workers = len(missing)
	}
	type outcome struct {
		pair Pair
		name string
	}
	queue := make(chan Pair)
	results := make(chan outcome)
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for pair := range queue {
				name := ""
				if user, err := r.client.GetUser(pair.Login); err == nil && user != nil {
					name = strings.TrimSpace(user.Name)
				}
				results <- outcome{pair: pair, name: name}
			}
		}()
	}
	go func() {
		for _, pair := range missing {
			queue <- pair
		}
		close(queue)
		group.Wait()
		close(results)
	}()

	fetched := map[string]any{}
	done := 0
	for item := range results {
		done++
		names[item.pair.Repo] = item.name
		// Un profil sans nom est mémorisé aussi : inutile d'y revenir.
		fetched[cache.ProfileKey(item.pair.Login)] = item.name
		r.mutex.Lock()
		r.byLogin[strings.ToLower(item.pair.Login)] = item.name
		r.mutex.Unlock()
		if onProgress != nil {
			onProgress(done, len(missing), item.pair.Repo)
		}
	}
	r.store.SetMany(fetched)
	return names
}
