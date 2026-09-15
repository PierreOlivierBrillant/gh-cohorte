package exchange_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/exchange"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/similarity"
)

func catalogue() exchange.Catalog {
	return exchange.Catalog{Teaching: []exchange.Teaching{
		{Scope: "a26.5n6.01", Assignment: "tp1", Teacher: "prof", Copies: 30,
			LastHandin: "2026-10-01", UpdatedAt: "2026-10-02"},
		{Scope: "a26.5n6.02", Assignment: "tp1", Teacher: "collegue", Copies: 28,
			Indexed: true, UpdatedAt: "2026-10-02"},
		{Scope: "h25.5m6.01", Assignment: "tp1", Teacher: "collegue", Copies: 25,
			UpdatedAt: "2025-03-01"},
	}}
}

// Le catalogue dit ce qu'un collègue a donné, sans rien dire de ses étudiants.
func TestLeCatalogueNommeDesTravauxEtPasDesPersonnes(t *testing.T) {
	valide, err := catalogue().Validate()
	if err != nil {
		t.Fatalf("catalogue refusé : %v", err)
	}
	if len(valide.Teaching) != 3 {
		t.Fatalf("lignes : %+v", valide.Teaching)
	}
	contenu, err := exchange.EncodeCatalog(valide)
	if err != nil {
		t.Fatalf("écriture : %v", err)
	}
	// Rien d'un nom de dépôt ne doit s'y trouver : le dernier niveau nomme une
	// personne, et le publier reviendrait à publier la liste de classe.
	if strings.Contains(string(contenu), "tp1.") {
		t.Fatalf("un nom de dépôt s'est glissé dans le catalogue : %s", contenu)
	}
	if ligne, trouve := valide.Find("a26.5n6.02.tp1"); !trouve || ligne.Copies != 28 {
		t.Fatalf("recherche : %+v", ligne)
	}
}

// Publier ce qu'on donne ne doit pas retirer ce qu'un collègue a publié : le
// catalogue n'appartient à personne, et chacun n'y écrit que ses lignes.
func TestVerserAuCatalogueNeRetireRienAuxAutres(t *testing.T) {
	depart, _ := catalogue().Validate()
	apres, bouge, err := depart.With([]exchange.Teaching{
		{Scope: "a26.5n6.01", Assignment: "tp2", Teacher: "prof", Copies: 30,
			UpdatedAt: "2026-11-01"},
	})
	if err != nil || !bouge {
		t.Fatalf("versement : %v (bougé : %v)", err, bouge)
	}
	if len(apres.Teaching) != 4 {
		t.Fatalf("lignes : %+v", apres.Teaching)
	}
	if _, trouve := apres.Find("a26.5n6.02.tp1"); !trouve {
		t.Fatal("la ligne d'un collègue a disparu")
	}

	// Une ligne réécrite remplace la sienne, et la plus récente gagne.
	remplace, bouge, err := apres.With([]exchange.Teaching{
		{Scope: "a26.5n6.01", Assignment: "tp1", Teacher: "prof", Copies: 31,
			UpdatedAt: "2026-12-01"},
	})
	if err != nil || !bouge {
		t.Fatalf("remplacement : %v", err)
	}
	if ligne, _ := remplace.Find("a26.5n6.01.tp1"); ligne.Copies != 31 {
		t.Fatalf("la ligne la plus récente doit gagner : %+v", ligne)
	}
	if len(remplace.Teaching) != 4 {
		t.Fatalf("lignes : %+v", remplace.Teaching)
	}

	// Verser ce qui s'y trouve déjà ne bouge rien : un commit sans effet salit
	// l'historique sans rien apprendre.
	if _, bouge, _ := remplace.With(remplace.Teaching); bouge {
		t.Fatal("un versement sans changement ne doit rien écrire")
	}
}

func TestLeCatalogueSeLitParEnseignantEtParCours(t *testing.T) {
	valide, _ := catalogue().Validate()
	if siens := valide.Of("collegue"); len(siens) != 2 {
		t.Fatalf("travaux du collègue : %+v", siens)
	}
	// Du plus récent au plus ancien : c'est celui de cette session qu'on
	// cherche, pas celui d'il y a trois ans.
	if siens := valide.Of("collegue"); siens[0].Scope != "a26.5n6.02" {
		t.Fatalf("ordre : %+v", siens)
	}
	// Les sigles équivalents sont dépliés par l'appelant : c'est « rules » qui
	// sait qu'un cours a changé de nom, pas le catalogue.
	if cours := valide.Course([]string{"5n6", "5m6"}); len(cours) != 3 {
		t.Fatalf("travaux du cours : %+v", cours)
	}
	if cours := valide.Course([]string{"5n6"}); len(cours) != 2 {
		t.Fatalf("sans équivalence : %+v", cours)
	}
	if comptes := valide.Teachers(); len(comptes) != 2 || comptes[0] != "collegue" {
		t.Fatalf("enseignants, du plus prolifique au moins : %v", comptes)
	}
}

// Le fichier se modifie à la main sur github.com : une ligne mal écrite ne doit
// pas priver toute l'organisation de son catalogue.
func TestUneLigneMalEcriteNEmportePasLeCatalogue(t *testing.T) {
	contenu := []byte(`{
      "version": 1,
      "teaching": [
        {"scope": "a26.5n6.01", "assignment": "tp1", "teacher": "prof", "copies": 30},
        {"scope": "pas-une-place", "assignment": "tp1", "teacher": "prof"},
        {"scope": "a26.5n6.02", "assignment": "tp2", "teacher": "collegue", "copies": 3}
      ]
    }`)
	lu, soucis := exchange.DecodeCatalog(contenu)
	if len(lu.Teaching) != 2 {
		t.Fatalf("lignes gardées : %+v", lu.Teaching)
	}
	if len(soucis) != 1 || !strings.Contains(soucis[0], "pas-une-place") {
		t.Fatalf("la ligne fautive doit être nommée : %v", soucis)
	}
	if _, soucis := exchange.DecodeCatalog([]byte(`{"version": 99}`)); len(soucis) == 0 ||
		!strings.Contains(soucis[0], "upgrade") {
		t.Fatalf("une version future doit proposer la mise à jour : %v", soucis)
	}
}

// ------------------------------------------------------------ index publié

func indexPublie() exchange.Published {
	return exchange.Published{
		Assignment: "a26.5n6.02.tp1", Teacher: "collegue", Origin: "groupe 02",
		Settings: exchange.Settings{Profile: "tout", Kgram: 23, Window: 17},
		Corpus: similarity.Corpus{Works: []similarity.Work{
			{ID: "K7DM2X", Files: []similarity.File{{
				Path: "src/Solution.java", Kgram: 23, Window: 17,
				Prints: []similarity.Print{{Hash: 42, Index: 0}},
			}}},
		}},
	}
}

// Un index qui porterait des noms de dépôts serait une liste de classe publiée
// à toute l'équipe. Le refus est à l'écriture comme à la lecture, plutôt que
// laissé à la vigilance de l'appelant.
func TestUnIndexNePeutPasPorterDeNomDeDepot(t *testing.T) {
	nommant := indexPublie()
	nommant.Corpus.Works[0].ID = "a26.5n6.02.tp1.emilie-cote"
	if _, err := exchange.EncodePublished(nommant); err == nil ||
		!strings.Contains(err.Error(), "nom de dépôt") {
		t.Fatalf("un nom de dépôt doit être refusé : %v", err)
	}

	etiquete := indexPublie()
	etiquete.Corpus.Works[0].Label = "Émilie Côté"
	if _, err := exchange.EncodePublished(etiquete); err == nil ||
		!strings.Contains(err.Error(), "ne nomme personne") {
		t.Fatalf("une étiquette doit être refusée : %v", err)
	}

	contenu, err := exchange.EncodePublished(indexPublie())
	if err != nil {
		t.Fatalf("écriture : %v", err)
	}
	relu, err := exchange.DecodePublished(contenu)
	if err != nil || relu.Copies() != 1 || relu.Prints() != 1 {
		t.Fatalf("index relu : %+v (%v)", relu, err)
	}
}

// Deux index aux bornes différentes ne portent pas les mêmes empreintes pour le
// même code. Les mêler donnerait un rapport rassurant et faux.
func TestUnIndexCalculeAutrementEstRefuseEnDisantPourquoi(t *testing.T) {
	publie := indexPublie()
	if err := publie.Comparable(publie.Settings); err != nil {
		t.Fatalf("un réglage identique doit passer : %v", err)
	}

	cas := map[string]exchange.Settings{
		"profil d'inspection":  {Profile: "next", Kgram: 23, Window: 17},
		"k-grammes":            {Profile: "tout", Kgram: 15, Window: 17},
		"fenêtre de winnowing": {Profile: "tout", Kgram: 23, Window: 8},
		"langages":             {Profile: "tout", Kgram: 23, Window: 17, Languages: []string{"java"}},
	}
	for quoi, mien := range cas {
		err := publie.Comparable(mien)
		if err == nil {
			t.Fatalf("« %s » : l'écart doit être refusé", quoi)
		}
		// Le refus nomme ce qui diffère : sans cela, on ne saurait pas quoi
		// aligner.
		if !strings.Contains(err.Error(), "collegue") {
			t.Fatalf("« %s » : le refus doit dire à qui s'adresser (%v)", quoi, err)
		}
	}
	if len(publie.Settings.Differences(cas["k-grammes"])) != 1 {
		t.Fatalf("écarts : %v", publie.Settings.Differences(cas["k-grammes"]))
	}
}

func TestLeCheminDUnIndexEstStable(t *testing.T) {
	if chemin := exchange.IndexPath("A26.5N6.01.TP1"); chemin != "index/a26.5n6.01.tp1.json" {
		t.Fatalf("chemin : %q", chemin)
	}
	if !strings.Contains(string(exchange.IndexReadmeText("acme")), "ni code, ni nom") {
		t.Fatal("le dépôt doit expliquer ce qu'il ne contient pas")
	}
}

// Une demande passe d'« en attente » à « accordée » en se réécrivant : c'est la
// dernière écrite qui vaut, sans quoi une décision n'en serait jamais une.
func TestTrancherUneDemandeRemplaceLaPrecedente(t *testing.T) {
	depart, err := exchange.Asks{Asks: []exchange.Ask{
		{ID: "K7DM2X", From: "prof", To: "collegue",
			Assignment: "a26.5n6.02.tp1", Token: "BCDFGH", CreatedAt: "2026-10-01T09:00:00Z"},
		{ID: "ZZ99ZZ", From: "collegue", To: "prof",
			Assignment: "a26.5n6.01.tp1", Token: "JKLMNP", CreatedAt: "2026-10-02T09:00:00Z"},
	}}.Validate()
	if err != nil {
		t.Fatalf("demandes refusées : %v", err)
	}
	if len(depart.Waiting("collegue")) != 1 || len(depart.By("prof")) != 1 {
		t.Fatalf("répartition : %+v", depart.Asks)
	}

	tranchee, _ := depart.Find("K7DM2X")
	tranchee.State, tranchee.DecidedAt = exchange.AskGranted, "2026-10-03T09:00:00Z"
	apres, bouge, err := depart.With([]exchange.Ask{tranchee})
	if err != nil || !bouge {
		t.Fatalf("décision : %v", err)
	}
	if len(apres.Asks) != 2 {
		t.Fatalf("demandes : %+v", apres.Asks)
	}
	relue, _ := apres.Find("K7DM2X")
	if relue.State != exchange.AskGranted {
		t.Fatalf("la décision doit l'emporter : %+v", relue)
	}
	if len(apres.Waiting("collegue")) != 0 {
		t.Fatal("une demande tranchée n'attend plus")
	}
	// Et celle du voisin n'a pas bougé.
	if voisine, _ := apres.Find("ZZ99ZZ"); !voisine.Pending() {
		t.Fatalf("la demande d'un autre a été touchée : %+v", voisine)
	}
}

func TestUnIdentifiantDeDemandeNeFormePasDeMot(t *testing.T) {
	vus := map[string]bool{}
	for essai := 0; essai < 300; essai++ {
		id, err := exchange.NewAskID()
		if err != nil {
			t.Fatalf("tirage : %v", err)
		}
		if strings.ContainsAny(id, "AEIOUYaeiouy") {
			t.Fatalf("l'identifiant « %s » porte une voyelle", id)
		}
		if len(id) != 6 {
			t.Fatalf("longueur : %q", id)
		}
		vus[id] = true
	}
	if len(vus) < 290 {
		t.Fatalf("trop de collisions : %d identifiants distincts sur 300", len(vus))
	}
}
