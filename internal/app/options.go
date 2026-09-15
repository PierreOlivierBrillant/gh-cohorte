// Package app assemble l'outil : lecture des drapeaux, assistant de création,
// gestion d'un groupe existant et options avancées.
package app

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/inspect"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/plan"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/users"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Version de l'extension, renseignée à la compilation.
var Version = "dev"

// Sentinelle : distingue « drapeau absent » de « drapeau sans valeur ».
const unset = "\x00absent"

// Options rassemble les drapeaux de la ligne de commande.
type Options struct {
	Org             string
	Manage          string // vide = choisir le groupe dans la liste
	ManageRequested bool
	// StudentsRequested ouvre l'annuaire : les utilisateurs de l'organisation
	// entière, avec les cours que chacun a suivis.
	StudentsRequested bool
	// User ouvre la fiche d'un compte : son rôle, ses comptes, et la
	// chronologie de son passage — cours suivis et cours donnés mêlés.
	User string
	// FullName donne son nom complet au compte visé par User. Il ne renomme
	// aucun dépôt : le slug qu'il produit s'ajoute à ceux que la personne
	// portait déjà, et ce qui existe reste à elle.
	FullName string
	// Teacher reconnaît le compte visé comme enseignant, ou l'en défait.
	// TeacherSet distingue « drapeau absent » de « --teacher=false » : sans
	// lui, la fiche se contente de s'afficher.
	Teacher    bool
	TeacherSet bool
	// Teachers est la composition exacte de l'équipe enseignante du groupe
	// géré, comptes séparés par des virgules. TeachersOn distingue le drapeau
	// absent — qui ne fait qu'afficher le cloisonnement — d'une liste vide,
	// qui retire tout le monde.
	Teachers   []string
	TeachersOn bool
	// Import reprend des dépôts nommés autrement — « travail-compte », ce que
	// GitHub Classroom produit — et les fait entrer dans la nomenclature. Sans
	// valeur, il montre les travaux que ces dépôts dessinent.
	Import          string
	ImportRequested bool
	// Into est la place d'arrivée d'une importation : « a26.5n6.1030 ».
	Into string
	// Repos restreint une reprise aux dépôts nommés, séparés par des virgules.
	// Vide, le travail est repris entier.
	Repos string
	// NamedOnly laisse où ils sont les dépôts d'une importation dont on ne
	// connaît pas la personne, plutôt que de les reprendre sous le compte
	// qu'ils portent.
	NamedOnly bool
	// PublishRegistry verse au registre de l'organisation les noms que ce
	// poste a accumulés, puis quitte. Avec --dry-run, il montre seulement ce
	// qu'il ferait ; avec --yes, il ne demande pas confirmation.
	PublishRegistry bool
	// PreferLocal fait gagner les noms de ce poste sur ceux du registre quand
	// les deux diffèrent. Sans lui, le registre garde les siens.
	PreferLocal bool
	// ForgetRegistryHistory réécrit la branche du registre en un commit sans
	// passé, puis quitte. Le nom du dépôt doit être retapé : aucune option,
	// « --yes » compris, ne court-circuite cette confirmation.
	ForgetRegistryHistory bool
	// RegistryTeam donne à une équipe de l'organisation accès au registre,
	// puis quitte.
	RegistryTeam string
	Roster       string
	// Filter, Sort et SortDesc règlent ce que la liste d'un groupe montre et
	// dans quel ordre. Ce que ces critères signifient est décidé dans
	// « students » : les trois interfaces s'y tiennent.
	Filter   users.Filter
	Sort     users.Key
	SortDesc bool
	// Handin ne garde que les dépôts dont la remise en est là — « en retard »,
	// « non accepté »… Il vit à part du filtre des personnes : ce qu'il
	// regarde n'est pas une ligne d'annuaire, c'est ce que l'historique et les
	// accès déjà relevés disent d'un dépôt.
	Handin     classroom.HandinState
	Assignment string
	// Teams dit que le travail se distribue aux équipes : un dépôt par équipe
	// plutôt qu'un dépôt par personne. En mode gestion, sans autre drapeau
	// d'équipe, il liste les équipes du groupe ; à la reprise, il dit que le
	// dernier niveau des noms désigne une équipe et non une personne.
	Teams bool
	// Team désigne la ou les équipes visées. En gestion, une seule à la fois ;
	// à la distribution, celles à servir — les autres attendront.
	Team []string
	// Opérations sur l'équipe visée, en mode gestion.
	TeamMembers   []string
	TeamMembersOn bool
	TeamAdd       []string
	TeamRemove    []string
	TeamRename    string
	TeamAdopt     string
	// TeamMove est la place du groupe où l'équipe s'en va, avec ses membres et
	// tout ce qu'ils ont rendu.
	TeamMove   string
	TeamDelete bool
	// TeamDeleteRepos emporte aussi les dépôts que l'équipe a rendus. Ils ne
	// la suivent pas d'eux-mêmes : le travail survit à l'équipe qui l'a fait.
	TeamDeleteRepos bool
	TeamShare       bool
	// Due est la date cible du travail : « 2026-10-01 », ou
	// « 2026-10-01T23:59 » quand l'heure compte. À la distribution, elle est
	// fixée d'emblée ; avec « --manage », elle change celle du travail géré.
	// Une valeur vide la retire, ce qui oblige à distinguer le drapeau absent.
	Due    string
	DueSet bool
	// Handins relève les historiques des dépôts du travail géré : le nombre de
	// commits, les remises postérieures à la date cible, et les personnes dont
	// aucun commit ne porte la trace.
	Handins bool
	// Plagiarism compare entre elles les copies du travail géré : elle mesure
	// leurs ressemblances et les classe par ordre de suspicion. Ce n'est pas un
	// verdict — le rapport le dit lui-même à chaque fois.
	Plagiarism bool
	// NoSign renonce à la marque invisible déposée dans le README de chaque
	// dépôt distribué. NoSignSet distingue le drapeau absent d'un
	// « --no-sign=false », qui la rétablit après l'avoir désactivée.
	NoSign    bool
	NoSignSet bool
	// EmitWorkflow dépose le gabarit de passe automatisée, puis quitte. Sans
	// valeur, il l'écrit à sa place ordinaire.
	EmitWorkflow    string
	EmitWorkflowSet bool
	// PublishIndex publie l'index d'empreintes du travail géré, pour que les
	// collègues puissent y comparer leurs copies sans lire les nôtres.
	PublishIndex bool
	// Against nomme les travaux d'un collègue dont l'index publié entre dans
	// l'analyse, séparés par des virgules.
	Against string
	// ExportZip écrit une archive anonymisée des copies du travail géré, pour
	// l'envoyer à un collègue. ImportZip fait l'inverse : il verse dans
	// l'analyse des copies reçues.
	ExportZip string
	ImportZip string
	// AnonymizeParts cherche aussi les fragments de noms pris isolément — le
	// nom de famille seul, le prénom seul. C'est plus agressif, et cela abîme
	// parfois du texte ordinaire : « Côté » est aussi un mot français.
	AnonymizeParts bool
	// Requests liste les demandes de levée du voile, Grant et Deny en tranchent
	// une par son identifiant, et Ask en dépose une — « travail:jeton ». Une
	// demande est le seul chemin par lequel le code d'un collègue devient
	// lisible : elle se nomme, elle se décide, et elle reste écrite.
	Requests bool
	Grant    string
	Deny     string
	Ask      string
	// Reason accompagne une demande déposée ou un refus : c'est ce qu'on dit à
	// l'autre, et la seule chose qu'il lira.
	Reason string
	// Reach dit jusqu'où le corpus s'étend : ce groupe, tout le cours, ou
	// toutes les sessions — équivalences de sigles comprises.
	Reach string
	// Rules surcharge, le temps d'une analyse, les règles déclarées par
	// l'organisation. PublishRules fait l'inverse : il les y écrit.
	Rules        string
	PublishRules string
	// Profile, Languages, Only et Ignore disent quoi inspecter. Les langages et
	// les motifs se séparent par des virgules, comme partout ailleurs dans
	// l'outil.
	Profile   string
	Languages string
	Only      string
	Ignore    string
	// Kgram et Window remplacent les bornes de winnowing du langage. Les
	// monter allège le calcul et rend la détection plus grossière ; les
	// descendre fait l'inverse.
	Kgram  int
	Window int
	// Noise est la part des copies au-delà de laquelle une empreinte est tenue
	// pour banale, MinSimilarity le plancher des paires rapportées.
	Noise         float64
	MinSimilarity float64
	// NoBaseline renonce à écarter le gabarit distribué. Sans lui, l'outil s'en
	// sert : il sait quel modèle il a déposé, et toutes les copies le portent.
	NoBaseline bool
	// MoveTo déplace le travail ouvert vers une place de la nomenclature
	// courante — « a26.5n6.01 » —, et RenameTo dit le nom qu'il y prendra. Sans
	// MoveTo, RenameTo renomme le travail là où il est déjà.
	MoveTo           string
	RenameTo         string
	Template         string
	TemplateSet      bool
	Pattern          string
	Starter          string
	StarterSet       bool
	CommitMessage    string
	ForceStarter     bool
	Visibility       string
	Permission       string
	NoCollaborator   bool
	NoVerifyAccounts bool
	DryRun           bool
	Yes              bool
	NonInteractive   bool
	Web              bool
	CLI              bool
	NoBrowser        bool
	Delay            float64
	DelaySet         bool
	Jobs             int
	Depth            int
	Host             string
	ConfigPath       string
	ReportDir        string
	NoSaveConfig     bool
	NoCache          bool
	ClearCache       bool
	// RefreshToken régénère le jeton GitHub avec les portées de Scopes, puis
	// quitte. Scopes vide vaut pour toutes celles dont l'outil se sert.
	RefreshToken bool
	Scopes       string
	ShowVersion  bool

	// Réglés par les tests seulement : jamais exposés en ligne de commande.
	BaseURL  string
	CacheDir string
}

// Usage décrit l'outil et ses drapeaux.
func Usage(out io.Writer) {
	champs := make([]string, 0, len(plan.Placeholders))
	for _, name := range plan.Placeholders {
		champs = append(champs, "{"+name+"}")
	}
	fmt.Fprintf(out, `gh cohorte %s — un dépôt GitHub par personne, à la manière de GitHub Classroom.

Utilisation :
  gh cohorte                                  interface graphique dans le navigateur
  gh cohorte --cli                            assistant interactif au terminal
  gh cohorte --manage tp1                     gérer le groupe « tp1 »
  gh cohorte --students --session a26         utilisateurs de la session a26
  gh cohorte --students --role enseignant     les enseignants de l'organisation
  gh cohorte --user ecote                     la fiche de @ecote et son passage
  gh cohorte --user jdupont --teacher         reconnaître @jdupont comme enseignant
  gh cohorte --user aleksilepaj --full-name "Aleksi Lepaj"
  gh cohorte --manage a26.5n6.01 --teachers "prof,jdupont" -y
  gh cohorte --import                         reprendre des dépôts nommés autrement
  gh cohorte --import tp1 --into a26.5n6.1030 --roster liste.csv --dry-run
  gh cohorte --import projet --teams --into a26.5n6.01 -y
  gh cohorte --publish-registry --dry-run     ce que publier les noms ferait
  gh cohorte --manage travail-de --move-to a26.5n6.01 --rename-to tp1 -y
  gh cohorte --manage a26.5n6.01.tp1 --rename-to projet-final -y
  gh cohorte --manage a26.5n6.01.tp1 --due 2026-10-01
  gh cohorte --manage a26.5n6.01.tp1 --handins
  gh cohorte --plagiarism --manage a26.5n6.01.tp1
  gh cohorte --plagiarism --manage a26.5n6.01.tp1 --profile next --languages tsx,css
  gh cohorte --plagiarism --manage a26.5n6.01.tp1 --reach annees
  gh cohorte --publish-rules regles.json      déclarer « 5N6 devient 5M6 »
  gh cohorte --export-zip envoi.zip --manage a26.5n6.01.tp1
  gh cohorte --plagiarism --manage a26.5n6.01.tp1 --import-zip recu.zip
  gh cohorte --publish-index --manage a26.5n6.01.tp1
  gh cohorte --publish-index --org acme -y       rattraper les index manquants
  gh cohorte --plagiarism --manage a26.5n6.01.tp1 --against a26.5n6.02.tp1
  gh cohorte --ask a26.5n6.02.tp1:K7DM2X --reason "deux TP quasi identiques"
  gh cohorte --requests
  gh cohorte --grant D4K2M9 --export-zip envoi.zip
  gh cohorte --refresh-token --scopes delete_repo
  gh cohorte --roster cohorte.csv --dry-run   simulation, sans rien créer
  gh cohorte --org acme --assignment tp1 --roster cohorte.csv --yes
  gh cohorte --org acme --manage a26.5n6.01 --teams
  gh cohorte --org acme --manage a26.5n6.01 --team eq1 --team-members "ec,jlp"
  gh cohorte --org acme --assignment a26.5n6.01.tp1 --teams --team eq1,eq2 -y

Drapeaux :
  --org ORG                organisation GitHub cible
  --manage [PREFIXE]       gérer un groupe existant au lieu d'en créer un
  --students               lister les utilisateurs de l'organisation et ce qu'ils ont suivi
  --user COMPTE            fiche d'un utilisateur : son rôle, ses comptes, son passage
  --full-name NOM          donner son nom complet au compte de --user (ne renomme aucun dépôt)
  --teacher[=false]        reconnaître le compte de --user comme enseignant, ou l'en défaire
  --teachers COMPTES       composition de l'équipe enseignante du groupe de --manage ;
                           elle reçoit ses dépôts, et elle seule les voit
  --import [TRAVAIL]       reprendre des dépôts « travail-compte » ; vide, les lister
  --into PLACE             place d'arrivée d'une importation (« a26.5n6.1030 »)
  --publish-registry       verser au registre de l'organisation les noms de ce poste
  --prefer-local           en cas de désaccord, garder le nom de ce poste
  --registry-team EQUIPE   donner à une équipe accès au registre
  --forget-registry-history  réécrire le registre sans son historique
  --role enseignant|étudiant  ne lister que les enseignants, ou que les étudiants
  --session COURT          ne lister que les étudiants d'une session (« a26 »)
  --course SIGLE           ne lister que les étudiants d'un cours (« 5n6 »)
  --filter TEXTE           ne lister que les dépôts dont le nom ou le compte contient TEXTE
  --pushed-after DATE      ne lister que les envois postérieurs à DATE (AAAA-MM-JJ)
  --pushed-before DATE     ne lister que les envois antérieurs à DATE
  --never-pushed           ne lister que les dépôts sans aucun envoi
  --handin ÉTAT            ne lister qu'un état de remise : remis, en retard,
                           non remis, non accepté, non relevé
  --sort nom|compte|envoi  colonne de tri de la liste (défaut : nom)
  --sort-desc              trier du plus grand au plus petit
  --repos DEPOTS           ne reprendre que ces dépôts (noms séparés par des virgules)
  --named-only             ne reprendre que les dépôts dont l'étudiant est connu
  --roster FICHIER         liste « nom complet, compte GitHub » au format CSV
  --assignment NOM         identifiant du travail (préfixe des dépôts)
  --due DATE               date cible du travail (AAAA-MM-JJ ou AAAA-MM-JJTHH:MM,
                           vide pour la retirer)
  --handins                relever les commits des dépôts du travail géré
  --plagiarism             comparer entre elles les copies du travail géré et les
                           classer par ordre de suspicion (ce n'est pas un verdict :
                           une ressemblance forte n'est pas une preuve)
  --emit-workflow [CHEMIN] déposer le gabarit de passe GitHub Actions puis
                           quitter (défaut : .github/workflows/plagiat.yml)
  --no-sign[=false]        ne pas déposer, dans le README de chaque dépôt, une
                           marque invisible propre à son destinataire. Deux
                           travaux qui portent la même n'ont pas d'explication
                           innocente ; l'absence de marque, elle, ne prouve rien
  --publish-index          publier l'index d'empreintes du travail géré, pour que
                           vos collègues y comparent leurs copies. Ce sont des
                           hachés : ni code, ni nom ne quittent votre poste.
                           Sans « --manage », publie l'index de chaque travail
                           annoncé qui n'en a pas encore : c'est la forme qu'une
                           passe automatisée répète
  --against TRAVAUX        travaux d'un collègue à comparer aux vôtres, par leur
                           identifiant (« a26.5n6.02.tp1 »). Leur index doit
                           avoir été publié
  --export-zip FICHIER     écrire une archive anonymisée des copies du travail
                           géré, à envoyer à un collègue. La table de
                           correspondance est écrite à côté, jamais dedans
  --import-zip FICHIERS    archives de copies reçues, à comparer aux vôtres
                           (chemins séparés par des virgules)
  --anonymize-parts        anonymiser aussi les noms de famille et prénoms pris
                           isolément (plus sûr, mais abîme le texte ordinaire)
  --ask TRAVAIL:COPIE      demander à voir une copie mesurée dans l'index d'un
                           collègue (« a26.5n6.02.tp1:K7DM2X »)
  --requests               lister les demandes reçues et faites
  --grant DEMANDE          accorder une demande : l'archive anonymisée de la
                           seule copie concernée est écrite à --export-zip, et
                           c'est vous qui l'envoyez
  --deny DEMANDE           refuser une demande
  --reason TEXTE           ce qu'on dit en déposant une demande ou en la refusant
  --reach PORTEE           jusqu'où comparer : groupe (défaut), cours, annees.
                           « annees » suit les sigles qu'un cours a portés, tels
                           que le registre de l'organisation les déclare
  --rules FICHIER          règles de comparaison qui surchargent celles de
                           l'organisation, le temps d'une analyse
  --publish-rules FICHIER  écrire des règles de comparaison dans le registre de
                           l'organisation, puis quitter
  --profile NOM            profil d'inspection : tout, code-seul, next, angular,
                           flutter, java, aspnet, python
  --languages LANGAGES     langages à comparer (virgules) : kotlin, csharp, dart, sql,
                           java, markdown, python, typescript, tsx, javascript, css,
                           html, xml
  --only MOTIFS            n'inspecter que ces chemins (motifs « .gitignore »)
  --ignore MOTIFS          chemins à ne pas inspecter, en plus des exclusions d'office
  --kgram N                jetons par k-gramme (défaut : celui du langage)
  --window N               fenêtre de winnowing ; la monter allège le calcul
  --noise PART             part des copies au-delà de laquelle une empreinte est
                           tenue pour banale (défaut : 0,30)
  --min-similarity PART    similarité en deçà de laquelle une paire n'est pas rapportée
  --no-baseline            ne pas écarter le gabarit distribué au travail
  --teams                  travail d'équipe : un dépôt par équipe, partagé avec elle
                           (avec --manage seul : liste les équipes du groupe ;
                            avec --import : le dernier niveau nomme une équipe)
  --team NOM[,NOM]         équipe visée ; à la distribution, celles à servir
  --team-members COMPTES   composition exacte de l'équipe visée (la crée au besoin)
  --team-add COMPTES       inscrire des comptes dans l'équipe (ils quittent la leur)
  --team-remove COMPTES    retirer des comptes de l'équipe
  --team-rename NOM        renommer l'équipe visée
  --team-move PLACE        déplacer l'équipe vers un autre groupe
  --team-delete            supprimer l'équipe visée
  --team-delete-repos      supprimer aussi ses dépôts (avec --team-delete)
  --team-adopt EQUIPE      adopter une équipe de l'organisation sous le nom de --team
  --team-share             (re)partager les dépôts du travail avec leurs équipes
  --move-to PLACE          déplacer le travail géré vers « session.cours.groupe »
  --rename-to NOM          nom que le travail prend ; seul, il le renomme sur place
  --template ORG/DEPOT     dépôt modèle (vide = dépôt neuf initialisé)
  --pattern GABARIT        gabarit de nom des dépôts (défaut : {assignment}-{username})
  --starter DOSSIER        dossier local déposé dans chaque dépôt, en un commit
  --commit-message TEXTE   message du commit des fichiers de départ
  --force-starter          déposer même dans un dépôt déjà garni
  --visibility private|public
  --permission pull|triage|push|maintain|admin
  --no-collaborator        ne pas inviter les personnes
  --no-verify-accounts     ne pas vérifier l'existence des comptes
  --delay SECONDES         marge entre deux créations (défaut : 1 s)
  --jobs N                 travaux menés en parallèle (défaut : 4)
  --depth N                profondeur d'historique au clonage (0 = complet)
  --dry-run                simuler sans rien créer
  -y, --yes                passer la confirmation finale
  --non-interactive        échouer plutôt que poser une question
  --web                    ouvrir l'interface graphique sur la boucle locale (défaut)
  --cli                    rester au terminal : assistant interactif
  --no-browser             ne pas ouvrir le navigateur, afficher l'adresse
  --host HOTE              hôte GitHub (github.com ou instance Enterprise)
  --config FICHIER         fichier de réglages
  --report-dir DOSSIER     dossier des bilans (défaut : rapports)
  --no-save-config         ne pas mémoriser les réglages
  --no-cache               ignorer le cache local
  --clear-cache            vider le cache local puis quitter
  --refresh-token          régénérer le jeton GitHub puis quitter
  --scopes LISTE           portées à obtenir (défaut : celles dont l'outil se sert)
  --version                afficher la version

Champs des gabarits : %s

Codes de retour : 0 succès, 1 au moins un échec, 2 erreur de validation,
130 interruption ou annulation.
`, Version, strings.Join(champs, ", "))
}

// Parse analyse les arguments de la ligne de commande.
func Parse(args []string, out io.Writer) (*Options, error) {
	// L'aide est traitée avant tout : le paquet flag l'écrirait en anglais.
	for _, argument := range args {
		if argument == "-h" || argument == "--help" || argument == "help" {
			Usage(out)
			return nil, flag.ErrHelp
		}
	}

	options := &Options{}
	set := flag.NewFlagSet("cohorte", flag.ContinueOnError)
	// Les messages du paquet flag sont en anglais : ils sont remplacés plus bas.
	set.SetOutput(io.Discard)
	set.Usage = func() {}

	manage := set.String("manage", unset, "gérer un groupe existant")
	importer := set.String("import", unset, "reprendre des dépôts nommés autrement")
	template := set.String("template", unset, "dépôt modèle")
	starter := set.String("starter", unset, "dossier de fichiers de départ")
	delay := set.Float64("delay", -1, "marge entre deux créations")
	due := set.String("due", unset, "date cible du travail")

	set.StringVar(&options.Org, "org", "", "organisation GitHub cible")
	set.BoolVar(&options.StudentsRequested, "students", false,
		"lister les utilisateurs de l'organisation")
	set.StringVar(&options.User, "user", "", "ouvrir la fiche d'un compte")
	set.StringVar(&options.FullName, "full-name", "",
		"donner son nom complet au compte de --user")
	enseignant := set.String("teacher", unset,
		"reconnaître le compte visé comme enseignant (--teacher=false le retire)")
	enseignants := set.String("teachers", unset,
		"composition de l'équipe enseignante du groupe, séparée par des virgules")
	set.BoolVar(&options.PublishRegistry, "publish-registry", false,
		"verser au registre les noms de ce poste")
	set.BoolVar(&options.PreferLocal, "prefer-local", false,
		"garder les noms de ce poste en cas de désaccord")
	set.BoolVar(&options.ForgetRegistryHistory, "forget-registry-history", false,
		"réécrire le registre sans son historique")
	set.StringVar(&options.RegistryTeam, "registry-team", "",
		"donner à une équipe accès au registre")
	role := set.String("role", "", "ne lister qu'un rôle : enseignant ou étudiant")
	session := set.String("session", "", "ne lister qu'une session")
	sigle := set.String("course", "", "ne lister qu'un cours")
	filtre := set.String("filter", "", "ne lister que les dépôts correspondants")
	apres := set.String("pushed-after", "", "envois postérieurs à cette date")
	avant := set.String("pushed-before", "", "envois antérieurs à cette date")
	muets := set.Bool("never-pushed", false, "dépôts sans aucun envoi")
	remise := set.String("handin", "",
		"ne lister qu'un état de remise : remis, en retard, non remis, non accepté, non relevé")
	tri := set.String("sort", "", "colonne de tri de la liste")
	set.BoolVar(&options.SortDesc, "sort-desc", false, "trier du plus grand au plus petit")

	set.StringVar(&options.Into, "into", "", "place d'arrivée d'une importation")
	set.StringVar(&options.Repos, "repos", "",
		"dépôts à reprendre, séparés par des virgules (défaut : tous)")
	set.BoolVar(&options.NamedOnly, "named-only", false,
		"ne reprendre que les dépôts dont l'étudiant est connu")
	set.StringVar(&options.Roster, "roster", "", "liste des personnes")
	set.StringVar(&options.Assignment, "assignment", "", "identifiant du travail")
	set.BoolVar(&options.Teams, "teams", false, "travail d'équipe")
	set.BoolVar(&options.Plagiarism, "plagiarism", false,
		"comparer entre elles les copies du travail géré")
	emitWorkflow := set.String("emit-workflow", unset,
		"déposer le gabarit de passe GitHub Actions puis quitter")
	noSign := set.String("no-sign", unset,
		"ne pas déposer de marque invisible dans les dépôts distribués")
	set.BoolVar(&options.PublishIndex, "publish-index", false,
		"publier l'index d'empreintes du travail géré")
	set.StringVar(&options.Against, "against", "",
		"travaux d'un collègue à comparer, par leur identifiant")
	set.StringVar(&options.ExportZip, "export-zip", "",
		"écrire une archive anonymisée des copies du travail géré")
	set.StringVar(&options.ImportZip, "import-zip", "",
		"archives de copies reçues à verser dans l'analyse")
	set.BoolVar(&options.AnonymizeParts, "anonymize-parts", false,
		"anonymiser aussi les fragments de noms pris isolément")
	set.BoolVar(&options.Requests, "requests", false,
		"lister les demandes de levée du voile")
	set.StringVar(&options.Grant, "grant", "",
		"accorder une demande, et préparer l'envoi de la copie")
	set.StringVar(&options.Deny, "deny", "", "refuser une demande")
	set.StringVar(&options.Ask, "ask", "",
		"demander à voir une copie — « a26.5n6.02.tp1:K7DM2X »")
	set.StringVar(&options.Reason, "reason", "",
		"ce qu'on dit en déposant une demande ou en la refusant")
	set.StringVar(&options.Reach, "reach", "", "portée de la comparaison")
	set.StringVar(&options.Rules, "rules", "", "fichier de règles de comparaison")
	set.StringVar(&options.PublishRules, "publish-rules", "",
		"publier des règles de comparaison dans le registre puis quitter")
	set.StringVar(&options.Profile, "profile", "", "profil d'inspection")
	set.StringVar(&options.Languages, "languages", "", "langages à comparer")
	set.StringVar(&options.Only, "only", "", "n'inspecter que ces chemins")
	set.StringVar(&options.Ignore, "ignore", "", "chemins à ne pas inspecter")
	set.IntVar(&options.Kgram, "kgram", 0, "jetons par k-gramme")
	set.IntVar(&options.Window, "window", 0, "fenêtre de winnowing")
	set.Float64Var(&options.Noise, "noise", 0, "part des copies au-delà de laquelle une empreinte est banale")
	set.Float64Var(&options.MinSimilarity, "min-similarity", 0, "similarité minimale rapportée")
	set.BoolVar(&options.NoBaseline, "no-baseline", false, "ne pas écarter le gabarit distribué")
	set.BoolVar(&options.Handins, "handins", false,
		"relever les commits des dépôts du travail")
	equipe := set.String("team", "", "équipe visée")
	membres := set.String("team-members", unset, "composition exacte de l'équipe")
	ajouts := set.String("team-add", "", "comptes à inscrire dans l'équipe")
	retraits := set.String("team-remove", "", "comptes à retirer de l'équipe")
	set.StringVar(&options.TeamRename, "team-rename", "", "nouveau nom de l'équipe")
	set.StringVar(&options.TeamAdopt, "team-adopt", "", "équipe existante à adopter")
	set.StringVar(&options.TeamMove, "team-move", "",
		"place du groupe où déplacer l'équipe")
	set.BoolVar(&options.TeamDelete, "team-delete", false, "supprimer l'équipe visée")
	set.BoolVar(&options.TeamDeleteRepos, "team-delete-repos", false,
		"supprimer aussi les dépôts de l'équipe")
	set.BoolVar(&options.TeamShare, "team-share", false, "repartager les dépôts avec les équipes")
	set.StringVar(&options.MoveTo, "move-to", "", "place d'arrivée du travail géré")
	set.StringVar(&options.RenameTo, "rename-to", "", "nom que le travail prend")
	set.StringVar(&options.Pattern, "pattern", "", "gabarit de nom des dépôts")
	set.StringVar(&options.CommitMessage, "commit-message", "", "message du commit")
	set.BoolVar(&options.ForceStarter, "force-starter", false, "déposer même dans un dépôt garni")
	set.StringVar(&options.Visibility, "visibility", "", "visibilité des dépôts")
	set.StringVar(&options.Permission, "permission", "", "droit accordé")
	set.BoolVar(&options.NoCollaborator, "no-collaborator", false, "ne pas inviter")
	set.BoolVar(&options.NoVerifyAccounts, "no-verify-accounts", false, "ne pas vérifier les comptes")
	set.BoolVar(&options.DryRun, "dry-run", false, "simuler")
	set.BoolVar(&options.Yes, "yes", false, "passer la confirmation")
	set.BoolVar(&options.Yes, "y", false, "passer la confirmation")
	set.BoolVar(&options.NonInteractive, "non-interactive", false, "ne poser aucune question")
	set.BoolVar(&options.Web, "web", false, "ouvrir l'interface graphique locale")
	set.BoolVar(&options.CLI, "cli", false, "rester au terminal")
	set.BoolVar(&options.NoBrowser, "no-browser", false, "ne pas ouvrir le navigateur")
	set.IntVar(&options.Jobs, "jobs", 4, "travaux en parallèle")
	set.IntVar(&options.Depth, "depth", 0, "profondeur d'historique")
	set.StringVar(&options.Host, "host", "", "hôte GitHub")
	set.StringVar(&options.ConfigPath, "config", "", "fichier de réglages")
	set.StringVar(&options.ReportDir, "report-dir", "rapports", "dossier des bilans")
	set.BoolVar(&options.NoSaveConfig, "no-save-config", false, "ne pas mémoriser les réglages")
	set.BoolVar(&options.NoCache, "no-cache", false, "ignorer le cache")
	set.BoolVar(&options.ClearCache, "clear-cache", false, "vider le cache puis quitter")
	set.BoolVar(&options.RefreshToken, "refresh-token", false, "régénérer le jeton GitHub")
	set.StringVar(&options.Scopes, "scopes", "", "portées à obtenir")
	set.BoolVar(&options.ShowVersion, "version", false, "afficher la version")

	if err := set.Parse(normalizeArgs(args)); err != nil {
		return nil, translateFlagError(err)
	}
	if rest := set.Args(); len(rest) > 0 {
		return nil, valid.Errorf(
			"Argument inattendu : « %s ». Lancez « gh cohorte --help » pour la liste des drapeaux.",
			rest[0])
	}

	// Les critères de liste sont validés ici : une date mal écrite doit
	// arrêter la ligne de commande, pas se perdre en cours de route.
	options.Filter = users.Filter{
		Text: *filtre, PushedAfter: *apres, PushedBefore: *avant,
		Session: *session, Course: *sigle, Role: users.Role(*role),
	}
	if *muets {
		options.Filter.Activity = users.Silent
	}
	filtreValide, err := options.Filter.Validate()
	if err != nil {
		return nil, err
	}
	options.Filter = filtreValide
	if options.Handin, err = classroom.ParseHandinState(*remise); err != nil {
		return nil, err
	}
	if options.Sort, err = users.ParseKey(*tri); err != nil {
		return nil, err
	}

	options.Team = splitList(*equipe)
	options.TeamAdd = splitList(*ajouts)
	options.TeamRemove = splitList(*retraits)
	// Une composition vide reste une composition : « --team-members "" » vide
	// l'équipe, alors que le drapeau absent ne demande rien.
	if *membres != unset {
		options.TeamMembersOn = true
		options.TeamMembers = splitList(*membres)
	}
	// « --teacher » sans valeur vaut « oui » : c'est la forme d'un booléen au
	// terminal, et c'est le geste courant. « --teacher=false » retire le rôle.
	if *enseignant != unset {
		options.TeacherSet = true
		options.Teacher = *enseignant == "" || strings.EqualFold(*enseignant, "true")
	}
	if *enseignants != unset {
		options.TeachersOn = true
		options.Teachers = splitList(*enseignants)
	}

	if *noSign != unset {
		options.NoSignSet = true
		options.NoSign = *noSign == "" || strings.EqualFold(*noSign, "true")
	}
	if *emitWorkflow != unset {
		options.EmitWorkflowSet = true
		options.EmitWorkflow = *emitWorkflow
	}
	if *manage != unset {
		options.ManageRequested = true
		options.Manage = *manage
	}
	// Comparer des copies, c'est forcément gérer un travail existant : le
	// drapeau ouvre donc le mode gestion de lui-même. Sans préfixe, l'assistant
	// demande lequel, comme « --manage » sans valeur.
	// « --grant » se sert du même « --export-zip » pour dire où écrire, mais il
	// ne gère aucun travail : c'est la demande qui dit lequel. Et
	// « --publish-index » sans travail ne gère rien non plus : il rattrape tous
	// ceux qui n'ont pas d'index.
	if options.Plagiarism || (options.PublishIndex && options.Manage != "") ||
		(options.ExportZip != "" && !options.decidesAsk()) {
		options.ManageRequested = true
	}
	if *importer != unset {
		options.ImportRequested = true
		options.Import = *importer
	}
	if *template != unset {
		options.TemplateSet = true
		options.Template = *template
	}
	if *starter != unset {
		options.StarterSet = true
		options.Starter = *starter
	}
	if *delay >= 0 {
		options.DelaySet = true
		options.Delay = *delay
	}
	// Une date vide reste une date : « --due "" » retire l'échéance, alors que
	// le drapeau absent ne demande rien.
	if *due != unset {
		options.DueSet = true
		if options.Due, err = valid.NormalizeDue(*due); err != nil {
			return nil, err
		}
	}
	// Repartager un travail suppose de savoir lequel : le dire ici évite un
	// refus surgi du fond de la distribution.
	if options.TeamShare && strings.TrimSpace(options.Assignment) == "" {
		return nil, valid.Errorf(
			"--team-share : indiquez le travail à repartager avec « --assignment a26.5n6.01.tp1 ».")
	}
	// « --team-delete-repos » seul détruirait sans qu'on ait demandé la
	// suppression : il accompagne « --team-delete », il ne la remplace pas.
	if options.TeamDeleteRepos && !options.TeamDelete {
		return nil, valid.Errorf(
			"--team-delete-repos accompagne « --team-delete » : ajoutez-le pour supprimer l'équipe.")
	}
	if options.Jobs < 1 {
		options.Jobs = 1
	}
	if options.Depth < 0 {
		options.Depth = 0
	}
	return options, nil
}

// decidesAsk dit si les drapeaux tranchent ou déposent une demande. Ces
// gestes-là ne gèrent aucun travail : la demande dit elle-même lequel.
func (o *Options) decidesAsk() bool {
	return o.Requests || strings.TrimSpace(o.Grant) != "" ||
		strings.TrimSpace(o.Deny) != "" || strings.TrimSpace(o.Ask) != ""
}

// splitList découpe une liste écrite d'un trait — « eq1,eq2 » ou « eq1 eq2 » —
// en écartant les vides : une virgule en trop ne doit pas produire un nom vide.
func splitList(value string) []string {
	items := make([]string, 0, 4)
	for _, item := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == ';'
	}) {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// translateFlagError met en français les messages du paquet flag.
func translateFlagError(err error) error {
	message := err.Error()
	if name, found := strings.CutPrefix(message, "flag provided but not defined: "); found {
		return valid.Errorf(
			"Drapeau inconnu : « %s ». Lancez « gh cohorte --help » pour la liste des drapeaux.", name)
	}
	if rest, found := strings.CutPrefix(message, "flag needs an argument: "); found {
		return valid.Errorf("Valeur manquante pour le drapeau %s.", rest)
	}
	if strings.HasPrefix(message, "invalid value ") {
		return valid.Errorf("Valeur invalide : %s.", strings.TrimPrefix(message, "invalid value "))
	}
	return valid.Errorf("Ligne de commande : %s.", message)
}

// normalizeArgs permet d'écrire « --manage tp1 » comme « --manage=tp1 », et
// « --manage » seul comme « --manage= ». Le paquet flag ne sait pas gérer seul
// un drapeau dont la valeur est facultative.
//
// « --teacher » en relève aussi : c'est un booléen à trois états — absent, oui,
// non — et « --teacher » seul doit valoir oui, comme n'importe quel booléen au
// terminal, sans qu'on perde la possibilité d'écrire « --teacher=false ».
func normalizeArgs(args []string) []string {
	optional := map[string]bool{
		"-manage": true, "--manage": true, "-import": true, "--import": true,
		"-teacher": true, "--teacher": true,
	}
	normalized := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !optional[argument] {
			normalized = append(normalized, argument)
			continue
		}
		if index+1 < len(args) && !strings.HasPrefix(args[index+1], "-") {
			normalized = append(normalized, argument+"="+args[index+1])
			index++
			continue
		}
		normalized = append(normalized, argument+"=")
	}
	return normalized
}

// inspection traduit les drapeaux d'inspection en réglage du domaine.
//
// La validation est faite là où les règles vivent — un profil inconnu, un
// langage qui n'existe pas —, et non ici : la ligne de commande et le
// navigateur doivent refuser les mêmes choses avec les mêmes mots.
func (o *Options) inspection() (inspect.Settings, error) {
	settings := inspect.Settings{
		Profile:   strings.TrimSpace(o.Profile),
		Languages: liste(o.Languages),
		Include:   liste(o.Only),
		Exclude:   liste(o.Ignore),
	}
	if _, err := inspect.New(settings, nil); err != nil {
		return settings, err
	}
	return settings, nil
}

// liste découpe une valeur séparée par des virgules.
func liste(value string) []string {
	items := make([]string, 0, 4)
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}
