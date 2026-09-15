package signature_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/signature"
)

const readme = `# Travail pratique 1

Consignes du travail, recopiées du plan de cours.

## Ce qu'il faut remettre

Le code, et rien d'autre.
`

// La marque doit être invisible : ni à la lecture du fichier, ni au rendu du
// markdown. Une ligne vide qui contient des blancs reste une ligne vide.
func TestLaMarqueNeSeVoitPas(t *testing.T) {
	token, err := signature.New()
	if err != nil {
		t.Fatalf("tirage : %v", err)
	}
	signe := string(signature.Sign([]byte(readme), token))

	// Rien de visible n'a bougé : chaque ligne, une fois ses blancs de fin
	// retirés, est celle d'avant.
	avant := strings.Split(strings.TrimRight(readme, "\n"), "\n")
	apres := strings.Split(strings.TrimRight(signe, "\n"), "\n")
	if len(apres) < len(avant) {
		t.Fatalf("des lignes ont disparu :\n%q", signe)
	}
	for index, ligne := range avant {
		if strings.TrimRight(apres[index], " \t") != strings.TrimRight(ligne, " \t") {
			t.Fatalf("ligne %d changée : %q devient %q", index, ligne, apres[index])
		}
	}
	// Et rien n'a été ajouté qui se voie.
	for _, ligne := range apres[len(avant):] {
		if strings.TrimSpace(ligne) != "" {
			t.Fatalf("une ligne visible a été ajoutée : %q", ligne)
		}
	}
}

// Une ligne de texte ne peut pas porter la marque : deux espaces en fin de
// ligne valent un saut de ligne en markdown, et elle en ajouterait un.
func TestLaMarqueNeSePosePasSurUneLigneDeTexte(t *testing.T) {
	token, _ := signature.New()
	signe := string(signature.Sign([]byte(readme), token))
	for _, ligne := range strings.Split(signe, "\n") {
		if strings.TrimSpace(ligne) == "" {
			continue
		}
		if strings.HasSuffix(ligne, " ") || strings.HasSuffix(ligne, "\t") {
			t.Fatalf("une ligne de texte porte des blancs de fin : %q", ligne)
		}
	}
}

func TestLaMarqueSeRelitEtSeCompteEnTrois(t *testing.T) {
	token, _ := signature.New()
	signe := signature.Sign([]byte(readme), token)

	trouves := signature.Find(signe)
	if len(trouves) != 1 || trouves[0] != token {
		t.Fatalf("jetons trouvés : %v, attendu %v", trouves, token)
	}
	// Trois exemplaires : un formateur qui coupe les blancs en efface souvent
	// plusieurs, mais une retouche à la main n'en touche qu'un.
	exemplaires := 0
	for _, ligne := range strings.Split(string(signe), "\n") {
		if _, signee := signature.Read(ligne); signee {
			exemplaires++
		}
	}
	if exemplaires != signature.Copies {
		t.Fatalf("exemplaires posés : %d", exemplaires)
	}

	// Un seul suffit : les deux autres peuvent disparaître.
	lignes := strings.Split(string(signe), "\n")
	coupes := 0
	for index, ligne := range lignes {
		if _, signee := signature.Read(ligne); signee && coupes < 2 {
			lignes[index] = ""
			coupes++
		}
	}
	if trouve, ok := signature.First([]byte(strings.Join(lignes, "\n"))); !ok || trouve != token {
		t.Fatalf("un seul exemplaire doit suffire : %v", ok)
	}
}

// Un README d'une seule ligne n'a pas de ligne vide : la marque s'écrit alors
// à la fin, et cela ne change rien à ce qu'on lit.
func TestUnFichierSansLigneVideSeSigneQuandMeme(t *testing.T) {
	token, _ := signature.New()
	signe := signature.Sign([]byte("# Travail\n"), token)
	if trouve, ok := signature.First(signe); !ok || trouve != token {
		t.Fatalf("signature : %v", ok)
	}
	if !strings.HasPrefix(string(signe), "# Travail\n") {
		t.Fatalf("le contenu doit survivre : %q", signe)
	}
	if !strings.HasSuffix(string(signe), "\n") {
		t.Fatalf("un fichier texte finit par un saut de ligne : %q", signe)
	}
}

// Resigner remplace la marque plutôt que d'en empiler une seconde : deux
// jetons dans un même travail rendraient la détection ambiguë.
func TestResignerRemplaceLaMarque(t *testing.T) {
	premier, _ := signature.New()
	second, _ := signature.New()
	signe := signature.Sign(signature.Sign([]byte(readme), premier), second)

	trouves := signature.Find(signe)
	if len(trouves) != 1 || trouves[0] != second {
		t.Fatalf("jetons : %v, attendu le second seul", trouves)
	}
}

// Une suite de blancs quelconque ne doit pas passer pour une marque : c'est à
// cela que sert la somme de contrôle.
func TestUneIndentationNEstPasUneMarque(t *testing.T) {
	cas := []string{
		strings.Repeat(" ", signature.Length),
		strings.Repeat("\t", signature.Length),
		strings.Repeat(" ", 200),
		strings.Repeat(" \t", signature.Length),
		"", "    ", "du texte ordinaire",
	}
	for _, blancs := range cas {
		if token, signee := signature.Read(blancs); signee {
			t.Fatalf("« %q » a été pris pour la marque %v", blancs, token)
		}
	}
	// Et une marque qu'on abîme cesse d'en être une. Les blancs sont inversés
	// plutôt que fixés : les fixer laisserait passer le cas — fréquent — où ils
	// valaient déjà cela.
	token, _ := signature.New()
	marque := []rune(signature.Mark(token))
	for _, rang := range []int{3, 7, 19} {
		if marque[rang] == ' ' {
			marque[rang] = '\t'
		} else {
			marque[rang] = ' '
		}
	}
	if _, signee := signature.Read(string(marque)); signee {
		t.Fatal("une marque abîmée ne doit pas se relire")
	}
}

// Une ligne qui portait déjà des blancs en garde : la marque se pose après eux.
func TestUneLigneDejaBlancheGardeSesBlancs(t *testing.T) {
	token, _ := signature.New()
	marque := "   " + signature.Mark(token)
	if trouve, signee := signature.Read(marque); !signee || trouve != token {
		t.Fatalf("relecture après des blancs existants : %v", signee)
	}
}

// Un jeton se recopie parfois à la main, d'un rapport à un courriel.
func TestUnJetonSeLitEtSeRelit(t *testing.T) {
	token, _ := signature.New()
	texte := signature.Text(token)
	if len(texte) != 14 || strings.Count(texte, "-") != 2 {
		t.Fatalf("forme lisible : %q", texte)
	}
	relu, err := signature.Parse(texte)
	if err != nil || relu != token {
		t.Fatalf("relecture : %v (%v)", relu, err)
	}
	if _, err := signature.Parse("pas-une-marque"); err == nil {
		t.Fatal("une forme invalide doit être refusée")
	}
}

// Deux tirages ne doivent pas coïncider : c'est ce qui fait qu'une même marque
// dans deux travaux n'a pas d'explication innocente.
func TestDeuxTiragesNeCoincidentPas(t *testing.T) {
	vus := map[uint64]bool{}
	for essai := 0; essai < 500; essai++ {
		token, err := signature.New()
		if err != nil {
			t.Fatalf("tirage : %v", err)
		}
		if vus[token] {
			t.Fatalf("deux tirages identiques : %v", token)
		}
		vus[token] = true
	}
}
