package web

import (
	"errors"
	"net/http"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/picker"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Choisir un fichier depuis une page web demande un détour : le navigateur ne
// donne jamais le chemin d'un fichier déposé, par sécurité. Le serveur, lui,
// tourne sur la machine de la personne. Deux routes s'offrent donc à
// l'interface — ouvrir la fenêtre du système, ou lui montrer un explorateur
// interne quand la plateforme n'en a pas.

// pathRequest est ce que l'interface envoie pour ouvrir ou parcourir.
type pathRequest struct {
	// Path est le point de départ : un dossier, ou le chemin déjà saisi.
	Path string `json:"path"`
	// Dirs demande un dossier plutôt qu'un fichier.
	Dirs bool `json:"dirs"`
	// Title est le titre de la fenêtre du système.
	Title string `json:"title"`
}

// handlePickPath ouvre le sélecteur du système et attend le choix.
func (s *Server) handlePickPath(writer http.ResponseWriter, request *http.Request) {
	var body pathRequest
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	titre := body.Title
	if titre == "" {
		titre = "Choisir"
	}

	chemin, err := picker.Pick(picker.Request{
		Title: titre, Dir: body.Dirs, Start: body.Path,
	})
	switch {
	case errors.Is(err, picker.ErrCanceled):
		// Refermer la fenêtre n'est pas une panne : l'interface ne fait rien.
		writeJSON(writer, http.StatusOK, map[string]any{"canceled": true})
	case errors.Is(err, picker.ErrUnavailable):
		fail(writer, valid.Errorf(
			"Cette machine n'a pas de fenêtre de sélection : utilisez l'explorateur "+
				"de l'interface."))
	case err != nil:
		fail(writer, valid.Errorf("Sélecteur de fichiers : %v", err))
	default:
		writeJSON(writer, http.StatusOK, map[string]any{"path": chemin})
	}
}

// handleBrowsePath liste un dossier pour l'explorateur interne.
func (s *Server) handleBrowsePath(writer http.ResponseWriter, request *http.Request) {
	var body pathRequest
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	listing, err := picker.Browse(body.Path, body.Dirs)
	if err != nil {
		fail(writer, valid.Errorf("Dossier illisible : %v", err))
		return
	}
	writeJSON(writer, http.StatusOK, listing)
}
