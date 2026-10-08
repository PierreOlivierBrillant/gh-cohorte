package web

import (
	"strings"
	"testing"

	"github.com/PierreOlivierBrillant/gh-nestor/internal/brand"
)

// Le terminal lit ses couleurs dans brand, le navigateur dans theme.css. Rien
// ne les relie à l'exécution : c'est ce test qui les tient ensemble, pour
// qu'une teinte changée d'un côté ne laisse pas l'autre en arrière.
func TestLaPaletteDuWebEstCelleDeLaMarque(t *testing.T) {
	feuille, err := assets.ReadFile("assets/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	css := strings.ToLower(string(feuille))
	for nom, couleur := range map[string]string{
		"Encre": brand.Encre, "Papier": brand.Papier, "Bleu": brand.Bleu,
		"BleuClair": brand.BleuClair, "Vert": brand.Vert, "Rouge": brand.Rouge,
		"Jaune": brand.Jaune, "Gris": brand.Gris,
	} {
		if !strings.Contains(css, strings.ToLower(couleur)) {
			t.Errorf("brand.%s = %s n'apparaît pas dans theme.css : le terminal et le navigateur divergent", nom, couleur)
		}
	}
}

func TestLaPageChargeSonThemeAvantSesComposants(t *testing.T) {
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	theme, composants := strings.Index(html, "/theme.css"), strings.Index(html, "/app.css")
	if theme < 0 || composants < 0 || theme > composants {
		t.Error("theme.css doit être lié avant app.css : les composants lisent ses jetons")
	}
	if !strings.Contains(html, "<title>"+brand.Name+"</title>") {
		t.Errorf("le titre de la page doit être « %s »", brand.Name)
	}
}

func TestLesComposantsNeCodentAucuneCouleur(t *testing.T) {
	// Une couleur écrite en dur dans app.css échappe au thème : elle ne suit
	// ni le mode sombre ni un changement de palette. Tout passe par un jeton.
	feuille, err := assets.ReadFile("assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for numero, ligne := range strings.Split(string(feuille), "\n") {
		if strings.Contains(ligne, "#") && !strings.Contains(ligne, "/*") && !strings.HasPrefix(strings.TrimSpace(ligne), "#") {
			t.Errorf("app.css:%d : une couleur codée en dur — %s", numero+1, strings.TrimSpace(ligne))
		}
		if strings.Contains(ligne, "rgba(") || strings.Contains(ligne, "rgb(") {
			t.Errorf("app.css:%d : une couleur codée en dur — %s", numero+1, strings.TrimSpace(ligne))
		}
	}
}
