package preload_test

import (
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/PierreOlivierBrillant/gh-milou/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-milou/internal/groups"
	"github.com/PierreOlivierBrillant/gh-milou/internal/identity"
	"github.com/PierreOlivierBrillant/gh-milou/internal/preload"
)

// lecteur joue le résolveur : il note ce qu'on lui demande, sans rien lire.
type lecteur struct {
	mutex sync.Mutex
	// avant s'ouvre quand le premier lot est parti, et celui-ci ne rend la
	// main qu'une fois « retenir » arrêté : c'est ce qui permet d'observer un
	// préchargement coupé en plein travail.
	avant   chan struct{}
	retenir *preload.Warmer
	// histoires est ce que la mémoire sait déjà des dépôts.
	histoires map[string]groups.Handin
	remises   []string
	// relus nomme les dépôts relus d'office, mémoire comprise.
	relus []string
	acces []string
	lots  int
}

func (l *lecteur) Handins(_ string, repos []string, jusqua identity.Reading,
	_ func(done, total int, repo string)) map[string]groups.Handin {
	if jusqua == identity.Refresh {
		l.relire(repos)
		return nil
	}
	l.note(&l.remises, repos, jusqua)
	return nil
}

func (l *lecteur) Histories(_ string, repos []string) map[string]groups.Handin {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	trouvees := make(map[string]groups.Handin, len(repos))
	for _, repo := range repos {
		if histoire, connue := l.histoires[repo]; connue {
			trouvees[repo] = histoire
		}
	}
	return trouvees
}

// relire note une relecture d'office. Elle n'a de sens que pour un historique
// déjà relevé qu'un envoi a dépassé : relire la mémoire pour autre chose, ce
// serait payer ce que le préchargement prétend épargner.
func (l *lecteur) relire(repos []string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for _, repo := range repos {
		if _, releve := l.histoires[repo]; !releve {
			panic("un préchargement ne relit d'office que ce qu'un envoi a dépassé")
		}
	}
	l.relus = append(l.relus, repos...)
	l.lots++
}

func (l *lecteur) Accesses(_ string, repos []string, jusqua identity.Reading,
	_ func(done, total int, repo string)) map[string]identity.Access {
	l.note(&l.acces, repos, jusqua)
	return nil
}

func (l *lecteur) note(cible *[]string, repos []string, jusqua identity.Reading) {
	l.mutex.Lock()
	// Un préchargement ne relit pas ce qu'on sait déjà : c'est ce qui le rend
	// gratuit au deuxième lancement.
	if jusqua != identity.Fetch {
		l.mutex.Unlock()
		panic("un préchargement ne doit aller chercher que ce qui manque")
	}
	*cible = append(*cible, repos...)
	l.lots++
	premier := l.lots == 1
	l.mutex.Unlock()

	// Le premier lot ne rend la main qu'une fois l'arrêt demandé : c'est
	// l'instant précis où le suivant doit renoncer à partir.
	if premier && l.retenir != nil {
		close(l.avant)
		for !l.retenir.Stopped() {
			runtime.Gosched()
		}
	}
}

func (l *lecteur) vu() (string, string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return strings.Join(l.remises, " "), strings.Join(l.acces, " ")
}

func (l *lecteur) relu() string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return strings.Join(l.relus, " ")
}

func cours(session, matiere, groupe string) classroom.Classroom {
	return classroom.Classroom{Org: "acme", Session: session, Course: matiere, Group: groupe}
}

func depots(noms ...string) []groups.RepoInfo {
	inventaire := make([]groups.RepoInfo, 0, len(noms))
	for _, nom := range noms {
		inventaire = append(inventaire, groups.RepoInfo{Name: nom, Private: true})
	}
	return inventaire
}

func TestReposNeGardeQueLaSessionLaPlusRecente(t *testing.T) {
	noms := preload.Repos(
		[]classroom.Classroom{cours("a26", "5n6", "01"), cours("h27", "5n6", "01"),
			cours("h27", "4w6", "02")},
		depots(
			"h27.5n6.01.tp1.emilie-cote",
			"h27.4w6.02.tp1.jlpicard",
			"a26.5n6.01.tp1.emilie-cote",
			"notes-du-cours",
		))
	// Les deux cours de l'hiver 2027, et rien de l'automne précédent : une
	// session passée ne recevra plus rien.
	if strings.Join(noms, " ") != "h27.5n6.01.tp1.emilie-cote h27.4w6.02.tp1.jlpicard" {
		t.Fatalf("dépôts préparés : %v", noms)
	}
}

// Deux groupes d'une même session partagent parfois un dépôt adopté : le
// préparer deux fois le lirait deux fois.
func TestReposNeNommePasDeuxFoisLeMemeDepot(t *testing.T) {
	noms := preload.Repos(
		[]classroom.Classroom{cours("a26", "5n6", "01"), cours("a26", "5n6", "01")},
		depots("a26.5n6.01.tp1.emilie-cote"))
	if len(noms) != 1 {
		t.Fatalf("dépôts préparés : %v", noms)
	}
}

func TestWarmPrepareLesDeuxLecturesUneSeuleFoisParOrganisation(t *testing.T) {
	journal := &lecteur{}
	chauffage := preload.New()
	liste := []classroom.Classroom{cours("a26", "5n6", "01")}
	inventaire := depots("a26.5n6.01.tp1.emilie-cote", "a26.5n6.01.tp1.jlpicard")

	chauffage.Warm(journal, "acme", liste, inventaire)
	chauffage.Wait()

	remises, acces := journal.vu()
	attendu := "a26.5n6.01.tp1.emilie-cote a26.5n6.01.tp1.jlpicard"
	if remises != attendu || acces != attendu {
		t.Fatalf("remises = %q, accès = %q", remises, acces)
	}

	// Le deuxième appel ne relance rien : ce qui a été lu est mémorisé, et le
	// relire en boucle coûterait le temps qu'on prétend gagner.
	chauffage.Warm(journal, "ACME", liste, inventaire)
	chauffage.Wait()
	if encore, _ := journal.vu(); encore != attendu {
		t.Fatalf("le préchargement a recommencé : %q", encore)
	}
}

// Un historique qu'un envoi a dépassé est relu d'office, si récent soit-il :
// l'écran ne le montre plus, et attendre qu'il vieillisse le laisserait vide.
// Les autres — jamais relevés, ou relevés et encore complets — ne sont lus que
// s'ils manquent.
func TestWarmRelitDOfficeLesHistoriquesDepasses(t *testing.T) {
	journal := &lecteur{histoires: map[string]groups.Handin{
		"a26.5n6.01.tp1.emilie-cote": {Seen: "2026-09-30T12:00:00Z"},
		"a26.5n6.01.tp1.jlpicard":    {Seen: "2026-09-30T12:00:00Z"},
	}}
	inventaire := []groups.RepoInfo{
		{Name: "a26.5n6.01.tp1.emilie-cote", PushedAt: "2026-09-30T18:00:00Z"},
		{Name: "a26.5n6.01.tp1.jlpicard", PushedAt: "2026-09-30T11:00:00Z"},
		{Name: "a26.5n6.01.tp1.nouveau", PushedAt: "2026-09-30T18:00:00Z"},
	}
	chauffage := preload.New()
	chauffage.Warm(journal, "acme", []classroom.Classroom{cours("a26", "5n6", "01")},
		inventaire)
	chauffage.Wait()

	if relus := journal.relu(); relus != "a26.5n6.01.tp1.emilie-cote" {
		t.Errorf("relus d'office : %q", relus)
	}
	if remises, _ := journal.vu(); remises != "a26.5n6.01.tp1.jlpicard a26.5n6.01.tp1.nouveau" {
		t.Errorf("lus s'ils manquent : %q", remises)
	}
}

func TestWarmNeFaitRienSansDepotDansLaSession(t *testing.T) {
	journal := &lecteur{}
	chauffage := preload.New()
	chauffage.Warm(journal, "acme", []classroom.Classroom{cours("a26", "5n6", "01")},
		depots("notes-du-cours"))
	chauffage.Wait()
	if remises, acces := journal.vu(); remises != "" || acces != "" {
		t.Fatalf("remises = %q, accès = %q", remises, acces)
	}
}

// Un arrêt demandé est pris entre deux lots : ce qui avait été lu est mémorisé,
// et le reste ne part pas. « Stop » attend le lot en route, sans quoi il
// écrirait dans le dos de qui vient de quitter.
func TestStopAbandonneLesLotsQuiRestent(t *testing.T) {
	chauffage := preload.New()
	journal := &lecteur{avant: make(chan struct{}), retenir: chauffage}

	noms := make([]string, 0, preload.Batch+1)
	for index := 0; index <= preload.Batch; index++ {
		noms = append(noms, "a26.5n6.01.tp1.etudiant"+string(rune('a'+index%26))+
			string(rune('a'+index/26)))
	}
	chauffage.Warm(journal, "acme", []classroom.Classroom{cours("a26", "5n6", "01")},
		depots(noms...))

	// Le premier lot est parti : c'est en plein travail qu'on abandonne.
	<-journal.avant
	chauffage.Stop()

	journal.mutex.Lock()
	lots := journal.lots
	journal.mutex.Unlock()
	if lots != 1 {
		t.Fatalf("%d lot(s) demandés : l'arrêt devait couper après le premier", lots)
	}
}

// Un arrêt déjà demandé ne lance plus rien : quitter ferme la porte.
func TestWarmNeCommencePasApresUnArret(t *testing.T) {
	journal := &lecteur{}
	chauffage := preload.New()
	chauffage.Stop()
	chauffage.Warm(journal, "acme", []classroom.Classroom{cours("a26", "5n6", "01")},
		depots("a26.5n6.01.tp1.emilie-cote"))
	chauffage.Wait()
	if remises, _ := journal.vu(); remises != "" {
		t.Fatalf("remises = %q", remises)
	}
}
