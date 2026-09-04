// Package roster charge et valide la liste des personnes (nom complet + compte GitHub).
package roster

import (
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// En-têtes reconnus, en français comme en anglais.
var nameHeaders = map[string]bool{
	"nom": true, "nom complet": true, "nom_complet": true, "name": true,
	"full_name": true, "fullname": true, "etudiant": true, "étudiant": true,
}

var loginHeaders = map[string]bool{
	"github": true, "github_username": true, "username": true, "login": true,
	"compte": true, "utilisateur": true, "handle": true,
}

// Séparateurs de colonnes reconnus.
var delimiters = []rune{',', ';', '\t'}

// Person est une personne de la cohorte.
type Person struct {
	FullName string `json:"full_name"`
	Username string `json:"username"`
}

// Key sert au dédoublonnage : le compte GitHub est insensible à la casse.
func (p Person) Key() string { return strings.ToLower(p.Username) }

// String affiche la personne de façon lisible.
func (p Person) String() string { return p.FullName + " <@" + p.Username + ">" }

// Issue décrit un problème détecté sur une ligne de la liste.
type Issue struct {
	Line    int    `json:"line"`
	Raw     string `json:"raw"`
	Message string `json:"message"`
}

// Roster est le résultat d'un chargement : les personnes valides et les lignes rejetées.
type Roster struct {
	People []Person
	Issues []Issue
}

// IsValid indique une liste exploitable sans aucun rejet.
func (r Roster) IsValid() bool { return len(r.Issues) == 0 && len(r.People) > 0 }

// sniffDelimiter devine le séparateur de colonnes, avec la virgule comme repli.
func sniffDelimiter(text string) rune {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		best, bestCount := ',', 0
		for _, candidate := range delimiters {
			if count := strings.Count(line, string(candidate)); count > bestCount {
				best, bestCount = candidate, count
			}
		}
		if bestCount > 0 {
			return best
		}
		return ','
	}
	return ','
}

func normalizeHeader(value string) string {
	cleaned := strings.TrimPrefix(value, "\ufeff")
	return strings.ToLower(strings.TrimSpace(cleaned))
}

// detectColumns repère les colonnes « nom » et « github » dans une ligne d'en-tête.
func detectColumns(row []string) (nameIndex, loginIndex int, found bool) {
	nameIndex, loginIndex = -1, -1
	for index, cell := range row {
		header := normalizeHeader(cell)
		if nameIndex < 0 && nameHeaders[header] {
			nameIndex = index
		}
		if loginIndex < 0 && loginHeaders[header] {
			loginIndex = index
		}
	}
	if nameIndex < 0 || loginIndex < 0 {
		return 0, 1, false
	}
	return nameIndex, loginIndex, true
}

// Parse analyse un contenu CSV/TSV et renvoie les personnes valides et les erreurs.
func Parse(text string) Roster {
	text = strings.TrimPrefix(text, "\ufeff")
	if strings.TrimSpace(text) == "" {
		return Roster{Issues: []Issue{{Message: "Le fichier est vide."}}}
	}

	delimiter := sniffDelimiter(text)
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1 // les lignes courtes sont signalées, pas rejetées par le lecteur
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	var rows [][]string
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Une ligne illisible ne doit pas interrompre le reste du fichier.
			rows = append(rows, nil)
			continue
		}
		rows = append(rows, row)
	}

	result := Roster{}
	seen := map[string]Person{}

	nameIndex, loginIndex := 0, 1
	start := 0
	if len(rows) > 0 {
		if name, login, found := detectColumns(rows[0]); found {
			nameIndex, loginIndex, start = name, login, 1
		}
	}

	for offset := start; offset < len(rows); offset++ {
		row := rows[offset]
		line := offset + 1
		raw := strings.Join(row, string(delimiter))

		cells := make([]string, len(row))
		empty := true
		for index, cell := range row {
			cells[index] = strings.TrimSpace(cell)
			if cells[index] != "" {
				empty = false
			}
		}
		if empty || strings.HasPrefix(cells[0], "#") {
			continue // ligne vide ou commentaire
		}
		if len(cells) <= max(nameIndex, loginIndex) {
			result.Issues = append(result.Issues, Issue{line, raw,
				"Deux colonnes attendues : nom complet et compte GitHub."})
			continue
		}

		fullName, err := valid.FullName(cells[nameIndex])
		if err != nil {
			result.Issues = append(result.Issues, Issue{line, raw, err.Error()})
			continue
		}
		username, err := valid.Login(cells[loginIndex], "")
		if err != nil {
			result.Issues = append(result.Issues, Issue{line, raw, err.Error()})
			continue
		}

		person := Person{FullName: fullName, Username: username}
		if previous, exists := seen[person.Key()]; exists {
			result.Issues = append(result.Issues, Issue{line, raw,
				"Compte « " + person.Username + " » déjà présent pour « " + previous.FullName + " »."})
			continue
		}
		seen[person.Key()] = person
		result.People = append(result.People, person)
	}

	if len(result.People) == 0 && len(result.Issues) == 0 {
		result.Issues = append(result.Issues, Issue{Message: "Aucune personne trouvée dans le fichier."})
	}
	return result
}

// Load charge une liste depuis un fichier CSV/TSV.
func Load(path string) (Roster, error) {
	expanded, err := ExpandPath(path)
	if err != nil {
		return Roster{}, err
	}
	info, err := os.Stat(expanded)
	if err != nil || info.IsDir() {
		return Roster{}, valid.Errorf("Fichier introuvable : %s", expanded)
	}
	content, err := os.ReadFile(expanded)
	if err != nil {
		return Roster{}, valid.Errorf("Fichier illisible : %v", err)
	}
	if !isUTF8(content) {
		return Roster{}, valid.Errorf("Fichier illisible : encodage UTF-8 attendu (%s).", expanded)
	}
	return Parse(string(content)), nil
}

// Write écrit une liste de personnes au format CSV.
func Write(path string, people []Person) (string, error) {
	expanded, err := ExpandPath(path)
	if err != nil {
		return "", err
	}
	if parent := filepath.Dir(expanded); parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return "", valid.Errorf("Enregistrement impossible : %v", err)
		}
	}
	file, err := os.Create(expanded)
	if err != nil {
		return "", valid.Errorf("Enregistrement impossible : %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	records := [][]string{{"nom_complet", "github_username"}}
	for _, person := range people {
		records = append(records, []string{person.FullName, person.Username})
	}
	if err := writer.WriteAll(records); err != nil {
		return "", valid.Errorf("Enregistrement impossible : %v", err)
	}
	return expanded, nil
}

// ExpandPath développe « ~ » et rend le chemin utilisable tel quel.
func ExpandPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", valid.Errorf("Chemin : la valeur est vide.")
	}
	if trimmed == "~" || strings.HasPrefix(trimmed, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", valid.Errorf("Chemin : dossier personnel introuvable (%v).", err)
		}
		trimmed = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(trimmed, "~"), "/"))
	}
	return trimmed, nil
}

// isUTF8 vérifie que le contenu est bien de l'UTF-8.
func isUTF8(content []byte) bool {
	return strings.ToValidUTF8(string(content), "�") == string(content)
}
