// Package preload va chercher d'avance ce qu'un écran montrera.
//
// Deux lectures coûtent cher, et ce sont justement celles qu'on attend :
// l'historique des dépôts — « Relever les remises » — et leurs accès —
// « Inspecter les accès ». Deux requêtes chacune, par dépôt, et un groupe en
// compte autant que d'étudiants par travail. Les demander au moment où l'écran
// s'ouvre, c'est attendre devant lui ; les demander avant, c'est ne plus
// attendre du tout.
//
// Ce qui se prépare est borné à ce qu'on regarde vraiment : les cours de la
// session la plus récente. Les sessions passées ne bougent plus — leurs dépôts
// ne recevront plus rien, leurs accès ne changeront plus —, et les relire
// coûterait cher pour un écran que personne ne rouvre.
//
// Le préchargement ne va chercher que ce qui manque, et écrit dans la même
// mémoire que les trois interfaces consultent : relancé dans l'heure, il ne
// coûte rien, et ce qu'il a lu sert aussi bien au navigateur qu'au terminal.
package preload

import (
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
)

// Batch borne ce qu'un préchargement demande d'un coup. Le découpage sert deux
// choses : un arrêt demandé est pris entre deux lots plutôt qu'à la fin, et ce
// qui a été lu est mémorisé au fur et à mesure — un préchargement interrompu
// laisse la moitié de l'écran prête plutôt que rien.
const Batch = 20

// Reader est ce qu'un préchargement lit. Le résolveur de « identity » le fait
// déjà ; l'interface n'est là que pour que les tests puissent l'observer.
type Reader interface {
	Handins(org string, repos []string, jusqua identity.Reading,
		onProgress func(done, total int, repo string)) map[string]groups.Handin
	Accesses(org string, repos []string, jusqua identity.Reading,
		onProgress func(done, total int, repo string)) map[string]identity.Access
}

// Repos nomme les dépôts qu'un préchargement prépare : ceux de tous les cours
// de la session la plus récente, travaux confondus.
func Repos(cours []classroom.Classroom, repos []groups.RepoInfo) []string {
	vus := map[string]bool{}
	noms := make([]string, 0, len(repos))
	// Deux groupes d'une même session partagent parfois un dépôt adopté : le
	// préparer deux fois le lirait deux fois.
	for _, groupe := range classroom.LatestSession(cours) {
		for _, depot := range groupe.Owned(repos) {
			if vus[strings.ToLower(depot.Name)] {
				continue
			}
			vus[strings.ToLower(depot.Name)] = true
			noms = append(noms, depot.Name)
		}
	}
	return noms
}

// Warmer tient les préchargements d'une séance de travail : un par
// organisation, lancé une seule fois, abandonné dès qu'on quitte.
type Warmer struct {
	arret chan struct{}
	once  sync.Once

	mutex  sync.Mutex
	lances map[string]bool
	fini   sync.WaitGroup
}

// New prépare un préchargeur au repos.
func New() *Warmer {
	return &Warmer{arret: make(chan struct{}), lances: map[string]bool{}}
}

// Warm prépare en arrière-plan les dépôts des cours de la session la plus
// récente. Le premier appel pour une organisation lance le travail ; les
// suivants ne font rien — ce qui a été lu est mémorisé, et le relire en boucle
// coûterait le temps qu'on prétend gagner.
//
// Rien n'est rendu et rien n'est signalé : un préchargement qui échoue ne prive
// de rien, puisque le geste explicite qu'il devait épargner reste là.
func (w *Warmer) Warm(reader Reader, org string, cours []classroom.Classroom,
	repos []groups.RepoInfo) {
	if reader == nil || strings.TrimSpace(org) == "" || w.Stopped() {
		return
	}
	noms := Repos(cours, repos)
	if len(noms) == 0 {
		return
	}

	w.mutex.Lock()
	if w.lances[strings.ToLower(org)] {
		w.mutex.Unlock()
		return
	}
	w.lances[strings.ToLower(org)] = true
	w.fini.Add(1)
	w.mutex.Unlock()

	go func() {
		defer w.fini.Done()
		w.lire(reader, org, noms)
	}()
}

// lire enchaîne les deux lectures, lot par lot. Les historiques d'abord : ils
// se périment le plus vite, et c'est la pastille qu'on vient voir.
func (w *Warmer) lire(reader Reader, org string, noms []string) {
	for _, lot := range lots(noms) {
		if w.Stopped() {
			return
		}
		reader.Handins(org, lot, identity.Fetch, nil)
	}
	for _, lot := range lots(noms) {
		if w.Stopped() {
			return
		}
		reader.Accesses(org, lot, identity.Fetch, nil)
	}
}

// Stop abandonne les préchargements en cours. Il ne les attend pas : ce qu'ils
// avaient lu est déjà mémorisé, et le reste ne manquera à personne.
func (w *Warmer) Stop() { w.once.Do(func() { close(w.arret) }) }

// Stopped dit qu'un arrêt a été demandé.
func (w *Warmer) Stopped() bool {
	select {
	case <-w.arret:
		return true
	default:
		return false
	}
}

// Wait attend la fin des préchargements lancés. Les tests en ont besoin ; les
// interfaces, non — c'est un travail qu'on oublie.
func (w *Warmer) Wait() { w.fini.Wait() }

// lots découpe une liste de dépôts en tranches de taille bornée.
func lots(noms []string) [][]string {
	tranches := make([][]string, 0, len(noms)/Batch+1)
	for debut := 0; debut < len(noms); debut += Batch {
		fin := debut + Batch
		if fin > len(noms) {
			fin = len(noms)
		}
		tranches = append(tranches, noms[debut:fin])
	}
	return tranches
}
