// Package app assemble l'outil : lecture des drapeaux, assistant de création,
// gestion d'un groupe existant et options avancées.
package app

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Version de l'extension, renseignée à la compilation.
var Version = "dev"

// Sentinelle : distingue « drapeau absent » de « drapeau sans valeur ».
const unset = "\x00absent"

// Options rassemble les drapeaux de la ligne de commande.
type Options struct {
	Org              string
	Manage           string // vide = choisir le groupe dans la liste
	ManageRequested  bool
	Roster           string
	Assignment       string
	Template         string
	TemplateSet      bool
	Pattern          string
	Starter          string
	StarterSet       bool
	CommitMessage    string
	ForceStarter     bool
	Visibility       string
	Permission       string
	NoCollaborator   bool
	NoVerifyAccounts bool
	DryRun           bool
	Yes              bool
	NonInteractive   bool
	Web              bool
	CLI              bool
	NoBrowser        bool
	Delay            float64
	DelaySet         bool
	Jobs             int
	Depth            int
	Host             string
	ConfigPath       string
	ReportDir        string
	NoSaveConfig     bool
	NoCache          bool
	ClearCache       bool
	ShowVersion      bool

	// Réglés par les tests seulement : jamais exposés en ligne de commande.
	BaseURL  string
	CacheDir string
}

// Usage décrit l'outil et ses drapeaux.
func Usage(out io.Writer) {
	champs := make([]string, 0, len(plan.Placeholders))
	for _, name := range plan.Placeholders {
		champs = append(champs, "{"+name+"}")
	}
	fmt.Fprintf(out, `gh cohorte %s — un dépôt GitHub par personne, à la manière de GitHub Classroom.

Utilisation :
  gh cohorte                                  interface graphique dans le navigateur
  gh cohorte --cli                            assistant interactif au terminal
  gh cohorte --manage tp1                     gérer le groupe « tp1 »
  gh cohorte --roster cohorte.csv --dry-run   simulation, sans rien créer
  gh cohorte --org acme --assignment tp1 --roster cohorte.csv --yes

Drapeaux :
  --org ORG                organisation GitHub cible
  --manage [PREFIXE]       gérer un groupe existant au lieu d'en créer un
  --roster FICHIER         liste « nom complet, compte GitHub » au format CSV
  --assignment NOM         identifiant du travail (préfixe des dépôts)
  --template ORG/DEPOT     dépôt modèle (vide = dépôt neuf initialisé)
  --pattern GABARIT        gabarit de nom des dépôts (défaut : {assignment}-{username})
  --starter DOSSIER        dossier local déposé dans chaque dépôt, en un commit
  --commit-message TEXTE   message du commit des fichiers de départ
  --force-starter          déposer même dans un dépôt déjà garni
  --visibility private|public
  --permission pull|triage|push|maintain|admin
  --no-collaborator        ne pas inviter les personnes
  --no-verify-accounts     ne pas vérifier l'existence des comptes
  --delay SECONDES         marge entre deux créations (défaut : 1 s)
  --jobs N                 travaux menés en parallèle (défaut : 4)
  --depth N                profondeur d'historique au clonage (0 = complet)
  --dry-run                simuler sans rien créer
  -y, --yes                passer la confirmation finale
  --non-interactive        échouer plutôt que poser une question
  --web                    ouvrir l'interface graphique sur la boucle locale (défaut)
  --cli                    rester au terminal : assistant interactif
  --no-browser             ne pas ouvrir le navigateur, afficher l'adresse
  --host HOTE              hôte GitHub (github.com ou instance Enterprise)
  --config FICHIER         fichier de réglages
  --report-dir DOSSIER     dossier des bilans (défaut : rapports)
  --no-save-config         ne pas mémoriser les réglages
  --no-cache               ignorer le cache local
  --clear-cache            vider le cache local puis quitter
  --version                afficher la version

Champs des gabarits : %s

Codes de retour : 0 succès, 1 au moins un échec, 2 erreur de validation,
130 interruption ou annulation.
`, Version, strings.Join(champs, ", "))
}

// Parse analyse les arguments de la ligne de commande.
func Parse(args []string, out io.Writer) (*Options, error) {
	// L'aide est traitée avant tout : le paquet flag l'écrirait en anglais.
	for _, argument := range args {
		if argument == "-h" || argument == "--help" || argument == "help" {
			Usage(out)
			return nil, flag.ErrHelp
		}
	}

	options := &Options{}
	set := flag.NewFlagSet("cohorte", flag.ContinueOnError)
	// Les messages du paquet flag sont en anglais : ils sont remplacés plus bas.
	set.SetOutput(io.Discard)
	set.Usage = func() {}

	manage := set.String("manage", unset, "gérer un groupe existant")
	template := set.String("template", unset, "dépôt modèle")
	starter := set.String("starter", unset, "dossier de fichiers de départ")
	delay := set.Float64("delay", -1, "marge entre deux créations")

	set.StringVar(&options.Org, "org", "", "organisation GitHub cible")
	set.StringVar(&options.Roster, "roster", "", "liste des personnes")
	set.StringVar(&options.Assignment, "assignment", "", "identifiant du travail")
	set.StringVar(&options.Pattern, "pattern", "", "gabarit de nom des dépôts")
	set.StringVar(&options.CommitMessage, "commit-message", "", "message du commit")
	set.BoolVar(&options.ForceStarter, "force-starter", false, "déposer même dans un dépôt garni")
	set.StringVar(&options.Visibility, "visibility", "", "visibilité des dépôts")
	set.StringVar(&options.Permission, "permission", "", "droit accordé")
	set.BoolVar(&options.NoCollaborator, "no-collaborator", false, "ne pas inviter")
	set.BoolVar(&options.NoVerifyAccounts, "no-verify-accounts", false, "ne pas vérifier les comptes")
	set.BoolVar(&options.DryRun, "dry-run", false, "simuler")
	set.BoolVar(&options.Yes, "yes", false, "passer la confirmation")
	set.BoolVar(&options.Yes, "y", false, "passer la confirmation")
	set.BoolVar(&options.NonInteractive, "non-interactive", false, "ne poser aucune question")
	set.BoolVar(&options.Web, "web", false, "ouvrir l'interface graphique locale")
	set.BoolVar(&options.CLI, "cli", false, "rester au terminal")
	set.BoolVar(&options.NoBrowser, "no-browser", false, "ne pas ouvrir le navigateur")
	set.IntVar(&options.Jobs, "jobs", 4, "travaux en parallèle")
	set.IntVar(&options.Depth, "depth", 0, "profondeur d'historique")
	set.StringVar(&options.Host, "host", "", "hôte GitHub")
	set.StringVar(&options.ConfigPath, "config", "", "fichier de réglages")
	set.StringVar(&options.ReportDir, "report-dir", "rapports", "dossier des bilans")
	set.BoolVar(&options.NoSaveConfig, "no-save-config", false, "ne pas mémoriser les réglages")
	set.BoolVar(&options.NoCache, "no-cache", false, "ignorer le cache")
	set.BoolVar(&options.ClearCache, "clear-cache", false, "vider le cache puis quitter")
	set.BoolVar(&options.ShowVersion, "version", false, "afficher la version")

	if err := set.Parse(normalizeArgs(args)); err != nil {
		return nil, translateFlagError(err)
	}
	if rest := set.Args(); len(rest) > 0 {
		return nil, valid.Errorf(
			"Argument inattendu : « %s ». Lancez « gh cohorte --help » pour la liste des drapeaux.",
			rest[0])
	}

	if *manage != unset {
		options.ManageRequested = true
		options.Manage = *manage
	}
	if *template != unset {
		options.TemplateSet = true
		options.Template = *template
	}
	if *starter != unset {
		options.StarterSet = true
		options.Starter = *starter
	}
	if *delay >= 0 {
		options.DelaySet = true
		options.Delay = *delay
	}
	if options.Jobs < 1 {
		options.Jobs = 1
	}
	if options.Depth < 0 {
		options.Depth = 0
	}
	return options, nil
}

// translateFlagError met en français les messages du paquet flag.
func translateFlagError(err error) error {
	message := err.Error()
	if name, found := strings.CutPrefix(message, "flag provided but not defined: "); found {
		return valid.Errorf(
			"Drapeau inconnu : « %s ». Lancez « gh cohorte --help » pour la liste des drapeaux.", name)
	}
	if rest, found := strings.CutPrefix(message, "flag needs an argument: "); found {
		return valid.Errorf("Valeur manquante pour le drapeau %s.", rest)
	}
	if strings.HasPrefix(message, "invalid value ") {
		return valid.Errorf("Valeur invalide : %s.", strings.TrimPrefix(message, "invalid value "))
	}
	return valid.Errorf("Ligne de commande : %s.", message)
}

// normalizeArgs permet d'écrire « --manage tp1 » comme « --manage=tp1 », et
// « --manage » seul comme « --manage= ». Le paquet flag ne sait pas gérer seul
// un drapeau dont la valeur est facultative.
func normalizeArgs(args []string) []string {
	optional := map[string]bool{"-manage": true, "--manage": true}
	normalized := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !optional[argument] {
			normalized = append(normalized, argument)
			continue
		}
		if index+1 < len(args) && !strings.HasPrefix(args[index+1], "-") {
			normalized = append(normalized, argument+"="+args[index+1])
			index++
			continue
		}
		normalized = append(normalized, argument+"=")
	}
	return normalized
}
