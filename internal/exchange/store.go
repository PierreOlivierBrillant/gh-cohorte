package exchange

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Le dépôt des index s'écrit comme le registre : par échange conditionnel.
//
// Celui qui publie lit la tête de la branche, fabrique un commit qui en
// descend, puis demande à GitHub de faire avancer la référence sans forcer. Si
// quelqu'un a publié entre-temps, le refus tombe et l'on recommence sur l'état
// frais. Comme chaque travail a son fichier, deux publications simultanées ne
// se marchent jamais dessus : elles se suivent, c'est tout.

// Attempts borne les reprises, comme pour le registre.
const Attempts = 5

// Store lit et écrit les index d'une organisation.
type Store struct {
	client *ghapi.Client
	org    string
	now    func() time.Time
}

// NewStore ouvre le dépôt des index d'une organisation.
func NewStore(client *ghapi.Client, org string) *Store {
	return &Store{client: client, org: org, now: time.Now}
}

// WithClock fixe l'horloge, pour que deux exécutions d'une épreuve coïncident.
func (s *Store) WithClock(now func() time.Time) *Store {
	s.now = now
	return s
}

// Read rend l'index publié d'un travail.
//
// Un dépôt absent, un fichier absent : ce n'est pas une panne, c'est
// simplement qu'il n'y a rien de publié. La différence compte — « personne n'a
// publié » appelle une demande, « GitHub refuse » appelle autre chose.
func (s *Store) Read(assignment string) (Published, bool, error) {
	head, err := s.client.BranchHead(s.org, IndexRepo, IndexBranch)
	if err != nil {
		if ghapi.StatusOf(err) == 404 {
			return Published{}, false, nil
		}
		return Published{}, false, s.step("lecture de la branche", err)
	}
	if head == "" {
		return Published{}, false, nil
	}
	file, err := s.client.ReadFile(s.org, IndexRepo, IndexPath(assignment), head)
	if err != nil {
		return Published{}, false, s.step("lecture de l'index", err)
	}
	if file == nil {
		return Published{}, false, nil
	}
	published, err := DecodePublished(file.Content)
	if err != nil {
		return Published{}, false, err
	}
	return published, true, nil
}

// Publish écrit l'index d'un travail.
func (s *Store) Publish(published Published) error {
	valide, err := published.Validate()
	if err != nil {
		return err
	}
	valide.PublishedAt = s.now().Format(time.RFC3339)
	contenu, err := EncodePublished(valide)
	if err != nil {
		return err
	}

	var dernier error
	for attempt := 1; attempt <= Attempts; attempt++ {
		head, err := s.head()
		if err != nil {
			return err
		}
		agi, err := s.ensureRepo(head)
		if err != nil {
			return err
		}
		if agi {
			// Le dépôt vient de naître : la tête relevée avant ne vaut plus.
			continue
		}

		fichiers := []ghapi.PushFile{{
			Path: IndexPath(valide.Assignment), Mode: "100644", Content: contenu,
		}}
		if head == "" {
			fichiers = append(fichiers, ghapi.PushFile{
				Path: IndexReadme, Mode: "100644", Content: IndexReadmeText(s.org),
			})
		}
		message := "Index de « " + valide.Assignment + " »"
		_, err = s.client.PushFilesOnto(s.org, IndexRepo, fichiers, message, IndexBranch, head)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ghapi.ErrNotFastForward) {
			return s.step("écriture de l'index", err)
		}
		dernier = err
	}
	return valid.Errorf(
		"Index de « %s » : %d écritures refusées coup sur coup. Quelqu'un publie "+
			"sans arrêt, ou la branche « %s » a été forcée ailleurs (%v).",
		published.Assignment, Attempts, IndexBranch, dernier)
}

// head relève la tête de la branche ; une branche absente rend une chaîne vide.
func (s *Store) head() (string, error) {
	head, err := s.client.BranchHead(s.org, IndexRepo, IndexBranch)
	if err != nil {
		if ghapi.StatusOf(err) == 404 {
			return "", nil
		}
		return "", s.step("lecture de la branche", err)
	}
	return head, nil
}

// ensureRepo s'assure que le dépôt existe et qu'il est privé. Le booléen dit
// qu'il a fallu agir : ce qu'on avait relevé ne vaut alors plus.
//
// Un index ne porte ni code ni nom, mais il porte tout de même la trace de ce
// qui a été remis : combien de copies, quelle taille, quels chemins de
// fichiers. Cela n'a rien à faire en public, et le dépôt est créé privé.
func (s *Store) ensureRepo(head string) (bool, error) {
	repo, err := s.client.GetRepo(s.org, IndexRepo)
	if err != nil {
		return false, s.step("lecture du dépôt", err)
	}
	// Un dépôt absent est rendu sans erreur : c'est un « nil », pas une panne.
	if repo == nil {
		if _, err := s.client.CreateOrgRepo(
			s.org, IndexRepo, true, IndexDescription, true); err != nil {
			return false, s.step("création du dépôt", err)
		}
		return true, nil
	}
	if !repo.Private {
		return false, valid.Errorf(
			"Dépôt « %s/%s » : il est public. Les index disent ce qui a été remis "+
				"et par combien de personnes ; ils n'ont rien à faire en public. "+
				"Repassez-le en privé avant de publier.", s.org, IndexRepo)
	}
	_ = head
	return false, nil
}

// step nomme l'étape qui a échoué sans perdre l'erreur d'origine : son statut
// et la portée qui lui manque servent encore en aval.
func (s *Store) step(quoi string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s de « %s/%s » : %w", quoi, s.org, IndexRepo, err)
}

// Grant donne à une équipe l'accès aux index.
//
// C'est la même équipe que celle du registre : ceux qui enseignent. Un index ne
// nomme personne, mais il dit ce qui a été remis, et cela suffit à ne pas
// l'ouvrir aux étudiants membres de l'organisation.
func (s *Store) Grant(team string) error {
	team = strings.TrimSpace(team)
	if team == "" {
		return valid.Errorf("Équipe : donnez son nom.")
	}
	return s.client.GrantTeamRepo(s.org, team, s.org, IndexRepo, "push")
}
