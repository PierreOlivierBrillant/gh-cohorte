// Package clone récupère les dépôts d'un groupe en local, ou met à jour des
// clones existants. Le jeton n'apparaît jamais dans une ligne de commande, dans
// une URL de dépôt ni dans .git/config : l'authentification est déléguée à
// « gh auth git-credential », le fournisseur d'identifiants de la CLI GitHub.
package clone

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
	"github.com/cli/go-gh/v2"
)

// Statuts possibles pour un dépôt traité.
const (
	Cloned  = "cloné"
	Updated = "mis à jour"
	Failed  = "échec"
	Skipped = "ignoré"
)

// DefaultJobs est le nombre de clonages menés de front.
const DefaultJobs = 4

// GitTimeout borne la durée d'une commande git.
const GitTimeout = 5 * time.Minute

// Result est l'issue du traitement d'un dépôt.
type Result struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Path   string `json:"path"`
	Error  string `json:"error"`
}

// IsFailed indique un échec.
func (r Result) IsFailed() bool { return r.Status == Failed }

// Target est un dépôt à cloner.
type Target struct {
	Name string
	URL  string
}

// Clone est un clone déjà présent sur le disque.
type Clone struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Cloner clone ou met à jour une série de dépôts en parallèle.
type Cloner struct {
	Jobs  int
	Depth int

	// git et credentialArgs sont extraits pour les tests ; en production, git
	// vient du PATH et les identifiants de gh.
	git             string
	credentialArgs  []string
	credentialsOnce sync.Once
}

// New construit un cloneur.
func New(jobs, depth int) *Cloner {
	if jobs < 1 {
		jobs = DefaultJobs
	}
	if depth < 0 {
		depth = 0
	}
	return &Cloner{Jobs: jobs, Depth: depth}
}

// EnsureGit vérifie que git est installé et renvoie son chemin.
func EnsureGit() (string, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return "", valid.Errorf(
			"git est introuvable : installez-le pour cloner " +
				"(pacman -S git, brew install git, ou git-scm.com sous Windows).")
	}
	return path, nil
}

// credentials renvoie les arguments qui branchent git sur le fournisseur
// d'identifiants de gh. Rien n'est écrit dans .git/config : ces réglages ne
// valent que pour la commande en cours.
func (c *Cloner) credentials() []string {
	c.credentialsOnce.Do(func() {
		path, err := gh.Path()
		if err != nil {
			return
		}
		c.credentialArgs = []string{
			"-c", "credential.helper=",
			"-c", fmt.Sprintf(`credential.helper=!%q auth git-credential`, path),
		}
	})
	return c.credentialArgs
}

// run exécute une commande git et transforme l'échec en erreur lisible.
func (c *Cloner) run(git string, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), GitTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, git, args...)
	// Sans cela, git pourrait bloquer en réclamant un mot de passe au clavier.
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	detail := ""
	for index := len(lines) - 1; index >= 0; index-- {
		if strings.TrimSpace(lines[index]) != "" {
			detail = strings.TrimSpace(lines[index])
			break
		}
	}
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("%s", detail)
}

// gitArgs assemble les arguments d'une commande, avec les identifiants de gh
// quand l'adresse est distante.
func (c *Cloner) gitArgs(url string, args ...string) []string {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return append(append([]string{}, c.credentials()...), args...)
	}
	return args
}

// PrepareDestination valide le dossier de destination et le crée au besoin.
func PrepareDestination(path string) (string, error) {
	expanded, err := roster.ExpandPath(path)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(expanded); err == nil && !info.IsDir() {
		return "", valid.Errorf("Destination : « %s » n'est pas un dossier.", expanded)
	}
	if err := os.MkdirAll(expanded, 0o755); err != nil {
		return "", valid.Errorf("Destination inutilisable : %v", err)
	}
	absolute, err := filepath.Abs(expanded)
	if err != nil {
		return "", valid.Errorf("Destination inutilisable : %v", err)
	}
	// Un dossier non accessible en écriture ferait échouer chaque clone.
	probe := filepath.Join(absolute, ".cohorte-ecriture")
	file, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return "", valid.Errorf("Destination : « %s » n'est pas accessible en écriture.", absolute)
	}
	file.Close()
	_ = os.Remove(probe)
	return absolute, nil
}

// FindClones liste les dépôts git présents directement sous un dossier.
func FindClones(directory string) ([]Clone, error) {
	expanded, err := roster.ExpandPath(directory)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(expanded)
	if err != nil {
		return nil, valid.Errorf("Dossier introuvable : %s", expanded)
	}
	var found []Clone
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(expanded, entry.Name())
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			continue
		}
		found = append(found, Clone{Name: entry.Name(), Path: path})
	}
	sort.Slice(found, func(i, j int) bool {
		return strings.ToLower(found[i].Name) < strings.ToLower(found[j].Name)
	})
	return found, nil
}

// Run clone les dépôts demandés, ou met à jour ceux déjà présents.
func (c *Cloner) Run(targets []Target, destination string,
	onDone func(done, total int, result Result)) ([]Result, error) {
	git, err := EnsureGit()
	if err != nil {
		return nil, err
	}
	return c.parallel(len(targets), onDone, func(position int) Result {
		return c.one(git, targets[position], destination)
	}), nil
}

// Update met à jour des clones existants, sans jamais écraser un travail local.
func (c *Cloner) Update(clones []Clone, onDone func(done, total int, result Result)) ([]Result, error) {
	git, err := EnsureGit()
	if err != nil {
		return nil, err
	}
	return c.parallel(len(clones), onDone, func(position int) Result {
		return c.pull(git, clones[position])
	}), nil
}

// parallel exécute une tâche par élément, au plus Jobs à la fois, et conserve
// l'ordre d'entrée dans les résultats.
func (c *Cloner) parallel(total int, onDone func(done, total int, result Result),
	work func(position int) Result) []Result {
	if total == 0 {
		return nil
	}
	workers := c.Jobs
	if workers > total {
		workers = total
	}
	results := make([]Result, total)
	queue := make(chan int)
	var mutex sync.Mutex
	var group sync.WaitGroup
	done := 0

	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for position := range queue {
				outcome := work(position)
				mutex.Lock()
				results[position] = outcome
				done++
				if onDone != nil {
					onDone(done, total, outcome)
				}
				mutex.Unlock()
			}
		}()
	}
	for position := 0; position < total; position++ {
		queue <- position
	}
	close(queue)
	group.Wait()
	return results
}

func (c *Cloner) one(git string, target Target, destination string) Result {
	path := filepath.Join(destination, target.Name)

	if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.IsDir() {
		// Déjà cloné : on rapatrie les nouveautés sans toucher au travail local.
		if err := c.run(git, c.gitArgs(target.URL, "-C", path, "pull", "--ff-only")); err != nil {
			return Result{Name: target.Name, Status: Failed, Path: path, Error: err.Error()}
		}
		return Result{Name: target.Name, Status: Updated, Path: path}
	}
	if entries, err := os.ReadDir(path); err == nil && len(entries) > 0 {
		return Result{Name: target.Name, Status: Skipped, Path: path,
			Error: "le dossier existe déjà et n'est pas un dépôt git"}
	}

	args := []string{"clone", "--quiet"}
	if c.Depth > 0 {
		args = append(args, "--depth", strconv.Itoa(c.Depth))
	}
	args = append(args, target.URL, path)
	if err := c.run(git, c.gitArgs(target.URL, args...)); err != nil {
		return Result{Name: target.Name, Status: Failed, Path: path, Error: err.Error()}
	}
	return Result{Name: target.Name, Status: Cloned, Path: path}
}

func (c *Cloner) pull(git string, item Clone) Result {
	if _, err := os.Stat(filepath.Join(item.Path, ".git")); err != nil {
		return Result{Name: item.Name, Status: Skipped, Path: item.Path,
			Error: "ce dossier n'est pas un dépôt git"}
	}
	// L'adresse d'origine est celle du clone : les identifiants de gh servent
	// toujours, l'URL n'ayant pas à être connue ici.
	args := append(append([]string{}, c.credentials()...), "-C", item.Path, "pull", "--ff-only")
	if err := c.run(git, args); err != nil {
		return Result{Name: item.Name, Status: Failed, Path: item.Path, Error: err.Error()}
	}
	return Result{Name: item.Name, Status: Updated, Path: item.Path}
}
