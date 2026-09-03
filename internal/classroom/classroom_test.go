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

// personnes construit une liste d'étudiants « nom complet / compte ».
func personnes(couples ...string) []roster.Person {
	liste := make([]roster.Person, 0, len(couples)/2)
	for index := 0; index+1 < len(couples); index += 2 {
		liste = append(liste, roster.Person{FullName: couples[index], Username: couples[index+1]})
	}
	return liste
}

// groupe déclare un groupe de la nomenclature courante.
func groupe(session, cours, section string, etudiants []roster.Person) classroom.Classroom {
	return classroom.Classroom{
		Org: "acme", Session: session, Course: cours, Group: section,
		Name:     cours + " " + section,
		Students: etudiants,
		Defaults: classroom.DefaultsFrom(config.Default()),
	}
}

// heritage déclare un groupe resté à l'ancienne nomenclature.
func heritage(prefixe string, comptes ...string) classroom.Classroom {
	return classroom.Classroom{
		Org: "acme", LegacyPrefix: prefixe, Name: prefixe,
		Students: classroom.StudentsOf(comptes),
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

var cohorte = personnes(
	"Émilie Côté", "emilie-cote",
	"Jean-Luc Picard", "jlpicard",
	"Aminata Diallo", "aminata-d",
)

// ------------------------------------------------------------ identifiants

func TestIdentifiantDeTravail(t *testing.T) {
	cours := groupe("a26", "5n6", "01", cohorte)
	if id := cours.AssignmentID("tp1"); id != "a26.5n6.01.tp1" {
		t.Fatalf("identifiant %q", id)
	}
	if court := cours.ShortName("a26.5n6.01.travail-session"); court != "travail-session" {
		t.Fatalf("nom court %q", court)
	}
	if !cours.Owns("a26.5n6.01.tp1") || cours.Owns("a26.5n6.02.tp1") || cours.Owns("a26.5n6.01") {
		t.Fatal("le périmètre du groupe est mal délimité")
	}
	if cours.Scope() != "a26.5n6.01" {
		t.Fatalf("portée %q", cours.Scope())
	}
}

func TestPlusieursGroupesDansUnMemeCours(t *testing.T) {
	inventaire := depots(
		"a26.5n6.01.tp1.emilie-cote",
		"a26.5n6.02.tp1.jean-luc-picard",
	)
	premier := groupe("a26", "5n6", "01", cohorte)
	second := groupe("a26", "5n6", "02", cohorte)

	if travaux := premier.Assignments(inventaire); len(travaux) != 1 ||
		travaux[0].Repos != 1 || travaux[0].Students != 1 {
		t.Fatalf("groupe 01 : %+v", travaux)
	}
	if travaux := second.Assignments(inventaire); len(travaux) != 1 ||
		travaux[0].Repos != 1 || travaux[0].Students != 1 {
		t.Fatalf("groupe 02 : %+v", travaux)
	}
}

// ---------------------------------------------------------------- travaux

func TestTravauxDuGroupe(t *testing.T) {
	inventaire := depots(
		"a26.5n6.01.travail-session.emilie-cote", "a26.5n6.01.travail-session.jean-luc-picard",
		"a26.5n6.01.tp1.emilie-cote",
		"a26.4w6.01.tp1.jean-luc-picard",
		"notes-du-cours",
	)
	cours := groupe("a26", "5n6", "01", cohorte)

	travaux := cours.Assignments(inventaire)
	if len(travaux) != 2 {
		t.Fatalf("travaux trouvés : %v", noms(travaux))
	}
	trouves := map[string]classroom.Assignment{}
	for _, travail := range travaux {
		trouves[travail.Name] = travail
	}
	if trouves["travail-session"].Repos != 2 || trouves["travail-session"].Students != 2 {
		t.Fatalf("travail-session : %+v", trouves["travail-session"])
	}
	// Un travail distribué à une seule personne se lit aussi bien : le
	// séparateur réservé n'exige pas deux dépôts pour conclure.
	if trouves["tp1"].Repos != 1 || trouves["tp1"].Students != 1 {
		t.Fatalf("tp1 : %+v", trouves["tp1"])
	}
	// Le cours voisin ne déborde pas.
	if _, present := trouves["4w6"]; present {
		t.Fatalf("un dépôt d'un autre cours a été rattaché : %v", noms(travaux))
	}
}

func TestDepotHorsListeCompteApart(t *testing.T) {
	inventaire := depots(
		"a26.5n6.01.tp1.emilie-cote",
		"a26.5n6.01.tp1.jean-luc-picard",
		"a26.5n6.01.tp1.visiteur-inconnu",
	)
	cours := groupe("a26", "5n6", "01", cohorte)

	travaux := cours.Assignments(inventaire)
	if len(travaux) != 1 {
		t.Fatalf("travaux trouvés : %v", noms(travaux))
	}
	if travaux[0].Students != 2 || travaux[0].Others != 1 {
		t.Fatalf("comptage : %+v", travaux[0])
	}
}

func TestServedRepereLesEtudiantsDejaServis(t *testing.T) {
	inventaire := depots("a26.5n6.01.tp1.emilie-cote")
	cours := groupe("a26", "5n6", "01", cohorte)

	servis := cours.Served("a26.5n6.01.tp1", inventaire)
	if !servis["emilie-cote"] || servis["jlpicard"] {
		t.Fatalf("étudiants servis : %v", servis)
	}
}

func TestDepotRattacheASonEtudiant(t *testing.T) {
	cours := groupe("a26", "5n6", "01", cohorte)

	student, inscrit := cours.StudentOf("a26.5n6.01.tp1.jean-luc-picard")
	if !inscrit || student.Username != "jlpicard" {
		t.Fatalf("étudiant retrouvé : %+v (%v)", student, inscrit)
	}
	if _, inscrit := cours.StudentOf("a26.5n6.01.tp1.inconnu"); inscrit {
		t.Fatal("un dépôt hors liste a été rattaché")
	}
}

func TestNomCompletManquantEmpecheDeNommer(t *testing.T) {
	cours := groupe("a26", "5n6", "01", append(
		personnes("Émilie Côté", "emilie-cote"),
		roster.Person{Username: "sans-nom"},
	))
	incomplets := cours.MissingNames()
	if len(incomplets) != 1 || incomplets[0].Username != "sans-nom" {
		t.Fatalf("étudiants incomplets : %+v", incomplets)
	}
}

func TestReglagesDuTravailReprennentLeGroupe(t *testing.T) {
	cours := groupe("a26", "5n6", "01", cohorte)
	cours.Defaults.Visibility = "public"
	cours.Defaults.Template = "acme/modele"

	reglages := cours.Settings("tp1")
	if reglages.Assignment != "a26.5n6.01.tp1" {
		t.Fatalf("travail %q", reglages.Assignment)
	}
	if reglages.NamePattern != classroom.NamePattern {
		t.Fatalf("gabarit %q : il n'est pas réglable", reglages.NamePattern)
	}
	if reglages.Org != "acme" || reglages.Visibility != "public" ||
		reglages.Template != "acme/modele" {
		t.Fatalf("réglages : %+v", reglages)
	}
}

// ------------------------------------------------------- ancienne nomenclature

func TestGroupeHeriteResteLisible(t *testing.T) {
	inventaire := depots(
		"a26-5n6-travailsession-emilie-cote", "a26-5n6-travailsession-jlpicard",
		"a26-5n6-tp1-emilie-cote", "a26-5n6-tp1-jlpicard",
		"a26-4w6-tp1-jlpicard", "a26-4w6-tp1-aminata-d",
	)
	cours := heritage("a26-5n6", "emilie-cote", "jlpicard")
	if !cours.Legacy() {
		t.Fatal("le groupe devrait être reconnu comme hérité")
	}

	travaux := cours.Assignments(inventaire)
	if len(travaux) != 2 {
		t.Fatalf("travaux trouvés : %v", noms(travaux))
	}
	for _, travail := range travaux {
		if travail.Repos != 2 || travail.Students != 2 {
			t.Fatalf("« %s » : %+v", travail.Name, travail)
		}
	}
}

func TestGroupeHeriteRattacheSesDepotsParLeCompte(t *testing.T) {
	cours := heritage("a26-5n6", "emilie-cote", "jlpicard")
	cours.Students = personnes("Émilie Côté", "emilie-cote", "Jean-Luc Picard", "jlpicard")

	student, inscrit := cours.StudentOf("a26-5n6-tp1-jlpicard")
	if !inscrit || student.Username != "jlpicard" {
		t.Fatalf("étudiant retrouvé : %+v (%v)", student, inscrit)
	}
}

// --------------------------------------------------------------- candidats

func TestCandidatsDeLaNouvelleNomenclature(t *testing.T) {
	inventaire := depots(
		"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.jean-luc-picard",
		"a26.5n6.02.tp1.aminata-diallo",
	)
	candidats := classroom.Candidates(inventaire)
	if len(candidats) != 2 {
		t.Fatalf("candidats : %+v", candidats)
	}
	trouves := map[string]classroom.Candidate{}
	for _, candidat := range candidats {
		if candidat.Legacy {
			t.Fatalf("un candidat de la nouvelle nomenclature est marqué hérité : %+v", candidat)
		}
		trouves[candidat.Prefix] = candidat
	}
	premier, present := trouves["a26.5n6.01"]
	if !present || premier.Session != "a26" || premier.Course != "5n6" || premier.Group != "01" {
		t.Fatalf("candidat : %+v", premier)
	}
	if premier.Repos != 2 || len(premier.Students) != 2 {
		t.Fatalf("comptage : %+v", premier)
	}
}

func TestCandidatsHeritesSignalesCommeTels(t *testing.T) {
	inventaire := depots(
		"a26-5n6-travailsession-emilie-cote", "a26-5n6-travailsession-jlpicard",
		"a26.5n6.02.tp1.aminata-diallo", "a26.5n6.02.tp1.emilie-cote",
	)
	candidats := classroom.Candidates(inventaire)

	trouves := map[string]classroom.Candidate{}
	for _, candidat := range candidats {
		trouves[candidat.Prefix] = candidat
	}
	if ancien, present := trouves["a26-5n6"]; !present || !ancien.Legacy {
		t.Fatalf("le préfixe hérité n'est pas signalé : %+v", candidats)
	}
	if nouveau, present := trouves["a26.5n6.02"]; !present || nouveau.Legacy {
		t.Fatalf("le candidat courant est mal classé : %+v", candidats)
	}
	// Les candidats de la nomenclature courante passent devant.
	if candidats[0].Legacy {
		t.Fatalf("ordre des candidats : %+v", candidats)
	}
}

// ----------------------------------------------------------------- magasin

func TestMagasinEcritEtRelit(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "groupes.json")
	magasin := classroom.Open(chemin)

	cree, err := magasin.Add(groupe("a26", "5n6", "01", cohorte))
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
	if !present || retrouve.Session != "a26" || retrouve.Course != "5n6" || retrouve.Group != "01" ||
		len(retrouve.Students) != 3 {
		t.Fatalf("groupe relu : %+v", retrouve)
	}

	retrouve.Name = "5N6 — Automne 2026, groupe 01"
	if _, err := relu.Update(retrouve); err != nil {
		t.Fatalf("mise à jour : %v", err)
	}
	if encore := classroom.Open(chemin); len(encore.List()) != 1 ||
		encore.List()[0].Name != "5N6 — Automne 2026, groupe 01" {
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
	cree, err := magasin.Add(groupe("a26", "5n6", "01", personnes("", "emilie-cote")))
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

func TestMagasinRefuseDeuxGroupesAuMemeEndroit(t *testing.T) {
	magasin := classroom.Open(filepath.Join(t.TempDir(), "groupes.json"))
	if _, err := magasin.Add(groupe("a26", "5n6", "01", cohorte)); err != nil {
		t.Fatalf("premier ajout : %v", err)
	}
	_, err := magasin.Add(groupe("a26", "5n6", "01", cohorte))
	if err == nil {
		t.Fatal("le doublon a été accepté")
	}
	// Un autre groupe du même cours reste possible.
	if _, err := magasin.Add(groupe("a26", "5n6", "02", cohorte)); err != nil {
		t.Fatalf("second groupe du cours : %v", err)
	}
}

func TestMagasinRefuseUnSeparateurDansUnNiveau(t *testing.T) {
	magasin := classroom.Open(filepath.Join(t.TempDir(), "groupes.json"))

	// Le point saisi dans un champ est slugifié, jamais conservé : le groupe
	// « 01.b » devient « 01-b » et la nomenclature garde ses cinq niveaux.
	cree, err := magasin.Add(groupe("a26", "5n6", "01.b", cohorte))
	if err != nil {
		t.Fatalf("ajout : %v", err)
	}
	if cree.Group != "01-b" {
		t.Fatalf("groupe enregistré : %q", cree.Group)
	}
	if strings.Count(cree.Scope(), ".") != 2 {
		t.Fatalf("portée mal découpée : %q", cree.Scope())
	}
}

func TestNomLongDeSession(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "groupes.json")
	magasin := classroom.Open(chemin)
	if _, err := magasin.Add(groupe("a26", "5n6", "01", cohorte)); err != nil {
		t.Fatalf("ajout : %v", err)
	}

	// Sans nom long, la session s'affiche par son nom court.
	if nom := magasin.SessionName("a26"); nom != "a26" {
		t.Fatalf("nom par défaut %q", nom)
	}
	if err := magasin.SetSessionName("a26", "Automne 2026"); err != nil {
		t.Fatalf("nom long : %v", err)
	}

	// Il est partagé par tous les groupes de la session, et survit au fichier.
	if _, err := magasin.Add(groupe("a26", "4w6", "01", cohorte)); err != nil {
		t.Fatalf("second groupe : %v", err)
	}
	relu := classroom.Open(chemin)
	if nom := relu.SessionName("A26"); nom != "Automne 2026" {
		t.Fatalf("nom long relu %q", nom)
	}
	sessions := relu.Sessions()
	if len(sessions) != 1 || sessions[0].Short != "a26" || sessions[0].Name != "Automne 2026" {
		t.Fatalf("sessions : %+v", sessions)
	}
}
