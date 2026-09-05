# Consignes pour ce dépôt

`gh cohorte` est une extension du GitHub CLI, écrite en Go, qui reproduit
GitHub Classroom : un dépôt par étudiant dans une organisation. Elle s'utilise
de trois façons — interface web locale, assistant interactif au terminal, et
ligne de commande scriptable. Le README présente l'outil à qui le découvre ; ce
fichier ne dit que ce qui doit rester vrai à chaque changement.

## Rien ne doit dépendre d'un système d'exploitation

L'outil tourne sur Linux, macOS et Windows, et rien ne permet de supposer
lequel. Concrètement :

- Les chemins se composent avec `path/filepath`, jamais par concaténation de
  `/`. Les chemins d'une URL ou d'une API GitHub, eux, restent en `path`.
- Aucune commande externe n'est tenue pour acquise. Ce qui n'existe pas partout
  — un sélecteur de fichiers natif, un ouvreur de navigateur — se détecte à
  l'exécution, et un chemin de repli entièrement interne prend le relais quand
  rien n'est disponible. Un échec de détection n'est jamais une erreur fatale.
- Le séparateur de ligne, la casse des systèmes de fichiers, les droits `0600`
  et `0700` : ce qui n'a pas de sens sur une plateforme doit y être neutre, pas
  bloquant.
- Un test qui dépend d'une commande absente ailleurs se saute (`t.Skip`) plutôt
  que d'échouer.

## Chaque fonctionnalité existe dans les trois interfaces

Une capacité ajoutée à l'une des interfaces doit être atteignable depuis les
deux autres : l'interface web, l'assistant interactif au terminal, et les
drapeaux de la ligne de commande. Si l'une des trois ne peut pas l'offrir —
c'est parfois le cas, un explorateur de fichiers n'a pas d'équivalent
scriptable — il faut le dire explicitement plutôt que de laisser un trou
silencieux : c'est l'une des rares raisons de toucher au README.

La règle qui rend cela tenable : **la logique vit dans les paquets du domaine**
(`internal/naming`, `internal/classroom`, `internal/plan`, `internal/groups`,
`internal/roster`, `internal/students`, `internal/runner`, `internal/clone`), et
les trois interfaces n'en sont que des façades. Une validation, un gabarit, un
refus de collision ne doivent jamais être écrits dans `internal/web` ou dans
`internal/app` : les deux interfaces divergeraient. Un comportement ajouté au
bon endroit est automatiquement disponible partout.

## Le README se modifie rarement

Le README s'adresse à quelqu'un qui découvre l'outil : il doit rester court et
se lire d'un trait. Il a déjà enflé une fois jusqu'à mille lignes, chaque
fonctionnalité y ayant ajouté ses trois paragraphes, et plus personne ne le
lisait. **Ajouter une fonctionnalité n'est pas une raison suffisante de le
modifier.**

Ce qui en est une :

- une étape d'installation ou un prérequis qui change ;
- une commande donnée en exemple qui ne marche plus ;
- une affirmation devenue fausse ;
- une capacité qu'une des trois interfaces ne peut pas offrir, à signaler
  explicitement ;
- un concept sans lequel l'outil ne se comprend pas — la nomenclature des dépôts
  en est un ; le détail d'un écran n'en est pas un.

Le reste se documente ailleurs : `gh cohorte --help` pour les drapeaux, les
commentaires du code pour le *pourquoi* d'une décision, ce fichier pour les
règles. Une section ajoutée au README doit en remplacer une autre, ou tenir en
quelques lignes.

## Le reste

- Le code, les commentaires, les messages et les tests sont en français, accents
  compris. Les identifiants Go publics suivent l'usage du langage.
- Les commentaires disent *pourquoi*, pas *quoi* : le code dit déjà ce qu'il
  fait.
- `gofmt`, `go vet` et `go test ./...` doivent passer avant tout commit.
