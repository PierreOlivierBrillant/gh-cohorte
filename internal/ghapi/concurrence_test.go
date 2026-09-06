package ghapi_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
)

// Le registre des étudiants s'écrira par échange conditionnel : on lit la tête
// de la branche, on fabrique un commit qui en descend, puis on demande à GitHub
// de faire avancer la référence — sans forcer. GitHub refuse alors tout ce qui
// n'est pas une avance rapide, et c'est ce refus qui tient lieu de verrou entre
// deux personnes qui écrivent en même temps.
//
// Ces tests éprouvent le refus lui-même. Sans lui, le mécanisme d'écriture du
// registre serait bâti sur une garantie que rien n'atteste.

// ecrire fabrique un commit qui descend de « base » et tente de faire avancer
// la branche dessus. Le booléen dit que GitHub l'a accepté.
func ecrire(t *testing.T, c *ghapi.Client, repo, fichier, contenu, base string) (string, bool) {
	t.Helper()
	blob, err := c.CreateBlob("acme", repo, []byte(contenu))
	if err != nil {
		t.Fatalf("CreateBlob : %v", err)
	}
	arbreBase := ""
	var parents []string
	if base != "" {
		if arbreBase, err = c.CommitTree("acme", repo, base); err != nil {
			t.Fatalf("CommitTree : %v", err)
		}
		parents = []string{base}
	}
	arbre, err := c.CreateTree("acme", repo,
		[]ghapi.TreeEntry{{Path: fichier, Mode: "100644", Type: "blob", SHA: blob}}, arbreBase)
	if err != nil {
		t.Fatalf("CreateTree : %v", err)
	}
	commit, err := c.CreateCommit("acme", repo, "Écrit "+fichier, arbre, parents)
	if err != nil {
		t.Fatalf("CreateCommit : %v", err)
	}
	return commit, c.SetBranchHead("acme", repo, "main", commit, base == "") == nil
}

// Deux personnes partent du même commit. La première passe ; la seconde est
// refusée, parce que son commit ne descend pas de ce qui est désormais en
// place. C'est exactement ce qu'il faut pour qu'aucune écriture n'en efface une
// autre sans qu'on le sache.
func TestDeuxEcrituresDepuisLaMemeTeteSExcluent(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", ".cohorte", true)
	c, _ := client(t, state)

	// Premier commit : la branche n'existe pas encore.
	depart, ok := ecrire(t, c, ".cohorte", "etudiants.json", `{"v":0}`, "")
	if !ok {
		t.Fatal("le premier commit doit passer")
	}

	// Les deux personnes lisent la même tête.
	tete, err := c.BranchHead("acme", ".cohorte", "main")
	if err != nil || tete != depart {
		t.Fatalf("BranchHead = %q, %v", tete, err)
	}

	if _, ok := ecrire(t, c, ".cohorte", "etudiants.json", `{"v":1}`, tete); !ok {
		t.Fatal("la première des deux écritures doit passer")
	}
	if _, ok := ecrire(t, c, ".cohorte", "etudiants.json", `{"v":2}`, tete); ok {
		t.Fatal("la seconde doit être refusée : elle ne descend pas de ce qui est en place")
	}

	// Ce qui est en place est bien la première, intacte.
	relu, err := c.ReadFile("acme", ".cohorte", "etudiants.json", "main")
	if err != nil || relu == nil {
		t.Fatalf("ReadFile = %+v, %v", relu, err)
	}
	if string(relu.Content) != `{"v":1}` {
		t.Fatalf("contenu = %q : une écriture en a effacé une autre", relu.Content)
	}
}

// Le rejeu : la seconde personne relit la tête, refait son commit dessus, et
// passe — sans rien perdre de ce que la première a écrit. C'est la boucle
// entière que le registre suivra.
func TestLeRejeuSurLaTeteFraichePasseEtConserveTout(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", ".cohorte", true)
	c, _ := client(t, state)

	depart, _ := ecrire(t, c, ".cohorte", "etudiants.json", `{"v":0}`, "")
	if _, ok := ecrire(t, c, ".cohorte", "groupes.json", `{"a":1}`, depart); !ok {
		t.Fatal("la première écriture doit passer")
	}
	// La seconde est partie de « depart » et se fait refuser.
	if _, ok := ecrire(t, c, ".cohorte", "etudiants.json", `{"v":2}`, depart); ok {
		t.Fatal("une écriture périmée doit être refusée")
	}

	// Elle relit, rejoue, et passe.
	fraiche, err := c.BranchHead("acme", ".cohorte", "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ecrire(t, c, ".cohorte", "etudiants.json", `{"v":2}`, fraiche); !ok {
		t.Fatal("le rejeu sur la tête fraîche doit passer")
	}

	// Les deux travaux sont là : ni fusion, ni perte.
	fichiers := map[string]string{
		"etudiants.json": `{"v":2}`,
		"groupes.json":   `{"a":1}`,
	}
	for nom, attendu := range fichiers {
		relu, err := c.ReadFile("acme", ".cohorte", nom, "")
		if err != nil || relu == nil {
			t.Fatalf("ReadFile(%s) = %+v, %v", nom, relu, err)
		}
		if string(relu.Content) != attendu {
			t.Errorf("%s = %q, attendu %q", nom, relu.Content, attendu)
		}
	}
}

// Un fichier absent n'est pas une panne : c'est l'état d'un registre qu'on n'a
// pas encore amorcé.
//
// GitHub le dit de deux façons, et c'est là le piège : 404 quand le fichier
// manque d'un dépôt garni, 409 « Git Repository is empty. » quand le dépôt n'a
// aucun commit. Les deux répondent la même chose à qui demande un fichier.
func TestFichierAbsentNestPasUneErreur(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", ".cohorte", true) // créé, jamais rempli
	state.AddRepo("acme", "garni", true)
	state.SeedCommit("acme/garni", map[string]string{"README.md": "# ici\n"}, "main")
	c, _ := client(t, state)

	// Dépôt sans aucun commit : GitHub répond 409.
	relu, err := c.ReadFile("acme", ".cohorte", "etudiants.json", "")
	if err != nil || relu != nil {
		t.Fatalf("dépôt vide : ReadFile = %+v, %v", relu, err)
	}
	// Dépôt garni, fichier absent : GitHub répond 404.
	relu, err = c.ReadFile("acme", "garni", "etudiants.json", "")
	if err != nil || relu != nil {
		t.Fatalf("fichier absent : ReadFile = %+v, %v", relu, err)
	}
	// Et ce qui est là se lit toujours.
	if lu, err := c.ReadFile("acme", "garni", "README.md", ""); err != nil || lu == nil {
		t.Fatalf("ReadFile(README.md) = %+v, %v", lu, err)
	}
	if _, err := c.ReadFile("acme", "absent", "etudiants.json", ""); err != nil {
		t.Errorf("un dépôt absent doit se lire comme un fichier absent : %v", err)
	}
}

// Le contenu se relit tel qu'il a été écrit, accents compris : le registre
// portera des noms complets.
func TestContenuReluTelQuEcrit(t *testing.T) {
	state := fakegh.NewState()
	state.AddRepo("acme", ".cohorte", true)
	c, _ := client(t, state)

	contenu := `[{"compte":"ecote","nom":"Émilie Côté"}]`
	if _, ok := ecrire(t, c, ".cohorte", "registre/etudiants.json", contenu, ""); !ok {
		t.Fatal("l'écriture doit passer")
	}
	relu, err := c.ReadFile("acme", ".cohorte", "registre/etudiants.json", "main")
	if err != nil || relu == nil {
		t.Fatalf("ReadFile = %+v, %v", relu, err)
	}
	if string(relu.Content) != contenu {
		t.Fatalf("contenu = %q", relu.Content)
	}
	if !strings.Contains(string(relu.Content), "Côté") {
		t.Error("les accents doivent survivre à l'aller-retour")
	}
}
