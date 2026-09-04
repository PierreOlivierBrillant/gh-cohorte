// Package students dresse la liste des étudiants d'un groupe — chacun avec les
// dépôts qu'il a déjà —, puis la filtre et la trie.
//
// Ces trois opérations vivent ici plutôt que dans une interface : « trié par
// dernier envoi » doit donner le même ordre au navigateur et au terminal, et
// « avant le 1er octobre » doit y vouloir dire la même chose. Une interface ne
// fait que dire ce qu'elle veut ; c'est ce paquet qui sait ce que cela signifie.
package students

import (
	"sort"
	"strings"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// DateFormat est la forme attendue d'une date de filtre.
const DateFormat = "2006-01-02"

// Repo est un dépôt qu'un étudiant possède déjà.
type Repo struct {
	// Assignment est le nom court du travail, ID son identifiant complet.
	Assignment string
	ID         string
	Name       string
	URL        string
	// PushedAt est la date seule — « 2026-08-21 » —, vide si rien n'y a été envoyé.
	PushedAt string
}

// Row est un étudiant et ce que ses dépôts racontent de lui.
type Row struct {
	FullName string
	Username string
	Repos    []Repo
	// PushedAt est le plus récent envoi de ses dépôts ; vide s'il n'y en a eu aucun.
	PushedAt string
}

// Build croise les étudiants du groupe avec les dépôts de l'organisation :
// c'est l'équivalent du « a accepté le devoir » de GitHub Classroom, déduit des
// dépôts existants plutôt que d'une invitation.
func Build(cours classroom.Classroom, repos []groups.RepoInfo) []Row {
	parEtudiant := map[string][]Repo{}
	for _, travail := range cours.Assignments(repos) {
		for _, depot := range cours.Repos(travail.ID, repos) {
			student, inscrit := cours.StudentOf(depot.Name)
			if !inscrit {
				continue
			}
			cle := strings.ToLower(student.Username)
			parEtudiant[cle] = append(parEtudiant[cle], Repo{
				Assignment: travail.Name, ID: travail.ID, Name: depot.Name,
				URL: depot.URL, PushedAt: depot.PushedAt,
			})
		}
	}

	lignes := make([]Row, 0, len(cours.Students))
	for _, student := range cours.Students {
		lignes = append(lignes, compose(student.FullName, student.Username,
			parEtudiant[strings.ToLower(student.Username)]))
	}
	return lignes
}

// FromGroup dresse les mêmes lignes à partir d'un groupe lu par préfixe : c'est
// ainsi que l'assistant du terminal travaille, un dépôt par personne. Les noms
// complets, qu'un préfixe ne dit pas, sont fournis à part — dépôt par dépôt.
func FromGroup(group groups.Group, names map[string]string) []Row {
	lignes := make([]Row, 0, group.Len())
	for _, depot := range group.Repos {
		lignes = append(lignes, compose(names[depot.Name], depot.Suffix, []Repo{{
			Assignment: group.Prefix, ID: group.Prefix, Name: depot.Name,
			URL: depot.URL, PushedAt: depot.PushedAt,
		}}))
	}
	return lignes
}

// compose assemble une ligne et en retient le plus récent envoi.
func compose(fullName, username string, depots []Repo) Row {
	if depots == nil {
		depots = []Repo{}
	}
	ligne := Row{FullName: fullName, Username: username, Repos: depots}
	for _, depot := range depots {
		if depot.PushedAt > ligne.PushedAt {
			ligne.PushedAt = depot.PushedAt
		}
	}
	return ligne
}

// ------------------------------------------------------------------- filtres

// Activity retient les étudiants selon ce que leurs dépôts montrent d'eux.
type Activity string

const (
	// AnyActivity ne retient rien : tout le monde passe.
	AnyActivity Activity = ""
	// WithRepos ne garde que ceux qui ont au moins un dépôt.
	WithRepos Activity = "avec"
	// WithoutRepos ne garde que ceux qui n'en ont aucun.
	WithoutRepos Activity = "sans"
	// Silent ne garde que ceux qui ont un dépôt mais n'y ont jamais rien envoyé.
	Silent Activity = "muet"
)

// Activities énumère les valeurs acceptées, dans l'ordre où les proposer.
var Activities = []Activity{AnyActivity, WithRepos, WithoutRepos, Silent}

// Filter retient les étudiants qui répondent à des critères. Un critère laissé
// vide ne retient rien : il laisse simplement passer.
//
// Les dates portent sur le dernier envoi de la personne, bornes incluses.
// Quelqu'un qui n'a jamais rien envoyé n'a pas de date : il est écarté des deux
// bornes plutôt que rangé arbitrairement d'un côté. « sans » et « muet » sont
// là pour le retrouver.
type Filter struct {
	// Text est cherché à la fois dans le nom complet et dans le compte.
	Text string
	// Name et Username cherchent dans l'un ou dans l'autre seulement.
	Name     string
	Username string
	// Assignment ne garde que ceux qui ont un dépôt pour ce travail, désigné
	// par son nom court ou par son identifiant complet.
	Assignment string
	// PushedAfter et PushedBefore encadrent le dernier envoi, « 2026-10-01 ».
	PushedAfter  string
	PushedBefore string
	Activity     Activity
}

// Validate met le filtre en forme et refuse ce qui ne peut pas être appliqué.
func (f Filter) Validate() (Filter, error) {
	after, err := ParseDate(f.PushedAfter, "Dernier envoi après")
	if err != nil {
		return f, err
	}
	before, err := ParseDate(f.PushedBefore, "Dernier envoi avant")
	if err != nil {
		return f, err
	}
	if after != "" && before != "" && after > before {
		return f, valid.Errorf(
			"Dernier envoi : « %s » est postérieur à « %s ».", after, before)
	}
	f.PushedAfter, f.PushedBefore = after, before

	activite := Activity(strings.ToLower(strings.TrimSpace(string(f.Activity))))
	connue := false
	for _, candidate := range Activities {
		if candidate == activite {
			connue = true
			break
		}
	}
	if !connue {
		return f, valid.Errorf(
			"Activité : « %s » est inconnu (attendu : avec, sans, muet, ou rien).", f.Activity)
	}
	f.Activity = activite
	f.Text = strings.TrimSpace(f.Text)
	f.Name = strings.TrimSpace(f.Name)
	f.Username = strings.TrimSpace(f.Username)
	f.Assignment = strings.TrimSpace(f.Assignment)
	return f, nil
}

// ParseDate valide une date de filtre et la renvoie sous sa forme normale.
func ParseDate(value, label string) (string, error) {
	date := strings.TrimSpace(value)
	if date == "" {
		return "", nil
	}
	parsed, err := time.Parse(DateFormat, date)
	if err != nil {
		return "", valid.Errorf("%s : « %s » n'est pas une date (attendu AAAA-MM-JJ).",
			label, date)
	}
	return parsed.Format(DateFormat), nil
}

// Keep dit si une ligne passe le filtre.
func (f Filter) Keep(row Row) bool {
	if !contains(row.FullName+" "+row.Username, f.Text) ||
		!contains(row.FullName, f.Name) ||
		!contains(row.Username, f.Username) {
		return false
	}
	if !f.keepActivity(row) || !f.keepAssignment(row) {
		return false
	}
	// Sans date connue, les bornes ne peuvent rien dire de cette personne.
	if f.PushedAfter != "" && (row.PushedAt == "" || row.PushedAt < f.PushedAfter) {
		return false
	}
	if f.PushedBefore != "" && (row.PushedAt == "" || row.PushedAt > f.PushedBefore) {
		return false
	}
	return true
}

func (f Filter) keepActivity(row Row) bool {
	switch f.Activity {
	case WithRepos:
		return len(row.Repos) > 0
	case WithoutRepos:
		return len(row.Repos) == 0
	case Silent:
		return len(row.Repos) > 0 && row.PushedAt == ""
	}
	return true
}

func (f Filter) keepAssignment(row Row) bool {
	if f.Assignment == "" {
		return true
	}
	for _, depot := range row.Repos {
		if strings.EqualFold(depot.Assignment, f.Assignment) ||
			strings.EqualFold(depot.ID, f.Assignment) {
			return true
		}
	}
	return false
}

// contains cherche une sous-chaîne sans tenir compte de la casse ni des
// accents : « cote » doit trouver « Côté », que la personne se soit inscrite
// avec ou sans clavier accentué.
func contains(haystack, needle string) bool {
	wanted := valid.Slugify(needle)
	if wanted == "" {
		return true
	}
	return strings.Contains(valid.Slugify(haystack), wanted)
}

// --------------------------------------------------------------------- tri

// Key désigne la colonne sur laquelle trier.
type Key string

const (
	// ByName trie par nom complet.
	ByName Key = "nom"
	// ByUsername trie par compte GitHub.
	ByUsername Key = "compte"
	// ByPushed trie par date du dernier envoi.
	ByPushed Key = "envoi"
)

// Keys énumère les tris acceptés, dans l'ordre où les proposer.
var Keys = []Key{ByName, ByUsername, ByPushed}

// ParseKey valide un tri saisi. Sans valeur, c'est le nom.
func ParseKey(value string) (Key, error) {
	key := Key(strings.ToLower(strings.TrimSpace(value)))
	if key == "" {
		return ByName, nil
	}
	for _, candidate := range Keys {
		if candidate == key {
			return key, nil
		}
	}
	return ByName, valid.Errorf(
		"Tri : « %s » est inconnu (attendu : nom, compte ou envoi).", value)
}

// Apply filtre puis trie une liste d'étudiants, sans toucher à l'originale.
//
// Le tri est total : à valeur égale, le nom puis le compte tranchent, si bien
// que deux appels donnent toujours le même ordre. Ce qui manque — une date,
// un nom complet — se range avant tout le reste : en tête d'un tri croissant,
// en queue d'un tri décroissant.
func Apply(rows []Row, filter Filter, key Key, desc bool) []Row {
	retenues := make([]Row, 0, len(rows))
	for _, row := range rows {
		if filter.Keep(row) {
			retenues = append(retenues, row)
		}
	}
	sort.SliceStable(retenues, func(i, j int) bool {
		gauche, droite := retenues[i], retenues[j]
		if compare := compareOn(key, gauche, droite); compare != 0 {
			return (compare < 0) != desc
		}
		return fallback(gauche, droite)
	})
	return retenues
}

// compareOn compare deux lignes sur la colonne demandée.
func compareOn(key Key, gauche, droite Row) int {
	switch key {
	case ByUsername:
		return strings.Compare(fold(gauche.Username), fold(droite.Username))
	case ByPushed:
		return strings.Compare(gauche.PushedAt, droite.PushedAt)
	}
	return strings.Compare(fold(gauche.FullName), fold(droite.FullName))
}

// fallback départage deux lignes que la colonne de tri laisse à égalité.
func fallback(gauche, droite Row) bool {
	if gauche.FullName != droite.FullName {
		return fold(gauche.FullName) < fold(droite.FullName)
	}
	return fold(gauche.Username) < fold(droite.Username)
}

// fold rend un texte comparable : « Émilie » se range avec les E.
func fold(value string) string { return valid.Slugify(value) }
