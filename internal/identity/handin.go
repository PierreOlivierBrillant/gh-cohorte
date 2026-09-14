package identity

import (
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
)

// L'historique d'un dépôt coûte deux requêtes, et un groupe en compte autant
// que d'étudiants par travail. Le relevé passe donc par ici, comme les accès et
// les équipes : mémorisé, demandé en parallèle, et interrogeable sans réseau.
//
// Ce dernier point décide de l'affichage. Ouvrir un groupe ne doit pas déclencher
// des centaines d'appels ; l'écran montre ce qu'on sait déjà, et c'est un geste
// explicite qui va chercher le reste.

// Reading dit jusqu'où un relevé a le droit d'aller.
type Reading int

const (
	// Cached s'en tient à ce qui est mémorisé : rien n'est demandé à GitHub,
	// et les dépôts qu'on n'a pas encore lus restent absents de la réponse.
	// C'est ce qui permet à une liste de montrer ses pastilles sans attendre,
	// et de dire du même coup ce qu'elle ignore.
	Cached Reading = iota
	// Fetch va chercher ce qui manque et garde le reste.
	Fetch
	// Refresh relit tout, mémoire comprise : la veille d'une remise, une heure
	// de mémoire est une heure de trop.
	Refresh
)

// Handins rend ce que l'historique de chaque dépôt dit d'une remise.
func (r *Resolver) Handins(org string, repos []string, jusqua Reading,
	onProgress func(done, total int, repo string)) map[string]groups.Handin {
	trouves := make(map[string]groups.Handin, len(repos))
	var manquants []string
	for _, repo := range repos {
		var remise groups.Handin
		if jusqua != Refresh &&
			r.store.Get(cache.HandinKey(org, repo), cache.HandinTTL, &remise) {
			trouves[repo] = remise
			continue
		}
		manquants = append(manquants, repo)
	}
	if jusqua == Cached || len(manquants) == 0 || r.client == nil {
		return trouves
	}
	for repo, remise := range r.fetchHandins(org, manquants, onProgress) {
		trouves[repo] = remise
	}
	return trouves
}

// fetchHandins interroge GitHub pour les dépôts qu'on ne connaît pas encore.
//
// Un dépôt dont la lecture échoue est laissé de côté plutôt que mémorisé vide :
// un jeton sans droit sur un dépôt privé ferait croire, sinon, que personne n'y
// a rien remis.
func (r *Resolver) fetchHandins(org string, repos []string,
	onProgress func(done, total int, repo string)) map[string]groups.Handin {
	workers := r.jobs
	if workers > len(repos) {
		workers = len(repos)
	}
	type resultat struct {
		repo   string
		remise groups.Handin
		err    error
	}
	file := make(chan string)
	sorties := make(chan resultat)
	var groupe sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		groupe.Add(1)
		go func() {
			defer groupe.Done()
			for repo := range file {
				remise, err := r.client.Handin(org, repo)
				sorties <- resultat{repo: repo, remise: remise, err: err}
			}
		}()
	}
	go func() {
		for _, repo := range repos {
			file <- repo
		}
		close(file)
		groupe.Wait()
		close(sorties)
	}()

	trouves := make(map[string]groups.Handin, len(repos))
	appris := map[string]any{}
	faits := 0
	for item := range sorties {
		faits++
		if item.err == nil {
			trouves[item.repo] = item.remise
			appris[cache.HandinKey(org, item.repo)] = item.remise
		}
		if onProgress != nil {
			onProgress(faits, len(repos), item.repo)
		}
	}
	r.store.SetMany(appris)
	return trouves
}
