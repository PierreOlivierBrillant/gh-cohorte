package classroom_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// depots construit un inventaire à partir de noms.
func depots(noms ...string) []groups.RepoInfo {
	inventaire := make([]groups.RepoInfo, 0, len(noms))
	for _, nom := range noms {
		inventaire = append(inventaire, groups.RepoInfo{Name: nom, Private: true})
	}
	return inventaire
}

func etudiants(comptes ...string) []roster.Person {
	return classroom.StudentsOf(comptes)
}

func groupe(prefixe string, comptes ...string) classroom.Classroom {
	return classroom.Classroom{
		Org: "acme", Prefix: prefixe, Name: prefixe,
		Students: etudiants(comptes...),
		Defaults: classroom.DefaultsFrom(config.Default()),
	}
}

func noms(travaux []classroom.Assignment) []string {
	liste := make([]string, 0, len(travaux))
	for _, travail := range travaux {
		liste = append(liste, travail.Name)
	}
	return liste
}

// ------------------------------------------------------------ identifiants

func TestIdentifiantDeTravail(t *testing.T) {
	cours := groupe("a26-5n6")
	if id := cours.AssignmentID("tp1"); id != "a26-5n6-tp1" {
		t.Fatalf("identifiant %q", id)
	}
	if court := cours.ShortName("a26-5n6-travailsession"); court != "travailsession" {
		t.Fatalf("nom court %q", court)
	}
	if !cours.Owns("a26-5n6-tp1") || cours.Owns("a26-4w6-tp1") || cours.Owns("a26-5n6") {
		t.Fatal("le périmètre du groupe est mal délimité")
	}

	// Sans préfixe, le groupe couvre les travaux nommés à la racine.
	racine := groupe("")
	if id := racine.AssignmentID("tp1"); id != "tp1" {
		t.Fatalf("identifiant sans préfixe %q", id)
	}
	if !racine.Owns("tp1") || racine.ShortName("tp1") != "tp1" {
		t.Fatal("le groupe sans préfixe devrait tout couvrir")
	}
}

// ---------------------------------------------------------------- travaux

func TestTravauxDuGroupeExistant(t *testing.T) {
	// La nomenclature réelle : session, cours, puis nom du travail.
	inventaire := depots(
		"a26-5n6-travailsession-emilie-cote", "a26-5n6-travailsession-jlpicard",
		"a26-5n6-tp1-emilie-cote", "a26-5n6-tp1-jlpicard",
		"a26-4w6-tp1-jlpicard", "a26-4w6-tp1-aminata-d",
		"a26-notes",
	)
	cours := groupe("a26-5n6", "emilie-cote", "jlpicard")

	travaux := cours.Assignments(inventaire)
	if len(travaux) != 2 {
		t.Fatalf("travaux trouvés : %v", noms(travaux))
	}
	trouves := map[string]classroom.Assignment{}
	for _, travail := range travaux {
		trouves[travail.Name] = travail
	}
	for _, attendu := range []string{"travailsession", "tp1"} {
		travail, present := trouves[attendu]
		if !present {
			t.Fatalf("« %s » manquant : %v", attendu, noms(travaux))
		}
		if travail.Repos != 2 || travail.Students != 2 {
			t.Fatalf("« %s » : %d dépôt(s), %d étudiant(s)", attendu, travail.Repos, travail.Students)
		}
	}
	// Le cours voisin ne déborde pas.
	if _, present := trouves["a26-4w6-tp1"]; present {
		t.Fatal("un travail d'un autre cours a été rattaché")
	}
}

func TestTravailDistribueAUneSeulePersonne(t *testing.T) {
	// La détection par préfixe exige deux dépôts ; la liste des étudiants, non.
	inventaire := depots("a26-5n6-rattrapage-emilie-cote")
	cours := groupe("a26-5n6", "emilie-cote", "jlpicard")

	travaux := cours.Assignments(inventaire)
	if len(travaux) != 1 || travaux[0].Name != "rattrapage" {
		t.Fatalf("travaux trouvés : %v", noms(travaux))
	}
	if travaux[0].Students != 1 || travaux[0].Others != 0 {
		t.Fatalf("comptage : %+v", travaux[0])
	}
}

func TestDepotHorsListeCompteApart(t *testing.T) {
	inventaire := depots(
		"a26-5n6-tp1-emilie-cote", "a26-5n6-tp1-jlpicard", "a26-5n6-tp1-visiteur",
	)
	cours := groupe("a26-5n6", "emilie-cote", "jlpicard")

	travaux := cours.Assignments(inventaire)
	if len(travaux) != 1 {
		t.Fatalf("travaux trouvés : %v", noms(travaux))
	}
	if travaux[0].Students != 2 || travaux[0].Others != 1 {
		t.Fatalf("comptage : %+v", travaux[0])
	}
}

func TestTravailSansAucunEtudiantInscrit(t *testing.T) {
	// Aucun compte du groupe n'a de dépôt : la détection par préfixe rattrape.
	inventaire := depots("a26-5n6-tp9-zoe", "a26-5n6-tp9-max")
	cours := groupe("a26-5n6", "emilie-cote")

	travaux := cours.Assignments(inventaire)
	if len(travaux) != 1 || travaux[0].Name != "tp9" {
		t.Fatalf("travaux trouvés : %v", noms(travaux))
	}
	if travaux[0].Students != 0 || travaux[0].Others != 2 {
		t.Fatalf("comptage : %+v", travaux[0])
	}
}

func TestLectureAmbigueRetientLeTravailLePlusPrecis(t *testing.T) {
	// « tp1-final-cote » se lit de deux façons quand « cote » et « final-cote »
	// sont tous deux inscrits. Un seul travail doit en sortir.
	inventaire := depots("tp1-final-cote")
	cours := groupe("", "cote", "final-cote")

	travaux := cours.Assignments(inventaire)
	if len(travaux) != 1 || travaux[0].Name != "tp1-final" {
		t.Fatalf("travaux trouvés : %v", noms(travaux))
	}
}

func TestServedRepereLesEtudiantsDejaServis(t *testing.T) {
	inventaire := depots("a26-5n6-tp1-emilie-cote")
	cours := groupe("a26-5n6", "emilie-cote", "jlpicard")

	servis := cours.Served("a26-5n6-tp1", inventaire)
	if !servis["emilie-cote"] || servis["jlpicard"] {
		t.Fatalf("étudiants servis : %v", servis)
	}
}

// --------------------------------------------------------------- candidats

func TestCandidatsDeduitsDesDepots(t *testing.T) {
	inventaire := depots(
		"a26-5n6-travailsession-emilie-cote", "a26-5n6-travailsession-jlpicard",
		"a26-4w6-tp1-jlpicard", "a26-4w6-tp1-aminata-d",
	)
	candidats := classroom.Candidates(inventaire)

	trouves := map[string]classroom.Candidate{}
	for _, candidat := range candidats {
		trouves[candidat.Prefix] = candidat
	}
	pour5n6, present := trouves["a26-5n6"]
	if !present {
		t.Fatalf("candidats : %+v", candidats)
	}
	if len(pour5n6.Assignments) != 1 || pour5n6.Assignments[0] != "travailsession" {
		t.Fatalf("travaux du candidat : %+v", pour5n6)
	}
	if strings.Join(pour5n6.Students, ",") != "emilie-cote,jlpicard" {
		t.Fatalf("étudiants devinés : %v", pour5n6.Students)
	}
	if _, present := trouves["a26-4w6"]; !present {
		t.Fatalf("le second cours n'est pas proposé : %+v", candidats)
	}
}

func TestCandidatSansPrefixePourUneNomenclaturePlate(t *testing.T) {
	inventaire := depots("tp1-emilie-cote", "tp1-jlpicard", "tp2-jlpicard", "tp2-emilie-cote")
	candidats := classroom.Candidates(inventaire)

	if len(candidats) != 1 || candidats[0].Prefix != "" {
		t.Fatalf("candidats : %+v", candidats)
	}
	if len(candidats[0].Assignments) != 2 {
		t.Fatalf("travaux du candidat : %+v", candidats[0])
	}
}

// ----------------------------------------------------------------- magasin

func TestMagasinEcritEtRelit(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "groupes.json")
	magasin := classroom.Open(chemin)

	cree, err := magasin.Add(groupe("a26-5n6", "emilie-cote"))
	if err != nil {
		t.Fatalf("ajout : %v", err)
	}
	if cree.ID == "" || cree.CreatedAt == "" {
		t.Fatalf("groupe incomplet : %+v", cree)
	}

	info, err := os.Stat(chemin)
	if err != nil {
		t.Fatalf("fichier absent : %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("permissions %o, attendu 600", mode)
	}

	relu := classroom.Open(chemin)
	retrouve, present := relu.Get(cree.ID)
	if !present || retrouve.Prefix != "a26-5n6" || len(retrouve.Students) != 1 {
		t.Fatalf("groupe relu : %+v", retrouve)
	}

	retrouve.Name = "5N6 — Automne 2026"
	if _, err := relu.Update(retrouve); err != nil {
		t.Fatalf("mise à jour : %v", err)
	}
	if encore := classroom.Open(chemin); len(encore.List()) != 1 ||
		encore.List()[0].Name != "5N6 — Automne 2026" {
		t.Fatalf("mise à jour non enregistrée : %+v", encore.List())
	}

	if err := relu.Delete(cree.ID); err != nil {
		t.Fatalf("suppression : %v", err)
	}
	if reste := classroom.Open(chemin); len(reste.List()) != 0 {
		t.Fatalf("groupe encore présent : %+v", reste.List())
	}
}

func TestMagasinNePartagePasSesTranches(t *testing.T) {
	magasin := classroom.Open(filepath.Join(t.TempDir(), "groupes.json"))
	cree, err := magasin.Add(groupe("a26-5n6", "emilie-cote"))
	if err != nil {
		t.Fatalf("ajout : %v", err)
	}

	// Modifier ce qu'on a reçu ne doit pas écrire dans le magasin.
	rendu, _ := magasin.Get(cree.ID)
	rendu.Students[0].FullName = "Écrit par mégarde"

	encore, _ := magasin.Get(cree.ID)
	if encore.Students[0].FullName != "" {
		t.Fatalf("le magasin a été modifié dans son dos : %q", encore.Students[0].FullName)
	}
}

func TestMagasinRefuseDeuxGroupesSurLeMemePrefixe(t *testing.T) {
	magasin := classroom.Open(filepath.Join(t.TempDir(), "groupes.json"))
	if _, err := magasin.Add(groupe("a26-5n6")); err != nil {
		t.Fatalf("premier ajout : %v", err)
	}
	_, err := magasin.Add(groupe("a26-5n6"))
	if err == nil {
		t.Fatal("le doublon de préfixe a été accepté")
	}
	if !strings.Contains(err.Error(), "a26-5n6") {
		t.Fatalf("message : %v", err)
	}
}

func TestMagasinRefuseUnGabaritSansChampDistinctif(t *testing.T) {
	magasin := classroom.Open(filepath.Join(t.TempDir(), "groupes.json"))
	cours := groupe("a26-5n6")
	cours.Defaults.NamePattern = "{assignment}"

	if _, err := magasin.Add(cours); err == nil {
		t.Fatal("un gabarit non distinctif a été accepté")
	}
}

func TestReglagesDuTravailReprennentLeGroupe(t *testing.T) {
	cours := groupe("a26-5n6", "emilie-cote")
	cours.Defaults.Visibility = "public"
	cours.Defaults.Template = "acme/modele"

	reglages := cours.Settings("tp1")
	if reglages.Assignment != "a26-5n6-tp1" {
		t.Fatalf("travail %q", reglages.Assignment)
	}
	if reglages.Org != "acme" || reglages.Visibility != "public" ||
		reglages.Template != "acme/modele" {
		t.Fatalf("réglages : %+v", reglages)
	}
}
