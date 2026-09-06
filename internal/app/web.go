package app

import (
	"context"
	"os"
	"os/signal"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/web"
)

// serveWeb ouvre l'interface graphique locale et la tient jusqu'à l'arrêt.
// L'assistant du terminal ne pose plus de question : tout se passe dans le
// navigateur, et les réglages qui en reviennent sont mémorisés en quittant.
func (s *Session) serveWeb() (int, error) {
	reportDir, err := roster.ExpandPath(s.Options.ReportDir)
	if err != nil {
		reportDir = s.Options.ReportDir
	}

	server, err := web.New(web.Deps{
		Client:      s.Client,
		Cache:       s.Cache,
		Settings:    s.Settings,
		ConfigFile:  s.ConfigFile,
		Viewer:      s.Viewer,
		Host:        s.Client.Host(),
		TokenOrigin: s.tokenOrigin,
		Refresher:   s.Refresher,
		Version:     Version,
		ReportDir:   reportDir,
		Jobs:        s.Options.Jobs,
		Depth:       s.Options.Depth,
		SaveConfig:  !s.Options.NoSaveConfig,
	})
	if err != nil {
		return ExitFailure, err
	}

	s.Console.Heading("Interface web")
	s.Console.Printf("  Adresse : %s", s.Console.OK(server.URL()))
	s.Console.Note("Le serveur n'écoute que sur cette machine et n'accepte que " +
		"cette adresse : le jeton en fait partie.")
	s.Console.Note("Ctrl-C pour fermer, ou « Quitter » dans le navigateur.")

	if !s.Options.NoBrowser {
		if err := web.Open(server.URL()); err != nil {
			s.Console.Warning("Navigateur non ouvert : %v — copiez l'adresse ci-dessus.", err)
		}
	}

	lifetime, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err = server.Serve(lifetime)
	// Ce que l'interface a retenu — organisation, gabarits, dossiers — devient
	// l'état de la session : « persist » l'écrira comme après l'assistant.
	s.Settings = server.Settings()
	if err != nil {
		return ExitFailure, err
	}
	s.Console.Blank()
	s.Console.Success("Interface fermée.")
	return ExitOK, nil
}
