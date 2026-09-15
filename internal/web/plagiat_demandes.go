package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/exchange"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plagiarism"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Demander à voir une copie, et décider de la montrer.
//
// Le dépistage s'arrête là où il devient utile : on voit qu'une paire sort du
// lot, et rien de plus. Pour juger, il faut lire les passages communs — donc du
// code d'en face.
//
// Il ne se lit pas tout seul. Le demandeur dépose une demande nommée ; le
// propriétaire la voit en ouvrant l'outil et décide. S'il accorde, l'outil lui
// prépare l'envoi anonymisé de cette seule copie, sous le jeton qu'elle portait
// déjà — et c'est lui qui l'envoie. Rien ne traîne dans l'organisation à la vue
// de toute l'équipe, et rien ne part dans son dos.

// handleAsks rend les demandes qui concernent celui qui regarde.
func (s *Server) handleAsks(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	set, _ := s.names(org)
	demandes := set.Asks()
	viewer := s.deps.Viewer

	writeJSON(writer, http.StatusOK, map[string]any{
		"org": org, "viewer": viewer,
		// Reçues et faites : les deux se regardent, et pas pour la même raison.
		// Les premières attendent une décision ; les secondes, une réponse.
		"received": decoreesAvecNoms(set, demandes.For(viewer)),
		"sent":     decoreesAvecNoms(set, demandes.By(viewer)),
		"waiting":  len(demandes.Waiting(viewer)),
	})
}

// decoreesAvecNoms ajoute à des demandes le nom complet de qui les a faites.
// Un écran qui ne parlerait qu'en comptes obligerait à traduire chaque ligne.
func decoreesAvecNoms(set *registry.Set, asks []exchange.Ask) []map[string]any {
	rendues := make([]map[string]any, 0, len(asks))
	for _, demande := range asks {
		rendues = append(rendues, map[string]any{
			"id": demande.ID, "from": demande.From, "to": demande.To,
			"from_name": set.Name(demande.From), "to_name": set.Name(demande.To),
			"assignment": demande.Assignment, "token": demande.Token,
			"similarity": demande.Similarity, "note": demande.Note,
			"state": demande.State, "created_at": demande.CreatedAt,
			"decided_at": demande.DecidedAt, "reason": demande.Reason,
		})
	}
	return rendues
}

// askInput est une demande, telle que la page la dépose.
type askInput struct {
	Assignment string  `json:"assignment"`
	Token      string  `json:"token"`
	Similarity float64 `json:"similarity"`
	Note       string  `json:"note"`
}

// handleAsk dépose une demande de levée du voile.
func (s *Server) handleAsk(writer http.ResponseWriter, request *http.Request) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		fail(writer, err)
		return
	}
	var body askInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}

	set, _ := s.names(org)
	// À qui s'adresser : c'est le catalogue qui le dit, et lui seul. Sans
	// annonce, on ne sait pas qui détient la copie, et la demande n'irait nulle
	// part.
	ligne, annonce := set.Catalog().Find(body.Assignment)
	if !annonce {
		fail(writer, valid.Errorf(
			"Demande : « %s » n'est pas annoncé au catalogue de l'organisation. "+
				"Sans cela, rien ne dit à qui la copie appartient.", body.Assignment))
		return
	}
	id, err := exchange.NewAskID()
	if err != nil {
		fail(writer, err)
		return
	}

	demande := exchange.Ask{
		ID: id, From: s.deps.Viewer, To: ligne.Teacher,
		Assignment: ligne.ID(), Token: body.Token,
		Similarity: body.Similarity, Note: strings.TrimSpace(body.Note),
		State: exchange.AskPending, CreatedAt: time.Now().Format(time.RFC3339),
	}
	if _, err := demande.Validated(); err != nil {
		fail(writer, err)
		return
	}
	if _, err := s.registryOf(org).Apply(registry.AskFor(demande)); err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"id": demande.ID, "to": demande.To, "to_name": set.Name(demande.To),
		"assignment": demande.Assignment, "token": demande.Token,
	})
}

// decideInput est ce qu'on répond à une demande.
type decideInput struct {
	// Destination est le fichier ZIP à écrire, quand on accorde.
	Destination string `json:"destination"`
	// Reason est ce qu'on répond, quand on refuse.
	Reason string `json:"reason"`
	Parts  bool   `json:"parts"`
}

// handleDenyAsk refuse une demande.
func (s *Server) handleDenyAsk(writer http.ResponseWriter, request *http.Request) {
	org, demande, err := s.askOf(request)
	if err != nil {
		fail(writer, err)
		return
	}
	var body decideInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	demande.State = exchange.AskDenied
	demande.DecidedAt = time.Now().Format(time.RFC3339)
	demande.Reason = strings.TrimSpace(body.Reason)
	if _, err := s.registryOf(org).Apply(registry.AskFor(demande)); err != nil {
		fail(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"id": demande.ID, "state": demande.State,
	})
}

// handleGrantAsk accorde une demande : l'envoi est préparé, la décision
// inscrite, et c'est le propriétaire qui enverra le fichier.
func (s *Server) handleGrantAsk(writer http.ResponseWriter, request *http.Request) {
	org, demande, err := s.askOf(request)
	if err != nil {
		fail(writer, err)
		return
	}
	var body decideInput
	if err := decode(request, &body); err != nil {
		fail(writer, err)
		return
	}
	destination, err := destinationDArchive(body.Destination,
		"demande-"+strings.ToLower(demande.ID))
	if err != nil {
		fail(writer, err)
		return
	}

	// Quelle copie le jeton désigne : la table écrite à la publication le dit,
	// et elle seule. Perdue, personne ne peut plus répondre — et il vaut mieux
	// le dire que d'envoyer la mauvaise.
	trouvee, err := plagiarism.LocateToken(s.reportDir(), demande.Assignment, demande.Token)
	if err != nil {
		fail(writer, err)
		return
	}
	requete, err := s.requeteDuTravail(demande.Assignment)
	if err != nil {
		fail(writer, err)
		return
	}

	envoi, err := plagiarism.Grant(s.deps.Client, demande, trouvee, requete,
		anonymize.Options{Parts: body.Parts})
	if err != nil {
		fail(writer, err)
		return
	}
	bilan, err := ecrireEnvoi(destination, envoi)
	if err != nil {
		fail(writer, err)
		return
	}

	// La décision n'est inscrite qu'une fois l'envoi préparé : un envoi qu'on
	// n'a pas su écrire ne doit pas laisser une demande marquée « accordée ».
	demande.State = exchange.AskGranted
	demande.DecidedAt = time.Now().Format(time.RFC3339)
	if _, err := s.registryOf(org).Apply(registry.AskFor(demande)); err != nil {
		fail(writer, err)
		return
	}
	bilan["id"], bilan["state"] = demande.ID, demande.State
	bilan["to"] = demande.From
	writeJSON(writer, http.StatusOK, bilan)
}

// askOf retrouve la demande que l'adresse désigne, et refuse d'y toucher quand
// elle ne s'adresse pas à celui qui regarde.
func (s *Server) askOf(request *http.Request) (string, exchange.Ask, error) {
	org, err := valid.Login(request.PathValue("org"), "Organisation")
	if err != nil {
		return "", exchange.Ask{}, err
	}
	set, _ := s.names(org)
	demande, trouvee := set.Asks().Find(request.PathValue("id"))
	if !trouvee {
		return org, exchange.Ask{}, valid.Errorf(
			"Demande « %s » : inconnue.", request.PathValue("id"))
	}
	// Ce n'est pas cette vérification qui protège quoi que ce soit — le
	// registre est ouvert à toute l'équipe enseignante. Elle évite qu'on
	// tranche par mégarde une demande qui ne nous regarde pas.
	if !strings.EqualFold(demande.To, s.deps.Viewer) {
		return org, exchange.Ask{}, valid.Errorf(
			"Demande %s : elle s'adresse à @%s, pas à vous.", demande.ID, demande.To)
	}
	if !demande.Pending() {
		return org, exchange.Ask{}, valid.Errorf(
			"Demande %s : elle est déjà %s.", demande.ID, demande.State)
	}
	return org, demande, nil
}

// requeteDuTravail rebâtit la demande d'analyse d'un travail à soi, pour en
// tirer l'envoi d'une copie.
func (s *Server) requeteDuTravail(assignment string) (plagiarism.Request, error) {
	scope, _, ok := naming.SplitAssignment(assignment)
	if !ok {
		return plagiarism.Request{}, valid.Errorf(
			"« %s » n'est pas un travail de la nomenclature.", assignment)
	}
	cours, err := s.placeAt(scope)
	if err != nil {
		return plagiarism.Request{}, err
	}
	repos, _, err := s.repos(cours.Org, false)
	if err != nil {
		return plagiarism.Request{}, err
	}
	cours = s.enrichi(cours, repos)

	declarees := s.rulesOf(cours.Org)
	cibles, err := s.plagiatTargets(cours, assignment, repos, plagiatInput{}, declarees)
	if err != nil {
		return plagiarism.Request{}, err
	}
	return plagiarism.Request{
		Assignment: assignment, Org: cours.Org, Targets: cibles, Rules: declarees,
	}, nil
}
