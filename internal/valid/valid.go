// Package valid regroupe la validation et la normalisation des saisies :
// comptes GitHub, noms complets, noms de dépôts et références « owner/repo ».
package valid

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Un compte GitHub : lettres et chiffres, tirets simples internes.
// Écrite sans anticipation, cette forme interdit d'elle-même le tiret en tête,
// en fin et les tirets consécutifs — RE2, le moteur de Go, ne connaît pas « (?=…) ».
var loginRe = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`)

// Un nom de dépôt GitHub : lettres, chiffres, point, tiret, souligné.
var repoNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// Une référence « owner/repo ».
var repoRefRe = regexp.MustCompile(`^([^/\s]+)/([^/\s]+)$`)

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

const (
	// MaxLoginLength est la longueur maximale d'un compte GitHub.
	MaxLoginLength = 39
	// MaxFullNameLength borne le nom complet d'une personne.
	MaxFullNameLength = 120
	// MaxSlugLength borne un fragment libre slugifié (identifiant de travail).
	MaxSlugLength = 60
)

// Error signale une saisie invalide ; son message est destiné à l'affichage.
type Error struct {
	Message string
}

func (e *Error) Error() string { return e.Message }

// Errorf construit une erreur de validation.
func Errorf(format string, args ...any) error {
	return &Error{Message: fmt.Sprintf(format, args...)}
}

// IsValidation indique si l'erreur vient d'une saisie invalide.
func IsValidation(err error) bool {
	if err == nil {
		return false
	}
	var target *Error
	return errorsAs(err, &target)
}

// Login valide un compte GitHub (personne ou organisation) et le renvoie nettoyé.
// « @octocat » et une URL de profil collée telle quelle sont acceptés.
func Login(value, label string) (string, error) {
	if label == "" {
		label = "Nom d'utilisateur GitHub"
	}
	login := strings.TrimSpace(value)
	login = strings.TrimPrefix(login, "@")
	if index := strings.Index(login, "github.com/"); index >= 0 {
		rest := strings.Trim(login[index+len("github.com/"):], "/")
		login = strings.SplitN(rest, "/", 2)[0]
	}
	login = strings.TrimSpace(login)
	if login == "" {
		return "", Errorf("%s : la valeur est vide.", label)
	}
	if len([]rune(login)) > MaxLoginLength {
		return "", Errorf("%s : « %s » dépasse %d caractères.", label, login, MaxLoginLength)
	}
	if !loginRe.MatchString(login) {
		return "", Errorf(
			"%s : « %s » est invalide (lettres, chiffres et tirets simples uniquement, "+
				"sans tiret au début ni à la fin).", label, login)
	}
	return login, nil
}

// FullName valide un nom complet : non vide, sans caractère de contrôle.
func FullName(value string) (string, error) {
	name := strings.Join(strings.Fields(value), " ")
	if name == "" {
		return "", Errorf("Nom complet : la valeur est vide.")
	}
	if len([]rune(name)) > MaxFullNameLength {
		runes := []rune(name)
		return "", Errorf("Nom complet : « %s… » dépasse %d caractères.",
			string(runes[:30]), MaxFullNameLength)
	}
	for _, char := range name {
		if unicode.IsControl(char) {
			return "", Errorf("Nom complet : caractères de contrôle interdits.")
		}
	}
	return name, nil
}

// RepoName valide un nom de dépôt GitHub.
func RepoName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", Errorf("Nom de dépôt : la valeur est vide.")
	}
	if name == "." || name == ".." {
		return "", Errorf("Nom de dépôt : « . » et « .. » sont réservés.")
	}
	if !repoNameRe.MatchString(name) {
		return "", Errorf(
			"Nom de dépôt : « %s » est invalide (lettres, chiffres, « . », « - » et « _ » "+
				"uniquement, 100 caractères max).", name)
	}
	return name, nil
}

// RepoRef valide une référence « owner/repo » et renvoie le couple correspondant.
func RepoRef(value string) (owner string, repo string, err error) {
	ref := strings.TrimSpace(value)
	for _, prefix := range []string{"https://github.com/", "http://github.com/", "github.com/"} {
		if strings.HasPrefix(ref, prefix) {
			ref = ref[len(prefix):]
			break
		}
	}
	ref = strings.Trim(ref, "/")
	ref = strings.TrimSuffix(ref, ".git")

	parts := repoRefRe.FindStringSubmatch(ref)
	if parts == nil {
		return "", "", Errorf(
			"Dépôt modèle : « %s » est invalide, format attendu « organisation/depot ».", value)
	}
	owner, err = Login(parts[1], "Propriétaire du dépôt modèle")
	if err != nil {
		return "", "", err
	}
	repo, err = RepoName(parts[2])
	if err != nil {
		return "", "", err
	}
	return owner, repo, nil
}

// Slugify transforme un texte libre en fragment utilisable dans un nom de dépôt :
// translittération ASCII, minuscules, tirets.
func Slugify(value string) string {
	// NFKD sépare les diacritiques du caractère de base, que l'on retire ensuite.
	chain := transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	folded, _, err := transform.String(chain, value)
	if err != nil {
		folded = value
	}
	var ascii strings.Builder
	for _, char := range strings.ToLower(folded) {
		if char < 128 {
			ascii.WriteRune(char)
		}
	}
	slug := nonAlphanumRe.ReplaceAllString(ascii.String(), "-")
	return strings.Trim(slug, "-")
}

// SlugFragment valide un fragment libre (identifiant de travail, préfixe) après slugification.
func SlugFragment(value, label string) (string, error) {
	slug := Slugify(value)
	if slug == "" {
		return "", Errorf(
			"%s : « %s » ne contient aucun caractère utilisable (lettres ou chiffres attendus).",
			label, value)
	}
	if len(slug) > MaxSlugLength {
		return "", Errorf("%s : « %s » dépasse %d caractères.", label, slug, MaxSlugLength)
	}
	return slug, nil
}
