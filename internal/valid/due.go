package valid

import (
	"strings"
	"time"
)

// La forme d'une date cible est ici, et non dans le paquet qui lui donne son
// sens : le registre de l'organisation l'écrit, le groupe la lit, et aucun des
// deux ne doit dépendre de l'autre pour savoir ce qu'est une date bien écrite.

// Formes acceptées d'une date cible, de la plus précise à la plus lâche.
const (
	DueDateTime = "2006-01-02T15:04"
	DueDate     = "2006-01-02"
)

// NormalizeDue met une date cible sous sa forme normale, ou refuse ce qui n'en
// est pas une. Une valeur vide reste vide : c'est ainsi qu'on retire une date.
func NormalizeDue(value string) (string, error) {
	texte := strings.TrimSpace(value)
	if texte == "" {
		return "", nil
	}
	// La forme longue d'abord : « 2026-10-01 » est un préfixe de
	// « 2026-10-01T23:59 », et l'essayer en premier tronquerait l'heure.
	if moment, err := time.ParseInLocation(DueDateTime, texte, time.Local); err == nil {
		return moment.Format(DueDateTime), nil
	}
	if jour, err := time.ParseInLocation(DueDate, texte, time.Local); err == nil {
		return jour.Format(DueDate), nil
	}
	return "", Errorf(
		"Date cible : « %s » n'est pas une date (attendu AAAA-MM-JJ ou AAAA-MM-JJTHH:MM).",
		texte)
}

// ParseDue relit une date cible et rend l'instant qu'elle désigne, dans le
// fuseau de la machine — celui où travaille la personne qui enseigne, et celui
// dans lequel elle a annoncé l'échéance à sa classe.
//
// Une date sans heure vaut la fin de la journée : « remis le 1er octobre » veut
// dire avant que le 1er octobre ne soit fini, pas avant qu'il ne commence.
//
// Une valeur vide n'est pas une erreur : c'est un travail sans échéance.
func ParseDue(value string) (time.Time, error) {
	texte := strings.TrimSpace(value)
	if texte == "" {
		return time.Time{}, nil
	}
	if moment, err := time.ParseInLocation(DueDateTime, texte, time.Local); err == nil {
		return moment, nil
	}
	if jour, err := time.ParseInLocation(DueDate, texte, time.Local); err == nil {
		// Fin de journée : la seconde qui précède le lendemain.
		return jour.AddDate(0, 0, 1).Add(-time.Second), nil
	}
	return time.Time{}, Errorf(
		"Date cible : « %s » n'est pas une date (attendu AAAA-MM-JJ ou AAAA-MM-JJTHH:MM).",
		texte)
}
