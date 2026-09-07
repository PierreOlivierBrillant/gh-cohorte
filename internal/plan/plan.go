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

// PlannedRepo est un dépôt à créer pour une personne, ou pour une équipe.
//
// Les deux ne se distinguent que par leur destinataire : un travail individuel
// remplit « Person », un travail d'équipe remplit « Team » et « TeamSlug ».
// L'accès s'accorde ensuite à l'un ou à l'autre — à l'équipe entière plutôt
// qu'à chacun de ses membres, ce qui fait que changer sa composition suffit à
// changer qui voit le dépôt.
type PlannedRepo struct {
	Person      roster.Person
	Team        string // nom court de l'équipe ; vide pour un travail individuel
	TeamSlug    string // adresse GitHub de l'équipe
	Name        string
	Description string
}

// ForTeam dit si le dépôt est celui d'une équipe.
func (p PlannedRepo) ForTeam() bool { return strings.TrimSpace(p.TeamSlug) != "" }

// Recipient nomme le destinataire du dépôt, pour l'affichage et les bilans.
func (p PlannedRepo) Recipient() string {
	if p.ForTeam() {
		return "Équipe " + p.Team
	}
	return p.Person.FullName
}

// TeamTarget est une équipe à qui distribuer un travail. Ses membres ne servent
// pas à nommer le dépôt — l'équipe le fait —, mais à dire qui il concerne.
type TeamTarget struct {
	Short   string
	Slug    string
	Members []string
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

// teamFields calcule les valeurs des champs pour une équipe.
//
// {name} rend le nom court de l'équipe : c'est lui qui occupe le dernier niveau
// du nom du dépôt, là où un travail individuel écrit le nom de l'étudiant. Le
// gabarit de nom est donc le même pour les deux, et « a26.5n6.01.tp1.eq1 » se
// lit comme « a26.5n6.01.tp1.emilie-cote ». {fullname} dit « Équipe eq1 », pour
// que la description d'un dépôt reste lisible sans gabarit particulier.
func teamFields(target TeamTarget, assignment string, index int) map[string]string {
	return map[string]string{
		"assignment": assignment,
		"title":      title(assignment),
		"username":   "",
		"name":       target.Short,
		"fullname":   "Équipe " + target.Short,
		"first":      "",
		"last":       "",
		"index":      fmt.Sprintf("%02d", index),
	}
}

// Render remplit un gabarit pour une personne.
func Render(pattern string, person roster.Person, assignment string, index int) string {
	return fill(pattern, fields(person, assignment, index))
}

// RenderTeam remplit un gabarit pour une équipe.
func RenderTeam(pattern string, target TeamTarget, assignment string, index int) string {
	return fill(pattern, teamFields(target, assignment, index))
}

func fill(pattern string, values map[string]string) string {
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
			// GitHub ajoute « -1 » au bout d'un nom de dépôt déjà pris. Quand
			// c'est la personne qui termine le nom, la marque se colle à elle :
			// « tp1-jlpicard-1 » reste le dépôt de jlpicard, et le lui refuser
			// en ferait un orphelin.
			if distinctive[champ] && bornes[1] == len(pattern) {
				motif.WriteString(`(?:-\d+)?`)
			}
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
	description, err := patterns(settings)
	if err != nil {
		return nil, err
	}
	assembleur := &assembler{settings: settings, description: description,
		seen: map[string]string{}}
	plan := make([]PlannedRepo, 0, len(people))
	for position, person := range people {
		index := position + 1
		item, err := assembleur.add(
			Render(settings.NamePattern, person, settings.Assignment, index),
			Render(description, person, settings.Assignment, index),
			person.FullName)
		if err != nil {
			return nil, err
		}
		item.Person = person
		plan = append(plan, item)
	}
	return plan, nil
}

// BuildTeams construit le plan d'un travail d'équipe : un dépôt par équipe,
// nommé d'après elle. C'est le même plan que pour des personnes — mêmes
// gabarits, mêmes collisions refusées —, seul le destinataire change.
func BuildTeams(targets []TeamTarget, settings config.Settings) ([]PlannedRepo, error) {
	description, err := patterns(settings)
	if err != nil {
		return nil, err
	}
	assembleur := &assembler{settings: settings, description: description,
		seen: map[string]string{}}
	plan := make([]PlannedRepo, 0, len(targets))
	for position, target := range targets {
		index := position + 1
		item, err := assembleur.add(
			RenderTeam(settings.NamePattern, target, settings.Assignment, index),
			RenderTeam(description, target, settings.Assignment, index),
			"l'équipe "+target.Short)
		if err != nil {
			return nil, err
		}
		item.Team, item.TeamSlug = target.Short, target.Slug
		plan = append(plan, item)
	}
	return plan, nil
}

// patterns valide les gabarits et renvoie celui des descriptions.
func patterns(settings config.Settings) (string, error) {
	if _, err := ValidatePattern(settings.NamePattern, "Gabarit de nom", true); err != nil {
		return "", err
	}
	description := settings.DescriptionPattern
	if description == "" {
		return "", nil
	}
	return ValidatePattern(description, "Gabarit de description", false)
}

// assembler compose les lignes du plan et retient les noms déjà visés : deux
// destinataires ne peuvent pas se voir attribuer le même dépôt.
type assembler struct {
	settings    config.Settings
	description string
	seen        map[string]string
}

// add valide un nom rendu et compose la ligne. « owner » nomme le destinataire :
// il ne sert qu'à dire, en cas de collision, qui se dispute le nom.
func (a *assembler) add(rendered, described, owner string) (PlannedRepo, error) {
	name, err := valid.RepoName(rendered)
	if err != nil {
		return PlannedRepo{}, err
	}
	if clash, exists := a.seen[strings.ToLower(name)]; exists {
		return PlannedRepo{}, valid.Errorf(
			"Collision de noms : « %s » servirait à la fois à %s et à %s. Ajustez le gabarit de nom.",
			name, clash, owner)
	}
	a.seen[strings.ToLower(name)] = owner

	text := ""
	if a.description != "" {
		text = strings.TrimSpace(described)
	}
	// Découpe en runes : une description accentuée ne doit pas être coupée en deux.
	if runes := []rune(text); len(runes) > 350 {
		text = string(runes[:350])
	}
	return PlannedRepo{Name: name, Description: text}, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
