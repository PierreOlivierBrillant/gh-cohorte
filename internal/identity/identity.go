// Package identity retrouve le nom complet de la personne derrière un dépôt.
// Les sources sont interrogées du moins cher au plus cher : bilans d'exécution
// déjà présents sur le disque, cache local, puis profil GitHub.
package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
)

// Au-delà, on ne relit pas tout l'historique des bilans.
const (
	MaxReports  = 200
	DefaultJobs = 8
)

// Pair associe un nom de dépôt au compte GitHub qu'il concerne.
type Pair struct {
	Repo  string
	Login string
}

// Resolver retrouve les noms complets et mémorise ce qu'il apprend.
type Resolver struct {
	client     *ghapi.Client
	store      *cache.Cache
	reportsDir string
	jobs       int

	mutex         sync.Mutex
	byRepo        map[string]string
	byLogin       map[string]string
	reportsLoaded bool
}

// New construit un résolveur ; client peut être nil pour s'en tenir au disque.
func New(client *ghapi.Client, store *cache.Cache, reportsDir string, jobs int) *Resolver {
	if jobs < 1 {
		jobs = DefaultJobs
	}
	if store == nil {
		store = cache.NewIn(os.TempDir(), false)
	}
	return &Resolver{
		client: client, store: store, reportsDir: reportsDir, jobs: jobs,
		byRepo: map[string]string{}, byLogin: map[string]string{},
	}
}

// reportEntry ne retient du bilan que ce qui sert à nommer les personnes.
type reportEntry struct {
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Repo     string `json:"repo"`
}

type reportFile struct {
	Results []reportEntry `json:"results"`
}

// LoadReports relit les bilans d'exécution : ils portent déjà les noms complets.
func (r *Resolver) LoadReports() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.loadReportsLocked()
}

func (r *Resolver) loadReportsLocked() {
	if r.reportsLoaded {
		return
	}
	r.reportsLoaded = true
	if r.reportsDir == "" {
		return
	}
	matches, err := filepath.Glob(filepath.Join(r.reportsDir, "*.json"))
	if err != nil || len(matches) == 0 {
		return
	}
	// Du plus ancien au plus récent : les informations fraîches l'emportent.
	sort.Slice(matches, func(i, j int) bool {
		left, errLeft := os.Stat(matches[i])
		right, errRight := os.Stat(matches[j])
		if errLeft != nil || errRight != nil {
			return matches[i] < matches[j]
		}
		return left.ModTime().Before(right.ModTime())
	})
	if len(matches) > MaxReports {
		matches = matches[len(matches)-MaxReports:]
	}

	for _, path := range matches {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var report reportFile
		if err := json.Unmarshal(content, &report); err != nil {
			continue
		}
		for _, entry := range report.Results {
			fullName := strings.TrimSpace(entry.FullName)
			if fullName == "" {
				continue
			}
			if entry.Repo != "" {
				r.byRepo[strings.ToLower(entry.Repo)] = fullName
			}
			if entry.Username != "" {
				r.byLogin[strings.ToLower(entry.Username)] = fullName
			}
		}
	}
}

// Known renvoie un nom déjà connu sans le moindre appel réseau.
func (r *Resolver) Known(repoName, login string) (string, bool) {
	r.mutex.Lock()
	r.loadReportsLocked()
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
