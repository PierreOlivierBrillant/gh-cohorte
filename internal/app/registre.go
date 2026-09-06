package app

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
)

// Les noms accumulés sur ce poste ne montent pas d'eux-mêmes au registre : ils
// sont dans un fichier que plus rien ne lira. Cet écran les y verse, une fois,
// après avoir montré ce qu'il ferait — comme tout ce qui écrit dans cet outil.

// publishRegistry montre puis, si on l'accorde, publie.
func (s *Session) publishRegistry() (int, error) {
	org := s.Settings.Org
	s.Console.Heading("Registre des étudiants de « " + org + " »")

	set, avis := s.names(org)
	if avis != "" {
		s.Console.Print(s.Console.Warn(avis))
	}
	store := classroom.Open(classroom.PathNextTo(s.ConfigFile))
	locales := store.People(org)
	if len(locales) == 0 {
		s.Console.Note("Ce poste ne connaît aucun étudiant dans « %s » : "+
			"il n'y a rien à publier.", org)
		return ExitOK, nil
	}

	plan := registry.Plan(set, locales)
	s.showPublication(plan, set.Len())
	if avertissement := s.registryOf(org).Exposure(); avertissement != "" {
		s.Console.Blank()
		s.Console.Print(s.Console.Warn(avertissement))
	}

	if plan.Empty() {
		s.Console.Blank()
		s.Console.Note("Le registre connaît déjà tout ce que ce poste sait : rien à publier.")
		return ExitOK, nil
	}
	if s.Options.DryRun {
		s.Console.Blank()
		s.Console.Note("Simulation : rien n'a été écrit.")
		return ExitOK, nil
	}

	if !s.Options.Yes {
		suite, err := s.Prompt.Confirm(plural(
			"Publier %d fiche(s) dans « "+org+" » ?", plan.Count()), false)
		if err != nil {
			return ExitOK, err
		}
		if !suite {
			s.Console.Warning("Annulé : rien n'a été publié.")
			return ExitAborted, nil
		}
	}

	publie, err := s.registryOf(org).Apply(plan.Apply(s.Options.PreferLocal))
	if err != nil {
		return ExitFailure, err
	}
	s.Console.Success("%d fiche(s) publiée(s) ; le registre en compte %d.",
		plan.Count(), publie.Len())
	// Le fichier local reste en place : publier n'efface rien, et rien
	// n'oblige à recommencer si quelque chose s'est mal passé.
	s.Console.Note("Le fichier des groupes de ce poste est inchangé.")
	return ExitOK, nil
}

// showPublication écrit ce que la publication ferait.
func (s *Session) showPublication(plan registry.Publication, connues int) {
	console := s.Console
	console.Printf("  Le registre connaît %s fiche(s) ; ce poste en apporte %s à écrire.",
		console.Info(itoa(connues)), console.OK(itoa(plan.Count())))

	if len(plan.New) > 0 {
		console.Blank()
		console.Printf("  %s", console.OK(plural("%d nouvelle(s) fiche(s)", len(plan.New))))
		rows := make([][]string, 0, len(plan.New))
		for _, fiche := range plan.New {
			rows = append(rows, []string{fiche.FullName, "@" + fiche.Username})
		}
		console.Table([]string{"Nom complet", "Compte"}, rows, 15)
	}

	if len(plan.Renamed) > 0 {
		console.Blank()
		garde := "le registre garde le sien"
		if s.Options.PreferLocal {
			garde = "celui de ce poste l'emporte"
		}
		console.Printf("  %s — %s",
			console.Warn(plural("%d désaccord(s) de nom", len(plan.Renamed))), garde)
		rows := make([][]string, 0, len(plan.Renamed))
		for _, desaccord := range plan.Renamed {
			rows = append(rows, []string{
				"@" + desaccord.Username, desaccord.Registry, desaccord.Local})
		}
		console.Table([]string{"Compte", "Au registre", "Sur ce poste"}, rows, 0)
	}

	if len(plan.Ambiguous) > 0 {
		console.Blank()
		console.Printf("  %s : ce poste les nomme de plusieurs façons. Le premier "+
			"est retenu ; les autres restent rattachés par leur slug.",
			console.Warn(plural("%d compte(s) ambigu(s)", len(plan.Ambiguous))))
		rows := make([][]string, 0, len(plan.Ambiguous))
		for _, ambigu := range plan.Ambiguous {
			rows = append(rows, []string{
				"@" + ambigu.Username, ambigu.Chosen, strings.Join(ambigu.Names, " · ")})
		}
		console.Table([]string{"Compte", "Retenu", "Trouvés"}, rows, 0)
	}

	if len(plan.Nameless) > 0 {
		console.Blank()
		console.Printf("  %s : leur nom complet est inconnu, rien ne peut être publié "+
			"pour eux. %s",
			console.Warn(plural("%d compte(s) sans nom", len(plan.Nameless))),
			console.Dim("@"+strings.Join(plan.Nameless, ", @")))
	}
}
