package plagiarism

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/anonymize"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/similarity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Comparer avec un collègue qui n'est pas dans la même organisation, ou qui
// n'utilise pas l'outil : il n'y a alors ni index publié, ni dépôt lisible, et
// le seul chemin praticable est qu'il envoie les copies.
//
// Dans les deux sens, ce qui voyage est anonymisé. À l'export, l'outil efface
// les noms et garde la table chez lui ; à la réception, les copies entrent dans
// le corpus par exactement le même chemin que celles de GitHub — mêmes
// exclusions, mêmes bornes —, sans quoi leurs mesures ne se compareraient pas
// aux nôtres.
//
// Seuls les fichiers que l'inspection retient sont envoyés. Ce sont ceux qui
// seront comparés, et ce sont aussi les seuls qu'on sache anonymiser : une
// capture d'écran ou un PDF porteraient un nom que nul remplacement de chaîne
// n'atteindrait.

// Exported est ce qu'un export produit.
type Exported struct {
	Bundle anonymize.Bundle
	// Problems nomme les copies qui n'ont pas pu être envoyées, et pourquoi.
	Problems []corpus.Problem
	// Copies compte ce qui est parti.
	Copies int
}

// Export anonymise les copies d'une demande et en fait une archive.
func Export(client corpus.Client, request Request, options anonymize.Options,
	progress func(done, total int, id string)) (*Exported, error) {

	request = request.normalized()
	if err := request.ValidateSettings(); err != nil {
		return nil, err
	}
	if len(request.Targets) == 0 {
		return nil, valid.Errorf("Envoi anonymisé : aucune copie à envoyer.")
	}
	inspector, err := inspect.New(request.Inspection, request.Rules.Profiles)
	if err != nil {
		return nil, err
	}

	identities := make([]anonymize.Identity, 0, len(request.Targets))
	for _, target := range request.Targets {
		identities = append(identities, anonymize.Identity{
			Work: target.ID, Person: target.Person, Origin: target.Origin,
			HandedIn: target.HandedIn, Token: target.Token,
		})
	}
	anonymizer, err := anonymize.New(identities, options)
	if err != nil {
		return nil, err
	}

	fetched, problems := fetchAll(client, request.Targets, inspector, request.Jobs, progress)
	bundle, err := anonymize.Export(anonymizer, anonymize.Manifest{
		Tool: "gh cohorte", Assignment: request.Assignment,
		Profile: inspector.Profile().ID,
	}, fetched)
	if err != nil {
		return nil, err
	}
	return &Exported{Bundle: bundle, Problems: problems, Copies: len(fetched)}, nil
}

// fetchAll récupère les fichiers retenus de chaque copie.
func fetchAll(client corpus.Client, targets []corpus.Target,
	inspector *inspect.Inspector, jobs int,
	progress func(done, total int, id string)) ([]anonymize.Copy, []corpus.Problem) {

	if jobs < 1 {
		jobs = corpus.DefaultJobs
	}
	if jobs > len(targets) {
		jobs = len(targets)
	}

	type outcome struct {
		copie   anonymize.Copy
		problem *corpus.Problem
	}
	outcomes := make([]outcome, len(targets))

	var wait sync.WaitGroup
	var counter sync.Mutex
	done := 0
	queue := make(chan int)

	for worker := 0; worker < jobs; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for position := range queue {
				target := targets[position]
				copie, problem := fetchOne(client, target, inspector)
				outcomes[position] = outcome{copie: copie, problem: problem}
				if progress != nil {
					counter.Lock()
					done++
					current := done
					counter.Unlock()
					progress(current, len(targets), target.ID)
				}
			}
		}()
	}
	for position := range targets {
		queue <- position
	}
	close(queue)
	wait.Wait()

	copies := make([]anonymize.Copy, 0, len(targets))
	problems := make([]corpus.Problem, 0, 4)
	for _, outcome := range outcomes {
		if outcome.problem != nil {
			problems = append(problems, *outcome.problem)
			continue
		}
		copies = append(copies, outcome.copie)
	}
	return copies, problems
}

func fetchOne(client corpus.Client, target corpus.Target,
	inspector *inspect.Inspector) (anonymize.Copy, *corpus.Problem) {

	archive, err := client.Archive(target.Owner, target.Repo, target.Ref)
	if err != nil {
		return anonymize.Copy{}, corpus.FetchProblem(target, err)
	}
	sources, _, err := corpus.Untar(archive)
	if err != nil {
		return anonymize.Copy{}, &corpus.Problem{
			ID: target.ID, Repo: target.FullName(),
			Reason: corpus.Broken, Detail: err.Error(),
		}
	}

	selection := inspector.Select(sources)
	if len(selection.Kept) == 0 {
		return anonymize.Copy{}, &corpus.Problem{
			ID: target.ID, Repo: target.FullName(), Reason: corpus.NothingKept,
			Skipped: selection.Skipped,
		}
	}
	copie := anonymize.Copy{Work: target.ID}
	for _, kept := range selection.Kept {
		copie.Files = append(copie.Files,
			anonymize.File{Path: kept.Path, Content: kept.Content})
	}
	return copie, nil
}

// ------------------------------------------------------------- réception

// loadArchives lit les ZIP d'une demande et en tire des copies empreintées.
//
// Une archive illisible n'arrête pas l'analyse : elle devient un motif, comme
// un dépôt qu'on n'a pas su lire. Refuser tout le reste parce qu'un fichier
// manque ferait perdre le travail déjà fait.
func loadArchives(request Request, options corpus.Options,
	pris map[string]bool) ([]similarity.Work, []corpus.Inspected, []corpus.Problem) {

	works := make([]similarity.Work, 0, 16)
	inspected := make([]corpus.Inspected, 0, 16)
	problems := make([]corpus.Problem, 0, 4)

	for _, chemin := range request.Archives {
		etiquette := archiveLabel(chemin)
		content, err := os.ReadFile(chemin)
		if err != nil {
			problems = append(problems, corpus.Problem{
				ID: etiquette, Repo: chemin, Reason: corpus.Unreachable,
				Detail: err.Error(),
			})
			continue
		}
		recu, err := anonymize.Import(content)
		if err != nil {
			problems = append(problems, corpus.Problem{
				ID: etiquette, Repo: chemin, Reason: corpus.Broken,
				Detail: err.Error(),
			})
			continue
		}
		for _, avis := range recu.Warnings {
			problems = append(problems, corpus.Problem{
				ID: etiquette, Repo: chemin, Reason: ArchiveNotice, Detail: avis,
			})
		}

		for _, copie := range recu.Copies {
			// Un jeton qui porterait le nom d'un dépôt à nous écraserait une
			// vraie copie en silence : mieux vaut l'écarter et le dire.
			if pris[copie.Work] {
				problems = append(problems, corpus.Problem{
					ID: copie.Work, Repo: chemin, Reason: ArchiveClash,
					Detail: "une copie de ce nom vient déjà d'ailleurs",
				})
				continue
			}
			pris[copie.Work] = true

			origine := recu.OriginOf(copie.Work)
			if origine == "" {
				origine = etiquette
			}
			sources := make([]inspect.Source, 0, len(copie.Files))
			for _, fichier := range copie.Files {
				sources = append(sources, inspect.Source{
					Path: fichier.Path, Content: fichier.Content,
				})
			}
			work, vue, souci := corpus.Fingerprint(corpus.Loose{
				ID: copie.Work, Label: copie.Work, Origin: origine,
				Sources: sources,
			}, options)
			if souci != nil {
				problems = append(problems, *souci)
				continue
			}
			works = append(works, work)
			inspected = append(inspected, vue)
		}
	}
	return works, inspected, problems
}

// Motifs propres aux archives reçues.
const (
	ArchiveNotice = "archive reçue"
	ArchiveClash  = "identifiant déjà pris"
)

// archiveLabel nomme une archive par son fichier : sans manifeste, c'est tout
// ce qu'on sait d'où les copies viennent.
func archiveLabel(path string) string {
	name := filepath.Base(path)
	return "archive " + strings.TrimSuffix(name, filepath.Ext(name))
}

// ValidateArchives refuse d'emblée une archive qu'on ne saura pas lire.
func (r Request) ValidateArchives() error {
	for _, chemin := range r.Archives {
		if strings.TrimSpace(chemin) == "" {
			return valid.Errorf("Archive : un chemin vide.")
		}
		info, err := os.Stat(chemin)
		if err != nil {
			return valid.Errorf("Archive « %s » : %v.", chemin, err)
		}
		if info.IsDir() {
			return valid.Errorf(
				"Archive « %s » : c'est un dossier, et il faut un fichier ZIP.", chemin)
		}
	}
	return nil
}
