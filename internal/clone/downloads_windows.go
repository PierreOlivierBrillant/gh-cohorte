//go:build windows

package clone

import "golang.org/x/sys/windows"

// knownDownloads demande au shell de Windows où est vraiment le dossier des
// téléchargements. L'appel passe par shell32, sans commande externe : reg.exe
// ne serait pas garanti partout.
func knownDownloads() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Downloads, windows.KF_FLAG_DEFAULT)
}
