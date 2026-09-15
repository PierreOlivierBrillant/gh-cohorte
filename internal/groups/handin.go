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
	// LastBy date le commit le plus récent de chaque compte, au même format.
	// Les clés sont en minuscules, et la clé vide porte les commits qu'aucune
	// adresse ne rattache à un compte.
	//
	// Elle ne couvre que les commits les plus récents de la branche, et cela
	// suffit : un auteur qui n'y paraît pas a forcément commis plus tôt que le
	// plus ancien de ceux-là, et ne peut donc être le dernier de personne.
	LastBy map[string]string `json:"last_by,omitempty"`
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

// LastBut rend la date du commit le plus récent dont l'auteur n'est pas écarté.
//
// C'est ce qui date une remise. Un gabarit poussé à l'ouverture du travail, une
// correction déposée après coup, une note ajoutée au dépôt une fois l'échéance
// passée sont l'œuvre de qui enseigne : les compter daterait la remise du jour
// où l'enseignant y a touché, et mettrait l'étudiant en retard pour cela.
//
// Sans auteurs relevés, la date du dernier commit est tout ce qu'on sait, et
// c'est elle qui est rendue : mieux vaut une date trop tardive que pas de date
// du tout.
func (h Handin) LastBut(ignore func(login string) bool) string {
	if ignore == nil || len(h.LastBy) == 0 {
		return h.Last
	}
	dernier := ""
	for login, quand := range h.LastBy {
		if login != "" && ignore(login) {
			continue
		}
		// Les dates sont en UTC et de forme fixe : les comparer comme du texte
		// les range dans l'ordre du temps.
		if quand > dernier {
			dernier = quand
		}
	}
	return dernier
}

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
