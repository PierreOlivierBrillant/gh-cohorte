package teams_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/teams"
)

// cohorte décrit trois équipes de l'organisation : deux d'un groupe, une d'un
// autre, plus une qui ne relève d'aucun groupe.
func cohorte() []teams.Info {
	return []teams.Info{
		{Slug: "a26-5n6-01-eq1", Name: "a26.5n6.01.eq1", Members: []string{"emilie-cote"}},
		{Slug: "a26-5n6-01-eq2", Name: "a26.5n6.01.eq2", Members: []string{"jlpicard", "aminata-d"}},
		{Slug: "a26-5n6-02-eq1", Name: "a26.5n6.02.eq1", Members: []string{"autre"}},
		{Slug: "les-anciens", Name: "Les anciens", Members: []string{"jlpicard"}},
	}
}

func TestInNeRetientQueLesEquipesDuGroupe(t *testing.T) {
	trouvees := teams.In("a26", "5n6", "01", cohorte())
	if len(trouvees) != 2 {
		t.Fatalf("2 équipes attendues, %d trouvées", len(trouvees))
	}
	if trouvees[0].Short != "eq1" || trouvees[1].Short != "eq2" {
		t.Fatalf("équipes mal nommées ou mal ordonnées : %v", teams.Names(trouvees))
	}
	// Deux groupes peuvent avoir chacun leur « eq1 » : c'est la place inscrite
	// dans le nom qui les distingue, et c'est tout l'intérêt de l'y écrire.
	autre := teams.In("a26", "5n6", "02", cohorte())
	if len(autre) != 1 || autre[0].Short != "eq1" {
		t.Fatalf("le groupe 02 devrait avoir son propre « eq1 » : %v", teams.Names(autre))
	}
	if autre[0].Slug == trouvees[0].Slug {
		t.Fatal("deux « eq1 » de groupes différents partagent une adresse GitHub")
	}
}

func TestLooseNeGardeQueCeQuAucunGroupeNeReclame(t *testing.T) {
	libres := teams.Loose(cohorte())
	if len(libres) != 1 || libres[0].Slug != "les-anciens" {
		t.Fatalf("une seule équipe est libre d'un groupe, trouvé %d", len(libres))
	}
}

func TestUneEquipeNAccueilleQuUneFoisLaMemePersonne(t *testing.T) {
	liste := teams.In("a26", "5n6", "01", cohorte())
	if _, err := teams.PlanAssign(liste, "eq1", []string{"emilie-cote"}); err == nil {
		t.Fatal("inscrire quelqu'un là où il est déjà devrait être refusé")
	}
}

func TestChangerDEquipeRetireDeLAncienneAvantDInscrire(t *testing.T) {
	liste := teams.In("a26", "5n6", "01", cohorte())
	etapes, err := teams.PlanAssign(liste, "eq1", []string{"jlpicard"})
	if err != nil {
		t.Fatalf("déplacement refusé : %v", err)
	}
	if len(etapes) != 2 {
		t.Fatalf("un retrait puis une inscription attendus, %d étape(s)", len(etapes))
	}
	// L'ordre compte : entre les deux écritures, la personne ne doit jamais se
	// retrouver dans deux équipes à la fois.
	if etapes[0].Kind != teams.Leave || etapes[0].Team != "eq2" {
		t.Fatalf("le retrait de son ancienne équipe devrait venir d'abord : %+v", etapes[0])
	}
	if etapes[1].Kind != teams.Join || etapes[1].Team != "eq1" {
		t.Fatalf("l'inscription devrait suivre : %+v", etapes[1])
	}
}

func TestComposerRetireCeuxQuOnNAttendPlus(t *testing.T) {
	liste := teams.In("a26", "5n6", "01", cohorte())
	etapes, err := teams.PlanCompose(liste, "eq2", []string{"jlpicard"})
	if err != nil {
		t.Fatalf("composition refusée : %v", err)
	}
	if len(etapes) != 1 || etapes[0].Kind != teams.Leave || etapes[0].Username != "aminata-d" {
		t.Fatalf("seule Aminata devrait sortir : %+v", etapes)
	}
}

func TestComposerAVideNeLaissePersonne(t *testing.T) {
	liste := teams.In("a26", "5n6", "01", cohorte())
	etapes, err := teams.PlanCompose(liste, "eq2", nil)
	if err != nil {
		t.Fatalf("composition refusée : %v", err)
	}
	if len(etapes) != 2 {
		t.Fatalf("les deux membres devraient sortir, %d étape(s)", len(etapes))
	}
	for _, etape := range etapes {
		if etape.Kind != teams.Leave {
			t.Fatalf("seuls des retraits sont attendus : %+v", etape)
		}
	}
}

func TestRenommerRefuseUnNomDejaPris(t *testing.T) {
	liste := teams.In("a26", "5n6", "01", cohorte())
	if _, _, err := teams.PlanRename(liste, "eq1", "eq2"); err == nil {
		t.Fatal("renommer vers un nom occupé devrait être refusé")
	}
	_, nom, err := teams.PlanRename(liste, "eq1", "rouge")
	if err != nil {
		t.Fatalf("renommage refusé : %v", err)
	}
	// La place ne bouge pas : une équipe ne change pas de groupe.
	if nom != "a26.5n6.01.rouge" {
		t.Fatalf("nom d'arrivée inattendu : %s", nom)
	}
}

func TestAdopterRefuseUneEquipeDejaDansUnGroupe(t *testing.T) {
	liste := teams.In("a26", "5n6", "01", cohorte())
	_, _, err := teams.PlanAdopt(liste, cohorte(), "a26-5n6-02-eq1", "eq3", "a26", "5n6", "01")
	if err == nil {
		t.Fatal("une équipe qui relève déjà d'un groupe ne s'adopte pas")
	}
	if !strings.Contains(err.Error(), "relève déjà") {
		t.Fatalf("message peu explicite : %v", err)
	}
}

func TestAdopterRenommeSansToucherAuxMembres(t *testing.T) {
	liste := teams.In("a26", "5n6", "01", cohorte())
	source, nom, err := teams.PlanAdopt(liste, cohorte(), "les-anciens", "eq3", "a26", "5n6", "01")
	if err != nil {
		t.Fatalf("adoption refusée : %v", err)
	}
	if nom != "a26.5n6.01.eq3" {
		t.Fatalf("nom d'arrivée inattendu : %s", nom)
	}
	if len(source.Members) != 1 || source.Members[0] != "jlpicard" {
		t.Fatalf("l'équipe adoptée garde ses membres : %v", source.Members)
	}
}

func TestUnNomCourtSeSlugifieCommeLesAutresNiveaux(t *testing.T) {
	court, err := teams.ShortName("Équipe Rouge")
	if err != nil {
		t.Fatalf("nom refusé : %v", err)
	}
	if court != "equipe-rouge" {
		t.Fatalf("slugification inattendue : %s", court)
	}
	// Le point sépare les niveaux du nom : il ne peut pas entrer dans l'un d'eux.
	if strings.Contains(court, ".") {
		t.Fatal("un nom court ne doit jamais porter le séparateur")
	}
}

func TestOfRetrouveLEquipeDUnePersonne(t *testing.T) {
	liste := teams.In("a26", "5n6", "01", cohorte())
	equipe, membre := teams.Of(liste, "AMINATA-D")
	if !membre || equipe.Short != "eq2" {
		t.Fatalf("Aminata devrait être reconnue dans eq2 : %v %+v", membre, equipe)
	}
	if _, membre := teams.Of(liste, "inconnu"); membre {
		t.Fatal("un compte étranger ne devrait appartenir à aucune équipe")
	}
}
