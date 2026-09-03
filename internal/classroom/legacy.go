package classroom

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// Ce fichier ne sert qu'aux dépôts nommés avant la nomenclature à quatre
// niveaux : « a26-5n6-tp1-emilie-cote ». Rien n'y est créé — ils se lisent, se
// listent, et se migrent. Le jour où plus aucune organisation n'en contient, il
// disparaît d'un bloc.

// LegacyNamePattern est le gabarit qui a servi à nommer ces dépôts.
const LegacyNamePattern = "{assignment}" + groups.Separator + "{username}"

// legacyAssignments retrouve les travaux d'un groupe hérité. Faute de
// séparateur réservé, deux lectures se complètent : les dépôts des étudiants
// inscrits, relus par le gabarit — exact, même pour un travail distribué à une
// seule personne —, et la détection par préfixe, qui rattrape les travaux dont
// aucun étudiant inscrit n'a de dépôt.
func (c Classroom) legacyAssignments(repos []groups.RepoInfo) []Assignment {
	identifiants := map[string]string{} // minuscules → identifiant tel qu'écrit
	retenir := func(id string) {
		if c.Owns(id) {
			identifiants[strings.ToLower(id)] = id
		}
	}

	for _, student := range c.Students {
		expression := plan.Matcher(LegacyNamePattern, student)
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

	trouves = withoutUmbrellas(trouves)
	sortAssignments(trouves)
	return trouves
}

// legacyStudentOf rattache un dépôt hérité à son étudiant. Faute de séparateur
// réservé, le nom ne se découpe pas : il faut le confronter au gabarit, compte
// par compte.
func (c Classroom) legacyStudentOf(repoName string) (roster.Person, bool) {
	for _, student := range c.Students {
		expression := plan.Matcher(LegacyNamePattern, student)
		if expression == nil {
			break
		}
		if _, reconnu := plan.Assignment(expression, repoName); reconnu {
			return student, true
		}
	}
	return roster.Person{}, false
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

// legacyCandidates propose les préfixes hérités : les travaux détectés sont
// regroupés par ce qui les précède. « a26-5n6-tp1 » et « a26-5n6-travailsession »
// proposent ainsi « a26-5n6 », avec les comptes qu'on y trouve déjà.
func legacyCandidates(repos []groups.RepoInfo) []Candidate {
	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		// Un dépôt de la nouvelle nomenclature n'a rien à faire ici.
		if _, reconnu := naming.Parse(repo.Name); reconnu {
			continue
		}
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
			candidat = &Candidate{Prefix: prefix, Legacy: true}
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
		candidat.Students = triees(comptes[prefix])
		sort.Strings(candidat.Assignments)
		proposes = append(proposes, *candidat)
	}
	return proposes
}
