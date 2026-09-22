package clone

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultParent propose le dossier où ranger les clones tant que personne n'en
// a choisi : celui des téléchargements, à défaut l'accueil, à défaut le dossier
// courant. Le dossier courant n'est qu'un dernier recours, car c'est le hasard
// du terminal ou du raccourci qui a lancé l'outil, souvent un dépôt de travail.
//
// Rien n'est créé ici : ce n'est qu'une proposition, que la personne peut
// changer. PrepareDestination crée la destination retenue, à la confirmation.
func DefaultParent() string {
	return defaultParent(system{
		GOOS:     runtime.GOOS,
		Getenv:   os.Getenv,
		Home:     os.UserHomeDir,
		ReadFile: os.ReadFile,
		IsDir: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && info.IsDir()
		},
		KnownFolder: knownDownloads,
	})
}

// system rassemble ce que la recherche lit de la machine. Le séparer du vrai
// système permet d'éprouver les branches des trois plateformes depuis
// n'importe quel poste.
type system struct {
	GOOS     string
	Getenv   func(string) string
	Home     func() (string, error)
	ReadFile func(string) ([]byte, error)
	IsDir    func(string) bool
	// KnownFolder interroge FOLDERID_Downloads ; nil hors de Windows.
	KnownFolder func() (string, error)
}

// defaultParent renvoie le premier candidat qui existe. Chaque échec — une
// variable absente, un fichier illisible, un appel système refusé — passe
// simplement au suivant : une proposition ne vaut pas une erreur.
func defaultParent(sys system) string {
	home, err := sys.Home()
	if err != nil {
		home = ""
	}
	for _, candidate := range downloadCandidates(sys, home) {
		if candidate != "" && sys.IsDir(candidate) {
			return candidate
		}
	}
	if home != "" && sys.IsDir(home) {
		return home
	}
	return "."
}

// downloadCandidates liste, du plus précis au plus sûr, les emplacements
// possibles du dossier des téléchargements.
func downloadCandidates(sys system, home string) []string {
	var candidates []string
	inHome := func(name string) string {
		// Sans accueil, filepath.Join rendrait un chemin relatif au dossier
		// courant : exactement le hasard qu'on cherche à éviter.
		if home == "" {
			return ""
		}
		return filepath.Join(home, name)
	}
	switch sys.GOOS {
	case "windows":
		// Le dossier se déplace, sur un autre disque ou dans OneDrive : seul
		// le dossier connu dit où il est vraiment.
		if sys.KnownFolder != nil {
			if path, err := sys.KnownFolder(); err == nil {
				candidates = append(candidates, path)
			}
		}
		candidates = append(candidates, inHome("Downloads"))
	case "darwin":
		// Le Finder affiche « Téléchargements » grâce à un .localized, mais le
		// nom sur le disque reste anglais.
		candidates = append(candidates, inHome("Downloads"))
	default:
		candidates = append(candidates,
			xdgPath(sys.Getenv("XDG_DOWNLOAD_DIR"), home),
			userDirsDownload(sys, home),
			inHome("Downloads"))
	}
	return candidates
}

// userDirsDownload lit le dossier des téléchargements dans user-dirs.dirs, que
// xdg-user-dirs écrit dans la langue de la session : « Téléchargements »,
// « Descargas »… Un ~/Downloads codé en dur raterait la plupart des postes en
// français.
func userDirsDownload(sys system, home string) string {
	// La norme XDG tient pour invalide un XDG_CONFIG_HOME relatif.
	config := sys.Getenv("XDG_CONFIG_HOME")
	if !strings.HasPrefix(config, "/") {
		if home == "" {
			return ""
		}
		config = filepath.Join(home, ".config")
	}
	content, err := sys.ReadFile(filepath.Join(config, "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	// Le fichier est lu par un shell : la dernière affectation l'emporte.
	found := ""
	for _, line := range strings.Split(string(content), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "XDG_DOWNLOAD_DIR" {
			found = xdgPath(unquote(strings.TrimSpace(value)), home)
		}
	}
	return found
}

// xdgPath développe une valeur XDG. La norme n'en admet que deux formes,
// « $HOME/… » et un chemin absolu ; toute autre dépendrait du dossier courant.
func xdgPath(value, home string) string {
	switch {
	case value == "$HOME" || strings.HasPrefix(value, "$HOME/"):
		if home == "" {
			return ""
		}
		return filepath.Join(home, strings.TrimPrefix(value, "$HOME"))
	case strings.HasPrefix(value, "/"):
		return value
	}
	return ""
}

// unquote retire les guillemets d'une valeur de shell et ses échappements,
// qu'xdg-user-dirs place devant un guillemet ou un « $ » dans un nom.
func unquote(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	var builder strings.Builder
	escaped := false
	for _, char := range value {
		if char == '\\' && !escaped {
			escaped = true
			continue
		}
		escaped = false
		builder.WriteRune(char)
	}
	return builder.String()
}
