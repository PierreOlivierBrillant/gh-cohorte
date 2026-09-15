// Package rules porte ce que l'organisation déclare et que le code ne doit pas
// savoir.
//
// Un cours change de sigle : « 5N6 » devient « 5M6 », et les copies d'il y a
// trois ans cessent d'être retrouvées par celles d'aujourd'hui. Un travail
// change de nom : « tp1 » devient « tp-1 ». Rien de tout cela n'appartient à
// l'outil — c'est l'affaire d'un département, cela varie d'un collège à
// l'autre, et cela changera encore. L'écrire dans le code obligerait à publier
// une version de l'extension chaque fois qu'un programme est révisé.
//
// Ces règles vivent donc dans le registre de l'organisation, à côté des noms et
// des échéances : partagées par toute l'équipe, versionnées, modifiables à la
// main sur github.com. Un fichier passé en argument peut les surcharger pour
// une analyse — le temps d'essayer quelque chose, ou pour une GitHub Action qui
// porte les siennes.
package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Version est celle du schéma écrit. Elle est relue, jamais devinée : un
// fichier venu d'une version ultérieure doit pouvoir se signaler plutôt que
// d'être à moitié compris.
const Version = 1

// Rules est ce que l'organisation déclare.
type Rules struct {
	Version int `json:"version"`
	// Courses réunit les sigles qui désignent le même cours au fil des ans.
	Courses []Course `json:"courses,omitempty"`
	// Assignments réunit les noms qui désignent le même travail.
	Assignments []Alias `json:"assignments,omitempty"`
	// Profiles sont les profils d'inspection de l'équipe, qui s'ajoutent à ceux
	// que l'outil connaît d'office et les remplacent à identifiant égal.
	Profiles []inspect.Profile `json:"profiles,omitempty"`
	Defaults Defaults          `json:"defaults,omitzero"`
}

// Course réunit les sigles d'un même cours.
type Course struct {
	// ID nomme le cours d'un nom qui ne bouge pas : c'est lui qu'on écrit une
	// fois, et les sigles tournent autour.
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	// Codes énumère les sigles, du plus récent au plus ancien de préférence.
	Codes []string `json:"codes"`
}

// Alias réunit les noms d'un même travail.
type Alias struct {
	ID      string   `json:"id"`
	Aliases []string `json:"aliases"`
}

// Defaults sont les bornes que l'équipe préfère. Zéro laisse celles de l'outil.
type Defaults struct {
	Kgram  int     `json:"kgram,omitempty"`
	Window int     `json:"window,omitempty"`
	Noise  float64 `json:"noise,omitempty"`
}

// Empty dit qu'il n'y a rien à écrire.
func (r Rules) Empty() bool {
	return len(r.Courses) == 0 && len(r.Assignments) == 0 &&
		len(r.Profiles) == 0 && r.Defaults == Defaults{}
}

// Validate met les règles en forme et refuse ce qui ne peut pas s'appliquer.
//
// Un sigle qui figurerait dans deux cours rendrait la question « à quel cours
// appartient 5N6 » sans réponse : selon l'ordre de lecture, une analyse
// ramènerait les copies d'un programme ou d'un autre. Mieux vaut refuser à
// l'écriture que rendre un corpus faux sans le dire.
func (r Rules) Validate() (Rules, error) {
	r.Version = Version
	vus := map[string]string{}
	cours := make([]Course, 0, len(r.Courses))
	for _, item := range r.Courses {
		id := slug(item.ID)
		if id == "" {
			return r, valid.Errorf("Règles : un cours sans identifiant.")
		}
		codes := uniques(item.Codes)
		if len(codes) == 0 {
			return r, valid.Errorf("Règles : le cours « %s » ne nomme aucun sigle.", id)
		}
		for _, code := range codes {
			if autre, deja := vus[code]; deja && autre != id {
				return r, valid.Errorf(
					"Règles : le sigle « %s » appartient à la fois à « %s » et à « %s ». "+
						"Un sigle ne peut désigner qu'un cours.", code, autre, id)
			}
			vus[code] = id
		}
		cours = append(cours, Course{ID: id, Label: strings.TrimSpace(item.Label), Codes: codes})
	}
	sort.Slice(cours, func(first, second int) bool { return cours[first].ID < cours[second].ID })
	r.Courses = cours

	vusTravaux := map[string]string{}
	travaux := make([]Alias, 0, len(r.Assignments))
	for _, item := range r.Assignments {
		id := slug(item.ID)
		if id == "" {
			return r, valid.Errorf("Règles : un travail sans identifiant.")
		}
		noms := uniques(append([]string{id}, item.Aliases...))
		for _, nom := range noms {
			if autre, deja := vusTravaux[nom]; deja && autre != id {
				return r, valid.Errorf(
					"Règles : le nom de travail « %s » appartient à la fois à « %s » et "+
						"à « %s ».", nom, autre, id)
			}
			vusTravaux[nom] = id
		}
		travaux = append(travaux, Alias{ID: id, Aliases: without(noms, id)})
	}
	sort.Slice(travaux, func(first, second int) bool {
		return travaux[first].ID < travaux[second].ID
	})
	r.Assignments = travaux

	for _, profil := range r.Profiles {
		if slug(profil.ID) == "" {
			return r, valid.Errorf("Règles : un profil d'inspection sans identifiant.")
		}
	}
	if r.Defaults.Noise < 0 || r.Defaults.Noise > 1 {
		return r, valid.Errorf(
			"Règles : la part d'empreintes banales doit être comprise entre 0 et 1 (%.2f).",
			r.Defaults.Noise)
	}
	return r, nil
}

// SameCourse rend les sigles qui désignent le même cours, celui-ci compris.
//
// Un sigle qu'aucune règle ne nomme n'a d'équivalent que lui-même : ne rien
// déclarer revient donc à comparer un cours avec lui seul, ce qui est le
// comportement qu'on attend quand on n'a rien dit.
func (r Rules) SameCourse(code string) []string {
	code = slug(code)
	if code == "" {
		return nil
	}
	for _, cours := range r.Courses {
		if contains(cours.Codes, code) {
			return append([]string(nil), cours.Codes...)
		}
	}
	return []string{code}
}

// SameAssignment rend les noms qui désignent le même travail, celui-ci compris.
func (r Rules) SameAssignment(name string) []string {
	name = slug(name)
	if name == "" {
		return nil
	}
	for _, travail := range r.Assignments {
		if travail.ID == name || contains(travail.Aliases, name) {
			return append([]string{travail.ID}, travail.Aliases...)
		}
	}
	return []string{name}
}

// CourseLabel rend le nom long d'un cours à partir d'un de ses sigles.
func (r Rules) CourseLabel(code string) string {
	code = slug(code)
	for _, cours := range r.Courses {
		if contains(cours.Codes, code) {
			if cours.Label != "" {
				return cours.Label
			}
			return cours.ID
		}
	}
	return ""
}

// Merge superpose d'autres règles sur celles-ci : à identifiant égal, les
// secondes gagnent. C'est ce qui permet à un fichier passé en argument de
// corriger le registre pour une analyse, sans le réécrire.
func (r Rules) Merge(other Rules) Rules {
	fusion := Rules{Version: Version, Defaults: r.Defaults}
	if other.Defaults.Kgram > 0 {
		fusion.Defaults.Kgram = other.Defaults.Kgram
	}
	if other.Defaults.Window > 0 {
		fusion.Defaults.Window = other.Defaults.Window
	}
	if other.Defaults.Noise > 0 {
		fusion.Defaults.Noise = other.Defaults.Noise
	}

	remplaces := map[string]bool{}
	for _, cours := range other.Courses {
		remplaces[slug(cours.ID)] = true
	}
	for _, cours := range r.Courses {
		if !remplaces[cours.ID] {
			fusion.Courses = append(fusion.Courses, cours)
		}
	}
	fusion.Courses = append(fusion.Courses, other.Courses...)

	remplacesTravaux := map[string]bool{}
	for _, travail := range other.Assignments {
		remplacesTravaux[slug(travail.ID)] = true
	}
	for _, travail := range r.Assignments {
		if !remplacesTravaux[travail.ID] {
			fusion.Assignments = append(fusion.Assignments, travail)
		}
	}
	fusion.Assignments = append(fusion.Assignments, other.Assignments...)

	remplacesProfils := map[string]bool{}
	for _, profil := range other.Profiles {
		remplacesProfils[slug(profil.ID)] = true
	}
	for _, profil := range r.Profiles {
		if !remplacesProfils[slug(profil.ID)] {
			fusion.Profiles = append(fusion.Profiles, profil)
		}
	}
	fusion.Profiles = append(fusion.Profiles, other.Profiles...)
	return fusion
}

// ------------------------------------------------------------------ fichier

// Decode relit des règles écrites. Ce qui ne se comprend pas est signalé plutôt
// que de priver toute l'organisation des siennes : le fichier se modifie à la
// main sur github.com, et une virgule de trop ne doit pas tout emporter.
func Decode(content []byte) (Rules, []string) {
	var lues Rules
	if err := json.Unmarshal(content, &lues); err != nil {
		return Rules{}, []string{fmt.Sprintf("Règles illisibles : %v.", err)}
	}
	if lues.Version > Version {
		return Rules{}, []string{fmt.Sprintf(
			"Règles : elles viennent d'une version %d de l'outil, qui n'en connaît que %d. "+
				"Mettez l'extension à jour (gh extension upgrade cohorte).",
			lues.Version, Version)}
	}
	valides, err := lues.Validate()
	if err != nil {
		return Rules{}, []string{err.Error()}
	}
	return valides, nil
}

// Encode écrit les règles.
func Encode(rules Rules) ([]byte, error) {
	valides, err := rules.Validate()
	if err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(valides, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

// Load lit un fichier de règles passé en argument.
func Load(path string) (Rules, error) {
	if strings.TrimSpace(path) == "" {
		return Rules{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Rules{}, valid.Errorf("Règles « %s » illisibles : %v.", path, err)
	}
	lues, soucis := Decode(content)
	if len(soucis) > 0 {
		return Rules{}, valid.Errorf("Règles « %s » : %s", path, soucis[0])
	}
	return lues, nil
}

// ------------------------------------------------------------------ outils

// slug met un identifiant en forme : minuscules, sans espaces autour. Les
// sigles s'écrivent « 5N6 » sur un plan de cours et « 5n6 » dans un nom de
// dépôt ; les distinguer ferait échouer l'équivalence sur une majuscule.
func slug(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func uniques(values []string) []string {
	vus := map[string]bool{}
	gardes := make([]string, 0, len(values))
	for _, value := range values {
		if mis := slug(value); mis != "" && !vus[mis] {
			vus[mis] = true
			gardes = append(gardes, mis)
		}
	}
	return gardes
}

func without(values []string, removed string) []string {
	gardes := make([]string, 0, len(values))
	for _, value := range values {
		if value != removed {
			gardes = append(gardes, value)
		}
	}
	return gardes
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
