// Package registry tient le registre des étudiants d'une organisation : le nom
// complet en face de chaque compte GitHub, rangé dans l'organisation elle-même
// plutôt que sur le poste de chacun.
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
// l'organisation. Une lecture complète coûte une requête ; deux mille étudiants
// sur cinq ans pèsent quelques centaines de kilo-octets. Le découper resterait
// possible : rien de ce qui suit ne dépend du nombre de fichiers.
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
	// StudentsFile porte le registre lui-même.
	StudentsFile = "etudiants.json"
	// ReadmeFile explique le dépôt à qui l'ouvre sur github.com. C'est bien
	// « README.md » : GitHub n'affiche que celui-là sur la page du dépôt, et
	// c'est aussi le fichier que la création avec « auto_init » y dépose — le
	// nôtre prend sa place plutôt que de s'ajouter à côté.
	ReadmeFile = "README.md"
)

// Version est celle du schéma écrit. Elle est relue, jamais devinée : un
// fichier venu d'une version ultérieure de l'outil doit pouvoir se signaler.
const Version = 1

// Student est une personne connue de l'organisation.
type Student struct {
	Username string `json:"username"`
	FullName string `json:"full_name"`
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
func (s Student) Key() string { return strings.ToLower(strings.TrimSpace(s.Username)) }

// Person rend la personne telle que le reste de l'outil la manipule.
func (s Student) Person() roster.Person {
	return roster.Person{FullName: s.FullName, Username: s.Username}
}

// From compose la fiche d'une personne. Le slug que son nom complet produit y
// est joint d'emblée : c'est celui que porteront ses dépôts, et le retenir
// maintenant évite d'avoir à le deviner plus tard.
func From(person roster.Person) Student {
	fiche := Student{Username: person.Username, FullName: person.FullName}
	if slug, err := naming.Student(person.FullName); err == nil {
		fiche.Slugs = []string{slug}
	}
	return fiche
}

// validate met une fiche en forme et refuse ce qui ne peut désigner personne.
// Le nom complet, lui, peut manquer : un compte adopté depuis des dépôts
// hérités n'a pas encore le sien.
func (s Student) validate() (Student, error) {
	username, err := valid.Login(s.Username, "Compte GitHub")
	if err != nil {
		return s, err
	}
	s.Username = username
	if nom := strings.TrimSpace(s.FullName); nom != "" {
		if s.FullName, err = valid.FullName(nom); err != nil {
			return s, err
		}
	} else {
		s.FullName = ""
	}
	s.Slugs = cleanSlugs(s.Slugs)
	return s, nil
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
	students []Student // rangés par compte, casse ignorée
	byLogin  map[string]int
	bySlug   map[string]int
}

// newSet range les fiches et dresse ses index.
func newSet(students []Student) *Set {
	rangees := append([]Student(nil), students...)
	sort.SliceStable(rangees, func(i, j int) bool {
		return rangees[i].Key() < rangees[j].Key()
	})
	set := &Set{
		students: rangees,
		byLogin:  make(map[string]int, len(rangees)),
		bySlug:   make(map[string]int, 2*len(rangees)),
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
func (s *Set) Len() int { return len(s.students) }

// All rend les fiches, rangées par compte. La copie évite qu'un appelant
// modifie le registre dans son dos.
func (s *Set) All() []Student { return append([]Student(nil), s.students...) }

// Find retrouve une personne par son compte GitHub.
func (s *Set) Find(username string) (Student, bool) {
	position, connu := s.byLogin[strings.ToLower(strings.TrimSpace(username))]
	if !connu {
		return Student{}, false
	}
	return s.students[position], true
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
func (s *Set) Resolve(slug string) (Student, bool) {
	fragment := strings.ToLower(strings.TrimSpace(slug))
	if position, connu := s.bySlug[fragment]; connu {
		return s.students[position], true
	}
	base, marque := roster.WithoutDuplicateMarker(fragment)
	if !marque {
		return Student{}, false
	}
	position, connu := s.bySlug[strings.ToLower(base)]
	if !connu {
		return Student{}, false
	}
	return s.students[position], true
}

// Lookup répond à la question que « classroom » pose au registre : qui se
// cache derrière le dernier niveau d'un nom de dépôt ? Une fiche sans nom
// complet ne répond rien : elle n'apprendrait rien à qui la reçoit.
func (s *Set) Lookup(fragment string) (roster.Person, bool) {
	fiche, trouve := s.Resolve(fragment)
	if !trouve || strings.TrimSpace(fiche.FullName) == "" {
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
	Learn []Student
	// Forget retire des personnes, par compte. C'est rare et lourd de
	// conséquences : un compte oublié laisse ses dépôts sans nom.
	Forget []string
	// Reason est ce que dira le message de commit. Vide, il est composé.
	Reason string
}

// Learn compose le changement qui fait connaître des personnes.
func Learn(people ...roster.Person) Change {
	fiches := make([]Student, 0, len(people))
	for _, person := range people {
		fiches = append(fiches, From(person))
	}
	return Change{Learn: fiches}
}

// LearnSlug retient qu'un slug désigne un compte. C'est ce qu'on apprend en
// adoptant des dépôts déjà nommés autrement que par le nom complet.
func LearnSlug(username, slug string) Change {
	return Change{Learn: []Student{{Username: username, Slugs: []string{slug}}}}
}

// Empty dit qu'il n'y a rien à écrire.
func (c Change) Empty() bool { return len(c.Learn) == 0 && len(c.Forget) == 0 }

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

	if len(change.Forget) > 0 {
		oublies := map[string]bool{}
		for _, username := range change.Forget {
			oublies[strings.ToLower(strings.TrimSpace(username))] = true
		}
		restantes := make([]Student, 0, len(fiches))
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
func merge(connu, appris Student) Student {
	// Un nom vide n'efface jamais un nom connu : apprendre un compte sans son
	// nom — ce que fait l'adoption de dépôts hérités — n'est pas l'oublier.
	if strings.TrimSpace(appris.FullName) != "" {
		connu.FullName = appris.FullName
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
func same(left, right Student) bool {
	return left.Username == right.Username && left.FullName == right.FullName &&
		left.AddedAt == right.AddedAt &&
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
type document struct {
	Version  int       `json:"version"`
	Students []Student `json:"students"`
}

// Encode met le registre en forme. Le fichier est trié et indenté toujours
// pareil : c'est ce qui rend ses différences lisibles sur github.com, où deux
// personnes viendront les relire.
func (s *Set) Encode() ([]byte, error) {
	payload, err := json.MarshalIndent(
		document{Version: Version, Students: s.students}, "", "  ")
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
	fiches := make([]Student, 0, len(lu.Students))
	vus := map[string]bool{}
	for _, fiche := range lu.Students {
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

Ce dépôt appartient à l'extension ` + "`gh cohorte`" + `. Il retient une seule
chose : **le nom complet en face de chaque compte GitHub** des étudiants de
l'organisation.

Les dépôts d'étudiants sont nommés ` + "`session.cours.groupe.travail.étudiant`" + `,
et le dernier niveau est le nom slugifié — pas le compte. Rien dans les noms de
dépôts ne dit donc à qui ils appartiennent : c'est ` + "`" + StudentsFile + "`" + ` qui le dit,
pour tout le monde et depuis n'importe quel poste.

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
