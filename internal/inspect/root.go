package inspect

import (
	"path"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/tokens"
)

// Deux étudiants rendent le même travail : l'un met son projet à la racine du
// dépôt, l'autre le range sous « tp1/mon_tp ». Rien ne les distingue sur le
// fond, mais un profil qui dit « n'inspecte que app/ » ne trouve rien chez le
// second. C'est le genre de détail qui fait qu'un outil de comparaison rend un
// rapport à moitié vide sans jamais dire pourquoi.
//
// La racine du projet est donc cherchée avant que les motifs ne s'appliquent :
// tant qu'un niveau ne contient qu'un dossier et rien qui ressemble à un
// projet, on descend. On s'arrête dès qu'on voit du code ou un fichier de
// projet — c'est là que le projet commence.
//
// Deviner se trompe parfois. C'est pourquoi la racine retenue est rendue avec
// la sélection : l'interface la montre, et laisse la corriger.

// MaxDescent borne la descente. Au-delà de trois niveaux, ce n'est plus un
// dossier d'emballage, c'est une arborescence que personne n'a voulue.
const MaxDescent = 3

// Structural nomme les dossiers qui appartiennent à la structure d'un projet
// plutôt qu'à son emballage. Descendre dedans reviendrait à effacer ce que les
// profils d'inspection désignent.
var Structural = map[string]bool{
	"src": true, "app": true, "lib": true, "main": true, "source": true,
	"sources": true, "test": true, "tests": true, "include": true,
	"pages": true, "components": true, "core": true, "assets": true,
	"public": true, "static": true, "docs": true,
}

// Markers nomment les fichiers qui disent « le projet commence ici ».
var Markers = []string{
	"package.json", "tsconfig.json", "deno.json", "bun.lockb",
	"pubspec.yaml", "pom.xml", "build.gradle", "build.gradle.kts",
	"settings.gradle", "settings.gradle.kts",
	"*.csproj", "*.sln", "*.fsproj",
	"pyproject.toml", "requirements.txt", "setup.py", "Pipfile",
	"Cargo.toml", "go.mod", "composer.json", "Gemfile",
	"CMakeLists.txt", "Makefile", "Dockerfile", "docker-compose.yml",
	"index.html", "manifest.json",
}

// Root devine la racine du projet dans une liste de chemins.
//
// On descend niveau par niveau, jamais d'un coup jusqu'au dossier commun à
// tous les fichiers. La différence est décisive : un projet Java range tout
// sous « src/main/java/ca/cegep/tp1 », et couper au plus profond ferait de
// « Main.java » un fichier posé à la racine — aucun profil qui parle de
// « src/main » ne le retrouverait. La descente s'arrête donc où le projet
// commence, pas où les fichiers se séparent.
func Root(paths []string) string {
	root := ""
	// Un niveau de plus que MaxDescent : le premier est le préfixe que toute
	// archive de GitHub ajoute — « organisation-depot-sha » —, et il ne compte
	// pas comme un emballage voulu par l'étudiant.
	for depth := 0; depth <= MaxDescent; depth++ {
		only, deeper := singleDirectory(paths, root)
		if !deeper {
			break
		}
		root = path.Join(root, only)
	}
	return root
}

// Strip rend le chemin d'un fichier vu depuis la racine du projet. Un fichier
// qui n'est pas sous cette racine garde le sien : l'écarter en silence
// reviendrait à perdre le README d'un dépôt dont le projet est rangé plus bas.
func Strip(root, name string) string {
	if root == "" {
		return name
	}
	if trimmed, under := strings.CutPrefix(name, root+"/"); under {
		return trimmed
	}
	return name
}

// singleDirectory dit si un niveau ne contient qu'un dossier et rien qui
// annonce un projet — auquel cas il faut descendre dedans.
func singleDirectory(paths []string, root string) (string, bool) {
	directories := map[string]bool{}
	only := ""
	for _, name := range paths {
		rest := Strip(root, name)
		if root != "" && rest == name {
			continue // pas sous cette racine
		}
		head, _, nested := strings.Cut(rest, "/")
		if nested && Structural[strings.ToLower(head)] {
			// « src », « app », « lib » ne sont pas des emballages : ce sont
			// les dossiers du projet lui-même, et les traverser ferait perdre
			// la structure sur laquelle les profils portent.
			return "", false
		}
		if !nested {
			// Un fichier posé à ce niveau : s'il annonce un projet ou s'il est
			// du code, la racine est ici.
			if marker(head) {
				return "", false
			}
			continue
		}
		directories[head] = true
		only = head
		if len(directories) > 1 {
			return "", false
		}
	}
	if len(directories) != 1 {
		return "", false
	}
	return only, true
}

// marker dit qu'un fichier annonce le début d'un projet : un fichier de projet,
// ou simplement du code dans un langage connu.
//
// La documentation n'annonce rien. Un dépôt qui porte un « README.md » à sa
// racine et son projet sous « tp1/ » est le cas le plus courant qui soit ; s'y
// arrêter laisserait chaque profil d'inspection les mains vides. Le README
// n'est pas perdu pour autant : il garde son chemin et reste comparable, il ne
// sert simplement pas à décider où le projet commence.
func marker(name string) bool {
	if language, code := tokens.Detect(name); code && language.ID != "markdown" {
		return true
	}
	_, found := Matches(Markers, name)
	return found
}
