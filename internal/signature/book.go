package signature

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Le registre des marques délivrées.
//
// Une marque est un jeton tiré au hasard : elle ne dit rien d'elle-même. Ce qui
// la relie à quelqu'un est ce fichier, et il vit dans le dépôt privé de
// l'organisation — là où vivent déjà les noms. L'étudiant n'y a jamais eu accès,
// et c'est ce qui fait qu'il ne peut ni lire sa marque ni deviner celle d'un
// autre.
//
// Deux travaux qui portent la même marque se reconnaissent sans ce fichier :
// la comparaison des jetons suffit. Il ne sert qu'à répondre à la question
// suivante — de qui s'agit-il —, et seul celui qui a distribué le travail peut
// y répondre. C'est voulu : un collègue qui reçoit des copies anonymisées voit
// la coïncidence sans jamais voir les noms.

// Version est celle du schéma écrit.
const Version = 1

// BookFile porte les marques, dans le registre de l'organisation.
const BookFile = "signatures.json"

// Issued relie une marque à une personne, pour un travail.
type Issued struct {
	// Assignment est l'identifiant complet du travail.
	Assignment string `json:"assignment"`
	Username   string `json:"username"`
	// Token est la marque, sous sa forme lisible.
	Token    string `json:"token"`
	IssuedAt string `json:"issued_at,omitempty"`
}

// Key sert au rangement : un travail, une personne.
func (i Issued) Key() string {
	return strings.ToLower(i.Assignment + "\x00" + i.Username)
}

func (i Issued) validate() (Issued, error) {
	i.Assignment = strings.ToLower(strings.TrimSpace(i.Assignment))
	i.Username = strings.ToLower(strings.TrimSpace(i.Username))
	i.Token = strings.ToLower(strings.TrimSpace(i.Token))
	if _, _, ok := naming.SplitAssignment(i.Assignment); !ok {
		return i, valid.Errorf(
			"Signatures : « %s » n'est pas un travail de la nomenclature.", i.Assignment)
	}
	if i.Username == "" {
		return i, valid.Errorf(
			"Signatures : une marque de « %s » ne dit pas à qui elle est.", i.Assignment)
	}
	if _, err := Parse(i.Token); err != nil {
		return i, err
	}
	return i, nil
}

// Book est le registre des marques délivrées.
type Book struct {
	Version int      `json:"version"`
	Issued  []Issued `json:"issued"`
}

// Empty dit qu'il n'y a rien à écrire.
func (b Book) Empty() bool { return len(b.Issued) == 0 }

// Validate met le registre en forme.
func (b Book) Validate() (Book, error) {
	b.Version = Version
	vues := map[string]int{}
	marques := map[string]string{}
	lignes := make([]Issued, 0, len(b.Issued))
	for _, ligne := range b.Issued {
		valide, err := ligne.validate()
		if err != nil {
			return b, err
		}
		// Deux personnes ne peuvent pas porter la même marque : la détection
		// les confondrait, et c'est exactement ce qu'elle existe pour éviter.
		if autre, deja := marques[valide.Token]; deja && autre != valide.Key() {
			return b, valid.Errorf(
				"Signatures : la marque « %s » est délivrée deux fois. Une marque "+
					"ne désigne qu'une personne.", valide.Token)
		}
		marques[valide.Token] = valide.Key()
		// Une même personne redistribuée reçoit une nouvelle marque : la plus
		// récente remplace l'ancienne.
		if rang, deja := vues[valide.Key()]; deja {
			lignes[rang] = valide
			continue
		}
		vues[valide.Key()] = len(lignes)
		lignes = append(lignes, valide)
	}
	sort.Slice(lignes, func(first, second int) bool {
		return lignes[first].Key() < lignes[second].Key()
	})
	b.Issued = lignes
	return b, nil
}

// With verse des marques et rend le registre qui en résulte, avec un booléen
// qui dit s'il a bougé.
func (b Book) With(issued []Issued) (Book, bool, error) {
	if len(issued) == 0 {
		return b, false, nil
	}
	fusion := Book{Version: Version, Issued: append([]Issued(nil), b.Issued...)}
	fusion.Issued = append(fusion.Issued, issued...)
	valide, err := fusion.Validate()
	if err != nil {
		return b, false, err
	}
	avant, err := EncodeBook(b)
	if err != nil {
		return b, false, err
	}
	apres, err := EncodeBook(valide)
	if err != nil {
		return b, false, err
	}
	return valide, string(avant) != string(apres), nil
}

// Who rend la personne derrière une marque, et le travail où elle a été posée.
func (b Book) Who(token string) (Issued, bool) {
	token = strings.ToLower(strings.TrimSpace(token))
	for _, ligne := range b.Issued {
		if ligne.Token == token {
			return ligne, true
		}
	}
	return Issued{}, false
}

// Of rend la marque d'une personne pour un travail.
func (b Book) Of(assignment, username string) (Issued, bool) {
	cle := Issued{Assignment: assignment, Username: username}.Key()
	for _, ligne := range b.Issued {
		if ligne.Key() == cle {
			return ligne, true
		}
	}
	return Issued{}, false
}

// DecodeBook relit un registre de marques.
func DecodeBook(content []byte) (Book, []string) {
	var lu Book
	if err := json.Unmarshal(content, &lu); err != nil {
		return Book{}, []string{fmt.Sprintf("Signatures illisibles : %v.", err)}
	}
	if lu.Version > Version {
		return Book{}, []string{fmt.Sprintf(
			"Signatures : elles viennent d'une version %d de l'outil, qui n'en "+
				"connaît que %d. Mettez l'extension à jour.", lu.Version, Version)}
	}
	// Une ligne mal écrite est écartée et signalée : perdre une marque est un
	// désagrément, perdre toutes les autres serait une perte sèche.
	gardes := make([]Issued, 0, len(lu.Issued))
	soucis := make([]string, 0, 2)
	for _, ligne := range lu.Issued {
		valide, err := ligne.validate()
		if err != nil {
			soucis = append(soucis, err.Error())
			continue
		}
		gardes = append(gardes, valide)
	}
	lu.Issued = gardes
	valide, err := lu.Validate()
	if err != nil {
		return Book{}, append(soucis, err.Error())
	}
	return valide, soucis
}

// EncodeBook écrit un registre de marques.
func EncodeBook(book Book) ([]byte, error) {
	valide, err := book.Validate()
	if err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(valide, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}
