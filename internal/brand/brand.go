// Package brand est le seul endroit qui sache comment l'outil s'appelle et de
// quelles couleurs il se peint. Tout ce qui nomme le produit — bannière du
// terminal, titre de la page web, aide, dépôts de service — vient d'ici, pour
// qu'un changement d'identité se fasse en un seul fichier.
//
// Nestor est le majordome de Moulinsart : il sert, il range, et rien ne le
// démonte. C'est ce qu'on demande à l'outil — un dépôt par personne, livré sur
// un plateau, et des groupes tenus en ordre. Le thème marie deux mondes : la
// structure sobre de GitHub, que les enseignants connaissent, et la « ligne
// claire » des albums — aplats francs, contours nets, pas de dégradé.
package brand

import (
	"os"
	"path/filepath"
)

// Identité.
const (
	// Name est le nom affiché : en-tête de la page web, bannière du terminal.
	Name = "Nestor"
	// Command est ce qu'on tape : l'extension s'appelle par son dépôt, et
	// « gh » en retire le préfixe.
	Command = "gh nestor"
	// Slug nomme les dossiers de réglages et de cache, et l'extension installée.
	Slug = "nestor"
	// LegacySlug est l'ancien nom. Les dossiers qui le portent encore sont
	// adoptés au premier lancement (voir AdoptLegacyDir) : un renommage ne
	// doit faire perdre à personne ses groupes déclarés.
	LegacySlug = "cohorte"
	// Tagline tient en une ligne ce que fait l'outil.
	Tagline = "Un dépôt GitHub par personne, servi sur un plateau"
	// Repository est le dépôt de l'extension, celui que « gh extension
	// install » attend.
	Repository = "PierreOlivierBrillant/gh-nestor"
	// URL mène au dépôt.
	URL = "https://github.com/" + Repository
)

// Palette « ligne claire ». Les mêmes valeurs sont déclarées dans
// internal/web/assets/theme.css, où le navigateur les lit ; un test du paquet
// web vérifie que les deux ne divergent pas. Elles sont en toutes lettres
// plutôt que prises de la palette du terminal : un « bleu » de thème clair y
// est souvent délavé, et ce qui est retenu doit se voir partout pareil.
const (
	// Encre trace les contours et habille la barre du haut : le frac de
	// Nestor.
	Encre = "#1c1d2b"
	// Papier est le fond des pages, celui d'un album.
	Papier = "#fbf7ee"
	// Bleu est l'accent — le chandail de Tintin — : liens, ligne retenue,
	// nœud papillon du logo.
	Bleu = "#2466b0"
	// BleuClair est le même accent porté par du texte, lisible sur fond sombre
	// comme sur fond clair.
	BleuClair = "#3d7fd0"
	// Vert dit ce qui est fait, coché, créé.
	Vert = "#2d9a5a"
	// Rouge dit un refus ou une erreur.
	Rouge = "#d2342b"
	// Jaune signale sans alarmer.
	Jaune = "#f5c431"
	// Gris est ce qui n'est pas retenu.
	Gris = "#7c8496"
)

// AdoptLegacyDir renvoie le dossier de l'outil sous base — « base/nestor » —
// après avoir repris, s'il existe encore seul, le dossier que l'ancien nom y
// avait laissé. Le renommage est silencieux et ne se fait qu'une fois ; s'il
// échoue, l'ancien dossier reste en service plutôt que d'être abandonné avec
// ce qu'il contient.
func AdoptLegacyDir(base string) string {
	current := filepath.Join(base, Slug)
	if _, err := os.Stat(current); err == nil {
		return current
	}
	legacy := filepath.Join(base, LegacySlug)
	if _, err := os.Stat(legacy); err != nil {
		return current
	}
	if err := os.Rename(legacy, current); err != nil {
		return legacy
	}
	return current
}
