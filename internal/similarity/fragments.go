package similarity

import "sort"

// Un score dit qu'il y a ressemblance ; il ne dit pas où. Les fragments le
// disent, et c'est d'eux que dépend tout ce qui rend un rapport vérifiable :
// le surlignage côte à côte, la mesure du plus long passage commun, la part de
// chaque fichier réellement recouverte.
//
// Reconstituer un fragment revient à recoudre des empreintes isolées. Deux
// empreintes appartiennent au même passage si elles sont décalées de la même
// distance dans les deux fichiers — autrement dit si elles sont sur la même
// diagonale. Un passage recopié plus haut ou plus bas garde sa diagonale ; un
// passage réécrit la perd, et le fragment s'arrête là. C'est exactement la
// coupure qu'on veut.

// Fragment est un passage commun aux deux fichiers, en rangs de jetons, la fin
// exclue.
type Fragment struct {
	LeftStart  int `json:"left_start"`
	LeftEnd    int `json:"left_end"`
	RightStart int `json:"right_start"`
	RightEnd   int `json:"right_end"`
}

// Length rend la longueur du fragment en jetons. Les deux côtés ont la même :
// un fragment est une diagonale.
func (f Fragment) Length() int { return f.LeftEnd - f.LeftStart }

// seed est une empreinte partagée, située dans les deux fichiers.
type seed struct{ left, right int }

// weave recoud les empreintes partagées en fragments.
//
// L'écart toléré entre deux empreintes d'un même fragment est la fenêtre de
// winnowing, et ce n'est pas un réglage : l'algorithme garantit qu'au moins une
// empreinte est retenue par fenêtre de w k-grammes consécutifs. Deux empreintes
// d'un même passage continu ne peuvent donc pas être plus éloignées que cela.
// Tolérer davantage recoudrait deux passages sans rapport ; tolérer moins
// couperait un passage continu en morceaux.
func weave(seeds []seed, kgram, window int) []Fragment {
	if len(seeds) == 0 {
		return nil
	}
	if kgram < 1 {
		kgram = 1
	}
	if window < 1 {
		window = 1
	}

	// Trier par diagonale puis par position range côte à côte tout ce qui
	// appartient au même passage : il n'y a plus qu'à parcourir.
	sort.Slice(seeds, func(first, second int) bool {
		firstDiagonal := seeds[first].left - seeds[first].right
		secondDiagonal := seeds[second].left - seeds[second].right
		if firstDiagonal != secondDiagonal {
			return firstDiagonal < secondDiagonal
		}
		return seeds[first].left < seeds[second].left
	})

	fragments := make([]Fragment, 0, 8)
	start, previous := seeds[0], seeds[0]
	flush := func() {
		fragments = append(fragments, Fragment{
			LeftStart: start.left, LeftEnd: previous.left + kgram,
			RightStart: start.right, RightEnd: previous.right + kgram,
		})
	}

	for _, current := range seeds[1:] {
		sameDiagonal := current.left-current.right == previous.left-previous.right
		if sameDiagonal && current.left-previous.left <= window {
			previous = current
			continue
		}
		flush()
		start, previous = current, current
	}
	flush()

	sort.Slice(fragments, func(first, second int) bool {
		return fragments[first].LeftStart < fragments[second].LeftStart
	})
	return fragments
}

// longest rend la longueur du plus long fragment, en jetons.
func longest(fragments []Fragment) int {
	most := 0
	for _, fragment := range fragments {
		if length := fragment.Length(); length > most {
			most = length
		}
	}
	return most
}

// covered rend le nombre de jetons d'un côté que les fragments recouvrent.
//
// Les fragments se chevauchent — deux passages recopiés peuvent se recouper
// dans le fichier de gauche sans se recouper dans celui de droite —, et les
// additionner compterait deux fois les mêmes jetons. On fusionne donc les
// intervalles avant de compter : la part annoncée d'un fichier ne peut pas
// dépasser cent pour cent.
func covered(fragments []Fragment, left bool) int {
	if len(fragments) == 0 {
		return 0
	}
	spans := make([][2]int, 0, len(fragments))
	for _, fragment := range fragments {
		if left {
			spans = append(spans, [2]int{fragment.LeftStart, fragment.LeftEnd})
		} else {
			spans = append(spans, [2]int{fragment.RightStart, fragment.RightEnd})
		}
	}
	sort.Slice(spans, func(first, second int) bool {
		return spans[first][0] < spans[second][0]
	})

	total, start, end := 0, spans[0][0], spans[0][1]
	for _, span := range spans[1:] {
		if span[0] > end {
			total += end - start
			start, end = span[0], span[1]
			continue
		}
		if span[1] > end {
			end = span[1]
		}
	}
	return total + end - start
}
