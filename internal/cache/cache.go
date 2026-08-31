// Package cache conserve sur disque les données GitHub peu changeantes
// (liste des dépôts d'une organisation, noms des profils).
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Durées de validité : la liste des dépôts bouge plus souvent qu'un nom de profil.
const (
	ReposTTL   = 6 * time.Hour
	OrgsTTL    = 12 * time.Hour
	ProfileTTL = 30 * 24 * time.Hour
	fileName   = "cache.json"
)

// ReposKey est la clé de la liste des dépôts d'une organisation.
func ReposKey(org string) string { return "repos:" + strings.ToLower(org) }

// OrgsKey est la clé des organisations accessibles à un compte.
func OrgsKey(viewer string) string { return "orgs:" + strings.ToLower(viewer) }

// ProfileKey est la clé du nom complet associé à un compte GitHub.
func ProfileKey(login string) string { return "profile:" + strings.ToLower(login) }

// legacyName est le nom du dossier de cache des versions précédentes de
// l'outil, écrites en Python : leur contenu a exactement la même forme.
const legacyName = "classroom"

// marker retient qu'un cache hérité a déjà été repris, pour qu'une purge
// volontaire ne soit pas défaite au lancement suivant.
const marker = "reprise-classroom"

// LegacyPath renvoie l'emplacement du cache de la version précédente de l'outil.
func LegacyPath() string {
	if base := os.Getenv("XDG_CACHE_HOME"); base != "" {
		return filepath.Join(base, legacyName, fileName)
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, legacyName, fileName)
}

// Dir renvoie l'emplacement du cache, conforme au standard du système.
func Dir() string {
	if base := os.Getenv("XDG_CACHE_HOME"); base != "" {
		return filepath.Join(base, "cohorte")
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(".", ".cache", "cohorte")
	}
	return filepath.Join(base, "cohorte")
}

type entry struct {
	At    float64         `json:"at"`
	Value json.RawMessage `json:"value"`
}

// Cache est un stockage clé-valeur sur disque, avec péremption par entrée.
// Tout tient dans un seul fichier JSON, ce qui rend la purge triviale et évite
// d'éparpiller des centaines de petits fichiers.
type Cache struct {
	Directory string
	Enabled   bool

	mutex   sync.Mutex
	entries map[string]entry
	loaded  bool
}

// New ouvre le cache du système, actif ou non.
func New(enabled bool) *Cache {
	return &Cache{Directory: Dir(), Enabled: enabled}
}

// NewIn ouvre un cache dans un dossier donné (utile aux tests).
func NewIn(directory string, enabled bool) *Cache {
	return &Cache{Directory: directory, Enabled: enabled}
}

// Path renvoie le chemin du fichier de cache.
func (c *Cache) Path() string { return filepath.Join(c.Directory, fileName) }

func (c *Cache) load() map[string]entry {
	if c.loaded {
		return c.entries
	}
	c.loaded = true
	c.entries = map[string]entry{}
	content, err := os.ReadFile(c.Path())
	if err != nil {
		return c.entries
	}
	if err := json.Unmarshal(content, &c.entries); err != nil {
		c.entries = map[string]entry{}
	}
	return c.entries
}

func (c *Cache) save() {
	if !c.Enabled {
		return
	}
	payload, err := json.Marshal(c.entries)
	if err != nil {
		return
	}
	if err := os.MkdirAll(c.Directory, 0o700); err != nil {
		return // un cache indisponible ne doit jamais interrompre le travail
	}
	// Écriture atomique : un remplacement rate moins qu'une réécriture.
	temporary := c.Path() + ".tmp"
	if err := os.WriteFile(temporary, payload, 0o600); err != nil {
		return
	}
	if err := os.Rename(temporary, c.Path()); err != nil {
		os.Remove(temporary)
		return
	}
	_ = os.Chmod(c.Path(), 0o600)
}

// Get lit une valeur encore valide et la décode dans target.
func (c *Cache) Get(key string, maxAge time.Duration, target any) bool {
	if !c.Enabled {
		return false
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	item, found := c.load()[key]
	if !found || len(item.Value) == 0 {
		return false
	}
	if time.Since(time.Unix(int64(item.At), 0)) > maxAge {
		return false
	}
	return json.Unmarshal(item.Value, target) == nil
}

// Set enregistre une valeur et l'horodate.
func (c *Cache) Set(key string, value any) {
	if !c.Enabled {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	c.load()[key] = entry{At: float64(time.Now().Unix()), Value: payload}
	c.save()
}

// SetMany enregistre plusieurs valeurs en une seule écriture.
func (c *Cache) SetMany(values map[string]any) {
	if !c.Enabled || len(values) == 0 {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	entries := c.load()
	stamp := float64(time.Now().Unix())
	for key, value := range values {
		payload, err := json.Marshal(value)
		if err != nil {
			continue
		}
		entries[key] = entry{At: stamp, Value: payload}
	}
	c.save()
}

// Forget oublie une entrée précise.
func (c *Cache) Forget(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	entries := c.load()
	if _, found := entries[key]; found {
		delete(entries, key)
		c.save()
	}
}

// Stats renvoie le nombre d'entrées et la taille du fichier, en octets.
func (c *Cache) Stats() (int, int64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	count := len(c.load())
	var size int64
	if info, err := os.Stat(c.Path()); err == nil {
		size = info.Size()
	}
	return count, size
}

// Describe résume l'état du cache en une ligne lisible.
func (c *Cache) Describe() string {
	count, size := c.Stats()
	if count == 0 {
		return "vide"
	}
	c.mutex.Lock()
	recent := time.Duration(0)
	first := true
	for _, item := range c.load() {
		age := time.Since(time.Unix(int64(item.At), 0))
		if first || age < recent {
			recent, first = age, false
		}
	}
	c.mutex.Unlock()
	return fmt.Sprintf("%d entrée(s), %.1f Kio, plus récente il y a %s",
		count, float64(size)/1024, Age(recent))
}

// Clear vide le cache et renvoie le nombre d'entrées supprimées.
func (c *Cache) Clear() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	count := len(c.load())
	c.entries = map[string]entry{}
	_ = os.Remove(c.Path())
	return count
}

// Adopt reprend le cache d'une version précédente de l'outil : les inventaires
// et les noms de profils déjà connus restent valables. L'ancien fichier n'est
// pas touché, et la reprise n'a lieu qu'une fois.
func (c *Cache) Adopt(legacyPath string) int {
	if !c.Enabled || legacyPath == "" {
		return 0
	}
	content, err := os.ReadFile(legacyPath)
	if err != nil {
		return 0
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if _, err := os.Stat(filepath.Join(c.Directory, marker)); err == nil {
		return 0 // déjà repris : une purge volontaire doit le rester
	}

	var inherited map[string]entry
	if err := json.Unmarshal(content, &inherited); err != nil {
		return 0
	}
	entries := c.load()
	adopted := 0
	for key, item := range inherited {
		// Ce qui a été appris depuis l'emporte sur ce qui a été hérité.
		if _, found := entries[key]; found || len(item.Value) == 0 {
			continue
		}
		entries[key] = item
		adopted++
	}
	c.save()
	if err := os.MkdirAll(c.Directory, 0o700); err == nil {
		_ = os.WriteFile(filepath.Join(c.Directory, marker), []byte(legacyPath+"\n"), 0o600)
	}
	return adopted
}

// Age met une durée sous une forme lisible.
func Age(duration time.Duration) string {
	seconds := duration.Seconds()
	switch {
	case seconds < 90:
		return fmt.Sprintf("%.0f s", seconds)
	case seconds < 5400:
		return fmt.Sprintf("%.0f min", seconds/60)
	case seconds < 172800:
		return fmt.Sprintf("%.0f h", seconds/3600)
	default:
		return fmt.Sprintf("%.0f j", seconds/86400)
	}
}
