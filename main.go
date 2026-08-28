// gh-cohorte crée un dépôt GitHub par personne dans une organisation, à la
// manière de GitHub Classroom, puis aide à gérer les groupes déjà créés.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
	"github.com/mattn/go-isatty"
)

// version est renseignée à la compilation (-ldflags "-X main.version=…").
var version = "dev"

func main() {
	app.Version = version

	options, err := app.Parse(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(app.ExitOK)
		}
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		os.Exit(app.ExitValidation)
	}
	if options.ShowVersion {
		fmt.Println("gh cohorte " + version)
		os.Exit(app.ExitOK)
	}

	console := ui.NewConsole()
	os.Exit(app.New(options, console, prompter(console, options)).Run())
}

// prompter choisit la façon de poser les questions : listes aux flèches sur un
// vrai terminal, listes numérotées quand la sortie est redirigée mais qu'une
// personne reste au clavier, et refus pur et simple en mode script.
func prompter(console *ui.Console, options *app.Options) ui.Prompter {
	if options.NonInteractive || !isTerminal(os.Stdin) {
		return &ui.ScriptPrompter{}
	}
	if console.TTY && os.Getenv("COHORTE_NO_ARROWS") == "" {
		return ui.NewPrompter(console)
	}
	return ui.NewLinePrompter(console, os.Stdin)
}

func isTerminal(file *os.File) bool {
	return isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
}
