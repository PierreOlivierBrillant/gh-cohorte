package fakegh

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"sort"
	"strings"
)

// L'archive d'un dépôt, telle que GitHub la rend.
//
// Le faux serveur la fabrique vraiment — un vrai tar, vraiment compressé, avec
// le préfixe « organisation-depot-sha » que GitHub ajoute. C'est ce préfixe qui
// oblige l'inspection à deviner la racine d'un projet, et une archive de test
// qui ne l'aurait pas laisserait ce travail sans épreuve.

// tarball rend l'archive d'un dépôt à une référence donnée. L'appelant tient
// déjà le verrou de l'état.
func (s *Server) tarball(writer http.ResponseWriter, fullName, ref string) {
	state := s.State
	repo, found := state.Repos[fullName]
	if !found {
		s.notFound(writer)
		return
	}

	commit, known := state.resolveLocked(fullName, ref, repo.DefaultBranch)
	if !known {
		// Un dépôt sans commit n'a pas d'archive : GitHub répond 404, et
		// l'analyse doit savoir dire « dépôt vide » plutôt que « panne ».
		s.notFound(writer)
		return
	}

	files := map[string]string{}
	for path, entry := range state.Trees[state.Commits[commit].Tree] {
		files[path] = string(state.Blobs[entry.Blob])
	}

	prefix := strings.ReplaceAll(fullName, "/", "-") + "-" + short(commit)
	archive, err := Tarball(prefix, files)
	if err != nil {
		s.send(writer, 500, map[string]string{"message": err.Error()})
		return
	}
	writer.Header().Set("Content-Type", "application/x-gzip")
	writer.WriteHeader(http.StatusOK)
	writer.Write(archive)
}

// Tarball fabrique une archive tar compressée, comme celle d'un dépôt.
func Tarball(prefix string, files map[string]string) ([]byte, error) {
	var buffer bytes.Buffer
	compressor := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(compressor)

	// Un ordre stable rend deux archives d'un même contenu identiques : sans
	// cela, une épreuve qui compare deux archives serait capricieuse.
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		content := files[name]
		header := &tar.Header{
			Name: prefix + "/" + name, Mode: 0o644,
			Size: int64(len(content)), Typeflag: tar.TypeReg,
		}
		if err := archive.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := archive.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	if err := compressor.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// resolveLocked traduit une référence en commit : un nom de branche, ou un SHA
// même abrégé.
//
// GitHub accepte les deux, et l'outil s'en sert vraiment : l'analyse note le
// SHA abrégé que porte le préfixe de l'archive, puis le redemande pour rouvrir
// une paire sur exactement le même état. Un faux serveur qui n'accepterait que
// les branches laisserait ce chemin sans épreuve.
func (s *State) resolveLocked(fullName, ref, fallback string) (string, bool) {
	if ref == "" {
		ref = fallback
	}
	if commit, known := s.Refs[fullName+"@"+ref]; known {
		return commit, true
	}
	for commit := range s.Commits {
		if strings.HasPrefix(commit, ref) && reachable(s, fullName, commit) {
			return commit, true
		}
	}
	return "", false
}

// reachable dit qu'un commit appartient bien à ce dépôt : deux dépôts peuvent
// porter le même contenu, donc le même SHA, et rendre l'archive de l'un pour
// l'autre masquerait une confusion au lieu de la révéler.
func reachable(state *State, fullName, commit string) bool {
	for key, head := range state.Refs {
		if head == commit && strings.HasPrefix(key, fullName+"@") {
			return true
		}
	}
	return false
}
