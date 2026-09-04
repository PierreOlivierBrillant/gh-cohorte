// Package naming compose et relit les noms de dépôts.
//
// Un dépôt porte cinq niveaux, séparés par un point :
//
//	session . cours . groupe . travail . étudiant
//	a26.5n6.01.tp1.emilie-cote
//
// Le point est réservé à cette découpe, et rien d'autre ne peut en produire :
// la slugification remplace tout caractère non alphanumérique par un tiret, si
// bien qu'un nom de cours, de travail ou d'étudiant venu d'un CSV en est
// nettoyé sans qu'on ait à s'en occuper. Un compte GitHub, lui, n'en contient
// jamais.
//
// C'est ce qui distingue cette nomenclature de l'ancienne, tout en tirets : un
// nom se relit sans rien deviner. « a26-5n6-tp1-emilie-cote » ne disait pas où
// finissait le travail et où commençait la personne ; « a26.5n6.01.tp1.emilie-cote »
// le dit.
//
// Seul le nom court d'une session entre dans un dépôt. Son nom long — « Automne
// 2026 » — ne sert qu'à l'affichage et vit dans le fichier des groupes.
package naming

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Separator sépare les niveaux d'un nom de dépôt.
const Separator = "."

// Levels est le nombre de niveaux d'un nom complet.
const Levels = 5

// Parts est un nom de dépôt découpé.
type Parts struct {
	Session    string `json:"session"`
	Course     string `json:"course"`
	Group      string `json:"group"`
	Assignment string `json:"assignment"`
	Student    string `json:"student"`
}

// Prefix est ce qui désigne un groupe : « a26.5n6.01 ».
func Prefix(session, course, group string) string {
	return join(session, course, group)
}

// AssignmentID est ce qui désigne un travail : « a26.5n6.01.tp1 ».
func AssignmentID(session, course, group, assignment string) string {
	return join(session, course, group, assignment)
}

// Compose assemble le nom complet d'un dépôt.
func Compose(session, course, group, assignment, student string) string {
	return join(session, course, group, assignment, student)
}

func join(fragments ...string) string {
	cleaned := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		cleaned = append(cleaned, strings.TrimSpace(fragment))
	}
	return strings.Join(cleaned, Separator)
}

// Parse découpe un nom de dépôt. Un nom qui n'a pas exactement cinq niveaux
// non vides n'est pas de cette nomenclature : il ne relève pas de l'outil.
func Parse(name string) (Parts, bool) {
	fragments := strings.Split(strings.TrimSpace(name), Separator)
	if len(fragments) != Levels {
		return Parts{}, false
	}
	for _, fragment := range fragments {
		if fragment == "" {
			return Parts{}, false
		}
	}
	return Parts{
		Session: fragments[0], Course: fragments[1], Group: fragments[2],
		Assignment: fragments[3], Student: fragments[4],
	}, true
}

// Belongs dit si un dépôt appartient au groupe donné.
func Belongs(parts Parts, session, course, group string) bool {
	return strings.EqualFold(parts.Session, session) &&
		strings.EqualFold(parts.Course, course) &&
		strings.EqualFold(parts.Group, group)
}

// Fragment valide un niveau saisi à la main — session, cours, groupe, travail.
// La slugification écarte déjà le séparateur ; la vérification qui suit n'est
// là que pour que l'invariant soit dit à l'endroit où il compte.
func Fragment(value, label string) (string, error) {
	slug, err := valid.SlugFragment(value, label)
	if err != nil {
		return "", err
	}
	if strings.Contains(slug, Separator) {
		return "", valid.Errorf(
			"%s : « %s » est réservé à la séparation des niveaux du nom.", label, Separator)
	}
	return slug, nil
}

// Student compose le fragment d'une personne à partir de son nom complet. Un
// nom complet vide ne donne rien : le dépôt ne peut pas être nommé.
func Student(fullName string) (string, error) {
	slug := valid.Slugify(fullName)
	if slug == "" {
		return "", valid.Errorf(
			"Nom complet manquant : le nom du dépôt en dépend désormais.")
	}
	if len(slug) > valid.MaxSlugLength {
		slug = strings.Trim(slug[:valid.MaxSlugLength], "-")
	}
	return slug, nil
}
