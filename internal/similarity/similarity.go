// Package similarity mesure ce que deux copies ont en commun.
//
// Le procédé est celui de Moss puis de Dolos, et il tient en trois temps. Le
// flux de jetons d'un fichier est découpé en k-grammes — k jetons qui se
// suivent —, chaque k-gramme est haché, et l'on ne retient qu'une partie de ces
// hachés : le plus petit de chaque fenêtre de w. C'est le « winnowing », et il
// a une propriété qu'aucun échantillonnage naïf n'a — deux fichiers qui
// partagent un passage assez long retiennent forcément les mêmes hachés dedans,
// où que ce passage se trouve dans l'un et dans l'autre.
//
// De là découle tout le reste. Réordonner des fonctions ne change rien, puisque
// rien ne dépend de la position. Insérer du code entre deux passages recopiés
// ne change rien non plus. Ce qui casse l'appariement, c'est de réécrire
// vraiment — et c'est précisément ce qu'on veut : un travail réécrit n'est plus
// une copie.
//
// Ce paquet ne connaît ni GitHub, ni les fichiers, ni les étudiants. Il reçoit
// des flux de jetons, il rend des mesures. C'est aussi ce qui permet à l'index
// qu'il produit de circuler entre enseignants sans qu'une ligne de code ne
// circule avec lui : une empreinte est un haché de 64 bits, on ne remonte pas
// au texte depuis elle.
package similarity

import "sort"

// Print est une empreinte retenue : le haché d'un k-gramme, et le rang du
// premier jeton de ce k-gramme dans le flux.
//
// Les noms JSON sont courts à dessein. Un index compte des centaines de
// milliers d'empreintes et voyage d'un enseignant à l'autre ; « hash » et
// « index » écrits au long doubleraient son poids sans rien apprendre à
// personne.
type Print struct {
	Hash  uint64 `json:"h"`
	Index int    `json:"i"`
}

// File est un fichier analysé, tel que l'index le retient.
//
// Il ne porte aucun contenu : un chemin, un langage, un décompte de jetons et
// des empreintes. C'est délibéré — c'est cet objet-là qui traverse le
// cloisonnement entre enseignants, et il ne doit rien pouvoir révéler.
type File struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Tokens   int    `json:"tokens"`
	// Kgram et Window sont les bornes avec lesquelles ce fichier a été
	// empreinté. Elles sont écrites ici plutôt que déduites du langage parce
	// qu'un index voyage : celui d'un collègue a pu être calculé avec d'autres
	// bornes, et interpréter ses empreintes avec les nôtres donnerait des
	// fragments faux sans que rien ne le signale.
	Kgram  int     `json:"k"`
	Window int     `json:"w"`
	Prints []Print `json:"prints"`
}

// Work est une copie : tout ce qu'une personne ou une équipe a remis pour un
// travail.
type Work struct {
	// ID désigne la copie sans ambiguïté — le nom du dépôt, ou un jeton opaque
	// quand elle vient d'ailleurs.
	ID string `json:"id"`
	// Label est ce qu'on affiche. Il vaut l'identifiant quand rien de mieux
	// n'est connu ; il est vide sur une copie anonymisée.
	Label string `json:"label,omitempty"`
	// Origin étiquette la provenance : le groupe, la session, l'enseignant.
	// C'est elle qui colore les nuages de points, et elle seule qui permet de
	// dire « ces deux-là ne sont même pas du même groupe ».
	Origin string  `json:"origin,omitempty"`
	Files  []File  `json:"files"`
	Extras Signals `json:"signals,omitzero"`
}

// Signals porte ce qui se remarque hors du winnowing.
//
// Un commentaire recopié mot pour mot, faute de frappe comprise, dit souvent
// plus qu'un score de similarité : il n'y a aucune raison légitime pour que
// deux personnes écrivent la même phrase de travers. Ces signaux sont donc
// tenus à part et rapportés à part, jamais fondus dans la mesure principale.
type Signals struct {
	// Comments porte les commentaires du travail, déjà réduits.
	Comments []string `json:"comments,omitempty"`
	// Literals porte les chaînes de caractères assez longues pour être
	// parlantes — un message d'erreur, une invite, une phrase affichée.
	//
	// Le flux comparé les réduit toutes à « STR », et c'est voulu : deux
	// copies qui ne diffèrent que par une constante restent appariées. Mais ce
	// qu'on jette là a de la valeur ailleurs — une phrase que deux personnes
	// écrivent identique, faute comprise, ne s'écrit pas deux fois par hasard.
	Literals []string `json:"literals,omitempty"`
	// Signature est le jeton de la signature invisible, quand il y en a une.
	Signature string `json:"signature,omitempty"`
}

// Merged rend l'union de deux jeux de signaux, sans la signature — celle-ci
// désigne une copie, pas un ensemble.
func (s Signals) Merged(other Signals) Signals {
	return Signals{
		Comments: append(append([]string(nil), s.Comments...), other.Comments...),
		Literals: append(append([]string(nil), s.Literals...), other.Literals...),
	}
}

// TokenCount rend le nombre de jetons de la copie entière.
func (w Work) TokenCount() int {
	total := 0
	for _, file := range w.Files {
		total += file.Tokens
	}
	return total
}

// PrintCount rend le nombre d'empreintes de la copie entière.
func (w Work) PrintCount() int {
	total := 0
	for _, file := range w.Files {
		total += len(file.Prints)
	}
	return total
}

// Name rend ce qu'il faut afficher pour cette copie.
func (w Work) Name() string {
	if w.Label != "" {
		return w.Label
	}
	return w.ID
}

// Corpus est l'ensemble de ce qui est comparé. C'est lui qu'on sérialise pour
// le republier, et lui qu'on fusionne quand on ajoute les copies d'une session
// passée ou celles d'un collègue.
type Corpus struct {
	Works []Work `json:"works"`
}

// Add ajoute une copie au corpus. Une copie dont l'identifiant est déjà présent
// remplace la précédente : refaire une analyse ne doit pas dédoubler les
// copies.
func (c *Corpus) Add(work Work) {
	for index, existing := range c.Works {
		if existing.ID == work.ID {
			c.Works[index] = work
			return
		}
	}
	c.Works = append(c.Works, work)
}

// Merge verse un autre corpus dans celui-ci.
func (c *Corpus) Merge(other Corpus) {
	for _, work := range other.Works {
		c.Add(work)
	}
}

// Sort range les copies par identifiant, pour que deux sérialisations d'un même
// corpus soient identiques octet pour octet — sans quoi republier un index
// inchangé produirait un commit à chaque fois.
func (c *Corpus) Sort() {
	sort.Slice(c.Works, func(first, second int) bool {
		return c.Works[first].ID < c.Works[second].ID
	})
	for _, work := range c.Works {
		sort.Slice(work.Files, func(first, second int) bool {
			return work.Files[first].Path < work.Files[second].Path
		})
	}
}

// Empty dit qu'il n'y a rien à comparer.
func (c Corpus) Empty() bool { return len(c.Works) < 2 }
