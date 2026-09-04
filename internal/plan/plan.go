// Package plan construit le plan de génération : quel dépôt pour quelle personne.
package plan

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Placeholders énumère les champs autorisés dans les gabarits.
//
// {assignment} est l'identifiant complet du travail — « a26.5n6.01.tp1 » sous la
// nomenclature courante. {title} n'en garde que le dernier niveau, « tp1 » :
// c'est ce qu'on veut lire dans la description d'un dépôt, où le chemin complet
// n'apprend rien de plus que son nom.
var Placeholders = []string{
	"assignment", "title", "username", "name", "fullname", "first", "last", "index",
}

var placeholderRe = regexp.MustCompile(`\{([a-z_]+)\}`)

// Champs qui rendent un nom de dépôt distinctif d'une personne à l'autre.
var distinctive = map[string]bool{"username": true, "name": true, "index": true}

// PlannedRepo est un dépôt à créer pour une personne donnée.
type PlannedRepo struct {
	Person      roster.Person
	Name        string
	Description string
}

// ValidatePattern vérifie qu'un gabarit n'utilise que des champs connus et reste distinctif.
func ValidatePattern(pattern, label string, requireUnique bool) (string, error) {
	text := strings.TrimSpace(pattern)
	if text == "" {
		return "", valid.Errorf("%s : le gabarit est vide.", label)
	}
	used := map[string]bool{}
	for _, match := range placeholderRe.FindAllStringSubmatch(text, -1) {
		field := match[1]
		if !containsString(Placeholders, field) {
			allowed := make([]string, 0, len(Placeholders))
			for _, name := range Placeholders {
				allowed = append(allowed, "{"+name+"}")
			}
			return "", valid.Errorf("%s : champ inconnu {%s}. Champs disponibles : %s.",
				label, field, strings.Join(allowed, ", "))
		}
		used[field] = true
	}
	if requireUnique {
		unique := false
		for field := range used {
			if distinctive[field] {
				unique = true
			}
		}
		if !unique {
			return "", valid.Errorf(
				"%s : le gabarit doit contenir {username}, {name} ou {index} "+
					"pour que chaque personne ait un dépôt distinct.", label)
		}
	}
	return text, nil
}

// fields calcule les valeurs des champs pour une personne.
func fields(person roster.Person, assignment string, index int) map[string]string {
	parts := strings.Fields(person.FullName)
	first, last := "", ""
	if len(parts) > 0 {
		first = valid.Slugify(parts[0])
	}
	if len(parts) > 1 {
		last = valid.Slugify(parts[len(parts)-1])
	}
	return map[string]string{
		"assignment": assignment,
		"title":      title(assignment),
		"username":   person.Username,
		"name":       valid.Slugify(person.FullName),
		"fullname":   person.FullName,
		"first":      first,
		"last":       last,
		"index":      fmt.Sprintf("%02d", index),
	}
}

// title ne garde du travail que son dernier niveau.
func title(assignment string) string {
	if position := strings.LastIndex(assignment, "."); position >= 0 {
		return assignment[position+1:]
	}
	return assignment
}

// Render remplit un gabarit pour une personne.
func Render(pattern string, person roster.Person, assignment string, index int) string {
	values := fields(person, assignment, index)
	return placeholderRe.ReplaceAllStringFunc(pattern, func(match string) string {
		return values[strings.Trim(match, "{}")]
	})
}

// Matcher construit l'expression qui reconnaît les dépôts d'une personne et en
// extrait l'identifiant du travail : c'est l'inverse de Render. Elle permet de
// rattacher un dépôt existant à un travail sans deviner où finit le nom du
// travail et où commence le compte — « a26-5n6-tp1-emilie-cote » ne se découpe
// pas autrement que si l'on connaît déjà « emilie-cote ».
//
// Le champ {index} n'est pas connu à la relecture : n'importe quel nombre y est
// accepté. Un gabarit sans {assignment} ne se relit pas et renvoie nil.
func Matcher(pattern string, person roster.Person) *regexp.Regexp {
	// {title} ne se relit pas : deux champs libres dans un même nom ne se
	// découpent pas sans deviner.
	if !strings.Contains(pattern, "{assignment}") || strings.Contains(pattern, "{title}") {
		return nil
	}
	values := fields(person, "", 1)

	var motif strings.Builder
	// Les noms de dépôts GitHub ne se distinguent pas par la casse.
	motif.WriteString(`(?i)\A`)
	position := 0
	for _, bornes := range placeholderRe.FindAllStringSubmatchIndex(pattern, -1) {
		motif.WriteString(regexp.QuoteMeta(pattern[position:bornes[0]]))
		switch champ := pattern[bornes[2]:bornes[3]]; champ {
		case "assignment":
			motif.WriteString(`(.+)`)
		case "index":
			motif.WriteString(`\d+`)
		default:
			motif.WriteString(regexp.QuoteMeta(values[champ]))
		}
		position = bornes[1]
	}
	motif.WriteString(regexp.QuoteMeta(pattern[position:]))
	motif.WriteString(`\z`)

	expression, err := regexp.Compile(motif.String())
	if err != nil {
		return nil
	}
	return expression
}

// Assignment retrouve l'identifiant du travail auquel un dépôt appartient, pour
// une personne donnée.
func Assignment(expression *regexp.Regexp, repoName string) (string, bool) {
	if expression == nil {
		return "", false
	}
	parts := expression.FindStringSubmatch(repoName)
	if parts == nil || len(parts) < 2 || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// Build construit le plan complet et refuse toute collision de noms de dépôts.
func Build(people []roster.Person, settings config.Settings) ([]PlannedRepo, error) {
	if _, err := ValidatePattern(settings.NamePattern, "Gabarit de nom", true); err != nil {
		return nil, err
	}
	description := settings.DescriptionPattern
	if description != "" {
		if _, err := ValidatePattern(description, "Gabarit de description", false); err != nil {
			return nil, err
		}
	}

	plan := make([]PlannedRepo, 0, len(people))
	seen := map[string]roster.Person{}
	for position, person := range people {
		index := position + 1
		name, err := valid.RepoName(Render(settings.NamePattern, person, settings.Assignment, index))
		if err != nil {
			return nil, err
		}
		if clash, exists := seen[strings.ToLower(name)]; exists {
			return nil, valid.Errorf(
				"Collision de noms : « %s » servirait à la fois à %s et à %s. Ajustez le gabarit de nom.",
				name, clash.FullName, person.FullName)
		}
		seen[strings.ToLower(name)] = person

		text := ""
		if description != "" {
			text = strings.TrimSpace(Render(description, person, settings.Assignment, index))
		}
		// Découpe en runes : une description accentuée ne doit pas être coupée en deux.
		if runes := []rune(text); len(runes) > 350 {
			text = string(runes[:350])
		}
		plan = append(plan, PlannedRepo{Person: person, Name: name, Description: text})
	}
	return plan, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
