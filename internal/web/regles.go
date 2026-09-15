package web

import (
	"net/http"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/rules"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Les règles de comparaison vivent dans le registre de l'organisation, à côté
// des noms et des échéances. Elles y sont partagées par toute l'équipe : le
// jour où « 5N6 » devient « 5M6 », une seule personne le déclare, et les
// analyses de tout le monde retrouvent les copies d'il y a trois ans.
//
// Ce que l'écran laisse écrire est volontairement étroit : les équivalences de
// sigles et de noms de travaux, c'est-à-dire ce qui change vraiment d'une année
// à l'autre. Les profils d'inspection se déclarent en modifiant le fichier sur
// github.com — ils sont rares, ils sont longs, et un formulaire qui les
// couvrirait serait un éditeur de texte déguisé.

// handleRules rend les règles de l'organisation.
func (s *Server) handleRules(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	declarees := rules.Rules{}
	if set, _ := s.names(org); set != nil {
		declarees = set.Rules()
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"org": org,
		// Ce que le registre porte, et ce qui vaut vraiment pour les analyses
		// une fois le fichier du lancement superposé. Les deux sont rendus :
		// sans quoi une équivalence venue de « --rules » paraîtrait déclarée
		// dans l'organisation, et on la chercherait en vain sur github.com.
		"declared":  declarees,
		"effective": declarees.Merge(s.deps.Rules),
		"override":  !s.deps.Rules.Empty(),
	})
}

// handleSetRules écrit les règles dans le registre de l'organisation.
func (s *Server) handleSetRules(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	var body rules.Rules
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	// La validation est celle du domaine : un sigle qui désignerait deux cours
	// est refusé ici comme il le serait au terminal, et avec les mêmes mots.
	valides, err := body.Validate()
	if err != nil {
		fail(writer, err)
		return
	}
	set, err := s.registryOf(org).Apply(registry.Declare(valides))
	if err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"org": org, "declared": set.Rules(),
		"effective": set.Rules().Merge(s.deps.Rules),
	})
}
