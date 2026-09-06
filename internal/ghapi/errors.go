package ghapi

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/scopes"
	"github.com/cli/go-gh/v2/pkg/api"
)

// Error est une erreur renvoyée par l'API GitHub, enrichie du contexte utile
// à l'affichage. Le jeton n'y figure jamais.
type Error struct {
	Status  int
	Message string
	Details []string
	URL     string
	// Scope nomme la portée qui manque au jeton, quand GitHub l'a fait savoir.
	// C'est elle qui permet de proposer un renouvellement plutôt qu'un refus.
	Scope string
}

func (e *Error) Error() string {
	message := e.Message
	if len(e.Details) > 0 {
		message += " (" + strings.Join(e.Details, " ; ") + ")"
	}
	if e.Status == 0 {
		return message
	}
	return fmt.Sprintf("HTTP %d — %s", e.Status, message)
}

// StatusOf renvoie le code HTTP d'une erreur GitHub, ou 0.
func StatusOf(err error) int {
	var target *Error
	if errors.As(err, &target) {
		return target.Status
	}
	return 0
}

// MissingScope renvoie la portée qui manquait au jeton, ou une chaîne vide si
// l'échec vient d'autre chose.
func MissingScope(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Scope
	}
	return ""
}

// IsGitHub indique si l'erreur vient de l'API GitHub.
func IsGitHub(err error) bool {
	var target *Error
	return errors.As(err, &target)
}

// GitHub cite la portée qui lui manque entre accents graves : « refusing to
// allow an OAuth App to create or update workflow ... without `workflow` scope ».
var quotedScope = regexp.MustCompile("`([a-z][a-z0-9_:]*)`\\s+scope")

// missingScope désigne la portée absente du jeton, d'après ce que la réponse en
// dit. Les en-têtes tranchent : GitHub y annonce les portées acceptées par le
// point d'API et celles que le jeton porte. Le message ne sert que de recours.
func missingScope(header http.Header, message string) string {
	accepted := scopes.Parse(header.Get("X-Accepted-Oauth-Scopes"))
	if len(accepted) > 0 {
		granted := scopes.Parse(header.Get("X-Oauth-Scopes"))
		for _, name := range accepted {
			if scopes.Has(granted, name) {
				return ""
			}
		}
		return accepted[0]
	}
	if match := quotedScope.FindStringSubmatch(message); match != nil {
		return match[1]
	}
	// Certaines réponses se contentent de nommer la portée en clair.
	for _, scope := range scopes.Catalog {
		if !scope.Minimal && strings.Contains(message, scope.Name) {
			return scope.Name
		}
	}
	return ""
}

// convert traduit une erreur de go-gh en erreur affichable, en français.
func convert(err error) error {
	if err == nil {
		return nil
	}
	var httpError *api.HTTPError
	if !errors.As(err, &httpError) {
		return &Error{Message: fmt.Sprintf("Connexion impossible : %v", err)}
	}

	message := httpError.Message
	if message == "" {
		message = "Erreur inattendue"
	}
	var details []string
	for _, item := range httpError.Errors {
		reason := item.Message
		if reason == "" {
			reason = item.Code
		}
		if reason == "" {
			continue
		}
		field := item.Field
		if field == "" {
			field = item.Resource
		}
		if field != "" {
			details = append(details, field+" : "+reason)
		} else {
			details = append(details, reason)
		}
	}

	scope := ""
	switch {
	case httpError.StatusCode == 401:
		message = "Jeton invalide ou expiré."
	case httpError.StatusCode == 403:
		if scope = missingScope(httpError.Headers, strings.ToLower(message)); scope != "" {
			// La commande reste écrite en toutes lettres : elle est la seule
			// issue quand l'outil tourne sans personne pour répondre.
			message += fmt.Sprintf(" — la portée « %s » est requise "+
				"(gh auth refresh -s %s).", scope, scope)
		}
	}

	url := ""
	if httpError.RequestURL != nil {
		url = httpError.RequestURL.String()
	}
	return &Error{
		Status: httpError.StatusCode, Message: message,
		Details: details, URL: url, Scope: scope,
	}
}

// asHTTPError isole errors.As pour la lecture des en-têtes d'une erreur go-gh.
func asHTTPError(err error, target **api.HTTPError) bool { return errors.As(err, target) }
