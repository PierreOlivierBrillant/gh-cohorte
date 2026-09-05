package ui

import (
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

// spinnerDelay retarde l'apparition de la roue : une réponse déjà en cache
// revient en quelques millisecondes, et un clignotement se remarque plus
// qu'une attente courte.
const spinnerDelay = 200 * time.Millisecond

// Spinner signale une attente dont on ne connaît pas la durée — un appel dont
// on ne sait rien tant que GitHub n'a pas répondu, là où Progress demande un
// décompte. Sur un terminal, une roue tourne à côté du libellé puis s'efface ;
// ailleurs — sortie redirigée, script, journal — elle ne s'affiche pas : une
// animation n'a rien à faire dans un fichier, et la ligne qui suit dit déjà ce
// que l'attente a donné.
type Spinner struct {
	console *Console
	label   string
	frames  spinner.Spinner

	// life sérialise Start et Stop, mu protège ce que la roue dessine : deux
	// verrous, parce que Stop attend la fin d'un tour de dessin et ne peut pas
	// tenir celui que ce tour réclame.
	life sync.Mutex
	stop chan struct{}
	done chan struct{}

	mu     sync.Mutex
	detail string
	drawn  bool
}

// NewSpinner prépare une attente. Rien ne s'affiche avant Start.
func NewSpinner(console *Console, label string) *Spinner {
	return &Spinner{console: console, label: label, frames: spinner.MiniDot}
}

// Start lance l'animation ; à tout Start doit répondre un Stop, y compris
// lorsque l'action échoue — sans quoi la roue resterait sur la ligne.
func (s *Spinner) Start() {
	if !s.console.TTY {
		return
	}
	s.life.Lock()
	defer s.life.Unlock()
	if s.stop != nil {
		return
	}
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go s.run()
}

func (s *Spinner) run() {
	defer close(s.done)
	ticker := time.NewTicker(s.frames.FPS)
	defer ticker.Stop()
	start := time.Now()
	frame := 0
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			if time.Since(start) < spinnerDelay {
				continue
			}
			s.draw(frame)
			frame++
		}
	}
}

func (s *Spinner) draw(frame int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	line := "  " + s.console.Info(s.frames.Frames[frame%len(s.frames.Frames)]) +
		" " + s.console.Dim(s.label)
	if s.detail != "" {
		line += " " + s.console.Dim(truncate(s.detail, 40))
	}
	fmt.Fprintf(s.console.Out, "\r\033[K%s", line)
	s.drawn = true
}

// Detail précise ce qui est attendu ; la roue le reprend au tour suivant.
func (s *Spinner) Detail(text string) {
	s.mu.Lock()
	s.detail = text
	s.mu.Unlock()
}

// Stop arrête la roue et efface sa ligne. Un second appel ne fait rien, et
// l'arrêt peut venir d'une autre goroutine que celle qui a lancé la roue.
func (s *Spinner) Stop() {
	s.life.Lock()
	defer s.life.Unlock()
	if s.stop == nil {
		return
	}
	close(s.stop)
	<-s.done
	s.stop, s.done = nil, nil
	// La goroutine est terminée : plus personne n'écrit, et la ligne peut être
	// rendue à ce qui vient ensuite.
	if s.drawn {
		fmt.Fprint(s.console.Out, "\r\033[K")
		s.drawn = false
	}
}

// Await anime une attente le temps que l'action se déroule. L'action ne doit
// rien écrire elle-même : la ligne appartient à la roue tant qu'elle tourne.
func Await(console *Console, label string, action func()) {
	spin := NewSpinner(console, label)
	spin.Start()
	defer spin.Stop()
	action()
}
