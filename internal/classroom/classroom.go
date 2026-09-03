// Package classroom tient la notion de groupe : des étudiants, une place dans
// la nomenclature des dépôts, et les réglages que ses travaux reprennent.
//
// GitHub reste la source de vérité. Un travail n'est pas une fiche enregistrée
// quelque part : c'est l'ensemble des dépôts nommés
// « session.cours.groupe.travail.étudiant », lus dans l'organisation. Le groupe ne retient que ce que les noms de dépôts
// ne savent pas dire — qui sont les étudiants, et avec quels réglages leurs
// dépôts sont créés. Un groupe se déclare donc sans rien écrire sur GitHub, et
// se supprime sans rien y effacer.
//
// Les groupes déclarés avant cette nomenclature gardent leur
// préfixe tout en tirets. Ils restent lisibles — leurs dépôts s'affichent — mais
// on ne leur distribue plus : il faut d'abord les migrer.
package classroom

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// NamePattern est le gabarit des noms de dépôts. Il n'est pas réglable : c'est
// la nomenclature elle-même, et tout le reste en dépend.
const NamePattern = "{assignment}" + naming.Separator + "{name}"

// Defaults rassemble les réglages que les travaux du groupe reprennent.
type Defaults struct {
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
	if strings.TrimSpace(d.DescriptionPattern) == "" {
		d.DescriptionPattern = repli.DescriptionPattern
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

// Classroom est un groupe : une session, un cours, une organisation, des
// étudiants. Seul le nom court de la session entre dans les dépôts ; son nom
// long vit dans le magasin, partagé par tous les groupes de la session.
type Classroom struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Org     string `json:"org"`
	Session string `json:"session"`
	Course  string `json:"course"`
	Group   string `json:"group"`
	// LegacyPrefix est le préfixe tout en tirets d'un groupe déclaré avant cette
	// nomenclature. Sa présence dit qu'il reste à migrer.
	LegacyPrefix string          `json:"prefix,omitempty"`
	Students     []roster.Person `json:"students"`
	RosterPath   string          `json:"roster_path,omitempty"`
	Defaults     Defaults        `json:"defaults"`
	CreatedAt    string          `json:"created_at"`
}

// Legacy dit si le groupe suit encore l'ancienne nomenclature.
func (c Classroom) Legacy() bool {
	return strings.TrimSpace(c.Session) == "" && strings.TrimSpace(c.LegacyPrefix) != ""
}

// Validate met le groupe en forme et refuse ce qui ne peut pas nommer un dépôt.
func (c Classroom) Validate() (Classroom, error) {
	org, err := valid.Login(c.Org, "Organisation")
	if err != nil {
		return c, err
	}
	c.Org = org

	if c.Legacy() {
		prefix, err := valid.SlugFragment(c.LegacyPrefix, "Préfixe du groupe")
		if err != nil {
			return c, err
		}
		c.LegacyPrefix = prefix
	} else {
		session, err := naming.Fragment(c.Session, "Session")
		if err != nil {
			return c, err
		}
		course, err := naming.Fragment(c.Course, "Cours")
		if err != nil {
			return c, err
		}
		group, err := naming.Fragment(c.Group, "Groupe")
		if err != nil {
			return c, err
		}
		c.Session, c.Course, c.Group, c.LegacyPrefix = session, course, group, ""
	}

	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		c.Name = c.Label()
	}
	if strings.TrimSpace(c.Defaults.DescriptionPattern) != "" {
		if _, err := plan.ValidatePattern(
			c.Defaults.DescriptionPattern, "Gabarit de description", false); err != nil {
			return c, err
		}
	}
	c.Defaults = c.Defaults.normalized()
	c.Students = dedupe(c.Students)
	return c, nil
}

// Label décrit le groupe quand il n'a pas de nom propre.
func (c Classroom) Label() string {
	if c.Legacy() {
		return c.LegacyPrefix
	}
	return c.Course + " " + c.Group
}

// Session identifie une session : un nom court pour les dépôts, un nom long
// pour l'affichage.
type Session struct {
	Short string `json:"short"`
	Name  string `json:"name"`
}

// Scope est ce qui précède le nom d'un travail dans les dépôts du groupe.
func (c Classroom) Scope() string {
	if c.Legacy() {
		return c.LegacyPrefix
	}
	return naming.Prefix(c.Session, c.Course, c.Group)
}

// AssignmentID compose l'identifiant complet d'un travail du groupe : c'est lui
// qui précède le nom de l'étudiant dans le nom du dépôt.
func (c Classroom) AssignmentID(name string) string {
	name = strings.TrimSpace(name)
	if c.Legacy() {
		return strings.Trim(c.LegacyPrefix+groups.Separator+name, groups.Separator)
	}
	return naming.AssignmentID(c.Session, c.Course, c.Group, name)
}

// ShortName retire du travail ce qui désigne le groupe.
func (c Classroom) ShortName(id string) string {
	scope := c.Scope()
	separator := naming.Separator
	if c.Legacy() {
		separator = groups.Separator
	}
	if scope != "" && len(id) > len(scope)+1 &&
		strings.EqualFold(id[:len(scope)+1], scope+separator) {
		return id[len(scope)+1:]
	}
	return id
}

// Owns dit si un identifiant de travail relève du groupe.
func (c Classroom) Owns(id string) bool {
	if strings.TrimSpace(id) == "" {
		return false
	}
	scope := c.Scope()
	if scope == "" {
		return false
	}
	separator := naming.Separator
	if c.Legacy() {
		separator = groups.Separator
	}
	return len(id) > len(scope)+1 &&
		strings.EqualFold(id[:len(scope)+1], scope+separator)
}

// Settings compose les réglages d'un travail du groupe, prêts pour le plan.
func (c Classroom) Settings(assignmentName string) config.Settings {
	defauts := c.Defaults.normalized()
	settings := config.Default()
	settings.Org = c.Org
	settings.Assignment = c.AssignmentID(assignmentName)
	settings.NamePattern = NamePattern
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

// MissingNames renvoie les étudiants dont le nom complet manque : leur dépôt ne
// peut pas être nommé.
func (c Classroom) MissingNames() []roster.Person {
	var incomplets []roster.Person
	for _, student := range c.Students {
		if _, err := naming.Student(student.FullName); err != nil {
			incomplets = append(incomplets, student)
		}
	}
	return incomplets
}

// fragments associe à chaque étudiant le fragment qui le nomme dans un dépôt.
func (c Classroom) fragments() map[string]roster.Person {
	connus := map[string]roster.Person{}
	for _, student := range c.Students {
		if fragment, err := naming.Student(student.FullName); err == nil {
			connus[strings.ToLower(fragment)] = student
		}
	}
	return connus
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
	Others   int    `json:"others"`   // dépôts dont l'étudiant n'est pas du groupe
	PushedAt string `json:"pushed_at"`
}

// Assignments retrouve les travaux du groupe parmi les dépôts de l'organisation.
// La nomenclature courante se relit sans rien deviner : un dépôt est du
// groupe, ou il ne l'est pas.
func (c Classroom) Assignments(repos []groups.RepoInfo) []Assignment {
	if c.Legacy() {
		return c.legacyAssignments(repos)
	}
	connus := c.fragments()
	parNom := map[string]*Assignment{}

	for _, repo := range repos {
		parts, reconnu := naming.Parse(repo.Name)
		if !reconnu || !naming.Belongs(parts, c.Session, c.Course, c.Group) {
			continue
		}
		cle := strings.ToLower(parts.Assignment)
		travail, deja := parNom[cle]
		if !deja {
			travail = &Assignment{
				ID:   naming.AssignmentID(c.Session, c.Course, c.Group, parts.Assignment),
				Name: parts.Assignment,
			}
			parNom[cle] = travail
		}
		travail.Repos++
		if _, inscrit := connus[strings.ToLower(parts.Student)]; inscrit {
			travail.Students++
		} else {
			travail.Others++
		}
		if repo.PushedAt > travail.PushedAt {
			travail.PushedAt = repo.PushedAt
		}
	}

	trouves := make([]Assignment, 0, len(parNom))
	for _, travail := range parNom {
		trouves = append(trouves, *travail)
	}
	sortAssignments(trouves)
	return trouves
}

func sortAssignments(found []Assignment) {
	sort.Slice(found, func(i, j int) bool {
		if found[i].PushedAt != found[j].PushedAt {
			return found[i].PushedAt > found[j].PushedAt
		}
		return strings.ToLower(found[i].Name) < strings.ToLower(found[j].Name)
	})
}

// Served renvoie les comptes du groupe qui ont déjà un dépôt pour ce travail.
func (c Classroom) Served(assignmentID string, repos []groups.RepoInfo) map[string]bool {
	servis := map[string]bool{}
	if c.Legacy() {
		taken := groups.Build(assignmentID, repos).Suffixes()
		for _, student := range c.Students {
			if taken[strings.ToLower(student.Username)] {
				servis[strings.ToLower(student.Username)] = true
			}
		}
		return servis
	}

	connus := c.fragments()
	for _, repo := range repos {
		parts, reconnu := naming.Parse(repo.Name)
		if !reconnu {
			continue
		}
		id := naming.AssignmentID(parts.Session, parts.Course, parts.Group, parts.Assignment)
		if !strings.EqualFold(id, assignmentID) {
			continue
		}
		if student, inscrit := connus[strings.ToLower(parts.Student)]; inscrit {
			servis[strings.ToLower(student.Username)] = true
		}
	}
	return servis
}

// Repos renvoie les dépôts d'un travail du groupe, du plus récent au plus ancien.
func (c Classroom) Repos(assignmentID string, repos []groups.RepoInfo) []groups.Repo {
	if c.Legacy() {
		return groups.Build(assignmentID, repos).Repos
	}
	trouves := make([]groups.Repo, 0)
	for _, repo := range repos {
		parts, reconnu := naming.Parse(repo.Name)
		if !reconnu {
			continue
		}
		id := naming.AssignmentID(parts.Session, parts.Course, parts.Group, parts.Assignment)
		if !strings.EqualFold(id, assignmentID) {
			continue
		}
		pushed := repo.PushedAt
		if len(pushed) > 10 {
			pushed = pushed[:10]
		}
		trouves = append(trouves, groups.Repo{
			Name: repo.Name, Suffix: parts.Student, Private: repo.Private,
			URL: repo.HTMLURL, PushedAt: pushed,
		})
	}
	sort.Slice(trouves, func(i, j int) bool {
		return strings.ToLower(trouves[i].Name) < strings.ToLower(trouves[j].Name)
	})
	return trouves
}

// StudentOf retrouve l'étudiant du groupe auquel un dépôt appartient.
func (c Classroom) StudentOf(repoName string) (roster.Person, bool) {
	if c.Legacy() {
		return c.legacyStudentOf(repoName)
	}
	parts, reconnu := naming.Parse(repoName)
	if !reconnu {
		return roster.Person{}, false
	}
	student, inscrit := c.fragments()[strings.ToLower(parts.Student)]
	return student, inscrit
}

// ----------------------------------------------------------------- candidats

// Candidate est un groupe possible, deviné des dépôts déjà présents.
type Candidate struct {
	Session     string   `json:"session"`
	Course      string   `json:"course"`
	Group       string   `json:"group"`
	Prefix      string   `json:"prefix"`
	Assignments []string `json:"assignments"`
	Repos       int      `json:"repos"`
	Students    []string `json:"students"`
	// Legacy dit que le candidat suit l'ancienne nomenclature : ses comptes
	// sont des comptes GitHub, et il demande une migration avant distribution.
	Legacy bool `json:"legacy"`
}

// Candidates propose les groupes lisibles dans les dépôts : d'abord ceux qui
// suivent la nomenclature courante, puis les préfixes hérités.
func Candidates(repos []groups.RepoInfo) []Candidate {
	proposes := append(currentCandidates(repos), legacyCandidates(repos)...)
	sort.SliceStable(proposes, func(i, j int) bool {
		if proposes[i].Legacy != proposes[j].Legacy {
			return !proposes[i].Legacy
		}
		if proposes[i].Repos != proposes[j].Repos {
			return proposes[i].Repos > proposes[j].Repos
		}
		return proposes[i].Prefix < proposes[j].Prefix
	})
	return proposes
}

// currentCandidates lit les couples cours/groupe présents dans les dépôts.
func currentCandidates(repos []groups.RepoInfo) []Candidate {
	parPrefixe := map[string]*Candidate{}
	travaux := map[string]map[string]bool{}
	etudiants := map[string]map[string]bool{}

	for _, repo := range repos {
		parts, reconnu := naming.Parse(repo.Name)
		if !reconnu {
			continue
		}
		prefixe := naming.Prefix(parts.Session, parts.Course, parts.Group)
		cle := strings.ToLower(prefixe)
		candidat, deja := parPrefixe[cle]
		if !deja {
			candidat = &Candidate{
				Session: parts.Session, Course: parts.Course, Group: parts.Group,
				Prefix: prefixe,
			}
			parPrefixe[cle] = candidat
			travaux[cle] = map[string]bool{}
			etudiants[cle] = map[string]bool{}
		}
		candidat.Repos++
		travaux[cle][parts.Assignment] = true
		etudiants[cle][parts.Student] = true
	}

	proposes := make([]Candidate, 0, len(parPrefixe))
	for cle, candidat := range parPrefixe {
		candidat.Assignments = triees(travaux[cle])
		candidat.Students = triees(etudiants[cle])
		proposes = append(proposes, *candidat)
	}
	return proposes
}

func triees(ensemble map[string]bool) []string {
	liste := make([]string, 0, len(ensemble))
	for valeur := range ensemble {
		liste = append(liste, valeur)
	}
	sort.Strings(liste)
	return liste
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
