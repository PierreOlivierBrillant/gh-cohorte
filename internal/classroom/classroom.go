// Package classroom tient la notion de groupe : des étudiants, un préfixe de
// dépôts, et les réglages que ses travaux reprennent.
//
// GitHub reste la source de vérité. Un travail n'est pas une fiche enregistrée
// quelque part : c'est l'ensemble des dépôts nommés « préfixe-travail-compte »,
// lus dans l'organisation. Le groupe ne retient que ce que les noms de dépôts
// ne savent pas dire — qui sont les étudiants, et avec quels réglages leurs
// dépôts sont créés. Un groupe se déclare donc sans rien écrire sur GitHub, et
// se supprime sans rien y effacer.
//
// C'est aussi la liste des étudiants qui permet de relire un nom de dépôt :
// « a26-5n6-tp1-emilie-cote » ne se découpe en travail et en compte que si l'on
// sait déjà que « emilie-cote » est du groupe.
package classroom

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Defaults rassemble les réglages que les travaux du groupe reprennent.
type Defaults struct {
	NamePattern        string  `json:"name_pattern"`
	DescriptionPattern string  `json:"description_pattern"`
	Template           string  `json:"template"`
	Visibility         string  `json:"visibility"`
	Permission         string  `json:"permission"`
	AddCollaborator    bool    `json:"add_collaborator"`
	StarterDir         string  `json:"starter_dir"`
	CommitMessage      string  `json:"commit_message"`
	DelaySeconds       float64 `json:"delay_seconds"`
}

// DefaultsFrom reprend les réglages généraux comme point de départ d'un groupe.
func DefaultsFrom(settings config.Settings) Defaults {
	return Defaults{
		NamePattern:        settings.NamePattern,
		DescriptionPattern: settings.DescriptionPattern,
		Template:           settings.Template,
		Visibility:         settings.Visibility,
		Permission:         settings.Permission,
		AddCollaborator:    settings.AddCollaborator,
		StarterDir:         settings.StarterDir,
		CommitMessage:      settings.CommitMessage,
		DelaySeconds:       settings.DelaySeconds,
	}
}

// normalized comble les valeurs absentes par celles de l'outil.
func (d Defaults) normalized() Defaults {
	repli := config.Default()
	if strings.TrimSpace(d.NamePattern) == "" {
		d.NamePattern = repli.NamePattern
	}
	if strings.TrimSpace(d.Visibility) == "" {
		d.Visibility = repli.Visibility
	}
	if strings.TrimSpace(d.Permission) == "" {
		d.Permission = repli.Permission
	}
	if strings.TrimSpace(d.CommitMessage) == "" {
		d.CommitMessage = repli.CommitMessage
	}
	if d.DelaySeconds < 0 {
		d.DelaySeconds = repli.DelaySeconds
	}
	return d
}

// Classroom est un groupe : une organisation, un préfixe, des étudiants.
type Classroom struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Org  string `json:"org"`
	// Prefix précède le nom de chaque travail. Vide, le groupe couvre les
	// travaux nommés directement à la racine de l'organisation.
	Prefix     string          `json:"prefix"`
	Students   []roster.Person `json:"students"`
	RosterPath string          `json:"roster_path,omitempty"`
	Defaults   Defaults        `json:"defaults"`
	CreatedAt  string          `json:"created_at"`
}

// Validate met le groupe en forme et refuse ce qui ne peut pas nommer un dépôt.
func (c Classroom) Validate() (Classroom, error) {
	org, err := valid.Login(c.Org, "Organisation")
	if err != nil {
		return c, err
	}
	c.Org = org

	if strings.TrimSpace(c.Prefix) != "" {
		prefix, err := valid.SlugFragment(c.Prefix, "Préfixe du groupe")
		if err != nil {
			return c, err
		}
		c.Prefix = prefix
	} else {
		c.Prefix = ""
	}

	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		c.Name = c.Label()
	}
	if _, err := plan.ValidatePattern(c.Defaults.NamePattern, "Gabarit de nom", true); err != nil {
		return c, err
	}
	c.Defaults = c.Defaults.normalized()
	c.Students = dedupe(c.Students)
	return c, nil
}

// Label décrit le groupe quand il n'a pas de nom propre.
func (c Classroom) Label() string {
	if c.Prefix == "" {
		return c.Org
	}
	return c.Prefix
}

// AssignmentID compose l'identifiant complet d'un travail du groupe : c'est lui
// qui préfixe les dépôts, et qui sert d'identifiant de travail à l'outil.
func (c Classroom) AssignmentID(name string) string {
	name = strings.Trim(strings.TrimSpace(name), groups.Separator)
	if c.Prefix == "" {
		return name
	}
	if name == "" {
		return c.Prefix
	}
	return c.Prefix + groups.Separator + name
}

// ShortName retire le préfixe du groupe d'un identifiant de travail.
func (c Classroom) ShortName(id string) string {
	if c.Prefix == "" {
		return id
	}
	if len(id) > len(c.Prefix)+1 &&
		strings.EqualFold(id[:len(c.Prefix)+1], c.Prefix+groups.Separator) {
		return id[len(c.Prefix)+1:]
	}
	return id
}

// Owns dit si un identifiant de travail relève du groupe.
func (c Classroom) Owns(id string) bool {
	if strings.TrimSpace(id) == "" {
		return false
	}
	if c.Prefix == "" {
		return true
	}
	return len(id) > len(c.Prefix)+1 &&
		strings.EqualFold(id[:len(c.Prefix)+1], c.Prefix+groups.Separator)
}

// Settings compose les réglages d'un travail du groupe, prêts pour le plan.
func (c Classroom) Settings(assignmentName string) config.Settings {
	defauts := c.Defaults.normalized()
	settings := config.Default()
	settings.Org = c.Org
	settings.Assignment = c.AssignmentID(assignmentName)
	settings.NamePattern = defauts.NamePattern
	settings.DescriptionPattern = defauts.DescriptionPattern
	settings.Template = defauts.Template
	settings.Visibility = defauts.Visibility
	settings.Permission = defauts.Permission
	settings.AddCollaborator = defauts.AddCollaborator
	settings.StarterDir = defauts.StarterDir
	settings.CommitMessage = defauts.CommitMessage
	settings.DelaySeconds = defauts.DelaySeconds
	settings.RosterPath = c.RosterPath
	return settings
}

// Has dit si un compte GitHub figure parmi les étudiants du groupe.
func (c Classroom) Has(username string) bool {
	for _, student := range c.Students {
		if strings.EqualFold(student.Username, username) {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------------- travaux

// Assignment est un travail du groupe, tel que les dépôts le racontent.
type Assignment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Repos    int    `json:"repos"`
	Students int    `json:"students"` // étudiants du groupe qui ont un dépôt
	Others   int    `json:"others"`   // dépôts dont le compte n'est pas du groupe
	PushedAt string `json:"pushed_at"`
}

// Assignments retrouve les travaux du groupe parmi les dépôts de l'organisation.
//
// Deux lectures se complètent : les dépôts des étudiants du groupe, relus par le
// gabarit — exact, même pour un travail distribué à une seule personne —, et la
// détection par préfixe, qui rattrape les travaux dont aucun étudiant inscrit
// n'a de dépôt.
func (c Classroom) Assignments(repos []groups.RepoInfo) []Assignment {
	identifiants := map[string]string{} // minuscules → identifiant tel qu'écrit
	retenir := func(id string) {
		if c.Owns(id) {
			identifiants[strings.ToLower(id)] = id
		}
	}

	pattern := c.Defaults.normalized().NamePattern
	for _, student := range c.Students {
		expression := plan.Matcher(pattern, student)
		if expression == nil {
			break
		}
		for _, repo := range repos {
			if id, found := plan.Assignment(expression, repo.Name); found {
				retenir(id)
			}
		}
	}

	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		names = append(names, repo.Name)
	}
	for _, detected := range groups.Detect(names, 2) {
		retenir(detected.Prefix)
	}

	trouves := make([]Assignment, 0, len(identifiants))
	for _, id := range identifiants {
		group := groups.Build(id, repos)
		if group.Len() == 0 {
			continue
		}
		travail := Assignment{ID: id, Name: c.ShortName(id), Repos: group.Len()}
		for _, repo := range group.Repos {
			if c.Has(repo.Suffix) {
				travail.Students++
			} else {
				travail.Others++
			}
			if repo.PushedAt > travail.PushedAt {
				travail.PushedAt = repo.PushedAt
			}
		}
		trouves = append(trouves, travail)
	}

	// Un travail dont les dépôts englobent ceux d'un autre n'en est pas un :
	// « a26-5n6 » disparaît devant « a26-5n6-tp1 » quand tous ses dépôts y sont.
	trouves = withoutUmbrellas(trouves)
	sort.Slice(trouves, func(i, j int) bool {
		if trouves[i].PushedAt != trouves[j].PushedAt {
			return trouves[i].PushedAt > trouves[j].PushedAt
		}
		return strings.ToLower(trouves[i].Name) < strings.ToLower(trouves[j].Name)
	})
	return trouves
}

// withoutUmbrellas écarte les identifiants qui ne font que chapeauter d'autres
// travaux, sans dépôt propre.
func withoutUmbrellas(found []Assignment) []Assignment {
	couverts := map[string]int{}
	for _, travail := range found {
		for _, autre := range found {
			if travail.ID == autre.ID {
				continue
			}
			if strings.HasPrefix(strings.ToLower(autre.ID),
				strings.ToLower(travail.ID)+groups.Separator) {
				couverts[travail.ID] += autre.Repos
			}
		}
	}
	gardes := make([]Assignment, 0, len(found))
	for _, travail := range found {
		if couverts[travail.ID] >= travail.Repos {
			continue
		}
		gardes = append(gardes, travail)
	}
	return gardes
}

// Served renvoie les comptes du groupe qui ont déjà un dépôt pour ce travail.
func (c Classroom) Served(assignmentID string, repos []groups.RepoInfo) map[string]bool {
	taken := groups.Build(assignmentID, repos).Suffixes()
	servis := map[string]bool{}
	for _, student := range c.Students {
		if taken[strings.ToLower(student.Username)] {
			servis[strings.ToLower(student.Username)] = true
		}
	}
	return servis
}

// ----------------------------------------------------------------- candidats

// Candidate est un groupe possible, deviné des dépôts déjà présents.
type Candidate struct {
	Prefix      string   `json:"prefix"`
	Assignments []string `json:"assignments"`
	Repos       int      `json:"repos"`
	Students    []string `json:"students"`
}

// Candidates propose des groupes à partir des dépôts de l'organisation : les
// travaux détectés sont regroupés par ce qui les précède. « a26-5n6-tp1 » et
// « a26-5n6-travailsession » proposent ainsi le groupe « a26-5n6 », avec les
// comptes qu'on y trouve déjà.
func Candidates(repos []groups.RepoInfo) []Candidate {
	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		names = append(names, repo.Name)
	}

	parents := map[string]*Candidate{}
	comptes := map[string]map[string]bool{}
	for _, detected := range groups.Detect(names, 2) {
		segments := strings.Split(detected.Prefix, groups.Separator)
		prefix := ""
		if len(segments) > 1 {
			prefix = strings.Join(segments[:len(segments)-1], groups.Separator)
		}
		court := segments[len(segments)-1]

		candidat, connu := parents[prefix]
		if !connu {
			candidat = &Candidate{Prefix: prefix}
			parents[prefix] = candidat
			comptes[prefix] = map[string]bool{}
		}
		candidat.Assignments = append(candidat.Assignments, court)
		group := groups.Build(detected.Prefix, repos)
		candidat.Repos += group.Len()
		for _, repo := range group.Repos {
			comptes[prefix][strings.ToLower(repo.Suffix)] = true
		}
	}

	proposes := make([]Candidate, 0, len(parents))
	for prefix, candidat := range parents {
		for compte := range comptes[prefix] {
			candidat.Students = append(candidat.Students, compte)
		}
		sort.Strings(candidat.Students)
		sort.Strings(candidat.Assignments)
		proposes = append(proposes, *candidat)
	}
	// D'abord les préfixes qui rassemblent le plus de travaux : ce sont les plus
	// susceptibles d'être de vrais groupes.
	sort.Slice(proposes, func(i, j int) bool {
		if len(proposes[i].Assignments) != len(proposes[j].Assignments) {
			return len(proposes[i].Assignments) > len(proposes[j].Assignments)
		}
		if proposes[i].Repos != proposes[j].Repos {
			return proposes[i].Repos > proposes[j].Repos
		}
		return proposes[i].Prefix < proposes[j].Prefix
	})
	return proposes
}

// StudentsOf construit une liste d'étudiants à partir de comptes GitHub seuls :
// les noms complets restent à retrouver.
func StudentsOf(usernames []string) []roster.Person {
	people := make([]roster.Person, 0, len(usernames))
	for _, login := range usernames {
		cleaned, err := valid.Login(login, "")
		if err != nil {
			continue
		}
		people = append(people, roster.Person{Username: cleaned})
	}
	return dedupe(people)
}

// dedupe écarte les doublons de comptes, en gardant le premier nom connu.
func dedupe(people []roster.Person) []roster.Person {
	vus := map[string]int{}
	uniques := make([]roster.Person, 0, len(people))
	for _, person := range people {
		person.Username = strings.TrimSpace(person.Username)
		person.FullName = strings.TrimSpace(person.FullName)
		if person.Username == "" {
			continue
		}
		if position, connu := vus[person.Key()]; connu {
			if uniques[position].FullName == "" && person.FullName != "" {
				uniques[position].FullName = person.FullName
			}
			continue
		}
		vus[person.Key()] = len(uniques)
		uniques = append(uniques, person)
	}
	return uniques
}
