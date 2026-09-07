package roster_test

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// cohorte reprend la forme des noms qu'une liste de cégep porte vraiment :
// traits d'union, accents, particules, noms composés en deux mots.
func cohorte() []roster.Entry {
	return []roster.Entry{
		{FullName: "Laurent Adam-Larocque", StudentID: "1680229", Permanent: "ADAL20059908"},
		{FullName: "Félix Bourassa", StudentID: "2143020", Permanent: "BOUF68040412"},
		{FullName: "Étienne Lyonnais", StudentID: "1983429", Permanent: "LYOE78040203"},
		{FullName: "Jacob Sauvé-Labonté", StudentID: "1944451", Permanent: "SAUJ87030203"},
		{FullName: "Ilyas El Haddadi", StudentID: "2115470", Permanent: "ELHI69100106"},
		{FullName: "Diego Garcia Rioja", StudentID: "1932943", Permanent: "GARD93080202"},
	}
}

// trouve retrouve un rapprochement par son compte.
func trouve(t *testing.T, rapprochements []roster.Pairing, login string) roster.Pairing {
	t.Helper()
	for _, r := range rapprochements {
		if r.Login == login {
			return r
		}
	}
	t.Fatalf("aucun rapprochement pour « %s »", login)
	return roster.Pairing{}
}

// Les pseudonymes qu'on rencontre vraiment, du plus explicite au plus avare.
func TestRapprochementDesFormesCourantes(t *testing.T) {
	cas := []struct {
		login   string
		nom     string
		minimum int
	}{
		{"laurent-adam-larocque", "Laurent Adam-Larocque", roster.Certain},
		{"felixbourassa", "Félix Bourassa", roster.Probable},
		{"ladamlarocque", "Laurent Adam-Larocque", roster.Probable},
		{"lyonnais", "Étienne Lyonnais", roster.Probable},
		{"jsauvelabonte", "Jacob Sauvé-Labonté", roster.Probable},
		// La particule « El » ne compte pas : c'est « haddadi » qui désigne.
		{"ihaddadi", "Ilyas El Haddadi", roster.Probable},
		// Un nom en deux mots : le plus long tranche.
		{"drioja", "Diego Garcia Rioja", roster.Probable},
	}
	for _, essai := range cas {
		t.Run(essai.login, func(t *testing.T) {
			// Un compte à la fois : c'est le score qui est éprouvé ici, non
			// la répartition entre plusieurs comptes d'une même personne.
			trouvee := trouve(t, roster.Match(cohorte(), []string{essai.login}, nil), essai.login)
			if trouvee.Entry.FullName != essai.nom {
				t.Fatalf("« %s » rapproché de %q (%d, %s), attendu %q",
					essai.login, trouvee.Entry.FullName, trouvee.Score, trouvee.Reason, essai.nom)
			}
			if trouvee.Score < essai.minimum {
				t.Errorf("score = %d (%s), attendu au moins %d",
					trouvee.Score, trouvee.Reason, essai.minimum)
			}
		})
	}
}

// Le numéro d'étudiant dans un pseudonyme n'est pas une coïncidence : c'est
// l'indice le plus sûr, et il l'emporte même quand le nom ne dit rien.
func TestLeNumeroDEtudiantTrancheSeul(t *testing.T) {
	rapprochements := roster.Match(cohorte(), []string{"xyz1680229"}, nil)
	trouvee := trouve(t, rapprochements, "xyz1680229")
	if trouvee.Entry.FullName != "Laurent Adam-Larocque" || trouvee.Score != 100 {
		t.Fatalf("rapprochement = %+v", trouvee)
	}
	if !strings.Contains(trouvee.Reason, "numéro") {
		t.Errorf("raison = %q", trouvee.Reason)
	}
}

// Le nom du profil GitHub, quand on le connaît, vaut mieux que le pseudonyme.
func TestLeProfilGitHubTrancheQuandLePseudoNeDitRien(t *testing.T) {
	profils := map[string]string{"xkcd42": "Étienne Lyonnais"}
	rapprochements := roster.Match(cohorte(), []string{"xkcd42"}, profils)
	trouvee := trouve(t, rapprochements, "xkcd42")
	if trouvee.Entry.FullName != "Étienne Lyonnais" || trouvee.Score < roster.Certain {
		t.Fatalf("rapprochement = %+v", trouvee)
	}
}

// Un pseudonyme que rien ne relie à personne reste sans réponse : inventer un
// rapprochement serait pire que de n'en proposer aucun.
func TestUnPseudoOpaqueNeSeRapprochePas(t *testing.T) {
	rapprochements := roster.Match(cohorte(), []string{"dark-wizard-99"}, nil)
	trouvee := trouve(t, rapprochements, "dark-wizard-99")
	if trouvee.Found() {
		t.Fatalf("rapprochement inventé : %+v", trouvee)
	}
}

// Deux personnes qui se valent ne se départagent pas : les nommer vaut mieux
// que d'en retenir une au hasard.
func TestDeuxPersonnesQuiSeValentSontSignalees(t *testing.T) {
	jumeaux := []roster.Entry{
		{FullName: "Marc Tremblay", StudentID: "1111111"},
		{FullName: "Marie Tremblay", StudentID: "2222222"},
	}
	rapprochements := roster.Match(jumeaux, []string{"tremblay"}, nil)
	trouvee := trouve(t, rapprochements, "tremblay")
	if !trouvee.Ambiguous || trouvee.Found() {
		t.Fatalf("rapprochement = %+v", trouvee)
	}
	if len(trouvee.Rivals) != 2 {
		t.Fatalf("rivales = %v", trouvee.Rivals)
	}
}

// Une personne n'est retenue qu'une fois, et c'est l'indice le plus sûr qui la
// prend : un pseudonyme vague ne vole pas la place d'un pseudonyme explicite.
func TestUnePersonneNEstRetenueQuUneFois(t *testing.T) {
	gens := []roster.Entry{
		{FullName: "Félix Bourassa", StudentID: "2143020"},
		{FullName: "Olivier Bournival", StudentID: "2162536"},
	}
	// « bour » ressemble aux deux ; « felixbourassa2143020 » ne ressemble qu'à un.
	rapprochements := roster.Match(gens, []string{"bourassa", "felixbourassa2143020"}, nil)
	sur := trouve(t, rapprochements, "felixbourassa2143020")
	if sur.Entry.FullName != "Félix Bourassa" || sur.Score != 100 {
		t.Fatalf("le compte le plus explicite = %+v", sur)
	}
	vague := trouve(t, rapprochements, "bourassa")
	if vague.Entry.FullName == "Félix Bourassa" {
		t.Fatalf("la même personne a été retenue deux fois : %+v", vague)
	}
}

// Deux exécutions disent la même chose : l'ordre ne dépend pas du hasard des
// tables de hachage.
func TestLeRapprochementEstStable(t *testing.T) {
	logins := []string{"ladamlarocque", "lyonnais", "drioja", "inconnu"}
	premier := roster.Match(cohorte(), logins, nil)
	for essai := 0; essai < 5; essai++ {
		suivant := roster.Match(cohorte(), logins, nil)
		for index := range premier {
			if premier[index].Login != suivant[index].Login ||
				premier[index].Entry.FullName != suivant[index].Entry.FullName {
				t.Fatalf("rapprochement instable : %+v puis %+v", premier, suivant)
			}
		}
	}
	// L'ordre rendu est celui des comptes fournis.
	if premier[0].Login != "ladamlarocque" || premier[3].Login != "inconnu" {
		t.Fatalf("ordre = %+v", premier)
	}
}

func TestFindNameReconnaitDeuxEcrituresDunMemeNom(t *testing.T) {
	liste := []roster.Entry{
		{FullName: "Ahmad Walid Loudin", StudentID: "111"},
		{FullName: "Guillaume Gobeil-Bouchard", StudentID: "222"},
		{FullName: "Félix Bourassa", StudentID: "333"},
	}
	cas := []struct {
		cherche, attendu string
	}{
		// Le registre garde le nom du profil GitHub, plus court que l'officiel.
		{"Ahmad Loudin", "Ahmad Walid Loudin"},
		{"Guillaume Bouchard", "Guillaume Gobeil-Bouchard"},
		// L'écriture exacte, à l'accent et à la casse près.
		{"FELIX BOURASSA", "Félix Bourassa"},
		{"Felix Bourassa", "Félix Bourassa"},
	}
	for _, essai := range cas {
		trouve, dans := roster.FindName(liste, essai.cherche)
		if !dans || trouve.FullName != essai.attendu {
			t.Errorf("FindName(%q) = %q, %v ; attendu %q",
				essai.cherche, trouve.FullName, dans, essai.attendu)
		}
	}
}

func TestFindNameNeTranchePasDansLeDoute(t *testing.T) {
	liste := []roster.Entry{
		{FullName: "Jean Tremblay"},
		{FullName: "Jean Marc Tremblay"},
	}
	// Deux personnes s'y prêtent : mieux vaut n'en nommer aucune.
	if trouve, dans := roster.FindName(liste, "Jean Tremblay"); !dans ||
		trouve.FullName != "Jean Tremblay" {
		t.Fatalf("l'écriture exacte doit primer : %q, %v", trouve.FullName, dans)
	}
	if trouve, dans := roster.FindName(liste, "Tremblay Jean Marc Junior"); dans {
		t.Fatalf("FindName = %q : deux personnes s'y prêtaient", trouve.FullName)
	}
	// Un prénom seul ne désigne personne.
	if trouve, dans := roster.FindName([]roster.Entry{{FullName: "Ahmad Walid Loudin"}},
		"Ahmad"); dans {
		t.Fatalf("FindName = %q : un mot seul ne suffit pas", trouve.FullName)
	}
	if _, dans := roster.FindName(liste, ""); dans {
		t.Fatal("un nom vide ne désigne personne")
	}
}
