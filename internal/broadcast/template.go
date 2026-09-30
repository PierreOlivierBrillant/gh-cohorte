// Package broadcast dépose un même fichier dans tous les dépôts d'un travail —
// des consignes corrigées, une grille, un fichier de configuration oublié —,
// adapté à chaque dépôt par un gabarit minimal : {nom_etudiant}, {groupe},
// {cours}…
//
// Tout ce qui décide vit ici : quels champs existent et ce qu'ils valent, quel
// chemin est acceptable, ce qu'on fait d'un fichier déjà présent. Le terminal,
// la ligne de commande et le navigateur n'en sont que des façades.
package broadcast

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Field est un champ du gabarit.
type Field struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Fields énumère les champs du gabarit, dans l'ordre où les montrer.
//
// Leurs noms sont en français, contrairement à ceux du gabarit de nom de
// dépôt : ceux-là ne sont lus que par l'enseignant, ceux-ci s'écrivent dans
// un fichier que l'étudiant ouvrira, et c'est l'enseignant qui les tape au
// milieu d'un texte en français.
var Fields = []Field{
	{"nom_etudiant", "nom complet de l'étudiant (« Équipe eq1 » pour une équipe)"},
	{"prenom", "premier mot du nom complet"},
	{"nom_famille", "le reste du nom complet"},
	{"compte", "compte GitHub de l'étudiant"},
	{"matricule", "matricule, quand la liste du collège le donne"},
	{"equipe", "nom court de l'équipe, pour un travail d'équipe"},
	{"session", "nom court de la session : a26"},
	{"session_nom", "nom long de la session : Automne 2026"},
	{"cours", "cours : 5n6"},
	{"groupe", "groupe : 01"},
	{"travail", "nom court du travail : tp1"},
	{"echeance", "date cible du travail, quand elle est fixée"},
	{"depot", "nom du dépôt"},
	{"date", "date du jour : AAAA-MM-JJ"},
}

// Les accolades d'un champ ne contiennent que des minuscules et des soulignés :
// « {x + 1} » ou « { nom } » dans un fichier de code ne ressemblent pas à un
// champ, et n'en sont pas.
var fieldRe = regexp.MustCompile(`\{([a-z_]+)\}`)

func known(name string) bool {
	for _, field := range Fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

// FieldList rend les champs sous la forme où on les tape : « {cours}, {groupe} ».
func FieldList() string {
	noms := make([]string, 0, len(Fields))
	for _, field := range Fields {
		noms = append(noms, "{"+field.Name+"}")
	}
	return strings.Join(noms, ", ")
}

// Fill remplace les champs connus d'un texte par leur valeur.
//
// Un champ inconnu reste tel quel plutôt que d'être refusé : un fichier de code
// porte volontiers « {name} » — une f-string Python, une interpolation — et le
// refuser empêcherait de le déposer. « Unknown » les nomme, pour qu'une faute
// de frappe ne passe pas inaperçue.
func Fill(text string, values map[string]string) string {
	return fieldRe.ReplaceAllStringFunc(text, func(match string) string {
		nom := match[1 : len(match)-1]
		if !known(nom) {
			return match
		}
		return values[nom]
	})
}

// Unknown rend, triés et sans doublon, les champs des textes qui ne sont pas
// des champs du gabarit.
func Unknown(texts ...string) []string {
	return collect(texts, func(nom string) bool { return !known(nom) })
}

// Used rend, triés et sans doublon, les champs du gabarit que les textes
// emploient.
func Used(texts ...string) []string {
	return collect(texts, known)
}

func collect(texts []string, keep func(string) bool) []string {
	vus := map[string]bool{}
	for _, text := range texts {
		for _, match := range fieldRe.FindAllStringSubmatch(text, -1) {
			if keep(match[1]) {
				vus[match[1]] = true
			}
		}
	}
	noms := make([]string, 0, len(vus))
	for nom := range vus {
		noms = append(noms, nom)
	}
	sort.Strings(noms)
	return noms
}

// Context est ce que tous les dépôts d'un travail ont en commun.
type Context struct {
	Session     string
	SessionName string
	Course      string
	Group       string
	Assignment  string
	// Due est la date cible, telle que le registre la garde : « 2026-10-01 »
	// ou « 2026-10-01T23:59 ».
	Due   string
	Today time.Time
}

// Values calcule la valeur de chaque champ pour un dépôt.
func (c Context) Values(recipient Recipient) map[string]string {
	nom := strings.TrimSpace(recipient.Person.FullName)
	if recipient.Team != "" {
		nom = "Équipe " + recipient.Team
	}
	prenom, famille := "", ""
	if recipient.Team == "" {
		if mots := strings.Fields(nom); len(mots) > 0 {
			prenom, famille = mots[0], strings.Join(mots[1:], " ")
		}
	}
	date := ""
	if !c.Today.IsZero() {
		date = c.Today.Format("2006-01-02")
	}
	return map[string]string{
		"nom_etudiant": nom,
		"prenom":       prenom,
		"nom_famille":  famille,
		"compte":       strings.TrimSpace(recipient.Person.Username),
		"matricule":    strings.TrimSpace(recipient.Person.StudentID),
		"equipe":       recipient.Team,
		"session":      c.Session,
		"session_nom":  c.SessionName,
		"cours":        c.Course,
		"groupe":       c.Group,
		"travail":      c.Assignment,
		// « 2026-10-01T23:59 » est la forme du registre ; dans un fichier que
		// l'étudiant lit, le « T » n'apprend rien.
		"echeance": strings.Replace(strings.TrimSpace(c.Due), "T", " ", 1),
		"depot":    recipient.Repo,
		"date":     date,
	}
}
