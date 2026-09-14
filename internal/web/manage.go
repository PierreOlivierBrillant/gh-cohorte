package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// target résout l'organisation et le dépôt désignés par l'adresse.
func target(request *http.Request) (string, string, error) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		return "", "", err
	}
	repo, err := valid.RepoName(request.PathValue("repo"))
	if err != nil {
		return "", "", err
	}
	return org, repo, nil
}

// handleAccess renvoie les accès d'un dépôt. Un panneau ouvert sur un dépôt
// précis relit toujours : c'est là qu'on vient vérifier ce qu'on vient de
// changer, et une mémoire d'une heure y dirait le contraire de la vérité. Ce
// qu'il lit est mémorisé pour le reste de l'écran.
func (s *Server) handleAccess(writer http.ResponseWriter, request *http.Request) {
	org, repo, err := target(request)
	if err != nil {
		fail(writer, err)
		return
	}
	acces, err := s.resolver(org).AccessOf(org, repo, identity.Refresh)
	if err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, acces)
}

// handleAddCollaborator invite une personne sur un dépôt.
func (s *Server) handleAddCollaborator(writer http.ResponseWriter, request *http.Request) {
	org, repo, err := target(request)
	if err != nil {
		fail(writer, err)
		return
	}
	var body struct {
		Username   string `json:"username"`
		Permission string `json:"permission"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	username, err := valid.Login(body.Username, "Compte GitHub")
	if err != nil {
		fail(writer, err)
		return
	}
	permission, err := config.ValidatePermission(body.Permission)
	if err != nil {
		fail(writer, err)
		return
	}
	if exists, err := s.deps.Client.UserExists(username); err == nil && !exists {
		fail(writer, valid.Errorf("Le compte « %s » n'existe pas sur GitHub.", username))
		return
	}

	state, err := s.deps.Client.AddCollaborator(org, repo, username, permission)
	if err != nil {
		fail(writer, err)
		return
	}
	// Ce qu'on savait des accès de ce dépôt vient de devenir faux.
	s.resolver(org).ForgetAccess(org, repo)

	label := "accès accordé"
	if state == ghapi.CollaboratorInvited {
		label = "invitation envoyée"
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"username": username, "permission": permission, "state": state, "label": label,
	})
}

// handleRemoveCollaborator retire un accès direct.
func (s *Server) handleRemoveCollaborator(writer http.ResponseWriter, request *http.Request) {
	org, repo, err := target(request)
	if err != nil {
		fail(writer, err)
		return
	}
	login, err := valid.Login(request.PathValue("login"), "Compte GitHub")
	if err != nil {
		fail(writer, err)
		return
	}
	if err := s.deps.Client.RemoveCollaborator(org, repo, login); err != nil {
		fail(writer, err)
		return
	}
	s.resolver(org).ForgetAccess(org, repo)
	writeJSON(writer, http.StatusOK, map[string]string{
		"message": "@" + login + " n'a plus accès à « " + repo + " ».",
	})
}

// handleCancelInvitation annule une invitation en attente.
func (s *Server) handleCancelInvitation(writer http.ResponseWriter, request *http.Request) {
	org, repo, err := target(request)
	if err != nil {
		fail(writer, err)
		return
	}
	identifier, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil {
		fail(writer, valid.Errorf("Invitation inconnue."))
		return
	}
	if err := s.deps.Client.CancelInvitation(org, repo, identifier); err != nil {
		fail(writer, err)
		return
	}
	s.resolver(org).ForgetAccess(org, repo)
	writeJSON(writer, http.StatusOK, map[string]string{"message": "Invitation annulée."})
}

// handleDeleteRepo supprime définitivement un dépôt. Le nom exact doit être
// retapé : aucune option ne court-circuite cette confirmation.
func (s *Server) handleDeleteRepo(writer http.ResponseWriter, request *http.Request) {
	org, repo, err := target(request)
	if err != nil {
		fail(writer, err)
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	if strings.TrimSpace(body.Confirm) != repo {
		fail(writer, valid.Errorf("Confirmation incorrecte : retapez « %s » exactement.", repo))
		return
	}
	if present, known := s.deps.Client.HasScope("delete_repo"); known && !present {
		failScope(writer, "delete_repo",
			"Le jeton n'a pas la portée « delete_repo » : la suppression serait refusée.")
		return
	}
	if err := s.deps.Client.DeleteRepo(org, repo); err != nil {
		fail(writer, err)
		return
	}
	s.deleted(org, repo)
	writeJSON(writer, http.StatusOK, map[string]string{
		"message": "« " + repo + " » supprimé.",
	})
}
