package inspect

import "strings"

// Ce qui est écarté d'office, et qui ne se discute pas.
//
// Trente copies parties du même « create-next-app » partagent quarante mille
// fichiers que personne n'a écrits. Les comparer coûte le plus clair du temps
// de calcul, et remonte des ressemblances de cent pour cent qui ne veulent rien
// dire. La première chose qu'un comparateur doit savoir faire, c'est donc ne
// pas regarder.
//
// La liste ci-dessous est volontairement large : ce qu'elle écarte à tort est
// du code qu'aucun étudiant n'a écrit à la main, et son absence ne coûte rien.
// Ce qu'elle laisserait passer, en revanche, se paierait en bruit dans chaque
// rapport.

// Dossiers d'outillage : dépendances installées, sorties de compilation,
// caches, réglages d'éditeur. Rien de ce qui s'y trouve n'a été écrit pour ce
// travail.
var Directories = []string{
	".git", ".svn", ".hg",
	"node_modules", "bower_components", "jspm_packages", "vendor",
	"target", "build", "dist", "out", "bin", "obj", "Debug", "Release",
	".next", ".nuxt", ".svelte-kit", ".astro", ".angular", ".expo", ".parcel-cache",
	".dart_tool", ".pub-cache", ".flutter-plugins",
	".gradle", ".mvn", ".m2",
	"__pycache__", ".venv", "venv", ".tox", ".mypy_cache", ".pytest_cache",
	".ruff_cache", "site-packages", ".eggs",
	"Pods", "Carthage", "DerivedData", ".swiftpm",
	"packages", "bin-debug", "bin-release",
	"coverage", "htmlcov", ".nyc_output", "test-results", "playwright-report",
	".idea", ".vs", ".vscode", ".fleet", ".settings",
	".terraform", ".serverless", ".cache", ".turbo", ".yarn",
	"migrations", "Migrations",
}

// Fichiers de verrouillage : des dizaines de milliers de lignes engendrées par
// un gestionnaire de paquets, identiques chez tout le monde.
var Locks = []string{
	"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "npm-shrinkwrap.json",
	"bun.lockb", "bun.lock", "deno.lock",
	"composer.lock", "Gemfile.lock", "poetry.lock", "Pipfile.lock", "uv.lock",
	"pubspec.lock", "Cargo.lock", "go.sum", "packages.lock.json",
	"gradle.lockfile", "mix.lock", "flake.lock",
}

// Fichiers engendrés : écrits par un outil, jamais à la main.
var Generated = []string{
	"*.min.js", "*.min.css", "*.map", "*.bundle.js", "*.chunk.js",
	"*.g.dart", "*.freezed.dart", "*.gr.dart", "*.mocks.dart", "*.config.dart",
	"*.g.cs", "*.Designer.cs", "*.designer.cs", "*.generated.cs", "AssemblyInfo.cs",
	"*.generated.*", "*_pb2.py", "*_pb.js", "*.pb.go", "*.d.ts",
	"gradlew", "gradlew.bat", "mvnw", "mvnw.cmd",
	"*.lock", "*.iml", "*.suo", "*.user",
	".DS_Store", "Thumbs.db", "desktop.ini",
}

// Extensions binaires. L'extension est un indice, jamais une preuve : le
// contenu tranche aussi, plus bas. Mais elle évite de lire le fichier pour
// rien, et sur quarante mille fichiers cela compte.
//
// « .svg » y figure bien que ce soit du XML : c'est un actif graphique, exporté
// par un outil de dessin, pas quelque chose qu'on écrit.
var BinaryExtensions = []string{
	".ico", ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp", ".avif", ".tiff",
	".svg", ".psd", ".ai", ".eps",
	".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".ods",
	".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".jar", ".war", ".apk",
	".aab", ".ipa", ".dmg", ".iso",
	".class", ".o", ".obj", ".a", ".lib", ".dll", ".so", ".dylib", ".exe", ".pdb",
	".pyc", ".pyo", ".wasm", ".bin", ".dat",
	".woff", ".woff2", ".ttf", ".otf", ".eot",
	".mp3", ".wav", ".ogg", ".flac", ".m4a",
	".mp4", ".avi", ".mov", ".mkv", ".webm", ".wmv",
	".db", ".sqlite", ".sqlite3", ".mdb", ".realm",
	".keystore", ".jks", ".p12", ".pfx",
}

// Bornes de lecture d'un fichier.
const (
	// MaxFileBytes : au-delà, un fichier source n'a pas été écrit à la main.
	// Un fichier de deux méga-octets, c'est un jeu de données ou du code
	// engendré, et l'analyser coûterait autant que tout le reste du dépôt.
	MaxFileBytes = 2 << 20
	// SniffBytes est ce qu'on lit pour décider si un fichier est binaire.
	SniffBytes = 8 << 10
	// BinaryRatio est la part d'octets non imprimables au-delà de laquelle un
	// fichier est tenu pour binaire, même sans octet nul.
	BinaryRatio = 0.30
)

// Motifs d'exclusion par défaut, tous confondus.
func defaultPatterns() []string {
	patterns := make([]string, 0,
		len(Directories)+len(Locks)+len(Generated))
	patterns = append(patterns, Directories...)
	patterns = append(patterns, Locks...)
	patterns = append(patterns, Generated...)
	return patterns
}

// Binary dit si un contenu est binaire.
//
// L'octet nul tranche seul : aucun format texte n'en contient. À défaut, c'est
// la proportion d'octets de contrôle qui décide — un fichier texte n'en a
// pratiquement aucun, un fichier compressé en est plein.
func Binary(content []byte) bool {
	head := content
	if len(head) > SniffBytes {
		head = head[:SniffBytes]
	}
	if len(head) == 0 {
		return false
	}
	control := 0
	for _, octet := range head {
		if octet == 0 {
			return true
		}
		if octet < 0x09 || (octet > 0x0d && octet < 0x20) || octet == 0x7f {
			control++
		}
	}
	return float64(control)/float64(len(head)) > BinaryRatio
}

// BinaryName dit si un nom de fichier annonce un binaire.
func BinaryName(name string) bool {
	lowered := strings.ToLower(name)
	for _, extension := range BinaryExtensions {
		if strings.HasSuffix(lowered, extension) {
			return true
		}
	}
	return false
}
