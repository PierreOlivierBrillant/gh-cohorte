package similarity

// Un seuil ne se devine pas à l'œil, et le fixer une fois pour toutes ne marche
// pas : la bonne valeur dépend de l'exercice. Un travail très contraint — une
// interface imposée, un squelette à remplir — produit des copies honnêtes qui
// se ressemblent beaucoup ; un travail ouvert produit des copies honnêtes qui
// ne se ressemblent pas du tout. Le même 0,6 est anodin dans le premier cas et
// accablant dans le second.
//
// Ce que la distribution montre, en revanche, se lit partout de la même façon :
// une masse de copies qui se ressemblent un peu, et — s'il y a eu copie — une
// poignée à l'écart, tout à droite. Le seuil proposé est l'endroit qui sépare
// le mieux ces deux populations, au sens où il maximise l'écart entre leurs
// moyennes : c'est la méthode d'Otsu, plus robuste sur peu de données que
// l'ajustement de deux gaussiennes, et qui ne suppose aucune forme de courbe.
//
// Il est proposé, jamais imposé. Ce qui est au-dessus n'est pas du plagiat :
// c'est ce qu'il faut regarder en premier.

// Buckets est le nombre de tranches de l'histogramme. Vingt donne des tranches
// de cinq points, assez fines pour voir une bosse et assez larges pour qu'une
// classe de trente n'ait pas des tranches vides partout.
const Buckets = 20

// MinPairsForThreshold est le nombre de paires en deçà duquel aucun seuil n'est
// proposé. Séparer deux populations dans huit points, c'est en inventer une.
const MinPairsForThreshold = 12

// Bucket est une tranche de l'histogramme des similarités.
type Bucket struct {
	From  float64 `json:"from"`
	To    float64 `json:"to"`
	Count int     `json:"count"`
}

// newHistogram prépare les tranches, de 0 à 1.
func newHistogram() []Bucket {
	histogram := make([]Bucket, Buckets)
	for index := range histogram {
		histogram[index].From = float64(index) / Buckets
		histogram[index].To = float64(index+1) / Buckets
	}
	return histogram
}

// bump range une similarité dans sa tranche.
func bump(histogram []Bucket, similarity float64) {
	slot := int(similarity * Buckets)
	if slot >= Buckets {
		slot = Buckets - 1
	}
	if slot < 0 {
		slot = 0
	}
	histogram[slot].Count++
}

// NoThreshold explique pourquoi aucun seuil n'est proposé. La phrase est ici
// plutôt que dans chaque interface : les trois doivent dire la même chose, et
// « seuil : 0 » sans explication se lit comme « tout est suspect ».
const NoThreshold = "Trop peu de paires mesurées pour distinguer deux " +
	"populations : aucun seuil n'est proposé. Déplacez-le à la main si vous " +
	"voulez filtrer, en sachant qu'il ne repose alors que sur votre jugement."

// ThresholdNote dit ce qu'un seuil proposé vaut.
const ThresholdNote = "Seuil proposé automatiquement : c'est la coupure qui " +
	"sépare le mieux les deux populations de ce corpus. Il n'a rien d'absolu — " +
	"ce qui est au-dessus est à regarder en premier, pas à sanctionner."

// threshold propose la coupure qui sépare le mieux les deux populations, ou
// zéro quand il n'y a pas de quoi le dire.
func threshold(histogram []Bucket) float64 {
	total, weighted := 0, 0.0
	for _, bucket := range histogram {
		total += bucket.Count
		weighted += center(bucket) * float64(bucket.Count)
	}
	if total < MinPairsForThreshold {
		return 0
	}

	best, cut := 0.0, 0.0
	below, belowWeighted := 0, 0.0
	for index := 0; index < Buckets-1; index++ {
		below += histogram[index].Count
		belowWeighted += center(histogram[index]) * float64(histogram[index].Count)
		above := total - below
		if below == 0 || above == 0 {
			continue
		}
		lowMean := belowWeighted / float64(below)
		highMean := (weighted - belowWeighted) / float64(above)
		// La variance interclasse d'Otsu : le produit des effectifs par le
		// carré de l'écart des moyennes. Elle est maximale là où les deux
		// populations sont le plus nettement séparées.
		spread := float64(below) * float64(above) * (highMean - lowMean) * (highMean - lowMean)
		if spread > best {
			best, cut = spread, histogram[index].To
		}
	}
	return cut
}

func center(bucket Bucket) float64 { return (bucket.From + bucket.To) / 2 }
