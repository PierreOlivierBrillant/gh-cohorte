package web

import (
	"net/http"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Les noms accumulés poste par poste ne montent pas d'eux-mêmes au registre :
// ils sont dans un fichier que plus rien ne lira. La publication les y verse,
// une fois — et comme tout ce qui écrit dans cet outil, elle montre d'abord ce
// qu'elle ferait.

// publicationRendu est ce que l'aperçu et la publication renvoient tous deux.
// Après écriture, l'aperçu qui l'accompagne est celui de ce qui reste à faire :
// vide, si tout est monté.
type publicationRendu struct {
	Org          string               `json:"org"`
	Repo         string               `json:"repo"`
	Plan         registry.Publication `json:"plan"`
	Total        int                  `json:"total"`
	Published    int                  `json:"published"`
	Exposure     string               `json:"exposure,omitempty"`
	Notice       string               `json:"notice,omitempty"`
	RegistrySize int                  `json:"registry_size"`
}

// handleRegistryPreview montre ce que publier ferait, sans rien écrire.
func (s *Server) handleRegistryPreview(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	set, avis := s.names(org)
	locales := s.classrooms.People(org)
	plan := registry.Plan(set, locales)

	writeJSON(writer, http.StatusOK, publicationRendu{
		Org: org, Repo: registry.RepoName, Plan: plan, Total: plan.Count(),
		// L'avertissement sur la permission de base se donne ici : c'est le
		// moment où l'on s'apprête à déposer des noms dans l'organisation.
		Exposure: s.registryOf(org).Exposure(), Notice: avis,
		RegistrySize: set.Len(),
	})
}

// handleRegistryPublish verse au registre ce que l'aperçu a montré.
//
// Le désaccord entre le registre et le poste ne se tranche pas en silence :
// « prefer_local » est un choix que l'interface pose, et qui vaut pour cette
// publication seulement.
func (s *Server) handleRegistryPublish(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	var body struct {
		// PreferLocal fait gagner les noms de ce poste sur ceux du registre.
		PreferLocal bool `json:"prefer_local"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}

	set, _ := s.names(org)
	plan := registry.Plan(set, s.classrooms.People(org))
	if plan.Empty() {
		fail(writer, valid.Errorf(
			"Rien à publier : le registre de « %s » connaît déjà tout ce que ce poste sait.", org))
		return
	}
	publie, err := s.registryOf(org).Apply(plan.Apply(body.PreferLocal))
	if err != nil {
		fail(writer, err)
		return
	}

	// Ce qui reste à faire après coup : vide quand tout est monté. C'est plus
	// honnête qu'un simple « c'est fait », qui ne dirait rien des comptes sans
	// nom ni des désaccords qu'on a choisi de ne pas reprendre.
	restant := registry.Plan(publie, s.classrooms.People(org))
	writeJSON(writer, http.StatusOK, publicationRendu{
		Org: org, Repo: registry.RepoName, Plan: restant,
		Total: restant.Count(), Published: plan.Count(),
		Exposure: s.registryOf(org).Exposure(), RegistrySize: publie.Len(),
	})
}
