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
// demande donc presque rien de plus — un gabarit de workflow, une commande qui
// rattrape les index manquants sans qu'on ait à nommer chaque travail, et deux
// choses à dire clairement.
//
// La première est le jeton. Lire les dépôts d'un collègue demande un accès en
// lecture sur toute l'organisation, et un tel accès ne se met pas dans un
// workflow sans y penser : qui peut modifier ce fichier peut lui faire émettre
// n'importe quoi. Le dépôt qui le porte doit donc être protégé en écriture.
//
// La seconde est ce que la passe rend. Un rapport d'analyse porte des noms
// d'étudiants ; un index publié n'en porte aucun. Le gabarit publie l'index par
// défaut, et laisse le rapport en variante commentée, avec ce qu'il faut savoir.
//
// Ce qu'elle ne fait jamais : trancher une demande de levée du voile. Une
// approbation ne vaut que parce que le propriétaire des copies l'a écrite
// lui-même, et une passe qui accorderait à sa place annulerait le
// cloisonnement qu'elle est censée respecter.

// WorkflowFile est le nom sous lequel le gabarit se dépose.
const WorkflowFile = ".github/workflows/plagiat.yml"

// Workflow est le gabarit de passe automatisée.
const Workflow = `# Comparaison des copies, tournée par GitHub Actions.
#
# Déposé par « gh cohorte --emit-workflow ». Ce qu'il fait par défaut : publier
# l'index d'empreintes des travaux annoncés qui n'en ont pas, pour que les
# collègues de l'organisation puissent y comparer leurs copies. Un index ne
# porte ni code ni nom — rien de ce que cette passe produit ne peut être lu
# comme une liste d'étudiants.
#
# Une demande de levée du voile ne se tranche jamais ici, et c'est délibéré :
# l'approbation ne vaut que parce que le propriétaire des copies l'a écrite
# lui-même. Une passe qui accorderait à sa place annulerait ce qu'elle est
# censée respecter.
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
  # Chaque lundi : les travaux annoncés sans index sont rattrapés tout seuls.
  # Un travail déjà indexé n'est pas retouché, la passe ne coûte donc rien
  # quand il n'y a rien à faire.
  schedule:
    - cron: "0 6 * * 1"
  workflow_dispatch:
    inputs:
      travail:
        description: "Travail à publier (vide : tous ceux qui n'ont pas d'index)"
        required: false
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

      # Sans travail nommé — le cas du déclenchement périodique —, publie
      # l'index de chaque travail annoncé au catalogue qui n'en a pas encore.
      # Rien de ce qui sort ne nomme quiconque.
      - name: Publier les index manquants
        if: inputs.travail == ''
        env:
          GH_TOKEN: ${{ secrets.COHORTE_TOKEN }}
        run: |
          gh cohorte --publish-index \
            --org "${{ github.repository_owner }}" \
            --profile "${{ inputs.profil || 'tout' }}" \
            --non-interactive --yes

      # Un travail nommé est republié, qu'il ait déjà un index ou non : c'est
      # ce qu'on lance après avoir ajouté des copies.
      - name: Publier l'index d'un travail
        if: inputs.travail != ''
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
