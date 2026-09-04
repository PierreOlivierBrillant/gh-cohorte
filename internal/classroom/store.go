package classroom

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// FileName est le nom du fichier des groupes, voisin des réglages.
const FileName = "groupes.json"

// document est ce qui est écrit sur le disque.
type document struct {
	Version int `json:"version"`
	// Sessions donne le nom long d'une session — « a26 » → « Automne 2026 ».
	// Il est partagé par tous les groupes de la session, et ne sert qu'à
	// l'affichage : seul le nom court entre dans les dépôts.
	Sessions   map[string]string `json:"sessions,omitempty"`
	Classrooms []Classroom       `json:"classrooms"`
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

	mutex    sync.Mutex
	items    []Classroom
	sessions map[string]string
}

// Open lit le fichier des groupes ; son absence donne un magasin vide.
func Open(path string) *Store {
	store := &Store{path: path, sessions: map[string]string{}}
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
	for court, long := range lu.Sessions {
		store.sessions[strings.ToLower(court)] = long
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

// SessionName renvoie le nom long d'une session : celui qu'on lui a donné, ou
// celui que son nom court laisse déduire — « a26 » se lit « Automne 2026 » sans
// qu'on ait à l'écrire.
func (s *Store) SessionName(short string) string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.sessionNameLocked(short)
}

func (s *Store) sessionNameLocked(short string) string {
	if long := s.sessions[strings.ToLower(short)]; long != "" {
		return long
	}
	return naming.SessionLabel(short)
}

// Sessions renvoie les sessions qui portent au moins un groupe, de la plus
// récente à la plus ancienne — les noms courts se trient bien : a26, h27, e27.
func (s *Store) Sessions() []Session {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	vues := map[string]bool{}
	liste := make([]Session, 0, len(s.items))
	for _, item := range s.items {
		court := item.Session
		if court == "" || vues[strings.ToLower(court)] {
			continue
		}
		vues[strings.ToLower(court)] = true
		liste = append(liste, Session{Short: court, Name: s.sessionNameLocked(court)})
	}
	sort.Slice(liste, func(i, j int) bool {
		return strings.ToLower(liste[i].Short) < strings.ToLower(liste[j].Short)
	})
	return liste
}

// SetSessionName retient le nom long d'une session. Vide, il est oublié : la
// session s'affiche alors par son nom court.
func (s *Store) SetSessionName(short, name string) error {
	court, err := naming.Fragment(short, "Session")
	if err != nil {
		return err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	name = strings.TrimSpace(name)
	// Un nom qui répète ce que le nom court dit déjà n'a pas à être écrit.
	if name == "" || strings.EqualFold(name, court) ||
		strings.EqualFold(name, naming.SessionLabel(court)) {
		delete(s.sessions, strings.ToLower(court))
	} else {
		s.sessions[strings.ToLower(court)] = name
	}
	return s.saveLocked()
}

// Path renvoie l'emplacement du fichier.
func (s *Store) Path() string { return s.path }

// detach copie la liste des étudiants : sans cela, le groupe rendu partagerait
// sa tranche avec le magasin, et le modifier écrirait dans son dos.
func detach(classroom Classroom) Classroom {
	classroom.Students = append([]roster.Person(nil), classroom.Students...)
	return classroom
}

// List renvoie les groupes, classés par organisation puis par nom.
func (s *Store) List() []Classroom {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	copie := make([]Classroom, 0, len(s.items))
	for _, item := range s.items {
		copie = append(copie, detach(item))
	}
	sort.Slice(copie, func(i, j int) bool {
		if copie[i].Org != copie[j].Org {
			return strings.ToLower(copie[i].Org) < strings.ToLower(copie[j].Org)
		}
		return strings.ToLower(copie[i].Name) < strings.ToLower(copie[j].Name)
	})
	return copie
}

// Get retrouve un groupe par son identifiant.
func (s *Store) Get(id string) (Classroom, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, item := range s.items {
		if item.ID == id {
			return detach(item), true
		}
	}
	return Classroom{}, false
}

// Add déclare un groupe. Deux groupes ne peuvent pas viser le même préfixe dans
// la même organisation : ils désigneraient les mêmes dépôts.
func (s *Store) Add(classroom Classroom) (Classroom, error) {
	valide, err := classroom.Validate()
	if err != nil {
		return classroom, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, item := range s.items {
		if strings.EqualFold(item.Org, valide.Org) && strings.EqualFold(item.Scope(), valide.Scope()) {
			return classroom, valid.Errorf(
				"Un groupe couvre déjà « %s » dans « %s ».", valide.Label(), valide.Org)
		}
	}
	valide.ID = identifier()
	valide.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.items = append(s.items, detach(valide))
	return valide, s.saveLocked()
}

// Update remplace un groupe existant.
func (s *Store) Update(classroom Classroom) (Classroom, error) {
	valide, err := classroom.Validate()
	if err != nil {
		return classroom, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for position, item := range s.items {
		if item.ID != valide.ID {
			if strings.EqualFold(item.Org, valide.Org) && strings.EqualFold(item.Scope(), valide.Scope()) {
				return classroom, valid.Errorf(
					"Un autre groupe couvre déjà « %s » dans « %s ».", valide.Label(), valide.Org)
			}
			continue
		}
		valide.CreatedAt = item.CreatedAt
		s.items[position] = detach(valide)
	}
	for _, item := range s.items {
		if item.ID == valide.ID {
			return valide, s.saveLocked()
		}
	}
	return classroom, valid.Errorf("Groupe inconnu.")
}

// Delete retire un groupe de la liste. Aucun dépôt n'est touché sur GitHub.
func (s *Store) Delete(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	restants := make([]Classroom, 0, len(s.items))
	trouve := false
	for _, item := range s.items {
		if item.ID == id {
			trouve = true
			continue
		}
		restants = append(restants, item)
	}
	if !trouve {
		return valid.Errorf("Groupe inconnu.")
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
		document{Version: 1, Sessions: s.sessions, Classrooms: s.items}, "", "  ")
	if err != nil {
		return valid.Errorf("Groupes non enregistrés : %v", err)
	}
	if err := os.WriteFile(s.path, append(payload, '\n'), 0o600); err != nil {
		return valid.Errorf("Groupes non enregistrés : %v", err)
	}
	return nil
}

// identifier tire un identifiant de groupe stable, indépendant du préfixe : le
// renommer ne doit pas casser les adresses de l'interface.
func identifier() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(raw)
}
