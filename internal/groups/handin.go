package groups

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Un nom de dépôt dit à qui il appartient ; il ne dit pas ce qu'on y a mis.
// Savoir si un travail a été rendu, par qui, et quand demande son historique —
// la seule chose que l'inventaire d'une organisation ne porte pas.
//
// « pushed_at », que l'inventaire donne pourtant, ne répond pas à la question.
// Un envoi n'est pas un commit : renommer une branche, pousser une étiquette
// ou forcer une réécriture avancent la date sans que rien n'ait été écrit, et
// un commit ancien poussé tard porte la date du jour. Ce qu'on affiche à côté
// d'une date de remise doit être la date du commit, pas celle de son voyage.

// Author est un auteur que l'historique porte sans qu'aucun compte GitHub n'y
// réponde : l'adresse de courriel du commit n'est rattachée à personne.
type Author struct {
	Name    string `json:"name,omitempty"`
	Email   string `json:"email,omitempty"`
	Commits int    `json:"commits"`
}

// Handin est ce que l'historique d'un dépôt dit d'une remise : combien de
// commits, quand fut le dernier, et qui les a faits.
//
// Tout s'y rapporte à la branche par défaut. C'est celle que GitHub montre, et
// celle que l'étudiant croit remettre ; compter les commits d'une branche
// oubliée ferait dire à l'écran qu'un travail a été rendu alors que personne
// n'ira l'y chercher.
type Handin struct {
	// Commits est le nombre de commits de la branche par défaut.
	Commits int `json:"commits"`
	// Last est la date du commit le plus récent, au format RFC 3339. Elle est
	// vide quand le dépôt n'a rien reçu.
	Last string `json:"last,omitempty"`
	// Authors compte les commits de chaque compte GitHub. Les clés sont en
	// minuscules : GitHub ne distingue pas la casse d'un compte.
	Authors map[string]int `json:"authors,omitempty"`
	// Anonymous porte ce que l'historique sait des auteurs sans compte. Un
	// étudiant qui commet depuis une machine mal configurée est là, et nulle
	// part ailleurs : l'oublier le ferait passer pour muet.
	Anonymous []Author `json:"anonymous,omitempty"`
}

// Empty dit qu'aucun commit n'a été relevé dans le dépôt.
func (h Handin) Empty() bool { return h.Commits == 0 }

// By compte les commits d'une personne, tous ses comptes confondus.
func (h Handin) By(accounts []string) int {
	total := 0
	for _, compte := range accounts {
		total += h.Authors[strings.ToLower(strings.TrimSpace(compte))]
	}
	return total
}

// Signed dit si un auteur sans compte porte ce nom ou cette adresse. C'est le
// dernier recours pour rattacher un commit à quelqu'un : le nom écrit dans le
// commit est celui que la personne a donné à git, et il vaut souvent le sien.
//
// La comparaison passe par la slugification, celle-là même qui nomme les
// dépôts : « Émilie Côté », « emilie.cote@college.qc.ca » et « emilie-cote »
// désignent alors la même personne sans qu'on ait à traiter les accents ici.
func (h Handin) Signed(fullName string) bool {
	slug := valid.Slugify(fullName)
	if slug == "" {
		return false
	}
	for _, auteur := range h.Anonymous {
		if valid.Slugify(auteur.Name) == slug {
			return true
		}
		// « prenom.nom@… » : la partie locale d'une adresse porte souvent le
		// nom, et la comparer coûte moins qu'une question à qui enseigne.
		if local, _, coupe := strings.Cut(auteur.Email, "@"); coupe &&
			valid.Slugify(local) == slug {
			return true
		}
	}
	return false
}
