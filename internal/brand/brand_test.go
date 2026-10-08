package brand

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLeDossierDeLancienNomEstRepris(t *testing.T) {
	base := t.TempDir()
	ancien := filepath.Join(base, LegacySlug)
	if err := os.MkdirAll(ancien, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ancien, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	obtenu := AdoptLegacyDir(base)
	if obtenu != filepath.Join(base, Slug) {
		t.Fatalf("dossier rendu : %s", obtenu)
	}
	if _, err := os.Stat(filepath.Join(obtenu, "config.json")); err != nil {
		t.Errorf("le contenu de l'ancien dossier doit suivre : %v", err)
	}
	if _, err := os.Stat(ancien); !os.IsNotExist(err) {
		t.Errorf("l'ancien dossier ne doit pas rester : %v", err)
	}
}

func TestSansAncienDossierLeNouveauEstRendu(t *testing.T) {
	base := t.TempDir()
	if obtenu := AdoptLegacyDir(base); obtenu != filepath.Join(base, Slug) {
		t.Errorf("dossier rendu : %s", obtenu)
	}
	if _, err := os.Stat(filepath.Join(base, Slug)); !os.IsNotExist(err) {
		t.Error("rien ne doit être créé : le dossier naît à la première écriture")
	}
}

func TestLeNouveauDossierLemporteSurLancien(t *testing.T) {
	base := t.TempDir()
	for _, nom := range []string{Slug, LegacySlug} {
		if err := os.MkdirAll(filepath.Join(base, nom), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if obtenu := AdoptLegacyDir(base); obtenu != filepath.Join(base, Slug) {
		t.Errorf("dossier rendu : %s", obtenu)
	}
	if _, err := os.Stat(filepath.Join(base, LegacySlug)); err != nil {
		t.Error("un ancien dossier qui coexiste avec le nouveau est laissé tel quel")
	}
}

func TestLesCouleursSontEcritesEnToutesLettres(t *testing.T) {
	for nom, couleur := range map[string]string{
		"Encre": Encre, "Papier": Papier, "Bleu": Bleu, "BleuClair": BleuClair,
		"Vert": Vert, "Rouge": Rouge, "Jaune": Jaune, "Gris": Gris,
	} {
		if len(couleur) != 7 || couleur[0] != '#' {
			t.Errorf("%s = %q : une couleur de la marque est un hexadécimal à six chiffres", nom, couleur)
		}
	}
}
