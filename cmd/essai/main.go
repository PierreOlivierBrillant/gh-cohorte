package main

import (
	"fmt"
	"os"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/complete"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ui"
)

func main() {
	prompteur := ui.NewPrompter(ui.NewConsole())
	mode := complete.Path
	if len(os.Args) > 2 && os.Args[2] == "dir" {
		mode = complete.Dir
	}
	valeur, err := prompteur.Ask(ui.Question{Title: "Chemin du fichier CSV", Default: os.Args[1], Complete: mode})
	fmt.Printf("\nRESULTAT=[%s] ERREUR=%v\n", valeur, err)
}
