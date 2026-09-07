package registry_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// personne abrège l'écriture des cas.
func personne(nom, compte string) roster.Person {
	return roster.Person{FullName: nom, Username: compte}
}

// Apprendre quelqu'un retient d'emblée le slug que son nom produira : c'est
// celui que porteront ses dépôts.
func TestApprendreRetientLeSlugDuNom(t *testing.T) {
	set, _, err := appliquer(t, registry.Empty(),
		registry.Learn(personne("Émilie Côté", "ecote")))
	if err != nil {
		t.Fatal(err)
	}
	fiche, connue := set.Find("ecote")
	if !connue || fiche.FullName != "Émilie Côté" {
		t.Fatalf("fiche = %+v", fiche)
	}
	if len(fiche.Slugs) != 1 || fiche.Slugs[0] != "emilie-cote" {
		t.Fatalf("slugs = %v", fiche.Slugs)
	}
	// Le compte n'est pas sensible à la casse.
	if _, connue := set.Find("ECote"); !connue {
		t.Error("le compte doit se retrouver quelle que soit sa casse")
	}
}

// Le cœur de l'affaire : corriger l'orthographe d'un nom ne doit pas rendre
// orphelins les dépôts déjà créés sous l'ancien slug.
func TestCorrigerUnNomGardeLAncienSlug(t *testing.T) {
	// Le nom a d'abord été saisi avec une faute qui change le slug : les
	// premiers dépôts s'appellent « a26.5n6.01.tp1.emlie-cote ».
	set, _, err := appliquer(t, registry.Empty(),
		registry.Learn(personne("Emlie Côté", "ecote")))
	if err != nil {
		t.Fatal(err)
	}
	set, _, err = appliquer(t, set, registry.Learn(personne("Émilie Côté", "ecote")))
	if err != nil {
		t.Fatal(err)
	}

	fiche, _ := set.Find("ecote")
	if fiche.FullName != "Émilie Côté" {
		t.Fatalf("nom = %q", fiche.FullName)
	}
	// Les deux slugs mènent à elle : les dépôts d'avant la correction restent
	// rattachés, et ceux d'après le seront aussi.
	for _, slug := range []string{"emlie-cote", "emilie-cote"} {
		if trouvee, trouve := set.Resolve(slug); !trouve || trouvee.Username != "ecote" {
			t.Errorf("le slug « %s » ne mène plus à personne", slug)
		}
	}
	if len(fiche.Slugs) != 2 {
		t.Fatalf("slugs = %v : l'ancien doit être conservé", fiche.Slugs)
	}
}

// Adopter des dépôts hérités apprend un compte sans son nom. Cela ne doit
// jamais effacer un nom déjà connu.
func TestUnNomVideNEffaceJamaisUnNomConnu(t *testing.T) {
	set, _, err := appliquer(t, registry.Empty(),
		registry.Learn(personne("Jean-Luc Picard", "jlpicard")))
	if err != nil {
		t.Fatal(err)
	}
	set, bouge, err := appliquer(t, set, registry.Learn(personne("", "jlpicard")))
	if err != nil {
		t.Fatal(err)
	}
	if set.Name("jlpicard") != "Jean-Luc Picard" {
		t.Fatalf("nom = %q", set.Name("jlpicard"))
	}
	if bouge {
		t.Error("apprendre ce qu'on savait déjà ne doit rien changer")
	}
}

// Un dépôt adopté porte souvent le compte au lieu du nom : le registre doit
// le reconnaître aussi.
func TestLeCompteDesigneAussiLaPersonne(t *testing.T) {
	set, _, _ := appliquer(t, registry.Empty(),
		registry.Learn(personne("Jean-Luc Picard", "jlpicard")))
	if fiche, trouve := set.Resolve("jlpicard"); !trouve || fiche.FullName != "Jean-Luc Picard" {
		t.Fatalf("Resolve(compte) = %+v, %v", fiche, trouve)
	}
}

// GitHub ajoute « -1 » à un nom de dépôt déjà pris. La marque se pose à la fin,
// donc sur le slug quand c'est lui qui termine le nom : « emilie-cote-1 » reste
// le dépôt d'Émilie.
func TestLaMarqueDeDoublonNeFaitPasQuelquUnDAutre(t *testing.T) {
	set, _, _ := appliquer(t, registry.Empty(),
		registry.Learn(personne("Émilie Côté", "ecote")))
	if fiche, trouve := set.Resolve("emilie-cote-1"); !trouve || fiche.Username != "ecote" {
		t.Fatalf("Resolve = %+v, %v", fiche, trouve)
	}
	if _, trouve := set.Resolve("inconnu-1"); trouve {
		t.Error("un slug qui ne mène à personne ne doit pas être inventé")
	}
}

// Un slug appris explicitement — un dépôt adopté, nommé autrement — rattache
// lui aussi les dépôts à leur personne.
func TestUnSlugApprisRattacheLesDepots(t *testing.T) {
	set, _, _ := appliquer(t, registry.Empty(),
		registry.Learn(personne("Aminata Diallo", "aminata-d")))
	set, bouge, err := appliquer(t, set, registry.LearnSlug("aminata-d", "diallo-aminata"))
	if err != nil {
		t.Fatal(err)
	}
	if !bouge {
		t.Fatal("apprendre un slug inconnu doit changer le registre")
	}
	if fiche, trouve := set.Resolve("diallo-aminata"); !trouve || fiche.Username != "aminata-d" {
		t.Fatalf("Resolve = %+v, %v", fiche, trouve)
	}
	// Le nom n'a pas bougé : apprendre un slug n'est pas renommer.
	if set.Name("aminata-d") != "Aminata Diallo" {
		t.Errorf("nom = %q", set.Name("aminata-d"))
	}
}

// Le fichier doit se relire sur github.com : trié, indenté pareil, stable d'une
// écriture à l'autre.
func TestLeFichierEstStableEtTrie(t *testing.T) {
	set, _, _ := appliquer(t, registry.Empty(), registry.Change{Learn: []registry.Student{
		registry.From(personne("Jean-Luc Picard", "jlpicard")),
		registry.From(personne("Émilie Côté", "ecote")),
		registry.From(personne("Aminata Diallo", "aminata-d")),
	}})
	premier, err := set.Encode()
	if err != nil {
		t.Fatal(err)
	}
	relu, soucis := registry.Decode(premier)
	if len(soucis) != 0 {
		t.Fatalf("relecture : %v", soucis)
	}
	second, err := relu.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if string(premier) != string(second) {
		t.Fatalf("l'aller-retour change le fichier :\n%s\n---\n%s", premier, second)
	}

	// Rangé par compte : les différences restent lisibles d'une version à l'autre.
	var lu struct {
		Version  int              `json:"version"`
		Students []map[string]any `json:"students"`
	}
	if err := json.Unmarshal(premier, &lu); err != nil {
		t.Fatal(err)
	}
	if lu.Version != registry.Version {
		t.Errorf("version écrite = %d", lu.Version)
	}
	comptes := make([]string, 0, len(lu.Students))
	for _, fiche := range lu.Students {
		comptes = append(comptes, fiche["username"].(string))
	}
	if strings.Join(comptes, ",") != "aminata-d,ecote,jlpicard" {
		t.Fatalf("ordre = %v", comptes)
	}
	if !strings.HasSuffix(string(premier), "\n") {
		t.Error("le fichier doit finir par un saut de ligne")
	}
}

// Quelqu'un modifiera ce fichier à la main sur github.com. Une fiche mal écrite
// doit se signaler, pas priver toute l'organisation de ses noms.
func TestUneFicheIllisibleNePrivePasDesAutres(t *testing.T) {
	contenu := []byte(`{
  "version": 1,
  "students": [
    {"username": "ecote", "full_name": "Émilie Côté"},
    {"username": "", "full_name": "Sans compte"},
    {"username": "jlpicard", "full_name": "Jean-Luc Picard"},
    {"username": "ECOTE", "full_name": "Doublon"}
  ]
}`)
	set, soucis := registry.Decode(contenu)
	if set.Len() != 2 {
		t.Fatalf("%d fiche(s) retenue(s) : %+v", set.Len(), set.All())
	}
	if len(soucis) != 2 {
		t.Fatalf("soucis = %v", soucis)
	}
	if set.Name("ecote") != "Émilie Côté" {
		t.Errorf("le doublon a écrasé la fiche d'origine : %q", set.Name("ecote"))
	}
}

// Un registre écrit par une version ultérieure de l'outil se lit quand même,
// mais le dit.
func TestUneVersionPlusRecenteSeSignale(t *testing.T) {
	set, soucis := registry.Decode([]byte(
		`{"version": 99, "students": [{"username":"ecote","full_name":"Émilie Côté"}]}`))
	if set.Len() != 1 {
		t.Fatalf("%d fiche(s)", set.Len())
	}
	if len(soucis) != 1 || !strings.Contains(soucis[0], "version 99") {
		t.Fatalf("soucis = %v", soucis)
	}
}

// Un fichier vide, absent ou illisible n'est pas une panne : c'est un registre
// qu'on n'a pas encore amorcé.
func TestUnRegistreVideNEstPasUneErreur(t *testing.T) {
	if set, _ := registry.Decode(nil); set.Len() != 0 {
		t.Error("un contenu vide doit donner un registre vide")
	}
	set, soucis := registry.Decode([]byte("{ ceci n'est pas du JSON"))
	if set.Len() != 0 || len(soucis) != 1 {
		t.Fatalf("set = %+v, soucis = %v", set.All(), soucis)
	}
}

// Une fiche sans nom complet désigne quand même quelqu'un : son compte suffit
// à dire que le dépôt est celui d'une personne connue, et non d'un slug
// orphelin. C'est ce qui permet ensuite de la nommer ou de la déplacer.
func TestUneFicheSansNomDesigneQuandMemeQuelquun(t *testing.T) {
	set, _, err := appliquer(t, registry.Empty(),
		registry.Learn(personne("", "aleksilepaj")))
	if err != nil {
		t.Fatal(err)
	}
	trouvee, connue := set.Lookup("aleksilepaj")
	if !connue {
		t.Fatal("le compte doit se retrouver même sans nom complet")
	}
	if trouvee.Username != "aleksilepaj" || trouvee.FullName != "" {
		t.Fatalf("personne = %+v", trouvee)
	}
	// La marque de doublon de GitHub n'en fait pas quelqu'un d'autre.
	if trouvee, connue := set.Lookup("aleksilepaj-1"); !connue ||
		trouvee.Username != "aleksilepaj" {
		t.Errorf("« aleksilepaj-1 » = %+v, %v", trouvee, connue)
	}
	// Ce que le registre ignore reste ignoré : un slug n'invente personne.
	if _, connue := set.Lookup("emilie-cote"); connue {
		t.Error("un slug inconnu ne doit désigner personne")
	}
}

// appliquer déroule un changement hors réseau, comme le magasin le fera.
func appliquer(t *testing.T, set *registry.Set, change registry.Change) (*registry.Set, bool, error) {
	t.Helper()
	return set.With(change, "2026-09-06")
}
