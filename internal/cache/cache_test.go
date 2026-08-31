package cache_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
)

func TestSetGet(t *testing.T) {
	c := cache.NewIn(t.TempDir(), true)
	depots := []groups.RepoInfo{{Name: "tp1-a", Private: true}, {Name: "tp1-b"}}
	c.Set(cache.ReposKey("ACME"), depots)

	var relu []groups.RepoInfo
	if !c.Get(cache.ReposKey("acme"), cache.ReposTTL, &relu) {
		t.Fatal("la valeur devrait être en cache (clé insensible à la casse)")
	}
	if len(relu) != 2 || relu[0].Name != "tp1-a" {
		t.Fatalf("relu = %+v", relu)
	}
}

func TestGetPerime(t *testing.T) {
	dossier := t.TempDir()
	c := cache.NewIn(dossier, true)
	c.Set("profile:jlpicard", "Jean-Luc Picard")

	var nom string
	if !c.Get("profile:jlpicard", time.Hour, &nom) {
		t.Fatal("valeur absente")
	}
	if c.Get("profile:jlpicard", -time.Second, &nom) {
		t.Error("une entrée périmée ne doit pas être servie")
	}
}

func TestPersistanceEntreDeuxOuvertures(t *testing.T) {
	dossier := t.TempDir()
	premier := cache.NewIn(dossier, true)
	premier.Set("profile:emilie-cote", "Émilie Côté")

	second := cache.NewIn(dossier, true)
	var nom string
	if !second.Get("profile:emilie-cote", cache.ProfileTTL, &nom) || nom != "Émilie Côté" {
		t.Fatalf("relecture = %q", nom)
	}
}

func TestCacheDesactive(t *testing.T) {
	dossier := t.TempDir()
	c := cache.NewIn(dossier, false)
	c.Set("profile:x", "Nom")
	var nom string
	if c.Get("profile:x", cache.ProfileTTL, &nom) {
		t.Error("un cache désactivé ne doit rien servir")
	}
	if _, err := os.Stat(filepath.Join(dossier, "cache.json")); !os.IsNotExist(err) {
		t.Error("un cache désactivé ne doit rien écrire")
	}
}

func TestPermissionsDuFichier(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions POSIX")
	}
	dossier := t.TempDir()
	c := cache.NewIn(dossier, true)
	c.Set("k", "v")
	info, err := os.Stat(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("permissions %o, attendu 600", mode)
	}
}

func TestSetManyEtStats(t *testing.T) {
	c := cache.NewIn(t.TempDir(), true)
	c.SetMany(map[string]any{
		cache.ProfileKey("a"): "Personne A",
		cache.ProfileKey("b"): "", // un profil sans nom est mémorisé aussi
	})
	nombre, taille := c.Stats()
	if nombre != 2 || taille == 0 {
		t.Fatalf("Stats = %d, %d", nombre, taille)
	}
	var vide string
	if !c.Get(cache.ProfileKey("b"), cache.ProfileTTL, &vide) || vide != "" {
		t.Error("un nom vide doit rester mémorisé")
	}
	if description := c.Describe(); description == "vide" {
		t.Errorf("Describe = %q", description)
	}
}

func TestForgetEtClear(t *testing.T) {
	c := cache.NewIn(t.TempDir(), true)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Forget("a")
	var valeur int
	if c.Get("a", time.Hour, &valeur) {
		t.Error("l'entrée oubliée ne doit plus être servie")
	}
	if supprimees := c.Clear(); supprimees != 1 {
		t.Errorf("Clear = %d, attendu 1", supprimees)
	}
	if _, err := os.Stat(c.Path()); !os.IsNotExist(err) {
		t.Error("le fichier de cache doit disparaître")
	}
	if description := c.Describe(); description != "vide" {
		t.Errorf("Describe = %q", description)
	}
}

func TestFichierCorrompu(t *testing.T) {
	dossier := t.TempDir()
	if err := os.WriteFile(filepath.Join(dossier, "cache.json"), []byte("pas du JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := cache.NewIn(dossier, true)
	var valeur string
	if c.Get("k", time.Hour, &valeur) {
		t.Error("un cache corrompu doit se comporter comme un cache vide")
	}
	c.Set("k", "v")
	if !c.Get("k", time.Hour, &valeur) || valeur != "v" {
		t.Error("le cache doit redevenir utilisable après corruption")
	}
}

func TestNeConserveQueLesChampsUtiles(t *testing.T) {
	dossier := t.TempDir()
	c := cache.NewIn(dossier, true)
	c.Set(cache.ReposKey("acme"), []groups.RepoInfo{
		{Name: "tp1-a", Private: true, HTMLURL: "https://github.com/acme/tp1-a", PushedAt: "2026-08-01T00:00:00Z"},
	})
	contenu, err := os.ReadFile(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	var brut map[string]struct {
		Value []map[string]json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(contenu, &brut); err != nil {
		t.Fatal(err)
	}
	champs := brut[cache.ReposKey("acme")].Value[0]
	if len(champs) != 4 {
		t.Errorf("%d champ(s) conservé(s) : %+v", len(champs), champs)
	}
}

// heritage écrit un cache au format de la version précédente de l'outil.
func heritage(t *testing.T, dossier string, entrees string) string {
	t.Helper()
	racine := filepath.Join(dossier, "classroom")
	if err := os.MkdirAll(racine, 0o700); err != nil {
		t.Fatal(err)
	}
	chemin := filepath.Join(racine, "cache.json")
	if err := os.WriteFile(chemin, []byte(entrees), 0o600); err != nil {
		t.Fatal(err)
	}
	return chemin
}

func TestAdoptReprendLeCacheHerite(t *testing.T) {
	dossier := t.TempDir()
	maintenant := float64(time.Now().Unix())
	ancien := heritage(t, dossier, `{
      "repos:acme": {"at": `+itoa(maintenant)+`, "value": [{"name": "tp1-a", "private": true}]},
      "profile:jlpicard": {"at": `+itoa(maintenant)+`, "value": "Jean-Luc Picard"}
    }`)

	c := cache.NewIn(filepath.Join(dossier, "cohorte"), true)
	if repris := c.Adopt(ancien); repris != 2 {
		t.Fatalf("Adopt = %d, attendu 2", repris)
	}

	var depots []groups.RepoInfo
	if !c.Get(cache.ReposKey("acme"), cache.ReposTTL, &depots) || len(depots) != 1 {
		t.Errorf("inventaire reprisé = %+v", depots)
	}
	var nom string
	if !c.Get(cache.ProfileKey("jlpicard"), cache.ProfileTTL, &nom) || nom != "Jean-Luc Picard" {
		t.Errorf("nom reprisé = %q", nom)
	}
	// L'ancien fichier n'est jamais touché : l'outil précédent continue de servir.
	if _, err := os.Stat(ancien); err != nil {
		t.Errorf("l'ancien cache a disparu : %v", err)
	}
}

func TestAdoptConserveLHorodatage(t *testing.T) {
	dossier := t.TempDir()
	// Une entrée déjà périmée le reste après la reprise.
	vieux := float64(time.Now().Add(-48 * time.Hour).Unix())
	ancien := heritage(t, dossier, `{"repos:acme": {"at": `+itoa(vieux)+`, "value": [{"name": "tp1-a"}]}}`)

	c := cache.NewIn(filepath.Join(dossier, "cohorte"), true)
	if repris := c.Adopt(ancien); repris != 1 {
		t.Fatalf("Adopt = %d", repris)
	}
	var depots []groups.RepoInfo
	if c.Get(cache.ReposKey("acme"), cache.ReposTTL, &depots) {
		t.Error("une entrée héritée périmée ne doit pas être servie")
	}
	if !c.Get(cache.ReposKey("acme"), 72*time.Hour, &depots) {
		t.Error("l'entrée doit tout de même avoir été reprise")
	}
}

func TestAdoptNEcrasePasCeQuiEstConnu(t *testing.T) {
	dossier := t.TempDir()
	maintenant := float64(time.Now().Unix())
	ancien := heritage(t, dossier, `{"profile:jlpicard": {"at": `+itoa(maintenant)+`, "value": "Ancien Nom"}}`)

	c := cache.NewIn(filepath.Join(dossier, "cohorte"), true)
	c.Set(cache.ProfileKey("jlpicard"), "Jean-Luc Picard")
	if repris := c.Adopt(ancien); repris != 0 {
		t.Fatalf("Adopt = %d, rien ne devait être repris", repris)
	}
	var nom string
	if !c.Get(cache.ProfileKey("jlpicard"), cache.ProfileTTL, &nom) || nom != "Jean-Luc Picard" {
		t.Errorf("nom = %q", nom)
	}
}

func TestAdoptNeSeRepetePas(t *testing.T) {
	dossier := t.TempDir()
	maintenant := float64(time.Now().Unix())
	ancien := heritage(t, dossier, `{"profile:a": {"at": `+itoa(maintenant)+`, "value": "Personne A"}}`)
	racine := filepath.Join(dossier, "cohorte")

	premier := cache.NewIn(racine, true)
	if repris := premier.Adopt(ancien); repris != 1 {
		t.Fatalf("Adopt = %d", repris)
	}
	// Une purge volontaire ne doit pas être défaite au lancement suivant.
	premier.Clear()

	second := cache.NewIn(racine, true)
	if repris := second.Adopt(ancien); repris != 0 {
		t.Errorf("Adopt = %d : le cache vidé ne doit pas se remplir tout seul", repris)
	}
	var nom string
	if second.Get(cache.ProfileKey("a"), cache.ProfileTTL, &nom) {
		t.Error("le cache devrait être resté vide")
	}
}

func TestAdoptSansHeritage(t *testing.T) {
	dossier := t.TempDir()
	c := cache.NewIn(filepath.Join(dossier, "cohorte"), true)
	if repris := c.Adopt(filepath.Join(dossier, "absent.json")); repris != 0 {
		t.Errorf("Adopt = %d", repris)
	}
	// Rien ne doit être créé quand il n'y a rien à reprendre.
	if _, err := os.Stat(filepath.Join(dossier, "cohorte")); !os.IsNotExist(err) {
		t.Error("aucun dossier ne devait être créé")
	}
}

func TestAdoptFichierCorrompu(t *testing.T) {
	dossier := t.TempDir()
	ancien := heritage(t, dossier, "ceci n'est pas du JSON")
	c := cache.NewIn(filepath.Join(dossier, "cohorte"), true)
	if repris := c.Adopt(ancien); repris != 0 {
		t.Errorf("Adopt = %d", repris)
	}
}

func TestAdoptCacheDesactive(t *testing.T) {
	dossier := t.TempDir()
	maintenant := float64(time.Now().Unix())
	ancien := heritage(t, dossier, `{"profile:a": {"at": `+itoa(maintenant)+`, "value": "Personne A"}}`)
	c := cache.NewIn(filepath.Join(dossier, "cohorte"), false)
	if repris := c.Adopt(ancien); repris != 0 {
		t.Errorf("Adopt = %d avec --no-cache", repris)
	}
}

func TestLegacyPathRespecteXDG(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg-essai")
	attendu := filepath.Join("/tmp/xdg-essai", "classroom", "cache.json")
	if chemin := cache.LegacyPath(); chemin != attendu {
		t.Errorf("LegacyPath = %q, attendu %q", chemin, attendu)
	}
}

// itoa met un horodatage sous une forme utilisable dans du JSON.
func itoa(value float64) string {
	return strconv.FormatFloat(value, 'f', 0, 64)
}
