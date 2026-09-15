package plagiarism

import (
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/corpus"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/similarity"
)

// Une session passée ne change plus. Ses dépôts ne recevront plus de commit,
// ses empreintes seront les mêmes dans un an qu'aujourd'hui — et pourtant, à
// chaque analyse portant sur cinq ans, on retéléchargerait cinq ans d'archives
// pour recalculer cinq ans d'empreintes identiques.
//
// Un rapport porte son index. Il suffit donc de le relire : ce qui n'a pas
// bougé depuis y est déjà, et seule la session en cours a vraiment besoin
// d'être récupérée. C'est ce qui rend une comparaison sur plusieurs années
// tenable, sur un poste comme sur un runner.
//
// Reste à savoir ce qui n'a pas bougé, et sans le demander à GitHub — sinon on
// n'aurait rien gagné. L'inventaire le dit déjà : il porte la date du dernier
// envoi de chaque dépôt. Un dépôt dont le dernier envoi précède l'analyse ne
// peut pas avoir changé depuis. L'inverse n'est pas vrai — une date absente ou
// postérieure ne prouve rien —, et le doute fait retélécharger : se tromper
// dans ce sens ne coûte qu'une requête, se tromper dans l'autre comparerait un
// état que le dépôt n'a plus.

// Reuse porte ce qu'on peut reprendre d'analyses précédentes.
type Reuse struct {
	works     map[string]similarity.Work
	inspected map[string]corpus.Inspected
	since     map[string]string
}

// Reusable dresse ce qui est repris d'anciens rapports, pour une demande donnée.
//
// Un rapport dont les réglages diffèrent n'est d'aucun secours : ses empreintes
// ont été calculées sur d'autres fichiers, avec d'autres bornes, et les mêler
// aux nôtres donnerait des mesures qui ne veulent rien dire. La comparaison des
// réglages est donc stricte.
func Reusable(request Request, prior []*Report) *Reuse {
	reuse := &Reuse{
		works:     map[string]similarity.Work{},
		inspected: map[string]corpus.Inspected{},
		since:     map[string]string{},
	}
	for _, report := range prior {
		if report == nil || !request.sameAnalysis(report.Request) {
			continue
		}
		dates := map[string]corpus.Inspected{}
		for _, inspected := range report.Inspected {
			dates[inspected.ID] = inspected
		}
		for _, work := range report.Index.Works {
			inspected, connu := dates[work.ID]
			if !connu {
				continue
			}
			// Le plus récent gagne : deux rapports peuvent porter la même
			// copie, et c'est le dernier état empreinté qui vaut.
			if ancien, deja := reuse.since[work.ID]; deja && ancien >= report.CreatedAt {
				continue
			}
			reuse.works[work.ID] = work
			reuse.inspected[work.ID] = inspected
			reuse.since[work.ID] = report.CreatedAt
		}
	}
	return reuse
}

// Split partage les copies entre celles qu'on reprend telles quelles et celles
// qu'il faut aller chercher.
func (r *Reuse) Split(targets []corpus.Target) (fresh []corpus.Target, kept []corpus.Target) {
	if r == nil {
		return targets, nil
	}
	for _, target := range targets {
		if r.usable(target) {
			kept = append(kept, target)
			continue
		}
		fresh = append(fresh, target)
	}
	return fresh, kept
}

// usable dit qu'une copie n'a pas bougé depuis qu'on l'a empreintée.
func (r *Reuse) usable(target corpus.Target) bool {
	analyzed, connu := r.since[target.ID]
	if !connu || target.PushedAt == "" {
		return false
	}
	if _, empreinte := r.works[target.ID]; !empreinte {
		return false
	}
	// Les deux dates sont au format RFC 3339 : les comparer comme des chaînes
	// les compare comme des instants, et sans dépendre d'un fuseau.
	return target.PushedAt < analyzed
}

// Take rend ce qui est repris pour ces copies.
func (r *Reuse) Take(targets []corpus.Target) ([]similarity.Work, []corpus.Inspected) {
	works := make([]similarity.Work, 0, len(targets))
	inspected := make([]corpus.Inspected, 0, len(targets))
	for _, target := range targets {
		work, connu := r.works[target.ID]
		if !connu {
			continue
		}
		// L'étiquette et le nom viennent de la demande d'aujourd'hui : un
		// étudiant dont on a corrigé le nom depuis doit paraître sous le bon,
		// même si son index a été calculé avant.
		work.Label, work.Origin = target.Label, target.Origin
		works = append(works, work)
		inspected = append(inspected, r.inspected[target.ID])
	}
	return works, inspected
}

// sameAnalysis dit que deux demandes produisent des empreintes comparables.
func (r Request) sameAnalysis(other Request) bool {
	return r.Kgram == other.Kgram && r.Window == other.Window &&
		sameSettings(r.Inspection.Profile, other.Inspection.Profile) &&
		sameSettings(r.Inspection.Root, other.Inspection.Root) &&
		r.Inspection.NoStrip == other.Inspection.NoStrip &&
		sameList(r.Inspection.Languages, other.Inspection.Languages) &&
		sameList(r.Inspection.Include, other.Inspection.Include) &&
		sameList(r.Inspection.Exclude, other.Inspection.Exclude)
}

func sameSettings(left, right string) bool { return left == right }

func sameList(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
