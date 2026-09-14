package app_test

import (
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/registry"
)

// La fiche déroule le passage de quelqu'un, de la session la plus récente à la
// plus ancienne : la même chose qu'au navigateur, écrite au terminal.
func TestLaFicheDUnUtilisateurAuTerminal(t *testing.T) {
	h := college(t)
	h.Options.StudentsRequested = false
	h.Options.User = "emilie-cote"

	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("Émilie Côté — @emilie-cote", "étudiant",
		"Son passage dans l'organisation",
		// Les trois cours, chacun avec ce qu'il en reste.
		"h27.5n6.02", "a26.4w6.01", "a26.5n6.01",
		"Hiver 2027", "Automne 2026", "projet", "tp1")
}

// Un compte que rien ne connaît rend quand même une fiche : il existe sur
// GitHub, il n'a simplement rien fait ici.
func TestLaFicheDUnCompteInconnuAuTerminal(t *testing.T) {
	h := college(t)
	h.Options.StudentsRequested = false
	h.Options.User = "personne-du-tout"

	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("@personne-du-tout",
		"ne connaît pas @personne-du-tout",
		"Aucun cours suivi ni donné")
}

// Coopter écrit le rôle au registre. Le premier enseignant se déclare :
// personne ne peut le reconnaître avant lui.
func TestCoopterUnEnseignantAuTerminal(t *testing.T) {
	h := college(t)
	h.Options.StudentsRequested = false
	h.Options.User = "emilie-cote"
	h.Options.TeacherSet, h.Options.Teacher = true, true

	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("@emilie-cote est désormais enseignant")

	contenu := h.State.Files("acme/"+registry.RepoName, registry.Branch)[registry.UsersFile]
	if contenu == "" {
		t.Fatal("le registre n'a rien reçu")
	}
	set, soucis := registry.Decode([]byte(contenu))
	if len(soucis) > 0 {
		t.Fatalf("registre = %v", soucis)
	}
	if !set.Teaches("emilie-cote") {
		t.Fatalf("le rôle n'a pas été écrit : %+v", set.All())
	}
}

// Un compte qui n'enseigne pas ne coopte personne. Ce refus ne protège pas le
// registre — GitHub s'en charge — mais il le dit dans les mots de l'outil.
func TestUnNonEnseignantNeCoopteAuTerminal(t *testing.T) {
	h := college(t)
	h.Options.StudentsRequested = false
	h.Options.User = "emilie-cote"
	h.Options.TeacherSet, h.Options.Teacher = true, true
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("première cooptation : code = %d\n%s", code, h.texte())
	}

	// « prof » n'est toujours pas enseignant, et l'organisation en a un.
	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.User = "jlpicard"
	suivant.Options.TeacherSet, suivant.Options.Teacher = true, true
	if code := suivant.muet(); code == app.ExitOK {
		t.Fatalf("la cooptation devait être refusée\n%s", suivant.texte())
	}
	suivant.contient("Seul un enseignant")
}

// ------------------------------------------------------------ cloisonnement

// Cloisonner un groupe crée son équipe enseignante et lui donne ses dépôts —
// les siens seulement.
func TestCloisonnerUnGroupeAuTerminal(t *testing.T) {
	h := college(t)
	h.Options.StudentsRequested = false
	h.Options.User = "prof"
	h.Options.TeacherSet, h.Options.Teacher = true, true
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("cooptation : code = %d\n%s", code, h.texte())
	}

	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.User = ""
	suivant.Options.TeacherSet = false
	suivant.Options.ManageRequested, suivant.Options.Manage = true, "a26.5n6.01"
	suivant.Options.TeachersOn = true
	suivant.Options.Teachers = []string{"prof"}
	suivant.Options.Yes = true

	if code := suivant.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	suivant.contient("a26.5n6.01.enseignants", "est cloisonné derrière")

	partages := suivant.State.TeamRepos["acme/a26-5n6-01-enseignants"]
	if len(partages) != 2 {
		t.Fatalf("dépôts partagés = %v", partages)
	}
	for nom, droit := range partages {
		if droit != "admin" {
			t.Errorf("%s : droit = %q", nom, droit)
		}
	}
	// Le groupe voisin n'appartient pas à cette équipe : c'est tout l'objet.
	if _, partage := partages["acme/h27.5n6.02.tp1.emilie-cote"]; partage {
		t.Error("un dépôt d'un autre groupe est allé à cette équipe")
	}
}

// On n'inscrit à l'équipe d'un groupe que quelqu'un que le registre déclare
// enseignant : « is_teacher » ne donne rien, il autorise à donner.
func TestOnNeCloisonnePasDerriereUnEtudiantAuTerminal(t *testing.T) {
	h := college(t)
	h.Options.StudentsRequested = false
	h.Options.User = "prof"
	h.Options.TeacherSet, h.Options.Teacher = true, true
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("cooptation : code = %d\n%s", code, h.texte())
	}

	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.User = ""
	suivant.Options.TeacherSet = false
	suivant.Options.ManageRequested, suivant.Options.Manage = true, "a26.5n6.01"
	suivant.Options.TeachersOn = true
	suivant.Options.Teachers = []string{"prof", "emilie-cote"}
	suivant.Options.Yes = true

	if code := suivant.muet(); code == app.ExitOK {
		t.Fatalf("inscrire un étudiant devait être refusé\n%s", suivant.texte())
	}
	suivant.contient("emilie-cote")
}

// Un cours donné remonte dans la fiche de celui qui l'a donné : c'est ce qui
// permet de chercher ce qu'un collègue a déjà enseigné.
func TestUnCoursDonneRemonteDansLaFicheAuTerminal(t *testing.T) {
	h := college(t)
	h.Options.StudentsRequested = false
	h.Options.User = "prof"
	h.Options.TeacherSet, h.Options.Teacher = true, true
	if code := h.muet(); code != app.ExitOK {
		t.Fatalf("cooptation : code = %d\n%s", code, h.texte())
	}

	cloisonne := nouveauDansLeMemeDossier(t, h)
	cloisonne.Options.User = ""
	cloisonne.Options.TeacherSet = false
	cloisonne.Options.ManageRequested, cloisonne.Options.Manage = true, "a26.5n6.01"
	cloisonne.Options.TeachersOn = true
	cloisonne.Options.Teachers = []string{"prof"}
	cloisonne.Options.Yes = true
	if code := cloisonne.muet(); code != app.ExitOK {
		t.Fatalf("cloisonnement : code = %d\n%s", code, cloisonne.texte())
	}

	fiche := nouveauDansLeMemeDossier(t, h)
	fiche.Options.ManageRequested, fiche.Options.Manage = false, ""
	fiche.Options.TeachersOn = false
	fiche.Options.User = "prof"
	if code := fiche.muet(); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, fiche.texte())
	}
	fiche.contient("enseignant", "a26.5n6.01", "a donné ce cours")
}
