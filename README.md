# gh-cohorte

[![tests](https://github.com/PierreOlivierBrillant/gh-cohorte/actions/workflows/tests.yml/badge.svg)](https://github.com/PierreOlivierBrillant/gh-cohorte/actions/workflows/tests.yml)

Extension [GitHub CLI](https://cli.github.com) qui reproduit GitHub Classroom
pour une personne qui enseigne : **un dépôt par étudiant** dans une
organisation, créé à partir d'une liste « nom complet + compte GitHub », puis la
gestion de ce qui existe déjà — accès, clonage, mises à jour, suppression.

Trois façons de s'en servir, pour les mêmes opérations : une interface web
servie sur la boucle locale, un assistant au terminal, et des drapeaux
scriptables.

```bash
gh cohorte          # interface web (défaut)
gh cohorte --cli    # assistant au terminal
```

Écrite en Go et distribuée précompilée : aucune installation de Go n'est
nécessaire. Toute l'interface est en français.

## Installation

```bash
gh extension install PierreOlivierBrillant/gh-cohorte
```

Mise à jour :

```bash
gh extension upgrade cohorte
```

### Prérequis

- **`gh` authentifié** (`gh auth login`) : l'extension reprend son jeton, son
  hôte et ses limites de débit.
- **Portées** : `repo` pour créer les dépôts et inviter les personnes,
  `read:org` pour lister vos organisations. À la demande : `delete_repo` pour
  supprimer un dépôt, `workflow` pour déposer des fichiers dans
  `.github/workflows` (`gh auth refresh -s delete_repo,workflow`).
- **`git`**, uniquement pour cloner et mettre à jour des clones.

Le droit de créer des dépôts dans l'organisation visée est requis ; un rôle
insuffisant est signalé avant toute tentative.

## Démarrage rapide

Lancée sans argument, l'extension ouvre l'interface web dans le navigateur.
Tout y commence par le choix d'une organisation, puis se parcourt de haut en
bas :

```
session  →  cours  →  groupe  →  travail
a26         5N6        01        tp1
```

Un **groupe** rassemble des **étudiants**, à qui l'on **distribue des travaux** :
c'est le modèle de GitHub Classroom, avec ses mots. Chaque niveau a son adresse,
si bien qu'un lien ou un rechargement de page ramène au même endroit.

Au terminal, l'assistant enchaîne authentification → organisation → liste des
personnes → vérification des comptes → paramètres → récapitulatif →
confirmation → création → bilan :

```bash
gh cohorte --cli
```

Pour voir ce qui serait fait, sans rien créer :

```bash
gh cohorte --roster cohorte.csv --dry-run
```

Pour une exécution scriptée, sans aucune question :

```bash
gh cohorte --org acme --assignment tp1 --roster cohorte.csv --non-interactive --yes
```

En mode non interactif, une valeur requise mais absente est une erreur explicite
plutôt qu'une invite laissée en suspens.

## La liste des étudiants

Deux colonnes : le nom complet et le compte GitHub. Les en-têtes sont reconnus
en français comme en anglais (`nom_complet`/`name`, `github_username`/`login`…),
et le séparateur (`,`, `;` ou tabulation) est détecté automatiquement.

```csv
nom_complet,github_username
Émilie Côté,emilie-cote
Jean-Luc Picard,jlpicard
Aminata Diallo,aminata-d
```

Un exemple complet se trouve dans [`examples/cohorte.csv`](examples/cohorte.csv).
Les lignes vides et celles commençant par `#` sont ignorées ; chaque ligne
rejetée est signalée avec son numéro et la raison, et le reste du fichier
continue d'être lu.

## Nommage des dépôts

Un dépôt porte **cinq niveaux, séparés par un point** :

```
session . cours . groupe . travail . étudiant
a26.5n6.01.tp1.emilie-cote
```

Le point est réservé à cette découpe : la slugification remplace tout caractère
non alphanumérique par un tiret, si bien qu'un nom venu d'un CSV en est nettoyé
(« J.-P. Tremblay » devient `j-p-tremblay`) et qu'un compte GitHub n'en contient
jamais. Un nom se relit donc sans rien deviner.

Le dernier niveau est le **nom de l'étudiant**, pas son compte GitHub : un dépôt
se lit sans connaître le pseudonyme de personne. En contrepartie, le nom complet
est obligatoire et deux homonymes font échouer la préparation avant toute
écriture.

**GitHub reste la seule source de vérité** : sessions, cours, groupes, travaux
et étudiants se lisent tous dans le nom des dépôts. Un groupe n'a rien à
déclarer pour exister, et le fichier local ne retient que des choix déjà faits —
la liste importée d'un CSV, les réglages du dernier travail — jamais une
information qui ne serait pas déjà sur GitHub.

Une organisation en cours d'année n'a rien à renommer : les dépôts nommés
autrement sont repérés par préfixe ou décrits par un gabarit
(`projet-{assignment}-{student}`), puis adoptés tels quels. Les renommer reste
possible ensuite, avec un aperçu avant écriture ; GitHub garde une redirection
depuis chaque ancien nom.

## Ce que fait l'outil

- **Distribuer un travail** : dépôt modèle (`--template`) ou dossier de fichiers
  de départ (`--starter`) déposé en un seul commit par l'API Git, sans clone
  local. Un dépôt déjà garni n'est jamais réécrit ; un dépôt resté vide est
  complété à la relance. Un squelette d'exemple :
  [`examples/depart/`](examples/depart).
- **Gérer les accès** : invitations, collaborateurs, droit accordé
  (`--permission`).
- **Cloner et mettre à jour** : en parallèle (`--jobs`), par `git pull
  --ff-only` — un dossier où vous avez travaillé remonte en échec plutôt que
  d'être modifié — et sans jamais écrire le jeton dans une URL ou dans
  `.git/config`, l'authentification passant par `gh auth git-credential`.
- **Filtrer, trier, chercher** les listes d'étudiants et de travaux, à
  l'identique dans les trois interfaces (`--filter`, `--pushed-after`,
  `--pushed-before`, `--never-pushed`, `--sort`).
- **Déplacer un travail ou des étudiants** d'un groupe à l'autre, en renommant
  les dépôts si on le demande.

Ce que GitHub Classroom fait et que l'outil ne fait pas : pas de lien
d'invitation à distribuer — les dépôts sont créés directement —, pas d'échéance,
pas de correction automatique, pas de travail en équipe.

L'assistant du terminal ignore la notion de groupe et travaille par préfixe
(`--manage tp1`). Déclarer un groupe, tenir sa liste d'étudiants ou déplacer une
personne n'existent donc que dans l'interface web ; tout le reste est disponible
partout.

## Fiabilité

L'outil est **idempotent** : un dépôt existant est signalé « déjà présent »,
jamais recréé ni écrasé, et relancer la même commande après correction reprend
là où le lot s'était arrêté. Une erreur sur une personne n'interrompt pas le
reste : les échecs sont collectés et rapportés à la fin, et chaque exécution
dépose un bilan JSON et CSV dans `rapports/`.

Rien n'est écrit sans un récapitulatif suivi d'une confirmation explicite. Sont
vérifiés d'avance : l'existence des comptes et de l'organisation, la validité
des noms de dépôts, les collisions entre deux personnes, le drapeau
`is_template` du dépôt modèle, la taille du dossier de départ.

Les inventaires d'organisation sont mis en cache dans le répertoire du système
(`~/.cache/cohorte/cache.json` sous Linux, permissions `600`) ; `--no-cache` et
`--clear-cache` s'en passent ou le vident.

## Sécurité

- Le jeton n'est jamais affiché, journalisé ni écrit sur le disque : il vient de
  `gh` et ne sert qu'aux en-têtes HTTP.
- L'interface web n'écoute que sur `127.0.0.1`, sur un port tiré au hasard, et
  n'accepte que les requêtes portant un jeton de session lui aussi tiré au
  hasard — l'origine et l'en-tête `Host` sont vérifiés, si bien qu'un autre site
  ouvert dans le même navigateur ne peut rien déclencher.
- Les actions destructives demandent une confirmation explicite ; supprimer un
  dépôt exige d'en retaper le nom exact, et aucune option, `--yes` compris, ne
  court-circuite cette confirmation.
- Les données d'étudiants (bilans, listes, clones) sont exclues du dépôt par le
  `.gitignore`.

## Options

Les plus courantes :

| Drapeau | Effet |
| --- | --- |
| `--org ORG` | organisation GitHub cible |
| `--roster FICHIER` | liste « nom complet, compte GitHub » au format CSV |
| `--assignment NOM` | identifiant du travail |
| `--manage [PREFIXE]` | gérer un groupe existant au lieu d'en créer un |
| `--template ORG/DEPOT` | dépôt modèle |
| `--starter DOSSIER` | dossier local déposé dans chaque dépôt, en un commit |
| `--dry-run` | simuler sans rien créer |
| `-y`, `--yes` | passer la confirmation finale |
| `--non-interactive` | échouer plutôt que poser une question |
| `--cli` / `--no-browser` | rester au terminal / ne pas ouvrir le navigateur |

`gh cohorte --help` donne la liste complète. Codes de retour : `0` succès, `1`
au moins un échec, `2` erreur de validation, `130` interruption.

Trois variables d'environnement : `NO_COLOR` retire la couleur,
`COHORTE_NO_ARROWS` force les listes numérotées, `COHORTE_NO_SHELL_COMPLETION`
complète les chemins sans interroger le shell.

## Développement

```bash
go build .               # produit ./gh-cohorte
go test ./...            # toute la suite, sans aucun accès réseau
gh extension install .   # installer la version locale
```

Les tests montent un faux serveur GitHub local (`internal/fakegh`) et de vrais
dépôts git locaux (`file://`) : rien ne sort de la machine.

La logique vit dans les paquets du domaine — `internal/naming` (la
nomenclature), `internal/classroom` (les groupes), `internal/plan`,
`internal/groups`, `internal/roster`, `internal/students`, `internal/runner`,
`internal/clone` — et les trois interfaces (`internal/web`, `internal/app`) n'en
sont que des façades. C'est ce qui garantit qu'elles ne divergent pas.
[`CLAUDE.md`](CLAUDE.md) énonce les règles à ne pas perdre de vue.

Publication : pousser une étiquette `vX.Y.Z` déclenche le workflow
[`release`](.github/workflows/release.yml), qui construit linux, macOS et
Windows en amd64 comme en arm64.
