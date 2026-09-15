package inspect

import (
	"path"
	"strings"
)

// Les motifs suivent la convention de « .gitignore », parce que c'est celle que
// tout le monde connaît déjà et qu'en inventer une autre obligerait chacun à
// l'apprendre.
//
// Un motif sans barre oblique porte sur un nom : « node_modules » écarte ce
// dossier où qu'il soit, « *.min.js » écarte ces fichiers où qu'ils soient. Un
// motif avec barre oblique porte sur le chemin entier, et « ** » y traverse
// autant de niveaux qu'il faut : « app/**/*.tsx » est du TSX quelque part sous
// « app ».

// Matches dit si un chemin — séparateurs POSIX — répond à l'un des motifs.
func Matches(patterns []string, name string) (string, bool) {
	for _, pattern := range patterns {
		if Match(pattern, name) {
			return pattern, true
		}
	}
	return "", false
}

// Match dit si un chemin répond à un motif.
func Match(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	name = strings.Trim(name, "/")
	if pattern == "" || name == "" {
		return false
	}
	// Une barre oblique finale ne désigne qu'un dossier : « build/ ». Le nom
	// d'un fichier n'en porte jamais, et c'est le dossier qui doit répondre.
	pattern = strings.TrimSuffix(pattern, "/")

	if !strings.Contains(pattern, "/") {
		// Un nom seul : chaque niveau du chemin peut y répondre, dossiers
		// compris. C'est ce qui fait qu'écrire « vendor » suffit.
		for _, segment := range strings.Split(name, "/") {
			if ok, _ := path.Match(pattern, segment); ok {
				return true
			}
		}
		return false
	}
	pattern = strings.TrimPrefix(pattern, "/")
	return segments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

// segments apparie motif et chemin, niveau par niveau.
//
// « ** » est le seul cas qui demande de revenir en arrière : il peut n'avaler
// aucun niveau comme il peut tous les avaler, et rien ne dit d'avance lequel
// est le bon. On essaie donc, du plus court au plus long.
func segments(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			// « ** » en fin de motif prend tout le reste — « app/** » est tout
			// ce qui se trouve sous « app ».
			if len(pattern) == 1 {
				return true
			}
			for skip := 0; skip <= len(name); skip++ {
				if segments(pattern[1:], name[skip:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, _ := path.Match(pattern[0], name[0]); !ok {
			return false
		}
		pattern, name = pattern[1:], name[1:]
	}
	return len(name) == 0
}
