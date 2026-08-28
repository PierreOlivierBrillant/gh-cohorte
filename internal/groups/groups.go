// Package groups regroupe les dépôts d'une organisation par préfixe commun
// et interprète les sélections saisies au clavier.
package groups

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Separator sépare les segments d'un nom de dépôt.
const Separator = "-"

// Mots qui désignent la totalité d'une sélection.
var allKeywords = map[string]bool{
	"tous": true, "toutes": true, "tout": true, "all": true, "*": true,
}

var rangeRe = regexp.MustCompile(`^(\d+)\s*-\s*(\d+)$`)

// RepoInfo retient les seuls champs utiles d'un dépôt : le reste de la réponse
// de l'API alourdirait le cache pour rien.
type RepoInfo struct {
	Name     string `json:"name"`
	Private  bool   `json:"private"`
	HTMLURL  string `json:"html_url"`
	PushedAt string `json:"pushed_at"`
}

// Repo est un dépôt appartenant à un groupe.
type Repo struct {
	Name     string
	Suffix   string // ce qui suit le préfixe : compte GitHub ou nom slugifié
	Private  bool
	URL      string
	PushedAt string // date seule, « 2026-08-21 »
}

// Visibility décrit la visibilité en français.
func (r Repo) Visibility() string {
	if r.Private {
		return "privé"
	}
	return "public"
}

// Group rassemble les dépôts partageant un même préfixe.
type Group struct {
	Prefix string
	Repos  []Repo
}

// Len renvoie le nombre de dépôts du groupe.
func (g Group) Len() int { return len(g.Repos) }

// Find retrouve un dépôt par son nom complet ou son suffixe, sans tenir compte de la casse.
func (g Group) Find(name string) (Repo, int, bool) {
	wanted := strings.ToLower(strings.TrimSpace(name))
	for index, repo := range g.Repos {
		if strings.ToLower(repo.Name) == wanted || strings.ToLower(repo.Suffix) == wanted {
			return repo, index, true
		}
	}
	return Repo{}, -1, false
}

// Suffixes renvoie les suffixes déjà pris, pour repérer les personnes déjà servies.
func (g Group) Suffixes() map[string]bool {
	taken := make(map[string]bool, len(g.Repos))
	for _, repo := range g.Repos {
		taken[strings.ToLower(repo.Suffix)] = true
	}
	return taken
}

// Detected est un groupe deviné à partir des noms de dépôts.
type Detected struct {
	Prefix string
	Count  int
}

// candidatePrefixes énumère les préfixes possibles d'un nom, le nom entier exclu.
func candidatePrefixes(name string) []string {
	parts := strings.Split(name, Separator)
	prefixes := make([]string, 0, len(parts)-1)
	for size := 1; size < len(parts); size++ {
		prefixes = append(prefixes, strings.Join(parts[:size], Separator))
	}
	return prefixes
}

// commonPrefix calcule le plus long préfixe commun, segment par segment.
func commonPrefix(names []string) string {
	if len(names) == 0 {
		return ""
	}
	segments := make([][]string, len(names))
	shortest := -1
	for index, name := range names {
		segments[index] = strings.Split(name, Separator)
		if shortest < 0 || len(segments[index]) < shortest {
			shortest = len(segments[index])
		}
	}
	var common []string
	for position := 0; position < shortest-1; position++ {
		current := segments[0][position]
		same := true
		for _, item := range segments {
			if item[position] != current {
				same = false
				break
			}
		}
		if !same {
			break
		}
		common = append(common, current)
	}
	return strings.Join(common, Separator)
}

// Detect devine les groupes présents et les renvoie du plus grand au plus petit.
func Detect(names []string, minimum int) []Detected {
	if minimum < 1 {
		minimum = 2
	}
	counts := map[string]int{}
	for _, name := range names {
		for _, prefix := range candidatePrefixes(strings.ToLower(name)) {
			counts[prefix]++
		}
	}

	retained := map[string]bool{}
	for prefix, total := range counts {
		if total >= minimum {
			retained[prefix] = true
		}
	}

	// On ne garde que les préfixes les plus généraux : « tp1 » l'emporte sur « tp1-jean ».
	groups := map[string]int{}
	for prefix := range retained {
		general := true
		for other := range retained {
			if other != prefix && strings.HasPrefix(prefix, other+Separator) {
				general = false
				break
			}
		}
		if !general {
			continue
		}
		var members []string
		for _, name := range names {
			if strings.HasPrefix(strings.ToLower(name), prefix+Separator) {
				members = append(members, strings.ToLower(name))
			}
		}
		// Le préfixe est étendu à ce que partagent réellement ses membres :
		// « projet » redevient « projet-final » si tous les dépôts le portent.
		label := commonPrefix(members)
		if label == "" {
			label = prefix
		}
		if len(members) > groups[label] {
			groups[label] = len(members)
		}
	}

	detected := make([]Detected, 0, len(groups))
	for prefix, count := range groups {
		detected = append(detected, Detected{Prefix: prefix, Count: count})
	}
	sort.Slice(detected, func(i, j int) bool {
		if detected[i].Count != detected[j].Count {
			return detected[i].Count > detected[j].Count
		}
		return detected[i].Prefix < detected[j].Prefix
	})
	return detected
}

// Build retient les dépôts du préfixe donné, à partir des données de l'API.
func Build(prefix string, repos []RepoInfo) Group {
	wanted := strings.TrimRight(strings.ToLower(strings.TrimSpace(prefix)), Separator)
	group := Group{Prefix: wanted}
	if wanted == "" {
		return group
	}
	for _, raw := range repos {
		if !strings.HasPrefix(strings.ToLower(raw.Name), wanted+Separator) {
			continue
		}
		pushed := raw.PushedAt
		if len(pushed) > 10 {
			pushed = pushed[:10]
		}
		group.Repos = append(group.Repos, Repo{
			Name:     raw.Name,
			Suffix:   raw.Name[len(wanted)+1:],
			Private:  raw.Private,
			URL:      raw.HTMLURL,
			PushedAt: pushed,
		})
	}
	sort.Slice(group.Repos, func(i, j int) bool {
		return strings.ToLower(group.Repos[i].Name) < strings.ToLower(group.Repos[j].Name)
	})
	return group
}

// Resolver traduit un jeton non numérique en indice, ou renvoie -1.
type Resolver func(token string) int

var splitRe = regexp.MustCompile(`[,\s]+`)

// ParseSelection interprète « tous », « 1,3,5-8 » ou des noms, et renvoie des indices triés.
func ParseSelection(answer string, count int, resolve Resolver) ([]int, error) {
	raw := strings.TrimSpace(answer)
	if raw == "" || allKeywords[strings.ToLower(raw)] {
		all := make([]int, count)
		for index := range all {
			all[index] = index
		}
		return all, nil
	}

	chosen := map[int]bool{}
	for _, token := range splitRe.Split(raw, -1) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if span := rangeRe.FindStringSubmatch(token); span != nil {
			start, _ := strconv.Atoi(span[1])
			end, _ := strconv.Atoi(span[2])
			if start > end {
				start, end = end, start
			}
			for number := start; number <= end; number++ {
				index, err := checked(number, count, token)
				if err != nil {
					return nil, err
				}
				chosen[index] = true
			}
			continue
		}
		if number, err := strconv.Atoi(token); err == nil {
			index, err := checked(number, count, token)
			if err != nil {
				return nil, err
			}
			chosen[index] = true
			continue
		}
		index := -1
		if resolve != nil {
			index = resolve(token)
		}
		if index < 0 || index >= count {
			return nil, valid.Errorf("Sélection : « %s » ne correspond à aucune entrée de la liste.", token)
		}
		chosen[index] = true
	}

	indices := make([]int, 0, len(chosen))
	for index := range chosen {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	return indices, nil
}

func checked(number, count int, token string) (int, error) {
	if number < 1 || number > count {
		return 0, valid.Errorf("Sélection : « %s » sort de la liste (1 à %d).", token, count)
	}
	return number - 1, nil
}
