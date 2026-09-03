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

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

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
	store.items = lu.Classrooms
	return store
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
	payload, err := json.MarshalIndent(document{Version: 1, Classrooms: s.items}, "", "  ")
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
