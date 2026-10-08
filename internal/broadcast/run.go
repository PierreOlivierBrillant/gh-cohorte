package broadcast

import (
	"bytes"
	"errors"

	"github.com/PierreOlivierBrillant/gh-milou/internal/ghapi"
)

// Status est l'issue d'un dépôt, ou ce qu'elle serait en simulation.
type Status string

// Issues possibles.
const (
	Added    Status = "ajouté"
	Replaced Status = "remplacé"
	UpToDate Status = "déjà à jour"
	// Kept dit qu'un fichier du même nom, au contenu différent, était déjà là,
	// et qu'on l'a laissé. C'est le choix par défaut : ce fichier est peut-être
	// déjà celui que l'étudiant a modifié, et l'écraser effacerait son travail
	// sans que l'historique ne le lui signale ailleurs que dans un commit.
	Kept   Status = "différent, conservé"
	Failed Status = "échec"
)

// Result est l'issue d'un dépôt.
type Result struct {
	Repo      string `json:"repo"`
	Recipient string `json:"recipient"`
	Path      string `json:"path"`
	Status    Status `json:"status"`
	Error     string `json:"error,omitempty"`
}

// Summary dit l'issue en quelques mots.
func (r Result) Summary() string {
	if r.Error != "" {
		return string(r.Status) + " : " + r.Error
	}
	return string(r.Status)
}

// Written rend les dépôts où le fichier a été écrit.
//
// Leur historique est à relire aussitôt : le commit qu'on vient d'y faire
// avance leur « pushed_at », et tant que l'historique relevé date d'avant, rien
// ne permet de dire que c'est l'enseignant qui a poussé — le dernier envoi
// montré serait celui du dépôt de fichier.
func Written(results []Result) []string {
	noms := make([]string, 0, len(results))
	for _, result := range results {
		if result.Status == Added || result.Status == Replaced {
			noms = append(noms, result.Repo)
		}
	}
	return noms
}

// Options règle l'exécution.
type Options struct {
	// Overwrite remplace un fichier déjà présent dont le contenu diffère.
	Overwrite bool
	// DryRun lit l'état de chaque dépôt sans rien y écrire : les issues
	// rendues sont celles qu'un dépôt réel aurait.
	DryRun bool
	// OnResult est appelé après chaque dépôt.
	OnResult func(done, total int, result Result)
}

// Client est ce que le dépôt demande à GitHub.
type Client interface {
	GetRepo(owner, repo string) (*ghapi.Repo, error)
	BranchHead(owner, repo, branch string) (string, error)
	ReadFile(owner, repo, file, ref string) (*ghapi.File, error)
	PutFile(owner, repo, file, branch, message string, content []byte) (string, error)
	PushFilesOnto(owner, repo string, files []ghapi.PushFile,
		message, branch, parent string) (string, error)
}

// Counts compte les issues.
func Counts(results []Result) map[Status]int {
	total := map[Status]int{}
	for _, result := range results {
		total[result.Status]++
	}
	return total
}

// Run dépose le fichier dans chaque dépôt, un dépôt après l'autre.
//
// Les écritures ne se font pas en parallèle : GitHub freine vite qui crée des
// commits en rafale, et un travail compte rarement plus de quelques dizaines de
// dépôts. Un échec n'arrête pas le lot, et relancer la même demande ne refait
// que ce qui manque — un dépôt déjà servi répond « déjà à jour ».
func Run(client Client, org string, plan *Plan, options Options) []Result {
	results := make([]Result, 0, len(plan.Items))
	for index, item := range plan.Items {
		result := deposit(client, org, item, options)
		results = append(results, result)
		if options.OnResult != nil {
			options.OnResult(index+1, len(plan.Items), result)
		}
	}
	return results
}

// deposit dépose le fichier dans un dépôt.
//
// Le commit descend de la tête qu'on vient de lire, et la branche n'avance que
// si elle en est toujours là : si l'étudiant pousse au même instant, GitHub
// refuse, et on relit avant de rejouer. Sans ce verrou, le commit effacerait
// ce que l'étudiant vient d'envoyer.
func deposit(client Client, org string, item Item, options Options) Result {
	result := Result{Repo: item.Repo, Recipient: item.Label(), Path: item.Path}
	fail := func(err error) Result {
		result.Status, result.Error = Failed, err.Error()
		return result
	}

	depot, err := client.GetRepo(org, item.Repo)
	if err != nil {
		return fail(err)
	}
	branch := "main"
	if depot != nil && depot.DefaultBranch != "" {
		branch = depot.DefaultBranch
	}

	const essais = 3
	for essai := 1; ; essai++ {
		head, err := client.BranchHead(org, item.Repo, branch)
		if err != nil {
			return fail(err)
		}
		var present *ghapi.File
		if head != "" {
			// Lu au commit plutôt qu'à la branche : c'est de ce commit que le
			// nouveau descendra, et c'est à lui que la comparaison doit se faire.
			if present, err = client.ReadFile(org, item.Repo, item.Path, head); err != nil {
				return fail(err)
			}
		}
		switch {
		case present == nil:
			result.Status = Added
		case bytes.Equal(present.Content, item.Content):
			result.Status = UpToDate
			return result
		case !options.Overwrite:
			result.Status = Kept
			return result
		default:
			result.Status = Replaced
		}
		if options.DryRun {
			return result
		}

		if head == "" {
			// Un dépôt sans aucun commit n'est pas encore un dépôt Git pour
			// l'API Git de GitHub : seule celle des contenus y écrit.
			_, err = client.PutFile(org, item.Repo, item.Path, branch, item.Message, item.Content)
		} else {
			_, err = client.PushFilesOnto(org, item.Repo, []ghapi.PushFile{{
				Path: item.Path, Mode: "100644", Content: item.Content,
			}}, item.Message, branch, head)
		}
		if err == nil {
			return result
		}
		if !errors.Is(err, ghapi.ErrNotFastForward) || essai == essais {
			return fail(err)
		}
	}
}
