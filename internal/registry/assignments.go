package registry

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Les dates de remise vivent dans l'organisation, à côté des noms.
//
// Rien dans un nom de dépôt ne peut porter une échéance, et la retenir sur un
// poste la rendrait vraie d'une seule machine : le collègue qui ouvre le même
// groupe verrait les mêmes dépôts et les mêmes travaux sans voir la date à
// laquelle ils sont attendus. C'est exactement ce que le registre a résolu pour
// les noms, et la réponse est la même — un fichier de plus dans « .cohorte »,
// écrit une fois pour tout le monde.
//
// La clé est l'identifiant complet du travail — « a26.5n6.01.tp1 » —, jamais
// son nom court : un fichier d'organisation porte tous les groupes, et deux
// d'entre eux ont chacun leur « tp1 ».

// Assignment est ce que le registre retient d'un travail.
type Assignment struct {
	// ID est l'identifiant complet du travail, tel qu'il précède le nom de
	// l'étudiant dans le nom de chacun de ses dépôts.
	ID string `json:"id"`
	// Due est la date cible, sous sa forme normale : « 2026-10-01 », ou
	// « 2026-10-01T23:59 » quand l'heure a été donnée.
	Due string `json:"due"`
	// SetAt dit quand l'échéance a été fixée. Elle se déplace parfois d'un
	// commun accord, et savoir de quand date la version affichée aide à s'y
	// retrouver quand deux personnes enseignent le même cours.
	SetAt string `json:"set_at,omitempty"`
}

// Key sert au rangement : GitHub ne distingue pas la casse d'un nom de dépôt,
// et la nomenclature non plus.
func (a Assignment) Key() string { return strings.ToLower(strings.TrimSpace(a.ID)) }

// validate met une échéance en forme et refuse ce qui n'en est pas une.
func (a Assignment) validate() (Assignment, error) {
	a.ID = strings.TrimSpace(a.ID)
	if a.ID == "" {
		return a, valid.Errorf("Date cible : aucun travail indiqué.")
	}
	due, err := valid.NormalizeDue(a.Due)
	if err != nil {
		return a, err
	}
	a.Due = due
	a.SetAt = strings.TrimSpace(a.SetAt)
	return a, nil
}

// ------------------------------------------------------------------ fichier

// assignmentsFile est ce qui est écrit dans « travaux.json ».
type assignmentsFile struct {
	Version     int          `json:"version"`
	Assignments []Assignment `json:"assignments"`
}

// DecodeAssignments relit le fichier des travaux. Comme celui des étudiants, il
// se modifie à la main sur github.com : une ligne mal écrite se signale et
// s'écarte, elle ne prive pas toute l'organisation de ses échéances.
func DecodeAssignments(content []byte) ([]Assignment, []string) {
	if len(strings.TrimSpace(string(content))) == 0 {
		return nil, nil
	}
	var lu assignmentsFile
	if err := json.Unmarshal(content, &lu); err != nil {
		return nil, []string{"travaux.json est illisible (" + err.Error() +
			") : les dates de remise sont ignorées."}
	}
	var soucis []string
	if lu.Version > Version {
		soucis = append(soucis, "travaux.json vient d'une version plus récente de "+
			"l'outil : ce qui n'est pas compris est laissé tel quel.")
	}
	gardees := make([]Assignment, 0, len(lu.Assignments))
	vus := map[string]bool{}
	for _, travail := range lu.Assignments {
		propre, err := travail.validate()
		if err != nil {
			soucis = append(soucis, "« "+travail.ID+" » : "+err.Error())
			continue
		}
		// Une échéance vide n'en est pas une : c'est ainsi qu'on la retire, et
		// une ligne qui n'en porte pas ne dit rien.
		if propre.Due == "" || vus[propre.Key()] {
			continue
		}
		vus[propre.Key()] = true
		gardees = append(gardees, propre)
	}
	return gardees, soucis
}

// encodeAssignments écrit le fichier des travaux. L'ordre est fixe pour que
// deux écritures du même registre donnent le même fichier : un fichier qui
// change sans que rien n'ait changé se relit mal.
func encodeAssignments(travaux []Assignment) ([]byte, error) {
	ranges := append([]Assignment(nil), travaux...)
	sort.SliceStable(ranges, func(i, j int) bool { return ranges[i].Key() < ranges[j].Key() })
	if ranges == nil {
		ranges = []Assignment{}
	}
	payload, err := json.MarshalIndent(
		assignmentsFile{Version: Version, Assignments: ranges}, "", "  ")
	if err != nil {
		return nil, valid.Errorf("Travaux non encodés : %v", err)
	}
	return append(payload, '\n'), nil
}
