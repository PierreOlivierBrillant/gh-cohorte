package exchange

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// La demande, et la levée du voile.
//
// Le dépistage s'arrête là où il devient utile : on voit qu'une paire sort du
// lot, et on ne sait rien de plus. Pour juger, il faut lire les passages
// communs — donc du code d'en face, donc percer le cloisonnement.
//
// Il se perce sur un geste, jamais tout seul. Le demandeur dépose une demande
// nommée : ce travail, ce jeton, ce qu'il a mesuré, et ce qu'il en dit. Le
// propriétaire la voit, et décide. S'il accorde, l'outil lui prépare l'envoi
// anonymisé de cette seule copie, sous le jeton qu'elle portait déjà — et c'est
// lui qui l'envoie. Rien ne passe dans son dos, et rien ne traîne dans
// l'organisation à la vue de toute l'équipe.
//
// Ce que la demande ne dit pas compte autant : elle ne nomme pas la copie du
// demandeur. Le propriétaire n'a pas besoin de savoir lequel de ses étudiants
// est soupçonné chez l'autre pour décider de montrer le sien.

// AsksFile porte les demandes, dans le registre de l'organisation.
const AsksFile = "demandes.json"

// États d'une demande.
const (
	AskPending = "en attente"
	AskGranted = "accordée"
	AskDenied  = "refusée"
)

// Ask est une demande de levée du voile sur une copie.
type Ask struct {
	// ID désigne la demande. Il est court et tiré au hasard : il se recopie
	// dans un courriel, et il ne dit rien de ce qu'il désigne.
	ID string `json:"id"`
	// From demande, To détient la copie.
	From string `json:"from"`
	To   string `json:"to"`
	// Assignment et Token désignent la copie visée dans l'index publié.
	Assignment string `json:"assignment"`
	Token      string `json:"token"`
	// Similarity est ce que le demandeur a mesuré. Sans elle, une demande ne
	// dit pas pourquoi elle est faite.
	Similarity float64 `json:"similarity,omitempty"`
	Note       string  `json:"note,omitempty"`
	State      string  `json:"state"`
	CreatedAt  string  `json:"created_at"`
	DecidedAt  string  `json:"decided_at,omitempty"`
	// Reason est ce que le propriétaire répond, quand il refuse.
	Reason string `json:"reason,omitempty"`
}

// Pending dit que la demande attend une décision.
func (a Ask) Pending() bool { return a.State == AskPending }

// NewAskID tire l'identifiant d'une demande : six caractères sans voyelle, pour
// qu'il ne forme jamais un mot.
func NewAskID() (string, error) {
	const alphabet = "BCDFGHJKLMNPQRSTVWXZ23456789"
	var id strings.Builder
	for index := 0; index < 6; index++ {
		rang, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", valid.Errorf("Demande : tirage impossible (%v).", err)
		}
		id.WriteByte(alphabet[rang.Int64()])
	}
	return id.String(), nil
}

// Validated met une demande en forme et refuse ce qui ne se tient pas.
func (a Ask) Validated() (Ask, error) { return a.validate() }

func (a Ask) validate() (Ask, error) {
	a.ID = strings.ToUpper(strings.TrimSpace(a.ID))
	a.From = strings.ToLower(strings.TrimSpace(a.From))
	a.To = strings.ToLower(strings.TrimSpace(a.To))
	a.Assignment = strings.ToLower(strings.TrimSpace(a.Assignment))
	a.Token = strings.ToUpper(strings.TrimSpace(a.Token))
	if a.State == "" {
		a.State = AskPending
	}
	if a.ID == "" {
		return a, valid.Errorf("Demande : elle n'a pas d'identifiant.")
	}
	if _, _, ok := naming.SplitAssignment(a.Assignment); !ok {
		return a, valid.Errorf(
			"Demande %s : « %s » n'est pas un travail de la nomenclature.",
			a.ID, a.Assignment)
	}
	if a.From == "" || a.To == "" {
		return a, valid.Errorf(
			"Demande %s : elle ne dit pas qui demande, ou à qui.", a.ID)
	}
	if a.From == a.To {
		return a, valid.Errorf(
			"Demande %s : on ne se demande pas à soi-même de lever le voile.", a.ID)
	}
	if a.Token == "" {
		return a, valid.Errorf("Demande %s : elle ne désigne aucune copie.", a.ID)
	}
	switch a.State {
	case AskPending, AskGranted, AskDenied:
	default:
		return a, valid.Errorf("Demande %s : état « %s » inconnu.", a.ID, a.State)
	}
	return a, nil
}

// Asks est l'ensemble des demandes de l'organisation.
type Asks struct {
	Version int   `json:"version"`
	Asks    []Ask `json:"asks"`
}

// Empty dit qu'il n'y a rien à écrire.
func (a Asks) Empty() bool { return len(a.Asks) == 0 }

// Validate met les demandes en forme.
func (a Asks) Validate() (Asks, error) {
	a.Version = Version
	vues := map[string]int{}
	lignes := make([]Ask, 0, len(a.Asks))
	for _, demande := range a.Asks {
		valide, err := demande.validate()
		if err != nil {
			return a, err
		}
		// Une demande réécrite remplace la sienne : c'est ainsi qu'elle passe
		// d'« en attente » à « accordée ».
		if rang, deja := vues[valide.ID]; deja {
			lignes[rang] = valide
			continue
		}
		vues[valide.ID] = len(lignes)
		lignes = append(lignes, valide)
	}
	// De la plus récente à la plus ancienne : c'est celle d'aujourd'hui qu'on
	// vient traiter.
	sort.SliceStable(lignes, func(first, second int) bool {
		if lignes[first].CreatedAt != lignes[second].CreatedAt {
			return lignes[first].CreatedAt > lignes[second].CreatedAt
		}
		return lignes[first].ID < lignes[second].ID
	})
	a.Asks = lignes
	return a, nil
}

// With verse des demandes et rend l'ensemble qui en résulte, avec un booléen
// qui dit s'il a bougé.
func (a Asks) With(asks []Ask) (Asks, bool, error) {
	if len(asks) == 0 {
		return a, false, nil
	}
	// Les nouvelles en dernier : à identifiant égal, « Validate » garde la
	// dernière rencontrée, et c'est bien la dernière écrite qui vaut. C'est
	// ainsi qu'une demande passe d'« en attente » à « accordée ».
	fusion := Asks{Version: Version, Asks: append([]Ask(nil), a.Asks...)}
	fusion.Asks = append(fusion.Asks, asks...)
	valide, err := fusion.Validate()
	if err != nil {
		return a, false, err
	}
	avant, err := EncodeAsks(a)
	if err != nil {
		return a, false, err
	}
	apres, err := EncodeAsks(valide)
	if err != nil {
		return a, false, err
	}
	return valide, string(avant) != string(apres), nil
}

// Find retrouve une demande par son identifiant.
func (a Asks) Find(id string) (Ask, bool) {
	id = strings.ToUpper(strings.TrimSpace(id))
	for _, demande := range a.Asks {
		if demande.ID == id {
			return demande, true
		}
	}
	return Ask{}, false
}

// For rend les demandes adressées à quelqu'un.
func (a Asks) For(account string) []Ask {
	return a.filter(func(demande Ask) bool {
		return demande.To == strings.ToLower(strings.TrimSpace(account))
	})
}

// By rend les demandes faites par quelqu'un.
func (a Asks) By(account string) []Ask {
	return a.filter(func(demande Ask) bool {
		return demande.From == strings.ToLower(strings.TrimSpace(account))
	})
}

// Waiting rend les demandes qu'on doit encore trancher.
func (a Asks) Waiting(account string) []Ask {
	waiting := make([]Ask, 0, 4)
	for _, demande := range a.For(account) {
		if demande.Pending() {
			waiting = append(waiting, demande)
		}
	}
	return waiting
}

func (a Asks) filter(keep func(Ask) bool) []Ask {
	gardees := make([]Ask, 0, len(a.Asks))
	for _, demande := range a.Asks {
		if keep(demande) {
			gardees = append(gardees, demande)
		}
	}
	return gardees
}

// DecodeAsks relit des demandes écrites.
func DecodeAsks(content []byte) (Asks, []string) {
	var lues Asks
	if err := json.Unmarshal(content, &lues); err != nil {
		return Asks{}, []string{fmt.Sprintf("Demandes illisibles : %v.", err)}
	}
	if lues.Version > Version {
		return Asks{}, []string{fmt.Sprintf(
			"Demandes : elles viennent d'une version %d de l'outil, qui n'en "+
				"connaît que %d. Mettez l'extension à jour.", lues.Version, Version)}
	}
	gardees := make([]Ask, 0, len(lues.Asks))
	soucis := make([]string, 0, 2)
	for _, demande := range lues.Asks {
		valide, err := demande.validate()
		if err != nil {
			soucis = append(soucis, err.Error())
			continue
		}
		gardees = append(gardees, valide)
	}
	lues.Asks = gardees
	valide, err := lues.Validate()
	if err != nil {
		return Asks{}, append(soucis, err.Error())
	}
	return valide, soucis
}

// EncodeAsks écrit des demandes.
func EncodeAsks(asks Asks) ([]byte, error) {
	valide, err := asks.Validate()
	if err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(valide, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}
