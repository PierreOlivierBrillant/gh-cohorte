package classroom

import (
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/groups"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Une personne change de groupe en cours de session, ou son nom était mal
// orthographié : c'est fréquent. Sa fiche suit toujours ; ses dépôts, eux, ne
// bougent que si on le demande — les renommer est une écriture sur GitHub, pas
// un rangement local.
//
// Un travail entier se déplace aussi, et pour une autre raison. Une
// organisation adoptée sans convention range parfois sous un même préfixe —
// « travail-de » — les travaux de plusieurs groupes et de plusieurs sessions.
// Les séparer demande de sortir un travail de ce fourre-tout, et rien ne dit
// qui sont ses étudiants : c'est justement ce qu'on cherche à établir.
//
// D'où la règle qui tient tout : **le dernier niveau du nom est conservé tel
// quel quand rien ne permet de faire mieux**. Le dépôt arrive à la bonne place
// sous le fragment qu'il portait — souvent un compte GitHub —, le groupe le
// reconnaît quand même (voir « knownBy »), et le nom complet se corrige ensuite,
// une fois le groupe à la nomenclature courante. Sans cela, déplacer réclamerait
// un nom complet, et le retrouver réclamerait un groupe déplacé.
//
// Le plan se compose entièrement avant que le premier renommage ne parte : une
// collision refuse ainsi l'opération au lieu de l'interrompre à mi-chemin.

// Move est un dépôt à renommer parce que son étudiant a changé de groupe ou de
// nom, ou parce que son travail change de place.
type Move struct {
	Repo     string `json:"repo"`
	Target   string `json:"target"`
	Student  string `json:"student"`
	Username string `json:"username"`
}

// PlanMove compose le renommage des dépôts des personnes qui passent d'un
// groupe à l'autre. Un groupe d'arrivée qui ne suit pas la nomenclature
// courante ne sait pas nommer : le déplacement se fait alors sans les dépôts.
func PlanMove(depart, arrivee Classroom, personnes []roster.Person,
	repos []groups.RepoInfo) ([]Move, error) {
	if err := peutNommer(arrivee, "déplacez sans les dépôts"); err != nil {
		return nil, err
	}
	fragments := map[string]string{}
	for _, personne := range personnes {
		fragments[strings.ToLower(personne.Username)] = fragmentDe(personne)
	}
	return planRenames(depart, arrivee, fragments, repos)
}

// PlanRenameStudent compose le renommage des dépôts d'une personne dont le nom
// complet change. Le nom de l'étudiant est le dernier niveau du nom d'un dépôt :
// corriger sa fiche ne suffit pas, les dépôts déjà créés portent l'ancien.
//
// La personne est désignée par le compte qu'elle avait : c'est lui qui la
// retrouve dans le groupe tel qu'il est encore.
func PlanRenameStudent(cours Classroom, avant, apres roster.Person,
	repos []groups.RepoInfo) ([]Move, error) {
	if err := peutNommer(cours,
		"déplacez d'abord ses travaux à une place de la nomenclature courante"); err != nil {
		return nil, err
	}
	return planRenames(cours, cours,
		map[string]string{strings.ToLower(avant.Username): fragmentDe(apres)}, repos)
}

// peutNommer refuse une place qui ne sait pas nommer un dépôt. Le conseil
// change selon ce qu'on essayait de faire : c'est la seule chose qui distingue
// les deux refus.
func peutNommer(place Classroom, issue string) error {
	if !place.Legacy() {
		return nil
	}
	return valid.Errorf(
		"« %s » suit une nomenclature dépassée : ses dépôts ne peuvent pas être "+
			"nommés. Renommez-les d'abord, ou %s.", place.Label(), issue)
}

// fragmentDe rend le fragment qui nomme une personne dans un dépôt. Il est vide
// quand son nom complet manque : le dépôt gardera alors celui qu'il porte.
func fragmentDe(personne roster.Person) string {
	fragment, err := naming.Student(personne.FullName)
	if err != nil {
		return ""
	}
	return fragment
}

// planRenames compose les nouveaux noms : chaque dépôt du groupe de départ dont
// l'étudiant figure dans « fragments » rejoint la place d'arrivée sous le
// fragment qui lui est associé. Départ et arrivée peuvent être le même groupe —
// c'est le cas quand seul le nom de la personne change.
func planRenames(depart, arrivee Classroom, fragments map[string]string,
	repos []groups.RepoInfo) ([]Move, error) {
	pris := nouvellesCibles(repos)

	var lignes []Move
	for _, travail := range depart.Assignments(repos) {
		nom, err := naming.Fragment(depart.ShortName(travail.ID), "Travail")
		if err != nil {
			continue
		}
		for _, repo := range depart.Repos(travail.ID, repos) {
			student, inscrit := depart.StudentOf(repo.Name)
			if !inscrit {
				continue
			}
			fragment, concerne := fragments[strings.ToLower(student.Username)]
			if !concerne {
				continue
			}
			ligne, err := pris.viser(arrivee, nom, repo, fragment, student.Username)
			if err != nil {
				return nil, err
			}
			if ligne != nil {
				lignes = append(lignes, *ligne)
			}
		}
	}
	return lignes, nil
}

// ------------------------------------------------------ déplacer des travaux

// Relocation désigne un travail à déplacer et le nom qu'il portera à l'arrivée.
// Un nom laissé vide garde celui qu'il a — mais un travail tiré d'un préfixe
// fourre-tout mérite souvent mieux, et c'est le seul moment où le lui donner ne
// coûte rien.
type Relocation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PlanMoveAssignments compose le renommage des dépôts des travaux qui passent
// d'un groupe à un autre. Contrairement au déplacement d'une personne, il ne
// demande de connaître personne : un dépôt dont l'étudiant reste inconnu garde
// le fragment qu'il porte, et arrive quand même à la bonne place.
func PlanMoveAssignments(depart, arrivee Classroom, travaux []Relocation,
	repos []groups.RepoInfo) ([]Move, error) {
	if err := peutNommer(arrivee, "choisissez une autre place d'arrivée"); err != nil {
		return nil, err
	}
	if len(travaux) == 0 {
		return nil, valid.Errorf("Aucun travail à déplacer.")
	}
	connus := knownBy(append(append([]roster.Person(nil), depart.Students...),
		arrivee.Students...))
	pris := nouvellesCibles(repos)

	var lignes []Move
	for _, demande := range travaux {
		depots := depart.Repos(demande.ID, repos)
		if len(depots) == 0 {
			return nil, valid.Errorf("Aucun dépôt pour le travail « %s ».",
				depart.ShortName(demande.ID))
		}
		nom := strings.TrimSpace(demande.Name)
		if nom == "" {
			nom = depart.ShortName(demande.ID)
		}
		partantes, err := pris.travail(arrivee, nom, depots, connus)
		if err != nil {
			return nil, err
		}
		lignes = append(lignes, partantes...)
	}
	return lignes, nil
}

// PlanRelocate compose le renommage d'un lot de dépôts vers une place et un nom
// de travail donnés. C'est le même déplacement, dit sans groupe de départ :
// l'assistant du terminal ne connaît que des préfixes — un travail y est un
// préfixe et ses dépôts —, et il lui faut bien pouvoir en sortir un aussi.
//
// « connus » peut être vide : les dépôts gardent alors tous le dernier niveau
// de leur nom.
func PlanRelocate(arrivee Classroom, travail string, depots []groups.Repo,
	connus []roster.Person, repos []groups.RepoInfo) ([]Move, error) {
	if err := peutNommer(arrivee, "choisissez une autre place d'arrivée"); err != nil {
		return nil, err
	}
	if len(depots) == 0 {
		return nil, valid.Errorf("Aucun dépôt à déplacer.")
	}
	return nouvellesCibles(repos).travail(arrivee, travail, depots, knownBy(connus))
}

// Followers dit qui suit un déplacement de travaux, et qui quitte le groupe de
// départ. Une personne suit dès qu'un des dépôts qui partent est le sien ; elle
// ne quitte le départ que s'il ne lui en reste aucun — sinon elle serait
// inscrite là où elle n'a plus rien, et absente là où elle a encore quelque
// chose.
func Followers(depart Classroom, plan []Move, repos []groups.RepoInfo) (
	suivent []roster.Person, quittent []roster.Person) {
	partants := map[string]bool{}
	for _, ligne := range plan {
		partants[strings.ToLower(ligne.Repo)] = true
	}

	total := map[string]int{}
	emportes := map[string]int{}
	for _, travail := range depart.Assignments(repos) {
		for _, depot := range depart.Repos(travail.ID, repos) {
			student, inscrit := depart.StudentOf(depot.Name)
			if !inscrit {
				continue
			}
			cle := strings.ToLower(student.Username)
			total[cle]++
			if partants[strings.ToLower(depot.Name)] {
				emportes[cle]++
			}
		}
	}

	for _, personne := range depart.Students {
		cle := strings.ToLower(personne.Username)
		if emportes[cle] == 0 {
			continue
		}
		suivent = append(suivent, personne)
		if emportes[cle] >= total[cle] {
			quittent = append(quittent, personne)
		}
	}
	return suivent, quittent
}

// ------------------------------------------------------------------- cibles

// cibles retient les noms déjà portés et ceux que le plan vise déjà. Les deux
// se valent : GitHub refuserait le renommage dans un cas, et le plan se
// heurterait à lui-même dans l'autre.
type cibles struct {
	existants map[string]bool
	vises     map[string]string
}

func nouvellesCibles(repos []groups.RepoInfo) *cibles {
	pris := &cibles{
		existants: make(map[string]bool, len(repos)),
		vises:     map[string]string{},
	}
	for _, repo := range repos {
		pris.existants[strings.ToLower(repo.Name)] = true
	}
	return pris
}

// travail compose le renommage des dépôts d'un travail entier. Les noms visés
// restent retenus d'un appel à l'autre : deux travaux déplacés ensemble ne
// doivent pas se heurter.
func (c *cibles) travail(arrivee Classroom, nom string, depots []groups.Repo,
	connus map[string]roster.Person) ([]Move, error) {
	fragment, err := naming.Fragment(nom, "Travail")
	if err != nil {
		return nil, err
	}
	lignes := make([]Move, 0, len(depots))
	for _, depot := range depots {
		personne := connus[strings.ToLower(depot.Suffix)]
		ligne, err := c.viser(arrivee, fragment, depot,
			fragmentDe(personne), personne.Username)
		if err != nil {
			return nil, err
		}
		if ligne != nil {
			lignes = append(lignes, *ligne)
		}
	}
	return lignes, nil
}

// viser compose le nom d'arrivée d'un dépôt et le retient. Un fragment vide dit
// que rien de mieux n'est connu de l'étudiant : le dépôt garde le sien. Un
// dépôt qui porte déjà le nom visé n'a rien à faire dans le plan — il se
// heurterait à lui-même —, et rend une ligne nulle.
func (c *cibles) viser(arrivee Classroom, travail string, depot groups.Repo,
	fragment, username string) (*Move, error) {
	if fragment == "" {
		reprise, err := naming.Fragment(depot.Suffix, "Étudiant")
		if err != nil {
			return nil, valid.Errorf(
				"« %s » : le dernier niveau de son nom ne peut pas être repris. %v",
				depot.Name, err)
		}
		fragment = reprise
	}
	cible := naming.Compose(arrivee.Session, arrivee.Course, arrivee.Group,
		travail, fragment)
	if strings.EqualFold(cible, depot.Name) {
		return nil, nil
	}
	cle := strings.ToLower(cible)
	if precedent := c.vises[cle]; precedent != "" {
		return nil, valid.Errorf("« %s » et « %s » viseraient le même nom « %s ».",
			precedent, depot.Name, cible)
	}
	if c.existants[cle] {
		return nil, valid.Errorf("« %s » existe déjà.", cible)
	}
	c.vises[cle] = depot.Name
	return &Move{
		Repo: depot.Name, Target: cible, Student: fragment, Username: username,
	}, nil
}

// ------------------------------------------------------------------- listes

// Without rend le groupe privé des comptes donnés.
func (c Classroom) Without(usernames ...string) Classroom {
	partants := map[string]bool{}
	for _, username := range usernames {
		partants[strings.ToLower(strings.TrimSpace(username))] = true
	}
	restants := make([]roster.Person, 0, len(c.Students))
	for _, personne := range c.Students {
		if partants[strings.ToLower(personne.Username)] {
			continue
		}
		restants = append(restants, personne)
	}
	c.Students = restants
	return c
}

// With rend le groupe augmenté des personnes données, sans doublon de compte.
func (c Classroom) With(people ...roster.Person) Classroom {
	c.Students = dedupe(append(append([]roster.Person(nil), c.Students...), people...))
	return c
}
