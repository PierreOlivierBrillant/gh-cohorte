package corpus

import (
	"fmt"
	"sort"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/similarity"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/tokens"
)

// Prévoir avant de lancer, plutôt que de se faire tuer au milieu.
//
// Une analyse sur cinq ans de dépôts peut demander plusieurs gigaoctets et une
// dizaine de minutes. Sur un poste, c'est long ; sur un runner de GitHub —
// deux cœurs, sept gigaoctets — c'est un échec au bout de vingt minutes, sans
// rien à montrer. Il faut donc le dire avant, et dire quoi faire.
//
// L'estimation n'est pas calculée sur des constantes : quelques dépôts sont
// vraiment récupérés, vraiment inspectés, vraiment analysés, et le reste est
// extrapolé de ce qu'ils ont coûté. C'est la seule façon d'être juste, parce
// que tout dépend du travail — un dépôt Flutter et un dépôt SQL n'ont pas le
// même rapport entre octets et jetons.
//
// Le même échantillon sert à montrer ce que le profil d'inspection retient,
// fichier par fichier : prévoir le coût et montrer ce qui entre sont la même
// opération, et les séparer coûterait deux fois le téléchargement.

// DefaultSample est le nombre de dépôts inspectés pour prévoir. Trois suffit à
// corriger les gros écarts sans peser sur le temps d'attente.
const DefaultSample = 3

// MostOfIt est la part au-delà de laquelle retirer un langage reviendrait à
// renoncer à l'analyse plutôt qu'à l'alléger.
const MostOfIt = 0.75

// BytesPerPrint est ce qu'une empreinte coûte en mémoire pendant la
// comparaison : la structure elle-même, sa place dans l'ensemble des empreintes
// distinctes d'une copie, et sa place dans l'index inversé. C'est une borne
// haute, mesurée large à dessein — une estimation qui sous-évalue la mémoire ne
// sert à rien.
const BytesPerPrint = 96

// Budget borne ce qu'une analyse a le droit de demander.
type Budget struct {
	Memory  int64
	Seconds float64
}

// RunnerBudget est ce qu'offre un runner de GitHub : deux cœurs, sept
// gigaoctets, et un travail qui ne devrait pas durer une heure.
var RunnerBudget = Budget{Memory: 7 << 30, Seconds: 20 * 60}

// Sampled est un dépôt inspecté pour l'aperçu.
type Sampled struct {
	ID      string            `json:"id"`
	Repo    string            `json:"repo"`
	Root    string            `json:"root"`
	Kept    []inspect.Kept    `json:"kept"`
	Skipped []inspect.Skipped `json:"skipped"`
	Problem *Problem          `json:"problem,omitempty"`
}

// Remedy est une façon chiffrée d'alléger l'analyse.
//
// « Restreignez le profil » n'aide personne : il faut dire lequel, et combien
// il fait gagner. Un remède qui ne s'accompagne pas d'un chiffre mesuré sur les
// vrais dépôts n'est qu'un conseil.
type Remedy struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
	// Saves est la part de jetons épargnée, entre 0 et 1.
	Saves float64 `json:"saves"`
	// Profile, Languages et Window disent quel réglage appliquer.
	Profile   string   `json:"profile,omitempty"`
	Languages []string `json:"languages,omitempty"`
	Window    int      `json:"window,omitempty"`
}

// Estimate est ce qu'une analyse coûtera.
type Estimate struct {
	Repos   int `json:"repos"`
	Sampled int `json:"sampled"`
	Pairs   int `json:"pairs"`
	Files   int `json:"files"`
	// Download est ce qui transite par le réseau — les archives entières —,
	// Bytes ce qui est réellement analysé une fois le tri fait. Les deux
	// comptent, et pas pour la même chose : le premier décide du temps
	// d'attente, le second de la mémoire.
	Download int64 `json:"download"`
	Bytes    int64 `json:"bytes"`
	Tokens   int64 `json:"tokens"`
	Prints   int64 `json:"prints"`
	Memory   int64 `json:"memory"`
	// Seconds couvre le téléchargement et l'analyse, mesurés sur l'échantillon.
	Seconds float64 `json:"seconds"`
	// Fits dit que l'analyse tient dans le budget donné.
	Fits     bool     `json:"fits"`
	Warnings []string `json:"warnings,omitempty"`
	Remedies []Remedy `json:"remedies,omitempty"`
}

// Preview est ce qu'un aperçu rend : ce que le profil retient, et ce que
// l'analyse coûtera.
type Preview struct {
	Samples  []Sampled `json:"samples"`
	Estimate Estimate  `json:"estimate"`
}

// Sample inspecte quelques dépôts et extrapole le coût de l'analyse entière.
func Sample(client Client, targets []Target, options Options, size int) Preview {
	if size < 1 {
		size = DefaultSample
	}
	if size > len(targets) {
		size = len(targets)
	}

	preview := Preview{Estimate: Estimate{
		Repos: len(targets), Sampled: size,
		Pairs: len(targets) * (len(targets) - 1) / 2,
	}}

	var fetched, analyzed time.Duration
	var downloaded, bytes, tokenCount, printCount, files int
	var sources [][]inspect.Source
	perLanguage := map[string]int{}
	measured := 0

	for _, target := range spread(targets, size) {
		start := time.Now()
		archive, err := client.Archive(target.Owner, target.Repo, target.Ref)
		fetched += time.Since(start)
		if err != nil {
			preview.Samples = append(preview.Samples, Sampled{
				ID: target.ID, Repo: target.FullName(), Problem: FetchProblem(target, err),
			})
			continue
		}
		downloaded += len(archive)
		unpacked, _, err := Untar(archive)
		if err != nil {
			preview.Samples = append(preview.Samples, Sampled{
				ID: target.ID, Repo: target.FullName(),
				Problem: &Problem{ID: target.ID, Repo: target.FullName(),
					Reason: Broken, Detail: err.Error()},
			})
			continue
		}

		selection := options.Inspector.Select(unpacked)
		start = time.Now()
		for _, kept := range selection.Kept {
			analysis := tokens.Lex(kept.Language, kept.Content)
			file, _ := similarity.Analyze(kept.Path, analysis, options.Kgram, options.Window)
			tokenCount += file.Tokens
			printCount += len(file.Prints)
			perLanguage[kept.Language] += file.Tokens
		}
		analyzed += time.Since(start)

		measured++
		files += len(selection.Kept)
		bytes += selection.Bytes()
		sources = append(sources, unpacked)
		preview.Samples = append(preview.Samples, Sampled{
			ID: target.ID, Repo: target.FullName(), Root: selection.Root,
			Kept: stripped(selection.Kept), Skipped: selection.Skipped,
		})
	}

	if measured == 0 {
		preview.Estimate.Warnings = append(preview.Estimate.Warnings,
			"Aucun des dépôts de l'échantillon n'a pu être lu : l'analyse entière "+
				"risque de ne rien rendre.")
		return preview
	}

	scale := float64(len(targets)) / float64(measured)
	preview.Estimate.Files = int(float64(files) * scale)
	preview.Estimate.Download = int64(float64(downloaded) * scale)
	preview.Estimate.Bytes = int64(float64(bytes) * scale)
	preview.Estimate.Tokens = int64(float64(tokenCount) * scale)
	preview.Estimate.Prints = int64(float64(printCount) * scale)
	preview.Estimate.Memory = preview.Estimate.Prints * BytesPerPrint
	preview.Estimate.Seconds = (fetched + analyzed).Seconds() / float64(measured) *
		float64(len(targets))

	preview.Estimate.Remedies = remedies(options, sources, perLanguage, tokenCount)
	return preview
}

// Against confronte une estimation à un budget et rend ce qu'il faut dire.
func (e Estimate) Against(budget Budget) Estimate {
	e.Fits = true
	if budget.Memory > 0 && e.Memory > budget.Memory {
		e.Fits = false
		e.Warnings = append(e.Warnings, fmt.Sprintf(
			"L'analyse demanderait environ %s de mémoire, au-delà des %s disponibles.",
			Bytes(e.Memory), Bytes(budget.Memory)))
	}
	if budget.Seconds > 0 && e.Seconds > budget.Seconds {
		e.Fits = false
		e.Warnings = append(e.Warnings, fmt.Sprintf(
			"L'analyse durerait environ %s, au-delà des %s prévues.",
			Duration(e.Seconds), Duration(budget.Seconds)))
	}
	return e
}

// remedies propose des allègements, chiffrés sur l'échantillon.
func remedies(options Options, sources [][]inspect.Source,
	perLanguage map[string]int, total int) []Remedy {

	found := make([]Remedy, 0, 4)
	if total == 0 {
		return found
	}

	// Retirer un langage. Le markdown est le cas le plus fréquent : les
	// consignes recopiées dans chaque dépôt pèsent parfois plus que le code.
	current := options.Inspector.Profile()
	kept := keptLanguages(perLanguage)
	for _, language := range kept {
		share := float64(perLanguage[language]) / float64(total)
		// Trop peu : le retirer ne fait rien gagner. Trop : le retirer ne
		// laisse plus rien à comparer, et « retirer Java — 97 % » sur un
		// travail en Java est un conseil qui se moque de qui le lit.
		if share < 0.15 || share > MostOfIt || len(kept) < 2 {
			continue
		}
		label, _ := tokens.Get(language)
		found = append(found, Remedy{
			Label:     "Retirer " + label.Label,
			Detail:    fmt.Sprintf("%s pèse %s des jetons analysés.", label.Label, percent(share)),
			Saves:     share,
			Languages: without(kept, language),
		})
	}

	// Restreindre le profil. La part épargnée est mesurée en réinspectant
	// vraiment l'échantillon avec le profil candidat, jamais devinée.
	if current.ID == inspect.AllProfile {
		for _, candidate := range inspect.Catalog(nil) {
			if candidate.ID == inspect.AllProfile {
				continue
			}
			saves, usable := wouldSave(candidate, options, sources, total)
			if !usable || saves < 0.15 {
				continue
			}
			found = append(found, Remedy{
				Label:   "Restreindre au profil « " + candidate.Label + " »",
				Detail:  fmt.Sprintf("%s des jetons en moins. %s", percent(saves), candidate.Note),
				Saves:   saves,
				Profile: candidate.ID,
			})
		}
	}

	// Élargir la fenêtre de winnowing : deux fois moins d'empreintes pour une
	// fenêtre deux fois plus large, au prix des passages recopiés les plus
	// courts, qui cessent d'être vus.
	found = append(found, Remedy{
		Label: "Élargir la fenêtre de winnowing",
		Detail: "Environ deux fois moins d'empreintes en mémoire. En " +
			"contrepartie, les passages recopiés les plus courts cessent d'être vus.",
		Saves:  0.5,
		Window: tokens.CodeWindow * 2,
	})

	sort.SliceStable(found, func(first, second int) bool {
		return found[first].Saves > found[second].Saves
	})
	return found
}

// wouldSave mesure ce qu'un autre profil épargnerait sur l'échantillon.
func wouldSave(candidate inspect.Profile, options Options,
	sources [][]inspect.Source, total int) (float64, bool) {

	inspector, err := inspect.New(inspect.Settings{Profile: candidate.ID}, nil)
	if err != nil {
		return 0, false
	}
	after := 0
	for _, unpacked := range sources {
		for _, kept := range inspector.Select(unpacked).Kept {
			analysis := tokens.Lex(kept.Language, kept.Content)
			file, _ := similarity.Analyze(kept.Path, analysis, options.Kgram, options.Window)
			after += file.Tokens
		}
	}
	// Un profil qui ne laisse rien n'est pas un remède : il ne convient pas au
	// travail, et le proposer ferait perdre du temps à qui l'essaierait.
	if after == 0 || total == 0 {
		return 0, false
	}
	return 1 - float64(after)/float64(total), true
}

// spread choisit l'échantillon en le répartissant plutôt qu'en prenant les
// premiers : les dépôts d'une même place se suivent dans la liste, et les
// premiers d'un groupe ne ressemblent pas forcément à ceux d'un autre.
func spread(targets []Target, size int) []Target {
	if size >= len(targets) {
		return targets
	}
	chosen := make([]Target, 0, size)
	step := float64(len(targets)) / float64(size)
	for index := 0; index < size; index++ {
		chosen = append(chosen, targets[int(float64(index)*step)])
	}
	return chosen
}

// stripped retire le contenu des fichiers retenus : l'aperçu dit quels
// fichiers entrent, il n'a pas à porter ce qu'ils contiennent.
func stripped(kept []inspect.Kept) []inspect.Kept {
	light := make([]inspect.Kept, 0, len(kept))
	for _, file := range kept {
		file.Content = nil
		light = append(light, file)
	}
	return light
}

func keptLanguages(perLanguage map[string]int) []string {
	names := make([]string, 0, len(perLanguage))
	for language, count := range perLanguage {
		if count > 0 {
			names = append(names, language)
		}
	}
	sort.Strings(names)
	return names
}

func without(languages []string, removed string) []string {
	kept := make([]string, 0, len(languages))
	for _, language := range languages {
		if language != removed {
			kept = append(kept, language)
		}
	}
	return kept
}

func percent(share float64) string { return fmt.Sprintf("%.0f %%", share*100) }

// Bytes met un poids en forme, pour être lu par quelqu'un.
func Bytes(count int64) string {
	switch {
	case count >= 1<<30:
		return fmt.Sprintf("%.1f Go", float64(count)/(1<<30))
	case count >= 1<<20:
		return fmt.Sprintf("%.0f Mo", float64(count)/(1<<20))
	case count >= 1<<10:
		return fmt.Sprintf("%.0f Ko", float64(count)/(1<<10))
	}
	return fmt.Sprintf("%d o", count)
}

// Duration met une durée en forme.
func Duration(seconds float64) string {
	switch {
	case seconds >= 3600:
		return fmt.Sprintf("%.1f h", seconds/3600)
	case seconds >= 60:
		return fmt.Sprintf("%.0f min", seconds/60)
	case seconds >= 1:
		return fmt.Sprintf("%.0f s", seconds)
	}
	return "moins d'une seconde"
}
