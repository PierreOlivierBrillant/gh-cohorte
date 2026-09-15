package signature_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/signature"
)

func marque(t *testing.T) string {
	t.Helper()
	token, err := signature.New()
	if err != nil {
		t.Fatalf("tirage : %v", err)
	}
	return signature.Text(token)
}

func registre(t *testing.T) (signature.Book, string, string) {
	t.Helper()
	premiere, seconde := marque(t), marque(t)
	book, err := signature.Book{Issued: []signature.Issued{
		{Assignment: "a26.5n6.01.tp1", Username: "emilie-cote", Token: premiere},
		{Assignment: "a26.5n6.01.tp1", Username: "jlpicard", Token: seconde},
	}}.Validate()
	if err != nil {
		t.Fatalf("registre refusé : %v", err)
	}
	return book, premiere, seconde
}

// Le registre est le seul endroit qui relie une marque à quelqu'un : deux
// travaux qui portent la même se reconnaissent sans lui, mais nul autre ne peut
// dire de qui il s'agit.
func TestLeRegistreRepondADeQuiEstUneMarque(t *testing.T) {
	book, premiere, _ := registre(t)

	ligne, trouve := book.Who(premiere)
	if !trouve || ligne.Username != "emilie-cote" {
		t.Fatalf("recherche par marque : %+v", ligne)
	}
	if _, trouve := book.Of("a26.5n6.01.tp1", "jlpicard"); !trouve {
		t.Fatal("recherche par personne")
	}
	if _, trouve := book.Who(marque(t)); trouve {
		t.Fatal("une marque jamais délivrée ne doit désigner personne")
	}
}

// Deux personnes ne peuvent pas porter la même marque : la détection les
// confondrait, et c'est exactement ce qu'elle existe pour éviter.
func TestDeuxPersonnesNePeuventPasPorterLaMemeMarque(t *testing.T) {
	commune := marque(t)
	_, err := signature.Book{Issued: []signature.Issued{
		{Assignment: "a26.5n6.01.tp1", Username: "emilie-cote", Token: commune},
		{Assignment: "a26.5n6.01.tp1", Username: "jlpicard", Token: commune},
	}}.Validate()
	if err == nil || !strings.Contains(err.Error(), commune) {
		t.Fatalf("le doublon doit être refusé en nommant la marque : %v", err)
	}
}

// Redistribuer un travail délivre une nouvelle marque : c'est la plus récente
// qui vaut, et l'ancienne ne doit pas rester à désigner la même personne.
func TestRedistribuerRemplaceLaMarque(t *testing.T) {
	book, premiere, _ := registre(t)
	nouvelle := marque(t)

	apres, bouge, err := book.With([]signature.Issued{
		{Assignment: "a26.5n6.01.tp1", Username: "emilie-cote", Token: nouvelle},
	})
	if err != nil || !bouge {
		t.Fatalf("versement : %v", err)
	}
	if len(apres.Issued) != 2 {
		t.Fatalf("lignes : %+v", apres.Issued)
	}
	if ligne, _ := apres.Of("a26.5n6.01.tp1", "emilie-cote"); ligne.Token != nouvelle {
		t.Fatalf("la marque la plus récente doit gagner : %+v", ligne)
	}
	if _, trouve := apres.Who(premiere); trouve {
		t.Fatal("l'ancienne marque ne doit plus désigner personne")
	}
	// Celle du voisin survit : verser les siennes ne retire pas les autres.
	if _, trouve := apres.Of("a26.5n6.01.tp1", "jlpicard"); !trouve {
		t.Fatal("la marque d'un autre a disparu")
	}
	// Et verser ce qui s'y trouve déjà ne bouge rien.
	if _, bouge, _ := apres.With(apres.Issued); bouge {
		t.Fatal("un versement sans changement ne doit rien écrire")
	}
}

func TestDesMarquesIncompletesSontRefusees(t *testing.T) {
	bonne := marque(t)
	cas := map[string]signature.Issued{
		"travail hors nomenclature": {Assignment: "tp1", Username: "x", Token: bonne},
		"sans personne":             {Assignment: "a26.5n6.01.tp1", Token: bonne},
		"marque invalide": {Assignment: "a26.5n6.01.tp1", Username: "x",
			Token: "pas-une-marque"},
	}
	for nom, ligne := range cas {
		if _, err := (signature.Book{Issued: []signature.Issued{ligne}}).Validate(); err == nil {
			t.Fatalf("« %s » aurait dû être refusé", nom)
		}
	}
}

// Le fichier se modifie à la main sur github.com : perdre une marque est un
// désagrément, perdre toutes les autres serait une perte sèche.
func TestUneLigneMalEcriteNEmportePasLeRegistre(t *testing.T) {
	book, _, _ := registre(t)
	contenu, err := signature.EncodeBook(book)
	if err != nil {
		t.Fatalf("écriture : %v", err)
	}
	abime := strings.Replace(string(contenu), `"a26.5n6.01.tp1"`, `"tp1"`, 1)

	relu, soucis := signature.DecodeBook([]byte(abime))
	if len(relu.Issued) != 1 {
		t.Fatalf("lignes gardées : %+v", relu.Issued)
	}
	if len(soucis) != 1 {
		t.Fatalf("la ligne fautive doit être signalée : %v", soucis)
	}
	if _, soucis := signature.DecodeBook([]byte(`{"version": 99}`)); len(soucis) == 0 ||
		!strings.Contains(soucis[0], "Mettez l'extension à jour") {
		t.Fatalf("une version future doit proposer la mise à jour : %v", soucis)
	}
}
