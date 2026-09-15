package similarity

import "github.com/PierreOlivierBrillant/gh-cohorte/internal/tokens"

// Le hachage et le winnowing, c'est-à-dire tout ce qui transforme un flux de
// jetons en empreintes.
//
// Deux propriétés comptent, et il faut les deux. Le hachage doit être roulant :
// un fichier de cent mille jetons donne cent mille k-grammes, et les hacher un
// par un de zéro coûterait k fois trop cher. La sélection doit être locale :
// c'est elle qui garantit qu'un même passage donne les mêmes empreintes dans
// deux fichiers où il n'est pas au même endroit — ce qu'un « un k-gramme sur
// dix » ne garantit pas du tout.

// base est le multiplicateur du hachage polynomial. C'est le nombre premier de
// FNV, impair, ce qui suffit ici : la distribution est rattrapée à la sortie.
const base uint64 = 0x100000001b3

// offset est l'amorce de FNV-1a.
const offset uint64 = 0xcbf29ce484222325

// Fingerprints rend les empreintes d'un flux de jetons.
//
// Un flux plus court que k ne donne rien. C'est voulu : bâtir un k-gramme plus
// court pour un fichier de huit jetons ferait s'apparier toutes les amorces de
// classe de l'organisation, et il faudrait ensuite les écarter comme bruit. Un
// fichier trop court pour être comparé doit être signalé comme tel, pas
// comparé mal.
func Fingerprints(stream []tokens.Token, kgram, window int) []Print {
	return Winnow(Hashes(stream, kgram), window)
}

// Hashes rend le haché de chaque k-gramme du flux, dans l'ordre.
func Hashes(stream []tokens.Token, kgram int) []uint64 {
	if kgram < 1 || len(stream) < kgram {
		return nil
	}
	units := make([]uint64, len(stream))
	for index, token := range stream {
		units[index] = hashText(token.Text)
	}

	// power vaut base^(kgram-1) : c'est le poids du jeton qui sort de la
	// fenêtre. L'arithmétique déborde et c'est exactement ce qu'on veut — un
	// hachage se calcule modulo 2^64, et Go le fait sans rien dire.
	power := uint64(1)
	for index := 0; index < kgram-1; index++ {
		power *= base
	}

	rolling := uint64(0)
	for index := 0; index < kgram; index++ {
		rolling = rolling*base + units[index]
	}

	hashes := make([]uint64, 0, len(units)-kgram+1)
	hashes = append(hashes, scramble(rolling))
	for index := kgram; index < len(units); index++ {
		rolling = (rolling-units[index-kgram]*power)*base + units[index]
		hashes = append(hashes, scramble(rolling))
	}
	return hashes
}

// Winnow ne retient qu'une empreinte par fenêtre : la plus petite, et la plus à
// droite quand plusieurs sont à égalité.
//
// Choisir la plus à droite n'est pas un détail. À égalité, deux fenêtres qui se
// chevauchent désignent alors le même k-gramme, et l'empreinte n'est pas
// retenue deux fois : on garde le même pouvoir de détection pour moins
// d'empreintes. Choisir la plus à gauche en retiendrait davantage sans rien
// apporter.
//
// Une suite plus courte qu'une fenêtre en forme une seule : un fichier court
// doit garder au moins une empreinte, sans quoi il serait invisible.
func Winnow(hashes []uint64, window int) []Print {
	if len(hashes) == 0 {
		return nil
	}
	if window < 1 {
		window = 1
	}
	if len(hashes) < window {
		window = len(hashes)
	}

	prints := make([]Print, 0, len(hashes)/window+1)
	// queue tient les rangs des candidats, du plus petit haché au plus grand.
	// Le plus petit de la fenêtre est donc toujours en tête.
	queue := make([]int, 0, window)
	last := -1

	for index, hash := range hashes {
		// « >= » plutôt que « > » : à égalité, l'ancien sort et le nouveau
		// reste, ce qui fait gagner le plus à droite.
		for len(queue) > 0 && hashes[queue[len(queue)-1]] >= hash {
			queue = queue[:len(queue)-1]
		}
		queue = append(queue, index)
		// Le candidat de tête est sorti de la fenêtre.
		if queue[0] <= index-window {
			queue = queue[1:]
		}
		if index < window-1 {
			continue
		}
		if chosen := queue[0]; chosen != last {
			prints = append(prints, Print{Hash: hashes[chosen], Index: chosen})
			last = chosen
		}
	}
	return prints
}

// hashText hache le texte d'un jeton, par FNV-1a.
func hashText(text string) uint64 {
	hash := offset
	for index := 0; index < len(text); index++ {
		hash ^= uint64(text[index])
		hash *= base
	}
	return hash
}

// scramble brasse un haché polynomial.
//
// Un hachage polynomial sur des mots courts laisse ses bits de poids faible
// très peu variés : deux k-grammes qui ne diffèrent que par leur dernier jeton
// ont des hachés voisins. Le winnowing compare des hachés entre eux pour
// choisir le plus petit ; s'ils sont voisins, il choisit presque toujours au
// même endroit, et la sélection perd son caractère local. C'est le mélangeur
// de splitmix64, qui règle cela pour trois multiplications.
func scramble(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31
	return value
}
