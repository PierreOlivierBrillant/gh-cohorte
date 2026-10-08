// Package brand est le seul endroit qui sache comment l'outil s'appelle et de
// quelles couleurs il se peint. Tout ce qui nomme le produit — bannière du
// terminal, titre de la page web, aide, dépôts de service — vient d'ici, pour
// qu'un changement d'identité se fasse en un seul fichier.
//
// Milou est le fox-terrier de Tintin : il flaire, il court, il rapporte, et il
// ne lâche pas une piste. C'est ce qu'on demande à l'outil — un dépôt par
// personne, rapporté sans se faire prier, et des groupes dont il ne perd
// personne. Le thème marie deux mondes : la structure sobre de GitHub, que les
// enseignants connaissent, et la « ligne claire » des albums — aplats francs,
// contours nets, pas de dégradé.
package brand

import (
	"os"
	"path/filepath"
)

// Identité.
const (
	// Name est le nom affiché : en-tête de la page web, bannière du terminal.
	Name = "Milou"
	// Command est ce qu'on tape : l'extension s'appelle par son dépôt, et
	// « gh » en retire le préfixe.
	Command = "gh milou"
	// Slug nomme les dossiers de réglages et de cache, et l'extension installée.
	Slug = "milou"
	// Tagline tient en une ligne ce que fait l'outil.
	Tagline = "Un dépôt GitHub par personne, rapporté au pas de course"
	// Repository est le dépôt de l'extension, celui que « gh extension
	// install » attend.
	Repository = "PierreOlivierBrillant/gh-milou"
	// URL mène au dépôt.
	URL = "https://github.com/" + Repository
)

// LegacySlugs sont les noms que l'outil a portés, du plus récent au plus
// ancien. Les dossiers qui les portent encore sont adoptés au premier
// lancement (voir AdoptLegacyDir) : un renommage ne doit faire perdre à
// personne ses groupes déclarés.
var LegacySlugs = []string{"nestor", "cohorte"}

// Palette « ligne claire ». Les mêmes valeurs sont déclarées dans
// internal/web/assets/theme.css, où le navigateur les lit ; un test du paquet
// web vérifie que les deux ne divergent pas. Elles sont en toutes lettres
// plutôt que prises de la palette du terminal : un « bleu » de thème clair y
// est souvent délavé, et ce qui est retenu doit se voir partout pareil.
const (
	// Encre trace les contours et habille la barre du haut : le trait de
	// l'album, et le nez de Milou.
	Encre = "#1c1d2b"
	// Papier est le fond des pages, celui d'un album.
	Papier = "#fbf7ee"
	// Bleu est l'accent — le chandail de Tintin — : liens, ligne retenue, et
	// le carré sur lequel Milou pose sa tête dans le logo.
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

// AdoptLegacyDir renvoie le dossier de l'outil sous base — « base/milou » —
// après avoir repris, s'il existe encore seul, le dossier qu'un ancien nom y
// avait laissé. Le renommage est silencieux et ne se fait qu'une fois ; s'il
// échoue, l'ancien dossier reste en service plutôt que d'être abandonné avec
// ce qu'il contient.
func AdoptLegacyDir(base string) string {
	current := filepath.Join(base, Slug)
	if _, err := os.Stat(current); err == nil {
		return current
	}
	for _, slug := range LegacySlugs {
		legacy := filepath.Join(base, slug)
		if _, err := os.Stat(legacy); err != nil {
			continue
		}
		if err := os.Rename(legacy, current); err != nil {
			return legacy
		}
		return current
	}
	return current
}
