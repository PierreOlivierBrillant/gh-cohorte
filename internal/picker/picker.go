// Package picker ouvre le sélecteur de fichiers du système, quand il y en a un.
//
// Choisir un fichier en tapant son chemin est une corvée, et le navigateur ne
// peut pas aider : par sécurité, une page web ne voit jamais le chemin d'un
// fichier déposé. Le serveur local, lui, tourne sur la machine de la personne
// et peut demander au système d'ouvrir sa propre fenêtre.
//
// Aucun outil n'est tenu pour acquis. Chaque plateforme a son moyen — zenity ou
// kdialog sous Linux, osascript sous macOS, une fenêtre .NET sous Windows — et
// tous sont cherchés à l'exécution. Quand aucun ne répond, Available renvoie
// faux et l'appelant se rabat sur l'explorateur interne de l'interface, qui,
// lui, marche partout.
package picker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrCanceled dit que la fenêtre a été refermée sans rien choisir. Ce n'est pas
// une erreur : l'appelant n'a qu'à ne rien faire.
var ErrCanceled = errors.New("sélection annulée")

// ErrUnavailable dit qu'aucun sélecteur natif n'est joignable sur cette machine.
var ErrUnavailable = errors.New("aucun sélecteur de fichiers du système")

// Attente borne le temps qu'une fenêtre peut rester ouverte : au-delà, elle est
// tenue pour oubliée et le processus est libéré.
const Attente = 10 * time.Minute

// Request décrit la fenêtre à ouvrir.
type Request struct {
	// Title est le titre de la fenêtre.
	Title string
	// Dir demande un dossier plutôt qu'un fichier.
	Dir bool
	// Start est le dossier où s'ouvrir. Un chemin de fichier vaut son dossier.
	Start string
}

// outil est un sélecteur natif : de quoi le trouver, et de quoi le lancer.
type outil struct {
	nom       string
	graphique bool // exige une session graphique
	commande  func(Request, string) []string
}

// outils énumère les sélecteurs connus, du plus courant au plus rare pour la
// plateforme courante. L'ordre compte : le premier trouvé est celui qui sert.
func outils() []outil {
	switch runtime.GOOS {
	case "darwin":
		return []outil{{nom: "osascript", commande: osascript}}
	case "windows":
		return []outil{
			{nom: "powershell", commande: powershell},
			{nom: "pwsh", commande: powershell},
		}
	default:
		return []outil{
			{nom: "zenity", graphique: true, commande: zenity},
			{nom: "kdialog", graphique: true, commande: kdialog},
			{nom: "qarma", graphique: true, commande: zenity}, // clone de zenity
			{nom: "yad", graphique: true, commande: zenity},   // gabarit compatible
		}
	}
}

// disponible cherche le premier outil utilisable et renvoie son chemin.
func disponible() (outil, string, bool) {
	for _, candidat := range outils() {
		if candidat.graphique && !sessionGraphique() {
			continue
		}
		chemin, err := exec.LookPath(candidat.nom)
		if err != nil {
			continue
		}
		return candidat, chemin, true
	}
	return outil{}, "", false
}

// sessionGraphique dit si une fenêtre peut s'ouvrir. Sur un serveur atteint par
// SSH, aucune ne le peut : mieux vaut le savoir avant de lancer un outil qui
// resterait bloqué.
func sessionGraphique() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// Available dit si le système peut ouvrir une fenêtre de sélection.
func Available() bool {
	_, _, trouve := disponible()
	return trouve
}

// Name nomme le sélecteur qui servira, pour que l'interface puisse le dire.
func Name() string {
	choisi, _, trouve := disponible()
	if !trouve {
		return ""
	}
	return choisi.nom
}

// Pick ouvre la fenêtre et renvoie le chemin choisi.
func Pick(request Request) (string, error) {
	choisi, chemin, trouve := disponible()
	if !trouve {
		return "", ErrUnavailable
	}

	arguments := choisi.commande(request, depart(request))
	lifetime, cancel := context.WithTimeout(context.Background(), Attente)
	defer cancel()

	commande := exec.CommandContext(lifetime, chemin, arguments...)
	sortie, err := commande.Output()
	if err != nil {
		// Refermer la fenêtre sans choisir sort en erreur partout : c'est le
		// cas normal, pas une panne.
		var sorti *exec.ExitError
		if errors.As(err, &sorti) {
			return "", ErrCanceled
		}
		return "", err
	}

	choix := strings.TrimSpace(strings.SplitN(string(sortie), "\n", 2)[0])
	if choix == "" {
		return "", ErrCanceled
	}
	return filepath.Clean(choix), nil
}

// depart ramène le point de départ à un dossier existant : un chemin de fichier
// donne son dossier, et un chemin qui n'existe pas remonte jusqu'à ce qui
// existe. Sans cela, la fenêtre s'ouvre n'importe où.
func depart(request Request) string {
	candidat := strings.TrimSpace(request.Start)
	if candidat == "" {
		if maison, err := os.UserHomeDir(); err == nil {
			return maison
		}
		return "."
	}
	if !filepath.IsAbs(candidat) {
		if absolu, err := filepath.Abs(candidat); err == nil {
			candidat = absolu
		}
	}
	for {
		info, err := os.Stat(candidat)
		if err == nil {
			if info.IsDir() {
				return candidat
			}
			return filepath.Dir(candidat)
		}
		parent := filepath.Dir(candidat)
		if parent == candidat {
			if maison, err := os.UserHomeDir(); err == nil {
				return maison
			}
			return "."
		}
		candidat = parent
	}
}

// --------------------------------------------------------- une plateforme, un outil

func zenity(request Request, start string) []string {
	arguments := []string{"--file-selection", "--title=" + request.Title}
	if request.Dir {
		arguments = append(arguments, "--directory")
	}
	// zenity veut un chemin terminé par un séparateur pour comprendre « dans ce
	// dossier » plutôt que « ce fichier ».
	return append(arguments, "--filename="+start+string(filepath.Separator))
}

func kdialog(request Request, start string) []string {
	if request.Dir {
		return []string{"--getexistingdirectory", start, "--title", request.Title}
	}
	return []string{"--getopenfilename", start, "--title", request.Title}
}

func osascript(request Request, start string) []string {
	verbe := "choose file"
	if request.Dir {
		verbe = "choose folder"
	}
	script := "POSIX path of (" + verbe +
		" with prompt " + applescript(request.Title) +
		" default location POSIX file " + applescript(start) + ")"
	return []string{"-e", script}
}

// applescript met une chaîne entre guillemets à la façon d'AppleScript.
func applescript(valeur string) string {
	remplace := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + remplace.Replace(valeur) + `"`
}

func powershell(request Request, start string) []string {
	// La fenêtre .NET exige un fil « single-threaded apartment » ; sans -STA,
	// elle ne s'affiche pas.
	script := `Add-Type -AssemblyName System.Windows.Forms | Out-Null; `
	if request.Dir {
		script += `$f = New-Object System.Windows.Forms.FolderBrowserDialog; ` +
			`$f.Description = ` + posh(request.Title) + `; ` +
			`$f.SelectedPath = ` + posh(start) + `; ` +
			`if ($f.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) ` +
			`{ [Console]::Out.Write($f.SelectedPath) } else { exit 1 }`
	} else {
		script += `$f = New-Object System.Windows.Forms.OpenFileDialog; ` +
			`$f.Title = ` + posh(request.Title) + `; ` +
			`$f.InitialDirectory = ` + posh(start) + `; ` +
			`if ($f.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) ` +
			`{ [Console]::Out.Write($f.FileName) } else { exit 1 }`
	}
	return []string{"-NoProfile", "-STA", "-Command", script}
}

// posh met une chaîne entre apostrophes à la façon de PowerShell, où une
// apostrophe se double.
func posh(valeur string) string {
	return "'" + strings.ReplaceAll(valeur, "'", "''") + "'"
}
