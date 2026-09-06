package roster_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
)

// Une liste sortie d'Omnivox a trois particularités qu'une lecture naïve rate.
// Elle est donc écrite ici telle qu'elle arrive, octet par octet, plutôt que
// déposée à côté : le format se lit dans le test, et aucune donnée d'étudiant
// n'entre dans le dépôt.
//
// L'encodage est Windows-1252 : « é » y tient sur l'octet 0xE9, « É » sur 0xC9.
// Les fins de ligne sont CRLF. Chaque champ est blindé « ="valeur" », armure
// qu'Excel comprend comme du texte — sans elle, un numéro de groupe « 1030 »
// deviendrait un nombre et « 0102 » perdrait son zéro de tête.
func fichierOmnivox(t *testing.T, lignes ...string) string {
	t.Helper()
	var octets []byte
	for _, ligne := range lignes {
		for _, r := range ligne {
			switch {
			case r < 0x100: // Windows-1252 rejoint l'ISO-8859-1 au-delà de 0x9F
				octets = append(octets, byte(r))
			default:
				t.Fatalf("caractère hors Windows-1252 dans la pièce d'essai : %q", r)
			}
		}
		octets = append(octets, '\r', '\n')
	}
	chemin := filepath.Join(t.TempDir(), "ListeEtudiants.csv")
	if err := os.WriteFile(chemin, octets, 0o600); err != nil {
		t.Fatal(err)
	}
	return chemin
}

// entete est celui qu'Omnivox écrit quand on coche numéro d'étudiant, numéro de
// groupe, nom de l'étudiant et code permanent.
const entete = "No étudiant;Groupe;Nom de l'étudiant;Prénom de l'étudiant;Code perm.;"

func TestListeOmnivoxSeLitTelleQuelle(t *testing.T) {
	chemin := fichierOmnivox(t, entete,
		`="1680229";="1030";="Adam-Larocque";="Laurent";="ADAL20059908";`,
		`="2143020";="1030";="Bourassa";="Félix";="BOUF68040412";`,
		`="1983429";="1030";="Lyonnais";="Étienne";="LYOE78040203";`,
	)
	liste, err := roster.Load(chemin)
	if err != nil {
		t.Fatalf("Load : %v", err)
	}
	if len(liste.Issues) != 0 {
		t.Fatalf("lignes rejetées : %+v", liste.Issues)
	}
	if len(liste.Entries) != 3 {
		t.Fatalf("%d entrée(s) : %+v", len(liste.Entries), liste.Entries)
	}

	// Le prénom passe devant : c'est ainsi qu'on nomme une personne, et ainsi
	// que son nom entrera dans celui de ses dépôts.
	premiere := liste.Entries[0]
	if premiere.FullName != "Laurent Adam-Larocque" {
		t.Fatalf("nom composé = %q", premiere.FullName)
	}
	// L'armure « ="…" » est retirée, les zéros de tête survivent.
	if premiere.StudentID != "1680229" || premiere.Permanent != "ADAL20059908" ||
		premiere.Group != "1030" {
		t.Fatalf("entrée = %+v", premiere)
	}
	// Les accents ont traversé l'encodage.
	if liste.Entries[1].FullName != "Félix Bourassa" ||
		liste.Entries[2].FullName != "Étienne Lyonnais" {
		t.Fatalf("accents perdus : %+v", liste.Entries[1:])
	}

	// Aucun compte GitHub : la liste n'en promet pas, et ce n'est pas une faute.
	if liste.Named() || len(liste.People) != 0 {
		t.Fatalf("comptes trouvés là où il n'y en a pas : %+v", liste.People)
	}
	if !liste.IsValid() {
		t.Fatal("une liste sans compte reste exploitable : elle demande un rapprochement")
	}
}

// La même liste à laquelle on a ajouté une colonne de comptes se lit d'un bloc :
// il n'y a plus rien à rapprocher.
func TestListeOmnivoxAvecComptesGitHub(t *testing.T) {
	chemin := fichierOmnivox(t,
		"No étudiant;Groupe;Nom de l'étudiant;Prénom de l'étudiant;Code perm.;GitHub",
		`="1680229";="1030";="Adam-Larocque";="Laurent";="ADAL20059908";ladam`,
		`="2143020";="1030";="Bourassa";="Félix";="BOUF68040412";="felixb"`,
	)
	liste, err := roster.Load(chemin)
	if err != nil {
		t.Fatalf("Load : %v", err)
	}
	if len(liste.Issues) != 0 {
		t.Fatalf("lignes rejetées : %+v", liste.Issues)
	}
	if !liste.Named() || len(liste.People) != 2 {
		t.Fatalf("comptes = %+v", liste.People)
	}
	if liste.People[0].Username != "ladam" || liste.People[0].FullName != "Laurent Adam-Larocque" {
		t.Fatalf("première personne = %+v", liste.People[0])
	}
	// La colonne ajoutée à la main peut être blindée comme les autres, ou non.
	if liste.People[1].Username != "felixb" {
		t.Fatalf("seconde personne = %+v", liste.People[1])
	}
}

// Une colonne de comptes présente mais vide sur une ligne est une faute : la
// liste promettait un compte.
func TestUnCompteManquantEstSignaleQuandLaColonneExiste(t *testing.T) {
	chemin := fichierOmnivox(t,
		"No étudiant;Nom de l'étudiant;Prénom de l'étudiant;GitHub",
		`="1680229";="Adam-Larocque";="Laurent";ladam`,
		`="2143020";="Bourassa";="Félix";`,
	)
	liste, err := roster.Load(chemin)
	if err != nil {
		t.Fatalf("Load : %v", err)
	}
	if len(liste.People) != 1 || len(liste.Issues) != 1 {
		t.Fatalf("liste = %+v / %+v", liste.People, liste.Issues)
	}
}

// La liste à deux colonnes sans en-tête continue de se lire : c'est celle qu'on
// écrit à la main, et rien ne l'a remplacée.
func TestListeSansEnteteResteLue(t *testing.T) {
	liste := roster.Parse("Émilie Côté,emilie-cote\nJean-Luc Picard,jlpicard\n")
	if len(liste.People) != 2 || len(liste.Issues) != 0 {
		t.Fatalf("liste = %+v / %+v", liste.People, liste.Issues)
	}
	if liste.People[0].FullName != "Émilie Côté" {
		t.Fatalf("première personne = %+v", liste.People[0])
	}
}
