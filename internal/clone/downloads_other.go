//go:build !windows

package clone

import "errors"

// knownDownloads n'a d'équivalent qu'à Windows, le seul système où le dossier
// des téléchargements se déplace hors de l'accueil sans que rien ne le dise
// dans l'environnement. golang.org/x/sys/windows ne compile pas ailleurs : d'où
// ce fichier, là où un switch sur runtime.GOOS aurait suffi autrement.
func knownDownloads() (string, error) {
	return "", errors.New("aucun dossier connu hors de Windows")
}
