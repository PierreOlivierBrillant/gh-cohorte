package classroom

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Ce fichier ne retient pas ce qu'un groupe est — GitHub le dit déjà, dans les
// noms de ses dépôts. Il retient ce que les dépôts ne peuvent pas dire :
//
//   - la liste des étudiants, avec le compte GitHub en face de chaque nom, tant
//     qu'aucun dépôt ne l'a encore matérialisée ;
//   - les réglages que les prochains travaux reprendront ;
//   - un groupe déclaré mais encore vide, en attendant son premier travail.
//
// Rien de ce que l'interface affiche n'est inventé ici. Un groupe se reconnaît
// à sa place — « a26.5n6.1010 » —, et cette place est dans le nom de chacun de
// ses dépôts.

// FileName est le nom du fichier des groupes, voisin des réglages.
const FileName = "groupes.json"

// document est ce qui est écrit sur le disque.
type document struct {
	Version    int         `json:"version"`
	Classrooms []Classroom `json:"classrooms"`
}

// PathNextTo place le fichier des groupes à côté du fichier de réglages, pour
// que « --config » les déplace ensemble.
func PathNextTo(configFile string) string {
	return filepath.Join(filepath.Dir(configFile), FileName)
}

// Store retient les groupes déclarés. Le fichier contient des noms d'étudiants :
// il est écrit en 0600, comme les réglages.
type Store struct {
	path string

	mutex sync.Mutex
	items []Classroom
}

// Open lit le fichier des groupes ; son absence donne un magasin vide.
func Open(path string) *Store {
	store := &Store{path: path}
	content, err := os.ReadFile(path)
	if err != nil {
		return store
	}
	var lu document
	if err := json.Unmarshal(content, &lu); err != nil {
		return store
	}
	store.items = make([]Classroom, 0, len(lu.Classrooms))
	for _, item := range lu.Classrooms {
		// Les réglages sont remis en forme à la lecture, pas seulement à
		// l'écriture : un gabarit dépassé doit être corrigé avant d'être montré.
		item.Defaults = item.Defaults.normalized()
		store.items = append(store.items, awaitingSession(item))
	}
	return store
}

// awaitingSession ramène au rang de préfixe hérité un groupe déclaré sous la
// nomenclature à quatre niveaux, avant que la session n'existe. Sans cela il
// viserait « .cours.groupe » — une session vide —, et ses dépôts seraient
// introuvables. Ainsi rangé, il s'affiche et se migre comme les autres.
func awaitingSession(item Classroom) Classroom {
	if strings.TrimSpace(item.Session) != "" || strings.TrimSpace(item.Course) == "" {
		return item
	}
	item.LegacyPrefix = strings.Trim(
		item.Course+naming.Separator+item.Group, naming.Separator)
	item.Course, item.Group = "", ""
	return item
}

// SessionName rend le nom long d'une session. Il se déduit de son nom court —
// « a26 » se lit « Automne 2026 » — plutôt que d'être écrit quelque part : un
// nom retenu localement ne serait vrai que sur cette machine.
func SessionName(short string) string { return naming.SessionLabel(short) }

// Sessions renvoie les sessions qui portent au moins un groupe déclaré dans
// l'organisation, de la plus récente à la plus ancienne.
func (s *Store) Sessions(org string) []Session {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	courts := make([]string, 0, len(s.items))
	for _, item := range s.items {
		if !strings.EqualFold(item.Org, org) {
			continue
		}
		courts = append(courts, item.Session)
	}
	return SessionsOf(courts)
}

// Path renvoie l'emplacement du fichier.
func (s *Store) Path() string { return s.path }

// detach copie la liste des étudiants : sans cela, le groupe rendu partagerait
// sa tranche avec le magasin, et le modifier écrirait dans son dos.
func detach(classroom Classroom) Classroom {
	classroom.Students = append([]roster.Person(nil), classroom.Students...)
	return classroom
}

// List renvoie les groupes déclarés dans une organisation, classés par place.
func (s *Store) List(org string) []Classroom {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	copie := make([]Classroom, 0, len(s.items))
	for _, item := range s.items {
		if !strings.EqualFold(item.Org, org) {
			continue
		}
		copie = append(copie, detach(item))
	}
	sort.Slice(copie, func(i, j int) bool {
		return strings.ToLower(copie[i].Scope()) < strings.ToLower(copie[j].Scope())
	})
	return copie
}

// Find retrouve un groupe par sa place dans une organisation. C'est la seule
// façon de le désigner : un identifiant tiré au hasard ne voudrait rien dire
// hors de cette machine, alors que la place est dans le nom des dépôts.
func (s *Store) Find(org, scope string) (Classroom, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, item := range s.items {
		if strings.EqualFold(item.Org, org) && strings.EqualFold(item.Scope(), scope) {
			return detach(item), true
		}
	}
	return Classroom{}, false
}

// Save écrit ce qu'on retient d'un groupe : sa liste et ses réglages. Il n'y a
// rien à créer ni à mettre à jour séparément — un groupe est à sa place, ou il
// n'y est pas.
func (s *Store) Save(classroom Classroom) (Classroom, error) {
	valide, err := classroom.Validate()
	if err != nil {
		return classroom, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for position, item := range s.items {
		if strings.EqualFold(item.Org, valide.Org) &&
			strings.EqualFold(item.Scope(), valide.Scope()) {
			s.items[position] = detach(valide)
			return valide, s.saveLocked()
		}
	}
	s.items = append(s.items, detach(valide))
	return valide, s.saveLocked()
}

// Move suit un groupe qui change de place : ses dépôts viennent d'être
// renommés, et ce qu'on retient de lui doit les suivre.
func (s *Store) Move(org, scope string, cible Classroom) (Classroom, error) {
	valide, err := cible.Validate()
	if err != nil {
		return cible, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, item := range s.items {
		if strings.EqualFold(item.Org, valide.Org) &&
			strings.EqualFold(item.Scope(), valide.Scope()) &&
			!strings.EqualFold(item.Scope(), scope) {
			return cible, valid.Errorf(
				"Un groupe couvre déjà « %s » dans « %s ».", valide.Scope(), valide.Org)
		}
	}
	restants := make([]Classroom, 0, len(s.items)+1)
	for _, item := range s.items {
		if strings.EqualFold(item.Org, org) && strings.EqualFold(item.Scope(), scope) {
			continue
		}
		restants = append(restants, item)
	}
	s.items = append(restants, detach(valide))
	return valide, s.saveLocked()
}

// Forget oublie ce qu'on retenait d'un groupe. Aucun dépôt n'est touché : s'il
// en reste, le groupe continue d'exister sur GitHub et de s'afficher — sans sa
// liste ni ses réglages.
func (s *Store) Forget(org, scope string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	restants := make([]Classroom, 0, len(s.items))
	trouve := false
	for _, item := range s.items {
		if strings.EqualFold(item.Org, org) && strings.EqualFold(item.Scope(), scope) {
			trouve = true
			continue
		}
		restants = append(restants, item)
	}
	if !trouve {
		return valid.Errorf("Aucune liste retenue pour « %s ».", scope)
	}
	s.items = restants
	return s.saveLocked()
}

// saveLocked écrit le fichier ; appelée sous verrou.
func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return valid.Errorf("Groupes non enregistrés : %v", err)
	}
	payload, err := json.MarshalIndent(
		document{Version: 2, Classrooms: s.items}, "", "  ")
	if err != nil {
		return valid.Errorf("Groupes non enregistrés : %v", err)
	}
	if err := os.WriteFile(s.path, append(payload, '\n'), 0o600); err != nil {
		return valid.Errorf("Groupes non enregistrés : %v", err)
	}
	return nil
}
