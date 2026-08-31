package app_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/app"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
)

// plusieursOrgs prépare un compte membre de plusieurs organisations, avec des
// droits différents.
func plusieursOrgs(t *testing.T) *fakegh.State {
	t.Helper()
	state := fakegh.NewState()
	state.Orgs["college"] = "Collège Untel"
	state.Orgs["labo"] = "Laboratoire"
	state.Orgs["visiteur"] = "Organisation tierce"
	state.OrgRoles = map[string]string{
		"acme":     "admin",
		"college":  "member",
		"labo":     "member",
		"visiteur": "member",
	}
	state.MembersCanCreate = map[string]bool{
		"college":  true,  // les membres peuvent créer
		"visiteur": false, // création réservée aux propriétaires
		// « labo » ne révèle rien : le droit y reste indéterminé.
	}
	return state
}

func TestChoixParmiLesOrganisations(t *testing.T) {
	h := nouveau(t, plusieursOrgs(t))
	h.Options.Org = ""

	code, scripte := h.script("gerer", "acme", "revenir")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	menu, trouve := scripte.MenuFor("Organisation GitHub")
	if !trouve {
		t.Fatal("la liste des organisations n'a pas été proposée")
	}

	libelles := menu.Labels()
	if len(libelles) != 5 { // quatre organisations et la saisie libre
		t.Fatalf("entrées = %v", libelles)
	}
	// Les organisations où tout est possible viennent en tête.
	attendus := []string{
		"acme — ACME Éducation  · propriétaire",
		"college — Collège Untel  · membre, création autorisée",
		"labo — Laboratoire  · membre",
		"visiteur — Organisation tierce  · membre, création réservée aux propriétaires",
		"Saisir un autre nom…",
	}
	for index, attendu := range attendus {
		if libelles[index] != attendu {
			t.Errorf("entrée %d = %q, attendu %q", index, libelles[index], attendu)
		}
	}
}

func TestOrganisationMemoriseePreselectionnee(t *testing.T) {
	h := nouveau(t, plusieursOrgs(t))
	h.Options.Org = ""
	if code, _ := h.script("gerer", "college", "revenir"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}

	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.Org = ""
	code, scripte := suivant.script("gerer", "", "revenir")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	menu, _ := scripte.MenuFor("Organisation GitHub")
	if menu.Default != "college" {
		t.Errorf("organisation présélectionnée = %q", menu.Default)
	}
}

func TestSaisieLibreDepuisLeMenu(t *testing.T) {
	state := plusieursOrgs(t)
	state.Orgs["invisible"] = "Organisation non listée"
	// Le compte n'y a aucune adhésion : elle n'apparaît donc pas dans la liste.
	state.OrgRoles["invisible"] = ""
	h := nouveau(t, state)
	h.Options.Org = ""

	code, scripte := h.script("gerer", "Saisir un autre nom", "invisible", "revenir")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	menu, _ := scripte.MenuFor("Organisation GitHub")
	for _, libelle := range menu.Labels() {
		if strings.Contains(libelle, "invisible") {
			t.Errorf("une organisation sans adhésion ne doit pas être listée : %v", menu.Labels())
		}
	}
	h.contient("Organisation non listée")
}

func TestRepliSurLaSaisieSansAdhesionLisible(t *testing.T) {
	state := fakegh.NewState()
	// Sans la portée « read:org », GitHub refuse la liste des adhésions.
	state.FailOn["GET /user/memberships/orgs"] = fakegh.Failure{Status: 403, Message: "Forbidden"}
	h := nouveau(t, state)
	h.Options.Org = ""

	code, scripte := h.script("gerer", "acme", "revenir")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if _, trouve := scripte.MenuFor("Organisation GitHub"); trouve {
		t.Error("sans liste d'organisations, la question doit être libre")
	}
	if question, trouve := scripte.AskedFor("Organisation GitHub"); !trouve || question.Default != "" {
		t.Errorf("question = %+v, trouvée = %v", question, trouve)
	}
	h.contient("Liste des organisations indisponible")
}

func TestListeDesOrganisationsMiseEnCache(t *testing.T) {
	state := plusieursOrgs(t)
	h := nouveau(t, state)
	h.Options.Org = ""
	if code, _ := h.script("gerer", "acme", "revenir"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	premier := state.CallCount("GET /user/memberships/orgs")
	if premier == 0 {
		t.Fatal("les adhésions devaient être demandées")
	}

	suivant := nouveauDansLeMemeDossier(t, h)
	suivant.Options.Org = ""
	if code, _ := suivant.script("gerer", "acme", "revenir"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, suivant.texte())
	}
	if state.CallCount("GET /user/memberships/orgs") != premier {
		t.Errorf("la liste doit venir du cache : %d appel(s)", state.CallCount("GET /user/memberships/orgs"))
	}
}

func TestAvertissementSelonLeDroitDeCreation(t *testing.T) {
	h := nouveau(t, plusieursOrgs(t))
	h.Options.Org = ""
	if code, _ := h.script("gerer", "visiteur", "revenir"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	h.contient("création de dépôts est réservée aux propriétaires")

	autre := nouveau(t, plusieursOrgs(t))
	autre.Options.Org = ""
	if code, _ := autre.script("gerer", "college", "revenir"); code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, autre.texte())
	}
	// Là où la création est autorisée, aucun avertissement n'a lieu d'être.
	autre.absent("réservée aux propriétaires", "doit être autorisée")
}

func TestOrganisationFournieEnLigneDeCommandeNePasseParLeMenu(t *testing.T) {
	state := plusieursOrgs(t)
	h := nouveau(t, state)
	h.Options.Org = "college"

	code, scripte := h.script("gerer", "revenir")
	if code != app.ExitOK {
		t.Fatalf("code = %d\n%s", code, h.texte())
	}
	if _, trouve := scripte.MenuFor("Organisation GitHub"); trouve {
		t.Error("--org doit court-circuiter le menu")
	}
	if appels := state.CallCount("GET /user/memberships/orgs"); appels != 0 {
		t.Errorf("%d appel(s) inutile(s) aux adhésions", appels)
	}
}
