package web

import (
	"net/http"
	"strings"

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
	// Teams énumère les équipes de l'organisation, pour qu'on puisse donner
	// accès au registre à celle qui enseigne.
	Teams []string `json:"teams,omitempty"`
}

// teamsOf énumère les équipes de l'organisation. N'en voir aucune n'est pas une
// panne : un compte qui n'est pas membre n'en voit pas, et il n'y a alors rien
// à proposer.
func (s *Server) teamsOf(org string) []string {
	equipes, err := s.registryOf(org).Teams()
	if err != nil {
		return nil
	}
	noms := make([]string, 0, len(equipes))
	for _, equipe := range equipes {
		noms = append(noms, equipe.Slug)
	}
	return noms
}

// handleRegistryGrant donne à une équipe accès au registre.
func (s *Server) handleRegistryGrant(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	var body struct {
		Team string `json:"team"`
	}
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	if err := s.registryOf(org).Grant(strings.TrimSpace(body.Team)); err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"team": body.Team,
		"message": "L'équipe « " + body.Team + " » a désormais accès à « " +
			registry.RepoName + " ». Cela ouvre un accès sans en fermer aucun : " +
			"c'est la permission de base de l'organisation qui restreint le reste.",
	})
}

// handleRegistryForgetHistory réécrit la branche du registre en un commit sans
// passé.
//
// C'est irréversible, et c'est demandé pour l'être : quelqu'un veut qu'un nom
// cesse d'être atteignable. Le nom complet du dépôt doit donc être retapé,
// comme pour une suppression — aucune option ne court-circuite cette
// confirmation.
func (s *Server) handleRegistryForgetHistory(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
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
	attendu := org + "/" + registry.RepoName
	if strings.TrimSpace(body.Confirm) != attendu {
		fail(writer, valid.Errorf(
			"Confirmation incorrecte : retapez « %s » exactement.", attendu))
		return
	}

	commit, err := s.registryOf(org).ForgetHistory()
	if err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"commit": commit,
		"message": "L'historique du registre est réécrit : plus rien de l'ancien n'est " +
			"atteignable depuis « " + registry.Branch + " ». GitHub garde un temps les objets " +
			"devenus inaccessibles, et un clone déjà fait garde ce qu'il avait.",
	})
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
		RegistrySize: set.Len(), Teams: s.teamsOf(org),
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
