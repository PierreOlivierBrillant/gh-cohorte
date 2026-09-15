package ghapi

import (
	"io"
	"net/http"
	"strings"
)

// Récupérer le contenu d'un dépôt pour l'analyser.
//
// Trois chemins existent, et l'archive est le seul qui convienne. Le clonage
// demande git, un dossier de travail et autant de fois l'historique qu'il y a
// de dépôts. Lire l'arbre puis chaque blob coûte une requête par fichier —
// quarante mille requêtes pour un groupe, ce qui épuiserait le quota avant la
// fin. L'archive coûte une requête par dépôt et ne laisse rien sur le disque.
//
// Elle s'épingle en outre à une référence : c'est ce qui permet d'analyser la
// remise telle qu'elle était à l'échéance, et non ce que le dépôt est devenu
// depuis.

// MaxArchiveBytes borne ce qu'on accepte de télécharger pour un seul dépôt. Un
// travail d'étudiant qui pèse davantage porte des dépendances ou des médias
// qu'on écarterait de toute façon ; les avaler d'abord pour les jeter ensuite
// coûterait la mémoire et le réseau pour rien.
const MaxArchiveBytes = 64 << 20

// Archive télécharge l'archive tar compressée d'un dépôt.
//
// Une référence vide prend la branche par défaut. GitHub répond par une
// redirection vers une adresse signée : le jeton n'a pas à la suivre, et Go ne
// le transmet d'ailleurs pas d'un hôte à l'autre.
func (c *Client) Archive(owner, repo, ref string) ([]byte, error) {
	path := repoPath(owner, repo) + "/tarball"
	if ref = strings.TrimSpace(ref); ref != "" {
		path += "/" + escapePath(ref)
	}

	response, err := c.client().Request(http.MethodGet, c.url(path), nil)
	if err != nil {
		c.rememberScopesFromError(err)
		return nil, convert(err)
	}
	defer response.Body.Close()

	// Un octet de plus que la borne : c'est ce qui permet de distinguer « le
	// fichier fait exactement la taille maximale » de « il la dépasse ».
	content, err := io.ReadAll(io.LimitReader(response.Body, MaxArchiveBytes+1))
	if err != nil {
		return nil, &Error{Status: response.StatusCode,
			Message: "Archive illisible : " + err.Error()}
	}
	if len(content) > MaxArchiveBytes {
		return nil, &Error{Status: response.StatusCode, Message: "Archive trop volumineuse."}
	}
	c.rememberScopes(response.Header)
	return content, nil
}
