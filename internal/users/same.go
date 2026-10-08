package users

import (
	"slices"
	"strings"

	"github.com/PierreOlivierBrillant/gh-nestor/internal/registry"
	"github.com/PierreOlivierBrillant/gh-nestor/internal/valid"
)

// Réunir deux comptes d'une même personne se décide ici, une fois, pour les
// trois interfaces : qui peut le faire, ce qui est refusé, et ce qui l'emporte.
// Elles ne font que recueillir les deux comptes et montrer ce qui revient.
//
// La réunion s'écrit au registre, et nulle part ailleurs. C'est ce qui la fait
// valoir dans tous les groupes et sur tous les postes, et ce qui la rend
// réversible : aucune liste n'est touchée, aucun dépôt renommé ni retiré.
// Rien n'est jamais déduit du nom. Deux homonymes sont proposés en premier,
// à la rigueur ; c'est quelqu'un qui décide de les réunir.

// Joining est ce qu'une réunion fera, dit avant qu'elle ne s'écrive.
type Joining struct {
	// Account rejoint Principal : c'est Principal qui désigne la personne
	// ensuite, et sous lui que l'annuaire la montre.
	Account   string `json:"account"`
	Principal string `json:"principal"`
	// FullName, StudentID et IsTeacher sont ce qui l'emporte : le nom et le
	// matricule du compte qui désigne, ceux de l'autre à défaut ; et le rôle le
	// plus haut des deux.
	FullName  string `json:"full_name,omitempty"`
	StudentID string `json:"student_id,omitempty"`
	IsTeacher bool   `json:"is_teacher"`
	Role      string `json:"role"`
	// Accounts sont tous ses comptes une fois réunis, celui qui désigne
	// d'abord.
	Accounts []string `json:"accounts"`
	// Change est ce qui s'écrira au registre.
	Change registry.Change `json:"-"`
}

// Splitting est ce qu'une séparation fera.
type Splitting struct {
	// Account redevient une personne à lui seul ; Remaining restent ensemble.
	Account   string          `json:"account"`
	Remaining []string        `json:"remaining"`
	Change    registry.Change `json:"-"`
}

// Candidate est un compte qu'on peut réunir à une personne : celui qui désigne
// une autre ligne de l'annuaire.
type Candidate struct {
	FullName string   `json:"full_name,omitempty"`
	Username string   `json:"username"`
	Accounts []string `json:"accounts"`
	// SameName dit qu'elle porte le même nom. C'est une suggestion, pas une
	// preuve : deux homonymes sont deux personnes jusqu'à ce qu'on décide le
	// contraire.
	SameName bool `json:"same_name"`
}

// MayDecide dit si quelqu'un peut écrire une décision sur les personnes de
// l'organisation — coopter un enseignant, réunir deux comptes.
//
// Ce n'est pas elle qui protège le registre : un étudiant n'a jamais eu le
// droit d'écrire dans « .cohorte », et GitHub le lui refuserait bien avant.
// Elle dit le refus dans les mots de l'outil plutôt qu'en HTTP. Tant que
// l'organisation n'a aucun enseignant, le premier venu peut : quelqu'un doit
// pouvoir commencer.
func MayDecide(known Registry, viewer string) bool {
	return known == nil || known.Teaches(viewer) || len(known.Teachers()) == 0
}

// refuse dit à quelqu'un qu'il n'enseigne pas.
func refuse(viewer, org, geste string) error {
	return valid.Errorf("Seul un enseignant peut %s. @%s n'est pas déclaré enseignant "+
		"dans « %s ».", geste, viewer, org)
}

// Candidates énumère les personnes qu'on peut réunir à celle d'un compte :
// toutes les autres lignes de l'annuaire, celles qui portent le même nom
// d'abord.
func Candidates(rows []Row, account string) []Candidate {
	ligne, trouvee := rowOf(rows, account)
	nom := ""
	if trouvee {
		nom = valid.Slugify(ligne.FullName)
	}
	memes, autres := make([]Candidate, 0), make([]Candidate, 0, len(rows))
	for _, autre := range rows {
		if owns(autre, account) || autre.Username == "" {
			continue
		}
		candidat := Candidate{
			FullName: autre.FullName, Username: autre.Username, Accounts: autre.Accounts,
			SameName: nom != "" && valid.Slugify(autre.FullName) == nom,
		}
		if candidat.SameName {
			memes = append(memes, candidat)
			continue
		}
		autres = append(autres, candidat)
	}
	return append(memes, autres...)
}

// PlanJoin vérifie qu'on peut réunir deux comptes et dit ce que la réunion
// fera. « account » rejoint « principal », qui désigne la personne ensuite.
//
// Les lignes sont celles de l'annuaire : elles disent ce que les groupes
// savent de chacun — un nom, un matricule — que le registre ignore parfois.
func PlanJoin(rows []Row, known Registry, viewer, org, account, principal string) (Joining, error) {
	compte, err := valid.Login(account, "Compte GitHub")
	if err != nil {
		return Joining{}, err
	}
	garde, err := valid.Login(principal, "Même personne que")
	if err != nil {
		return Joining{}, err
	}
	if !MayDecide(known, viewer) {
		return Joining{}, refuse(viewer, org, "réunir deux comptes")
	}
	if strings.EqualFold(compte, garde) {
		return Joining{}, valid.Errorf(
			"@%s et @%s sont le même compte : il n'y a rien à réunir.", compte, garde)
	}
	if known != nil && holds(known.Accounts(compte), garde) {
		return Joining{}, valid.Errorf(
			"@%s et @%s sont déjà réunis : c'est une même personne.", compte, garde)
	}

	// Un compte qu'on ne voit nulle part est le plus souvent une faute de
	// frappe. Le réunir créerait au registre une fiche que rien ne désigne.
	deCompte, compteVu := rowOf(rows, compte)
	duGarde, gardeVu := rowOf(rows, garde)
	for _, cas := range []struct {
		compte string
		vu     bool
	}{{compte, compteVu}, {garde, gardeVu}} {
		if !cas.vu && (known == nil || len(known.Accounts(cas.compte)) == 0) {
			return Joining{}, valid.Errorf("@%s n'apparaît nulle part dans « %s » : ni "+
				"dans un groupe, ni au registre. Vérifiez son orthographe.", cas.compte, org)
		}
	}
	if !gardeVu {
		duGarde = Row{Username: garde, Accounts: []string{garde}}
	}
	if !compteVu {
		deCompte = Row{Username: compte, Accounts: []string{compte}}
	}
	if known != nil {
		duGarde.FullName = firstNonEmpty(duGarde.FullName, known.Name(garde))
		deCompte.FullName = firstNonEmpty(deCompte.FullName, known.Name(compte))
	}

	// Deux matricules différents sont deux personnes pour le collège : c'est
	// la seule chose qui identifie vraiment quelqu'un.
	if a, b := deCompte.StudentID, duGarde.StudentID; a != "" && b != "" &&
		!strings.EqualFold(a, b) {
		return Joining{}, valid.Errorf(
			"@%s porte le matricule %s et @%s le matricule %s : pour le collège, ce "+
				"sont deux personnes. Si l'un des deux est erroné, corrigez-le d'abord.",
			compte, a, garde, b)
	}

	reunion := Joining{
		Account: compte, Principal: garde,
		FullName:  firstNonEmpty(duGarde.FullName, deCompte.FullName),
		StudentID: firstNonEmpty(duGarde.StudentID, deCompte.StudentID),
		IsTeacher: duGarde.IsTeacher || deCompte.IsTeacher ||
			(known != nil && (known.Teaches(compte) || known.Teaches(garde))),
		Accounts: []string{garde},
		Change:   registry.Join(compte, garde),
	}
	reunion.Role = AsStudent
	if reunion.IsTeacher {
		reunion.Role = AsTeacher
	}
	for _, ligne := range []Row{duGarde, deCompte} {
		for _, autre := range accountsOf(ligne, known) {
			if !holds(reunion.Accounts, autre) {
				reunion.Accounts = append(reunion.Accounts, autre)
			}
		}
	}
	return reunion, nil
}

// PlanSplit vérifie qu'on peut séparer un compte de sa personne et dit ce que
// la séparation fera.
func PlanSplit(rows []Row, known Registry, viewer, org, account string) (Splitting, error) {
	compte, err := valid.Login(account, "Compte GitHub")
	if err != nil {
		return Splitting{}, err
	}
	if !MayDecide(known, viewer) {
		return Splitting{}, refuse(viewer, org, "séparer deux comptes")
	}
	var reunis []string
	if known != nil {
		reunis = known.Accounts(compte)
	}
	if len(reunis) < 2 {
		// Une liste de groupe peut déclarer deux comptes pour une personne.
		// Ce n'est pas au registre de le défaire : c'est la liste qu'il faut
		// corriger, et le dire évite de chercher un geste qui n'existe pas ici.
		if ligne, vue := rowOf(rows, compte); vue && len(ligne.Accounts) > 1 {
			return Splitting{}, valid.Errorf("@%s n'est réuni à aucun autre compte au "+
				"registre de « %s ». S'il paraît avec @%s, c'est la liste d'un groupe "+
				"de ce poste qui le dit : corrigez-la depuis ce groupe.", compte, org,
				strings.Join(slices.DeleteFunc(slices.Clone(ligne.Accounts), func(autre string) bool {
					return strings.EqualFold(autre, compte)
				}), ", @"))
		}
		return Splitting{}, valid.Errorf(
			"@%s n'est réuni à aucun autre compte : il n'y a rien à séparer.", compte)
	}
	restants := slices.DeleteFunc(slices.Clone(reunis), func(autre string) bool {
		return strings.EqualFold(autre, compte)
	})
	return Splitting{Account: compte, Remaining: restants, Change: registry.Split(compte)}, nil
}

// rowOf retrouve la ligne de l'annuaire qui porte un compte.
func rowOf(rows []Row, account string) (Row, bool) {
	for _, ligne := range rows {
		if owns(ligne, account) {
			return ligne, true
		}
	}
	return Row{}, false
}
