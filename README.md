# gh-cohorte

Extension [GitHub CLI](https://cli.github.com) qui reproduit le fonctionnement de
GitHub Classroom pour une personne qui enseigne : **un dépôt par étudiant** dans
une organisation, créé à partir d'une liste « nom complet + compte GitHub », puis
la gestion des groupes de dépôts déjà créés — accès, clonage, mises à jour,
suppression.

Toute l'interface est en français. L'extension est écrite en Go et distribuée
précompilée : aucune installation de Go n'est nécessaire pour s'en servir.

Deux façons de s'en servir, pour les mêmes opérations : l'assistant du terminal,
ou une **interface graphique** servie sur la boucle locale et ouverte dans le
navigateur.

```
gh cohorte          # assistant au terminal
gh cohorte --web    # interface graphique
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
| Portée `read:org` | pour lister vos organisations et y lire votre rôle |
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

L'organisation se choisit dans la liste de celles auxquelles vous appartenez,
annotées de ce que vous pouvez y faire — celles où vous êtes propriétaire
d'abord :

```
Organisation GitHub
▸ acme — ACME Éducation  · propriétaire
  college — Collège Untel  · membre, création autorisée
  labo — Laboratoire  · membre
  tierce — Organisation tierce  · membre, création réservée aux propriétaires
  Saisir un autre nom…
```

Le curseur se pose sur celle de la dernière fois. La liste est mise en cache
douze heures ; `--org` la court-circuite, et « Saisir un autre nom… » permet de
viser une organisation absente de la liste.

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
réellement en commun — `projet-final` plutôt que `projet`. Un préfixe qui
recouvre plusieurs travaux distincts est au contraire subdivisé :
`a26-5n6-travailsession` et `a26-4w6-tp1` restent deux groupes au lieu d'être
fondus dans `a26`. Au besoin, « Saisir un autre préfixe… » ouvre n'importe quel
préfixe, y compris plus court.

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

**Le cache de la version précédente est repris** : si `~/.cache/classroom/cache.json`
existe — l'outil Python dont cette extension est la suite —, ses inventaires et
ses noms de profils sont adoptés au premier lancement, sans que rien n'ait à
être retéléchargé. L'ancien fichier n'est pas touché, la reprise n'a lieu qu'une
fois, et une purge volontaire n'est jamais défaite. Les bilans de l'ancien outil
sont lus tels quels : `--report-dir chemin/vers/rapports` suffit à retrouver les
noms complets déjà connus.

```bash
gh cohorte --no-cache       # ignorer le cache pour cette exécution
gh cohorte --clear-cache    # vider le cache puis quitter
```

| Variable d'environnement | Effet |
| --- | --- |
| `NO_COLOR` | retire la couleur |
| `COHORTE_NO_ARROWS` | force les listes numérotées plutôt que les flèches |
| `COHORTE_NO_SHELL_COMPLETION` | complète les chemins sans interroger le shell |

Le menu **Options avancées**, accessible **sans authentification**, affiche
l'emplacement et l'état des trois fichiers gérés par l'outil — réglages, cache,
bilans — et permet de vider le cache ou d'oublier les réglages mémorisés.

## Interface web

`gh cohorte --web` monte un petit serveur sur la boucle locale et ouvre le
navigateur dessus. C'est la même extension, les mêmes vérifications et les mêmes
écritures : seule la façon de poser les questions change.

```
gh cohorte --web
```

```
Interface web
  Adresse : http://127.0.0.1:41287/?jeton=8f3c…
  Le serveur n'écoute que sur cette machine et n'accepte que cette adresse :
  le jeton en fait partie.
  Ctrl-C pour fermer, ou « Quitter » dans le navigateur.
```

L'organisation y tient lieu de **cours**, et l'écran reprend l'organisation de
GitHub Classroom : un en-tête de cours, trois onglets, une liste de travaux, et
un assistant en trois étapes pour en créer un.

| GitHub Classroom | Ici |
| --- | --- |
| Classroom | l'organisation GitHub |
| Assignment | un **travail** : les dépôts partageant un préfixe |
| Roster | la liste des **étudiants**, au format CSV |
| « a accepté le devoir » | a déjà un dépôt dans ce travail |
| Starter code repository | dépôt modèle, ou dossier de fichiers de départ |

Ce que Classroom faisait et que l'outil ne fait pas : pas de lien d'invitation à
distribuer — les dépôts sont créés directement —, pas d'échéance, pas de
correction automatique, pas de travail en équipe.

### Travaux

La liste des travaux détectés dans l'organisation, chacun avec son nombre de
dépôts. En ouvrir un donne la page du travail : un tableau **étudiant par
étudiant** — nom complet, dépôt, visibilité, dernier envoi, accès — et les
actions qui vont avec : retrouver les noms complets, inspecter les accès de tout
le travail, gérer les collaborateurs d'un dépôt, copier ou exporter les URL,
cloner une sélection, mettre à jour des clones, supprimer un dépôt.

### Nouveau travail, en trois étapes

Le bouton vert **Nouveau travail** ouvre l'assistant, calqué sur celui de
Classroom :

1. **Bases du travail** — identifiant, gabarit de nom (le nom d'un dépôt
   s'affiche au fil de la frappe), description, visibilité, invitation des
   étudiants et droit accordé.
2. **Code de départ** — dépôt modèle, ou dossier de fichiers de départ de cette
   machine, et le message du commit.
3. **Étudiants et création** — la liste (fichier de la machine, fichier déposé
   dans la page, ou liste collée), la vérification des comptes GitHub, et
   **l'aperçu des dépôts qui se recalcule à chaque frappe** : chaque nom est
   visible avant la moindre écriture. Puis la simulation, ou la création.

Une fois les dépôts créés, l'interface ouvre la page du travail — comme
Classroom mène à la page du devoir. Depuis cette page, **Ajouter des étudiants**
relance l'assistant directement à l'étape 3, en réutilisant le modèle du travail
et en écartant les étudiants qui ont déjà un dépôt.

### Étudiants

La liste de la cohorte et, pour chaque personne, les travaux où elle a déjà un
dépôt — l'équivalent de la colonne « accepté » de Classroom, déduit des dépôts
existants plutôt que d'une invitation.

### Réglages

Portées du jeton, emplacements des fichiers, purge du cache, marge entre deux
créations, mémorisation des réglages.

---

Les opérations longues — création, clonage, vérification des comptes, inspection
des accès — tournent en arrière-plan et se suivent dans un panneau de
progression, ligne par ligne, avec un bouton d'annulation. Les réglages modifiés
dans l'interface sont mémorisés en quittant, comme après l'assistant du terminal.

Le clonage, les fichiers de départ et les listes CSV désignent des chemins **de
la machine**, pas du navigateur : les champs correspondants se complètent au fil
de la frappe, le serveur local répondant à la place du shell.

L'interface est aussi accessible depuis le menu principal de `gh cohorte`, et
`--no-browser` se contente d'afficher l'adresse sans ouvrir de navigateur — utile
à travers une session SSH avec redirection de port.

## Interface

- Toutes les listes se parcourent aux flèches et défilent quand elles sont
  longues ; les sélections multiples (clonage, URL, mise à jour) sont des cases
  à cocher.
- **Ce qui est retenu se voit** : la ligne sous le curseur est surlignée et
  précédée d'un chevron `▸`, la case cochée porte une croix `[✓]`, et le bouton
  retenu d'une question fermée est plein quand l'autre n'est qu'un contour. Les
  couleurs ne sont pas celles de la palette du terminal, pour rester lisibles sur
  un fond clair comme sur un fond sombre ; le chevron, la croix et le trait épais
  suffisent quand la couleur manque — `NO_COLOR` ou terminal monochrome.
- **Les questions attendant un chemin se complètent à la tabulation** — fichier
  CSV, dossier de fichiers de départ, dossier de clonage, fichier d'export —
  exactement comme dans un shell : `⇥` complète jusqu'à ce qui est certain,
  `⇥⇥` liste les possibilités, `↵` valide, `échap` annule.

  ```
    Chemin du fichier CSV
    ⇥ complète · ⇥⇥ liste · ↵ valide · échap annule
  > ~/cours/co
    cohorte.csv   cohorte-hiver.csv   cours-2026/
  ```

  Après avoir atteint un dossier, une seule tabulation de plus liste ce qu'il
  contient ; sur un champ vide, `⇥⇥` liste le répertoire courant. Quand la
  tabulation n'a rien à ajouter, le nombre de possibilités est annoncé plutôt
  que de laisser croire qu'il ne se passe rien. Les candidats sont
  demandés au shell configuré (`$SHELL` : bash, zsh ou fish), `~` est développé,
  et les questions attendant un dossier ne proposent que des dossiers. Le mode
  ligne suit les mêmes règles, la tabulation arrivant dans la réponse.
- Les raccourcis affichés en bas des listes sont en français (`↑ monter`,
  `espace cocher`, `tab compléter`, `entrée valider`), et une confirmation
  s'accepte par `o` comme par `y`.
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
--web                    ouvrir l'interface graphique sur la boucle locale
--no-browser             avec --web, ne pas ouvrir le navigateur
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
  contient que l'organisation, le travail et les préférences d'affichage. Il est
  écrit à la fin de la session **quelle qu'en soit l'issue** — y compris après
  une annulation ou une interruption —, mais seulement si quelque chose a
  changé, et jamais après un oubli volontaire.
- Les actions destructives — suppression d'un dépôt, retrait d'un accès —
  demandent toujours une confirmation explicite. La suppression exige que le
  **nom exact du dépôt soit retapé** ; aucune option, `--yes` compris, ne
  court-circuite cette confirmation.
- Les données d'étudiants (bilans, listes, clones) sont exclues du dépôt par le
  `.gitignore`.
- **L'interface web n'écoute que sur `127.0.0.1`**, sur un port tiré au hasard à
  chaque lancement. Un jeton de session, lui aussi tiré au hasard, figure une
  seule fois dans l'adresse affichée puis vit dans un témoin `HttpOnly`,
  `SameSite=Strict` : sans lui, toute requête est refusée. L'en-tête `Host` doit
  désigner la boucle locale — une entrée DNS pointant sur `127.0.0.1` ne suffit
  pas —, l'origine de toute requête est vérifiée, et une écriture doit porter un
  en-tête que seule la page sait ajouter : un autre site ouvert dans le même
  navigateur ne peut donc rien déclencher. Le jeton GitHub, lui, ne quitte jamais
  le processus : la page ne parle qu'à cette API locale.

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
| `internal/orgs` | inventaire des organisations : rôle et droit de créer |
| `internal/starter` | lecture d'un dossier de fichiers de départ |
| `internal/config`, `internal/cache` | réglages et cache disque |
| `internal/ghapi` | client de l'API GitHub, bâti sur go-gh |
| `internal/identity` | résolution des noms complets |
| `internal/runner` | exécution du plan et bilans |
| `internal/clone` | clonage et mise à jour |
| `internal/complete` | complétion des chemins, déléguée au shell |
| `internal/ui` | console, questions, barres de progression |
| `internal/web` | serveur local, API JSON et page embarquée |
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
- **L'interface web est une API au-dessus des paquets du domaine, pas un
  terminal déguisé.** `internal/web` appelle `plan`, `groups`, `runner`, `clone`
  et `ghapi` directement : la validation, les gabarits, le refus des collisions
  de noms et la confirmation par nom exact restent au même endroit, et les deux
  interfaces ne peuvent pas diverger sur ce qui compte. Ce qui est propre au
  terminal — questions, barres, couleurs — n'y entre pas ; les opérations longues
  deviennent des travaux en arrière-plan suivis par un flux d'événements.
- **La page est embarquée dans le binaire** (`go:embed`), sans dépendance
  externe ni étape de construction : du HTML, du CSS et du JavaScript écrits à la
  main. La promesse « aucune installation nécessaire » vaut aussi pour
  l'interface graphique, et le graphe de dépendances du module ne bouge pas.
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
- **Les organisations où la création est restreinte restent proposées.** Elles
  sont annotées et placées en fin de liste, sans être masquées : le mode gestion
  — accès, clonage, URL — ne demande aucun droit de création, et GitHub ne
  révèle le réglage `members_can_create_repositories` qu'aux propriétaires, si
  bien qu'une organisation « membre » n'est pas forcément fermée.
- **Les questions de chemin ont leur propre champ.** Dans `huh`, `tab` passe au
  champ suivant, la complétion est sur `ctrl+e` et les suggestions ne
  s'affichent qu'une à une ; rien n'y permet le `tab` / `tab tab` d'un shell.
  Ces questions passent donc par un champ écrit pour l'occasion
  (`internal/ui/pathinput.go`, bâti sur `bubbles/textinput`), qui complète, liste
  et valide comme on s'y attend. Les autres questions restent des champs `huh`.
- **La complétion des chemins passe par le shell, avec un filet.** Les candidats
  viennent de `compgen` (bash), du globbing de `zsh` ou de `complete -C` (fish),
  selon `$SHELL`, complétés par un parcours natif du dossier : selon les
  réglages de chacun, un shell peut être plus avare que l'autre, et la liste est
  de toute façon filtrée, dédoublonnée et triée. Une saisie contenant des
  caractères qu'un shell interpréterait (`$`, backtick, `;`, `|`…) n'est jamais
  passée à un shell : la complétion native s'en charge seule. L'appel est borné
  à 400 ms et mis en cache, pour que la frappe n'attende jamais.
- **Le cache tient dans un seul fichier JSON.** La purge reste triviale et rien
  ne s'éparpille en centaines de petits fichiers ; seuls les champs utiles des
  dépôts y sont conservés (nom, visibilité, URL, date du dernier envoi).
