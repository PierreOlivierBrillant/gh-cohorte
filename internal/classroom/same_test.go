package classroom_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-milou/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-milou/internal/registry"
	"github.com/PierreOlivierBrillant/gh-milou/internal/roster"
)

// reunis rend un registre où @Mr-Commetuveux et @commetuveuxx sont une même
// personne, désignée par le second.
func reunis(t *testing.T) *registry.Set {
	t.Helper()
	set, _, err := registry.Empty().With(registry.Learn(
		roster.Person{FullName: "Jean Commetuveux", Username: "Mr-Commetuveux"},
		roster.Person{FullName: "Jean Commetuveux", Username: "commetuveuxx"},
	), "2026-10-07")
	if err != nil {
		t.Fatal(err)
	}
	set, _, err = set.With(registry.Join("Mr-Commetuveux", "commetuveuxx"), "2026-10-07")
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// Une réunion faite au registre vaut dans chaque groupe : la personne y a ses
// deux comptes, et elle y est servie sous l'un comme sous l'autre.
func TestLesComptesReunisAuRegistreRejoignentLeGroupe(t *testing.T) {
	cours := groupe("a26", "5n6", "01", []roster.Person{{Username: "commetuveuxx"}}).
		Enrich(reunis(t), nil)

	identite, trouvee := cours.IdentityOf("Mr-Commetuveux")
	if !trouvee || strings.Join(identite.Accounts, ",") != "commetuveuxx,Mr-Commetuveux" {
		t.Fatalf("identité = %+v", identite)
	}
	if comptes := cours.Accounts()["mr-commetuveux"]; len(comptes) != 2 {
		t.Errorf("comptes = %v", comptes)
	}
}

// Deux comptes inscrits chacun de son côté ne font plus qu'une personne : rien
// n'est retiré d'aucune liste, et pourtant le groupe ne compte plus double.
func TestDeuxInscriptionsReuniesNeFontQuUnePersonne(t *testing.T) {
	cours := groupe("a26", "5n6", "01", []roster.Person{
		{FullName: "Jean Commetuveux", Username: "commetuveuxx"},
		{FullName: "Jean Commetuveux", Username: "Mr-Commetuveux"},
	}).Enrich(reunis(t), nil)

	if identites := cours.Identities(); len(identites) != 1 {
		t.Fatalf("identités = %+v", identites)
	}
	if len(cours.Students) != 2 {
		t.Errorf("la liste ne doit rien perdre : %+v", cours.Students)
	}
}

// La réunion vit au registre, pas dans la liste du poste : l'écrire ici la
// rendrait indéfaisable.
func TestLaReunionNeSEcritPasDansLaListe(t *testing.T) {
	magasin := classroom.Open(filepath.Join(t.TempDir(), classroom.FileName))
	cours := groupe("a26", "5n6", "01", []roster.Person{
		{FullName: "Jean Commetuveux", Username: "commetuveuxx", Also: []string{"jc-perso"}},
	}).Enrich(reunis(t), nil)
	if _, err := magasin.Save(cours); err != nil {
		t.Fatal(err)
	}
	relu, _ := magasin.Find("acme", cours.Scope())
	if len(relu.Students) != 1 || strings.Join(relu.Students[0].Also, ",") != "jc-perso" {
		t.Fatalf("liste écrite = %+v", relu.Students)
	}
}
