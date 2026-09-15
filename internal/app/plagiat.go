package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/exchange"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plagiarism"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/rules"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/similarity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// La détection de plagiat, au terminal.
//
// Ce que le navigateur montre en quatre écrans tient ici en un enchaînement :
// ce que le profil retient, ce que l'analyse coûtera, la confirmation,
// l'analyse, le tableau des paires. Les deux vues de comparaison, elles, n'ont
// pas d'équivalent au terminal — surligner deux fichiers côte à côte et sauter
// d'un fragment au suivant n'a pas de sens dans un flux de texte. Le rapport
// étant un fichier, l'assistant dit où il est : l'interface web l'ouvre sans
// rien relancer.

// plagiat analyse le travail géré.
func (m *manageSession) plagiat(group *groups.Group) error {
	console := m.session.Console
	cours, nom, reconnu := m.travail(group)
	if !reconnu {
		return valid.Errorf(
			"« %s » ne dit pas à quel groupe il appartient : la détection de plagiat "+
				"a besoin d'un travail d'un groupe déclaré.", group.Prefix)
	}

	portee, err := corpus.ParseReach(m.session.Options.Reach)
	if err != nil {
		return err
	}
	declarees, err := m.reglesDeComparaison()
	if err != nil {
		return err
	}
	cibles, err := m.plagiatCibles(cours, nom, portee, declarees)
	if err != nil {
		return err
	}
	reglage, err := m.session.Options.inspection()
	if err != nil {
		return err
	}
	inspector, err := inspect.New(reglage, declarees.Profiles)
	if err != nil {
		return err
	}

	console.Heading("Détection de plagiat — " + nom)
	console.Note("%s", plagiarism.Disclaimer)
	console.Printf("Portée : %s — %d copie(s), %s.",
		corpus.ReachLabels[portee], len(cibles), placesDites(cibles))
	if archives := liste(m.session.Options.ImportZip); len(archives) > 0 {
		console.Printf("Archives reçues : %s.", strings.Join(archives, ", "))
	}
	console.Blank()

	options := corpus.Options{
		Inspector: inspector,
		Kgram:     m.session.Options.Kgram, Window: m.session.Options.Window,
		Jobs: m.session.Options.Jobs,
	}
	apercu := corpus.Sample(m.session.Client, cibles, options, corpus.DefaultSample)
	apercu.Estimate = apercu.Estimate.Against(corpus.RunnerBudget)
	montrerApercu(console, inspector.Profile(), apercu)

	if !apercu.Estimate.Fits && m.session.Options.NonInteractive {
		return valid.Errorf(
			"Détection de plagiat : l'analyse dépasse le budget prévu. " +
				"Restreignez le profil ou les langages, ou relancez sans « --non-interactive ».")
	}
	if !m.session.Options.Yes {
		suite, err := m.session.Prompt.Confirm(
			fmt.Sprintf("Analyser %d copie(s) ?", len(cibles)), apercu.Estimate.Fits)
		if err != nil || !suite {
			return err
		}
	}

	requete := plagiarism.Request{
		// L'identifiant complet désigne le travail sans ambiguïté : deux
		// groupes qui donnent chacun un « tp1 » ne produisent pas deux rapports
		// de même nom.
		Assignment: cours.AssignmentID(nom), Org: m.org,
		Targets: cibles, Inspection: reglage,
		Rules: declarees, Archives: liste(m.session.Options.ImportZip),
		Indexes: liste(m.session.Options.Against),
		Kgram:   m.session.Options.Kgram, Window: m.session.Options.Window,
		Noise: m.session.Options.Noise, MinSimilarity: m.session.Options.MinSimilarity,
		Jobs: m.session.Options.Jobs,
	}
	if !m.session.Options.NoBaseline {
		requete.Baseline = plagiatGabarit(cours)
	}

	progression := ui.NewProgress(console, "Copies", len(cibles))
	rapport, err := plagiarism.RunFrom(plagiarism.Sources{
		Client:  m.session.Client,
		Indexes: exchange.NewStore(m.session.Client, m.org),
		Prior:   anciensRapports(m.session.Options.ReportDir),
	}, requete, func(done, _ int, id string) { progression.Update(done, id) })
	progression.Clear()
	if err != nil {
		return err
	}

	if rapport.Screened > 0 {
		console.Note("%d copie(s) dépistées contre un index publié : on en a mesuré "+
			"les ressemblances sans en lire une ligne.", rapport.Screened)
	}
	if rapport.Received > 0 {
		console.Note("%d copie(s) venues d'une archive : elles n'ont ni dépôt ni "+
			"nom, et seul leur expéditeur sait qui elles sont.", rapport.Received)
	}
	if rapport.Reused > 0 {
		console.Note("%d copie(s) reprise(s) d'une analyse antérieure : leurs "+
			"dépôts n'ont rien reçu depuis, il n'y avait rien à retélécharger.",
			rapport.Reused)
	}
	montrerRapport(console, rapport)
	chemin, csv, err := rapport.Save(m.session.Options.ReportDir)
	if err != nil {
		return err
	}
	// Le rapport est écrit avant le refus : même stérile, il dit ce qui s'est
	// passé dépôt par dépôt, et c'est justement ce qu'il faut regarder.
	if err := rapport.Barren(); err != nil {
		console.Note("Rapport : %s", chemin)
		return err
	}
	console.Note("Rapport : %s", chemin)
	console.Note("Paires en CSV : %s", csv)
	console.Note("Pour comparer deux copies côte à côte, ouvrez l'interface web " +
		"(gh cohorte) : la vue de comparaison n'a pas d'équivalent au terminal.")
	return nil
}

// reglesDeComparaison rend ce que l'organisation déclare, surchargé par le
// fichier passé en argument.
func (m *manageSession) reglesDeComparaison() (rules.Rules, error) {
	set, avis := m.session.names(m.org)
	if avis != "" {
		m.session.Console.Print(m.session.Console.Warn(avis))
	}
	declarees := rules.Rules{}
	if set != nil {
		declarees = set.Rules()
	}
	return declarees.Merge(m.session.Rules).Validate()
}

// plagiatCibles dresse la liste des copies à comparer.
//
// La sélection appartient au domaine : l'assistant n'y ajoute que ce que seul
// ce poste sait, c'est-à-dire les noms complets et la date du dernier envoi de
// chaque dépôt.
func (m *manageSession) plagiatCibles(cours classroom.Classroom, nom string,
	portee corpus.Reach, declarees rules.Rules) ([]corpus.Target, error) {

	repos, err := m.loadRepos(false)
	if err != nil {
		return nil, err
	}
	cibles, err := corpus.Select(m.org, repos, cours.AssignmentID(nom), portee, declarees)
	if err != nil {
		return nil, err
	}

	envois := make(map[string]string, len(repos))
	for _, repo := range repos {
		envois[strings.ToLower(repo.Name)] = repo.PushedAt
	}
	set, _ := m.session.names(m.org)
	for index := range cibles {
		cibles[index].PushedAt = envois[strings.ToLower(cibles[index].Repo)]
		cibles[index].Label = nomDeCopie(cours, set, cibles[index].Repo)
		// L'identité ne sert pas à l'analyse : elle sert à l'effacer, le jour
		// où l'on anonymise ces copies ou qu'on en publie l'index.
		cibles[index].Person = identiteDe(cours, set, cibles[index].Repo)
	}
	return cibles, nil
}

// identiteDe retrouve la personne derrière un dépôt.
func identiteDe(cours classroom.Classroom, set *registry.Set,
	repo string) roster.Person {
	if student, inscrit := cours.StudentOf(repo); inscrit {
		return student
	}
	if user, connu := set.Find(repo); connu {
		return user.Person()
	}
	if parts, reconnu := naming.Parse(repo); reconnu && set != nil {
		if user, connu := set.Resolve(parts.Student); connu {
			return user.Person()
		}
	}
	return roster.Person{}
}

// nomDeCopie retrouve le nom complet derrière un dépôt : le groupe ouvert pour
// les siens, le registre de l'organisation pour les autres sessions.
func nomDeCopie(cours classroom.Classroom, set *registry.Set, repo string) string {
	if student, inscrit := cours.StudentOf(repo); inscrit && student.FullName != "" {
		return student.FullName
	}
	return set.NameFor(repo)
}

// placesDites nomme les places d'où viennent les copies.
func placesDites(cibles []corpus.Target) string {
	places := corpus.Places(cibles)
	if len(places) == 1 {
		return "toutes de « " + places[0] + " »"
	}
	return strconv.Itoa(len(places)) + " places : " + strings.Join(places, ", ")
}

// anciensRapports relit les derniers rapports de ce poste, pour reprendre ce
// qu'ils ont déjà empreinté plutôt que de le retélécharger.
func anciensRapports(directory string) []*plagiarism.Report {
	chemins := plagiarism.List(directory)
	if len(chemins) > MaxPriorReports {
		chemins = chemins[:MaxPriorReports]
	}
	anciens := make([]*plagiarism.Report, 0, len(chemins))
	for _, chemin := range chemins {
		if rapport, err := plagiarism.Load(chemin); err == nil {
			anciens = append(anciens, rapport)
		}
	}
	return anciens
}

// MaxPriorReports borne ce qu'on relit pour chercher des empreintes
// réutilisables.
const MaxPriorReports = 8

// plagiatGabarit rend le dépôt modèle du groupe, quand il y en a un.
func plagiatGabarit(cours classroom.Classroom) []corpus.Target {
	modele := strings.TrimSpace(cours.Defaults.Template)
	owner, repo, coupe := strings.Cut(modele, "/")
	if !coupe || owner == "" || repo == "" {
		return nil
	}
	return []corpus.Target{{
		ID: modele, Label: "gabarit distribué", Owner: owner, Repo: repo,
	}}
}

// montrerApercu dit ce que le profil retient et ce que l'analyse coûtera.
func montrerApercu(console *ui.Console, profil inspect.Profile, apercu corpus.Preview) {
	console.Printf("Profil d'inspection : %s", console.Bold(profil.Label))
	if profil.Note != "" {
		console.Print(console.Dim("  " + profil.Note))
	}

	for _, echantillon := range apercu.Samples {
		if echantillon.Problem != nil {
			console.Warning("%s : %s", echantillon.Repo, echantillon.Problem.Reason)
			continue
		}
		console.Printf("  %s — %d fichier(s) retenu(s), %d écarté(s)%s",
			echantillon.Repo, len(echantillon.Kept), len(echantillon.Skipped),
			racineDite(echantillon.Root))
	}

	estimation := apercu.Estimate
	console.Blank()
	console.Printf("Estimation sur %d copie(s) mesurée(s) : %d copies, %d paires, "+
		"%s à télécharger, %s de code retenu, %s de mémoire, environ %s.",
		estimation.Sampled, estimation.Repos, estimation.Pairs,
		corpus.Bytes(estimation.Download), corpus.Bytes(estimation.Bytes),
		corpus.Bytes(estimation.Memory), corpus.Duration(estimation.Seconds))
	for _, avertissement := range estimation.Warnings {
		console.Warning("%s", avertissement)
	}
	for _, remede := range estimation.Remedies {
		console.Note("%s — %s", remede.Label, remede.Detail)
	}
	console.Blank()
}

// racineDite nomme l'emballage retenu, quand il y en a un : une racine mal
// devinée explique à elle seule une copie qui paraît vide.
func racineDite(root string) string {
	if emballage := corpus.Packaging(root); emballage != "" {
		return " (racine : " + emballage + ")"
	}
	return ""
}

// montrerRapport affiche les paires, du plus suspect au moins suspect.
func montrerRapport(console *ui.Console, rapport *plagiarism.Report) {
	console.Blank()
	if écartées := rapport.Result.IgnoredBaseline + rapport.Result.IgnoredCommon; écartées > 0 {
		console.Note("%d empreinte(s) écartée(s) : %d venant du gabarit distribué, "+
			"%d présentes chez trop de copies pour vouloir dire quelque chose.",
			écartées, rapport.Result.IgnoredBaseline, rapport.Result.IgnoredCommon)
	}

	if rapport.Analyzed() < 2 {
		// Rien n'a été comparé : le dire vert serait un mensonge. Les motifs
		// suivent, dépôt par dépôt.
		montrerSoucis(console, rapport)
		return
	}
	if len(rapport.Result.Matches) == 0 {
		console.Success("Aucune paire au-dessus du seuil : rien ne ressort de ces %d copie(s).",
			rapport.Analyzed())
	} else {
		lignes := make([][]string, 0, len(rapport.Result.Matches))
		for _, match := range rapport.Result.Matches {
			lignes = append(lignes, []string{
				match.LeftName, match.RightName,
				fmt.Sprintf("%.0f %%", match.Similarity*100),
				fmt.Sprintf("%.0f %% / %.0f %%",
					match.LeftCoverage*100, match.RightCoverage*100),
				strconv.Itoa(match.LongestFragment),
				repere(rapport.Result.Threshold, match.Similarity),
			})
		}
		console.Heading("Paires, du plus suspect au moins suspect")
		console.Table([]string{
			"Copie", "Copie", "Similarité", "Couverture", "Plus long", "",
		}, lignes, 25)
	}

	if rapport.Result.Threshold > 0 {
		console.Note("Seuil : %.0f %%. %s",
			rapport.Result.Threshold*100, rapport.Result.ThresholdNote)
	} else if rapport.Result.ThresholdNote != "" {
		console.Note("%s", rapport.Result.ThresholdNote)
	}
	montrerSignaux(console, rapport)
	montrerSoucis(console, rapport)
}

// repere marque d'un signe les paires au-dessus du seuil.
func repere(seuil, similarite float64) string {
	if seuil > 0 && similarite >= seuil {
		return "au-dessus du seuil"
	}
	return ""
}

// montrerSignaux rapporte ce que le winnowing ne voit pas.
func montrerSignaux(console *ui.Console, rapport *plagiarism.Report) {
	if len(rapport.Result.Signals) == 0 {
		return
	}
	console.Heading("Signaux relevés hors de la mesure de similarité")
	for _, signal := range rapport.Result.Signals {
		switch signal.Kind {
		case similarity.SharedSignature:
			console.Warning("Même signature dans %s.", strings.Join(signal.Works, ", "))
		case similarity.SharedComment:
			console.Warning("Commentaire identique dans %s : « %s »",
				strings.Join(signal.Works, ", "), abrege(signal.Detail))
		}
	}
}

// montrerSoucis nomme les dépôts qui n'ont pas pu être analysés.
//
// Jamais en note de bas de page : un rapport qui tait des dépôts laisse croire
// qu'ils ont été regardés.
func montrerSoucis(console *ui.Console, rapport *plagiarism.Report) {
	if len(rapport.Problems) == 0 {
		return
	}
	console.Heading(fmt.Sprintf("%d dépôt(s) n'ont pas pu être analysés",
		len(rapport.Problems)))
	lignes := make([][]string, 0, len(rapport.Problems))
	for _, souci := range rapport.Problems {
		lignes = append(lignes, []string{souci.Repo, souci.Reason, abrege(souci.Detail)})
	}
	console.Table([]string{"Dépôt", "Motif", "Détail"}, lignes, 0)
}

func abrege(texte string) string {
	const limite = 70
	runes := []rune(texte)
	if len(runes) <= limite {
		return texte
	}
	return string(runes[:limite-1]) + "…"
}
