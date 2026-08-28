# gh-cohorte

Extension [GitHub CLI](https://cli.github.com) qui reproduit le fonctionnement de
GitHub Classroom pour une personne qui enseigne : **un dépôt par étudiant** dans
une organisation, créé à partir d'une liste « nom complet + compte GitHub », puis
la gestion des groupes de dépôts déjà créés — accès, clonage, mises à jour,
suppression.

Toute l'interface est en français. L'extension est écrite en Go et distribuée
précompilée : aucune installation de Go n'est nécessaire pour s'en servir.

```
gh cohorte
```

## Installation

```bash
gh extension install PierreOlivierBrillant/gh-cohorte
```

Mise à jour :

```bash
gh extension upgrade cohorte
```

### Prérequis

| Élément | Détail |
| --- | --- |
| `gh` authentifié | `gh auth login` — l'extension reprend le jeton, l'hôte et les limites de débit de la CLI |
| Portée `repo` | nécessaire pour créer les dépôts et inviter les personnes |
| Portée `read:org` | pour lire votre rôle dans l'organisation (avertissement seulement) |
| Portée `delete_repo` | uniquement pour supprimer un dépôt (`gh auth refresh -s delete_repo`) |
| Portée `workflow` | uniquement pour déposer des fichiers dans `.github/workflows` (`gh auth refresh -s workflow`) |
| `git` | uniquement pour cloner et mettre à jour des clones |

Le droit de créer des dépôts dans l'organisation visée est évidemment requis :
l'outil signale un rôle insuffisant avant de tenter quoi que ce soit.

## Démarrage rapide

Lancée sans argument, l'extension ouvre un menu : créer des dépôts, gérer un
groupe existant, options avancées, quitter.

```
gh cohorte
```

L'assistant enchaîne : authentification → organisation → liste des personnes →
vérification des comptes → paramètres → **récapitulatif** → confirmation →
création → bilan.

Pour voir ce qui serait fait, sans rien créer :

```bash
gh cohorte --roster cohorte.csv --dry-run
```

Pour une exécution scriptée, sans aucune question :

```bash
gh cohorte --org acme --assignment tp1 --roster cohorte.csv --non-interactive --yes
```

En mode non interactif, une valeur requise mais absente est une **erreur
explicite** (« Travail manquant : passez --assignment… »), jamais une invite
laissée en suspens.

## La liste des personnes

Deux colonnes : le nom complet et le compte GitHub. Les en-têtes sont reconnus
en français comme en anglais (`nom_complet`/`name`, `github_username`/`login`…),
et le séparateur (`,`, `;` ou tabulation) est détecté automatiquement.

```csv
nom_complet,github_username
Émilie Côté,emilie-cote
Jean-Luc Picard,jlpicard
Aminata Diallo,aminata-d
```

Un exemple se trouve dans [`examples/cohorte.csv`](examples/cohorte.csv). En
l'absence de fichier, l'assistant propose une saisie manuelle guidée et peut
enregistrer le résultat en CSV.

Les lignes vides et celles commençant par `#` sont ignorées. Chaque ligne
rejetée est signalée **avec son numéro et la raison** ; le reste du fichier
continue d'être lu.

## Nommage des dépôts

Gabarit par défaut : `{assignment}-{username}`.

| Champ | Exemple pour « Émilie Côté / emilie-cote », travail `tp1` |
| --- | --- |
| `{assignment}` | `tp1` |
| `{username}` | `emilie-cote` |
| `{name}` | `emilie-cote` (nom complet translittéré en ASCII, en minuscules) |
| `{fullname}` | `Émilie Côté` (à réserver à la description) |
| `{first}` / `{last}` | `emilie` / `cote` |
| `{index}` | `01` |

Le gabarit doit contenir `{username}`, `{name}` ou `{index}` pour que chaque
personne obtienne un dépôt distinct ; deux personnes visant le même nom de dépôt
font échouer la préparation avant toute écriture.

## Gérer un groupe existant

```bash
gh cohorte --manage tp1     # ouvre directement le groupe « tp1 »
gh cohorte --manage         # inventorie l'organisation et propose les groupes
```

Les groupes sont déduits des noms de dépôts : le préfixe retenu est le plus
général qui rassemble au moins deux dépôts, étendu à ce que ses membres ont
réellement en commun — `projet-final` plutôt que `projet`.

Le groupe choisi s'affiche avec, pour chaque dépôt, **le nom complet de la
personne**, la visibilité et la date du dernier envoi :

```
  #  Dépôt            Nom complet      Visibilité  Dernier envoi
  1  tp1-emilie-cote  Émilie Côté      privé       2026-08-21
  2  tp1-jlpicard     Jean-Luc Picard  privé       2026-08-19
```

Les noms complets sont retrouvés du moins cher au plus cher : les **bilans
d'exécution** déjà présents dans `rapports/`, puis le **cache local**, puis le
champ `name` du profil GitHub — ces appels étant menés en parallèle, avec barre
de progression, et mis en cache pour trente jours. Un compte dont le nom reste
inconnu s'affiche par son suffixe, en gris.

| Action | Effet |
| --- | --- |
| Ajouter des dépôts | même flux que la création, restreint aux personnes absentes du groupe |
| Afficher les accès | collaborateurs et invitations en attente de tous les dépôts |
| Afficher les URL | liste brute copiable, export CSV facultatif |
| Gérer les collaborateurs | ajouter, retirer un accès, annuler une invitation |
| Cloner | tout ou partie du groupe vers un dossier désigné |
| Mettre à jour des clones | `git pull --ff-only` groupé sur un dossier déjà cloné |
| Supprimer un dépôt | suppression définitive, confirmation renforcée |
| Recharger, changer de groupe, quitter | navigation |

**Le dépôt modèle est retrouvé tout seul** : l'outil interroge un dépôt existant
du groupe et réutilise son `template_repository`, pour que les dépôts ajoutés
soient identiques aux premiers.

## Fichiers de départ

`--starter` désigne un dossier local dont le contenu est déposé dans chaque
dépôt créé, **en un seul commit**, par l'API Git de GitHub — sans clone local :

```bash
gh cohorte --starter ./squelette-tp1 --commit-message "Squelette du TP1"
```

- Les sous-dossiers, les fichiers cachés et le bit exécutable sont conservés ;
  les fichiers binaires passent tels quels.
- `.git/`, `__pycache__/`, `.venv/`, les caches d'outils, `.DS_Store` et les
  liens symboliques sont écartés, et la liste des exclusions est affichée.
- Combiné à `--template`, le contenu du dossier s'ajoute par-dessus celui du
  modèle ; en cas de nom identique, le dossier local l'emporte.
- Un dépôt **déjà garni n'est jamais réécrit** : le travail déjà remis est
  préservé et le bilan indique « ignoré (dépôt non vide) ». `--force-starter`
  passe outre.
- Un dépôt créé mais resté vide — envoi interrompu — est **complété à la
  relance**.

Chaque fichier compte pour un appel d'API : au-delà d'une quarantaine de
fichiers, l'outil suggère un dépôt modèle (`--template`), qui ne coûte qu'un
appel par dépôt.

Un squelette prêt à servir se trouve dans [`examples/depart/`](examples/depart).

## Clonage

L'action « Cloner » récupère tout ou partie du groupe dans un dossier de votre
choix, en parallèle (`--jobs`, 4 par défaut), avec `--depth` pour ne récupérer
que le dernier état.

Relancer l'action sur un dossier déjà rempli **met à jour** les clones existants
(`git pull --ff-only`) au lieu de les recréer. Un dossier occupé qui n'est pas un
dépôt git est laissé intact et signalé, jamais écrasé. `--ff-only` refuse toute
fusion : un dossier où vous avez travaillé remonte en échec plutôt que d'être
modifié.

Le jeton n'apparaît **jamais** dans une ligne de commande, dans une URL de dépôt
ni dans `.git/config` : l'authentification est déléguée à
`gh auth git-credential`, le fournisseur d'identifiants de la CLI GitHub, réglé
le temps de la commande. Les clones obtenus ont donc un `origin` propre
(`https://github.com/org/depot`), utilisable ensuite avec les identifiants
habituels de chaque personne.

## Validations effectuées avant toute écriture

| Contrôle | Règle |
| --- | --- |
| Compte GitHub | 1 à 39 caractères, tirets simples internes ; `@octocat` et une URL de profil sont acceptés et nettoyés |
| Nom complet | non vide, sans caractère de contrôle, 120 caractères maximum |
| Doublons | un même compte deux fois dans la liste est rejeté |
| Existence des comptes | chaque compte est confronté à l'API ; les absents peuvent être retirés |
| Organisation | existence vérifiée, rôle insuffisant signalé |
| Nom de dépôt | lettres, chiffres, `.`, `-` et `_`, 100 caractères ; `.` et `..` interdits |
| Gabarit | champs inconnus refusés, unicité par personne exigée |
| Collisions | deux personnes ne peuvent pas viser le même nom de dépôt |
| Dépôt modèle | existence vérifiée **et** drapeau `is_template` contrôlé |
| Dossier de départ | existence, contenu non vide, taille et nombre de fichiers plafonnés |

Rien n'est créé sans un récapitulatif suivi d'une confirmation explicite.

## Reprise après incident

L'outil est **idempotent** : un dépôt qui existe déjà est signalé « déjà
présent », jamais recréé ni écrasé ; l'invitation manquante est renvoyée et les
fichiers de départ sont déposés s'il est encore vide. Une erreur sur une personne
n'interrompt pas le reste du lot : les échecs sont collectés et rapportés à la
fin. Il suffit de relancer la même commande une fois le problème corrigé.

Chaque exécution dépose un bilan JSON et CSV dans `rapports/`.

Une **marge d'une seconde** sépare deux créations (`--delay`) : GitHub applique
des limites secondaires sur les écritures en rafale. Les erreurs transitoires
(5xx, quotas, limites secondaires) sont retentées automatiquement, en respectant
l'en-tête `Retry-After`.

## Cache local et options avancées

Lister une organisation coûte plusieurs pages d'API : les résultats sont
conservés dans le répertoire de cache du système (`~/.cache/cohorte/cache.json`
sous Linux), permissions `600` — six heures pour la liste des dépôts, trente
jours pour les noms de profils. Une organisation de plusieurs centaines de
dépôts se réaffiche instantanément.

Le cache est renouvelé dès qu'un dépôt est créé ou supprimé, et l'action
« Recharger la liste » force un rafraîchissement.

```bash
gh cohorte --no-cache       # ignorer le cache pour cette exécution
gh cohorte --clear-cache    # vider le cache puis quitter
```

Le menu **Options avancées**, accessible **sans authentification**, affiche
l'emplacement et l'état des trois fichiers gérés par l'outil — réglages, cache,
bilans — et permet de vider le cache ou d'oublier les réglages mémorisés.

## Interface

- Toutes les listes se parcourent aux flèches et défilent quand elles sont
  longues ; les sélections multiples (clonage, URL, mise à jour) sont des cases
  à cocher.
- Les sélections acceptent aussi une expression : `tous`, `1,3`, `2-5`, un nom
  de dépôt, ou un mélange des trois.
- Barres de progression pour tout ce qui interroge l'API en boucle.
- **Hors terminal** — sortie redirigée, script, journal — rien ne s'affiche en
  couleur ni en animation : aucun retour chariot, aucune séquence d'échappement.
  Un journal reste lisible. Les listes numérotées prennent alors le relais des
  flèches, et les sélections multiples se saisissent comme une expression.
  `COHORTE_NO_ARROWS=1` force ce mode ; `NO_COLOR` retire la couleur.

## Options

```
--org ORG                organisation GitHub cible
--manage [PREFIXE]       gérer un groupe existant au lieu d'en créer un
--roster FICHIER         liste « nom complet, compte GitHub » au format CSV
--assignment NOM         identifiant du travail (préfixe des dépôts)
--template ORG/DEPOT     dépôt modèle (vide = dépôt neuf initialisé)
--pattern GABARIT        gabarit de nom des dépôts
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
--host HOTE              hôte GitHub (github.com ou instance Enterprise)
--config FICHIER         fichier de réglages
--report-dir DOSSIER     dossier des bilans (défaut : rapports)
--no-save-config         ne pas mémoriser les réglages
--no-cache               ignorer le cache local
--clear-cache            vider le cache local puis quitter
--version                afficher la version
```

Codes de retour : `0` succès, `1` au moins un échec, `2` erreur de validation,
`130` interruption ou annulation.

## Sécurité

- Le jeton n'est jamais affiché, journalisé ni écrit sur le disque : il vient de
  `gh` et ne sert qu'aux en-têtes HTTP.
- Le fichier de réglages (`~/.config/cohorte/config.json`, permissions `600`) ne
  contient que l'organisation, le travail et les préférences d'affichage.
- Les actions destructives — suppression d'un dépôt, retrait d'un accès —
  demandent toujours une confirmation explicite. La suppression exige que le
  **nom exact du dépôt soit retapé** ; aucune option, `--yes` compris, ne
  court-circuite cette confirmation.
- Les données d'étudiants (bilans, listes, clones) sont exclues du dépôt par le
  `.gitignore`.

## Développement

```bash
go build .          # produit ./gh-cohorte
go test ./...       # toute la suite, sans aucun accès réseau
gh extension install .   # installer la version locale
```

Les tests montent un **faux serveur GitHub** local (`internal/fakegh`) qui imite
les points d'API utilisés, et de **vrais dépôts git locaux** (`file://`) pour le
clonage. Ils couvrent notamment l'idempotence, l'échec isolé au milieu d'un lot,
le dépôt garni préservé, la reprise après un envoi interrompu, la suppression
annulée quand le nom retapé ne correspond pas, les sélections invalides et la
sortie propre hors terminal.

| Paquet | Rôle |
| --- | --- |
| `internal/valid` | validation et normalisation des saisies |
| `internal/roster` | lecture et écriture des listes CSV |
| `internal/plan` | gabarits et plan de génération |
| `internal/groups` | détection des groupes, sélections |
| `internal/starter` | lecture d'un dossier de fichiers de départ |
| `internal/config`, `internal/cache` | réglages et cache disque |
| `internal/ghapi` | client de l'API GitHub, bâti sur go-gh |
| `internal/identity` | résolution des noms complets |
| `internal/runner` | exécution du plan et bilans |
| `internal/clone` | clonage et mise à jour |
| `internal/ui` | console, questions, barres de progression |
| `internal/app` | assemblage : drapeaux, assistant, gestion, options avancées |

Publication : pousser une étiquette `vX.Y.Z` déclenche le workflow
[`release`](.github/workflows/release.yml), qui construit linux, macOS et Windows
en amd64 comme en arm64 (`script/build.sh`).

## Décisions d'architecture

Ces points étaient ambigus dans le cahier des charges ; voici ce qui a été
tranché, et pourquoi.

- **Nom `gh-cohorte`.** `gh-classroom` est déjà pris par l'extension officielle
  `github/gh-classroom`. Le dépôt et l'exécutable portent donc le nom
  `gh-cohorte`, et la commande s'écrit `gh cohorte`.
- **Authentification du clonage par `gh auth git-credential`.** Le cahier des
  charges laissait le choix entre `gh` et `go-git`. Passer par le fournisseur
  d'identifiants de `gh`, réglé en argument de la commande `git` (jamais dans un
  fichier), satisfait la contrainte principale : aucun jeton dans une ligne de
  commande, dans une URL ou dans `.git/config`, et un `origin` propre. Les
  adresses locales (`file://`) sont clonées sans identifiants, ce qui rend les
  tests possibles hors réseau.
- **`--manage` à valeur facultative.** Le paquet `flag` de Go ne sait pas gérer
  un drapeau dont la valeur est optionnelle ; les arguments sont donc
  normalisés en amont, si bien que `--manage`, `--manage tp1` et `--manage=tp1`
  fonctionnent tous les trois.
- **Le mode gestion suppose un terminal.** En mode non interactif, il exige au
  moins un préfixe (`--manage tp1`) et refuse de choisir un groupe à votre
  place.
- **Les questions passent par une interface `ui.Prompter`.** Quatre
  implémentations : `huh` sur un vrai terminal ; un questionneur en mode ligne
  (listes numérotées, sélections par expression) quand la sortie est redirigée
  mais qu'une personne reste au clavier ; un questionneur qui refuse toute
  question en mode script ; et un questionneur scripté qui permet aux tests de
  dérouler des parcours interactifs complets sans terminal.
- **Le résumé de l'API GitHub reste maison, le transport vient de go-gh.**
  `go-gh` fournit le jeton, l'hôte, les en-têtes et le client HTTP ; l'extension
  y ajoute la pagination par en-tête `Link` et une politique de nouvelles
  tentatives sur les limites secondaires, que `go-gh` ne couvre pas.
- **Les listes aux flèches ne se pilotent pas au chiffre.** `huh` offre les
  flèches, le défilement et le filtrage au clavier, mais pas le saut direct à une
  entrée par son numéro ; la numérotation reste disponible dans le mode ligne
  (`COHORTE_NO_ARROWS=1`), où chaque entrée se choisit par son chiffre.
- **Le cache tient dans un seul fichier JSON.** La purge reste triviale et rien
  ne s'éparpille en centaines de petits fichiers ; seuls les champs utiles des
  dépôts y sont conservés (nom, visibilité, URL, date du dernier envoi).
