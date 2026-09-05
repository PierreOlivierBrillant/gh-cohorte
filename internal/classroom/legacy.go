package classroom

import (
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// Ce fichier ne sert qu'aux dépôts nommés avant la nomenclature courante. Deux
// formes l'ont précédée :
//
//	a26-5n6-tp1-emilie-cote      tout en tirets, rien ne dit où finit le travail
//	5n6.a26-01.tp1.emilie-cote   quatre niveaux, sans la session
//
// À quoi s'ajoute ce que rien n'organise, adopté par gabarit — voir
// « pattern.go ». Le principe est le même : lisible, migrable, jamais créé.
//
// Rien n'y est créé — ces dépôts se lisent, se listent, et se migrent. Le jour
// où plus aucune organisation n'en contient, le fichier disparaît d'un bloc.

// LegacyNamePattern est le gabarit qui a servi à nommer ces dépôts.
const LegacyNamePattern = "{assignment}" + groups.Separator + "{username}"

// legacyAssignments retrouve les travaux d'un groupe hérité. Faute de
// séparateur réservé, deux lectures se complètent : les dépôts des étudiants
// inscrits, relus par le gabarit — exact, même pour un travail distribué à une
// seule personne —, et la détection par préfixe, qui rattrape les travaux dont
// aucun étudiant inscrit n'a de dépôt.
func (c Classroom) legacyAssignments(repos []groups.RepoInfo) []Assignment {
	if gabarit, ok := c.gabarit(); ok {
		return c.patternAssignments(gabarit, repos)
	}
	if strings.Contains(c.LegacyPrefix, naming.Separator) {
		return c.dottedAssignments(repos)
	}
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

// patternAssignments lit les travaux d'un groupe adopté par gabarit.
func (c Classroom) patternAssignments(gabarit Pattern, repos []groups.RepoInfo) []Assignment {
	parNom := map[string]*Assignment{}
	ajouter := func(travail string, inscrit bool, repo groups.RepoInfo) {
		if travail == "" {
			travail = c.Label()
		}
		cle := strings.ToLower(travail)
		trouve, deja := parNom[cle]
		if !deja {
			trouve = &Assignment{ID: travail, Name: travail}
			parNom[cle] = trouve
		}
		trouve.Repos++
		if inscrit {
			trouve.Students++
		} else {
			trouve.Others++
		}
		if repo.PushedAt > trouve.PushedAt {
			trouve.PushedAt = repo.PushedAt
		}
	}

	for _, repo := range repos {
		if travail, _, inscrit := c.patternParts(gabarit, repo.Name); inscrit || travail != "" {
			ajouter(travail, inscrit, repo)
			continue
		}
		if travail, _, reconnu := gabarit.Match(repo.Name); reconnu {
			ajouter(travail, false, repo)
		}
	}

	trouves := make([]Assignment, 0, len(parNom))
	for _, travail := range parNom {
		trouves = append(trouves, *travail)
	}
	sortAssignments(trouves)
	return trouves
}

// patternParts découpe un nom en s'appuyant sur la liste du groupe : c'est la
// lecture exacte, celle qui ne se trompe pas quand un compte contient le
// séparateur. Le booléen dit qu'une personne inscrite a été reconnue.
func (c Classroom) patternParts(gabarit Pattern, repoName string) (string, roster.Person, bool) {
	for fragment, student := range c.fragments() {
		if travail, reconnu := gabarit.MatchFor(repoName, fragment); reconnu {
			return travail, student, true
		}
	}
	return "", roster.Person{}, false
}

// dottedAssignments lit les travaux d'un groupe de la nomenclature à quatre
// niveaux : le point sépare déjà, il ne manque que la session.
func (c Classroom) dottedAssignments(repos []groups.RepoInfo) []Assignment {
	connus := c.fragments()
	parNom := map[string]*Assignment{}

	for _, repo := range repos {
		travail, etudiant, reconnu := c.dottedParts(repo.Name)
		if !reconnu {
			continue
		}
		cle := strings.ToLower(travail)
		trouve, deja := parNom[cle]
		if !deja {
			trouve = &Assignment{ID: c.AssignmentID(travail), Name: travail}
			parNom[cle] = trouve
		}
		trouve.Repos++
		if _, inscrit := connus[strings.ToLower(etudiant)]; inscrit {
			trouve.Students++
		} else {
			trouve.Others++
		}
		if repo.PushedAt > trouve.PushedAt {
			trouve.PushedAt = repo.PushedAt
		}
	}

	trouves := make([]Assignment, 0, len(parNom))
	for _, travail := range parNom {
		trouves = append(trouves, *travail)
	}
	sortAssignments(trouves)
	return trouves
}

// dottedParts découpe « préfixe.travail.étudiant » quand le dépôt relève du
// groupe.
func (c Classroom) dottedParts(repoName string) (string, string, bool) {
	prefixe := c.LegacyPrefix + naming.Separator
	if len(repoName) <= len(prefixe) ||
		!strings.EqualFold(repoName[:len(prefixe)], prefixe) {
		return "", "", false
	}
	reste := strings.Split(repoName[len(prefixe):], naming.Separator)
	if len(reste) != 2 || reste[0] == "" || reste[1] == "" {
		return "", "", false
	}
	return reste[0], reste[1], true
}

// legacyStudentOf rattache un dépôt hérité à son étudiant. Faute de séparateur
// réservé, le nom tout en tirets ne se découpe pas : il faut le confronter au
// gabarit, compte par compte. La forme à quatre niveaux, elle, se lit.
func (c Classroom) legacyStudentOf(repoName string) (roster.Person, bool) {
	if gabarit, ok := c.gabarit(); ok {
		_, student, reconnu := c.patternParts(gabarit, repoName)
		return student, reconnu
	}
	if strings.Contains(c.LegacyPrefix, naming.Separator) {
		_, etudiant, reconnu := c.dottedParts(repoName)
		if !reconnu {
			return roster.Person{}, false
		}
		student, inscrit := c.fragments()[strings.ToLower(etudiant)]
		return student, inscrit
	}
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

// legacyServed dit quels comptes du groupe ont déjà un dépôt pour ce travail.
// Les deux nomenclatures ne nomment pas la même chose : l'une porte le compte
// GitHub, l'autre le nom de l'étudiant.
func (c Classroom) legacyServed(assignmentID string, repos []groups.RepoInfo) map[string]bool {
	servis := map[string]bool{}
	if gabarit, ok := c.gabarit(); ok {
		for _, repo := range repos {
			travail, student, reconnu := c.patternParts(gabarit, repo.Name)
			if !reconnu || !strings.EqualFold(c.AssignmentID(nomOuGroupe(travail, c.Label())), assignmentID) {
				continue
			}
			servis[strings.ToLower(student.Username)] = true
		}
		return servis
	}
	if strings.Contains(c.LegacyPrefix, naming.Separator) {
		connus := c.fragments()
		for _, repo := range c.legacyRepos(assignmentID, repos) {
			if student, inscrit := connus[strings.ToLower(repo.Suffix)]; inscrit {
				servis[strings.ToLower(student.Username)] = true
			}
		}
		return servis
	}
	taken := groups.Build(assignmentID, repos).Suffixes()
	for _, student := range c.Students {
		if taken[strings.ToLower(student.Username)] {
			servis[strings.ToLower(student.Username)] = true
		}
	}
	return servis
}

// legacyRepos rassemble les dépôts d'un travail hérité.
func (c Classroom) legacyRepos(assignmentID string, repos []groups.RepoInfo) []groups.Repo {
	if gabarit, ok := c.gabarit(); ok {
		return c.patternRepos(gabarit, assignmentID, repos)
	}
	if !strings.Contains(c.LegacyPrefix, naming.Separator) {
		return groups.Build(assignmentID, repos).Repos
	}
	trouves := make([]groups.Repo, 0)
	for _, repo := range repos {
		travail, etudiant, reconnu := c.dottedParts(repo.Name)
		if !reconnu || !strings.EqualFold(c.AssignmentID(travail), assignmentID) {
			continue
		}
		pushed := repo.PushedAt
		if len(pushed) > 10 {
			pushed = pushed[:10]
		}
		trouves = append(trouves, groups.Repo{
			Name: repo.Name, Suffix: etudiant, Private: repo.Private,
			URL: repo.HTMLURL, PushedAt: pushed,
		})
	}
	sort.Slice(trouves, func(i, j int) bool {
		return strings.ToLower(trouves[i].Name) < strings.ToLower(trouves[j].Name)
	})
	return trouves
}

// patternRepos rassemble les dépôts d'un travail adopté par gabarit.
func (c Classroom) patternRepos(gabarit Pattern, assignmentID string,
	repos []groups.RepoInfo) []groups.Repo {
	trouves := make([]groups.Repo, 0)
	for _, repo := range repos {
		travail, etudiant := "", ""
		if nom, student, reconnu := c.patternParts(gabarit, repo.Name); reconnu {
			travail, etudiant = nom, student.Username
		} else if nom, personne, reconnu := gabarit.Match(repo.Name); reconnu {
			travail, etudiant = nom, personne
		} else {
			continue
		}
		if !strings.EqualFold(nomOuGroupe(travail, c.Label()), assignmentID) {
			continue
		}
		pousse := repo.PushedAt
		if len(pousse) > 10 {
			pousse = pousse[:10]
		}
		trouves = append(trouves, groups.Repo{
			Name: repo.Name, Suffix: etudiant, Private: repo.Private,
			URL: repo.HTMLURL, PushedAt: pousse,
		})
	}
	sort.Slice(trouves, func(i, j int) bool {
		return strings.ToLower(trouves[i].Name) < strings.ToLower(trouves[j].Name)
	})
	return trouves
}

// nomOuGroupe remplace un travail anonyme — gabarit sans {assignment} — par ce
// qui désigne le groupe : il faut bien appeler le seul travail qu'il porte.
func nomOuGroupe(travail, nomDuGroupe string) string {
	if travail == "" {
		return nomDuGroupe
	}
	return travail
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
	parents := map[string]*Candidate{}
	comptes := map[string]map[string]bool{}
	travauxVus := map[string]map[string]bool{}

	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		// Un dépôt de la nomenclature courante n'a rien à faire ici.
		if _, reconnu := naming.Parse(repo.Name); reconnu {
			continue
		}
		// Un dépôt de la nomenclature à quatre niveaux se lit directement :
		// « cours.groupe » fait le préfixe, il ne lui manque que la session.
		if niveaux := strings.Split(repo.Name, naming.Separator); len(niveaux) == 4 {
			prefixe := strings.Join(niveaux[:2], naming.Separator)
			candidat, connu := parents[prefixe]
			if !connu {
				candidat = &Candidate{Prefix: prefixe, Legacy: true}
				parents[prefixe] = candidat
				comptes[prefixe] = map[string]bool{}
				travauxVus[prefixe] = map[string]bool{}
			}
			candidat.Repos++
			travauxVus[prefixe][niveaux[2]] = true
			comptes[prefixe][strings.ToLower(niveaux[3])] = true
			continue
		}
		names = append(names, repo.Name)
	}

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
			travauxVus[prefix] = map[string]bool{}
		}
		travauxVus[prefix][court] = true
		group := groups.Build(detected.Prefix, repos)
		candidat.Repos += group.Len()
		for _, repo := range group.Repos {
			comptes[prefix][strings.ToLower(repo.Suffix)] = true
		}
	}

	proposes := make([]Candidate, 0, len(parents))
	for prefix, candidat := range parents {
		candidat.Students = triees(comptes[prefix])
		candidat.Assignments = triees(travauxVus[prefix])
		proposes = append(proposes, *candidat)
	}
	return proposes
}
