// Package signature dépose dans un travail une marque invisible, propre à la
// personne à qui il a été distribué.
//
// Le winnowing trouve ce qui se ressemble. Il ne trouve pas ce qui a été
// recopié puis entièrement réécrit — et il ne dit jamais avec certitude que
// deux travaux ont une origine commune : une ressemblance forte reste une
// ressemblance. Une marque, elle, ne se ressemble pas : elle est là ou elle ne
// l'est pas, et la même marque dans deux travaux n'a pas d'explication
// innocente.
//
// Elle est faite de blancs. Un jeton de 48 bits et sa somme de contrôle sont
// écrits en espaces et en tabulations, sur des lignes vides du README : une
// ligne vide qui contient des blancs reste une ligne vide, à l'affichage comme
// au rendu du markdown. Rien ne se voit.
//
// Elle ne fausse rien non plus. Des blancs ne produisent aucun jeton : le flux
// comparé d'un fichier signé est exactement celui du même fichier non signé, et
// la marque n'a donc pas à être retirée avant de mesurer. Elle traverse en
// revanche l'anonymisation d'un envoi, et c'est voulu — c'est ce qui permet à
// un collègue de voir deux copies porter la même marque.
//
// Ce que la marque porte est un jeton tiré au hasard, et non le compte de
// l'étudiant. Trois raisons, et la troisième suffirait : l'étudiant ne peut ni
// le lire ni le reconstituer ; rien ne fuite si le dépôt devient public ; et il
// traverse l'anonymisation d'un envoi intact, ce qui permet à un collègue de
// dire « ces deux copies portent la même marque » sans jamais savoir de qui.
//
// # Ce qu'elle ne prouve pas
//
// Un formateur réglé sur « trim trailing whitespace », un « pre-commit », un
// éditeur configuré ainsi : tous l'effacent sans le savoir, et « git diff » la
// montre en rouge. Son absence ne prouve donc rien du tout, et tout ce qui
// l'affiche doit le dire. Seule sa présence en double parle.
package signature

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// La marque tient en soixante-quatre blancs.
const (
	// TokenBits est la longueur du jeton. Quarante-huit bits font deux cent
	// quatre-vingts mille milliards de possibilités : de quoi ne jamais voir
	// deux étudiants porter la même marque, et de quoi rendre une devinette
	// sans espoir.
	TokenBits = 48
	// CheckBits est la longueur de la somme de contrôle. Seize bits font qu'une
	// suite de blancs quelconque a une chance sur soixante-cinq mille d'être
	// prise pour une marque — assez peu pour qu'une indentation malheureuse
	// n'invente pas de coïncidence.
	CheckBits = 16
	// Length est le nombre de blancs qu'une marque occupe.
	Length = TokenBits + CheckBits
)

// Les deux blancs, et ce qu'ils valent. L'espace est le zéro parce qu'une
// indentation faite d'espaces est plus courante : une marque accidentelle est
// alors une suite de zéros, que la somme de contrôle écarte aussitôt.
const (
	zero = ' '
	one  = '\t'
)

// New tire un jeton au hasard.
func New() (uint64, error) {
	borne := big.NewInt(0).Lsh(big.NewInt(1), TokenBits)
	tirage, err := rand.Int(rand.Reader, borne)
	if err != nil {
		return 0, valid.Errorf("Signature : tirage impossible (%v).", err)
	}
	return tirage.Uint64(), nil
}

// Mark rend la marque d'un jeton : soixante-quatre blancs.
func Mark(token uint64) string {
	token &= mask(TokenBits)
	bits := make([]byte, 0, Length)
	bits = append(bits, write(token, TokenBits)...)
	bits = append(bits, write(uint64(checksum(token)), CheckBits)...)
	return string(bits)
}

// Read lit le jeton d'une suite de blancs, et dit si elle en portait un.
//
// Seuls les derniers blancs comptent : une ligne qui portait déjà des espaces
// en garde, et la marque est posée après eux.
func Read(run string) (uint64, bool) {
	blancs := []rune(run)
	if len(blancs) < Length {
		return 0, false
	}
	blancs = blancs[len(blancs)-Length:]
	for _, blanc := range blancs {
		if blanc != zero && blanc != one {
			return 0, false
		}
	}
	token := read(blancs[:TokenBits])
	if uint16(read(blancs[TokenBits:])) != checksum(token) {
		return 0, false
	}
	return token, true
}

// ReadmeFile est le fichier où la marque se dépose. C'est celui que GitHub
// montre sur la page d'un dépôt, celui qu'« auto_init » y crée, et celui qu'un
// étudiant ouvre en premier : la marque y est à la fois la plus sûre d'exister
// et la moins susceptible d'être réécrite.
const ReadmeFile = "README.md"

// Copies est le nombre d'exemplaires déposés.
//
// Trois, et un seul suffit à la détection. La redondance n'est pas une
// coquetterie : un formateur qui coupe les blancs de fin en efface souvent
// plusieurs d'un coup, mais une retouche à la main n'en touche qu'un.
const Copies = 3

// Sign dépose la marque dans un contenu, et rend le contenu signé.
//
// Les lignes vides la portent. Une ligne vide qui contient des blancs reste une
// ligne vide : ni l'affichage ni le rendu du markdown ne changent. Une ligne de
// texte, elle, ne peut pas la porter — deux espaces en fin de ligne valent un
// saut de ligne en markdown, et la marque en ajouterait un là où il n'y en
// avait pas.
//
// Un contenu qui n'a pas assez de lignes vides en reçoit à la fin : c'est la
// seule façon de signer un README d'une seule ligne, et cela ne change rien à
// ce qu'on lit.
func Sign(content []byte, token uint64) []byte {
	marque := Mark(token)
	lignes := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")

	posees := 0
	for index, ligne := range lignes {
		if posees >= Copies {
			break
		}
		// Une ligne déjà signée n'est pas resignée : on remplace sa marque.
		if _, signee := Read(ligne); signee {
			lignes[index] = marque
			posees++
			continue
		}
		if strings.TrimSpace(ligne) == "" && len(ligne) < Length {
			lignes[index] = marque
			posees++
		}
	}
	for posees < Copies {
		lignes = append(lignes, marque)
		posees++
	}
	// Un fichier texte finit par un saut de ligne : la marque en queue ne doit
	// pas le lui retirer.
	if len(lignes) > 0 && lignes[len(lignes)-1] != "" {
		lignes = append(lignes, "")
	}
	return []byte(strings.Join(lignes, "\n"))
}

// Find rend les jetons qu'un contenu porte, sans doublon.
func Find(content []byte) []uint64 {
	trouves := make([]uint64, 0, Copies)
	vus := map[uint64]bool{}
	for _, ligne := range strings.Split(string(content), "\n") {
		ligne = strings.TrimSuffix(ligne, "\r")
		if token, signee := Read(ligne); signee && !vus[token] {
			vus[token] = true
			trouves = append(trouves, token)
		}
	}
	return trouves
}

// First rend le premier jeton d'un contenu, ou zéro.
func First(content []byte) (uint64, bool) {
	if trouves := Find(content); len(trouves) > 0 {
		return trouves[0], true
	}
	return 0, false
}

// Text rend un jeton sous une forme qui se lit et se recopie :
// « a3f9-2c81-77e4 ».
func Text(token uint64) string {
	token &= mask(TokenBits)
	return fmt.Sprintf("%04x-%04x-%04x",
		(token>>32)&0xffff, (token>>16)&0xffff, token&0xffff)
}

// Parse relit un jeton écrit.
func Parse(text string) (uint64, error) {
	brut := strings.ReplaceAll(strings.TrimSpace(text), "-", "")
	if len(brut) != TokenBits/4 {
		return 0, valid.Errorf(
			"Signature : « %s » n'est pas une marque (attendu : « a3f9-2c81-77e4 »).",
			text)
	}
	var token uint64
	if _, err := fmt.Sscanf(brut, "%x", &token); err != nil {
		return 0, valid.Errorf("Signature : « %s » n'est pas une marque.", text)
	}
	return token & mask(TokenBits), nil
}

// ------------------------------------------------------------------ outils

func mask(bits int) uint64 { return (uint64(1) << bits) - 1 }

// write écrit un nombre en blancs, du bit de poids fort au plus faible.
func write(value uint64, bits int) []byte {
	blancs := make([]byte, bits)
	for index := 0; index < bits; index++ {
		blancs[index] = zero
		if value&(1<<(bits-1-index)) != 0 {
			blancs[index] = one
		}
	}
	return blancs
}

func read(blancs []rune) uint64 {
	var value uint64
	for _, blanc := range blancs {
		value <<= 1
		if blanc == one {
			value |= 1
		}
	}
	return value
}

// checksum est la somme de contrôle d'un jeton : le repli sur seize bits de son
// haché FNV. Elle ne protège de rien — une marque n'a pas d'ennemi —, elle
// évite seulement qu'une indentation malheureuse passe pour une signature.
func checksum(token uint64) uint16 {
	octets := make([]byte, 8)
	binary.BigEndian.PutUint64(octets, token)
	hash := uint64(0xcbf29ce484222325)
	for _, octet := range octets {
		hash ^= uint64(octet)
		hash *= 0x100000001b3
	}
	return uint16(hash ^ (hash >> 16) ^ (hash >> 32) ^ (hash >> 48))
}
