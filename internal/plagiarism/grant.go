package plagiarism

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/exchange"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Accorder une demande.
//
// Le propriétaire de la copie prépare son envoi anonymisé — une seule copie,
// celle qui est demandée —, et c'est lui qui l'envoie. Rien n'est déposé dans
// l'organisation à la vue de toute l'équipe, rien ne part dans son dos, et le
// demandeur reçoit exactement ce qu'il a demandé : ni plus, ni une autre copie.
//
// Elle repart sous le jeton qu'elle portait dans l'index publié. C'est
// indispensable : le demandeur a mesuré « K7DM2X », et une copie arrivant sous
// un autre jeton serait, pour lui, une copie qu'il n'a jamais vue.
//
// Le jeton se retrouve dans la table écrite au moment de la publication, restée
// sur ce poste. Si elle a été perdue — un autre ordinateur, un dossier de
// bilans effacé —, personne ne peut plus dire quelle copie le jeton désignait,
// et l'outil le dit plutôt que d'envoyer la mauvaise.

// IndexTableSuffix termine le nom des tables écrites à la publication d'un
// index.
const IndexTableSuffix = "-index-correspondance.json"

// Located est ce qu'une table dit d'un jeton.
type Located struct {
	// Work est le dépôt que le jeton désignait.
	Work string
	// Table est le fichier qui l'a dit, pour qu'on puisse y retourner.
	Table string
	Token anonymize.Token
}

// LocateToken cherche, dans les tables d'index de ce poste, la copie qu'un
// jeton désigne.
//
// La plus récente gagne : republier un index renumérote les copies, et c'est la
// dernière publication qui correspond à ce que le demandeur a mesuré.
func LocateToken(directory, assignment, token string) (Located, error) {
	assignment = strings.ToLower(strings.TrimSpace(assignment))
	token = strings.ToUpper(strings.TrimSpace(token))
	if token == "" {
		return Located{}, valid.Errorf("Demande : elle ne désigne aucune copie.")
	}

	dossier := filepath.Join(directory, Dir)
	entries, err := os.ReadDir(dossier)
	if err != nil {
		return Located{}, valid.Errorf(
			"Demande : aucune table d'index sur ce poste (%s). Sans elle, rien ne "+
				"dit quelle copie « %s » désigne.", dossier, token)
	}
	// Les noms portent l'horodatage : les parcourir à l'envers, c'est aller de
	// la publication la plus récente à la plus ancienne.
	noms := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), IndexTableSuffix) {
			noms = append(noms, entry.Name())
		}
	}
	for index := len(noms) - 1; index >= 0; index-- {
		chemin := filepath.Join(dossier, noms[index])
		content, err := os.ReadFile(chemin)
		if err != nil {
			continue
		}
		var table anonymize.Table
		if err := json.Unmarshal(content, &table); err != nil {
			continue
		}
		if assignment != "" && strings.ToLower(table.Assignment) != assignment {
			continue
		}
		for _, entree := range table.Tokens {
			if strings.EqualFold(entree.Base, token) {
				return Located{Work: entree.Work, Table: chemin, Token: entree}, nil
			}
		}
	}
	return Located{}, valid.Errorf(
		"Demande : aucune table de ce poste ne dit quelle copie « %s » désigne "+
			"pour « %s ». La publication a peut-être été faite ailleurs, ou le "+
			"dossier des bilans a été vidé.", token, assignment)
}

// Grant prépare l'envoi anonymisé de la seule copie qu'une demande vise.
//
// La demande n'est pas tranchée ici : cette fonction prépare ce qu'il y a à
// envoyer, et c'est l'appelant qui inscrit la décision au registre. Les deux
// sont séparés exprès — un envoi qu'on n'a pas su préparer ne doit pas laisser
// une demande marquée « accordée » derrière lui.
func Grant(client corpus.Client, ask exchange.Ask, located Located,
	request Request, options anonymize.Options) (*Exported, error) {

	target, connue := request.Target(located.Work)
	if !connue {
		return nil, valid.Errorf(
			"Demande %s : la copie « %s » ne fait pas partie de ce travail.",
			ask.ID, located.Work)
	}
	// Le jeton de l'index publié, imposé : le demandeur a mesuré celui-là.
	target.Token = ask.Token
	// L'étiquette de provenance reste celle de l'index : c'est sous elle que la
	// copie a été mesurée.
	if located.Token.Origin != "" {
		target.Origin = located.Token.Origin
	}

	seule := request
	seule.Targets = []corpus.Target{target}
	seule.Archives, seule.Indexes, seule.Baseline = nil, nil, nil
	return Export(client, seule, options, nil)
}

// Target retrouve une cible de la demande par son identifiant.
func (r Request) Target(id string) (corpus.Target, bool) {
	for _, target := range r.Targets {
		if target.ID == id {
			return target, true
		}
	}
	return corpus.Target{}, false
}

// WriteIndexTable pose à côté des bilans la table qui relie les jetons d'un
// index publié aux dépôts, et rend son chemin.
//
// Elle est ici plutôt que dans chaque interface parce que c'est LocateToken qui
// la relira : la façon d'écrire ce fichier et celle de le retrouver doivent
// tenir au même endroit, sous peine qu'une publication faite au navigateur
// devienne indécodable au terminal.
//
// Droits resserrés : c'est le seul fichier qui dit qui se cache derrière un
// jeton, et il ne quitte jamais ce poste.
func WriteIndexTable(directory, base string, table anonymize.Table) (string, error) {
	dossier := filepath.Join(directory, Dir)
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		return "", err
	}
	chemin := filepath.Join(dossier, base+IndexTableSuffix)
	payload, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(chemin, append(payload, '\n'), 0o600); err != nil {
		return "", valid.Errorf("Table « %s » : %v.", chemin, err)
	}
	return chemin, nil
}

// Teaching tire du rapport la ligne de catalogue qui l'annonce.
//
// Ce qu'elle ne porte pas compte autant que ce qu'elle porte, et c'est pourquoi
// elle se décide ici : une place, un nom de travail, un décompte, une date au
// jour près. Laisser chaque interface composer la sienne finirait par publier
// ailleurs ce qu'on a refusé de publier ici.
//
// Le décompte ne porte que sur nos dépôts : les copies reçues d'ailleurs ne sont
// pas les nôtres à annoncer.
func (r *Report) Teaching(teacher string, indexed bool) (exchange.Teaching, error) {
	scope, nom, ok := naming.SplitAssignment(r.Request.Assignment)
	if !ok {
		return exchange.Teaching{}, valid.Errorf(
			"Publication : « %s » n'est pas un travail de la nomenclature. Seul un "+
				"travail d'un groupe déclaré se publie.", r.Request.Assignment)
	}
	dernier := ""
	for _, target := range r.Request.Targets {
		if target.HandedIn > dernier {
			dernier = target.HandedIn
		}
	}
	if len(dernier) > 10 {
		// Le jour suffit : l'heure daterait une personne.
		dernier = dernier[:10]
	}
	return exchange.Teaching{
		Scope: scope, Assignment: nom, Teacher: teacher,
		Copies: len(r.Request.Targets), LastHandin: dernier,
		Indexed: indexed, UpdatedAt: r.CreatedAt,
	}, nil
}
