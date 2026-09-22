package identity

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
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
	// Permission est le droit promis, tel qu'« AddCollaborator » l'attend :
	// c'est celui qu'un renvoi doit promettre à nouveau.
	Permission string `json:"permission,omitempty"`
	// Expired dit que le délai pour accepter est passé. L'invitation reste
	// affichée par GitHub, mais elle ne mène plus nulle part.
	Expired bool `json:"expired,omitempty"`
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

// Pending nomme les comptes invités qui n'ont pas encore accepté. Des accès
// qu'on n'a pas relevés n'en nomment aucun : ne pas savoir n'est pas savoir que
// personne n'attend.
func (a Access) Pending() []string {
	comptes := make([]string, 0, len(a.Invitations))
	for _, invitation := range a.Invitations {
		if login := strings.TrimSpace(invitation.Login); login != "" {
			comptes = append(comptes, login)
		}
	}
	return comptes
}

// InvitationState dit d'un mot où en est l'invitation d'une personne à son
// dépôt.
//
// Les mots vivent ici plutôt que dans une interface : « expirée » doit vouloir
// dire la même chose au navigateur et au terminal, et c'est lui qui décide
// qu'un renvoi a un sens.
type InvitationState string

const (
	// InvitationUnknown dit qu'on n'a pas regardé : les accès du dépôt n'ont
	// pas été relevés, et rien ne s'en conclut.
	InvitationUnknown InvitationState = ""
	// InvitationAccepted dit que la personne a accès au dépôt.
	InvitationAccepted InvitationState = "acceptée"
	// InvitationPending dit que l'invitation attend une réponse, et qu'il est
	// encore temps d'y répondre.
	InvitationPending InvitationState = "en attente"
	// InvitationExpired dit que le délai est passé : sans nouvelle
	// invitation, la personne n'entrera jamais dans son dépôt.
	InvitationExpired InvitationState = "expirée"
	// InvitationNone dit que la personne n'a ni accès ni invitation : elle n'a
	// jamais été invitée, elle a refusé, ou son invitation a été annulée —
	// GitHub ne garde la trace d'aucun des trois. Il lui en faut une première.
	InvitationNone InvitationState = "sans invitation"
)

// InvitationOf dit où en est l'invitation de l'un de ces comptes. Sans compte
// donné — un dépôt dont on ne sait pas qui il vise —, tous ceux qui y ont accès
// ou y sont invités comptent.
//
// Un accès établi l'emporte sur tout : une personne qui a accepté sous l'un de
// ses comptes est entrée, quoi que dise une vieille invitation sous l'autre.
// Une invitation encore valable l'emporte ensuite sur une expirée : elle
// suffit, et en renvoyer une serait de trop.
//
// L'invitation rendue est celle qui a décidé de l'état, quand il y en a une :
// c'est elle qu'un renvoi remplace.
func (a Access) InvitationOf(accounts []string) (InvitationState, Invitation) {
	for _, login := range a.Collaborators {
		if vise(accounts, login) {
			return InvitationAccepted, Invitation{}
		}
	}
	var expiree *Invitation
	for index, invitation := range a.Invitations {
		if !vise(accounts, invitation.Login) {
			continue
		}
		if !invitation.Expired {
			return InvitationPending, invitation
		}
		if expiree == nil {
			expiree = &a.Invitations[index]
		}
	}
	if expiree != nil {
		return InvitationExpired, *expiree
	}
	return InvitationNone, Invitation{}
}

// vise dit qu'un compte est l'un de ceux-là. Sans compte donné, tous le sont.
func vise(accounts []string, login string) bool {
	if len(accounts) == 0 {
		return true
	}
	for _, compte := range accounts {
		if strings.EqualFold(strings.TrimSpace(compte), strings.TrimSpace(login)) {
			return true
		}
	}
	return false
}

// Expired rend les invitations dont le délai est passé.
func (a Access) Expired() []Invitation {
	var expirees []Invitation
	for _, invitation := range a.Invitations {
		if invitation.Expired {
			expirees = append(expirees, invitation)
		}
	}
	return expirees
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

// Dispatch est une invitation à envoyer, et ce que l'envoi en a fait. C'est
// soit une neuve à la place d'une invitation expirée, soit la première d'une
// personne qui n'en a pas — jamais invitée, invitation refusée ou annulée.
type Dispatch struct {
	Repo string `json:"repo"`
	// Invitation est celle qu'on remplace, ou, pour une première invitation,
	// le compte et le droit qu'on promet — son identifiant vaut alors zéro.
	Invitation Invitation `json:"invitation"`
	// State dit ce que GitHub a fait de la demande : une invitation, ou un
	// accès direct si la personne est membre de l'organisation. Vide tant que
	// rien n'est parti.
	State string `json:"state,omitempty"`
	// Error dit pourquoi rien n'est parti.
	Error string `json:"error,omitempty"`
	// err garde l'erreur elle-même : une interface qui n'envoie qu'une
	// invitation la rend telle quelle, avec ce que GitHub en disait.
	err error
}

// First dit qu'il n'y a rien à remplacer : c'est la première invitation.
func (d Dispatch) First() bool { return d.Invitation.ID == 0 }

// Err rend l'erreur de l'envoi, ou nil s'il a réussi.
func (d Dispatch) Err() error { return d.err }

// Summary dit en une phrase ce qu'un envoi a fait, ou fera. Le navigateur et
// le terminal la reprennent telle quelle : les deux doivent dire la même chose.
func (d Dispatch) Summary() string {
	compte := "@" + d.Invitation.Login + " (" + d.Invitation.Permission + ")"
	switch {
	case d.Error != "":
		return d.Error
	case d.State == ghapi.CollaboratorAdded:
		// La personne est membre de l'organisation : GitHub lui ouvre le dépôt
		// sans rien lui demander.
		return "@" + d.Invitation.Login + " a désormais accès à « " + d.Repo + " »."
	case d.State == "" && d.First():
		return "Première invitation à envoyer à " + compte + "."
	case d.State == "":
		return "Invitation expirée de " + compte + ", à remplacer."
	case d.First():
		return "Invitation envoyée à " + compte + "."
	}
	return "Nouvelle invitation envoyée à " + compte + "."
}

// Dispatches rend ce qu'il faut envoyer pour que l'un de ces comptes entre
// dans le dépôt, au droit donné pour une première invitation.
//
// Rien quand la personne est entrée, ou qu'une invitation l'attend encore : un
// second courriel ne servirait à rien. Une neuve à la place de chaque
// invitation expirée. Sinon, une première invitation à chacun de ses comptes :
// c'est ce que fait la distribution, et une personne qui travaille sous deux
// comptes doit pouvoir entrer par l'un comme par l'autre.
//
// Sans compte donné, on ne sait pas qui inviter : seules les invitations
// expirées se renvoient, puisqu'elles nomment déjà quelqu'un.
func (a Access) Dispatches(accounts []string, permission string) []Dispatch {
	etat, _ := a.InvitationOf(accounts)
	var envois []Dispatch
	switch etat {
	case InvitationExpired:
		for _, invitation := range a.Expired() {
			if vise(accounts, invitation.Login) {
				envois = append(envois, Dispatch{Repo: a.Repo, Invitation: invitation})
			}
		}
	case InvitationNone:
		for _, compte := range accounts {
			if compte = strings.TrimSpace(compte); compte != "" {
				envois = append(envois, Dispatch{Repo: a.Repo,
					Invitation: Invitation{Login: compte, Permission: permission}})
			}
		}
	}
	return envois
}

// Resend remplace une invitation par une nouvelle, au même compte et avec le
// même droit.
//
// L'invitation est relue avant qu'on y touche : une personne qui a accepté
// entre-temps n'a pas à recevoir un second courriel, et le droit promis est
// celui que GitHub dit, pas celui qu'un écran ancien croyait.
func (r *Resolver) Resend(org, repo string, id int64) (Dispatch, error) {
	acces, err := r.AccessOf(org, repo, Refresh)
	if err != nil {
		return Dispatch{}, err
	}
	for _, invitation := range acces.Invitations {
		if invitation.ID != id {
			continue
		}
		fait := r.envoyer(org, Dispatch{Repo: repo, Invitation: invitation})
		return fait, fait.err
	}
	return Dispatch{}, valid.Errorf("Cette invitation n'existe plus sur « %s » : "+
		"elle a été acceptée ou annulée entre-temps.", repo)
}

// Send envoie ces invitations, l'une après l'autre. Chacune dit ce qu'il en
// est advenu : un échec n'arrête pas les suivantes, et c'est au bilan de le
// dire. « onEach » reçoit chaque envoi dès qu'il est fait, pour qu'une
// interface le montre sans attendre la fin.
func (r *Resolver) Send(org string, envois []Dispatch,
	onEach func(done, total int, envoi Dispatch)) []Dispatch {
	faits := make([]Dispatch, 0, len(envois))
	touches := make([]string, 0, len(envois))
	vus := map[string]bool{}
	for index, envoi := range envois {
		fait := r.envoyer(org, envoi)
		faits = append(faits, fait)
		if !vus[fait.Repo] {
			vus[fait.Repo] = true
			touches = append(touches, fait.Repo)
		}
		if onEach != nil {
			onEach(index+1, len(envois), fait)
		}
	}
	// Les dépôts touchés sont relus : oubliés seulement, ils passeraient à
	// l'écran pour des dépôts qu'on n'a pas regardés, alors qu'on sait très
	// bien qu'une invitation neuve vient d'y partir.
	r.Accesses(org, touches, Refresh, nil)
	return faits
}

// envoyer fait partir une invitation, après avoir annulé celle qu'elle
// remplace.
//
// GitHub n'a pas de geste « renvoyer » pour une invitation à un dépôt : la
// seule façon d'en faire partir une neuve est de retirer l'ancienne. Si la
// seconde étape échoue, la personne n'a plus d'invitation du tout — l'ancienne
// ne menait déjà nulle part —, et l'erreur le dit pour qu'on l'invite à la main.
func (r *Resolver) envoyer(org string, envoi Dispatch) Dispatch {
	invitation := envoi.Invitation
	// Quoi qu'il arrive ensuite, ce qu'on savait des accès est faux.
	defer r.ForgetAccess(org, envoi.Repo)
	echouer := func(err error) Dispatch {
		envoi.err, envoi.Error = err, err.Error()
		return envoi
	}
	permission := invitation.Permission
	if permission == "" {
		permission = ghapi.Invitation{}.Permission()
	}
	if !envoi.First() {
		if err := r.client.CancelInvitation(org, envoi.Repo, invitation.ID); err != nil {
			return echouer(fmt.Errorf("@%s : l'ancienne invitation n'a pas pu être "+
				"annulée : %w", invitation.Login, err))
		}
	}
	etat, err := r.client.AddCollaborator(org, envoi.Repo, invitation.Login, permission)
	switch {
	case err != nil && envoi.First():
		return echouer(fmt.Errorf("@%s : l'invitation n'a pas pu partir : %w",
			invitation.Login, err))
	case err != nil:
		return echouer(fmt.Errorf("@%s : l'ancienne invitation est annulée, mais la "+
			"nouvelle n'a pas pu partir — invitez la personne depuis les accès du "+
			"dépôt : %w", invitation.Login, err))
	}
	envoi.State = etat
	envoi.Invitation.Permission = permission
	return envoi
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
			acces.Invitations = append(acces.Invitations, Invitation{
				ID: invitation.ID, Login: login, Permission: invitation.Permission(),
				Expired: invitation.Expired,
			})
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
