package corpus

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// MaxUnpacked borne ce qu'une archive a le droit de rendre une fois
// décompressée.
//
// Une archive de soixante méga-octets peut en rendre des milliers : c'est la
// « bombe de décompression », et elle n'a même pas besoin d'être malveillante
// — un dossier de captures d'écran suffit. Rien ici ne justifie de dépasser
// cette borne, et la dépasser ferait tomber le processus entier au lieu d'un
// seul dépôt.
const MaxUnpacked = 256 << 20

// Untar dépaquette l'archive d'un dépôt.
//
// Le préfixe rendu est celui que GitHub ajoute — « organisation-depot-sha ». Il
// porte le commit archivé, et c'est la seule façon gratuite de le connaître :
// le noter permet de retélécharger exactement la même chose plus tard, quand
// une paire est ouverte, sans risquer d'analyser un dépôt qui a bougé entre
// les deux.
func Untar(archive []byte) (sources []inspect.Source, prefix string, err error) {
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, "", valid.Errorf("Archive illisible : %v.", err)
	}
	defer compressed.Close()

	reader := tar.NewReader(compressed)
	total := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", valid.Errorf("Archive illisible : %v.", err)
		}
		// Seuls les fichiers ordinaires comptent : un lien symbolique ne porte
		// pas de code, et un dossier n'en porte pas davantage.
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := clean(header.Name)
		if name == "" {
			continue
		}
		if prefix == "" {
			prefix, _, _ = strings.Cut(name, "/")
		}

		if total += int(header.Size); total > MaxUnpacked {
			return nil, prefix, valid.Errorf(
				"Archive trop volumineuse une fois décompressée (plus de %d Mo).",
				MaxUnpacked>>20)
		}
		content, err := io.ReadAll(io.LimitReader(reader, header.Size))
		if err != nil {
			return nil, prefix, valid.Errorf("Archive illisible : %v.", err)
		}
		sources = append(sources, inspect.Source{Path: name, Content: content})
	}
	return sources, prefix, nil
}

// clean met un chemin d'archive en forme et refuse ce qui sort de l'archive.
//
// Rien n'est écrit sur le disque ici, donc « ../.. » ne peut rien casser ; mais
// un chemin qui remonte ne veut rien dire non plus, et le garder ferait
// apparaître des fichiers fantômes dans les rapports.
func clean(name string) string {
	name = path.Clean(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	name = strings.TrimPrefix(name, "./")
	if name == "." || name == "/" || strings.HasPrefix(name, "../") ||
		strings.HasPrefix(name, "/") {
		return ""
	}
	return name
}

// Commit rend le commit qu'une archive portait, tiré de son préfixe.
// « acme-a26.5n6.01.tp1.alice-3f9c2ab » donne « 3f9c2ab ».
func Commit(prefix string) string {
	if index := strings.LastIndex(prefix, "-"); index >= 0 {
		return prefix[index+1:]
	}
	return ""
}

// Packaging rend l'emballage qu'un dépôt met autour de son projet, sans le
// préfixe que toute archive de GitHub ajoute.
//
// « acme-a26.5n6.01.tp1.alice-3f9c2ab » n'apprend rien à personne : chaque
// archive en porte un. « tp1/mon_tp », en revanche, explique à lui seul
// pourquoi un profil qui parle de « app/ » trouve quelque chose ici et rien
// ailleurs. Seul le second mérite d'être montré.
func Packaging(root string) string {
	_, rest, nested := strings.Cut(root, "/")
	if !nested {
		return ""
	}
	return rest
}
