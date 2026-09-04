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

// visibles rassemble les groupes de l'organisation : ceux qu'on a déclarés, et
// ceux que les dépôts dessinent sans qu'on ait rien eu à déclarer. Un groupe
// existe parce que ses dépôts existent ; le fichier local n'ajoute que ce
// qu'eux ne savent pas dire.
func (s *Server) visibles(org string, repos []groups.RepoInfo) []classroom.Classroom {
	declares := s.classrooms.List(org)
	vus := map[string]bool{}
	for _, cours := range declares {
		vus[normaliserScope(cours.Scope())] = true
	}

	liste := append([]classroom.Classroom(nil), declares...)
	for _, candidat := range classroom.Places(repos) {
		if vus[normaliserScope(candidat)] {
			continue
		}
		cours, err := classroom.AtScope(org, candidat, classroom.DefaultsFrom(s.Settings()))
		if err != nil {
			continue
		}
		vus[normaliserScope(candidat)] = true
		liste = append(liste, cours)
	}
	return liste
}

func normaliserScope(scope string) string {
	return classroom.NormalizeScope(scope)
}
