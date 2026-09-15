package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/exchange"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plagiarism"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Publier ce qu'on a donné, et voir ce que les autres ont donné.
//
// Deux choses se publient, et elles ne disent pas la même chose. Le catalogue
// annonce qu'un travail existe : une place, un nom, un décompte. L'index
// d'empreintes permet de le comparer sans le lire. Le premier suffit à ce qu'un
// collègue sache à qui s'adresser ; le second lui évite d'avoir à demander.
//
// Ni l'un ni l'autre ne nomme un étudiant.

// indexes rend le dépôt des index d'une organisation.
func (s *Server) indexes(org string) *exchange.Store {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.indexStores == nil {
		s.indexStores = map[string]*exchange.Store{}
	}
	if existing, found := s.indexStores[org]; found {
		return existing
	}
	store := exchange.NewStore(s.deps.Client, org)
	s.indexStores[org] = store
	return store
}

// handleCatalog rend le catalogue des travaux donnés dans l'organisation.
//
// C'est la moitié de ce qu'il faut pour comparer avec un collègue : savoir ce
// qu'il a donné. L'autre moitié — les dépôts eux-mêmes — ne lui sera jamais
// demandée, et c'est bien le propos.
func (s *Server) handleCatalog(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	set, avis := s.names(org)
	catalogue := set.Catalog()

	// Les travaux d'un cours : les sigles équivalents sont dépliés ici, par les
	// règles de l'organisation — le catalogue, lui, ne sait pas qu'un cours a
	// changé de nom.
	filtre := strings.TrimSpace(request.URL.Query().Get("course"))
	lignes := catalogue.Teaching
	if filtre != "" {
		lignes = catalogue.Course(s.rulesOf(org).SameCourse(filtre))
	}

	rendues := make([]map[string]any, 0, len(lignes))
	for _, ligne := range lignes {
		rendues = append(rendues, map[string]any{
			"id": ligne.ID(), "scope": ligne.Scope, "assignment": ligne.Assignment,
			"teacher": ligne.Teacher, "teacher_name": set.Name(ligne.Teacher),
			"copies": ligne.Copies, "last_handin": ligne.LastHandin,
			"indexed": ligne.Indexed, "updated_at": ligne.UpdatedAt,
			"mine": strings.EqualFold(ligne.Teacher, s.deps.Viewer),
		})
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"org": org, "teaching": rendues, "teachers": catalogue.Teachers(),
		"viewer": s.deps.Viewer, "warning": avis,
	})
}

// publishInput est ce que la page demande en publiant.
type publishInput struct {
	// Index dit s'il faut publier aussi les empreintes. Sans lui, seul le
	// catalogue est mis à jour : le travail est annoncé, mais il faudra une
	// demande pour le comparer.
	Index bool `json:"index"`
	// Origin est l'étiquette que les copies porteront chez les collègues.
	// Vide, c'est la place du groupe — elle ne nomme personne.
	Origin string `json:"origin"`
}

// handlePlagiarismPublish publie le catalogue et l'index d'un rapport.
func (s *Server) handlePlagiarismPublish(writer http.ResponseWriter, request *http.Request) {
	report, err := s.reportNamed(request.PathValue("name"))
	if err != nil {
		fail(writer, err)
		return
	}
	var body publishInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	org := strings.TrimSpace(report.Request.Org)
	if org == "" {
		org = s.org()
	}

	ligne, err := teachingOf(report)
	if err != nil {
		fail(writer, err)
		return
	}
	ligne.Teacher = s.deps.Viewer
	ligne.Indexed = body.Index

	if !body.Index {
		set, err := s.registryOf(org).Apply(registry.Publish(ligne))
		if err != nil {
			fail(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{
			"catalog": len(set.Catalog().Teaching), "indexed": false,
			"assignment": ligne.ID(),
		})
		return
	}

	origine := strings.TrimSpace(body.Origin)
	if origine == "" {
		origine = ligne.Scope
	}
	publie, table, err := plagiarism.Publishable(report, s.deps.Viewer, origine,
		anonymize.Options{})
	if err != nil {
		fail(writer, err)
		return
	}
	if err := s.indexes(org).Publish(publie); err != nil {
		fail(writer, err)
		return
	}
	// La table reste ici, à côté du rapport : c'est elle seule qui dit qui se
	// cache derrière un jeton, et elle ne monte jamais dans l'organisation.
	chemin, err := ecrireTable(s.reportDir(), report.Basename(), table)
	if err != nil {
		fail(writer, err)
		return
	}
	set, err := s.registryOf(org).Apply(registry.Publish(ligne))
	if err != nil {
		fail(writer, err)
		return
	}

	writeJSON(writer, http.StatusOK, map[string]any{
		"assignment": publie.Assignment, "copies": publie.Copies(),
		"prints": publie.Prints(), "origin": origine, "indexed": true,
		"table": chemin, "catalog": len(set.Catalog().Teaching),
	})
}

// teachingOf tire du rapport la ligne de catalogue qui lui correspond.
func teachingOf(report *plagiarism.Report) (exchange.Teaching, error) {
	scope, nom, ok := naming.SplitAssignment(report.Request.Assignment)
	if !ok {
		return exchange.Teaching{}, valid.Errorf(
			"Publication : « %s » n'est pas un travail de la nomenclature. Seul un "+
				"travail d'un groupe déclaré se publie.", report.Request.Assignment)
	}
	// Le décompte ne porte que sur nos dépôts : les copies reçues d'ailleurs ne
	// sont pas les nôtres à annoncer.
	copies, dernier := 0, ""
	for _, target := range report.Request.Targets {
		copies++
		if target.HandedIn > dernier {
			dernier = target.HandedIn
		}
	}
	if len(dernier) > 10 {
		dernier = dernier[:10] // le jour suffit : l'heure daterait une personne
	}
	return exchange.Teaching{
		Scope: scope, Assignment: nom, Copies: copies, LastHandin: dernier,
		UpdatedAt: report.CreatedAt,
	}, nil
}

// ecrireTable pose la table de correspondance à côté du rapport.
func ecrireTable(directory, base string, table anonymize.Table) (string, error) {
	dossier := filepath.Join(directory, plagiarism.Dir)
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		return "", err
	}
	chemin := filepath.Join(dossier, base+"-index-correspondance.json")
	payload, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(chemin, append(payload, '\n'), 0o600); err != nil {
		return "", valid.Errorf("Table « %s » : %v.", chemin, err)
	}
	return chemin, nil
}
