package fakegh

import (
	"sync"
	"time"
)

// Compter les requêtes ne dit pas si elles ont été menées de front. La barrière
// le dit : posée dans Hook, elle retient chaque requête jusqu'à ce qu'un
// certain nombre soit en vol, ce qui ne peut arriver que si l'appelant en a
// plusieurs ensemble.
//
// Le délai la libère si le compte n'est jamais atteint : un appelant resté en
// série doit échouer sur une assertion, pas figer le test.

// Barrier retient des requêtes jusqu'à ce que « seuil » d'entre elles se
// croisent.
type Barrier struct {
	seuil  int
	delai  time.Duration
	ouvrir sync.Once
	ouvert chan struct{}

	mutex   sync.Mutex
	envol   int
	franchi bool
}

// NewBarrier prépare une barrière. Le délai borne l'attente d'une requête
// isolée ; une seule la subit, la barrière s'ouvrant ensuite pour toutes.
func NewBarrier(seuil int, delai time.Duration) *Barrier {
	return &Barrier{seuil: seuil, delai: delai, ouvert: make(chan struct{})}
}

// Wait retient la requête courante jusqu'à ce que le seuil soit atteint.
func (b *Barrier) Wait() {
	b.mutex.Lock()
	b.envol++
	assez := b.envol >= b.seuil
	if assez {
		b.franchi = true
	}
	b.mutex.Unlock()

	if assez {
		b.open()
		return
	}
	select {
	case <-b.ouvert:
	case <-time.After(b.delai):
		b.open()
	}
}

// Reached dit si le seuil a été atteint : c'est l'assertion du test.
func (b *Barrier) Reached() bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.franchi
}

func (b *Barrier) open() { b.ouvrir.Do(func() { close(b.ouvert) }) }
