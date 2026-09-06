package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
)

// États possibles d'un travail.
const (
	JobRunning  = "en cours"
	JobDone     = "terminé"
	JobFailed   = "échec"
	JobCanceled = "annulé"
)

// Natures d'événements poussées au navigateur.
const (
	EventProgress = "avancement"
	EventLine     = "ligne"
	EventWarning  = "avertissement"
	EventEnd      = "fin"
)

// Event est une étape d'un travail, telle que la voit le navigateur.
type Event struct {
	Seq   int    `json:"seq"`
	Kind  string `json:"kind"`
	Text  string `json:"text,omitempty"`
	Done  int    `json:"done,omitempty"`
	Total int    `json:"total,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// Job est une opération longue menée en arrière-plan : création de dépôts,
// clonage, inspection des accès. Le navigateur en suit le déroulement.
type Job struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Started string `json:"started"`

	mutex   sync.Mutex
	status  string
	failure string
	scope   string // portée manquante, quand l'échec vient de là
	result  any
	events  []Event
	changed chan struct{}
	cancel  context.CancelFunc
	ctx     context.Context
	ended   time.Time
}

// State est l'instantané d'un travail, sans son journal.
type State struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Failure string `json:"failure,omitempty"`
	// Scope nomme la portée qui a manqué : l'interface propose alors de
	// renouveler le jeton plutôt que de laisser l'échec sans suite.
	Scope  string `json:"scope,omitempty"`
	Result any    `json:"result,omitempty"`
	Events int    `json:"events"`
}

// Context expire dès que le travail est annulé.
func (j *Job) Context() context.Context { return j.ctx }

// Canceled indique une annulation demandée depuis le navigateur.
func (j *Job) Canceled() bool { return j.ctx.Err() != nil }

// State renvoie l'instantané du travail.
func (j *Job) State() State {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	return State{
		ID: j.ID, Kind: j.Kind, Label: j.Label, Status: j.status,
		Failure: j.failure, Scope: j.scope, Result: j.result, Events: len(j.events),
	}
}

// emit ajoute un événement et réveille les navigateurs à l'écoute.
func (j *Job) emit(event Event) {
	j.mutex.Lock()
	event.Seq = len(j.events) + 1
	j.events = append(j.events, event)
	previous := j.changed
	j.changed = make(chan struct{})
	j.mutex.Unlock()
	close(previous)
}

// Progress signale un avancement chiffré.
func (j *Job) Progress(done, total int, text string) {
	j.emit(Event{Kind: EventProgress, Done: done, Total: total, Text: text})
}

// Line ajoute une ligne au journal, avec une donnée structurée facultative.
func (j *Job) Line(text string, data any) {
	j.emit(Event{Kind: EventLine, Text: text, Data: data})
}

// Warn signale un incident qui n'interrompt pas le travail.
func (j *Job) Warn(text string) { j.emit(Event{Kind: EventWarning, Text: text}) }

// since renvoie les événements postérieurs à seq, l'état de fin, et un canal
// refermé à la prochaine nouveauté.
func (j *Job) since(seq int) ([]Event, bool, <-chan struct{}) {
	j.mutex.Lock()
	defer j.mutex.Unlock()
	var fresh []Event
	if seq < len(j.events) {
		fresh = append(fresh, j.events[seq:]...)
	}
	return fresh, j.status != JobRunning, j.changed
}

// finish clôt le travail et publie son issue.
func (j *Job) finish(result any, err error) {
	j.mutex.Lock()
	switch {
	case j.ctx.Err() != nil:
		j.status = JobCanceled
	case err != nil:
		j.status, j.failure, j.scope = JobFailed, err.Error(), ghapi.MissingScope(err)
	default:
		j.status, j.result = JobDone, result
	}
	j.ended = time.Now()
	state := State{ID: j.ID, Kind: j.Kind, Label: j.Label, Status: j.status,
		Failure: j.failure, Scope: j.scope, Result: j.result}
	j.mutex.Unlock()
	j.emit(Event{Kind: EventEnd, Text: state.Status, Data: state})
}

// Jobs retient les travaux de la session, le temps que le navigateur en lise
// le bilan.
type Jobs struct {
	// Retention borne la durée de conservation d'un travail terminé.
	Retention time.Duration

	mutex sync.Mutex
	items map[string]*Job
}

// NewJobs prépare un registre vide.
func NewJobs() *Jobs {
	return &Jobs{Retention: 2 * time.Hour, items: map[string]*Job{}}
}

// Start lance un travail en arrière-plan et renvoie sa fiche aussitôt.
func (s *Jobs) Start(kind, label string, run func(*Job) (any, error)) *Job {
	lifetime, cancel := context.WithCancel(context.Background())
	job := &Job{
		ID:      identifier(),
		Kind:    kind,
		Label:   label,
		Started: time.Now().Format(time.RFC3339),
		status:  JobRunning,
		changed: make(chan struct{}),
		cancel:  cancel,
		ctx:     lifetime,
	}

	s.mutex.Lock()
	s.prune()
	s.items[job.ID] = job
	s.mutex.Unlock()

	go func() {
		defer cancel()
		// Un incident dans un travail ne doit pas emporter le serveur : il
		// devient l'échec du travail, visible dans le navigateur.
		defer func() {
			if recovered := recover(); recovered != nil {
				job.finish(nil, fmt.Errorf("interruption inattendue : %v", recovered))
			}
		}()
		result, err := run(job)
		job.finish(result, err)
	}()
	return job
}

// Get retrouve un travail par son identifiant.
func (s *Jobs) Get(id string) (*Job, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	job, found := s.items[id]
	return job, found
}

// Cancel demande l'arrêt d'un travail en cours.
func (s *Jobs) Cancel(id string) bool {
	job, found := s.Get(id)
	if !found {
		return false
	}
	job.cancel()
	return true
}

// prune oublie les travaux terminés depuis assez longtemps. Appelée sous verrou.
func (s *Jobs) prune() {
	for id, job := range s.items {
		job.mutex.Lock()
		expired := job.status != JobRunning && !job.ended.IsZero() &&
			time.Since(job.ended) > s.Retention
		job.mutex.Unlock()
		if expired {
			delete(s.items, id)
		}
	}
}

// identifier tire un identifiant de travail imprévisible.
func identifier() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(raw)
}
