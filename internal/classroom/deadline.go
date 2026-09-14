package classroom

import (
	"strings"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// La date cible d'un travail est ce qu'aucun dépôt ne peut dire. Leurs noms
// portent la place, le travail et la personne ; rien n'y porte une échéance, et
// GitHub n'a pas d'endroit où la mettre — ni jalon ni étiquette ne s'applique à
// trente dépôts à la fois.
//
// Elle vit donc là où vivent déjà les noms que les dépôts ne disent pas : dans
// le registre de l'organisation. Ce paquet ne la retient pas, il la demande —
// « Schedule » est la seconde des deux questions qu'il pose au registre, et la
// seule chose qu'il en connaisse avec « Lookup ».
//
// Une date fixée ici vaut donc pour l'équipe entière et suit d'un poste à
// l'autre, comme le nom derrière un compte.

// Schedule répond à la question : quand ce travail est-il à remettre ?
//
// Le registre de l'organisation la porte, et c'est tout ce que ce paquet a
// besoin de savoir de lui. En dépendre entièrement le lierait au client GitHub
// et au cache, dont il n'a que faire.
type Schedule interface {
	// Due rend la date cible d'un travail, désigné par son identifiant
	// complet, ou une chaîne vide s'il n'en a pas.
	Due(assignmentID string) string
}

// DueOf rend la date cible d'un travail, par son nom court ou par son
// identifiant complet — les deux désignent le même travail, et l'appelant ne
// devrait pas avoir à choisir.
//
// Un groupe dont on n'a pas versé le calendrier ne répond rien : c'est le cas
// d'un registre illisible, et il vaut mieux ne rien dire que de faire croire à
// un travail sans échéance.
func (c Classroom) DueOf(assignment string) string {
	if c.horaire == nil {
		return ""
	}
	nom := strings.TrimSpace(c.ShortName(strings.TrimSpace(assignment)))
	if nom == "" {
		return ""
	}
	return c.horaire.Due(c.AssignmentID(nom))
}

// Scheduling verse dans le groupe le calendrier de l'organisation. Comme
// « Enrich », il ne retient rien : il branche le groupe sur ce que le registre
// sait, et c'est le registre qui reste la source.
func (c Classroom) Scheduling(horaire Schedule) Classroom {
	c.horaire = horaire
	return c
}

// Deadline dit ce qu'un travail doit devenir : l'identifiant sous lequel son
// échéance se range, et la date qu'il prend. Une date vide la retire.
//
// C'est le vocabulaire dans lequel ce paquet décrit ses intentions ; les
// écrire revient à l'interface qui tient le registre. Rendre ici un changement
// de registre lierait la notion de groupe au client GitHub et au cache, dont
// elle n'a que faire.
type Deadline struct {
	// Assignment est l'identifiant complet du travail — « a26.5n6.01.tp1 ».
	// Le nom court ne suffirait pas : le registre porte toute l'organisation,
	// et deux groupes y ont chacun leur « tp1 ».
	Assignment string
	Due        string
}

// SetDue compose ce qu'il faut écrire pour fixer, changer ou retirer la date
// cible d'un travail du groupe. La date est mise en forme au passage : une date
// mal écrite s'arrête ici, pas au milieu d'une écriture.
//
// Une date vide retire l'échéance : il n'y a pas de geste séparé pour cela,
// c'est la même décision prise dans l'autre sens.
func (c Classroom) SetDue(assignment, due string) ([]Deadline, error) {
	nom := strings.TrimSpace(c.ShortName(strings.TrimSpace(assignment)))
	if nom == "" {
		return nil, valid.Errorf("Date cible : aucun travail indiqué.")
	}
	normale, err := valid.NormalizeDue(due)
	if err != nil {
		return nil, err
	}
	return []Deadline{{Assignment: c.AssignmentID(nom), Due: normale}}, nil
}

// RenameDue fait suivre à un travail renommé la date qu'il avait. Sans cela,
// corriger un nom ferait disparaître son échéance sans que rien ne le dise :
// elle est rangée sous l'identifiant du travail, et il vient de changer.
//
// Rien n'est rendu quand il n'y a rien à reporter.
func (c Classroom) RenameDue(from, to string) []Deadline {
	due := c.DueOf(from)
	if due == "" {
		return nil
	}
	return []Deadline{
		{Assignment: c.AssignmentID(c.ShortName(from))},
		{Assignment: c.AssignmentID(strings.TrimSpace(to)), Due: due},
	}
}

// MoveDue fait suivre à un groupe d'arrivée les dates cibles des travaux qui le
// rejoignent, et les retire de leur identifiant de départ.
//
// Un travail déplacé garde son échéance : c'est le même travail, rangé
// ailleurs. La date est rangée sous l'identifiant qu'il prend à l'arrivée, qui
// porte la place du groupe d'arrivée et le nom qu'on lui donne au passage.
func MoveDue(depart, arrivee Classroom, travaux []Relocation) []Deadline {
	var mouvements []Deadline
	for _, demande := range travaux {
		due := depart.DueOf(demande.ID)
		if due == "" {
			continue
		}
		nom := strings.TrimSpace(demande.Name)
		if nom == "" {
			nom = depart.ShortName(demande.ID)
		}
		mouvements = append(mouvements,
			Deadline{Assignment: depart.AssignmentID(depart.ShortName(demande.ID))},
			Deadline{Assignment: arrivee.AssignmentID(nom), Due: due})
	}
	return mouvements
}

// ------------------------------------------------------------ ce qu'on remet

// Review est ce qu'un dépôt raconte d'une remise, une fois confronté à la date
// cible et aux personnes qu'il vise.
type Review struct {
	Repo    string `json:"repo"`
	Commits int    `json:"commits"`
	// Last est la date du commit le plus récent, au format RFC 3339.
	Last string `json:"last,omitempty"`
	// Late dit qu'un commit suit la date cible. Sans date cible, jamais.
	Late bool `json:"late"`
	// Silent nomme les personnes visées par le dépôt dont aucun commit ne
	// porte la trace. Un dépôt d'équipe en nomme plusieurs ; un dépôt
	// individuel, au plus une.
	Silent []roster.Person `json:"silent,omitempty"`
}

// Missing dit qu'au moins une personne visée n'a rien remis.
func (r Review) Missing() bool { return len(r.Silent) > 0 }

// Targets rend les personnes qu'un dépôt vise : l'étudiant à qui il appartient,
// ou les membres de l'équipe quand le travail est d'équipe.
//
// C'est cette liste que le silence confronte. Elle peut être vide — un dépôt
// hors liste, une équipe dont on ne connaît aucun membre —, et rien ne s'en
// déduit alors : on ne reproche pas son silence à quelqu'un qu'on ne connaît
// pas.
func (c Classroom) Targets(repoName string, equipes []teams.Team) []roster.Person {
	if equipe, appartient := c.TeamOf(repoName, equipes); appartient {
		return c.Members(equipe)
	}
	if student, inscrit := c.StudentOf(repoName); inscrit {
		return []roster.Person{student}
	}
	return nil
}

// Review confronte ce qu'un dépôt a reçu à ce qu'on en attendait.
func (c Classroom) Review(repoName string, remise groups.Handin, due time.Time,
	equipes []teams.Team) Review {
	bilan := Review{Repo: repoName, Commits: remise.Commits, Last: remise.Last}
	if !due.IsZero() && remise.Last != "" {
		if dernier, err := time.Parse(time.RFC3339, remise.Last); err == nil {
			bilan.Late = dernier.After(due)
		}
	}
	for _, personne := range c.Targets(repoName, equipes) {
		if remise.By(personne.Accounts()) > 0 || remise.Signed(personne.FullName) {
			continue
		}
		bilan.Silent = append(bilan.Silent, personne)
	}
	return bilan
}

// Reviews confronte tous les dépôts d'un travail. Les dépôts dont l'historique
// n'a pas été relevé sont absents du résultat : l'écran doit pouvoir distinguer
// « rien remis » de « pas encore regardé ».
func (c Classroom) Reviews(assignmentID string, repos []groups.RepoInfo,
	equipes []teams.Team, remises map[string]groups.Handin) []Review {
	due, _ := valid.ParseDue(c.DueOf(assignmentID))
	bilans := make([]Review, 0, len(remises))
	for _, depot := range c.Repos(assignmentID, repos) {
		remise, releve := remises[depot.Name]
		if !releve {
			continue
		}
		bilans = append(bilans, c.Review(depot.Name, remise, due, equipes))
	}
	return bilans
}

// WithHandins verse dans les travaux ce que les historiques relevés en disent :
// les commits comptés, les retards et les silences.
//
// Les travaux reviennent complétés plutôt que recalculés : ce qui se lit dans
// les noms de dépôts a déjà été établi, et un relevé partiel ne doit rien y
// changer.
func (c Classroom) WithHandins(travaux []Assignment, repos []groups.RepoInfo,
	equipes []teams.Team, remises map[string]groups.Handin) []Assignment {
	if len(remises) == 0 {
		return travaux
	}
	completes := make([]Assignment, 0, len(travaux))
	for _, travail := range travaux {
		for _, bilan := range c.Reviews(travail.ID, repos, equipes, remises) {
			travail.Seen++
			travail.Commits += bilan.Commits
			if bilan.Late {
				travail.Late++
			}
			if bilan.Missing() {
				travail.Silent++
			}
		}
		completes = append(completes, travail)
	}
	return completes
}
