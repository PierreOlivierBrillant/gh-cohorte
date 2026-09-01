// Package complete propose les chemins qui prolongent une saisie, pour la
// complétion par tabulation des questions attendant un fichier ou un dossier.
//
// Les candidats sont demandés au shell de la personne — bash, zsh ou fish —
// afin que la complétion se comporte comme celle à laquelle elle est habituée.
// Une implémentation native prend le relais dès que le shell est indisponible,
// trop lent, ou que la saisie contient des caractères qu'il vaut mieux ne pas
// lui confier.
package complete

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Mode indique ce qu'une question attend.
type Mode int

const (
	// None : aucune complétion.
	None Mode = iota
	// Path : fichiers et dossiers.
	Path
	// Dir : dossiers seulement.
	Dir
)

const (
	// maxSuggestions borne la liste : au-delà, elle n'aide plus.
	maxSuggestions = 60
	// shellTimeout borne l'appel au shell : la frappe ne doit jamais attendre.
	shellTimeout = 400 * time.Millisecond
	// cacheSize borne le cache mémoire des suggestions.
	cacheSize = 256
)

// Caractères qui feraient d'une saisie autre chose qu'un chemin aux yeux d'un
// shell : la complétion native s'en charge alors, sans lancer quoi que ce soit.
const unsafeCharacters = "$`;|&<>()\n\r\"'\\*?[]{}!#~="

var (
	cacheMutex sync.Mutex
	cache      = map[string][]string{}
)

// Suggest renvoie les chemins qui prolongent la saisie. Chaque suggestion
// commence par la saisie elle-même : les champs de texte complètent en ajoutant
// ce qui manque. Les dossiers sont suivis d'un séparateur — celui de la saisie
// quand le système en accepte plusieurs, celui du système sinon.
func Suggest(input string, mode Mode) []string {
	if mode == None {
		return nil
	}
	key := string(rune('0'+int(mode))) + "\x00" + input
	if cached, found := readCache(key); found {
		return cached
	}

	// Les candidats du shell sont complétés par ceux du système de fichiers :
	// selon les réglages de chacun, un shell peut être plus avare que l'autre,
	// et la liste finale est de toute façon filtrée et dédoublonnée.
	candidates, _ := shellCandidates(input, mode)
	candidates = append(candidates, nativeCandidates(input)...)
	suggestions := normalize(input, candidates, mode)
	writeCache(key, suggestions)
	return suggestions
}

// Forget vide le cache des suggestions (utile aux tests).
func Forget() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	cache = map[string][]string{}
}

func readCache(key string) ([]string, bool) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	value, found := cache[key]
	return value, found
}

func writeCache(key string, value []string) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	if len(cache) >= cacheSize {
		cache = map[string][]string{}
	}
	cache[key] = value
}

// ------------------------------------------------------------------- le shell

// shellName renvoie le shell à interroger, ou une chaîne vide.
func shellName() string {
	if os.Getenv("COHORTE_NO_SHELL_COMPLETION") != "" {
		return ""
	}
	name := filepath.Base(strings.TrimSpace(os.Getenv("SHELL")))
	switch name {
	case "bash", "zsh", "fish", "sh":
		return name
	default:
		return ""
	}
}

// safeForShell écarte les saisies qu'un shell interpréterait autrement.
func safeForShell(input string) bool {
	return !strings.ContainsAny(input, unsafeCharacters)
}

// shellCommand construit l'appel non interactif correspondant au shell.
// Le préfixe est toujours passé en argument, jamais concaténé au script.
func shellCommand(shell, prefix string, mode Mode) (string, []string) {
	switch shell {
	case "bash", "sh":
		script := `compgen -f -- "$1"`
		if mode == Dir {
			script = `compgen -d -- "$1"`
		}
		return "bash", []string{"-c", script, "bash", prefix}
	case "zsh":
		script := `setopt nullglob; print -rl -- ${~1}*`
		if mode == Dir {
			script = `setopt nullglob; print -rl -- ${~1}*(/)`
		}
		return "zsh", []string{"-c", script, "zsh", prefix}
	case "fish":
		// « complete -C » demande à fish les complétions d'une ligne de commande ;
		// « cat » suffit à obtenir celles d'un chemin.
		return "fish", []string{"-c", `complete -C "cat $argv[1]"`, prefix}
	default:
		return "", nil
	}
}

// shellCandidates demande ses candidats au shell ; le second retour indique
// si la réponse est exploitable.
func shellCandidates(input string, mode Mode) ([]string, bool) {
	shell := shellName()
	if shell == "" || !safeForShell(input) {
		return nil, false
	}
	// Le tilde est développé ici : les shells ne le font pas sur un argument.
	prefix, _ := expandHome(input)
	name, args := shellCommand(shell, prefix, mode)
	if name == "" {
		return nil, false
	}
	if _, err := exec.LookPath(name); err != nil {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	// Aucun fichier d'initialisation à charger : la complétion doit être rapide.
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.Output()
	if err != nil || ctx.Err() != nil {
		return nil, false
	}

	var candidates []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		// fish ajoute une description après une tabulation.
		if index := strings.IndexByte(line, '\t'); index >= 0 {
			line = line[:index]
		}
		if line = strings.TrimSpace(line); line != "" {
			candidates = append(candidates, line)
		}
	}
	return candidates, true
}

// ------------------------------------------------------------------ le natif

// nativeCandidates liste le dossier concerné, sans dépendre d'aucun shell.
func nativeCandidates(input string) []string {
	expanded, _ := expandHome(input)
	directory, prefix := filepath.Split(expanded)
	if directory == "" {
		directory = "."
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}

	var candidates []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		candidates = append(candidates, filepath.Join(directory, name))
	}
	return candidates
}

// ---------------------------------------------------------------- mise en forme

// normalize filtre, marque les dossiers et rétablit la forme saisie. Les règles
// s'appliquent quelle que soit la source des candidats.
func normalize(input string, candidates []string, mode Mode) []string {
	_, home := expandHome(input)
	// Les chemins produits reprennent le séparateur de la saisie : une
	// suggestion se substitue au champ, elle ne doit pas en réécrire le début.
	sep := separator(input)
	// Les fichiers cachés ne se proposent que si la saisie les appelle.
	_, typed := filepath.Split(input)
	wantsHidden := strings.HasPrefix(typed, ".")

	seen := map[string]bool{}
	var suggestions []string

	for _, candidate := range candidates {
		clean := trimSeparators(candidate)
		if clean == "" {
			continue
		}
		// « . » et « .. » n'apportent rien à une complétion de chemin.
		base := filepath.Base(clean)
		if base == "." || base == ".." {
			continue
		}
		if strings.HasPrefix(base, ".") && !wantsHidden {
			continue
		}
		info, err := os.Stat(clean)
		if err != nil {
			continue
		}
		if mode == Dir && !info.IsDir() {
			continue
		}
		display := withSeparator(clean, sep)
		if info.IsDir() {
			display += sep
		}
		// « ./ » saisi en tête doit être conservé : les suggestions prolongent
		// la saisie, elles ne la réécrivent pas.
		for _, marker := range []string{"." + sep, ".." + sep} {
			if strings.HasPrefix(input, marker) && !strings.HasPrefix(display, marker) {
				display = marker + display
				break
			}
		}
		// La saisie utilisait « ~ » : les suggestions doivent la prolonger.
		if home != "" {
			if rest, found := strings.CutPrefix(display, withSeparator(home, sep)); found {
				display = "~" + rest
			}
		}
		if !strings.HasPrefix(fold(display), fold(input)) {
			continue
		}
		if seen[display] {
			continue
		}
		seen[display] = true
		suggestions = append(suggestions, display)
	}

	sort.Strings(suggestions)
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}
	return suggestions
}

// expandHome développe « ~ » et renvoie le dossier personnel utilisé, s'il l'a été.
func expandHome(input string) (string, string) {
	rest, found := cutTilde(input)
	if !found {
		return input, ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return input, ""
	}
	if rest == "" {
		return home, home
	}
	return filepath.Join(home, rest), home
}

// cutTilde reconnaît « ~ » suivi d'un séparateur et renvoie ce qui suit.
// Windows acceptant les deux formes, « ~/… » comme « ~\… » sont développés.
func cutTilde(input string) (string, bool) {
	if input == "~" {
		return "", true
	}
	if len(input) > 1 && input[0] == '~' && os.IsPathSeparator(input[1]) {
		return input[2:], true
	}
	return "", false
}

// ------------------------------------------------------- les séparateurs

// separator choisit le séparateur des suggestions : celui que la personne a
// déjà tapé quand le système en accepte plusieurs, celui du système sinon.
func separator(input string) string {
	if os.PathSeparator == '/' {
		return "/"
	}
	// Sur Windows, « / » et « \ » ouvrent le même dossier : la suggestion
	// reprend la forme saisie plutôt que d'imposer la sienne.
	for index := len(input) - 1; index >= 0; index-- {
		if os.IsPathSeparator(input[index]) {
			return input[index : index+1]
		}
	}
	return string(os.PathSeparator)
}

// withSeparator réécrit avec le séparateur voulu un chemin assemblé par filepath.
func withSeparator(path, sep string) string {
	if sep == string(os.PathSeparator) {
		return path
	}
	return strings.ReplaceAll(path, string(os.PathSeparator), sep)
}

// trimSeparators retire les séparateurs de fin, quelle que soit leur forme.
func trimSeparators(path string) string {
	for len(path) > 0 && os.IsPathSeparator(path[len(path)-1]) {
		path = path[:len(path)-1]
	}
	return path
}

// fold ramène un chemin à la forme qui sert aux comparaisons : casse ignorée,
// et séparateurs unifiés là où le système les tient pour équivalents.
func fold(path string) string {
	return strings.ToLower(filepath.ToSlash(path))
}
