package plagiarism

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/exchange"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/similarity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Publier un index, et comparer contre celui d'un collègue.
//
// C'est ce qui rend le dépistage possible sans percer le cloisonnement : des
// hachés traversent, pas du code. Un collègue mesure ses copies contre les
// nôtres, voit qu'une paire sort du lot, et ne sait toujours rien de qui elle
// concerne — c'est exactement ce qu'on veut à ce stade.

// Indexes lit les index publiés par les collègues.
type Indexes interface {
	Read(assignment string) (exchange.Published, bool, error)
}

// Sources dit d'où une analyse tire ce qu'elle compare.
type Sources struct {
	// Client va chercher les dépôts sur GitHub.
	Client corpus.Client
	// Indexes lit les index publiés. Nil n'en lit aucun.
	Indexes Indexes
	// Prior sont d'anciens rapports, pour reprendre ce qu'ils ont déjà
	// empreinté plutôt que de le retélécharger.
	Prior []*Report
}

// Settings rend les réglages de la demande, tels qu'un index les porte.
//
// C'est ce qui permet de refuser un index calculé autrement : deux index aux
// bornes différentes ne portent pas les mêmes empreintes pour le même code, et
// les mêler donnerait des similarités proches de zéro — c'est-à-dire un rapport
// rassurant et faux.
func (r Request) Settings() exchange.Settings {
	return exchange.Settings{
		Profile:   r.Inspection.Profile,
		Languages: r.Inspection.Languages,
		Include:   r.Inspection.Include,
		Exclude:   r.Inspection.Exclude,
		Root:      r.Inspection.Root,
		NoStrip:   r.Inspection.NoStrip,
		Kgram:     r.Kgram,
		Window:    r.Window,
	}
}

// Motifs propres aux index publiés.
const (
	NoIndex         = "aucun index publié"
	IndexMismatched = "index calculé autrement"
	IndexUnreadable = "index illisible"
)

// loadIndexes verse dans le corpus les index publiés que la demande nomme.
func loadIndexes(sources Sources, request Request,
	pris map[string]bool) ([]similarity.Work, []corpus.Inspected, []corpus.Problem) {

	works := make([]similarity.Work, 0, 16)
	inspected := make([]corpus.Inspected, 0, 16)
	problems := make([]corpus.Problem, 0, 2)
	if len(request.Indexes) == 0 {
		return works, inspected, problems
	}
	if sources.Indexes == nil {
		for _, id := range request.Indexes {
			problems = append(problems, corpus.Problem{
				ID: id, Repo: id, Reason: NoIndex,
				Detail: "aucun accès aux index de l'organisation",
			})
		}
		return works, inspected, problems
	}

	mien := request.Settings()
	for _, id := range request.Indexes {
		published, trouve, err := sources.Indexes.Read(id)
		if err != nil {
			problems = append(problems, corpus.Problem{
				ID: id, Repo: id, Reason: IndexUnreadable, Detail: err.Error(),
			})
			continue
		}
		if !trouve {
			problems = append(problems, corpus.Problem{
				ID: id, Repo: id, Reason: NoIndex,
				Detail: "personne ne l'a publié : demandez-le à qui donne ce travail",
			})
			continue
		}
		// Un index calculé autrement n'est pas mêlé à moitié : il est écarté
		// entier, avec la raison. Le mêler donnerait des mesures fausses sans
		// que rien ne le signale.
		if err := published.Comparable(mien); err != nil {
			problems = append(problems, corpus.Problem{
				ID: id, Repo: id, Reason: IndexMismatched, Detail: err.Error(),
			})
			continue
		}

		origine := published.Origin
		if origine == "" {
			origine = published.Assignment
		}
		for _, work := range published.Corpus.Works {
			if pris[work.ID] {
				problems = append(problems, corpus.Problem{
					ID: work.ID, Repo: id, Reason: ArchiveClash,
					Detail: "une copie de ce jeton vient déjà d'ailleurs",
				})
				continue
			}
			pris[work.ID] = true
			// L'étiquette reste vide : une copie d'un index ne nomme personne,
			// et lui en donner une ici reviendrait à en inventer une.
			work.Label = ""
			work.Origin = origine
			works = append(works, work)
			inspected = append(inspected, corpus.Inspected{
				ID: work.ID, Repo: "index de « " + published.Assignment + " »",
				Kept: len(work.Files), Tokens: work.TokenCount(),
				Prints: work.PrintCount(),
			})
		}
	}
	return works, inspected, problems
}

// ------------------------------------------------------------- publication

// Publishable fabrique l'index publiable d'un rapport.
//
// Les copies y perdent leur nom de dépôt au profit d'un jeton tiré au hasard,
// exactement comme dans un envoi anonymisé et pour la même raison : le dernier
// niveau d'un nom de dépôt nomme une personne. La table qui relie les jetons
// aux dépôts est rendue à part, et reste chez qui publie.
func Publishable(report *Report, teacher, origin string,
	options anonymize.Options) (exchange.Published, anonymize.Table, error) {

	if report == nil || report.Analyzed() == 0 {
		return exchange.Published{}, anonymize.Table{}, valid.Errorf(
			"Publication : ce rapport ne porte aucune copie.")
	}
	assignment := strings.TrimSpace(report.Request.Assignment)
	if teacher = strings.TrimSpace(teacher); teacher == "" {
		return exchange.Published{}, anonymize.Table{}, valid.Errorf(
			"Publication : dites qui publie.")
	}

	// Seules les copies de nos dépôts sont publiées. Republier l'index d'un
	// collègue, ou des copies reçues dans un ZIP, reviendrait à redistribuer ce
	// qu'on nous a confié — et sous nos jetons, ce qui brouillerait la piste
	// jusqu'à son propriétaire.
	identities := make([]anonymize.Identity, 0, report.Analyzed())
	nôtres := make([]similarity.Work, 0, report.Analyzed())
	for _, work := range report.Index.Works {
		target, connu := report.Target(work.ID)
		if !connu {
			continue
		}
		identities = append(identities, anonymize.Identity{
			Work: work.ID, Person: target.Person, Origin: target.Origin,
			HandedIn: target.HandedIn,
		})
		nôtres = append(nôtres, work)
	}
	if len(nôtres) == 0 {
		return exchange.Published{}, anonymize.Table{}, valid.Errorf(
			"Publication : ce rapport ne porte aucune copie de vos dépôts. On ne " +
				"republie pas l'index d'un collègue.")
	}

	anonymizer, err := anonymize.New(identities, options)
	if err != nil {
		return exchange.Published{}, anonymize.Table{}, err
	}

	published := exchange.Published{
		Assignment: assignment, Teacher: teacher, Origin: origin,
		Settings: report.Request.Settings(),
	}
	for _, work := range nôtres {
		token, connu := anonymizer.TokenOf(work.ID)
		if !connu {
			continue
		}
		// Les chemins de fichiers restent : ils disent où un fragment se
		// trouve, ce qui sert à juger, et « src/Solution.java » ne nomme
		// personne. Un chemin qui nommerait quelqu'un — « rendu-emilie/… » —
		// est anonymisé comme le reste.
		anonyme := similarity.Work{ID: token.Base, Origin: origin}
		for _, file := range work.Files {
			chemin, _ := anonymizer.ScrubPath(file.Path)
			file.Path = chemin
			anonyme.Files = append(anonyme.Files, file)
		}
		// Les commentaires ne sont pas publiés : ce sont des phrases écrites
		// par des étudiants, et un commentaire suffit parfois à reconnaître son
		// auteur. Le signal qu'ils portent se retrouve à la levée du voile.
		published.Corpus.Works = append(published.Corpus.Works, anonyme)
	}

	valide, err := published.Validate()
	if err != nil {
		return exchange.Published{}, anonymize.Table{}, err
	}
	table := anonymize.Table{
		Version: anonymize.Version, Assignment: assignment,
		CreatedAt: report.CreatedAt, Tokens: anonymizer.Tokens(),
	}
	return valide, table, nil
}
