package inspect

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/tokens"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Un profil d'inspection dit quoi regarder dans un travail d'un type donné.
//
// Sans lui, chacun doit réécrire à la main les mêmes exclusions à chaque
// analyse — et comme c'est fastidieux, personne ne le fait, et les rapports se
// remplissent de bruit. Avec lui, « Next.js — app/ » se choisit d'un clic et
// veut dire la même chose pour tout le monde.
//
// Les profils ci-dessous sont ceux que l'outil connaît d'office. Le registre de
// l'organisation en porte d'autres, écrits par l'équipe enseignante, et ils s'y
// ajoutent sans qu'il faille recompiler quoi que ce soit.

// Profile est un profil d'inspection.
type Profile struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Note dit en une phrase ce que le profil fait et pourquoi. L'interface
	// l'affiche à côté du choix : un profil dont on ne comprend pas l'effet ne
	// sera pas utilisé, ou sera utilisé de travers.
	Note string `json:"note,omitempty"`
	// Include restreint à ces chemins ; vide veut dire « tout ce qui reste ».
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
	// Languages restreint aux langages nommés ; vide veut dire « tous ».
	Languages []string `json:"languages,omitempty"`
}

// AllProfile est l'identifiant du profil qui ne restreint rien.
const AllProfile = "tout"

// BuiltIn énumère les profils connus d'office, dans l'ordre où les proposer.
var BuiltIn = []Profile{
	{
		ID: AllProfile, Label: "Tout le code",
		Note: "Tous les fichiers d'un langage reconnu, une fois retirés les " +
			"dépendances, les fichiers engendrés et les binaires.",
	},
	{
		ID: "code-seul", Label: "Le code seulement, sans la documentation",
		Note: "Comme « Tout le code », mais sans les fichiers Markdown. À " +
			"choisir quand les consignes sont recopiées dans chaque dépôt.",
		Exclude: []string{"*.md", "*.markdown"},
	},
	{
		ID: "next", Label: "Next.js — app/ et components/",
		Note: "Ce que l'étudiant a écrit, sans la configuration ni le " +
			"squelette engendré par « create-next-app ».",
		Include:   []string{"app/**", "src/app/**", "components/**", "src/components/**", "lib/**"},
		Exclude:   []string{"**/*.test.*", "**/*.spec.*", "public/**"},
		Languages: []string{"typescript", "tsx", "javascript", "css"},
	},
	{
		ID: "angular", Label: "Angular — src/app",
		Note:      "Composants, services et gabarits ; ni la configuration, ni les actifs.",
		Include:   []string{"src/app/**"},
		Exclude:   []string{"**/*.spec.ts"},
		Languages: []string{"typescript", "html", "css"},
	},
	{
		ID: "flutter", Label: "Flutter — lib/",
		Note:      "Le Dart écrit à la main ; ni android/, ni ios/, ni le code engendré.",
		Include:   []string{"lib/**", "test/**"},
		Exclude:   []string{"**/*.g.dart", "**/*.freezed.dart"},
		Languages: []string{"dart"},
	},
	{
		ID: "java", Label: "Java — src/main",
		Note:      "Les sources, sans les tests ni les ressources.",
		Include:   []string{"src/main/**", "src/**/*.java"},
		Exclude:   []string{"src/test/**"},
		Languages: []string{"java", "kotlin", "sql"},
	},
	{
		ID: "android", Label: "Android — app/src/main",
		Note: "Le code écrit à la main, quelle que soit la façon de le faire : " +
			"Java, Kotlin et Compose vivent tous sous « src/main », et les " +
			"gabarits XML avec eux. Ni les fichiers de compilation Gradle, ni " +
			"les ressources, ni ce que l'outillage engendre.",
		// Android Studio range le Kotlin sous « java/ » : c'est déroutant, mais
		// c'est ainsi, et un profil qui ne prendrait que « kotlin/ » ne
		// trouverait rien dans un projet ordinaire. Les deux chemins sont donc
		// nommés, et le second couvre les projets à plusieurs modules, où le
		// module ne s'appelle pas toujours « app ».
		Include: []string{
			"app/src/main/**", "**/src/main/java/**", "**/src/main/kotlin/**",
			"**/src/test/**", "**/src/androidTest/**",
		},
		Exclude: []string{
			// Engendré par la chaîne de compilation, jamais écrit à la main.
			"**/R.java", "**/R.kt", "**/BuildConfig.java", "**/BuildConfig.kt",
			"**/databinding/**", "**/generated/**",
			// Les ressources qui ne sont pas du code : couleurs, thèmes,
			// icônes, chaînes traduites. Les gabarits de « res/layout », eux,
			// sont écrits à la main et restent comparés.
			"**/res/values*/**", "**/res/drawable*/**", "**/res/mipmap*/**",
			"**/res/xml/**", "**/res/raw/**",
		},
		Languages: []string{"kotlin", "java", "xml"},
	},
	{
		ID: "aspnet", Label: "ASP.NET — Controllers, Models, Views",
		Note:      "Le C# et les vues écrits à la main ; ni wwwroot, ni le code engendré.",
		Include:   []string{"**/Controllers/**", "**/Models/**", "**/Views/**", "**/Pages/**", "**/Services/**"},
		Exclude:   []string{"wwwroot/**"},
		Languages: []string{"csharp", "html", "css", "sql"},
	},
	{
		ID: "python", Label: "Python — hors tests",
		Note:      "Les modules écrits à la main, sans les tests ni l'environnement.",
		Include:   []string{"**/*.py"},
		Exclude:   []string{"test_*.py", "*_test.py", "tests/**"},
		Languages: []string{"python", "sql"},
	},
}

// Catalog rassemble les profils connus d'office et ceux que l'organisation a
// déclarés. À identifiant égal, celui de l'organisation gagne : c'est le sien
// qu'elle a écrit, et c'est le sien qu'elle attend.
func Catalog(declared []Profile) []Profile {
	catalog := make([]Profile, 0, len(BuiltIn)+len(declared))
	replaced := map[string]Profile{}
	for _, profile := range declared {
		replaced[profile.ID] = profile
	}
	for _, profile := range BuiltIn {
		if custom, overridden := replaced[profile.ID]; overridden {
			catalog = append(catalog, custom)
			delete(replaced, profile.ID)
			continue
		}
		catalog = append(catalog, profile)
	}
	extra := make([]Profile, 0, len(replaced))
	for _, profile := range replaced {
		extra = append(extra, profile)
	}
	sort.Slice(extra, func(first, second int) bool { return extra[first].ID < extra[second].ID })
	return append(catalog, extra...)
}

// FindProfile retrouve un profil par son identifiant. Sans identifiant, c'est
// celui qui ne restreint rien.
func FindProfile(catalog []Profile, id string) (Profile, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		id = AllProfile
	}
	for _, profile := range catalog {
		if profile.ID == id {
			return profile, nil
		}
	}
	return Profile{}, valid.Errorf(
		"Profil d'inspection : « %s » est inconnu (connus : %s).",
		id, strings.Join(profileIDs(catalog), ", "))
}

func profileIDs(catalog []Profile) []string {
	ids := make([]string, 0, len(catalog))
	for _, profile := range catalog {
		ids = append(ids, profile.ID)
	}
	return ids
}

// ParseLanguages valide une liste de langages saisie à la main.
func ParseLanguages(names []string) ([]string, error) {
	cleaned := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if _, known := tokens.Get(name); !known {
			return nil, valid.Errorf(
				"Langage : « %s » n'est pas reconnu (reconnus : %s).",
				name, strings.Join(tokens.IDs(), ", "))
		}
		cleaned = append(cleaned, name)
	}
	return cleaned, nil
}
