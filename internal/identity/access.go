package identity

import (
	"sort"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
)

// À qui un dépôt appartient-il ? Son nom le dit mal : « kickmyb-firebase-alice »
// se découpe aussi bien en « kickmyb » et « firebase-alice », et le compte qu'on
// en tire alors n'existe pas. Les accès, eux, ne se devinent pas — la personne
// qui travaille dans un dépôt y a été ajoutée, et c'est cette liste-là qui fait
// foi.
//
// L'invitation compte autant que l'accès établi : un étudiant qui n'a pas encore
// cliqué sur le courriel de GitHub n'est pas collaborateur, et son dépôt est
// pourtant bien le sien.
//
// La lecture coûte deux requêtes par dépôt, comme celle des historiques : elle
// passe donc par le même chemin — mémorisée, demandée en parallèle, et
// interrogeable sans réseau. Un écran montre alors ce qu'on sait déjà sans
// rien redemander, et c'est un geste explicite qui va chercher le reste.

// Invitation est une personne invitée sur un dépôt qui n'a pas encore accepté.
// Son identifiant y est joint : il n'y a pas d'autre moyen de désigner une
// invitation pour l'annuler.
type Invitation struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// Access dit qui a accès à un dépôt : les collaborateurs directs, et les
// personnes invitées qui n'ont pas encore accepté. C'est ce que le cache
// retient, et c'est tel quel que les interfaces le montrent.
type Access struct {
	Repo          string       `json:"repo"`
	Collaborators []string     `json:"collaborators"`
	Invitations   []Invitation `json:"invitations"`
}

// Logins nomme tous les comptes qui ont accès au dépôt, les invités compris :
// pour qui cherche à qui le dépôt appartient, une invitation en attente vaut un
// accès.
func (a Access) Logins() []string {
	vus := map[string]bool{}
	comptes := make([]string, 0, len(a.Collaborators)+len(a.Invitations))
	ajouter := func(login string) {
		login = strings.TrimSpace(login)
		if login == "" || vus[strings.ToLower(login)] {
			return
		}
		vus[strings.ToLower(login)] = true
		comptes = append(comptes, login)
	}
	for _, login := range a.Collaborators {
		ajouter(login)
	}
	for _, invitation := range a.Invitations {
		ajouter(invitation.Login)
	}
	sort.Slice(comptes, func(i, j int) bool {
		return strings.ToLower(comptes[i]) < strings.ToLower(comptes[j])
	})
	return comptes
}

// Owner est ce qu'on a appris d'un dépôt.
type Owner struct {
	// Login est le compte de la personne à qui le dépôt appartient. Vide quand
	// les accès n'ont pas tranché.
	Login string
	// Access nomme tous les comptes qui y ont accès, l'enseignant compris.
	// C'est ce que le cache retient, et ce qu'une revue peut montrer.
	Access []string
}

// Owners rend, pour chaque dépôt, le compte que ses accès désignent.
//
// Le viewer — celui qui se sert de l'outil — est écarté d'office : il a accès à
// tout, et n'est donc l'indice de rien. Les dépôts déjà connus ne coûtent aucun
// appel ; les autres sont demandés en parallèle, comme les profils.
func (r *Resolver) Owners(org string, repos []string, viewer string,
	onProgress func(done, total int, repo string)) map[string]Owner {
	lus := r.Accesses(org, repos, Fetch, onProgress)
	trouves := make(map[string]Owner, len(lus))
	for repo, acces := range lus {
		trouves[repo] = decider(repo, acces.Logins(), viewer)
	}
	return trouves
}

// Accesses rend les accès de chaque dépôt. La règle de lecture est celle des
// remises : « Cached » ne demande rien, « Fetch » va chercher ce qui manque,
// « Refresh » relit tout. C'est ce qui permet de préparer les accès d'avance et
// de montrer sans attendre ce qu'on en sait déjà.
func (r *Resolver) Accesses(org string, repos []string, jusqua Reading,
	onProgress func(done, total int, repo string)) map[string]Access {
	trouves := make(map[string]Access, len(repos))
	var manquants []string
	for _, repo := range repos {
		// Une entrée d'une forme antérieure — une simple liste de comptes —
		// ne se décode pas ici : elle compte pour un dépôt qu'on n'a pas lu.
		var acces Access
		if jusqua != Refresh &&
			r.store.Get(cache.AccessKey(org, repo), cache.AccessTTL, &acces) {
			trouves[repo] = acces
			continue
		}
		manquants = append(manquants, repo)
	}
	if jusqua == Cached || len(manquants) == 0 || r.client == nil {
		return trouves
	}
	for repo, acces := range r.fetchAccesses(org, manquants, onProgress) {
		trouves[repo] = acces
	}
	return trouves
}

// AccessOf relève les accès d'un seul dépôt et dit ce qui a manqué. À l'unité,
// un dépôt illisible est une réponse qu'il faut montrer : c'est un panneau
// ouvert sur un dépôt précis, pas un relevé d'ensemble où un absent se tait.
func (r *Resolver) AccessOf(org, repo string, jusqua Reading) (Access, error) {
	var memorise Access
	if jusqua != Refresh &&
		r.store.Get(cache.AccessKey(org, repo), cache.AccessTTL, &memorise) {
		return memorise, nil
	}
	if jusqua == Cached || r.client == nil {
		return Access{Repo: repo, Collaborators: []string{}, Invitations: []Invitation{}}, nil
	}
	acces, err := r.accessOf(org, repo)
	if err != nil {
		return acces, err
	}
	r.store.Set(cache.AccessKey(org, repo), acces)
	return acces, nil
}

// ForgetAccess oublie ce qu'on savait des accès d'un dépôt. Une invitation
// qu'on vient d'envoyer ou de retirer rend la mémoire fausse à l'instant : la
// garder ferait mentir l'écran jusqu'à sa péremption.
func (r *Resolver) ForgetAccess(org, repo string) {
	r.store.Forget(cache.AccessKey(org, repo))
}

// fetchAccesses interroge GitHub pour les dépôts qu'on ne connaît pas encore.
//
// Un dépôt dont la lecture échoue est laissé de côté plutôt que mémorisé vide :
// un jeton sans droit dessus ferait croire, sinon, que personne n'y a accès.
func (r *Resolver) fetchAccesses(org string, repos []string,
	onProgress func(done, total int, repo string)) map[string]Access {
	workers := r.jobs
	if workers > len(repos) {
		workers = len(repos)
	}
	type resultat struct {
		repo  string
		acces Access
		err   error
	}
	file := make(chan string)
	sorties := make(chan resultat)
	var groupe sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		groupe.Add(1)
		go func() {
			defer groupe.Done()
			for repo := range file {
				acces, err := r.accessOf(org, repo)
				sorties <- resultat{repo: repo, acces: acces, err: err}
			}
		}()
	}
	go func() {
		for _, repo := range repos {
			file <- repo
		}
		close(file)
		groupe.Wait()
		close(sorties)
	}()

	trouves := make(map[string]Access, len(repos))
	appris := map[string]any{}
	faits := 0
	for item := range sorties {
		faits++
		if item.err == nil {
			// Un dépôt sans accès direct est mémorisé aussi : c'est une
			// réponse, et y revenir coûterait le même appel pour la même chose.
			trouves[item.repo] = item.acces
			appris[cache.AccessKey(org, item.repo)] = item.acces
		}
		if onProgress != nil {
			onProgress(faits, len(repos), item.repo)
		}
	}
	r.store.SetMany(appris)
	return trouves
}

// accessOf relève qui a accès à un dépôt : les collaborateurs directs, et les
// personnes invitées qui n'ont pas encore accepté. L'ordre est celui de GitHub.
func (r *Resolver) accessOf(org, repo string) (Access, error) {
	acces := Access{Repo: repo, Collaborators: []string{}, Invitations: []Invitation{}}

	collaborateurs, err := r.client.ListCollaborators(org, repo)
	if err != nil {
		return acces, err
	}
	for _, personne := range collaborateurs {
		if login := strings.TrimSpace(personne.Login); login != "" {
			acces.Collaborators = append(acces.Collaborators, login)
		}
	}
	invitations, err := r.client.ListInvitations(org, repo)
	if err != nil {
		return acces, err
	}
	for _, invitation := range invitations {
		if login := strings.TrimSpace(invitation.Invitee.Login); login != "" {
			acces.Invitations = append(acces.Invitations,
				Invitation{ID: invitation.ID, Login: login})
		}
	}
	return acces, nil
}

// decider choisit le compte que les accès désignent, une fois le viewer écarté.
func decider(repo string, acces []string, viewer string) Owner {
	candidats := make([]string, 0, len(acces))
	for _, compte := range acces {
		if viewer != "" && strings.EqualFold(compte, viewer) {
			continue
		}
		candidats = append(candidats, compte)
	}
	login, _ := groups.Owner(repo, candidats)
	return Owner{Login: login, Access: acces}
}
