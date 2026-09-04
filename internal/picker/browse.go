package picker

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// L'explorateur de repli. Quand le système n'a pas de fenêtre à offrir — un
// serveur sans session graphique, une distribution sans zenity —, l'interface
// en montre un à elle. Il ne sait rien faire de particulier : lister un
// dossier, et dire par où l'on est passé. C'est peu, mais cela marche partout.

// Entry est une entrée d'un dossier.
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

// Listing est le contenu d'un dossier, avec de quoi remonter.
type Listing struct {
	// Path est le dossier lu, en absolu.
	Path string `json:"path"`
	// Parent est le dossier au-dessus, vide à la racine.
	Parent string `json:"parent"`
	// Home est le dossier personnel, pour y revenir d'un geste.
	Home string `json:"home"`
	// Separator est celui de la plateforme, pour l'affichage.
	Separator string  `json:"separator"`
	Entries   []Entry `json:"entries"`
	// Truncated dit qu'un dossier trop fourni n'a pas été montré en entier.
	Truncated bool `json:"truncated"`
}

// maxEntries borne une liste : un dossier de dix mille fichiers n'aide personne,
// et la page n'a pas à les afficher.
const maxEntries = 500

// Browse liste un dossier. Un chemin vide part du dossier personnel, un chemin
// de fichier donne son dossier, et un chemin disparu remonte jusqu'à ce qui
// existe encore — ouvrir l'explorateur ne doit jamais échouer sur un souvenir.
func Browse(path string, dirsOnly bool) (Listing, error) {
	dossier := depart(Request{Start: path})
	entrees, err := os.ReadDir(dossier)
	if err != nil {
		return Listing{}, err
	}

	listing := Listing{
		Path:      dossier,
		Home:      maison(),
		Separator: string(filepath.Separator),
		Entries:   make([]Entry, 0, len(entrees)),
	}
	if parent := filepath.Dir(dossier); parent != dossier {
		listing.Parent = parent
	}

	for _, entree := range entrees {
		// Les fichiers cachés encombrent sans servir : un chemin connu se tape.
		if strings.HasPrefix(entree.Name(), ".") {
			continue
		}
		estDossier := entree.IsDir()
		if lien := entree.Type()&os.ModeSymlink != 0; lien {
			// Un lien vers un dossier se parcourt comme un dossier.
			if info, err := os.Stat(filepath.Join(dossier, entree.Name())); err == nil {
				estDossier = info.IsDir()
			}
		}
		if dirsOnly && !estDossier {
			continue
		}
		listing.Entries = append(listing.Entries, Entry{
			Name: entree.Name(),
			Path: filepath.Join(dossier, entree.Name()),
			Dir:  estDossier,
		})
	}

	// Les dossiers d'abord, puis l'ordre alphabétique en ignorant la casse :
	// c'est ce que font les explorateurs, et l'œil s'y retrouve.
	sort.SliceStable(listing.Entries, func(i, j int) bool {
		if listing.Entries[i].Dir != listing.Entries[j].Dir {
			return listing.Entries[i].Dir
		}
		return strings.ToLower(listing.Entries[i].Name) <
			strings.ToLower(listing.Entries[j].Name)
	})
	if len(listing.Entries) > maxEntries {
		listing.Entries = listing.Entries[:maxEntries]
		listing.Truncated = true
	}
	return listing, nil
}

func maison() string {
	if chemin, err := os.UserHomeDir(); err == nil {
		return chemin
	}
	return ""
}
