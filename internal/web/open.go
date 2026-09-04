package web

import (
	"os/exec"
	"runtime"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// launcher renvoie la commande qui ouvre une adresse dans le navigateur par
// défaut du système.
func launcher(system string) (string, []string) {
	switch system {
	case "darwin":
		return "open", nil
	case "windows":
		// rundll32 accepte l'adresse telle quelle, sans les pièges de
		// guillemets de « cmd /c start ».
		return "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		return "xdg-open", nil
	}
}

// Open demande au système d'ouvrir l'interface dans le navigateur. Un échec
// n'est jamais bloquant : l'adresse reste affichée dans le terminal.
func Open(target string) error {
	name, arguments := launcher(runtime.GOOS)
	path, err := exec.LookPath(name)
	if err != nil {
		return valid.Errorf("« %s » est introuvable.", name)
	}
	command := exec.Command(path, append(arguments, target)...)
	if err := command.Start(); err != nil {
		return err
	}
	// Le navigateur vit sa vie ; sans cette attente, le processus resterait
	// zombie jusqu'à la fin de la session.
	go func() { _ = command.Wait() }()
	return nil
}
