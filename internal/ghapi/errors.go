package ghapi

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
)

// Error est une erreur renvoyée par l'API GitHub, enrichie du contexte utile
// à l'affichage. Le jeton n'y figure jamais.
type Error struct {
	Status  int
	Message string
	Details []string
	URL     string
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

// IsGitHub indique si l'erreur vient de l'API GitHub.
func IsGitHub(err error) bool {
	var target *Error
	return errors.As(err, &target)
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

	lowered := strings.ToLower(message)
	switch {
	case httpError.StatusCode == 401:
		message = "Jeton invalide ou expiré."
	case httpError.StatusCode == 403 && strings.Contains(lowered, "delete_repo"):
		message += " — la portée « delete_repo » est requise pour supprimer un dépôt " +
			"(gh auth refresh -s delete_repo)."
	case httpError.StatusCode == 403 && strings.Contains(lowered, "workflow"):
		message += " — la portée « workflow » est requise pour déposer des fichiers " +
			"dans .github/workflows (gh auth refresh -s workflow)."
	}

	url := ""
	if httpError.RequestURL != nil {
		url = httpError.RequestURL.String()
	}
	return &Error{Status: httpError.StatusCode, Message: message, Details: details, URL: url}
}

// asHTTPError isole errors.As pour la lecture des en-têtes d'une erreur go-gh.
func asHTTPError(err error, target **api.HTTPError) bool { return errors.As(err, target) }
