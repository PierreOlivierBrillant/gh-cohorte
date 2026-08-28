package app

import (
	"errors"
	"strings"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/starter"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Codes de retour de l'outil.
const (
	ExitOK         = 0
	ExitFailure    = 1
	ExitValidation = 2
	ExitAborted    = 130
)

// Saisies qui effacent une valeur mémorisée lors d'une exécution précédente.
var clearKeywords = map[string]bool{"-": true, "aucun": true, "none": true, "annuler": true}

// Session déroule une exécution complète, du jeton jusqu'au bilan.
type Session struct {
	Options  *Options
	Console  *ui.Console
	Prompt   ui.Prompter
	Settings config.Settings
	Cache    *cache.Cache
	Client   *ghapi.Client
	Viewer   string
	Starter  *starter.Bundle

	ConfigFile string
	Sleep      func(time.Duration)
	Now        func() time.Time

	manager *manageSession
}

// New prépare une session à partir des drapeaux analysés.
func New(options *Options, console *ui.Console, prompter ui.Prompter) *Session {
	configFile := options.ConfigPath
	if configFile == "" {
		configFile = config.Path()
	}
	store := cache.New(!options.NoCache)
	if options.CacheDir != "" {
		store = cache.NewIn(options.CacheDir, !options.NoCache)
	}
	return &Session{
		Options:    options,
		Console:    console,
		Prompt:     prompter,
		Settings:   config.Load(configFile),
		Cache:      store,
		ConfigFile: configFile,
		Sleep:      time.Sleep,
		Now:        time.Now,
	}
}

// Interactive indique si des questions peuvent être posées.
func (s *Session) Interactive() bool { return s.Prompt.Interactive() }

// require impose une valeur en mode script : une absence y est une erreur.
func (s *Session) require(value, flagName, label string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return value, nil
	}
	return "", valid.Errorf("%s manquant : passez %s en mode non interactif.", label, flagName)
}

// Run enchaîne les étapes et renvoie le code de retour.
func (s *Session) Run() int {
	s.Console.Banner("gh cohorte "+Version, "Un dépôt GitHub par personne, pour une cohorte")

	code, err := s.run()
	if err == nil {
		return code
	}
	s.Console.Blank()
	switch {
	case errors.Is(err, ui.ErrAborted):
		s.Console.Print(s.Console.Warn("Interrompu : rien n'a été laissé à moitié fait."))
		return ExitAborted
	case valid.IsValidation(err):
		s.Console.Print(s.Console.Err("Erreur : " + err.Error()))
		return ExitValidation
	case ghapi.IsGitHub(err):
		s.Console.Print(s.Console.Err("Erreur GitHub : " + err.Error()))
		return ExitFailure
	default:
		s.Console.Print(s.Console.Err("Erreur : " + err.Error()))
		return ExitFailure
	}
}

func (s *Session) run() (int, error) {
	if s.Options.ClearCache {
		// Purge demandée en ligne de commande : ni jeton ni réseau nécessaires.
		removed := s.Cache.Clear()
		s.Console.Success("Cache vidé (%d entrée(s)).", removed)
		return ExitOK, nil
	}

	mode, err := s.chooseMode()
	if err != nil {
		return ExitOK, err
	}
	if mode == "quitter" {
		return ExitOK, nil
	}

	if err := s.authenticate(); err != nil {
		return ExitOK, err
	}
	if err := s.chooseOrg(); err != nil {
		return ExitOK, err
	}

	if mode == "gerer" {
		s.manager = newManageSession(s, s.Options.Manage)
		code, err := s.manager.run()
		s.persist() // dossier de clonage, gabarit, modèle détecté…
		return code, err
	}
	return s.create()
}

// chooseMode décide du mode : création, gestion d'un groupe, options avancées.
func (s *Session) chooseMode() (string, error) {
	if s.Options.ManageRequested {
		return "gerer", nil
	}
	if !s.Interactive() {
		return "creer", nil
	}
	// Un lancement déjà paramétré pour créer ne doit pas poser de question.
	if s.Options.Roster != "" || s.Options.Assignment != "" || s.Options.TemplateSet ||
		s.Options.StarterSet || s.Options.Pattern != "" || s.Options.Yes {
		return "creer", nil
	}

	for {
		choice, err := s.Prompt.Choose("Que voulez-vous faire ?", ui.Options(
			"creer", "Créer des dépôts pour une liste de personnes",
			"gerer", "Lister et gérer un groupe de dépôts existant",
			"avance", "Options avancées",
			"quitter", "Quitter",
		), "creer")
		if err != nil {
			return "", err
		}
		if choice != "avance" {
			return choice, nil
		}
		if err := s.advancedMenu(); err != nil {
			return "", err
		}
	}
}

// authenticate résout le jeton par gh et affiche le compte connecté.
func (s *Session) authenticate() error {
	s.Console.Heading("Authentification GitHub")
	host := s.Options.Host
	if host == "" {
		host = ghapi.DefaultHost()
	}

	token, origin := "", ""
	if s.Options.BaseURL == "" {
		token, origin = ghapi.TokenForHost(host)
		if strings.TrimSpace(token) == "" {
			return valid.Errorf(
				"Aucun jeton disponible pour %s : lancez « gh auth login », "+
					"ou définissez GH_TOKEN.", host)
		}
	} else {
		token, origin = "jeton-de-test", "configuration de test"
	}

	client, err := ghapi.New(ghapi.Options{
		Host: host, Token: token, BaseURL: s.Options.BaseURL, Sleep: s.Sleep, Now: s.Now,
	})
	if err != nil {
		return err
	}
	user, err := client.AuthenticatedUser()
	if err != nil {
		if ghapi.StatusOf(err) == 401 {
			return valid.Errorf(
				"Jeton refusé par GitHub. Vérifiez « gh auth status » " +
					"ou renouvelez-le avec « gh auth refresh -s repo ».")
		}
		return err
	}
	s.Client = client
	s.Viewer = user.Login
	s.Console.Printf("  Jeton fourni par %s.", s.Console.Dim(origin))
	s.Console.Printf("  Connecté en tant que %s sur %s.", s.Console.OK("@"+s.Viewer), s.Console.Dim(host))

	// Un jeton « fine-grained » n'annonce aucune portée : on n'alerte que si la
	// liste existe vraiment.
	if present, known := client.HasScope("repo"); known && !present {
		s.Console.Warning("La portée « repo » semble absente : la création de dépôts privés peut échouer.")
	}
	return nil
}

// chooseOrg retient l'organisation cible et signale un rôle insuffisant.
func (s *Session) chooseOrg() error {
	s.Console.Heading("Organisation cible")
	// En mode script, l'organisation mémorisée sert de repli.
	candidate := s.Options.Org
	if candidate == "" && !s.Interactive() {
		candidate = s.Settings.Org
	}

	for {
		var org string
		var err error
		if candidate != "" {
			org, err = valid.Login(candidate, "Organisation")
			if err != nil {
				if !s.Interactive() {
					return err
				}
				s.Console.Failure("%v", err)
				candidate = ""
				continue
			}
		} else {
			if !s.Interactive() {
				if _, err := s.require("", "--org", "Organisation"); err != nil {
					return err
				}
			}
			answer, err := s.Prompt.Ask(ui.Question{
				Title:   "Organisation GitHub",
				Default: s.Settings.Org,
				Validate: func(value string) (string, error) {
					return valid.Login(value, "Organisation")
				},
			})
			if err != nil {
				return err
			}
			org = answer
		}

		data, err := s.Client.GetOrg(org)
		if err != nil {
			message := err.Error()
			switch ghapi.StatusOf(err) {
			case 404:
				message = "L'organisation « " + org + " » est introuvable ou invisible pour @" + s.Viewer + "."
			case 403:
				message = "Accès refusé à l'organisation « " + org + " »."
			}
			if !s.Interactive() {
				return valid.Errorf("%s", message)
			}
			s.Console.Failure("%s", message)
			candidate = ""
			continue
		}

		s.Settings.Org = org
		label := data.Name
		if label == "" {
			label = org
		}
		s.Console.Printf("  Organisation : %s %s", s.Console.OK(label),
			s.Console.Dim("(github.com/"+org+")"))
		s.warnIfNotAdmin(org)
		return nil
	}
}

// warnIfNotAdmin signale, sans bloquer, un rôle insuffisant pour créer des dépôts.
func (s *Session) warnIfNotAdmin(org string) {
	role, err := s.Client.OrgMembership(org, s.Viewer)
	if err != nil {
		return
	}
	switch role {
	case "":
		s.Console.Warning("Rôle indéterminé dans l'organisation (portée « read:org » absente ?) : " +
			"la création peut échouer.")
	case "admin":
	default:
		s.Console.Warning("Vous êtes « %s » : la création de dépôts doit être autorisée "+
			"aux membres dans les réglages de l'organisation.", role)
	}
}

// persist enregistre les réglages réutilisables ; le jeton n'y figure jamais.
func (s *Session) persist() {
	if s.Options.NoSaveConfig {
		return
	}
	if err := s.Settings.Save(s.ConfigFile); err != nil {
		s.Console.Warning("Réglages non enregistrés : %v", err)
		return
	}
	s.Console.Note("Réglages mémorisés dans %s", s.ConfigFile)
}

// resetSettings repart des réglages par défaut après un oubli volontaire.
func (s *Session) resetSettings() {
	s.Settings = config.Default()
	s.Settings.Org = s.Options.Org
}

// invalidateCaches oublie ce qui a été retenu en mémoire pendant la session.
func (s *Session) invalidateCaches() {
	if s.manager != nil {
		s.manager.forget()
	}
}

// placeholderHint énumère les champs disponibles dans les gabarits.
func placeholderHint() string {
	fields := make([]string, 0, len(plan.Placeholders))
	for _, name := range plan.Placeholders {
		fields = append(fields, "{"+name+"}")
	}
	return strings.Join(fields, ", ")
}
