package similarity

// Compter les empreintes communes à chaque paire de copies est le seul endroit
// du calcul qui soit vraiment quadratique. Un dictionnaire y coûterait une
// recherche de hachage par incrément, des centaines de millions de fois ; un
// tableau plat coûte un calcul d'indice. Au-delà de quelques milliers de
// copies, le tableau pèserait plus qu'il ne fait gagner, et le dictionnaire
// reprend la main.

// counter compte les empreintes communes de chaque paire de copies.
type counter struct {
	works  int
	dense  []int32
	sparse map[[2]int]int
}

func newCounter(works int) *counter {
	if works <= DenseLimit {
		return &counter{works: works, dense: make([]int32, works*(works-1)/2)}
	}
	return &counter{works: works, sparse: map[[2]int]int{}}
}

// slot rend le rang d'une paire dans le tableau plat. La paire est toujours
// rangée dans l'ordre croissant : (3, 7) et (7, 3) sont la même.
func (c *counter) slot(left, right int) int {
	return left*c.works - left*(left+1)/2 + right - left - 1
}

func (c *counter) add(left, right int) {
	if left > right {
		left, right = right, left
	}
	if c.dense != nil {
		c.dense[c.slot(left, right)]++
		return
	}
	c.sparse[[2]int{left, right}]++
}

// each parcourt les paires qui partagent au moins une empreinte.
func (c *counter) each(visit func(left, right, count int)) {
	if c.dense == nil {
		for pair, count := range c.sparse {
			visit(pair[0], pair[1], count)
		}
		return
	}
	for left := 0; left < c.works; left++ {
		for right := left + 1; right < c.works; right++ {
			if count := c.dense[c.slot(left, right)]; count > 0 {
				visit(left, right, int(count))
			}
		}
	}
}
