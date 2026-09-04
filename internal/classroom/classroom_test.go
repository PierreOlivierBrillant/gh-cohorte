package classroom_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
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
		Students: etudiants,
		Defaults: classroom.DefaultsFrom(config.Default()),
	}
}

// heritage déclare un groupe resté à l'ancienne nomenclature.
func heritage(prefixe string, comptes ...string) classroom.Classroom {
	return classroom.Classroom{
		Org: "acme", LegacyPrefix: prefixe,
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

func TestGroupeAQuatreNiveauxRedevientLisible(t *testing.T) {
	// Écrit tel que la version sans session l'enregistrait : un cours et un
	// groupe, mais pas de session.
	chemin := filepath.Join(t.TempDir(), "groupes.json")
	contenu := `{"version":1,"classrooms":[{"id":"abc","name":"5n6 a26-01",` +
		`"org":"acme","session":"","course":"5n6","group":"a26-01",` +
		`"students":[{"full_name":"Émilie Côté","username":"emilie-cote"}],` +
		`"defaults":{}}]}`
	if err := os.WriteFile(chemin, []byte(contenu), 0o600); err != nil {
		t.Fatalf("écriture : %v", err)
	}

	cours, ok := classroom.Open(chemin).Find("acme", "5n6.a26-01")
	if !ok {
		t.Fatal("groupe introuvable")
	}
	if !cours.Legacy() || cours.Scope() != "5n6.a26-01" {
		t.Fatalf("portée %q (hérité : %v)", cours.Scope(), cours.Legacy())
	}

	inventaire := depots(
		"5n6.a26-01.tp1.emilie-cote", "5n6.a26-01.travailsession.emilie-cote",
		"5n6.a26-01.tp1.inconnu", "4w6.a26-01.tp1.emilie-cote",
	)
	travaux := cours.Assignments(inventaire)
	if len(travaux) != 2 {
		t.Fatalf("travaux trouvés : %v", noms(travaux))
	}
	for _, travail := range travaux {
		attendu := 1
		if travail.Name == "tp1" {
			attendu = 2 // le dépôt « inconnu » compte, sans être inscrit
		}
		if travail.Repos != attendu || travail.Students != 1 {
			t.Fatalf("« %s » : %+v", travail.Name, travail)
		}
	}

	id := cours.AssignmentID("tp1")
	if id != "5n6.a26-01.tp1" {
		t.Fatalf("identifiant %q", id)
	}
	if depots := cours.Repos(id, inventaire); len(depots) != 2 {
		t.Fatalf("dépôts du travail : %+v", depots)
	}
	if servis := cours.Served(id, inventaire); !servis["emilie-cote"] {
		t.Fatalf("servis : %+v", servis)
	}
	student, inscrit := cours.StudentOf("5n6.a26-01.tp1.emilie-cote")
	if !inscrit || student.Username != "emilie-cote" {
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

func TestCandidatsAQuatreNiveauxSontHerites(t *testing.T) {
	proposes := classroom.Candidates(depots(
		"5n6.a26-01.tp1.emilie-cote", "5n6.a26-01.travailsession.emilie-cote",
		"a26.4w6.01.tp1.jean-luc-picard",
	))
	if len(proposes) != 2 {
		t.Fatalf("candidats : %+v", proposes)
	}
	// Celui de la nomenclature courante passe devant.
	if proposes[0].Legacy || proposes[0].Prefix != "a26.4w6.01" {
		t.Fatalf("premier candidat : %+v", proposes[0])
	}
	ancien := proposes[1]
	if !ancien.Legacy || ancien.Prefix != "5n6.a26-01" || ancien.Repos != 2 {
		t.Fatalf("candidat hérité : %+v", ancien)
	}
	if strings.Join(ancien.Assignments, ",") != "tp1,travailsession" {
		t.Fatalf("travaux du candidat : %v", ancien.Assignments)
	}
	if strings.Join(ancien.Students, ",") != "emilie-cote" {
		t.Fatalf("étudiants du candidat : %v", ancien.Students)
	}
}

func TestMagasinEcritEtRelit(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "groupes.json")
	magasin := classroom.Open(chemin)

	cree, err := magasin.Save(groupe("a26", "5n6", "01", cohorte))
	if err != nil {
		t.Fatalf("enregistrement : %v", err)
	}
	// Un groupe se désigne par sa place, pas par un numéro inventé ici.
	if cree.Scope() != "a26.5n6.01" {
		t.Fatalf("place %q", cree.Scope())
	}

	info, err := os.Stat(chemin)
	if err != nil {
		t.Fatalf("fichier absent : %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("permissions %o, attendu 600", mode)
	}

	relu := classroom.Open(chemin)
	retrouve, present := relu.Find("acme", "a26.5n6.01")
	if !present || retrouve.Session != "a26" || retrouve.Course != "5n6" ||
		retrouve.Group != "01" || len(retrouve.Students) != 3 {
		t.Fatalf("groupe relu : %+v", retrouve)
	}
	// La casse ne distingue pas deux dépôts : elle ne distingue pas deux places.
	if _, present := relu.Find("ACME", "A26.5N6.01"); !present {
		t.Fatal("la place devrait se retrouver sans égard à la casse")
	}

	retrouve.Defaults.Template = "acme/modele"
	if _, err := relu.Save(retrouve); err != nil {
		t.Fatalf("mise à jour : %v", err)
	}
	encore := classroom.Open(chemin).List("acme")
	if len(encore) != 1 || encore[0].Defaults.Template != "acme/modele" {
		t.Fatalf("mise à jour non enregistrée : %+v", encore)
	}

	if err := relu.Forget("acme", "a26.5n6.01"); err != nil {
		t.Fatalf("oubli : %v", err)
	}
	if reste := classroom.Open(chemin).List("acme"); len(reste) != 0 {
		t.Fatalf("groupe encore retenu : %+v", reste)
	}
}

func TestMagasinSuitUnGroupeQuiChangeDePlace(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "groupes.json")
	magasin := classroom.Open(chemin)
	if _, err := magasin.Save(groupe("a26", "5n6", "01", cohorte)); err != nil {
		t.Fatalf("enregistrement : %v", err)
	}

	// Les dépôts viennent d'être renommés : la liste doit les suivre.
	if _, err := magasin.Move("acme", "a26.5n6.01", groupe("h27", "5n6", "02", cohorte)); err != nil {
		t.Fatalf("déplacement : %v", err)
	}
	relu := classroom.Open(chemin)
	if _, present := relu.Find("acme", "a26.5n6.01"); present {
		t.Fatal("l'ancienne place ne devrait plus rien retenir")
	}
	arrivee, present := relu.Find("acme", "h27.5n6.02")
	if !present || len(arrivee.Students) != 3 {
		t.Fatalf("nouvelle place : %+v", arrivee)
	}
}

func TestMagasinNePartagePasSesTranches(t *testing.T) {
	magasin := classroom.Open(filepath.Join(t.TempDir(), "groupes.json"))
	if _, err := magasin.Save(groupe("a26", "5n6", "01", personnes("", "emilie-cote"))); err != nil {
		t.Fatalf("enregistrement : %v", err)
	}

	// Modifier ce qu'on a reçu ne doit pas écrire dans le magasin.
	rendu, _ := magasin.Find("acme", "a26.5n6.01")
	rendu.Students[0].FullName = "Écrit par mégarde"

	encore, _ := magasin.Find("acme", "a26.5n6.01")
	if encore.Students[0].FullName != "" {
		t.Fatalf("le magasin a été modifié dans son dos : %q", encore.Students[0].FullName)
	}
}

func TestMagasinNeGardeQuUneListeParPlace(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "groupes.json")
	magasin := classroom.Open(chemin)
	if _, err := magasin.Save(groupe("a26", "5n6", "01", cohorte)); err != nil {
		t.Fatalf("premier enregistrement : %v", err)
	}
	// Réenregistrer la même place remplace, sans créer de doublon : deux
	// listes pour les mêmes dépôts n'auraient aucun sens.
	if _, err := magasin.Save(groupe("a26", "5n6", "01", personnes("Aminata Diallo", "aminata-d"))); err != nil {
		t.Fatalf("second enregistrement : %v", err)
	}
	retenus := magasin.List("acme")
	if len(retenus) != 1 || len(retenus[0].Students) != 1 {
		t.Fatalf("groupes retenus : %+v", retenus)
	}
	// Un autre groupe du même cours reste un autre groupe.
	if _, err := magasin.Save(groupe("a26", "5n6", "02", cohorte)); err != nil {
		t.Fatalf("second groupe du cours : %v", err)
	}
	if len(magasin.List("acme")) != 2 {
		t.Fatalf("groupes retenus : %+v", magasin.List("acme"))
	}
}

func TestPlaceOuvreUnGroupeJamaisDeclare(t *testing.T) {
	// Un groupe existe parce que ses dépôts existent : il s'ouvre sans avoir
	// été déclaré nulle part.
	cours, err := classroom.AtScope("acme", "a26.5n6.01", classroom.DefaultsFrom(config.Default()))
	if err != nil {
		t.Fatalf("place : %v", err)
	}
	if cours.Legacy() || cours.Session != "a26" || cours.Course != "5n6" || cours.Group != "01" {
		t.Fatalf("groupe composé : %+v", cours)
	}
	if cours.Label() != "Groupe 01" || cours.SessionName() != "Automne 2026" {
		t.Fatalf("libellés : %q / %q", cours.Label(), cours.SessionName())
	}

	// Ce qui ne suit pas la nomenclature reste un préfixe hérité.
	herite, err := classroom.AtScope("acme", "a26-5n6", classroom.DefaultsFrom(config.Default()))
	if err != nil || !herite.Legacy() || herite.LegacyPrefix != "a26-5n6" {
		t.Fatalf("préfixe hérité : %+v (%v)", herite, err)
	}
	// Et un gabarit d'adoption se reconnaît à ses champs.
	adopte, err := classroom.AtScope("acme", "projet-{assignment}-{student}",
		classroom.DefaultsFrom(config.Default()))
	if err != nil || adopte.LegacyPattern == "" {
		t.Fatalf("gabarit : %+v (%v)", adopte, err)
	}
}

func TestPlacesLuesDansLesDepots(t *testing.T) {
	places := classroom.Places(depots(
		"a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp2.emilie-cote",
		"a26.4w6.02.tp1.jlpicard",
		"a26-5n6-tp1-jlpicard", // nomenclature dépassée : rien à en tirer
	))
	if strings.Join(places, ",") != "a26.4w6.02,a26.5n6.01" {
		t.Fatalf("places : %v", places)
	}
}

func TestMagasinRefuseUnSeparateurDansUnNiveau(t *testing.T) {
	magasin := classroom.Open(filepath.Join(t.TempDir(), "groupes.json"))

	// Le point saisi dans un champ est slugifié, jamais conservé : le groupe
	// « 01.b » devient « 01-b » et la nomenclature garde ses cinq niveaux.
	cree, err := magasin.Save(groupe("a26", "5n6", "01.b", cohorte))
	if err != nil {
		t.Fatalf("enregistrement : %v", err)
	}
	if cree.Group != "01-b" {
		t.Fatalf("groupe enregistré : %q", cree.Group)
	}
	if strings.Count(cree.Scope(), ".") != 2 {
		t.Fatalf("portée mal découpée : %q", cree.Scope())
	}
}

func TestSessionsDesGroupesRetenus(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "groupes.json")
	magasin := classroom.Open(chemin)
	for _, cours := range []classroom.Classroom{
		groupe("a26", "5n6", "01", cohorte),
		groupe("a26", "4w6", "01", cohorte),
		groupe("h27", "5n6", "01", cohorte),
	} {
		if _, err := magasin.Save(cours); err != nil {
			t.Fatalf("enregistrement : %v", err)
		}
	}

	// Le nom long ne s'écrit nulle part : il se déduit du nom court.
	sessions := classroom.Open(chemin).Sessions("acme")
	if len(sessions) != 2 || sessions[0].Short != "a26" ||
		sessions[0].Name != "Automne 2026" || sessions[1].Name != "Hiver 2027" {
		t.Fatalf("sessions : %+v", sessions)
	}
	// Une autre organisation n'a rien à voir ici.
	if autres := classroom.Open(chemin).Sessions("college"); len(autres) != 0 {
		t.Fatalf("sessions d'une autre organisation : %+v", autres)
	}
}

func TestNomDeSessionDeduitDeLaSaison(t *testing.T) {
	cas := map[string]string{
		"a26": "Automne 2026", "h27": "Hiver 2027",
		"e27": "Été 2027", "p28": "Printemps 2028",
		"A26": "Automne 2026",
		// Ce qui ne suit pas la convention se rend tel quel.
		"x26": "x26", "automne": "automne", "a2026": "a2026",
	}
	for court, attendu := range cas {
		if nom := naming.SessionLabel(court); nom != attendu {
			t.Errorf("%s → %q, attendu %q", court, nom, attendu)
		}
	}
}
