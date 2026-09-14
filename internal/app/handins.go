package app

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/identity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// La date cible et les remises, au terminal. L'assistant gère un préfixe, et un
// préfixe de la nomenclature courante est un travail : « a26.5n6.01.tp1 » dit à
// la fois le groupe et le travail, et c'est tout ce qu'il faut pour retrouver
// l'échéance retenue et confronter les historiques à elle.
//
// Un préfixe hérité — « tp1-jlpicard » et ses semblables — ne dit pas à quel
// groupe il appartient : les commits s'y comptent quand même, mais rien ne s'y
// rapporte à une date cible. Le déplacer vers une place lui donne le reste.

// travail résout le groupe et le nom du travail que le préfixe géré désigne.
// Le second retour vaut faux pour un préfixe qui ne dit pas sa place.
func (m *manageSession) travail(group *groups.Group) (classroom.Classroom, string, bool) {
	place, nom, reconnu := naming.SplitAssignment(group.Prefix)
	if !reconnu {
		return classroom.Classroom{}, "", false
	}
	cours, err := classroom.AtScope(m.org, place, classroom.DefaultsFrom(m.session.Settings))
	if err != nil {
		return classroom.Classroom{}, "", false
	}
	if connu, existe := m.session.groupStore().Find(cours.Org, cours.Scope()); existe {
		cours = connu
	}
	set, _ := m.session.names(cours.Org)
	// Le registre répond aux deux questions qu'un groupe lui pose : qui se
	// cache derrière un nom de dépôt, et quand chaque travail est attendu.
	return cours.Scheduling(set).Enrich(set, m.repos), nom, true
}

// equipesDe lit les équipes du groupe, une fois pour la séance. Leur absence
// n'empêche rien : le travail est alors individuel, et c'est déjà ce que la
// liste vide dit.
func (m *manageSession) equipesDe(cours classroom.Classroom) []teams.Team {
	if m.equipesLues {
		return m.equipes
	}
	m.equipesLues = true
	infos, err := m.session.Client.LoadOrgTeams(cours.Org, m.session.Options.Jobs)
	if err != nil {
		return nil
	}
	m.equipes = cours.Teams(infos)
	return m.equipes
}

// bilans confronte les dépôts affichés à ce qu'on sait déjà de leurs
// historiques. Rien n'est demandé à GitHub : c'est « remises » qui va chercher,
// et la liste ne montre que ce qui a déjà été relevé.
func (m *manageSession) bilans(group *groups.Group) map[string]classroom.Review {
	noms := make([]string, 0, group.Len())
	for _, repo := range group.Repos {
		noms = append(noms, repo.Name)
	}
	remises := m.resolver.Handins(m.org, noms, identity.Cached, nil)
	if len(remises) == 0 {
		return nil
	}
	cours, nom, reconnu := m.travail(group)
	trouves := make(map[string]classroom.Review, len(remises))
	if !reconnu {
		// Sans place, les commits se comptent mais rien ne se juge : il n'y a
		// ni date cible ni liste d'étudiants à confronter.
		for repo, remise := range remises {
			trouves[repo] = classroom.Review{
				Repo: repo, Commits: remise.Commits, Last: remise.Last,
			}
		}
		return trouves
	}
	due, _ := valid.ParseDue(cours.DueOf(nom))
	equipes := m.equipesDe(cours)
	for repo, remise := range remises {
		trouves[repo] = cours.Review(repo, remise, due, equipes)
	}
	return trouves
}

// remises relève les historiques des dépôts du groupe, puis remontre la liste.
// C'est deux requêtes par dépôt : le geste est explicite, et son résultat
// mémorisé — la liste le reprend ensuite sans rien redemander.
func (m *manageSession) remises(group *groups.Group) error {
	noms := make([]string, 0, group.Len())
	for _, repo := range group.Repos {
		noms = append(noms, repo.Name)
	}
	if len(noms) == 0 {
		return valid.Errorf("Aucun dépôt dans « %s ».", group.Prefix)
	}
	console := m.session.Console
	progression := ui.NewProgress(console, "Historiques", len(noms))
	remises := m.resolver.Handins(m.org, noms, identity.Refresh,
		func(done, _ int, repo string) { progression.Update(done, repo) })
	progression.Clear()
	if manquants := len(noms) - len(remises); manquants > 0 {
		console.Warning("%d dépôt(s) n'ont pas pu être lus : leur historique reste inconnu.",
			manquants)
	}

	cours, nom, reconnu := m.travail(group)
	if !reconnu {
		console.Note("« %s » ne dit pas à quel groupe il appartient : les commits se "+
			"comptent, mais aucune date cible ne s'y rapporte.", group.Prefix)
		m.show(group)
		return nil
	}
	if due := cours.DueOf(nom); due != "" {
		console.Note("Date cible de « %s » : %s.", nom, due)
	} else {
		console.Note("« %s » n'a pas de date cible : rien n'y sera dit en retard.", nom)
	}
	m.show(group)
	return nil
}

// echeance demande la date cible du travail géré et l'enregistre.
func (m *manageSession) echeance(group *groups.Group) error {
	cours, nom, reconnu := m.travail(group)
	if !reconnu {
		return valid.Errorf(
			"« %s » ne dit pas à quel groupe il appartient : une date cible s'attache "+
				"à un travail d'un groupe. Déplacez-le d'abord vers une place "+
				"« session%[2]scours%[2]sgroupe ».", group.Prefix, naming.Separator)
	}
	console := m.session.Console
	console.Note("Une date seule vaut la fin de la journée. Laissez vide pour retirer "+
		"la date cible de « %s ».", nom)
	reponse, err := m.session.Prompt.Ask(ui.Question{
		Title:      "Date cible (AAAA-MM-JJ ou AAAA-MM-JJTHH:MM)",
		Default:    cours.DueOf(nom),
		AllowEmpty: true,
		Validate:   valid.NormalizeDue,
	})
	if err != nil {
		return err
	}
	return m.fixerEcheance(cours, nom, reponse)
}

// fixerEcheance porte au registre de l'organisation la date cible d'un travail,
// et dit ce qu'elle est devenue.
//
// Elle monte là où vivent déjà les noms que les dépôts ne disent pas : une date
// fixée ici vaut pour l'équipe entière, et suit d'un poste à l'autre.
func (m *manageSession) fixerEcheance(cours classroom.Classroom, nom, due string) error {
	lignes, err := cours.SetDue(nom, due)
	if err != nil {
		return err
	}
	set, err := m.session.registryOf(cours.Org).Apply(echeances(lignes))
	if err != nil {
		return err
	}
	if fixee := set.Due(cours.AssignmentID(nom)); fixee != "" {
		m.session.Console.Success("« %s » est à remettre le %s.", nom, fixee)
	} else {
		m.session.Console.Success("« %s » n'a plus de date cible.", nom)
	}
	return nil
}

// resumeDeRemise dit en deux cases ce qu'un dépôt a reçu : combien de commits,
// et ce qu'il faut en penser — le retard d'abord, les silences ensuite. Un
// dépôt qu'on n'a pas encore relevé ne prétend rien : ses deux cases sont
// vides, et ce n'est pas la même chose qu'un dépôt vide.
func resumeDeRemise(console *ui.Console, bilan classroom.Review, connu bool) (string, string) {
	if !connu {
		return console.Dim("—"), console.Dim("—")
	}
	commits := itoa(bilan.Commits)
	if bilan.Late {
		return commits, console.Err("en retard")
	}
	if bilan.Missing() {
		noms := make([]string, 0, len(bilan.Silent))
		for _, personne := range bilan.Silent {
			noms = append(noms, nommer(personne))
		}
		return commits, "rien de " + strings.Join(noms, ", ")
	}
	if bilan.Commits == 0 {
		return commits, console.Dim("aucun commit")
	}
	return commits, "à jour"
}

// dueFromFlag applique la date cible donnée en ligne de commande, sans rien
// demander. Elle est déjà mise en forme par la lecture des drapeaux : une date
// mal écrite arrête la ligne de commande avant qu'aucun appel n'ait lieu.
func (m *manageSession) dueFromFlag(group *groups.Group) error {
	cours, nom, reconnu := m.travail(group)
	if !reconnu {
		return valid.Errorf(
			"--due : « %s » ne dit pas à quel groupe il appartient. Une date cible "+
				"s'attache à un travail d'un groupe — déplacez-le d'abord vers une "+
				"place « session%[2]scours%[2]sgroupe ».", group.Prefix, naming.Separator)
	}
	return m.fixerEcheance(cours, nom, m.session.Options.Due)
}
