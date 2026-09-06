package web

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/scopes"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// refreshing empêche deux renouvellements de se croiser : gh prend le terminal
// pour lui, et deux flux d'appareil menés de front s'y mélangeraient.
var refreshing sync.Mutex

// tokenPayload décrit le jeton tel que l'interface le montre.
type tokenPayload struct {
	Viewer string          `json:"viewer"`
	Host   string          `json:"host"`
	Origin string          `json:"origin"`
	Scopes []scopes.Status `json:"scopes"`
	// Refreshable dit si gh peut renouveler ce jeton : celui d'une variable
	// d'environnement lui échappe, et l'interface ne doit rien promettre.
	Refreshable bool `json:"refreshable"`
	// Missing nomme ce qui manque encore après un renouvellement : GitHub
	// accorde ce qu'on lui a accordé, pas ce qu'on a demandé.
	Missing []string `json:"missing,omitempty"`
	Command string   `json:"command"`
}

// tokenState décrit l'état courant du jeton.
func (s *Server) tokenState(missing []string) tokenPayload {
	granted, known := s.deps.Client.ScopeList()
	return tokenPayload{
		Viewer:      s.deps.Viewer,
		Host:        s.deps.Host,
		Origin:      s.deps.TokenOrigin,
		Scopes:      scopes.Inventory(granted, known),
		Refreshable: !scopes.FromEnvironment(s.deps.TokenOrigin),
		Missing:     missing,
		Command:     scopes.Command(s.deps.Host, s.wanted(nil), nil),
	}
}

// wanted renvoie les portées que l'outil réclame, celles déjà accordées
// comprises : un renouvellement ne doit jamais faire perdre du terrain.
func (s *Server) wanted(extra []string) []string {
	granted, _ := s.deps.Client.ScopeList()
	minimal := make([]string, 0, len(scopes.Catalog))
	for _, scope := range scopes.Catalog {
		if scope.Minimal {
			minimal = append(minimal, scope.Name)
		}
	}
	return scopes.Union(scopes.Union(granted, minimal), extra)
}

// handleToken décrit le jeton : compte, provenance, portées.
func (s *Server) handleToken(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, s.tokenState(nil))
}

// handleRefreshToken régénère le jeton avec les portées demandées.
//
// L'échange se joue dans le terminal d'où l'outil a été lancé : GitHub y
// affiche un code à recopier dans le navigateur. La requête reste ouverte le
// temps que la personne réponde, et l'abandon de l'onglet arrête gh.
func (s *Server) handleRefreshToken(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Scopes []string `json:"scopes"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}

	desired, err := cleaned(body.Scopes)
	if err != nil {
		fail(writer, err)
		return
	}
	granted, known := s.deps.Client.ScopeList()
	add := s.wanted(desired)
	remove := droppable(granted, known, desired)

	if !refreshing.TryLock() {
		fail(writer, valid.Errorf("Un renouvellement est déjà en cours : "+
			"répondez-lui dans le terminal, ou attendez qu'il expire."))
		return
	}
	defer refreshing.Unlock()

	// La sortie de gh part au terminal — c'est là qu'elle sert — et une copie
	// reste sous la main pour dire ce qui a échoué, le cas échéant.
	var echo bytes.Buffer
	refresher := s.deps.Refresher
	if refresher == nil {
		refresher = scopes.NewRefresher()
	}
	fresh, err := refresher.Do(request.Context(), scopes.Request{
		Host:   s.deps.Host,
		Origin: s.deps.TokenOrigin,
		Add:    add,
		Remove: remove,
		In:     os.Stdin,
		Out:    io.MultiWriter(os.Stdout, &echo),
		Err:    io.MultiWriter(os.Stderr, &echo),
	})
	if err != nil {
		fail(writer, enrich(err, echo.String()))
		return
	}
	if err := s.deps.Client.UseToken(fresh); err != nil {
		fail(writer, err)
		return
	}
	// C'est cet appel qui révèle les portées du nouveau jeton : sans lui,
	// l'interface les afficherait encore comme inconnues.
	if _, err := s.deps.Client.AuthenticatedUser(); err != nil {
		fail(writer, err)
		return
	}
	renewed, _ := s.deps.Client.ScopeList()
	writeJSON(writer, http.StatusOK, s.tokenState(scopes.Missing(renewed, desired)))
}

// cleaned valide les portées demandées par l'interface.
func cleaned(names []string) ([]string, error) {
	var list []string
	for _, name := range names {
		clean, err := scopes.Validate(name)
		if err != nil {
			return nil, err
		}
		if !scopes.Has(list, clean) {
			list = append(list, clean)
		}
	}
	return list, nil
}

// droppable dit ce qu'un renouvellement peut retirer : seulement des portées du
// catalogue, et seulement si le jeton les annonce. Ce qu'un autre outil a
// obtenu — « gist », « admin:org » — ne regarde pas celui-ci.
func droppable(granted []string, known bool, desired []string) []string {
	if !known {
		return nil
	}
	var remove []string
	for _, scope := range scopes.Catalog {
		if !scope.Minimal && scopes.Has(granted, scope.Name) && !scopes.Has(desired, scope.Name) {
			remove = append(remove, scope.Name)
		}
	}
	return remove
}

// enrich ajoute la dernière ligne dite par gh : c'est elle qui nomme la cause.
func enrich(err error, output string) error {
	last := ""
	for _, line := range strings.Split(output, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			last = trimmed
		}
	}
	if last == "" {
		return err
	}
	return valid.Errorf("%s (%s)", err.Error(), last)
}
