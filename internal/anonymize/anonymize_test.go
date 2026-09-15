package anonymize_test

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

func identites() []anonymize.Identity {
	return []anonymize.Identity{
		{
			Work: "a26.5n6.01.tp1.emilie-cote", Origin: "groupe 01",
			HandedIn: "2026-10-01",
			Person: roster.Person{
				FullName: "Émilie Côté", Username: "ecote", StudentID: "2100123",
			},
		},
		{
			Work: "a26.5n6.01.tp1.jean-luc-picard", Origin: "groupe 01",
			Person: roster.Person{FullName: "Jean-Luc Picard", Username: "jlpicard"},
		},
	}
}

func anonymiseur(t *testing.T, options anonymize.Options) *anonymize.Anonymizer {
	t.Helper()
	if options.Seed == 0 {
		options.Seed = 42 // une épreuve doit donner deux fois le même résultat
	}
	anonymizer, err := anonymize.New(identites(), options)
	if err != nil {
		t.Fatalf("anonymiseur : %v", err)
	}
	return anonymizer
}

func jeton(t *testing.T, anonymizer *anonymize.Anonymizer, work string) string {
	t.Helper()
	token, connu := anonymizer.TokenOf(work)
	if !connu {
		t.Fatalf("aucun jeton pour « %s »", work)
	}
	return token.Base
}

// C'est la règle dont tout le reste dépend : un remplacement plus court ou plus
// long déplacerait ce qui suit, et les fragments communs ne tomberaient plus au
// même endroit des deux côtés.
func TestLeRemplacementConserveLaLongueur(t *testing.T) {
	anonymizer := anonymiseur(t, anonymize.Options{})
	sources := []string{
		"// Travail remis par Émilie Côté\nclass Solution { }\n",
		"package ca.acme.ecote;\n// voir aussi Jean-Luc Picard\n",
		"const auteur = \"emilie.cote\"; // matricule 2100123\n",
		"Rien de personnel ici : un accent é, une ligne ordinaire.\n",
	}
	for _, source := range sources {
		anonymise, _ := anonymizer.Scrub(source)
		if len([]rune(anonymise)) != len([]rune(source)) {
			t.Fatalf("longueur changée :\n%q\n%q", source, anonymise)
		}
		// Les lignes aussi : c'est ce qui garde les colonnes là où elles étaient.
		if strings.Count(anonymise, "\n") != strings.Count(source, "\n") {
			t.Fatalf("lignes changées : %q", anonymise)
		}
	}
}

func TestLesVariantesDUnNomSontToutesTrouvees(t *testing.T) {
	anonymizer := anonymiseur(t, anonymize.Options{})
	base := jeton(t, anonymizer, "a26.5n6.01.tp1.emilie-cote")

	variantes := []string{
		"Émilie Côté", "Emilie Cote", "EMILIE COTE", "Côté Émilie",
		"emilie-cote", "emilie.cote", "emilie_cote", "EmilieCote",
		"ecote", "2100123", "a26.5n6.01.tp1.emilie-cote",
	}
	for _, variante := range variantes {
		anonymise, hits := anonymizer.Scrub("avant " + variante + " après")
		if strings.Contains(strings.ToLower(anonymise), "cote") ||
			strings.Contains(anonymise, "2100123") {
			t.Fatalf("« %s » a survécu : %q", variante, anonymise)
		}
		if !strings.Contains(anonymise, base[:3]) {
			t.Fatalf("« %s » : aucun jeton posé (%q)", variante, anonymise)
		}
		if len(hits) == 0 {
			t.Fatalf("« %s » : remplacement non rapporté", variante)
		}
	}
}

// La chaîne la plus longue passe d'abord : sans cela « emilie » serait remplacé
// dans « emilie-cote », et « -cote » survivrait au milieu d'un jeton.
func TestLaPlusLongueOccurrenceGagne(t *testing.T) {
	anonymizer := anonymiseur(t, anonymize.Options{Parts: true})
	anonymise, _ := anonymizer.Scrub("le dossier emilie-cote de Émilie Côté")
	if strings.Contains(strings.ToLower(anonymise), "cote") ||
		strings.Contains(strings.ToLower(anonymise), "emilie") {
		t.Fatalf("un fragment a survécu : %q", anonymise)
	}
}

// Un travail qui nomme un camarade doit voir ce nom remplacé lui aussi, et par
// le jeton de ce camarade : c'est ce qui rend la collusion visible au lieu de
// l'effacer.
func TestUnCamaradeNommeRecoitSonPropreJeton(t *testing.T) {
	anonymizer := anonymiseur(t, anonymize.Options{})
	sien := jeton(t, anonymizer, "a26.5n6.01.tp1.emilie-cote")
	autre := jeton(t, anonymizer, "a26.5n6.01.tp1.jean-luc-picard")
	if sien == autre {
		t.Fatal("deux copies ne peuvent pas porter le même jeton")
	}

	anonymise, hits := anonymizer.Scrub("// merci à Jean-Luc Picard pour l'aide\n")
	if !strings.Contains(anonymise, autre[:3]) {
		t.Fatalf("le camarade n'a pas reçu son jeton : %q", anonymise)
	}
	trouve := false
	for _, hit := range hits {
		if hit.Work == "a26.5n6.01.tp1.jean-luc-picard" {
			trouve = true
		}
	}
	if !trouve {
		t.Fatalf("la mention du camarade doit être rapportée : %+v", hits)
	}
}

// Un dossier « rendu-emilie-cote » ou un paquet « ca.acme.ecote » nommerait son
// auteur aussi sûrement qu'une ligne de code.
func TestLesCheminsSontAnonymisesAussi(t *testing.T) {
	anonymizer := anonymiseur(t, anonymize.Options{})
	chemin, hits := anonymizer.ScrubPath("rendu-emilie-cote/src/ca/acme/ecote/Main.java")
	if strings.Contains(strings.ToLower(chemin), "cote") {
		t.Fatalf("chemin anonymisé : %q", chemin)
	}
	if !strings.HasSuffix(chemin, "/Main.java") || len(hits) == 0 {
		t.Fatalf("chemin abîmé : %q (%+v)", chemin, hits)
	}
}

func TestUneGraineDonneDeuxFoisLeMemeResultat(t *testing.T) {
	premier := anonymiseur(t, anonymize.Options{Seed: 7})
	second := anonymiseur(t, anonymize.Options{Seed: 7})
	if jeton(t, premier, "a26.5n6.01.tp1.emilie-cote") !=
		jeton(t, second, "a26.5n6.01.tp1.emilie-cote") {
		t.Fatal("deux tirages de même graine doivent coïncider")
	}
	autre := anonymiseur(t, anonymize.Options{Seed: 8})
	if jeton(t, premier, "a26.5n6.01.tp1.emilie-cote") ==
		jeton(t, autre, "a26.5n6.01.tp1.emilie-cote") {
		t.Fatal("deux graines différentes ne devraient pas coïncider")
	}
}

// ------------------------------------------------------------- le contrôle

// Une anonymisation n'est jamais complète. Ce qu'on n'a pas su effacer doit se
// voir avant que le ZIP ne parte, et non après.
func TestLeControleMontreCeQuiARésisté(t *testing.T) {
	anonymizer := anonymiseur(t, anonymize.Options{})
	source := strings.Join([]string{
		"// @author Émilie",
		"// contact : quelquun@cegep.qc.ca",
		"// dépôt : https://github.com/uneautrepersonne/travail",
		"const chemin = \"/home/mlaflamme/tp1\";",
		"// le côté gauche de l'écran",
		"class Solution { }",
	}, "\n")

	anonymise, _ := anonymizer.Scrub(source)
	residues := anonymizer.Inspect("src/Solution.java", anonymise)

	attendus := map[string]bool{
		anonymize.ResidueAuthor: false, anonymize.ResidueMail: false,
		anonymize.ResidueGitHub: false, anonymize.ResidueHome: false,
		anonymize.ResidueName: false,
	}
	for _, residue := range residues {
		attendus[residue.Kind] = true
		if residue.Path != "src/Solution.java" || residue.Line < 1 {
			t.Fatalf("résidu mal situé : %+v", residue)
		}
	}
	for kind, vu := range attendus {
		if !vu {
			t.Fatalf("« %s » n'a pas été signalé : %+v", kind, residues)
		}
	}
	// « Côté » est un nom de famille et un mot français : il n'est pas remplacé
	// d'office, mais il est montré.
	if strings.Count(anonymise, "côté") != 1 {
		t.Fatalf("le mot ordinaire ne doit pas être abîmé : %q", anonymise)
	}
	if compte := anonymize.Summary(residues); compte[anonymize.ResidueMail] != 1 {
		t.Fatalf("résumé : %+v", compte)
	}
}

// ------------------------------------------------------------- l'archive

func copies() []anonymize.Copy {
	return []anonymize.Copy{
		{Work: "a26.5n6.01.tp1.emilie-cote", Files: []anonymize.File{
			{Path: "src/Solution.java", Content: []byte("// Émilie Côté\nclass Solution { }\n")},
			{Path: "rendu-emilie-cote/notes.md", Content: []byte("# Notes de ecote\n")},
		}},
		{Work: "a26.5n6.01.tp1.jean-luc-picard", Files: []anonymize.File{
			{Path: "src/Solution.java", Content: []byte("// Jean-Luc Picard\nclass Solution { }\n")},
		}},
	}
}

func lireZip(t *testing.T, archive []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("archive : %v", err)
	}
	contenu := map[string]string{}
	for _, entry := range reader.File {
		file, err := entry.Open()
		if err != nil {
			t.Fatalf("lecture : %v", err)
		}
		var buffer bytes.Buffer
		buffer.ReadFrom(file)
		file.Close()
		contenu[entry.Name] = buffer.String()
	}
	return contenu
}

// La table ne voyage jamais avec le ZIP. C'est tout ce qui sépare « comparer du
// code » de « transmettre une liste de noms ».
func TestLaTableNeVoyagePasAvecLeZip(t *testing.T) {
	anonymizer := anonymiseur(t, anonymize.Options{})
	bundle, err := anonymize.Export(anonymizer,
		anonymize.Manifest{Assignment: "a26.5n6.01.tp1"}, copies())
	if err != nil {
		t.Fatalf("export : %v", err)
	}

	contenu := lireZip(t, bundle.Zip)
	tout := strings.Join(valeurs(contenu), "\n") + "\n" + strings.Join(cles(contenu), "\n")
	for _, interdit := range []string{
		"Émilie", "Côté", "emilie", "ecote", "Picard", "jlpicard", "2100123",
	} {
		if strings.Contains(tout, interdit) {
			t.Fatalf("« %s » se trouve dans le ZIP", interdit)
		}
	}
	if _, present := contenu["correspondance.csv"]; present {
		t.Fatal("la table ne doit pas être dans l'archive")
	}

	// Elle est rendue à part, et elle porte tout ce que le ZIP tait.
	if len(bundle.Table.Tokens) != 2 {
		t.Fatalf("table : %+v", bundle.Table.Tokens)
	}
	brut, err := bundle.Table.CSV()
	if err != nil {
		t.Fatalf("CSV : %v", err)
	}
	lignes, err := csv.NewReader(bytes.NewReader(brut)).ReadAll()
	if err != nil || len(lignes) != 3 || lignes[0][0] != "jeton" {
		t.Fatalf("CSV de la table : %v (%v)", lignes, err)
	}
	if !strings.Contains(string(brut), "Émilie Côté") {
		t.Fatalf("la table doit porter les noms : %s", brut)
	}

	// Et le ZIP dit où elle est plutôt que de laisser chercher.
	if !strings.Contains(contenu[anonymize.NoticeFile], "n'est pas ici") {
		t.Fatalf("la note doit dire où est la table : %s", contenu[anonymize.NoticeFile])
	}
}

func TestLeManifesteDecritLEnvoiSansNommerPersonne(t *testing.T) {
	anonymizer := anonymiseur(t, anonymize.Options{})
	bundle, err := anonymize.Export(anonymizer, anonymize.Manifest{
		Assignment: "a26.5n6.01.tp1", Profile: "tout",
	}, copies())
	if err != nil {
		t.Fatalf("export : %v", err)
	}

	recu, err := anonymize.Import(bundle.Zip)
	if err != nil {
		t.Fatalf("import : %v", err)
	}
	if recu.Manifest.Assignment != "a26.5n6.01.tp1" || recu.Manifest.Profile != "tout" {
		t.Fatalf("manifeste : %+v", recu.Manifest)
	}
	if len(recu.Copies) != 2 || len(recu.Warnings) != 0 {
		t.Fatalf("copies reçues : %d, avis : %v", len(recu.Copies), recu.Warnings)
	}
	// La provenance suit : c'est elle qui colore les nuages de points chez le
	// destinataire.
	if origine := recu.OriginOf(recu.Copies[0].Work); origine != "groupe 01" {
		t.Fatalf("provenance : %q", origine)
	}
	for _, copie := range recu.Copies {
		for _, fichier := range copie.Files {
			if strings.Contains(string(fichier.Content), "class Solution") {
				continue
			}
			if !strings.HasSuffix(fichier.Path, ".md") {
				t.Fatalf("contenu abîmé : %s", fichier.Content)
			}
		}
	}
}

// Un collègue qui n'utilise pas l'outil enverra un ZIP ordinaire. Le refuser
// reviendrait à ne servir à rien pour le seul cas où l'on en a besoin.
func TestUnZipSansManifesteSeLitQuandMeme(t *testing.T) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for nom, contenu := range map[string]string{
		"etudiant-a/Main.java": "class Main { }",
		"etudiant-b/Main.java": "class Main { }",
	} {
		entry, _ := archive.Create(nom)
		entry.Write([]byte(contenu))
	}
	archive.Close()

	recu, err := anonymize.Import(buffer.Bytes())
	if err != nil {
		t.Fatalf("import : %v", err)
	}
	if len(recu.Copies) != 2 {
		t.Fatalf("copies : %+v", recu.Copies)
	}
	if len(recu.Warnings) == 0 {
		t.Fatal("l'absence de manifeste doit être signalée")
	}
	if recu.OriginOf("etudiant-a") != "" {
		t.Fatal("sans manifeste, la provenance est inconnue")
	}
}

func TestUneArchiveSuspecteEstRefuseeOuEcartee(t *testing.T) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for nom, contenu := range map[string]string{
		"../../etc/passwd":     "racine",
		"a-la-racine.txt":      "sans copie",
		"etudiant-a/Main.java": "class Main { }",
		"etudiant-b/Main.java": "class Main { }",
	} {
		entry, _ := archive.Create(nom)
		entry.Write([]byte(contenu))
	}
	archive.Close()

	recu, err := anonymize.Import(buffer.Bytes())
	if err != nil {
		t.Fatalf("import : %v", err)
	}
	for _, copie := range recu.Copies {
		if strings.Contains(copie.Work, "..") {
			t.Fatalf("un chemin sortant de l'archive est entré : %+v", copie)
		}
	}
	if len(recu.Warnings) < 2 {
		t.Fatalf("les écartements doivent être dits : %v", recu.Warnings)
	}
	if _, err := anonymize.Import([]byte("ceci n'est pas un zip")); err == nil {
		t.Fatal("une archive illisible doit être refusée")
	}
}

func cles(contenu map[string]string) []string {
	noms := make([]string, 0, len(contenu))
	for nom := range contenu {
		noms = append(noms, nom)
	}
	return noms
}

func valeurs(contenu map[string]string) []string {
	textes := make([]string, 0, len(contenu))
	for _, texte := range contenu {
		textes = append(textes, texte)
	}
	return textes
}

// Un jeton se lit dans un rapport et s'écrit à un collègue : il ne doit jamais
// former un mot par accident.
func TestUnJetonNePeutPasFormerDeMot(t *testing.T) {
	voyelles := "AEIOUYaeiouy"
	for graine := int64(1); graine <= 200; graine++ {
		anonymizer := anonymiseur(t, anonymize.Options{Seed: graine})
		for _, token := range anonymizer.Tokens() {
			if strings.ContainsAny(token.Base, voyelles) {
				t.Fatalf("le jeton « %s » porte une voyelle", token.Base)
			}
			if len(token.Base) != anonymize.TokenLength {
				t.Fatalf("longueur du jeton : %q", token.Base)
			}
		}
	}
}
