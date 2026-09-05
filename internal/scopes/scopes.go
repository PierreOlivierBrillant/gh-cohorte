// Package scopes dit ce que le jeton GitHub autorise, et le renouvelle quand il
// manque une portée. L'outil ne fabrique aucun jeton lui-même : il relance
// « gh auth refresh », qui garde les identifiants là où gh les range déjà et
// mène le flux d'appareil de GitHub à la place de l'outil.
package scopes

import (
	"context"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/auth"
)

// Scope est une portée du jeton, et ce que l'outil en fait.
type Scope struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Purpose string `json:"purpose"`
	// Minimal marque une portée du socle de gh : elle est toujours accordée et
	// ne peut pas être retirée.
	Minimal bool `json:"minimal"`
}

// Catalog énumère les portées dont l'outil se sert. Les deux premières font
// partie du socle que gh accorde à tout jeton qu'il crée ; les deux autres se
// demandent, et un jeton ordinaire ne les a pas.
var Catalog = []Scope{
	{"repo", "Dépôts", "Créer, lire et modifier les dépôts de l'organisation.", true},
	{"read:org", "Organisations", "Lister vos organisations et y lire votre rôle.", true},
	{"workflow", "Actions", "Déposer des fichiers dans .github/workflows.", false},
	{"delete_repo", "Suppression", "Supprimer définitivement un dépôt.", false},
}

// États d'une portée pour un jeton donné.
const (
	Present = "présente"
	Absent  = "absente"
	Unknown = "inconnue"
)

// Find retrouve une portée du catalogue.
func Find(name string) (Scope, bool) {
	for _, scope := range Catalog {
		if scope.Name == name {
			return scope, true
		}
	}
	return Scope{}, false
}

// Label nomme une portée en français, ou renvoie son nom brut si l'outil ne
// s'en sert pas : GitHub en accorde d'autres, et elles doivent rester lisibles.
func Label(name string) string {
	if scope, found := Find(name); found {
		return scope.Label
	}
	return name
}

// Describe met en mots ce qu'un jeton annonce d'une portée. Un jeton
// « fine-grained » n'annonce aucune portée : rien ne peut alors être affirmé.
func Describe(present, known bool) string {
	switch {
	case !known:
		return Unknown
	case present:
		return Present
	default:
		return Absent
	}
}

// Parse découpe la liste que GitHub renvoie dans « X-OAuth-Scopes ».
func Parse(header string) []string {
	var list []string
	for _, item := range strings.Split(header, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			list = append(list, trimmed)
		}
	}
	return list
}

// Has dit si une portée figure dans une liste.
func Has(list []string, name string) bool {
	for _, item := range list {
		if item == name {
			return true
		}
	}
	return false
}

// Missing renvoie, parmi les portées voulues, celles que la liste n'a pas.
func Missing(current, wanted []string) []string {
	var absent []string
	for _, name := range wanted {
		if !Has(current, name) && !Has(absent, name) {
			absent = append(absent, name)
		}
	}
	return absent
}

// Union rassemble les portées déjà accordées et celles demandées. C'est ce que
// le renouvellement doit viser : obtenir une portée de plus ne doit jamais
// faire perdre celles qui étaient là.
func Union(current, wanted []string) []string {
	seen := map[string]bool{}
	var all []string
	for _, name := range append(append([]string{}, current...), wanted...) {
		if name = strings.TrimSpace(name); name != "" && !seen[name] {
			seen[name] = true
			all = append(all, name)
		}
	}
	return sorted(all)
}

// sorted range les portées dans l'ordre du catalogue, puis alphabétiquement :
// une commande reproductible se relit et se compare.
func sorted(list []string) []string {
	rank := map[string]int{}
	for index, scope := range Catalog {
		rank[scope.Name] = index
	}
	ordered := append([]string{}, list...)
	sort.SliceStable(ordered, func(a, b int) bool {
		left, leftKnown := rank[ordered[a]]
		right, rightKnown := rank[ordered[b]]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown {
			return left < right
		}
		return ordered[a] < ordered[b]
	})
	return ordered
}

// Status est l'état d'une portée du catalogue pour un jeton donné.
type Status struct {
	Scope
	State string `json:"state"`
}

// Inventory confronte le catalogue aux portées annoncées par un jeton.
func Inventory(current []string, known bool) []Status {
	inventory := make([]Status, 0, len(Catalog))
	for _, scope := range Catalog {
		inventory = append(inventory, Status{
			Scope: scope,
			State: Describe(Has(current, scope.Name), known),
		})
	}
	return inventory
}

// ------------------------------------------------------------ renouvellement

// FromEnvironment reconnaît un jeton posé par l'environnement. gh ne peut rien
// renouveler dans ce cas : la variable l'emporte sur tout ce qu'il enregistre.
func FromEnvironment(origin string) bool {
	switch origin {
	case "GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN":
		return true
	}
	return false
}

// Un nom de portée s'écrit en minuscules ; le reste est refusé avant d'arriver
// sur la ligne de commande de gh, où un tiret de tête passerait pour un drapeau.
var validName = regexp.MustCompile(`^[a-z][a-z0-9_:]*$`)

// Validate vérifie qu'un nom de portée est écrit comme GitHub les écrit.
func Validate(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !validName.MatchString(name) {
		return "", valid.Errorf("Portée : « %s » n'est pas un nom de portée GitHub.", name)
	}
	return name, nil
}

// arguments compose l'appel à gh.
func arguments(host string, add, remove []string) []string {
	args := []string{"auth", "refresh"}
	if host != "" {
		args = append(args, "--hostname", host)
	}
	if len(add) > 0 {
		args = append(args, "--scopes", strings.Join(add, ","))
	}
	if len(remove) > 0 {
		args = append(args, "--remove-scopes", strings.Join(remove, ","))
	}
	return args
}

// Command énonce la commande gh équivalente, à recopier quand l'outil ne peut
// pas la lancer lui-même.
func Command(host string, add, remove []string) string {
	return "gh " + strings.Join(arguments(host, add, remove), " ")
}

// Request décrit un renouvellement de jeton.
type Request struct {
	Host   string
	Origin string   // provenance du jeton, telle que gh la rapporte
	Add    []string // portées à obtenir, celles déjà acquises comprises
	Remove []string // portées à retirer

	// In, Out et Err branchent gh sur le terminal d'où l'outil a été lancé.
	// GitHub y affiche un code à recopier et attend une confirmation : c'est un
	// échange avec une personne, et rien ne peut le mener à sa place — même
	// quand la demande vient du navigateur.
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Refresher renouvelle le jeton. Ses trois fonctions isolent tout ce qui sort
// du processus : les tests les remplacent, le flux d'appareil de GitHub ne
// pouvant pas être joué pour de vrai.
type Refresher struct {
	Locate  func() (string, error)
	Run     func(lifetime context.Context, path string, args []string, request Request) error
	Read    func(host string) (string, string)
	Timeout time.Duration
}

// DefaultTimeout borne l'attente : le code affiché par GitHub expire de toute
// façon, et un gh oublié garderait le terminal pour lui.
const DefaultTimeout = 15 * time.Minute

// NewRefresher construit un renouvellement branché sur le vrai gh.
func NewRefresher() *Refresher {
	return &Refresher{Locate: gh.Path, Run: runGh, Read: auth.TokenForHost, Timeout: DefaultTimeout}
}

// runGh lance gh en lui laissant le terminal : c'est là que l'échange se joue.
func runGh(lifetime context.Context, path string, args []string, request Request) error {
	command := exec.CommandContext(lifetime, path, args...)
	command.Stdin, command.Stdout, command.Stderr = request.In, request.Out, request.Err
	return command.Run()
}

// Do renouvelle le jeton et renvoie celui qui vaut ensuite.
func (r *Refresher) Do(lifetime context.Context, request Request) (string, error) {
	if FromEnvironment(request.Origin) {
		return "", valid.Errorf(
			"Le jeton vient de la variable d'environnement %s : gh ne peut pas le renouveler. "+
				"Effacez cette variable et lancez « gh auth login », ou donnez-lui un jeton "+
				"portant les portées voulues.", request.Origin)
	}

	add, err := validated(request.Add)
	if err != nil {
		return "", err
	}
	remove, err := validated(request.Remove)
	if err != nil {
		return "", err
	}
	if len(add) == 0 && len(remove) == 0 {
		return "", valid.Errorf("Aucune portée demandée : rien à renouveler.")
	}
	// Retirer une portée du socle ferait échouer gh à mi-chemin.
	for _, name := range remove {
		if scope, found := Find(name); found && scope.Minimal {
			return "", valid.Errorf(
				"La portée « %s » ne peut pas être retirée : gh l'exige de tout jeton.", name)
		}
	}
	request.Add, request.Remove = add, remove

	path, err := r.Locate()
	if err != nil || strings.TrimSpace(path) == "" {
		return "", valid.Errorf(
			"gh est introuvable : lancez « %s » vous-même pour obtenir ces portées.",
			Command(request.Host, add, remove))
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	bounded, cancel := context.WithTimeout(lifetime, timeout)
	defer cancel()
	if err := r.Run(bounded, path, arguments(request.Host, add, remove), request); err != nil {
		return "", valid.Errorf("Renouvellement interrompu : %v — la commande était « %s ».",
			err, Command(request.Host, add, remove))
	}

	token, _ := r.Read(request.Host)
	if strings.TrimSpace(token) == "" {
		return "", valid.Errorf("Jeton introuvable après le renouvellement : vérifiez « gh auth status ».")
	}
	return token, nil
}

// validated nettoie une liste de portées et refuse ce qui n'en est pas une.
func validated(names []string) ([]string, error) {
	var list []string
	for _, name := range names {
		clean, err := Validate(name)
		if err != nil {
			return nil, err
		}
		if !Has(list, clean) {
			list = append(list, clean)
		}
	}
	return sorted(list), nil
}
