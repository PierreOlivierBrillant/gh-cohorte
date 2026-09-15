package anonymize

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Le ZIP et la table de correspondance.
//
// Deux fichiers, et ils ne voyagent jamais ensemble. Le ZIP part chez le
// collègue ; la table reste ici. C'est tout ce qui sépare « comparer du code »
// de « transmettre une liste de noms », et cette séparation ne doit pas
// dépendre de la vigilance de qui envoie : la table n'est pas mise dans
// l'archive, il n'y a pas d'option pour l'y mettre, et le ZIP porte une note
// qui dit où elle est.

// ManifestFile est le manifeste d'un envoi anonymisé, à la racine du ZIP.
const ManifestFile = "cohorte.json"

// NoticeFile explique l'envoi à qui l'ouvre.
const NoticeFile = "LISEZ-MOI.md"

// Version est celle du schéma du manifeste.
const Version = 1

// MaxArchiveBytes borne ce qu'on accepte de lire d'un ZIP reçu.
const MaxArchiveBytes = 256 << 20

// Manifest décrit un envoi. Il ne porte aucun nom, et c'est le point.
type Manifest struct {
	Version int    `json:"version"`
	Tool    string `json:"tool,omitempty"`
	// Assignment nomme le travail. Il n'identifie personne, et sans lui le
	// destinataire ne saurait pas à quoi comparer.
	Assignment string `json:"assignment,omitempty"`
	CreatedAt  string `json:"created_at"`
	// Profile dit ce qui a été retenu à l'inspection : le destinataire doit
	// comparer les mêmes fichiers, sinon les mesures ne se rapportent à rien.
	Profile string `json:"profile,omitempty"`
	Works   []Work `json:"works"`
}

// Work est une copie de l'envoi, telle que le destinataire la voit.
type Work struct {
	Token string `json:"token"`
	// Origin étiquette la provenance sans nommer personne : « groupe A »,
	// « autre collège ». C'est elle qui colore les nuages de points chez le
	// destinataire, et qui lui permet de dire « ces deux-là ne viennent même
	// pas du même endroit ».
	Origin string `json:"origin,omitempty"`
	Files  int    `json:"files"`
}

// Copy est une copie, telle qu'on l'anonymise ou telle qu'on la reçoit.
type Copy struct {
	// Work est l'identifiant chez soi à l'export, le jeton à l'import.
	Work  string
	Files []File
}

// File est un fichier d'une copie.
type File struct {
	Path    string
	Content []byte
}

// Table est la correspondance entre les jetons et les personnes. Elle reste
// chez celui qui a produit l'envoi, et n'entre jamais dans le ZIP.
type Table struct {
	Version    int     `json:"version"`
	Assignment string  `json:"assignment,omitempty"`
	CreatedAt  string  `json:"created_at"`
	Tokens     []Token `json:"tokens"`
}

// CSV met la table en forme pour un tableur ou un dossier.
func (t Table) CSV() ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	records := [][]string{{
		"jeton", "depot", "nom_complet", "compte_github", "provenance", "remise",
	}}
	for _, token := range t.Tokens {
		records = append(records, []string{
			token.Base, token.Work, token.FullName, token.Username,
			token.Origin, token.HandedIn,
		})
	}
	if err := writer.WriteAll(records); err != nil {
		return nil, err
	}
	return buffer.Bytes(), writer.Error()
}

// Bundle est ce qu'un export produit.
type Bundle struct {
	// Zip est l'archive à transmettre.
	Zip []byte
	// Table reste ici. Elle est rendue à part du ZIP, et non dedans : il n'y a
	// pas de geste qui les réunisse par inadvertance.
	Table Table
	// Hits dit ce qui a été remplacé, Residues ce qui a résisté.
	Hits     []Hit     `json:"hits"`
	Residues []Residue `json:"residues"`
}

// Export anonymise des copies et en fait une archive.
func Export(anonymizer *Anonymizer, manifest Manifest, copies []Copy) (Bundle, error) {
	manifest.Version, manifest.CreatedAt = Version, time.Now().Format(time.RFC3339)
	bundle := Bundle{Table: Table{
		Version: Version, Assignment: manifest.Assignment,
		CreatedAt: manifest.CreatedAt, Tokens: anonymizer.Tokens(),
	}}

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	compte := map[Hit]int{}

	sort.Slice(copies, func(first, second int) bool {
		return copies[first].Work < copies[second].Work
	})
	for _, copie := range copies {
		token, connu := anonymizer.TokenOf(copie.Work)
		if !connu {
			return Bundle{}, valid.Errorf(
				"Anonymisation : la copie « %s » n'a pas de jeton.", copie.Work)
		}
		fichiers, hits, residues, err := writeCopy(archive, anonymizer, token, copie)
		if err != nil {
			return Bundle{}, err
		}
		for _, hit := range hits {
			compte[Hit{What: hit.What, Work: hit.Work}] += hit.Count
		}
		bundle.Residues = append(bundle.Residues, residues...)
		manifest.Works = append(manifest.Works, Work{
			Token: token.Base, Origin: token.Origin, Files: fichiers,
		})
	}

	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Bundle{}, err
	}
	if err := add(archive, ManifestFile, append(payload, '\n')); err != nil {
		return Bundle{}, err
	}
	if err := add(archive, NoticeFile, notice(manifest)); err != nil {
		return Bundle{}, err
	}
	if err := archive.Close(); err != nil {
		return Bundle{}, err
	}

	bundle.Zip = buffer.Bytes()
	for hit, count := range compte {
		hit.Count = count
		bundle.Hits = append(bundle.Hits, hit)
	}
	sort.Slice(bundle.Hits, func(first, second int) bool {
		if bundle.Hits[first].Work != bundle.Hits[second].Work {
			return bundle.Hits[first].Work < bundle.Hits[second].Work
		}
		return bundle.Hits[first].What < bundle.Hits[second].What
	})
	sort.Slice(bundle.Residues, func(first, second int) bool {
		if bundle.Residues[first].Path != bundle.Residues[second].Path {
			return bundle.Residues[first].Path < bundle.Residues[second].Path
		}
		return bundle.Residues[first].Line < bundle.Residues[second].Line
	})
	return bundle, nil
}

// writeCopy anonymise une copie et l'écrit dans l'archive.
func writeCopy(archive *zip.Writer, anonymizer *Anonymizer, token Token,
	copie Copy) (int, []Hit, []Residue, error) {

	hits := make([]Hit, 0, 4)
	residues := make([]Residue, 0, 4)
	ecrits := 0
	for _, fichier := range copie.Files {
		// Le chemin aussi porte des noms : un dossier « rendu-emilie-cote » ou
		// un paquet « ca.acme.ecote » nommerait son auteur aussi sûrement
		// qu'une ligne de code.
		chemin, cheminHits := anonymizer.ScrubPath(fichier.Path)
		contenu, contenuHits := anonymizer.Scrub(string(fichier.Content))
		hits = append(hits, cheminHits...)
		hits = append(hits, contenuHits...)

		for _, residue := range anonymizer.Inspect(chemin, contenu) {
			residue.Path = path.Join(token.Base, residue.Path)
			residues = append(residues, residue)
		}
		if err := add(archive, path.Join(token.Base, chemin), []byte(contenu)); err != nil {
			return 0, nil, nil, err
		}
		ecrits++
	}
	return ecrits, hits, residues, nil
}

func add(archive *zip.Writer, name string, content []byte) error {
	entry, err := archive.Create(name)
	if err != nil {
		return valid.Errorf("Archive : %v.", err)
	}
	if _, err := entry.Write(content); err != nil {
		return valid.Errorf("Archive : %v.", err)
	}
	return nil
}

// notice explique l'envoi à qui l'ouvre, et dit ce qui n'y est pas.
func notice(manifest Manifest) []byte {
	var texte strings.Builder
	texte.WriteString("# Copies anonymisées\n\n")
	texte.WriteString("Ces copies viennent de `gh cohorte`. Les noms, les comptes " +
		"GitHub et les matricules y ont été remplacés par des jetons de longueur " +
		"égale : `A7F3K2`, et `A7F3K2A7` là où le nom d'origine était plus long.\n\n")
	if manifest.Assignment != "" {
		texte.WriteString("Travail : `" + manifest.Assignment + "`.\n\n")
	}
	texte.WriteString("**La table de correspondance n'est pas ici.** Elle est " +
		"restée chez qui vous a envoyé ces copies : c'est à cette personne " +
		"qu'il faut demander qui se cache derrière un jeton, et elle seule " +
		"peut le dire.\n\n")
	texte.WriteString("L'anonymisation n'est jamais complète : un prénom dans un " +
		"commentaire, un chemin absolu ou une capture d'écran peuvent avoir " +
		"survécu. Si vous en voyez, dites-le à l'expéditeur.\n\n")
	texte.WriteString("`cohorte.json` décrit l'envoi — un jeton par copie, et " +
		"d'où chacune vient. Il ne nomme personne.\n")
	return []byte(texte.String())
}

// ------------------------------------------------------------------ import

// Imported est ce qu'un ZIP reçu contient.
type Imported struct {
	Manifest Manifest
	Copies   []Copy
	// Warnings dit ce qui a dû être deviné ou écarté.
	Warnings []string
}

// Import lit une archive de copies anonymisées.
//
// Le manifeste n'est pas exigé : un collègue qui n'utilise pas l'outil enverra
// un ZIP ordinaire, un dossier par copie. Le refuser reviendrait à n'accepter
// que ce que l'outil a lui-même produit, c'est-à-dire à ne servir à rien pour
// le seul cas où l'on en a besoin.
func Import(archive []byte) (Imported, error) {
	if len(archive) > MaxArchiveBytes {
		return Imported{}, valid.Errorf(
			"Archive : elle dépasse %d Mo.", MaxArchiveBytes>>20)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return Imported{}, valid.Errorf("Archive illisible : %v.", err)
	}

	imported := Imported{}
	parCopie := map[string]*Copy{}
	ordre := make([]string, 0, 16)
	total := 0

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		nom := cleanPath(entry.Name)
		if nom == "" {
			imported.Warnings = append(imported.Warnings,
				"Un chemin sortant de l'archive a été écarté : "+entry.Name)
			continue
		}
		contenu, err := read(entry)
		if err != nil {
			return Imported{}, err
		}
		if total += len(contenu); total > MaxArchiveBytes {
			return Imported{}, valid.Errorf(
				"Archive : son contenu dépasse %d Mo une fois décompressé.",
				MaxArchiveBytes>>20)
		}

		if nom == ManifestFile {
			if err := json.Unmarshal(contenu, &imported.Manifest); err != nil {
				imported.Warnings = append(imported.Warnings,
					"Manifeste illisible : les provenances seront inconnues.")
			}
			continue
		}
		if nom == NoticeFile {
			continue
		}

		jeton, reste, dedans := strings.Cut(nom, "/")
		if !dedans {
			imported.Warnings = append(imported.Warnings,
				"Fichier posé à la racine de l'archive, hors de toute copie : "+nom)
			continue
		}
		if parCopie[jeton] == nil {
			parCopie[jeton] = &Copy{Work: jeton}
			ordre = append(ordre, jeton)
		}
		parCopie[jeton].Files = append(parCopie[jeton].Files,
			File{Path: reste, Content: contenu})
	}

	if imported.Manifest.Version > Version {
		return Imported{}, valid.Errorf(
			"Archive : son manifeste vient d'une version %d de l'outil, qui n'en "+
				"connaît que %d. Mettez l'extension à jour.",
			imported.Manifest.Version, Version)
	}
	sort.Strings(ordre)
	for _, jeton := range ordre {
		imported.Copies = append(imported.Copies, *parCopie[jeton])
	}
	if len(imported.Copies) == 0 {
		return Imported{}, valid.Errorf(
			"Archive : aucune copie trouvée. Elle doit porter un dossier par copie.")
	}
	if imported.Manifest.Version == 0 {
		imported.Warnings = append(imported.Warnings,
			"Aucun manifeste : les copies ont été reconnues à leurs dossiers, et "+
				"leur provenance reste à donner.")
	}
	return imported, nil
}

// OriginOf rend la provenance d'un jeton, telle que le manifeste la donne.
func (i Imported) OriginOf(token string) string {
	for _, work := range i.Manifest.Works {
		if work.Token == token {
			return work.Origin
		}
	}
	return ""
}

func read(entry *zip.File) ([]byte, error) {
	file, err := entry.Open()
	if err != nil {
		return nil, valid.Errorf("Archive : « %s » illisible (%v).", entry.Name, err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, MaxArchiveBytes))
	if err != nil {
		return nil, valid.Errorf("Archive : « %s » illisible (%v).", entry.Name, err)
	}
	return content, nil
}

// cleanPath met un chemin d'archive en forme et refuse ce qui en sort.
func cleanPath(name string) string {
	name = path.Clean(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	name = strings.TrimPrefix(name, "./")
	if name == "." || name == "/" || strings.HasPrefix(name, "../") ||
		strings.HasPrefix(name, "/") {
		return ""
	}
	return name
}
