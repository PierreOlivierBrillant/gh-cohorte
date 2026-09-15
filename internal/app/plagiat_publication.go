package app

import (
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/exchange"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plagiarism"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Publier ce qu'on a donné.
//
// Deux choses partent, et elles ne disent pas la même chose. Le catalogue
// annonce qu'un travail existe : une place, un nom, un décompte. L'index
// d'empreintes permet de le comparer sans le lire — ce sont des hachés, on ne
// remonte pas au code depuis eux.
//
// Ni l'un ni l'autre ne nomme un étudiant, et la table qui relie les jetons aux
// personnes ne quitte pas ce poste.

// publierIndex analyse le travail géré et publie son index.
func (m *manageSession) publierIndex(group *groups.Group) error {
	console := m.session.Console
	cours, nom, reconnu := m.travail(group)
	if !reconnu {
		return valid.Errorf(
			"« %s » ne dit pas à quel groupe il appartient : seul un travail d'un "+
				"groupe déclaré se publie.", group.Prefix)
	}

	declarees, err := m.reglesDeComparaison()
	if err != nil {
		return err
	}
	// Ses propres copies, et rien d'autre : publier l'index d'un collègue
	// redistribuerait ce qu'on nous a confié.
	cibles, err := m.plagiatCibles(cours, nom, corpus.ReachGroup, declarees)
	if err != nil {
		return err
	}
	reglage, err := m.session.Options.inspection()
	if err != nil {
		return err
	}

	console.Heading("Publication — " + nom)
	console.Note("Ce qui part : des empreintes et des jetons. Ni code, ni nom. " +
		"La table qui relie les jetons aux personnes reste sur ce poste.")

	requete := plagiarism.Request{
		Assignment: cours.AssignmentID(nom), Org: m.org, Targets: cibles,
		Inspection: reglage, Rules: declarees,
		Kgram: m.session.Options.Kgram, Window: m.session.Options.Window,
		Jobs: m.session.Options.Jobs,
	}
	progression := ui.NewProgress(console, "Copies", len(cibles))
	rapport, err := plagiarism.RunFrom(plagiarism.Sources{
		Client: m.session.Client, Prior: anciensRapports(m.session.Options.ReportDir),
	}, requete, func(done, _ int, id string) { progression.Update(done, id) })
	progression.Clear()
	if err != nil {
		return err
	}
	if err := rapport.Barren(); err != nil {
		montrerSoucis(console, rapport)
		return err
	}

	publie, table, err := plagiarism.Publishable(rapport, m.session.Viewer,
		cours.Scope(), anonymize.Options{})
	if err != nil {
		return err
	}
	console.Printf("  %d copie(s), %d empreinte(s), sous l'étiquette « %s ».",
		publie.Copies(), publie.Prints(), publie.Origin)
	if !m.session.Options.Yes {
		suite, err := m.session.Prompt.Confirm(
			"Publier cet index dans l'organisation ?", true)
		if err != nil || !suite {
			return err
		}
	}

	if err := exchange.NewStore(m.session.Client, m.org).Publish(publie); err != nil {
		return err
	}
	chemin, err := plagiarism.WriteIndexTable(m.session.Options.ReportDir,
		rapport.Basename(), table)
	if err != nil {
		return err
	}

	ligne, err := rapport.Teaching(m.session.Viewer, true)
	if err != nil {
		return err
	}
	if _, err := m.session.registryOf(m.org).Apply(registry.Publish(ligne)); err != nil {
		return err
	}

	console.Success("Index publié dans « %s/%s ».", m.org, exchange.IndexRepo)
	console.Note("Catalogue mis à jour : vos collègues voient que vous avez donné "+
		"« %s » à %d personnes.", ligne.ID(), ligne.Copies)
	console.Note("Table de correspondance, restée ici : %s", chemin)
	return nil
}

// montrerCatalogue liste les travaux d'un enseignant, tels que le catalogue de
// l'organisation les annonce.
//
// C'est ce qui manquait pour comparer avec un collègue : les équipes disent
// déjà quels cours il a donnés, mais rien ne disait quels travaux — leurs noms
// ne se lisent que dans des dépôts qu'on ne voit pas.
func montrerCatalogue(console *ui.Console, catalogue exchange.Catalog, compte string) {
	siens := catalogue.Of(compte)
	if len(siens) == 0 {
		return
	}
	console.Heading("Travaux annoncés au catalogue")
	lignes := make([][]string, 0, len(siens))
	for _, ligne := range siens {
		empreintes := ""
		if ligne.Indexed {
			empreintes = "index publié"
		}
		lignes = append(lignes, []string{
			ligne.ID(), plural("%d copie(s)", ligne.Copies),
			orDim(console, ligne.LastHandin, "—"), empreintes,
		})
	}
	console.Table([]string{"Travail", "Copies", "Dernière remise", ""}, lignes, 0)
	console.Note("Comparer vos copies aux siennes : gh cohorte --plagiarism "+
		"--manage VOTRE-TRAVAIL --against %s — ce sont des empreintes qui "+
		"circulent, jamais du code.", siens[0].ID())
}
