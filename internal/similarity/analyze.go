package similarity

import "github.com/PierreOlivierBrillant/gh-cohorte/internal/tokens"

// Le pont entre les jetons et l'index : c'est ici, et nulle part ailleurs, que
// le contenu d'un fichier devient une suite d'empreintes. Tout ce qui est en
// aval ne voit plus que des hachés — c'est ce qui permet à un index de circuler
// sans que le code circule avec lui.

// TooShort dit qu'un fichier n'a pas assez de jetons pour qu'on en tire un seul
// k-gramme. Ce n'est pas une erreur : c'est un fichier qui n'a pas de quoi être
// comparé, et le rapport doit le dire plutôt que de le passer sous silence.
type TooShort struct {
	Tokens int
	Kgram  int
}

// Analyze empreinte un fichier analysé.
//
// Des bornes nulles prennent celles du langage. Les passer explicitement ne
// sert qu'à un réglage d'analyse : un enseignant qui monte k pour alléger le
// calcul doit le faire pour tous les fichiers à la fois, sans quoi les
// fragments de deux langages ne se compareraient plus.
func Analyze(path string, result tokens.Result, kgram, window int) (File, *TooShort) {
	kgram, window = bounds(result.Language, kgram, window)
	file := File{
		Path: path, Language: result.Language, Tokens: len(result.Tokens),
		Kgram: kgram, Window: window,
	}
	if len(result.Tokens) < kgram {
		return file, &TooShort{Tokens: len(result.Tokens), Kgram: kgram}
	}
	file.Prints = Fingerprints(result.Tokens, kgram, window)
	return file, nil
}

// bounds arrête les bornes de winnowing d'un fichier.
func bounds(language string, kgram, window int) (int, int) {
	known, found := tokens.Get(language)
	if kgram <= 0 {
		kgram = tokens.CodeKgram
		if found {
			kgram = known.Kgram
		}
	}
	if window <= 0 {
		window = tokens.CodeWindow
		if found {
			window = known.Window
		}
	}
	return kgram, window
}

// Baseline rend les empreintes d'un gabarit, à écarter de la comparaison.
//
// C'est ce que l'outil peut faire et que les outils génériques ne peuvent pas :
// il sait quel dépôt modèle ou quel dossier de départ il a distribué, donc il
// sait exactement ce que toutes les copies ont en commun sans que personne
// n'ait rien copié. Dolos demande à l'enseignant de le lui désigner ; ici,
// personne n'a rien à désigner.
func Baseline(works ...Work) map[uint64]struct{} {
	prints := map[uint64]struct{}{}
	for _, work := range works {
		for _, file := range work.Files {
			for _, print := range file.Prints {
				prints[print.Hash] = struct{}{}
			}
		}
	}
	return prints
}

// Lines rend les lignes que recouvre un intervalle de jetons, bornes comprises.
// C'est ce qui traduit un fragment — qui se compte en jetons — en quelque chose
// qu'on peut surligner dans un fichier.
func Lines(stream []tokens.Token, start, end int) (from, to int) {
	if len(stream) == 0 {
		return 0, 0
	}
	if start < 0 {
		start = 0
	}
	if end > len(stream) {
		end = len(stream)
	}
	if start >= end {
		return 0, 0
	}
	from, to = stream[start].Line, stream[start].Line
	for _, token := range stream[start:end] {
		if token.Line < from {
			from = token.Line
		}
		if token.Line > to {
			to = token.Line
		}
	}
	return from, to
}

// Ordinary rend les signaux que le gabarit distribué porte lui-même.
//
// Ils sont retirés d'office, exactement comme ses empreintes : un commentaire
// d'en-tête imposé et un message d'erreur fourni dans le squelette se
// retrouvent dans toutes les copies, et les rapporter comme des coïncidences
// noierait les vraies sous les fausses.
func Ordinary(works ...Work) Signals {
	ordinary := Signals{}
	for _, work := range works {
		ordinary = ordinary.Merged(work.Extras)
	}
	return ordinary
}
