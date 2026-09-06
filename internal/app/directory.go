package app

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/students"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
)

// L'annuaire prend la hiérarchie à l'envers. Gérer un groupe répond à « qui est
// dans celui-ci » ; l'annuaire répond à « qu'a suivi cette personne », et c'est
// la seule vue où l'on voit quelqu'un revenir d'une session à l'autre.
//
// Le terminal n'en décide rien : « students » dresse l'annuaire, le filtre et
// le trie, comme il le fait pour le navigateur.

// orgRepos charge les dépôts d'une organisation : cache d'abord, puis GitHub.
// Deux écrans s'en servent — la gestion d'un groupe et l'annuaire — et ils
// doivent lire le même inventaire.
func (s *Session) orgRepos(org string, force bool) ([]groups.RepoInfo, error) {
	key := cache.ReposKey(org)
	if !force {
		var cached []groups.RepoInfo
		if s.Cache.Get(key, cache.ReposTTL, &cached) && len(cached) > 0 {
			s.Console.Printf("  %s dépôt(s) %s", s.Console.OK(itoa(len(cached))),
				s.Console.Dim("(cache : "+s.Cache.Describe()+")"))
			return cached, nil
		}
	}

	spin := ui.NewSpinner(s.Console, "Chargement des dépôts de "+org+"…")
	spin.Start()
	repos, err := s.Client.ListOrgRepos(org, func(total int) {
		spin.Detail(itoa(total) + " lus")
	})
	spin.Stop()
	if err != nil {
		return nil, err
	}
	// Les dépôts de service sont écartés ici, une fois : rien de ce qui suit
	// n'a alors à se demander si « .github » est un groupe.
	repos = groups.Ordinary(repos)
	s.Console.Printf("  %s dépôt(s) dans l'organisation.", s.Console.OK(itoa(len(repos))))
	s.Cache.Set(key, repos)
	return repos, nil
}

// Menu de l'annuaire.
var directoryMenu = ui.Options(
	"session", "Ne garder qu'une session",
	"cours", "Ne garder qu'un cours",
	"chercher", "Chercher un nom ou un compte",
	"apres", "Ne garder que les envois postérieurs à une date",
	"avant", "Ne garder que les envois antérieurs à une date",
	"muets", "N'afficher que ceux qui n'ont jamais rien envoyé",
	"tri", "Changer le tri",
	"vider", "Tout effacer",
	"rafraichir", "Recharger depuis GitHub",
	"quitter", "Quitter",
)

// directorySession tient l'écran de l'annuaire : les lignes chargées et les
// critères posés dessus.
type directorySession struct {
	session   *Session
	org       string
	rows      []students.Row
	loaded    bool
	orphelins int
	filter    students.Filter
	sortKey   students.Key
	sortDesc  bool
}

func newDirectorySession(session *Session) *directorySession {
	return &directorySession{
		session:  session,
		org:      session.Settings.Org,
		filter:   session.Options.Filter,
		sortKey:  session.Options.Sort,
		sortDesc: session.Options.SortDesc,
	}
}

// run affiche l'annuaire, puis laisse en régler les critères. En mode script,
// la liste est écrite une fois et l'on s'arrête là : il n'y a personne pour
// répondre.
func (d *directorySession) run() (int, error) {
	if err := d.load(false); err != nil {
		return ExitFailure, err
	}
	d.show()
	if !d.session.Interactive() {
		return ExitOK, nil
	}
	return d.menu()
}

// load dresse l'annuaire des groupes de l'organisation — déclarés ou lus dans
// les noms de dépôts.
func (d *directorySession) load(force bool) error {
	repos, err := d.session.orgRepos(d.org, force)
	if err != nil {
		return err
	}
	store := classroom.Open(classroom.PathNextTo(d.session.ConfigFile))
	visibles := store.Visible(d.org, repos,
		classroom.DefaultsFrom(d.session.Settings))
	d.rows = students.Directory(visibles, repos)
	d.orphelins = students.Unmatched(visibles, repos)
	d.loaded = true
	return nil
}

// show écrit l'annuaire : une personne par ligne, ses cours en face.
func (d *directorySession) show() {
	console := d.session.Console
	visibles := students.Apply(d.rows, d.filter, d.sortKey, d.sortDesc)

	titre := "Étudiants de « " + d.org + " » — " + itoa(len(d.rows)) + " personne(s)"
	if len(visibles) != len(d.rows) {
		titre += ", " + itoa(len(visibles)) + " affichée(s)"
	}
	console.Heading(titre)

	rows := make([][]string, 0, len(visibles))
	for index, ligne := range visibles {
		nom := ligne.FullName
		if nom == "" {
			nom = console.Dim("nom inconnu")
		}
		envoi := ligne.PushedAt
		if envoi == "" {
			envoi = console.Dim("jamais")
		}
		suivis := coursSuivis(ligne)
		if suivis == "" {
			suivis = console.Dim("aucun")
		}
		rows = append(rows, []string{
			itoa(index + 1), nom, "@" + ligne.Username, suivis,
			itoa(len(ligne.Repos)), envoi,
		})
	}
	console.Table([]string{"#", "Nom complet", "Compte", "Cours suivis", "Dépôts", "Dernier envoi"},
		rows, 40)

	if len(visibles) == 0 && len(d.rows) > 0 {
		console.Warning("Personne ne répond aux critères.")
	}
	if len(d.rows) == 0 {
		console.Warning("Aucun étudiant connu dans « %s » : déclarez un groupe et "+
			"importez sa liste.", d.org)
	}
	// Un dépôt dont le dernier niveau ne désigne personne n'est pas quelqu'un de
	// plus : le taire ferait lire une liste trouée comme si elle était entière.
	if d.orphelins > 0 {
		console.Note("%d dépôt(s) ne se rattachent à aucun étudiant connu.", d.orphelins)
	}
	console.Note("%s", d.criteria())
}

// coursSuivis énumère les places des groupes suivis, de la session la plus
// récente à la plus ancienne : c'est l'ordre où l'on cherche quelqu'un.
func coursSuivis(ligne students.Row) string {
	places := make([]string, 0, len(ligne.Enrollments))
	for _, inscription := range ligne.Enrollments {
		places = append(places, inscription.Scope)
	}
	return strings.Join(places, "  ")
}

// criteria dit en une ligne ce que l'annuaire retient : un filtre qui ne se
// voit pas se retourne contre celui qui l'a posé.
func (d *directorySession) criteria() string {
	parts := []string{}
	if d.filter.Session != "" {
		parts = append(parts, "session "+classroom.SessionName(d.filter.Session))
	}
	if d.filter.Course != "" {
		parts = append(parts, "cours "+d.filter.Course)
	}
	if d.filter.Text != "" {
		parts = append(parts, "« "+d.filter.Text+" »")
	}
	if d.filter.PushedAfter != "" {
		parts = append(parts, "envoi après le "+d.filter.PushedAfter)
	}
	if d.filter.PushedBefore != "" {
		parts = append(parts, "envoi avant le "+d.filter.PushedBefore)
	}
	if d.filter.Activity == students.Silent {
		parts = append(parts, "aucun envoi")
	}
	sens := "croissant"
	if d.sortDesc {
		sens = "décroissant"
	}
	tri := map[students.Key]string{
		students.ByName: "nom", students.ByUsername: "compte", students.ByPushed: "dernier envoi",
	}[d.sortKey]
	if tri == "" {
		tri = "nom"
	}
	return strings.Join(append(parts, "tri par "+tri+" ("+sens+")"), " · ")
}

// menu règle les critères de l'annuaire. Ce qu'ils signifient est décidé dans
// « students » : le terminal ne fait que les recueillir, comme le navigateur
// les recueille dans sa barre.
func (d *directorySession) menu() (int, error) {
	for {
		action, err := d.session.Prompt.Choose("Annuaire", directoryMenu, "quitter")
		if err != nil {
			return ExitOK, err
		}
		switch action {
		case "quitter":
			return ExitOK, nil
		case "vider":
			d.filter = students.Filter{}
			d.sortKey, d.sortDesc = students.ByName, false
		case "session":
			if err := d.askSession(); err != nil {
				return ExitOK, err
			}
		case "cours":
			if err := d.askCourse(); err != nil {
				return ExitOK, err
			}
		case "chercher":
			texte, err := d.session.Prompt.Ask(ui.Question{
				Title:      "Nom ou compte (vide pour tout afficher)",
				Default:    d.filter.Text,
				AllowEmpty: true,
			})
			if err != nil {
				return ExitOK, err
			}
			d.filter.Text = texte
		case "apres", "avant":
			if err := d.askDate(action); err != nil {
				return ExitOK, err
			}
		case "muets":
			// Un seul état à basculer : ou bien tout le monde, ou bien ceux qui
			// n'ont jamais rien envoyé.
			if d.filter.Activity == students.Silent {
				d.filter.Activity = students.AnyActivity
			} else {
				d.filter.Activity = students.Silent
			}
		case "tri":
			if err := d.askSort(); err != nil {
				return ExitOK, err
			}
		case "rafraichir":
			if err := d.load(true); err != nil {
				d.session.Console.Failure("%v", err)
				continue
			}
		}
		if _, err := d.filter.Validate(); err != nil {
			d.session.Console.Failure("%v", err)
			continue
		}
		d.show()
	}
}

// askSession propose les sessions que l'annuaire porte, de la plus récente à
// la plus ancienne.
func (d *directorySession) askSession() error {
	options := []ui.Option{{Value: "", Label: "Toutes les sessions"}}
	for _, session := range students.SessionsIn(d.rows) {
		options = append(options, ui.Option{Value: session.Short,
			Label: session.Name + "  (" + session.Short + ")"})
	}
	if len(options) == 1 {
		d.session.Console.Warning("Aucune session dans l'annuaire.")
		return nil
	}
	choix, err := d.session.Prompt.Choose("Session", options, d.filter.Session)
	if err != nil {
		return err
	}
	d.filter.Session = choix
	// Un cours que la session choisie n'a pas porté ne filtrerait plus rien :
	// mieux vaut l'effacer que laisser une liste vide sans raison visible.
	if d.filter.Course != "" && !contientCours(students.CoursesIn(d.rows, choix), d.filter.Course) {
		d.filter.Course = ""
	}
	return nil
}

// askCourse propose les cours de la session retenue, ou tous si aucune ne l'est.
func (d *directorySession) askCourse() error {
	options := []ui.Option{{Value: "", Label: "Tous les cours"}}
	for _, cours := range students.CoursesIn(d.rows, d.filter.Session) {
		options = append(options, ui.Option{Value: cours, Label: cours})
	}
	if len(options) == 1 {
		d.session.Console.Warning("Aucun cours dans l'annuaire.")
		return nil
	}
	choix, err := d.session.Prompt.Choose("Cours", options, d.filter.Course)
	if err != nil {
		return err
	}
	d.filter.Course = choix
	return nil
}

func contientCours(liste []string, cherche string) bool {
	for _, cours := range liste {
		if strings.EqualFold(cours, cherche) {
			return true
		}
	}
	return false
}

// askDate recueille une des deux bornes du dernier envoi.
func (d *directorySession) askDate(borne string) error {
	titre, courant := "Dernier envoi après (AAAA-MM-JJ, vide pour aucune borne)",
		d.filter.PushedAfter
	if borne == "avant" {
		titre, courant = "Dernier envoi avant (AAAA-MM-JJ, vide pour aucune borne)",
			d.filter.PushedBefore
	}
	date, err := d.session.Prompt.Ask(ui.Question{
		Title: titre, Default: courant, AllowEmpty: true,
		Validate: func(value string) (string, error) {
			return students.ParseDate(value, "Dernier envoi")
		},
	})
	if err != nil {
		return err
	}
	if borne == "avant" {
		d.filter.PushedBefore = date
	} else {
		d.filter.PushedAfter = date
	}
	return nil
}

// askSort recueille la colonne de tri et son sens.
func (d *directorySession) askSort() error {
	choix, err := d.session.Prompt.Choose("Trier par", sortMenu, string(d.sortKey))
	if err != nil {
		return err
	}
	key, err := students.ParseKey(choix)
	if err != nil {
		return err
	}
	decroissant, err := d.session.Prompt.Confirm("Du plus grand au plus petit ?",
		key == students.ByPushed)
	if err != nil {
		return err
	}
	d.sortKey, d.sortDesc = key, decroissant
	return nil
}
