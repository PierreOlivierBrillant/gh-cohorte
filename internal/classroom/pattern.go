package classroom

import (
	"regexp"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/naming"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Un gabarit d'adoption dit comment lire des noms de dépôts que rien
// n'organise. Toutes les organisations n'ont pas suivi une nomenclature :
// « projet-web-tp1-jlpicard », « kickmyb-equipe-3 », « h23-4204n6-tp1-alice »
// existent, et il faut bien pouvoir s'en servir sans tout renommer d'abord.
//
// Le gabarit se lit comme les autres de l'outil : du texte, et deux champs.
//
//	{assignment}   le travail       projet-{assignment}-{student}
//	{student}      la personne      {assignment}.{student}
//
// Un nom seul ne se découpe pas toujours : « projet-tp1-emilie-cote » peut se
// lire « tp1 » + « emilie-cote » ou « tp1-emilie » + « cote ». Deux lectures
// lèvent l'ambiguïté, comme pour l'ancienne nomenclature tout en tirets.
//
// Quand la liste des étudiants est connue, chaque nom lui est confronté personne
// par personne : c'est exact, même si un compte contient le séparateur. Au
// moment d'adopter, où la liste n'existe pas encore, les noms s'éclairent les
// uns les autres — un travail reconnu ailleurs tranche.

// Champs reconnus dans un gabarit d'adoption.
const (
	champTravail  = "{assignment}"
	champEtudiant = "{student}"
)

// Pattern est un gabarit compilé.
type Pattern struct {
	source string
	// avant, entre et apres découpent le texte littéral autour des champs.
	avant, entre, apres string
	// travailDabord dit lequel des deux champs vient en premier.
	travailDabord bool
	// glouton lit un nom sans rien savoir de la personne.
	glouton *regexp.Regexp
}

// etudiantSeul décrit ce qu'un fragment de personne peut contenir : le point
// est réservé à la nomenclature courante, il n'a rien à faire ici.
const etudiantSeul = `[^.]+?`

// ParsePattern compile un gabarit d'adoption.
func ParsePattern(source string) (Pattern, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return Pattern{}, valid.Errorf("Gabarit vide.")
	}
	if strings.Count(source, champEtudiant) != 1 {
		return Pattern{}, valid.Errorf(
			"Le gabarit doit contenir %s une fois exactement.", champEtudiant)
	}
	if compte := strings.Count(source, champTravail); compte > 1 {
		return Pattern{}, valid.Errorf(
			"Le gabarit ne peut contenir %s qu'une fois.", champTravail)
	}
	if reste := champsInconnus(source); reste != "" {
		return Pattern{}, valid.Errorf(
			"Champ inconnu dans le gabarit : %s. Champs disponibles : %s et %s.",
			reste, champTravail, champEtudiant)
	}

	gabarit := Pattern{source: source}
	if !strings.Contains(source, champTravail) {
		// Sans travail nommé, tous les dépôts en relèvent : un seul travail,
		// qui prend le nom du groupe. C'est le cas d'un préfixe simple.
		avant, apres, _ := strings.Cut(source, champEtudiant)
		gabarit.avant, gabarit.apres = avant, apres
		gabarit.glouton = regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(avant) +
			`(` + etudiantSeul + `)` + regexp.QuoteMeta(apres) + `$`)
		return gabarit, nil
	}

	gabarit.travailDabord = strings.Index(source, champTravail) < strings.Index(source, champEtudiant)
	premier, second := champTravail, champEtudiant
	if !gabarit.travailDabord {
		premier, second = champEtudiant, champTravail
	}
	avant, reste, _ := strings.Cut(source, premier)
	entre, apres, _ := strings.Cut(reste, second)
	gabarit.avant, gabarit.entre, gabarit.apres = avant, entre, apres

	if entre == "" {
		return Pattern{}, valid.Errorf(
			"Le gabarit doit séparer %s de %s par au moins un caractère.",
			champTravail, champEtudiant)
	}

	// Le travail est glouton, la personne non : « a26-tp1-de-session-alice »
	// donne bien « tp1-de-session » et « alice ».
	travail, etudiant := `(.+)`, `(`+etudiantSeul+`)`
	un, deux := travail, etudiant
	if !gabarit.travailDabord {
		un, deux = etudiant, travail
	}
	gabarit.glouton = regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(avant) + un +
		regexp.QuoteMeta(entre) + deux + regexp.QuoteMeta(apres) + `$`)
	return gabarit, nil
}

// champsInconnus renvoie le premier champ qui n'est pas reconnu.
var champRe = regexp.MustCompile(`\{[^}]*\}`)

func champsInconnus(source string) string {
	for _, trouve := range champRe.FindAllString(source, -1) {
		if trouve != champTravail && trouve != champEtudiant {
			return trouve
		}
	}
	return ""
}

// Valid dit si le gabarit a été compilé.
func (p Pattern) Valid() bool { return p.glouton != nil }

// String rend le gabarit tel qu'il a été écrit.
func (p Pattern) String() string { return p.source }

// Prefix est le texte qui précède le premier champ : ce qui, dans le gabarit,
// désigne le groupe. Il sert à le nommer, pas à le lire.
func (p Pattern) Prefix() string {
	return strings.Trim(p.avant, "-._ ")
}

// Split est une découpe possible d'un nom de dépôt.
type Split struct {
	Repo       string `json:"repo"`
	Assignment string `json:"assignment"`
	Student    string `json:"student"`
}

// Splits énumère toutes les découpes qu'un nom accepte. « projet-tp1-emilie-cote »
// en a deux — « tp1 » et « emilie-cote », ou « tp1-emilie » et « cote » —, et
// rien dans ce nom seul ne dit laquelle est la bonne.
func (p Pattern) Splits(name string) []Split {
	if p.glouton == nil {
		return nil
	}
	if len(name) < len(p.avant)+len(p.apres) ||
		!strings.EqualFold(name[:len(p.avant)], p.avant) ||
		!strings.EqualFold(name[len(name)-len(p.apres):], p.apres) {
		return nil
	}
	milieu := name[len(p.avant) : len(name)-len(p.apres)]
	if p.entre == "" {
		if milieu == "" || strings.Contains(milieu, naming.Separator) {
			return nil
		}
		return []Split{{Repo: name, Student: milieu}}
	}

	var trouves []Split
	for position := 0; position+len(p.entre) <= len(milieu); position++ {
		if !strings.EqualFold(milieu[position:position+len(p.entre)], p.entre) {
			continue
		}
		gauche, droite := milieu[:position], milieu[position+len(p.entre):]
		if gauche == "" || droite == "" {
			continue
		}
		travail, etudiant := gauche, droite
		if !p.travailDabord {
			travail, etudiant = droite, gauche
		}
		// Le point est réservé à la nomenclature courante : il ne peut pas
		// faire partie du nom d'une personne.
		if strings.Contains(etudiant, naming.Separator) {
			continue
		}
		trouves = append(trouves, Split{Repo: name, Assignment: travail, Student: etudiant})
	}
	return trouves
}

// Resolve découpe une liste de noms en les éclairant les uns par les autres.
// Pris isolément, « projet-tp1-emilie-cote » se coupe mal ; mais si
// « projet-tp1-jlpicard » existe à côté, « tp1 » se reconnaît comme un travail,
// et la bonne découpe s'impose. C'est ce qui rend l'écran d'adoption utile
// avant qu'une liste d'étudiants existe.
func (p Pattern) Resolve(names []string) []Split {
	possibles := make([][]Split, 0, len(names))
	frequence := map[string]int{}
	for _, name := range names {
		decoupes := p.Splits(name)
		if len(decoupes) == 0 {
			continue
		}
		possibles = append(possibles, decoupes)
		vus := map[string]bool{}
		for _, decoupe := range decoupes {
			cle := strings.ToLower(decoupe.Assignment)
			if vus[cle] {
				continue
			}
			vus[cle] = true
			frequence[cle]++
		}
	}

	retenus := make([]Split, 0, len(possibles))
	for _, decoupes := range possibles {
		meilleur := decoupes[0]
		for _, decoupe := range decoupes[1:] {
			vainqueur := frequence[strings.ToLower(decoupe.Assignment)]
			sortant := frequence[strings.ToLower(meilleur.Assignment)]
			// À égalité, le travail le plus court gagne : c'est la personne qui
			// porte un séparateur dans son nom, pas le travail.
			if vainqueur > sortant ||
				(vainqueur == sortant && len(decoupe.Assignment) < len(meilleur.Assignment)) {
				meilleur = decoupe
			}
		}
		retenus = append(retenus, meilleur)
	}
	return retenus
}

// Match découpe un nom de dépôt sans rien savoir des personnes.
func (p Pattern) Match(name string) (string, string, bool) {
	if p.glouton == nil {
		return "", "", false
	}
	trouve := p.glouton.FindStringSubmatch(name)
	if trouve == nil {
		return "", "", false
	}
	if len(trouve) == 2 {
		// Gabarit sans travail : le dépôt relève du seul travail du groupe.
		return "", trouve[1], true
	}
	if p.travailDabord {
		return trouve[1], trouve[2], true
	}
	return trouve[2], trouve[1], true
}

// MatchFor découpe un nom de dépôt en sachant qui l'on cherche. C'est la
// lecture exacte : le fragment de la personne est posé tel quel, et ce qui
// reste est le travail, quel que soit ce qu'il contient.
func (p Pattern) MatchFor(name, fragment string) (string, bool) {
	if p.glouton == nil || fragment == "" {
		return "", false
	}
	if p.entre == "" && !strings.Contains(p.source, champTravail) {
		if strings.EqualFold(name, p.avant+fragment+p.apres) {
			return "", true
		}
		return "", false
	}
	if p.travailDabord {
		suffixe := p.entre + fragment + p.apres
		if len(name) <= len(p.avant)+len(suffixe) ||
			!strings.EqualFold(name[:len(p.avant)], p.avant) ||
			!strings.EqualFold(name[len(name)-len(suffixe):], suffixe) {
			return "", false
		}
		return name[len(p.avant) : len(name)-len(suffixe)], true
	}
	prefixe := p.avant + fragment + p.entre
	if len(name) <= len(prefixe)+len(p.apres) ||
		!strings.EqualFold(name[:len(prefixe)], prefixe) ||
		!strings.EqualFold(name[len(name)-len(p.apres):], p.apres) &&
			p.apres != "" {
		return "", false
	}
	return name[len(prefixe) : len(name)-len(p.apres)], true
}
