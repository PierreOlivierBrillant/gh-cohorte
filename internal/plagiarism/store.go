package plagiarism

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Un rapport est un fichier, et c'est ce qui rend tout le reste possible : le
// rouvrir sans refaire l'analyse, déplacer le seuil sans retélécharger trois
// cents dépôts, le produire dans une GitHub Action et le lire ici.
//
// Deux formats, pour deux usages. Le JSON porte tout — l'index, les fragments,
// les motifs d'écartement — et c'est lui que l'interface relit. Le CSV ne porte
// que les paires, une par ligne, et c'est lui qu'on ouvre dans un tableur ou
// qu'on joint à un dossier. Les données d'étudiants n'ont rien à faire dans le
// dépôt : le dossier des bilans est déjà exclu par le « .gitignore ».

// Dir est le sous-dossier des rapports d'analyse, sous le dossier des bilans.
const Dir = "plagiat"

// Save écrit le rapport en JSON et en CSV, et rend les deux chemins.
func (r *Report) Save(directory string) (string, string, error) {
	directory = filepath.Join(directory, Dir)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", err
	}
	base := filepath.Join(directory, r.Basename())

	payload, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", "", err
	}
	jsonPath := base + ".json"
	if err := os.WriteFile(jsonPath, append(payload, '\n'), 0o600); err != nil {
		return "", "", err
	}

	csvPath := base + ".csv"
	if err := r.saveCSV(csvPath); err != nil {
		return "", "", err
	}
	return jsonPath, csvPath, nil
}

// Basename compose le nom des fichiers d'un rapport : le travail analysé et le
// moment de l'analyse, pour que deux analyses du même travail se distinguent.
func (r *Report) Basename() string {
	stamp := strings.NewReplacer(":", "", "-", "").Replace(r.CreatedAt)
	if len(stamp) > 15 {
		stamp = stamp[:15]
	}
	base := r.Request.Assignment
	if base == "" {
		base = "plagiat"
	}
	return base + "-" + stamp
}

func (r *Report) saveCSV(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	records := [][]string{{
		"copie_a", "copie_b", "groupe_a", "groupe_b", "similarite",
		"couverture_a", "couverture_b", "plus_long_fragment", "empreintes_communes",
		"au_dessus_du_seuil",
	}}
	for _, match := range r.Result.Matches {
		records = append(records, []string{
			match.Left, match.Right, match.LeftOrigin, match.RightOrigin,
			ratio(match.Similarity), ratio(match.LeftCoverage), ratio(match.RightCoverage),
			strconv.Itoa(match.LongestFragment), strconv.Itoa(match.Shared),
			oui(r.Result.Threshold > 0 && match.Similarity >= r.Result.Threshold),
		})
	}
	if err := writer.WriteAll(records); err != nil {
		return err
	}
	return writer.Error()
}

// ratio écrit une part avec deux décimales, le point en séparateur : c'est ce
// qu'un tableur relit sans se tromper de colonne, quelle que soit sa langue.
func ratio(value float64) string { return fmt.Sprintf("%.3f", value) }

func oui(yes bool) string {
	if yes {
		return "oui"
	}
	return "non"
}

// Load relit un rapport écrit.
func Load(path string) (*Report, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, valid.Errorf("Rapport « %s » illisible : %v.", path, err)
	}
	report := &Report{}
	if err := json.Unmarshal(content, report); err != nil {
		return nil, valid.Errorf("Rapport « %s » incompréhensible : %v.", path, err)
	}
	if report.Version > Version {
		return nil, valid.Errorf(
			"Rapport « %s » : il vient d'une version %d de l'outil, qui n'en connaît "+
				"que %d. Mettez l'extension à jour (gh extension upgrade cohorte).",
			path, report.Version, Version)
	}
	if report.Version == 0 {
		return nil, valid.Errorf(
			"Rapport « %s » : ce n'est pas un rapport d'analyse de plagiat.", path)
	}
	return report, nil
}

// List énumère les rapports d'un dossier, du plus récent au plus ancien.
func List(directory string) []string {
	entries, err := os.ReadDir(filepath.Join(directory, Dir))
	if err != nil {
		return nil
	}
	found := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		found = append(found, filepath.Join(directory, Dir, entry.Name()))
	}
	// Le nom porte l'horodatage en queue : trier dessus range les rapports du
	// plus récent au plus ancien sans avoir à en ouvrir un seul, et sans que
	// le nom du travail ne vienne brouiller l'ordre.
	sort.SliceStable(found, func(first, second int) bool {
		return stampOf(found[first]) > stampOf(found[second])
	})
	return found
}

// stampOf extrait l'horodatage du nom d'un rapport.
func stampOf(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".json")
	if index := strings.LastIndex(name, "-"); index >= 0 {
		return name[index+1:]
	}
	return name
}
