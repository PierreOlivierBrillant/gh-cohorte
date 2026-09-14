// Package registry tient le registre des utilisateurs d'une organisation : le
// nom complet en face de chaque compte GitHub, et le rôle tenu — étudiant ou
// enseignant —, rangé dans l'organisation elle-même plutôt que sur le poste de
// chacun.
//
// Une organisation nomme ses dépôts « session.cours.groupe.travail.étudiant »,
// et le dernier niveau est le nom slugifié, pas le compte. Rien dans les dépôts
// ne dit donc que « emilie-cote » est « @ecote » : c'est ce que le registre
// retient, et lui seul. Tant qu'il vivait sur un poste, un collègue ouvrant la
// même organisation n'y voyait que des slugs, et chaque groupe réécrivait les
// mêmes noms pour son compte — trois exemplaires d'une même personne, libres
// de diverger.
//
// Le registre est un fichier unique dans un dépôt de service de
// l'organisation. Une lecture complète coûte une requête ; deux mille
// utilisateurs sur cinq ans pèsent quelques centaines de kilo-octets. Le
// découper resterait possible : rien de ce qui suit ne dépend du nombre de
// fichiers.
//
// Le rôle qu'il porte nomme, il n'autorise pas. Savoir qu'un compte enseigne
// sert à le proposer, à le montrer, à chercher les cours qu'il a donnés ; ce
// qui décide réellement de ce que quelqu'un peut lire, ce sont les droits
// GitHub — appartenance à l'organisation, équipes, collaborateurs. Un étudiant
// ne peut pas se déclarer enseignant, non parce que ce fichier le lui refuse,
// mais parce qu'il n'a jamais eu le droit d'y écrire.
package registry

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Emplacement du registre dans l'organisation.
const (
	// RepoName est le dépôt de service qui le porte. Le point de tête le range
	// avec « .github », le signale comme dépôt de service, et le met hors
	// d'atteinte de la nomenclature : un nom à cinq niveaux ne peut pas
	// commencer par un niveau vide.
	RepoName = ".cohorte"
	// Branch est la seule branche écrite.
	Branch = "main"
	// UsersFile porte le registre lui-même. Il garde son nom d'origine bien
	// qu'il porte désormais aussi les enseignants : le renommer obligerait
	// chaque organisation déjà amorcée à une migration, pour un mot.
	UsersFile = "etudiants.json"
	// ReadmeFile explique le dépôt à qui l'ouvre sur github.com. C'est bien
	// « README.md » : GitHub n'affiche que celui-là sur la page du dépôt, et
	// c'est aussi le fichier que la création avec « auto_init » y dépose — le
	// nôtre prend sa place plutôt que de s'ajouter à côté.
	ReadmeFile = "README.md"
)

// Version est celle du schéma écrit. Elle est relue, jamais devinée : un
// fichier venu d'une version ultérieure de l'outil doit pouvoir se signaler.
//
// La version 2 ajoute le rôle et range les fiches sous « users ». Une version 1
// se relit telle quelle — sans rôle, tout le monde est étudiant —, et se
// réécrit en version 2 à la première écriture.
const Version = 2

// User est une personne connue de l'organisation : un étudiant, ou quelqu'un
// qui enseigne.
type User struct {
	Username string `json:"username"`
	FullName string `json:"full_name"`
	// IsTeacher dit que cette personne enseigne. Il est écrit pour tout le
	// monde, « false » compris : un champ absent se lirait « on ne sait pas »,
	// alors qu'on sait — le registre est la liste de ce qu'on sait.
	//
	// Ce n'est pas un droit, c'est une déclaration. Personne n'accède à quoi
	// que ce soit parce que ce champ vaut « true » ; c'est l'inverse — seul
	// quelqu'un qui a déjà le droit d'écrire ici peut le mettre à « true ».
	IsTeacher bool `json:"is_teacher"`
	// StudentID est le matricule du collège. C'est lui qui identifie vraiment
	// quelqu'un : deux comptes qui le portent sont la même personne, et deux
	// personnes du même nom ne le partagent pas. Le registre le retient pour
	// que ce soit vrai d'un poste à l'autre.
	//
	// Un enseignant n'en a pas : il n'est pas inscrit au collège. Son compte
	// GitHub suffit à le désigner.
	StudentID string `json:"student_id,omitempty"`
	// Slugs énumère tout ce qui a désigné cette personne au dernier niveau d'un
	// nom de dépôt. La liste s'allonge, ne se raccourcit pas : corriger
	// l'orthographe d'un nom ne doit pas rendre orphelins les dépôts déjà
	// créés sous l'ancien slug.
	//
	// C'est la différence avec ce que l'outil faisait jusqu'ici, où le
	// rapprochement slug → personne était reconstruit à la lecture depuis le
	// nom courant et quelques heuristiques. Ici il est déclaré.
	Slugs   []string `json:"slugs,omitempty"`
	AddedAt string   `json:"added_at,omitempty"`
}

// Key sert au rangement : le compte GitHub est insensible à la casse.
func (u User) Key() string { return strings.ToLower(strings.TrimSpace(u.Username)) }

// Person rend la personne telle que le reste de l'outil la manipule.
func (u User) Person() roster.Person {
	return roster.Person{
		FullName: u.FullName, Username: u.Username, StudentID: u.StudentID,
	}
}

// Role nomme ce que la personne est, tel que les trois interfaces l'écrivent.
// Le mot est décidé ici pour qu'il soit le même partout.
func (u User) Role() string {
	if u.IsTeacher {
		return RoleTeacher
	}
	return RoleStudent
}

// Les deux rôles. « Utilisateur » n'en est pas un : c'est le mot qui les
// rassemble, et il ne s'écrit jamais en face de quelqu'un.
const (
	RoleStudent = "étudiant"
	RoleTeacher = "enseignant"
)

// From compose la fiche d'une personne. Le slug que son nom complet produit y
// est joint d'emblée : c'est celui que porteront ses dépôts, et le retenir
// maintenant évite d'avoir à le deviner plus tard.
func From(person roster.Person) User {
	fiche := User{
		Username: person.Username, FullName: person.FullName,
		StudentID: person.StudentID,
	}
	if slug, err := naming.Student(person.FullName); err == nil {
		fiche.Slugs = []string{slug}
	}
	return fiche
}

// validate met une fiche en forme et refuse ce qui ne peut désigner personne.
// Le nom complet, lui, peut manquer : un compte adopté depuis des dépôts
// hérités n'a pas encore le sien.
func (u User) validate() (User, error) {
	username, err := valid.Login(u.Username, "Compte GitHub")
	if err != nil {
		return u, err
	}
	u.Username = username
	if nom := strings.TrimSpace(u.FullName); nom != "" {
		if u.FullName, err = valid.FullName(nom); err != nil {
			return u, err
		}
	} else {
		u.FullName = ""
	}
	u.StudentID = strings.TrimSpace(u.StudentID)
	u.Slugs = cleanSlugs(u.Slugs)
	return u, nil
}

// cleanSlugs met les slugs en forme, les dédoublonne et les range. L'ordre est
// fixe pour que deux écritures du même registre donnent le même fichier.
func cleanSlugs(slugs []string) []string {
	vus := map[string]bool{}
	propres := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		propre := strings.ToLower(strings.TrimSpace(slug))
		if propre == "" || vus[propre] {
			continue
		}
		vus[propre] = true
		propres = append(propres, propre)
	}
	sort.Strings(propres)
	if len(propres) == 0 {
		return nil
	}
	return propres
}

// ------------------------------------------------------------------ registre

// Set est le registre en mémoire. Il ne se modifie pas : chaque changement en
// produit un nouveau, si bien qu'une écriture refusée peut être rejouée sans
// craindre d'avoir déjà entamé celui qu'on avait en main.
type Set struct {
	users   []User // rangés par compte, casse ignorée
	byLogin map[string]int
	bySlug  map[string]int
}

// newSet range les fiches et dresse ses index.
func newSet(users []User) *Set {
	rangees := append([]User(nil), users...)
	sort.SliceStable(rangees, func(i, j int) bool {
		return rangees[i].Key() < rangees[j].Key()
	})
	set := &Set{
		users:   rangees,
		byLogin: make(map[string]int, len(rangees)),
		bySlug:  make(map[string]int, 2*len(rangees)),
	}
	for position, fiche := range rangees {
		set.byLogin[fiche.Key()] = position
		// Le compte désigne aussi la personne : les dépôts adoptés d'une
		// organisation sans convention le portent souvent à la place du nom.
		set.bySlug[fiche.Key()] = position
		for _, slug := range fiche.Slugs {
			set.bySlug[slug] = position
		}
	}
	return set
}

// Empty rend un registre vide : celui d'une organisation qu'on n'a pas encore
// amorcée.
func Empty() *Set { return newSet(nil) }

// Len compte les personnes connues.
func (s *Set) Len() int { return len(s.users) }

// All rend les fiches, rangées par compte. La copie évite qu'un appelant
// modifie le registre dans son dos.
func (s *Set) All() []User { return append([]User(nil), s.users...) }

// Find retrouve une personne par son compte GitHub.
func (s *Set) Find(username string) (User, bool) {
	position, connu := s.byLogin[strings.ToLower(strings.TrimSpace(username))]
	if !connu {
		return User{}, false
	}
	return s.users[position], true
}

// Teachers rend ceux qui enseignent, rangés par compte. C'est ce qui permet de
// chercher les cours qu'un collègue a déjà donnés, et de proposer un nom quand
// il faut désigner l'enseignant d'un groupe.
func (s *Set) Teachers() []User {
	enseignants := make([]User, 0)
	for _, fiche := range s.users {
		if fiche.IsTeacher {
			enseignants = append(enseignants, fiche)
		}
	}
	return enseignants
}

// Teaches dit si un compte est déclaré enseignant. Un compte inconnu ne
// l'est pas : le registre est la liste de ce qu'on sait, et ce qu'il ignore
// n'enseigne pas.
func (s *Set) Teaches(username string) bool {
	fiche, connu := s.Find(username)
	return connu && fiche.IsTeacher
}

// Name rend le nom complet d'un compte, ou une chaîne vide s'il est inconnu.
func (s *Set) Name(username string) string {
	fiche, connu := s.Find(username)
	if !connu {
		return ""
	}
	return fiche.FullName
}

// Resolve retrouve à qui appartient le dernier niveau d'un nom de dépôt.
//
// La marque que GitHub ajoute à un nom déjà pris — « -1 », puis « -2 » — n'en
// fait pas quelqu'un d'autre : « emilie-cote-1 » est « emilie-cote ». Elle
// n'est retirée qu'en dernier recours, car un vrai slug peut se terminer
// pareil.
func (s *Set) Resolve(slug string) (User, bool) {
	fragment := strings.ToLower(strings.TrimSpace(slug))
	if position, connu := s.bySlug[fragment]; connu {
		return s.users[position], true
	}
	base, marque := roster.WithoutDuplicateMarker(fragment)
	if !marque {
		return User{}, false
	}
	position, connu := s.bySlug[strings.ToLower(base)]
	if !connu {
		return User{}, false
	}
	return s.users[position], true
}

// Lookup répond à la question que « classroom » pose au registre : qui se
// cache derrière le dernier niveau d'un nom de dépôt ?
//
// Une fiche sans nom complet répond quand même. Le compte GitHub est déjà une
// réponse : il dit que ce dépôt est celui de quelqu'un qu'on connaît, et non
// d'un slug orphelin. Refuser de le dire rendait invisibles — donc
// innommables et indéplaçables — les personnes qu'on n'a jamais eu l'occasion
// de nommer, celles des dépôts repris qui portent leur compte.
func (s *Set) Lookup(fragment string) (roster.Person, bool) {
	fiche, trouve := s.Resolve(fragment)
	if !trouve {
		return roster.Person{}, false
	}
	return fiche.Person(), true
}

// ---------------------------------------------------------------- changement

// Change est ce qu'une écriture veut faire au registre.
//
// Elle décrit une intention — « ces personnes sont connues » —, non un état.
// C'est ce qui permet de la rejouer telle quelle sur un registre fraîchement
// relu quand quelqu'un a écrit entre-temps : le travail de l'autre est
// conservé entier, et rien n'est jamais fusionné.
type Change struct {
	// Learn fait connaître des personnes, ou complète ce qu'on sait d'elles.
	Learn []User
	// Forget retire des personnes, par compte. C'est rare et lourd de
	// conséquences : un compte oublié laisse ses dépôts sans nom.
	Forget []string
	// Roles change le rôle de comptes déjà connus.
	//
	// Il est à part de « Learn » exprès. Apprendre quelqu'un ne dit rien de
	// son rôle : une liste de classe importée ne sait pas qui enseigne, et si
	// elle pouvait l'écrire, réimporter la liste d'un groupe ferait
	// redescendre étudiant l'enseignant qui s'y trouve. Le rôle ne change donc
	// que lorsqu'on l'a demandé, et pour les comptes qu'on a nommés.
	Roles []RoleChange
	// Reason est ce que dira le message de commit. Vide, il est composé.
	Reason string
}

// RoleChange dit ce que devient le rôle d'un compte.
type RoleChange struct {
	Username  string `json:"username"`
	IsTeacher bool   `json:"is_teacher"`
}

// Learn compose le changement qui fait connaître des personnes.
func Learn(people ...roster.Person) Change {
	fiches := make([]User, 0, len(people))
	for _, person := range people {
		fiches = append(fiches, From(person))
	}
	return Change{Learn: fiches}
}

// LearnSlug retient qu'un slug désigne un compte. C'est ce qu'on apprend en
// adoptant des dépôts déjà nommés autrement que par le nom complet.
func LearnSlug(username, slug string) Change {
	return Change{Learn: []User{{Username: username, Slugs: []string{slug}}}}
}

// SetRole compose le changement qui fait d'un compte un enseignant, ou l'en
// défait. C'est la cooptation : quelqu'un qui enseigne déjà reconnaît que
// quelqu'un d'autre enseigne aussi.
func SetRole(username string, teacher bool) Change {
	verbe := "Retire le rôle d'enseignant à @"
	if teacher {
		verbe = "Reconnaît @"
	}
	suite := " comme enseignant"
	if !teacher {
		suite = ""
	}
	return Change{
		Roles:  []RoleChange{{Username: username, IsTeacher: teacher}},
		Reason: verbe + strings.TrimSpace(username) + suite,
	}
}

// Empty dit qu'il n'y a rien à écrire.
func (c Change) Empty() bool {
	return len(c.Learn) == 0 && len(c.Forget) == 0 && len(c.Roles) == 0
}

// With applique un changement et rend le registre qui en résulte, avec un
// booléen qui dit s'il a bougé. Appliqué deux fois, le même changement donne
// le même résultat : c'est ce qui rend le rejeu sûr.
//
// La date est fournie par l'appelant plutôt que lue à l'horloge : celle du
// magasin quand il écrit, une date fixe quand un test veut deux exécutions
// identiques.
func (s *Set) With(change Change, today string) (*Set, bool, error) {
	fiches := s.All()
	position := make(map[string]int, len(fiches))
	for index, fiche := range fiches {
		position[fiche.Key()] = index
	}

	bouge := false
	for _, appris := range change.Learn {
		valide, err := appris.validate()
		if err != nil {
			return nil, false, err
		}
		index, connu := position[valide.Key()]
		if !connu {
			valide.AddedAt = today
			position[valide.Key()] = len(fiches)
			fiches = append(fiches, valide)
			bouge = true
			continue
		}
		fondu := merge(fiches[index], valide)
		if !same(fiches[index], fondu) {
			fiches[index] = fondu
			bouge = true
		}
	}

	// Le rôle se pose après l'apprentissage : coopter quelqu'un qu'on vient
	// d'apprendre doit marcher en une seule écriture.
	for _, role := range change.Roles {
		compte, err := valid.Login(role.Username, "Compte GitHub")
		if err != nil {
			return nil, false, err
		}
		index, connu := position[strings.ToLower(compte)]
		if !connu {
			return nil, false, valid.Errorf(
				"Le registre ne connaît pas @%s : son rôle ne peut pas être changé "+
					"tant qu'il n'y figure pas.", compte)
		}
		if fiches[index].IsTeacher == role.IsTeacher {
			continue
		}
		fiches[index].IsTeacher = role.IsTeacher
		bouge = true
	}

	if len(change.Forget) > 0 {
		oublies := map[string]bool{}
		for _, username := range change.Forget {
			oublies[strings.ToLower(strings.TrimSpace(username))] = true
		}
		restantes := make([]User, 0, len(fiches))
		for _, fiche := range fiches {
			if oublies[fiche.Key()] {
				bouge = true
				continue
			}
			restantes = append(restantes, fiche)
		}
		fiches = restantes
	}
	return newSet(fiches), bouge, nil
}

// merge fond ce qu'on vient d'apprendre dans ce qu'on savait déjà.
//
// Le rôle n'en fait pas partie : il ne s'apprend pas, il se décide. « Roles »
// est le seul chemin qui le change.
func merge(connu, appris User) User {
	// Un nom vide n'efface jamais un nom connu : apprendre un compte sans son
	// nom — ce que fait l'adoption de dépôts hérités — n'est pas l'oublier.
	if strings.TrimSpace(appris.FullName) != "" {
		connu.FullName = appris.FullName
	}
	// Le matricule non plus ne s'efface pas : c'est la seule chose qui
	// identifie vraiment quelqu'un, et un rapprochement qui l'ignore ne doit
	// pas faire oublier celui qu'une liste avait donné.
	if strings.TrimSpace(appris.StudentID) != "" {
		connu.StudentID = appris.StudentID
	}
	// Le compte garde son orthographe d'origine ; GitHub ne distingue pas la
	// casse, et en changer ferait un faux changement à chaque écriture.
	connu.Slugs = cleanSlugs(append(append([]string(nil), connu.Slugs...), appris.Slugs...))
	if connu.AddedAt == "" {
		connu.AddedAt = appris.AddedAt
	}
	return connu
}

// same compare deux fiches, pour n'écrire que ce qui a vraiment bougé : un
// commit qui ne change rien salit l'historique sans rien apprendre.
func same(left, right User) bool {
	return left.Username == right.Username && left.FullName == right.FullName &&
		left.StudentID == right.StudentID && left.AddedAt == right.AddedAt &&
		left.IsTeacher == right.IsTeacher &&
		strings.Join(left.Slugs, "\x00") == strings.Join(right.Slugs, "\x00")
}

// message compose ce que dira le commit. Il est écrit pour être lu : c'est
// l'historique du registre, et il tient lieu de traçabilité.
func (c Change) message() string {
	if raison := strings.TrimSpace(c.Reason); raison != "" {
		return raison
	}
	var parties []string
	if n := len(c.Learn); n > 0 {
		parties = append(parties, plural(n, "%d étudiant", "%d étudiants"))
	}
	if n := len(c.Forget); n > 0 {
		parties = append(parties, plural(n, "retire %d compte", "retire %d comptes"))
	}
	if len(parties) == 0 {
		return "Met le registre à jour"
	}
	return "Inscrit au registre : " + strings.Join(parties, ", ")
}

// plural accorde un décompte. Les messages de commit se lisent : « 1 étudiants »
// se remarque, et rien ne justifie de l'écrire.
func plural(count int, singulier, pluriel string) string {
	if count == 1 {
		return fmt.Sprintf(singulier, count)
	}
	return fmt.Sprintf(pluriel, count)
}

// -------------------------------------------------------------- lecture/écriture

// document est ce qui est écrit dans le dépôt.
//
// Les fiches se rangent sous « users » depuis la version 2, sous « students »
// avant elle. Les deux clés se relisent : une organisation amorcée par une
// version antérieure de l'outil ne doit pas perdre ses noms parce qu'un mot a
// changé. Seule « users » s'écrit.
type document struct {
	Version int    `json:"version"`
	Users   []User `json:"users"`
	// Legacy porte les fiches d'un fichier en version 1. Elle ne s'écrit
	// jamais : « omitempty » sur une tranche vide la tait.
	Legacy []User `json:"students,omitempty"`
}

// entries rend les fiches du document, d'où qu'elles viennent.
func (d document) entries() []User {
	if len(d.Users) > 0 {
		return d.Users
	}
	return d.Legacy
}

// Encode met le registre en forme. Le fichier est trié et indenté toujours
// pareil : c'est ce qui rend ses différences lisibles sur github.com, où deux
// personnes viendront les relire.
func (s *Set) Encode() ([]byte, error) {
	payload, err := json.MarshalIndent(
		document{Version: Version, Users: s.users}, "", "  ")
	if err != nil {
		return nil, valid.Errorf("Registre illisible à l'écriture : %v", err)
	}
	return append(payload, '\n'), nil
}

// Decode relit le registre. Une fiche que rien ne peut désigner est écartée et
// signalée, jamais fatale : quelqu'un modifiera ce fichier à la main sur
// github.com, et une virgule de trop ne doit pas priver toute l'organisation
// de ses noms.
func Decode(content []byte) (*Set, []string) {
	var lu document
	if err := json.Unmarshal(content, &lu); err != nil {
		return Empty(), []string{"Registre illisible : " + err.Error()}
	}
	var soucis []string
	if lu.Version > Version {
		soucis = append(soucis, fmt.Sprintf(
			"Le registre est en version %d, l'outil en connaît %d : mettez-le à jour "+
				"avant d'y écrire.", lu.Version, Version))
	}
	lues := lu.entries()
	fiches := make([]User, 0, len(lues))
	vus := map[string]bool{}
	for _, fiche := range lues {
		valide, err := fiche.validate()
		if err != nil {
			soucis = append(soucis, "Fiche écartée : "+err.Error())
			continue
		}
		if vus[valide.Key()] {
			soucis = append(soucis, "Fiche en double écartée : @"+valide.Username)
			continue
		}
		vus[valide.Key()] = true
		fiches = append(fiches, valide)
	}
	return newSet(fiches), soucis
}

// Readme explique le dépôt à qui l'ouvre sur github.com sans savoir ce que
// c'est. Il est écrit une fois, au premier commit.
func Readme(org string) []byte {
	return []byte(`# Registre de ` + org + `

Ce dépôt appartient à l'extension ` + "`gh cohorte`" + `. Il retient deux choses :
**le nom complet en face de chaque compte GitHub** des utilisateurs de
l'organisation, et **le rôle** que chacun y tient — étudiant ou enseignant.

Les dépôts d'étudiants sont nommés ` + "`session.cours.groupe.travail.étudiant`" + `,
et le dernier niveau est le nom slugifié — pas le compte. Rien dans les noms de
dépôts ne dit donc à qui ils appartiennent : c'est ` + "`" + UsersFile + "`" + ` qui le dit,
pour tout le monde et depuis n'importe quel poste.

## Le rôle ne donne aucun droit

` + "`is_teacher`" + ` nomme, il n'autorise pas. Personne n'accède à quoi que ce
soit parce que ce champ vaut ` + "`true`" + ` : ce sont les droits GitHub —
appartenance à l'organisation, équipes, collaborateurs — qui décident de ce que
chacun peut lire. C'est l'inverse qui est vrai, et c'est ce qui protège le
champ : seul quelqu'un qui a déjà le droit d'écrire ici peut le mettre à
` + "`true`" + `, et un étudiant ne l'a jamais eu.

## Ce qu'il ne contient pas

Ni les inscriptions aux groupes, ni les réglages de distribution : ceux-là
restent sur le poste de la personne qui enseigne.

## Précautions

Ce dépôt porte des renseignements personnels. Il doit rester **privé** —
l'outil refuse d'y écrire s'il devient public — et son accès mérite d'être
restreint à l'équipe enseignante.

Il se modifie à la main sans risque : une fiche mal écrite est écartée et
signalée, elle ne rend pas le reste illisible.
`)
}
