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

Lancée sans argument, l'extension **ouvre l'interface graphique** dans le
navigateur : c'est là que tout est accessible.

```
gh cohorte
```

Le terminal reste maître dès qu'un drapeau dit quoi faire — `--roster`,
`--assignment`, `--manage`, `--yes` —, et quand la sortie est redirigée ou que
`--non-interactive` est passé, rien ne s'ouvre et rien n'est demandé. `--cli`
ramène explicitement à l'assistant du terminal, qui ouvre alors un menu : créer
des dépôts, gérer un groupe existant, options avancées, quitter.

```
gh cohorte --cli
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

Cette section décrit l'assistant du **terminal**, qui compose encore les noms
par gabarit. L'interface web suit la nomenclature à cinq niveaux décrite plus
bas, qui n'est pas réglable.

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
| Filtrer ou trier la liste | réduit et ordonne l'affichage, et les sélections avec lui |
| Recharger, changer de groupe, quitter | navigation |

### Filtrer et trier la liste

**Filtrer ou trier la liste** réduit ce que le groupe montre et l'ordonne :
chercher un nom ou un compte, ne garder que les envois postérieurs ou
antérieurs à une date, ne garder que les dépôts sans aucun envoi, et trier par
nom complet, par compte GitHub ou par dernier envoi, dans un sens ou dans
l'autre. Les critères en vigueur sont rappelés sous la liste : un filtre qui ne
se voit pas se retourne contre celui qui l'a posé.

**Les sélections suivent la liste** : cloner, exporter les URL ou mettre à jour
des clones ne portent que sur ce qui est affiché. Les actions, elles, continuent
de connaître le groupe entier — filtrer sert à choisir, pas à faire oublier des
dépôts.

Les mêmes critères s'écrivent en ligne de commande, pour un script :

```bash
gh cohorte --cli --manage tp1 --pushed-before 2026-10-01 --sort envoi
gh cohorte --cli --manage tp1 --never-pushed
```

La recherche ignore la casse et les accents : `cote` trouve « Émilie Côté ». Une
personne qui n'a jamais rien envoyé n'a pas de date : elle n'est ni avant ni
après une borne, et c'est `--never-pushed` qui la retrouve.

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

C'est le mode par défaut : un petit serveur sur la boucle locale, et le
navigateur ouvert dessus. La même extension, les mêmes vérifications et les
mêmes écritures — seule la façon de poser les questions change.

```
gh cohorte
```

```
Interface web
  Adresse : http://127.0.0.1:41287/?jeton=8f3c…
  Le serveur n'écoute que sur cette machine et n'accepte que cette adresse :
  le jeton en fait partie.
  Ctrl-C pour fermer, ou « Quitter » dans le navigateur.
```

**Tout commence par le choix d'une organisation.** Au premier lancement, c'est
la seule chose que l'interface propose : rien d'autre n'est accessible tant
qu'elle n'est pas faite. Le choix est mémorisé, et se change ensuite depuis les
réglages.

Vient ensuite une hiérarchie, parcourue de haut en bas :

```
session  →  cours  →  groupe  →  travail
a26         5N6        01        tp1
```

**Chaque niveau a son adresse.** `/s/a26`, `/s/a26/5n6`, `/g/<groupe>/etudiants`,
`/g/<groupe>/travaux/tp1` : recharger la page, revenir en arrière ou garder un
lien ramène au même endroit. Un même bandeau — fil d'Ariane, titre, ce qu'il
faut signaler — suit du haut de la hiérarchie jusqu'au groupe.

Le **nom long d'une session se déduit de son nom court** : `a26` se lit
« Automne 2026 », et `h`, `e`, `p` font hiver, été et printemps. Rien n'oblige à
suivre la convention — un nom court qui ne la suit pas s'affiche tel quel — et
un nom écrit à la main l'emporte toujours.

Les sessions **remontent le calendrier** : la plus récente d'abord, l'année
puis la saison — automne, été, printemps, hiver. `h27` vient donc avant `a26`,
qui vient avant `h26` : c'est la session en cours qu'on ouvre, pas celle d'il y
a trois ans. Un nom court qui ne suit pas la convention ne se range pas dans
cette suite : il passe après, avec ses semblables, par ordre alphabétique.

L'interface reprend le modèle de GitHub Classroom : un **groupe** rassemble des
**étudiants**, à qui l'on **distribue des travaux**.

| GitHub Classroom | Ici |
| --- | --- |
| Classroom | un **groupe** : une session, un cours, une section, des étudiants |
| Roster | la liste des étudiants du groupe |
| Assignment | un **travail** distribué au groupe |
| « a accepté le devoir » | a un dépôt pour ce travail |
| Starter code repository | dépôt modèle, ou dossier de fichiers de départ |

Ce que Classroom faisait et que l'outil ne fait pas : pas de lien d'invitation à
distribuer — les dépôts sont créés directement —, pas d'échéance, pas de
correction automatique, pas de travail en équipe.

### La nomenclature des dépôts

Un dépôt porte **cinq niveaux, séparés par un point** :

```
session . cours . groupe . travail . étudiant
a26.5n6.01.tp1.emilie-cote
```

Une session a un **nom court**, celui qui entre dans les dépôts (`a26`), et un
**nom long** pour l'affichage (`Automne 2026`). Le nom long est partagé par tous
les groupes de la session et vit dans le fichier local : le renommer ne touche
aucun dépôt.

Le point est **réservé** à cette découpe, et rien d'autre ne peut en produire :
la slugification remplace tout caractère non alphanumérique par un tiret, si
bien qu'un nom de cours, de travail ou d'étudiant venu d'un CSV en est nettoyé
sans qu'on ait à s'en occuper — « J.-P. Tremblay » devient `j-p-tremblay`. Un
compte GitHub, lui, n'en contient jamais.

Un nom se relit donc **sans rien deviner** : cinq parties, pas une de plus.
C'est ce qui distingue cette nomenclature de la précédente, tout en tirets :
`a26-5n6-tp1-emilie-cote` ne disait pas où finissait le travail et où commençait
la personne. Une forme intermédiaire a existé, à quatre niveaux — la session n'y
était pas encore un niveau à part (`a26-5n6.1010.tp1.emilie-cote`) : elle se lit
comme l'autre ancienne, et se migre de la même façon.

Le dernier niveau est le **nom de l'étudiant**, pas son compte GitHub. Un dépôt
se lit sans connaître le pseudonyme de personne — au prix d'une contrainte : le
nom complet devient obligatoire, et deux homonymes font échouer la préparation
avant toute écriture, en nommant les deux personnes en cause. Le lien entre un
étudiant et son compte GitHub vit désormais dans la liste du groupe, et sur le
dépôt lui-même sous forme d'invitation — plus dans son nom.

Un cours peut avoir **plusieurs groupes** : `a26.5n6.01` et `a26.5n6.02` sont
deux groupes du même cours, chacun avec sa liste et ses travaux. Et le même
cours revient d'une session à l'autre : `a26.5n6.01` et `h27.5n6.01` ne se
mélangent pas.

### Ce qui vient de GitHub, et ce qui est retenu ici

**GitHub est la seule source de vérité.** Tout ce que l'interface affiche s'y
lit :

| Affiché | Lu où |
| --- | --- |
| les sessions, les cours, les groupes | dans le nom de chaque dépôt |
| le nom long d'une session — « Automne 2026 » | déduit du nom court, `a26` |
| le nom d'un groupe — « Groupe 1010 » | déduit de sa place |
| les travaux, et qui en a un | dans le nom de chaque dépôt |
| le nom d'un étudiant | dans le nom de son dépôt, ou son profil GitHub |
| les accès, les invitations | sur le dépôt lui-même |

Un groupe n'a donc **rien à déclarer pour exister** : ses dépôts suffisent, et il
apparaît dans la hiérarchie dès qu'ils existent. Il se désigne par sa place —
`a26.5n6.1010` —, celle-là même qui est écrite dans le nom de chacun d'eux, si
bien qu'un lien vers lui vaut d'une machine à l'autre.

Le fichier local `groupes.json`, voisin des réglages, ne **retient que des choix
déjà faits**, pour ne pas les redemander :

- la liste « nom complet, compte GitHub » importée d'un CSV — GitHub ne la
  connaîtra qu'une fois les dépôts créés, et un nom de dépôt ne dit pas quel
  compte lui répond ;
- les réglages que les prochains travaux du groupe reprendront ;
- un groupe déclaré à l'avance, en attendant son premier travail.

Rien de ce qui s'affiche n'y est inventé : oublier ce fichier ne fait perdre
aucune information sur ce qui existe, seulement la commodité de ne pas le
retaper. « Oublier la liste et les réglages », dans les réglages d'un groupe,
fait exactement cela — le groupe continue de s'afficher tant qu'il a des dépôts.

### Adopter ce qui existe déjà

Une organisation en cours d'année n'a rien à renommer. Les groupes qui suivent
la nomenclature sont déjà dans la hiérarchie, sans rien avoir eu à déclarer. En
dessous s'affichent les **groupes repérés** — ceux dont les noms ne se lisent
qu'en devinant, et qu'il faut donc confirmer.

```
Sessions                                       [Recharger] [Nouveau groupe]
  Hiver 2027          1 cours · 2 groupes                          h27  ›
  Automne 2026        2 cours · 3 groupes                          a26  ›

Groupes repérés dans l'organisation           [Adopter par gabarit…]
  a26-5n6      travailsession, tp1  dépassée   24 compte(s)  48 dépôts  Adopter
  a26-4w6      tp1                  dépassée   22 compte(s)  22 dépôts  Adopter
```

Cliquer une session donne ses cours, un cours donne ses groupes, un groupe donne
ses travaux. Un fil d'Ariane remonte à chaque niveau.

Adopter un tel préfixe **ne renomme rien** : le groupe reste lisible — ses
dépôts, ses travaux, ses accès s'affichent — mais on ne lui distribue plus tant
que ses dépôts n'ont pas été renommés.

### Adopter ce que rien n'organise

La détection par préfixe ne devine rien de `kickmyb-equipe-3` ou de
`tp1-h23-4204n6-alice` : beaucoup d'organisations n'ont jamais suivi de
convention. « Adopter par gabarit… » ouvre alors un écran où l'on **écrit
comment ces noms sont faits** :

```
Gabarit des noms de dépôts   projet-{assignment}-{student}          [Essayer]

34 dépôt(s) sur 554 · 2 travaux · 22 personne(s)

  Dépôt                       Travail   Personne
  projet-tp1-jlpicard         tp1       jlpicard
  projet-tp1-emilie-cote      tp1       emilie-cote
```

`{assignment}` est le travail, `{student}` la personne — son compte GitHub, ou
son nom. Tout le reste est pris à la lettre. Sans `{assignment}`, tous les
dépôts trouvés forment un seul travail.

Un nom seul ne se découpe pas toujours : `projet-tp1-emilie-cote` peut se lire
`tp1` + `emilie-cote` ou `tp1-emilie` + `cote`. **Les noms s'éclairent les uns
les autres** — `tp1` reconnu chez un voisin non ambigu tranche pour tous. Une
fois la liste des étudiants importée, la lecture devient exacte : chaque nom lui
est confronté personne par personne.

Le groupe garde ce gabarit jusqu'à son renommage. Rien n'est écrit sur GitHub à
l'adoption.

### Renommer, déplacer, migrer un groupe

« Réglages du groupe » propose de **renommer ses dépôts**, qu'il vienne d'une
nomenclature dépassée ou qu'il suive déjà la courante : c'est le même mécanisme,
et il n'y en a qu'un. On y indique la session, le cours et le groupe — préremplis
par sa place actuelle, ou par une découpe proposée de son préfixe hérité
(`a26-5n6` → session `a26`, cours `5n6`) — et le renommage se montre avant que
quoi que ce soit ne soit écrit :

```
Dépôt actuel                          Nouveau nom
a26-5n6-travailsession-jlpicard       a26.5n6.01.travailsession.jean-luc-picard
a26-5n6-travailsession-emilie-cote    a26.5n6.01.travailsession.emilie-cote
a26-5n6-tp1-visiteur                  « visiteur » ne correspond à aucun étudiant du groupe
```

**GitHub garde une redirection depuis chaque ancien nom** : les clones déjà
faits, les liens distribués et les scripts continuent de fonctionner.

Un dépôt est bloqué quand son compte n'est pas dans la liste du groupe, quand
son nom complet manque, ou quand le nom visé existe déjà. La migration refuse
alors de démarrer, en les nommant — le temps de compléter la liste ou de
retrouver les noms. On peut aussi accepter de **les laisser en place** : ils ne
sont pas touchés, et le groupe ne bascule pas tant qu'il en reste, pour
continuer de les voir.

Pour les cas que la détection ne devine pas, **Nouveau groupe** déclare un
groupe à la main : session, cours, groupe, liste d'étudiants. Le nom du dépôt
s'affiche au fil de la frappe.

### Travaux

La page d'un groupe liste ses travaux, chacun avec le nombre d'étudiants servis
— `18 étudiant(s) du groupe sur 24`. En ouvrir un donne la page du travail : un
tableau **étudiant par étudiant** — nom complet, dépôt, visibilité, dernier
envoi, accès — et les actions qui vont avec : inspecter les accès de tout le
travail, gérer les collaborateurs d'un dépôt, copier ou exporter les URL, cloner
une sélection, mettre à jour des clones, supprimer un dépôt.

Un dépôt dont le compte n'est pas dans la liste du groupe reste visible, signalé
« hors liste » : rien n'est caché sous prétexte que la liste est incomplète.

### Déplacer un travail dans un autre groupe

Une organisation reprise en cours de route range parfois **sous un même préfixe
les travaux de plusieurs groupes et de plusieurs sessions** — `travail-de` en
rassemble trois, et rien dans les noms ne dit lequel appartient à qui. Les
séparer ne se fait pas étudiant par étudiant : c'est le travail qui appartient à
un groupe, et c'est lui qu'on en sort.

Chaque ligne de la liste des travaux porte une case, et le bandeau au-dessus
**déplace la sélection**. Le groupe d'arrivée se choisit dans la liste, ou se
déclare au passage — session, cours, numéro. Un travail déplacé seul peut
**prendre un autre nom** à l'arrivée : il entre dans le nom de chaque dépôt,
c'est le moment de le corriger.

Le renommage se montre avant que quoi que ce soit ne soit écrit :

```
Dépôt actuel                  Nouveau nom
travail-de-tp1-jlpicard       a26.5n6.01.tp1.jlpicard
travail-de-tp1-aminata-d      a26.5n6.01.tp1.aminata-d
```

**Le dernier niveau du nom est conservé tel quel quand rien ne permet de faire
mieux.** C'est ce qui rend l'opération possible : un fourre-tout ne connaît
souvent que des comptes GitHub, et exiger le nom complet de chacun avant de
déplacer enfermerait dans un cercle — déplacer réclamerait un nom complet, et le
retrouver réclamerait un groupe déplacé, puisqu'un groupe hérité ne sait pas
nommer un dépôt. Le dépôt arrive donc à la bonne place sous le fragment qu'il
portait, **le groupe l'y reconnaît quand même** — un dernier niveau qui est le
compte GitHub d'un inscrit le rattache à lui —, et le nom complet se corrige
ensuite avec « Renommer… », qui renomme les dépôts au passage.

Les fiches suivent leurs dépôts : les personnes qui en ont un parmi ceux qui
partent rejoignent le groupe d'arrivée, et ne quittent celui de départ que s'il
ne leur y reste rien. Un nom déjà pris arrête le déplacement au lieu de
l'interrompre à mi-chemin, et les listes ne bougent qu'une fois tous les dépôts
renommés.

Au terminal, l'assistant fait la même chose sur le groupe ouvert —
**« Déplacer ce travail vers un groupe »** —, puisqu'un travail y est exactement
ce qu'il lui faut : un préfixe et ses dépôts. Il demande la place d'arrivée et le
nom du travail, montre le renommage, puis confirme. En ligne de commande :

```
gh cohorte --manage travail-de-tp1 --move-to a26.5n6.01 --rename-to tp1 --yes
```

`--dry-run` s'arrête après l'aperçu. L'assistant ne tient pas de liste
d'étudiants : il conserve **tous** les derniers niveaux tels quels, là où
l'interface web y met le nom complet quand elle le connaît.

Ce tableau **se trie et se cherche comme celui des étudiants**, et pour cause :
ce sont les mêmes lignes — un dépôt par personne — passées au même paquet. Le
tri est dans l'en-tête des colonnes *Étudiant*, *Dépôt* et *Dernier envoi* ;
la recherche ignore la casse et les accents ; le bouton *Filtrer* replie ce qui
sert plus rarement — les dépôts sans le moindre envoi, et les bornes du dernier
envoi. « Avec » et « sans dépôt » n'y figurent pas : un travail n'a qu'un dépôt
par personne. Le résumé dit toujours combien de dépôts sont affichés sur
combien, et la sélection ne survit pas à ce que le filtre écarte — on agit sur
ce qu'on voit. Les critères tiennent tant qu'on reste sur le travail, une
suppression ou une redistribution comprises ; en ouvrir un autre les efface.

### Distribuer un travail, en trois étapes

Le bouton vert **Nouveau travail** ouvre l'assistant, calqué sur celui de
Classroom :

1. **Bases du travail** — nom, gabarit (le nom d'un dépôt s'affiche au fil de la
   frappe), description, visibilité, invitation des étudiants et droit accordé.
2. **Code de départ** — dépôt modèle, ou dossier de fichiers de départ de cette
   machine, et le message du commit.
3. **Distribution** — les étudiants du groupe, tous cochés par défaut, et
   **l'aperçu des dépôts qui se recalcule à chaque frappe** : chaque nom est
   visible avant la moindre écriture. Puis la simulation, ou la distribution.

Les étudiants qui ont déjà un dépôt pour ce travail sont écartés et signalés :
redistribuer après avoir ajouté trois personnes ne crée que trois dépôts.
**Distribuer aux manquants**, depuis la page d'un travail, reprend l'assistant
directement à l'étape 3.

Le groupe retient les réglages du dernier travail distribué : le suivant n'a pas
à les retaper.

### Étudiants

La liste du groupe et, pour chaque personne, les travaux où elle a déjà un dépôt
— l'équivalent de la colonne « accepté » de Classroom, déduit des dépôts plutôt
que d'une invitation — avec **la date de son dernier envoi**, tous travaux
confondus. De là : retrouver les noms complets manquants (une fois retrouvés,
ils sont retenus), et remplacer la liste.

**Ajouter un étudiant** demande un nom complet et un compte GitHub : une
inscription tardive n'a pas à passer par le fichier, et le reste de la liste ne
bouge pas. Le compte est vérifié sur GitHub, et un compte déjà présent est
refusé plutôt que fondu — l'ajouter deux fois ne ferait rien, et le taire
laisserait croire le contraire.

Le dialogue liste aussi **les travaux déjà distribués, à cocher** : les dépôts
correspondants sont créés dans la foulée, **aux réglages que le groupe retient**
— visibilité, dépôt modèle, invitation, fichiers de départ —, ceux-là mêmes qui
ont servi aux autres. Une arrivée en cours de session se règle donc d'un seul
écran, au lieu de revenir distribuer travail par travail. Rien coché, la
personne rejoint seulement la liste.

Tout est préparé avant la première écriture : un travail qu'on ne saurait pas
nommer, ou un dossier de fichiers de départ disparu depuis la dernière
distribution, refuse l'ajout entier plutôt que de laisser quelqu'un inscrit à
moitié servi. Le nom complet peut manquer si aucun travail n'est coché : il se
retrouve ensuite depuis le profil, mais aucun dépôt ne peut être nommé d'ici là.

**Renommer…**, sur une ligne, corrige une fiche seule — le nom complet, le
compte GitHub, ou les deux. Une faute de frappe obligeait jusqu'ici à remplacer
la liste entière, donc celle de tout le monde, et à retrouver le fichier.

Le nom complet est le dernier niveau du nom des dépôts : le corriger laisse ceux
qui existent déjà sous l'ancien. Une case propose donc de **les renommer aussi**
— c'est une écriture sur GitHub, qui garde une redirection depuis chaque ancien
nom. Le compte, lui, n'entre pas dans le nom d'un dépôt : le changer ne touche
qu'à la liste, et le nouveau est vérifié sur GitHub comme à l'inscription. Le
plan est composé en entier avant la première écriture : un nom déjà pris refuse
le renommage au lieu de l'interrompre à mi-chemin.

**Le tri est dans l'en-tête des colonnes** : cliquer sur *Étudiant*, *Compte
GitHub* ou *Dernier envoi* trie dessus, recliquer retourne l'ordre. Une flèche
marque la colonne active, et chaque colonne part de son sens naturel — un nom de
A à Z, une date du plus récent au plus ancien.

**Le filtre tient sur une seule ligne** : un champ de recherche, qui sert à
chaque fois, et un bouton *Filtrer* qui replie le reste dans un menu — le
travail, la présence de dépôts (avec, sans, ou avec mais sans le moindre envoi),
et les bornes du dernier envoi. Le bouton compte les critères posés et se colore
quand il y en a : un filtre replié ne doit pas pouvoir se faire oublier.

La recherche ignore la casse et les accents : `cote` trouve « Émilie Côté ».

Le filtrage et le tri se font **sur le serveur local**, pas dans la page : ce
sont les mêmes critères, décidés au même endroit, que ceux de l'assistant du
terminal. « Avant le 1er octobre » veut donc dire la même chose des deux côtés.
Le résumé dit toujours combien de personnes sont affichées sur combien.

Chaque ligne porte une case, et le bandeau au-dessus déplace **la sélection
entière**. Une sélection ne survit pas à ce que le filtre écarte : on déplace ce
qu'on voit.

### Réglages du groupe

Deux blocs, qui ne font pas la même chose :

- **Renommer ou déplacer le groupe** — changer la session, le cours ou le numéro
  du groupe **renomme tous ses dépôts**, avec un aperçu avant d'écrire. C'est le
  même écran qui migre un groupe venu d'une nomenclature dépassée. Il n'y a pas
  d'autre façon de le renommer : son nom est sa place, et sa place est dans le
  nom de ses dépôts.
- **Réglages par défaut des travaux** — ce que les prochains travaux
  reprendront : gabarit de description, dépôt modèle, visibilité, droit accordé.
  Avec, en dessous, de quoi **oublier** la liste et les réglages retenus pour ce
  groupe, sans toucher à un seul dépôt.

### Déplacer des étudiants

Une personne change de groupe en cours de session, et rarement seule :
« Déplacer… » sur une ligne, ou **« Déplacer la sélection… »** pour toutes celles
qui sont cochées, les fait passer d'un groupe à l'autre à l'intérieur d'une même
organisation. Leurs fiches suivent toujours ; **leurs dépôts, seulement si on le
demande** — les renommer est une écriture sur GitHub, et le groupe d'arrivée
doit suivre la nomenclature courante pour savoir les nommer. Sans renommage,
leurs dépôts restent au nom du groupe de départ, qui continue de les montrer.

Un dépôt dont le nom complet manque encore n'arrête pas le déplacement : il
garde le dernier niveau de son nom — souvent le compte GitHub —, arrive quand
même à la bonne place, et se renomme le jour où le nom est retrouvé. C'est la
même règle que pour [le déplacement d'un travail](#déplacer-un-travail-dans-un-autre-groupe).

Le groupe d'arrivée n'a pas à exister d'avance. La liste des destinations se
termine par **« ＋ Nouveau groupe… »**, qui demande la session, le cours et le
numéro, et montre la place composée au fil de la frappe — `a26.5n6.03`. Le
groupe est déclaré au passage, sans avoir à quitter la liste pour le créer
ailleurs. Une place déjà occupée est refusée : ce serait une fusion déguisée, et
le groupe se choisit alors dans la liste.

Le plan de renommage est composé en entier avant la première écriture : un nom
déjà pris arrête le déplacement au lieu de l'interrompre à mi-chemin.

---

Les opérations longues — distribution, clonage, vérification des comptes,
inspection des accès — tournent en arrière-plan et se suivent dans un panneau de
progression, ligne par ligne, avec un bouton d'annulation.

L'assistant du terminal ignore les groupes : il continue de travailler par
préfixe (`--manage tp1`), ce qui revient au même puisque le préfixe d'un travail
est son identifiant : ce qu'il liste est ce que la page d'un travail montre.
Les deux interfaces lisent les mêmes dépôts, et le filtre et le tri de la liste
y sont les mêmes — celle du groupe comme celle d'un travail —, drapeaux compris
— `--filter`, `--pushed-after`, `--pushed-before`, `--never-pushed`, `--sort`,
`--sort-desc`.

**Ce qui n'existe que dans l'interface web**, faute d'une notion de groupe au
terminal : déclarer un groupe, le renommer, l'adopter par gabarit, tenir sa
liste d'étudiants — y inscrire quelqu'un, corriger une fiche, remplacer la
liste —, et **déplacer une personne** d'un groupe à l'autre. Un préfixe ne dit
pas où commence le groupe et où finit le travail ; l'assistant ne peut donc pas
les distinguer, et il n'a pas de liste à tenir — la sienne est le fichier CSV que
`--roster` désigne.

**Déplacer un travail**, en revanche, existe partout : un travail est un préfixe
et ses dépôts, ce dont l'assistant dispose déjà. Il le fait sans liste
d'étudiants, en conservant tous les derniers niveaux — l'interface web y met le
nom complet quand elle le connaît.

Le clonage, les fichiers de départ et les listes CSV désignent des chemins **de
la machine**, pas du navigateur. Chaque champ de chemin a donc un bouton
« Parcourir… » qui ouvre **la fenêtre de sélection du système** — zenity ou
kdialog sous Linux, `osascript` sous macOS, une fenêtre .NET sous Windows. Une
page web ne voit jamais le chemin d'un fichier déposé ; c'est le serveur local,
qui tourne sur la même machine, qui la demande. Là où aucune n'est disponible —
une machine sans session graphique —, l'interface montre son propre explorateur,
qui marche partout. Les champs se complètent aussi au fil de la frappe, le
serveur local répondant à la place du shell.

`--no-browser` se contente d'afficher l'adresse sans ouvrir de navigateur —
utile à travers une session SSH avec redirection de port. `--cli` reste au
terminal.

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
--filter TEXTE           ne lister que les dépôts dont le nom ou le compte contient TEXTE
--pushed-after DATE      ne lister que les envois postérieurs à DATE (AAAA-MM-JJ)
--pushed-before DATE     ne lister que les envois antérieurs à DATE
--never-pushed           ne lister que les dépôts sans aucun envoi
--sort nom|compte|envoi  colonne de tri de la liste (défaut : nom)
--sort-desc              trier du plus grand au plus petit
--roster FICHIER         liste « nom complet, compte GitHub » au format CSV
--assignment NOM         identifiant du travail (préfixe des dépôts)
--move-to PLACE          déplacer le travail géré vers « session.cours.groupe »
--rename-to NOM          nom que le travail déplacé prend à l'arrivée
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
--web                    ouvrir l'interface graphique sur la boucle locale (défaut)
--cli                    rester au terminal : assistant interactif
--no-browser             ne pas ouvrir le navigateur, afficher l'adresse
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
- Le serveur local lit et liste des dossiers de la machine, et peut ouvrir la
  fenêtre de sélection du système : c'est ce qui permet de choisir un fichier
  sans le taper. Ces routes ne sont joignables que depuis la page, sous les mêmes
  contrôles que tout le reste.

## Développement

```bash
go build .          # produit ./gh-cohorte
go test ./...       # toute la suite, sans aucun accès réseau
gh extension install .   # installer la version locale
```

`CLAUDE.md` énonce les deux règles à ne pas perdre de vue : rien ne doit
dépendre d'un système d'exploitation, et chaque fonctionnalité doit exister dans
les trois interfaces — web, assistant du terminal, drapeaux — ce que rend
tenable le fait que la logique vive dans les paquets du domaine.

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
| `internal/groups` | détection des groupes de dépôts, sélections |
| `internal/naming` | la nomenclature des dépôts : composition et relecture |
| `internal/picker` | la fenêtre de sélection du système, et l'explorateur de repli |
| `internal/classroom` | groupes : étudiants, place dans la nomenclature, travaux, déplacements et renommages |
| `internal/students` | liste des étudiants d'un groupe : construction, filtre, tri |
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
- **Un groupe est une convention de nommage, pas une base de données.** Le nom
  d'un dépôt porte l'appartenance ; GitHub reste la seule source de vérité, et
  la nomenclature déjà en place d'une organisation (`a26-5n6-travailsession-…`)
  est reconnue sans rien renommer. Déclarer ou oublier un groupe n'écrit ni
  n'efface rien sur GitHub.
- **Un groupe se désigne par sa place, jamais par un identifiant local.** Un
  numéro tiré au hasard, ou un nom d'affichage saisi à la main, n'auraient de
  sens que sur la machine qui les a écrits : deux enseignants regardant la même
  organisation n'y verraient pas la même chose, et un lien ne vaudrait pas d'un
  poste à l'autre. La place — `a26.5n6.1010` — est dans le nom de chaque dépôt,
  donc partout la même. Elle sert d'adresse dans l'interface comme dans l'API, et
  le fichier local s'y greffe au lieu de s'y substituer. C'est aussi pourquoi le
  nom long d'une session se **déduit** de son nom court plutôt que de s'écrire
  quelque part : « a26 » se lit « Automne 2026 » sur n'importe quelle machine.
  Conséquence assumée : renommer un groupe, c'est renommer ses dépôts.
- **Le fichier local ne retient que des choix déjà faits.** La liste importée
  d'un CSV, les réglages des prochains travaux, un groupe déclaré à l'avance :
  rien qui s'affiche n'y est inventé, et le perdre ne fait perdre aucune
  information sur ce qui existe. L'alternative — un dépôt de service dans
  l'organisation pour y stocker la liste — rendrait le groupe partageable entre
  plusieurs enseignants, au prix d'écritures que personne n'a demandées ; elle
  reste ouverte.
- **Un séparateur réservé rend les noms de dépôts relisibles.** GitHub
  n'autorise que `.`, `-` et `_` en plus des lettres et des chiffres ; le tiret
  servant déjà à l'intérieur des noms, le **point** sépare les cinq niveaux.
  Il est impossible à saisir sans effort particulier : la slugification remplace
  déjà tout caractère non alphanumérique par un tiret, si bien qu'un CSV ponctué
  est nettoyé et qu'un compte GitHub n'en contient jamais. Un nom se découpe donc
  sans rien deviner, là où `a26-5n6-tp1-emilie-cote` demandait de connaître
  d'avance la liste des comptes.
- **Le nom de l'étudiant, plutôt que son compte.** Un dépôt se lit sans
  connaître le pseudonyme de personne. En contrepartie, le nom complet devient
  obligatoire, les homonymes font échouer la préparation avant toute écriture, et
  **le compte GitHub n'est plus déductible du nom du dépôt** : il vit dans la
  liste du groupe, et sur le dépôt sous forme d'invitation.
- **Les anciennes nomenclatures restent lisibles, isolées dans un fichier.**
  `internal/classroom/legacy.go` rassemble tout ce qui ne sert qu'aux dépôts
  nommés avant la forme courante : la forme tout en tirets, relue par gabarit
  inversé (`plan.Matcher`) et par détection de préfixe, et la forme à quatre
  niveaux, qui se découpe déjà mais sans porter la session. Un groupe déclaré
  sous cette dernière est ramené au rang de préfixe hérité à la lecture du
  fichier, plutôt que de viser une session vide. Rien n'y est créé : ces groupes
  s'affichent et se migrent, mais on ne leur distribue plus. Le jour où plus
  aucune organisation n'en contient, le fichier disparaît d'un bloc.
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
