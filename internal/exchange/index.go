package exchange

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/similarity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// L'index publié : ce qui traverse le cloisonnement, et rien d'autre.
//
// Une empreinte est le haché sur 64 bits d'un k-gramme de vingt-trois jetons.
// On ne remonte pas au code depuis elle : elle confirme qu'un fragment est
// partagé, elle ne le montre pas. Un collègue peut donc mesurer ses copies
// contre les nôtres sans en lire une ligne — et c'est tout ce qu'il faut pour
// dépister.
//
// Les copies y portent des jetons opaques, jamais leur nom de dépôt : le
// dernier niveau d'un nom de dépôt nomme une personne. La table qui relie les
// jetons aux personnes reste chez qui a publié, exactement comme pour un envoi
// anonymisé — et pour la même raison.

// Emplacement des index dans l'organisation.
const (
	// IndexRepo est le second dépôt de service. Les index ne vont pas dans
	// « .cohorte » : un index pèse des centaines de kilo-octets là où le
	// registre entier en pèse quelques-uns, et l'historique du registre — qu'on
	// relit pour comprendre qui a corrigé quel nom — deviendrait illisible.
	IndexRepo = ".cohorte-empreintes"
	// IndexBranch est la seule branche écrite.
	IndexBranch = "main"
	// IndexReadme explique le dépôt à qui l'ouvre sur github.com.
	IndexReadme = "README.md"
)

// IndexDescription est ce que le dépôt annonce.
const IndexDescription = "Index d'empreintes — gh cohorte. Aucun code, aucun nom."

// IndexPath rend le chemin d'un index dans le dépôt.
func IndexPath(assignment string) string {
	return path.Join("index", strings.ToLower(strings.TrimSpace(assignment))+".json")
}

// Settings dit avec quoi un index a été calculé.
//
// Il voyage avec lui, et ce n'est pas une politesse : deux index calculés avec
// des bornes différentes ne portent pas les mêmes empreintes pour le même code.
// Les mêler donnerait des similarités proches de zéro et laisserait croire que
// personne n'a rien copié.
type Settings struct {
	Profile   string   `json:"profile,omitempty"`
	Languages []string `json:"languages,omitempty"`
	Include   []string `json:"include,omitempty"`
	Exclude   []string `json:"exclude,omitempty"`
	Root      string   `json:"root,omitempty"`
	NoStrip   bool     `json:"no_strip,omitempty"`
	Kgram     int      `json:"kgram,omitempty"`
	Window    int      `json:"window,omitempty"`
}

// Differences nomme ce qui empêche deux réglages de se comparer. Une liste vide
// veut dire qu'ils se comparent.
func (s Settings) Differences(other Settings) []string {
	ecarts := make([]string, 0, 4)
	if s.Profile != other.Profile {
		ecarts = append(ecarts, fmt.Sprintf(
			"le profil d'inspection (« %s » ici, « %s » là-bas)",
			orNone(other.Profile), orNone(s.Profile)))
	}
	for _, champ := range []struct {
		nom        string
		mien, sien []string
	}{
		{"les langages", other.Languages, s.Languages},
		{"les chemins inspectés", other.Include, s.Include},
		{"les exclusions", other.Exclude, s.Exclude},
	} {
		if !sameList(champ.mien, champ.sien) {
			ecarts = append(ecarts, champ.nom)
		}
	}
	if s.Root != other.Root || s.NoStrip != other.NoStrip {
		ecarts = append(ecarts, "la racine des projets")
	}
	if s.Kgram != other.Kgram {
		ecarts = append(ecarts, fmt.Sprintf(
			"la longueur des k-grammes (%s ici, %s là-bas)",
			orDefault(other.Kgram), orDefault(s.Kgram)))
	}
	if s.Window != other.Window {
		ecarts = append(ecarts, fmt.Sprintf(
			"la fenêtre de winnowing (%s ici, %s là-bas)",
			orDefault(other.Window), orDefault(s.Window)))
	}
	return ecarts
}

// Published est un index publié.
type Published struct {
	Version int `json:"version"`
	// Assignment nomme le travail. Il ne désigne personne : c'est une place et
	// un nom de travail.
	Assignment string `json:"assignment"`
	// Teacher est le compte qui a publié. Il est là pour qu'on sache à qui
	// s'adresser quand une paire sort du lot.
	Teacher     string `json:"teacher"`
	PublishedAt string `json:"published_at"`
	// Origin est l'étiquette que les copies porteront chez le destinataire.
	// C'est elle qui colore les nuages de points, et qui lui permet de dire
	// « ces deux-là ne viennent même pas du même groupe ».
	Origin   string            `json:"origin,omitempty"`
	Settings Settings          `json:"settings"`
	Corpus   similarity.Corpus `json:"corpus"`
}

// Copies compte les copies de l'index.
func (p Published) Copies() int { return len(p.Corpus.Works) }

// Prints compte ses empreintes.
func (p Published) Prints() int {
	total := 0
	for _, work := range p.Corpus.Works {
		total += work.PrintCount()
	}
	return total
}

// Validate refuse un index qui ne pourrait pas servir, ou qui en dirait trop.
//
// La seconde vérification est la plus importante : un index qui porterait des
// noms de dépôts serait une liste de classe publiée à toute l'équipe. Elle est
// faite ici, à l'écriture comme à la lecture, plutôt que laissée à la vigilance
// de l'appelant.
func (p Published) Validate() (Published, error) {
	p.Version = Version
	p.Assignment = strings.ToLower(strings.TrimSpace(p.Assignment))
	if _, _, ok := naming.SplitAssignment(p.Assignment); !ok {
		return p, valid.Errorf(
			"Index publié : « %s » n'est pas un travail de la nomenclature.", p.Assignment)
	}
	if len(p.Corpus.Works) == 0 {
		return p, valid.Errorf(
			"Index publié : « %s » ne porte aucune copie.", p.Assignment)
	}
	for _, work := range p.Corpus.Works {
		if _, reconnu := naming.Parse(work.ID); reconnu {
			return p, valid.Errorf(
				"Index publié : la copie « %s » porte un nom de dépôt. Un index ne "+
					"doit porter que des jetons opaques — le dernier niveau d'un nom "+
					"de dépôt nomme une personne.", work.ID)
		}
		if work.Label != "" {
			return p, valid.Errorf(
				"Index publié : la copie « %s » porte une étiquette (« %s »). Un "+
					"index ne nomme personne.", work.ID, work.Label)
		}
		for _, file := range work.Files {
			if file.Path == "" {
				return p, valid.Errorf(
					"Index publié : un fichier sans chemin dans « %s ».", work.ID)
			}
		}
	}
	return p, nil
}

// Comparable dit si un index se compare à une analyse, et pourquoi non.
func (p Published) Comparable(mine Settings) error {
	ecarts := p.Settings.Differences(mine)
	if len(ecarts) == 0 {
		return nil
	}
	return valid.Errorf(
		"Index de « %s » publié par @%s : il a été calculé autrement que votre "+
			"analyse — %s. Les empreintes ne se comparent pas d'un réglage à "+
			"l'autre : alignez le vôtre, ou demandez une republication.",
		p.Assignment, p.Teacher, strings.Join(ecarts, ", "))
}

// DecodePublished relit un index publié.
func DecodePublished(content []byte) (Published, error) {
	var lu Published
	if err := json.Unmarshal(content, &lu); err != nil {
		return Published{}, valid.Errorf("Index publié illisible : %v.", err)
	}
	if lu.Version > Version {
		return Published{}, valid.Errorf(
			"Index publié : il vient d'une version %d de l'outil, qui n'en connaît "+
				"que %d. Mettez l'extension à jour.", lu.Version, Version)
	}
	return lu.Validate()
}

// EncodePublished écrit un index publié.
func EncodePublished(published Published) ([]byte, error) {
	valide, err := published.Validate()
	if err != nil {
		return nil, err
	}
	valide.Corpus.Sort()
	payload, err := json.MarshalIndent(valide, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

// IndexReadmeText explique le dépôt des index à qui l'ouvre sur github.com.
func IndexReadmeText(org string) []byte {
	var texte strings.Builder
	texte.WriteString("# Index d'empreintes de `" + org + "`\n\n")
	texte.WriteString("Ce dépôt est tenu par [`gh cohorte`](https://github.com/" +
		"PierreOlivierBrillant/gh-cohorte). Il porte des **index d'empreintes** : " +
		"des hachés de fragments de code, publiés pour que les enseignants de " +
		"l'organisation puissent comparer leurs travaux entre eux.\n\n")
	texte.WriteString("**Il ne contient ni code, ni nom.** Une empreinte est le " +
		"haché sur 64 bits d'un fragment : elle confirme que deux travaux " +
		"partagent un passage, elle ne montre pas ce passage. Les copies y " +
		"portent des jetons tirés au hasard ; la table qui les relie aux " +
		"personnes reste sur le poste de qui a publié.\n\n")
	texte.WriteString("Voir un travail ici ne dit donc rien de personne. Pour " +
		"savoir qui se cache derrière un jeton, il faut le demander à " +
		"l'enseignant qui l'a publié — c'est le seul à pouvoir le dire.\n")
	return []byte(texte.String())
}

func orNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "aucun"
	}
	return value
}

func orDefault(value int) string {
	if value <= 0 {
		return "celle du langage"
	}
	return fmt.Sprintf("%d", value)
}

func sameList(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	gauche := append([]string(nil), left...)
	droite := append([]string(nil), right...)
	sort.Strings(gauche)
	sort.Strings(droite)
	for index := range gauche {
		if gauche[index] != droite[index] {
			return false
		}
	}
	return true
}
