package web

import (
	"net/http"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
)

// Un groupe se désigne par sa place — « a26.5n6.1010 » —, et cette place est
// écrite dans le nom de chacun de ses dépôts. Il n'y a donc rien à retrouver
// dans un fichier local pour l'ouvrir : la place suffit, et ce qu'on retient
// d'un groupe — sa liste, ses réglages — vient s'y greffer quand il existe.
//
// C'est ce qui fait qu'un groupe présent dans l'organisation s'affiche sans
// avoir été déclaré, et qu'un lien vers lui vaut d'une machine à l'autre.

// org est l'organisation de la session, celle que l'interface a choisie.
func (s *Server) org() string { return s.Settings().Org }

// place résout la place demandée : le groupe qu'on a déclaré là, ou, à défaut,
// celui que sa place seule décrit.
func (s *Server) place(request *http.Request) (classroom.Classroom, error) {
	return s.placeAt(request.PathValue("scope"))
}

// placeAt résout une place donnée autrement que par l'adresse — le groupe
// d'arrivée d'un déplacement, par exemple.
func (s *Server) placeAt(scope string) (classroom.Classroom, error) {
	org := s.org()
	if cours, trouve := s.classrooms.Find(org, scope); trouve {
		return cours, nil
	}
	return classroom.AtScope(org, scope, classroom.DefaultsFrom(s.Settings()))
}

// visibles rassemble les groupes de l'organisation. Le magasin en décide : le
// terminal doit voir les mêmes groupes que le navigateur.
func (s *Server) visibles(org string, repos []groups.RepoInfo) []classroom.Classroom {
	return s.classrooms.Visible(org, repos, classroom.DefaultsFrom(s.Settings()))
}
