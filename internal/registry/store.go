package registry

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Le registre s'écrit par échange conditionnel, jamais par fusion.
//
// Celui qui écrit lit la tête de la branche, fabrique un commit qui en descend,
// puis demande à GitHub de faire avancer la référence sans forcer. GitHub
// refuse alors tout ce qui n'est pas une avance rapide : si quelqu'un a écrit
// entre-temps, le refus tombe, et l'écriture est refaite sur l'état frais.
//
// Ce qui est rejoué est le changement — « ces personnes sont connues » —, non
// le registre qu'on avait en main. Le travail de l'autre est donc conservé
// entier. Aucun clone, aucune branche, aucun « git merge » : il n'y a rien à
// résoudre, donc rien qui puisse rester en conflit.

// Attempts borne les reprises. Cinq suffisent très largement : il faudrait que
// cinq écritures se glissent coup sur coup entre notre lecture et la nôtre.
// Au-delà, ce n'est plus de la concurrence, c'est une panne.
const Attempts = 5

// Description est ce que le dépôt du registre annonce sur github.com.
const Description = "Registre des étudiants — gh cohorte. Privé : contient des renseignements personnels."

// step nomme l'étape qui a échoué, sans perdre l'erreur d'origine : son statut
// HTTP et la portée qui lui manque servent encore en aval.
//
// « HTTP 409 — Git Repository is empty. » ne disait pas ce qu'on faisait au
// moment où il est tombé, et cela a coûté cher à diagnostiquer. Une erreur du
// registre doit dire de quel dépôt et de quelle étape elle vient.
func (s *Store) step(quoi string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s de « %s/%s » : %w", quoi, s.org, RepoName, err)
}

// Store lit et écrit le registre d'une organisation.
type Store struct {
	client *ghapi.Client
	org    string
	now    func() time.Time
	// local retient ce qu'on a déjà lu, scellé par le commit d'où il vient.
	// Il peut être nil : le registre se lit alors toujours depuis GitHub.
	local *cache.Cache

	// mutex sérialise les écritures de ce processus. L'interface web sert
	// plusieurs onglets : deux d'entre eux ne doivent pas se disputer la tête
	// de la branche avant même d'atteindre GitHub, où ils s'excluraient au prix
	// d'un aller-retour.
	mutex sync.Mutex
}

// New ouvre le registre d'une organisation. Le cache peut être nil.
func New(client *ghapi.Client, org string, local *cache.Cache) *Store {
	return &Store{client: client, org: org, now: time.Now, local: local}
}

// Org renvoie l'organisation dont c'est le registre.
func (s *Store) Org() string { return s.org }

// Snapshot est le registre tel qu'il était à un commit donné.
type Snapshot struct {
	Set *Set
	// Head est le commit d'où il vient ; vide quand rien n'a encore été écrit.
	// C'est lui qui dit si le registre a bougé depuis la dernière lecture.
	Head string
	// Issues énumère ce que la lecture a dû écarter. Le fichier se modifie à la
	// main sur github.com : une fiche mal écrite doit se signaler, pas priver
	// toute l'organisation de ses noms.
	Issues []string
	// Seeded dit que le fichier du registre est déjà dans le dépôt. Sans lui,
	// le prochain commit doit aussi y déposer de quoi l'expliquer.
	Seeded bool
	// Stale dit que GitHub n'a pas répondu et que ce registre vient du disque.
	// L'appelant doit le montrer : afficher des noms périmés en silence serait
	// pire que d'en afficher aucun.
	Stale bool
}

// keptSet est ce que le cache local retient : le registre, et le commit qui le
// scelle. Tant que la branche pointe sur ce commit, ce contenu vaut toujours.
type keptSet struct {
	Head     string    `json:"head"`
	Students []Student `json:"students"`
}

// Load lit le registre.
//
// Un dépôt absent, ou un registre pas encore amorcé, donne un registre vide
// sans erreur : une organisation où l'on n'a rien écrit n'est pas une panne.
// Le fichier est lu au commit exact qu'on vient de relever, non à la branche :
// entre les deux requêtes, quelqu'un peut avoir écrit, et la lecture doit
// rester d'une seule pièce.
func (s *Store) Load() (Snapshot, error) { return s.load(true) }

// load lit le registre ; « offline » autorise le repli sur le disque.
//
// Une écriture ne s'en autorise jamais : elle a besoin de la tête réelle de la
// branche, et écrire depuis une lecture périmée serait de toute façon refusé.
func (s *Store) load(offline bool) (Snapshot, error) {
	head, err := s.client.BranchHead(s.org, RepoName, Branch)
	if err != nil {
		// Seule une panne de liaison justifie le repli. Un refus de GitHub —
		// jeton expiré, droit manquant — doit remonter tel quel : montrer des
		// noms périmés au lieu de dire « votre jeton a expiré » égarerait.
		if offline && ghapi.StatusOf(err) == 0 {
			if garde, connu := s.kept(); connu {
				garde.Stale = true
				return garde, nil
			}
		}
		return Snapshot{}, s.step("lecture de la branche", err)
	}
	if head == "" {
		return Snapshot{Set: Empty()}, nil
	}
	// Le commit relevé sert de sceau : tant qu'il n'a pas changé, ce qu'on a
	// déjà lu vaut toujours, et le fichier n'est pas retéléchargé. Une lecture
	// courante coûte donc une seule requête.
	if garde, connu := s.kept(); connu && garde.Head == head {
		garde.Seeded = true
		return garde, nil
	}
	file, err := s.client.ReadFile(s.org, RepoName, StudentsFile, head)
	if err != nil {
		return Snapshot{}, s.step("lecture du fichier "+StudentsFile, err)
	}
	if file == nil {
		return Snapshot{Set: Empty(), Head: head}, nil
	}
	set, soucis := Decode(file.Content)
	s.keep(head, set)
	return Snapshot{Set: set, Head: head, Issues: soucis, Seeded: true}, nil
}

// kept relit ce que le disque retient du registre.
func (s *Store) kept() (Snapshot, bool) {
	if s.local == nil {
		return Snapshot{}, false
	}
	var garde keptSet
	if !s.local.Get(cache.RegistryKey(s.org), cache.RegistryTTL, &garde) || garde.Head == "" {
		return Snapshot{}, false
	}
	return Snapshot{Set: newSet(garde.Students), Head: garde.Head}, true
}

// keep scelle sur le disque ce qu'on vient de lire.
func (s *Store) keep(head string, set *Set) {
	if s.local == nil || head == "" {
		return
	}
	s.local.Set(cache.RegistryKey(s.org), keptSet{Head: head, Students: set.All()})
}

// Apply applique un changement et rend le registre tel qu'il devient.
//
// Un changement qui ne change rien n'écrit rien : un commit sans effet salit
// l'historique sans rien apprendre à personne.
func (s *Store) Apply(change Change) (*Set, error) {
	if change.Empty() {
		snapshot, err := s.load(false)
		return snapshot.Set, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var dernier error
	for attempt := 1; attempt <= Attempts; attempt++ {
		snapshot, err := s.load(false)
		if err != nil {
			return nil, err
		}
		suivant, bouge, err := snapshot.Set.With(change, s.today())
		if err != nil {
			return nil, err
		}
		if !bouge {
			return snapshot.Set, nil
		}
		cree, err := s.ensureRepo()
		if err != nil {
			return nil, err
		}
		if cree {
			// Le dépôt vient de naître avec son premier commit : la tête
			// relevée avant lui n'existait pas. On recommence, c'est tout.
			continue
		}
		commit, err := s.commit(suivant, snapshot, change.message())
		if err == nil {
			s.keep(commit, suivant)
			return suivant, nil
		}
		if !errors.Is(err, ghapi.ErrNotFastForward) {
			return nil, err
		}
		dernier = err
	}
	return nil, valid.Errorf(
		"Registre de « %s » : %d tentatives d'écriture refusées coup sur coup. "+
			"Quelqu'un écrit sans arrêt, ou la branche « %s » a été forcée ailleurs (%v).",
		s.org, Attempts, Branch, dernier)
}

// today rend la date du jour, telle que le registre l'écrit.
func (s *Store) today() string { return s.now().Format("2006-01-02") }

// commit écrit le registre dans un commit qui descend de la tête relevée. Une
// tête vide crée la branche — c'est l'amorçage, et aussi le cas d'un
// « .cohorte » créé sans jamais avoir été rempli.
//
// Ce qui explique le dépôt n'est déposé qu'une fois, avec le registre lui-même,
// et la condition porte sur le fichier plutôt que sur la tête : un dépôt qui a
// des commits mais pas encore de registre mérite son explication autant qu'un
// dépôt neuf.
func (s *Store) commit(set *Set, snapshot Snapshot, message string) (string, error) {
	payload, err := set.Encode()
	if err != nil {
		return "", err
	}
	fichiers := []ghapi.PushFile{{Path: StudentsFile, Mode: "100644", Content: payload}}
	if !snapshot.Seeded {
		fichiers = append(fichiers, ghapi.PushFile{
			Path: ReadmeFile, Mode: "100644", Content: Readme(s.org)})
	}
	commit, err := s.client.PushFilesOnto(
		s.org, RepoName, fichiers, message, Branch, snapshot.Head)
	if err != nil && !errors.Is(err, ghapi.ErrNotFastForward) {
		// Le refus d'avance rapide n'est pas une panne : il est attendu, et la
		// boucle d'écriture le reconnaît. L'enrober le rendrait méconnaissable
		// pour elle — et de toute façon, il n'a rien à expliquer.
		return "", s.step("écriture", err)
	}
	return commit, err
}

// ensureRepo s'assure que le dépôt du registre existe et qu'il est privé. Le
// booléen dit qu'il vient d'être créé.
//
// Il naît avec son premier commit — « auto_init ». Un dépôt sans aucun commit
// n'est pas un dépôt git pour GitHub, qui répond « Git Repository is empty. »
// à qui vient y lire ; le faire naître garni évite cet état transitoire au lieu
// d'avoir à le traverser.
//
// Le registre porte des noms d'étudiants. Il est créé privé, et l'outil refuse
// d'y écrire s'il a été rendu public : mieux vaut une écriture qui échoue
// bruyamment qu'une liste de noms exposée sans que personne s'en aperçoive.
func (s *Store) ensureRepo() (bool, error) {
	repo, err := s.client.GetRepo(s.org, RepoName)
	if err != nil {
		return false, s.step("ouverture", err)
	}
	if repo == nil {
		if _, err := s.client.CreateOrgRepo(
			s.org, RepoName, true, Description, true); err != nil {
			return false, s.step("création", err)
		}
		return true, nil
	}
	if !repo.Private {
		return false, valid.Errorf(
			"Le dépôt « %s/%s » est public : il porte des noms d'étudiants et rien n'y "+
				"sera écrit tant qu'il le restera. Repassez-le en privé dans ses réglages "+
				"GitHub.", s.org, RepoName)
	}
	return false, nil
}

// ForgetHistory réécrit la branche du registre en un seul commit, sans passé.
//
// Un étudiant retiré du registre reste dans l'historique : c'est ce que git
// est. Cette opération repart d'un commit orphelin portant l'état courant, et
// la branche n'a plus rien derrière elle.
//
// Ce n'est pas un effacement au sens fort, et il ne faut pas le présenter
// comme tel. Les objets devenus inaccessibles restent un temps chez GitHub
// avant d'être ramassés, et un clone déjà fait garde tout ce qu'il avait. Ce
// qui est vrai, et rien de plus : plus rien de l'ancien n'est atteignable
// depuis la branche.
func (s *Store) ForgetHistory() (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	head, err := s.client.BranchHead(s.org, RepoName, Branch)
	if err != nil {
		return "", s.step("lecture de la branche", err)
	}
	if head == "" {
		return "", valid.Errorf(
			"Le registre de « %s » n'a rien écrit : il n'y a pas d'historique à effacer.", s.org)
	}
	tree, err := s.client.CommitTree(s.org, RepoName, head)
	if err != nil {
		return "", s.step("lecture du commit en place", err)
	}
	orphelin, err := s.client.CreateCommit(s.org, RepoName,
		"Repart du registre courant, sans son historique", tree, nil)
	if err != nil {
		return "", s.step("écriture du commit orphelin", err)
	}
	if err := s.client.ResetBranchHead(s.org, RepoName, Branch, orphelin); err != nil {
		return "", s.step("réécriture de la branche", err)
	}
	// Le sceau change : ce qu'on retenait du registre ne vaut plus.
	if s.local != nil {
		s.local.Forget(cache.RegistryKey(s.org))
	}
	return orphelin, nil
}

// ------------------------------------------------------------ confidentialité

// Exposure dit qui, dans l'organisation, peut lire le registre sans qu'on le
// lui ait donné.
//
// Les étudiants sont d'ordinaire collaborateurs externes de leur seul dépôt :
// ils ne voient rien du registre. Mais une organisation peut accorder d'office
// un droit de lecture à tous ses membres, et un département qui inscrit ses
// étudiants comme membres leur ouvrirait alors la liste de leurs camarades.
//
// GitHub ne montre ce réglage qu'aux propriétaires. Une chaîne vide veut donc
// dire « on ne sait pas », et l'absence de réponse n'est pas un feu vert : elle
// ne dit rien, et l'appelant doit le présenter ainsi.
// Teams énumère les équipes de l'organisation, pour qu'on puisse en désigner
// une. Un compte qui n'est pas membre n'en voit aucune : ce n'est pas une
// panne, il n'y a simplement rien à proposer.
func (s *Store) Teams() ([]ghapi.Team, error) {
	equipes, err := s.client.ListOrgTeams(s.org)
	if err != nil {
		return nil, fmt.Errorf("lecture des équipes de « %s » : %w", s.org, err)
	}
	return equipes, nil
}

// Grant donne à une équipe un droit de lecture sur le registre.
//
// Cela ouvre un accès ; cela n'en ferme aucun. Ce qui restreint réellement le
// registre, c'est la permission de base de l'organisation — d'où l'intérêt de
// la mettre à « none » et de nommer ici l'équipe enseignante, plutôt que de
// croire que désigner une équipe suffit. « Exposure » le dit à sa façon.
func (s *Store) Grant(team string) error {
	if strings.TrimSpace(team) == "" {
		return valid.Errorf("Aucune équipe désignée.")
	}
	if _, err := s.ensureRepo(); err != nil {
		return err
	}
	return s.step("accès de l'équipe « "+team+" »",
		s.client.GrantTeamRepo(s.org, team, s.org, RepoName, TeamPermission))
}

// TeamPermission est le droit accordé à l'équipe enseignante : lire et écrire.
// Le registre se corrige à plusieurs, et le limiter à la lecture obligerait à
// passer par une seule personne.
const TeamPermission = "push"

func (s *Store) Exposure() string {
	// L'organisation est demandée sous la forme qui tolère un refus : ne pas
	// pouvoir lire ce réglage n'est pas une panne, c'est une ignorance.
	org, err := s.client.GetRepoOwnerOrg(s.org)
	if err != nil || org == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(org.DefaultRepositoryPermission)) {
	case "read", "write", "admin":
		return fmt.Sprintf(
			"Tout membre de « %s » a d'office un droit « %s » sur ses dépôts : si vos "+
				"étudiants y sont membres, ils peuvent lire le registre. Restreignez "+
				"« %s » à l'équipe enseignante, ou passez la permission de base à « none ».",
			s.org, org.DefaultRepositoryPermission, RepoName)
	}
	return ""
}
