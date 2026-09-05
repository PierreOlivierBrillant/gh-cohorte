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
	Prefix string `json:"prefix"`
	Count  int    `json:"count"`
}

// segmented découpe les noms en segments, en minuscules. Un nom sans séparateur
// ne peut porter aucun préfixe : il est écarté.
func segmented(names []string) [][]string {
	members := make([][]string, 0, len(names))
	for _, name := range names {
		segments := strings.Split(strings.ToLower(name), Separator)
		if len(segments) < 2 {
			continue
		}
		members = append(members, segments)
	}
	return members
}

// branches répartit les dépôts selon le segment qui suit le préfixe de longueur
// « depth ». Un dépôt dont il ne reste qu'un segment — son suffixe — ne peut pas
// descendre plus bas : il est compté à part.
func branches(members [][]string, depth int) (map[string][][]string, int) {
	byNext := make(map[string][][]string)
	terminal := 0
	for _, segments := range members {
		if len(segments) <= depth+1 {
			terminal++
			continue
		}
		next := segments[depth]
		byNext[next] = append(byNext[next], segments)
	}
	return byNext, terminal
}

// splits dit si le préfixe se subdivise en travaux distincts : au moins deux
// sous-préfixes rassemblent chacun un groupe entier, et les dépôts qui
// s'arrêtent au préfixe sont trop peu nombreux pour en former un eux-mêmes.
// C'est le cas de « a26 » quand l'organisation porte « a26-4w6-tp1-… » et
// « a26-5n6-travailsession-… » : deux travaux que fondre dans « a26 »
// n'aiderait personne. À l'inverse, un groupe dont les suffixes sont des
// comptes GitHub garde toujours des noms d'un seul segment — « tp1-jlpicard »
// à côté de « tp1-emilie-cote » — et reste donc entier.
func splits(byNext map[string][][]string, terminal, minimum int) bool {
	if terminal >= minimum {
		return false
	}
	subgroups := 0
	for _, sub := range byNext {
		if len(sub) >= minimum {
			subgroups++
		}
	}
	return subgroups >= 2
}

// collect retient le groupe formé par les « depth » premiers segments, ou
// descend dans ses sous-groupes s'il se subdivise.
func collect(members [][]string, depth, minimum int, found *[]Detected) {
	// Le préfixe est d'abord étendu à ce que partagent réellement ses membres :
	// « projet » redevient « projet-final » si tous les dépôts le portent.
	for {
		byNext, terminal := branches(members, depth)
		if terminal > 0 || len(byNext) != 1 {
			break
		}
		depth++
	}
	byNext, terminal := branches(members, depth)
	if splits(byNext, terminal, minimum) {
		for _, sub := range byNext {
			if len(sub) >= minimum {
				collect(sub, depth+1, minimum, found)
			}
		}
		return
	}
	if len(members) >= minimum {
		*found = append(*found, Detected{
			Prefix: strings.Join(members[0][:depth], Separator),
			Count:  len(members),
		})
	}
}

// Detect devine les groupes présents et les renvoie du plus grand au plus petit.
// Le préfixe retenu est le plus général qui rassemble au moins « minimum »
// dépôts sans se subdiviser en plusieurs groupes de cette taille.
func Detect(names []string, minimum int) []Detected {
	if minimum < 1 {
		minimum = 2
	}
	// Le premier segment est toujours un point de départ distinct : rien ne
	// rassemble « tp1-… » et « tp2-… ». Les branches trop petites tombent ici.
	roots, _ := branches(segmented(names), 0)
	detected := make([]Detected, 0, len(roots))
	for _, members := range roots {
		if len(members) < minimum {
			continue
		}
		collect(members, 1, minimum, &detected)
	}
	sort.Slice(detected, func(i, j int) bool {
		if detected[i].Count != detected[j].Count {
			return detected[i].Count > detected[j].Count
		}
		return detected[i].Prefix < detected[j].Prefix
	})
	return detected
}

// Separators sont les caractères qui peuvent suivre un préfixe : le tiret des
// noms qu'aucune convention n'organise, et le point de la nomenclature à cinq
// niveaux. Un préfixe se saisit sans dire lequel des deux le termine.
const Separators = Separator + "."

// Build retient les dépôts du préfixe donné, à partir des données de l'API.
func Build(prefix string, repos []RepoInfo) Group {
	wanted := strings.TrimRight(strings.ToLower(strings.TrimSpace(prefix)), Separators)
	group := Group{Prefix: wanted}
	if wanted == "" {
		return group
	}
	for _, raw := range repos {
		if !suit(strings.ToLower(raw.Name), wanted) {
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

// suit dit si un nom commence par le préfixe donné suivi d'un séparateur. Les
// deux nomenclatures se lisent ainsi sans que l'appelant ait à choisir : « tp1 »
// retrouve « tp1-emilie-cote », et « a26.5n6.01.tp1 » retrouve
// « a26.5n6.01.tp1.emilie-cote ».
func suit(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) || len(name) <= len(prefix) {
		return false
	}
	return strings.ContainsRune(Separators, rune(name[len(prefix)]))
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
