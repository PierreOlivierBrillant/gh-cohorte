package plagiarism

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// La passe automatisée.
//
// L'analyse est déjà scriptable : « gh cohorte --plagiarism … --non-interactive »
// ne demande rien à personne. La faire tourner dans une GitHub Action ne
// demande donc aucun code de plus — un gabarit de workflow, et deux choses à
// dire clairement.
//
// La première est le jeton. Lire les dépôts d'un collègue demande un accès en
// lecture sur toute l'organisation, et un tel accès ne se met pas dans un
// workflow sans y penser : qui peut modifier ce fichier peut lui faire émettre
// n'importe quoi. Le dépôt qui le porte doit donc être protégé en écriture.
//
// La seconde est ce que la passe rend. Un rapport d'analyse porte des noms
// d'étudiants ; un index publié n'en porte aucun. Le gabarit publie l'index par
// défaut, et laisse le rapport en variante commentée, avec ce qu'il faut savoir.

// WorkflowFile est le nom sous lequel le gabarit se dépose.
const WorkflowFile = ".github/workflows/plagiat.yml"

// Workflow est le gabarit de passe automatisée.
const Workflow = `# Comparaison des copies, tournée par GitHub Actions.
#
# Déposé par « gh cohorte --emit-workflow ». Ce qu'il fait par défaut : publier
# l'index d'empreintes d'un travail, pour que les collègues de l'organisation
# puissent y comparer leurs copies. Un index ne porte ni code ni nom — rien de
# ce que cette passe produit ne peut être lu comme une liste d'étudiants.
#
# ── Avant de s'en servir ────────────────────────────────────────────────────
#
# 1. Un jeton. « COHORTE_TOKEN » doit être un jeton à portée fine, ou celui
#    d'une GitHub App installée sur l'organisation, avec « Contents: read » sur
#    les dépôts à lire et « Contents: write » sur « .cohorte » et
#    « .cohorte-empreintes ». Le jeton « GITHUB_TOKEN » ne suffit pas : il ne
#    voit que le dépôt où la passe tourne.
#
# 2. Une protection. Ce jeton lit toute l'organisation. Qui peut modifier ce
#    fichier peut lui faire émettre n'importe quoi : le dépôt qui le porte doit
#    être protégé en écriture — une règle de branche exigeant une revue — et
#    distinct de « .cohorte », où toute l'équipe enseignante écrit.
#
#    Tant que les enseignants sont propriétaires de l'organisation, ils voient
#    déjà tout et la question reste théorique. Elle cesse de l'être le jour où
#    ils deviennent simples membres.

name: plagiat

on:
  workflow_dispatch:
    inputs:
      travail:
        description: "Travail à traiter (« a26.5n6.01.tp1 »)"
        required: true
      profil:
        description: "Profil d'inspection"
        required: false
        default: tout

jobs:
  publier:
    runs-on: ubuntu-latest
    steps:
      - name: Installer gh cohorte
        env:
          GH_TOKEN: ${{ secrets.COHORTE_TOKEN }}
        run: gh extension install PierreOlivierBrillant/gh-cohorte

      # Publie l'index d'empreintes du travail, et l'annonce au catalogue.
      # Rien de ce qui sort ne nomme quiconque.
      - name: Publier l'index
        env:
          GH_TOKEN: ${{ secrets.COHORTE_TOKEN }}
        run: |
          gh cohorte --publish-index \
            --org "${{ github.repository_owner }}" \
            --manage "${{ inputs.travail }}" \
            --profile "${{ inputs.profil }}" \
            --non-interactive --yes

      # ── Variante : rendre le rapport plutôt que l'index ──────────────────
      #
      # Un rapport d'analyse porte les noms des étudiants : il n'a pas à sortir
      # de l'équipe enseignante. Si vous décommentez ce qui suit, assurez-vous
      # que ce dépôt est privé et que seuls les enseignants peuvent télécharger
      # ses artefacts — un artefact se télécharge par toute personne ayant accès
      # au dépôt.
      #
      # - name: Comparer
      #   env:
      #     GH_TOKEN: ${{ secrets.COHORTE_TOKEN }}
      #   run: |
      #     gh cohorte --plagiarism \
      #       --org "${{ github.repository_owner }}" \
      #       --manage "${{ inputs.travail }}" \
      #       --profile "${{ inputs.profil }}" \
      #       --report-dir rapports --non-interactive --yes
      # - uses: actions/upload-artifact@v4
      #   with:
      #     name: rapport-plagiat
      #     path: rapports/plagiat
`

// EmitWorkflow dépose le gabarit de passe automatisée.
//
// Un fichier déjà présent n'est pas écrasé : il a peut-être été retouché — le
// nom du jeton, la version de l'extension —, et le remplacer sans prévenir
// effacerait ce travail.
func EmitWorkflow(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		path = WorkflowFile
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, filepath.Base(WorkflowFile))
	}
	if _, err := os.Stat(path); err == nil {
		return "", valid.Errorf(
			"« %s » existe déjà. Retirez-le ou donnez un autre chemin : il a "+
				"peut-être été retouché, et l'écraser effacerait ce travail.", path)
	}
	if dossier := filepath.Dir(path); dossier != "." {
		if err := os.MkdirAll(dossier, 0o755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(Workflow), 0o644); err != nil {
		return "", valid.Errorf("Gabarit « %s » : %v.", path, err)
	}
	return path, nil
}
