// Commande d'essai : monte l'interface web au-dessus d'un faux GitHub, garnie
// de quelques groupes, pour regarder le rendu sans toucher à une vraie
// organisation. Elle ne sert qu'au développement.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/cache"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/classroom"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/config"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/fakegh"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/ghapi"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/roster"
	"github.com/PierreOlivierBrillant/gh-cohorte/internal/web"
)

func main() {
	state := fakegh.NewState()
	for nom, envoi := range map[string]string{
		"a26.5n6.01.tp1.jean-luc-picard": "2026-09-01T10:00:00Z",
		"a26.5n6.01.tp1.emilie-cote":     "2026-09-20T10:00:00Z",
		"a26.5n6.01.tp2.emilie-cote":     "",
		"a26.4w6.01.projet.emilie-cote":  "2026-11-05T10:00:00Z",
		"h27.5n6.02.tp1.emilie-cote":     "2027-02-10T10:00:00Z",
		"h27.5n6.02.tp1.aleksilepaj":     "2027-02-11T10:00:00Z",
	} {
		state.AddRepo("acme", nom, true).PushedAt = envoi
	}
	garnirPourLaComparaison(state)
	faux := fakegh.New(state)
	defer faux.Close()

	client, err := ghapi.New(ghapi.Options{
		Host: "github.com", Token: "jeton", BaseURL: faux.URL(),
		Sleep: func(time.Duration) {}, Now: time.Now,
	})
	if err != nil {
		panic(err)
	}
	if _, err := client.AuthenticatedUser(); err != nil {
		panic(err)
	}

	dossier, _ := os.MkdirTemp("", "apercu")
	reglages := config.Default()
	reglages.Org = "acme"
	fichier := filepath.Join(dossier, "config.json")

	magasin := classroom.Open(classroom.PathNextTo(fichier))
	for _, cours := range []classroom.Classroom{
		{Org: "acme", Session: "a26", Course: "5n6", Group: "01",
			Defaults: gabarit(), Students: []roster.Person{
				{FullName: "Jean-Luc Picard", Username: "jlpicard"},
				{FullName: "Émilie Côté", Username: "emilie-cote", Also: []string{"emilie-perso"}, StudentID: "2100123"},
				{FullName: "Aminata Diallo", Username: "aminata-d"},
				{FullName: "Bruno Tanguay", Username: "btanguay"},
				{FullName: "Claire Otis", Username: "cotis"},
			}},
		{Org: "acme", Session: "a26", Course: "4w6", Group: "01", Students: []roster.Person{
			{FullName: "Émilie Côté", Username: "emilie-cote", StudentID: "2100123"},
		}},
		{Org: "acme", Session: "h27", Course: "5n6", Group: "02", Students: []roster.Person{
			{FullName: "Émilie Côté", Username: "emilie-cote", StudentID: "2100123"},
			{FullName: "Aminata Diallo", Username: "aminata-d"},
			// Un compte repris de dépôts hérités : personne ne l'a jamais nommé.
			{Username: "aleksilepaj"},
		}},
	} {
		valide, err := cours.Validate()
		if err != nil {
			panic(err)
		}
		if _, err := magasin.Save(valide); err != nil {
			panic(err)
		}
	}

	serveur, err := web.New(web.Deps{
		Client: client, Cache: cache.NewIn(filepath.Join(dossier, "cache"), true),
		Settings: reglages, ConfigFile: fichier, Viewer: state.Viewer,
		Host: "github.com", TokenOrigin: "oauth_token", Version: "apercu",
		ReportDir: filepath.Join(dossier, "rapports"), Jobs: 2, SaveConfig: true,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(serveur.URL())
	_ = serveur.Serve(context.Background())
}

// gabarit déclare le dépôt modèle du groupe : c'est lui que la comparaison
// écarte d'office, puisque toutes les copies le portent.
func gabarit() classroom.Defaults {
	defauts := classroom.DefaultsFrom(config.Default())
	defauts.Template = "acme/modele-tp1"
	return defauts
}

// garnirPourLaComparaison remplit les dépôts du premier travail, pour que
// l'écran de détection de plagiat ait de quoi montrer : deux copies identiques,
// une copie renommée, et deux travaux honnêtes.
func garnirPourLaComparaison(state *fakegh.State) {
	state.AddRepo("acme", "modele-tp1", true)
	state.SeedCommit("acme/modele-tp1",
		map[string]string{"src/Inventaire.java": modele}, "main")

	// Une copie d'il y a deux ans, sous le sigle que le cours portait alors :
	// c'est ce que la portée « toutes les sessions » doit retrouver, et elle ne
	// le peut que si l'équivalence a été déclarée.
	state.AddRepo("acme", "h24.5m6.02.tp-1.ancien-eleve", true)
	state.SeedCommit("acme/h24.5m6.02.tp-1.ancien-eleve", map[string]string{
		"src/Inventaire.java": modele, "src/Solution.java": solution,
	}, "main")

	state.AddRepo("acme", ".cohorte", true)
	state.SeedCommit("acme/.cohorte", map[string]string{
		"etudiants.json": `{"version": 2, "users": []}`,
		"regles.json":    reglesDeclarees,
	}, "main")

	copies := map[string]string{
		"a26.5n6.01.tp1.emilie-cote":     solution,
		"a26.5n6.01.tp1.jean-luc-picard": solution,
		"a26.5n6.01.tp1.bruno-tanguay":   renomme,
		"a26.5n6.01.tp1.aminata-diallo":  autre,
		"a26.5n6.01.tp1.claire-otis":     encoreAutre,
	}
	for nom, source := range copies {
		if _, connu := state.Repos["acme/"+nom]; !connu {
			state.AddRepo("acme", nom, true)
		}
		state.SeedCommit("acme/"+nom, map[string]string{
			"src/Inventaire.java": modele,
			"src/Solution.java":   source,
			"README.md":           "# Travail pratique 1\n\nConsignes recopiées du plan de cours.\n",
			"assets/logo.ico":     "\x00\x00 image",
			"node_modules/x/i.js": "module.exports = 1;",
		}, "main")
	}
}

// reglesDeclarees est ce que l'équipe a écrit dans le registre : le cours a
// changé de sigle, et le travail de nom.
const reglesDeclarees = `{
  "version": 1,
  "courses": [
    { "id": "prog3", "label": "Programmation 3", "codes": ["5n6", "5m6"] }
  ],
  "assignments": [
    { "id": "tp1", "aliases": ["tp-1"] }
  ]
}
`

const modele = `
package tp1;

import java.util.ArrayList;
import java.util.List;

public class Inventaire {
    private final List<String> articles = new ArrayList<>();

    public void ajouter(String article) {
        if (article == null || article.isEmpty()) {
            throw new IllegalArgumentException("article vide");
        }
        articles.add(article);
    }

    public boolean contient(String article) {
        return articles.contains(article);
    }

    public int taille() {
        return articles.size();
    }
}
`

const solution = `
public class Solution {
    public int[] statistiques(int[] valeurs) {
        if (valeurs.length == 0) {
            throw new IllegalArgumentException("tableau vide");
        }
        int plusGrand = valeurs[0];
        int plusPetit = valeurs[0];
        int total = 0;
        for (int index = 0; index < valeurs.length; index++) {
            int valeur = valeurs[index];
            if (valeur > plusGrand) {
                plusGrand = valeur;
            }
            if (valeur < plusPetit) {
                plusPetit = valeur;
            }
            total = total + valeur;
        }
        return new int[] { plusPetit, plusGrand, total, total / valeurs.length };
    }

    public void trier(int[] valeurs) {
        for (int passage = 0; passage < valeurs.length - 1; passage++) {
            for (int position = 0; position < valeurs.length - passage - 1; position++) {
                if (valeurs[position] > valeurs[position + 1]) {
                    int temporaire = valeurs[position];
                    valeurs[position] = valeurs[position + 1];
                    valeurs[position + 1] = temporaire;
                }
            }
        }
    }

    public int rechercher(int[] valeurs, int cible) {
        int debut = 0;
        int fin = valeurs.length - 1;
        while (debut <= fin) {
            int milieu = debut + (fin - debut) / 2;
            if (valeurs[milieu] == cible) {
                return milieu;
            }
            if (valeurs[milieu] < cible) {
                debut = milieu + 1;
            } else {
                fin = milieu - 1;
            }
        }
        return -1;
    }
}
`

// renomme est la même copie, tous ses noms changés et sa mise en page refaite :
// c'est le maquillage le moins coûteux, et la mesure ne doit pas s'y laisser
// prendre.
const renomme = `
public class Devoir {
    public int[] bilan(int[] tableau) {
        if (tableau.length == 0) { throw new IllegalArgumentException("vide"); }
        int record = tableau[0];
        int minimum = tableau[0];
        int cumul = 0;
        for (int i = 0; i < tableau.length; i++) {
            int element = tableau[i];
            if (element > record) { record = element; }
            if (element < minimum) { minimum = element; }
            cumul = cumul + element;
        }
        return new int[] { minimum, record, cumul, cumul / tableau.length };
    }

    public void ordonner(int[] tableau) {
        for (int tour = 0; tour < tableau.length - 1; tour++) {
            for (int curseur = 0; curseur < tableau.length - tour - 1; curseur++) {
                if (tableau[curseur] > tableau[curseur + 1]) {
                    int garde = tableau[curseur];
                    tableau[curseur] = tableau[curseur + 1];
                    tableau[curseur + 1] = garde;
                }
            }
        }
    }
}
`

const autre = `
import java.util.Arrays;
import java.util.IntSummaryStatistics;

public class Travail {
    public int[] statistiques(int[] entrees) {
        IntSummaryStatistics bilan = Arrays.stream(entrees).summaryStatistics();
        return new int[] {
            bilan.getMin(), bilan.getMax(), (int) bilan.getSum(), (int) bilan.getAverage()
        };
    }

    public void trier(int[] entrees) {
        Arrays.sort(entrees);
    }

    public int rechercher(int[] entrees, int cible) {
        return Arrays.binarySearch(entrees, cible);
    }
}
`

const encoreAutre = `
import java.util.List;
import java.util.stream.Collectors;
import java.util.stream.IntStream;

public class Rendu {
    public List<Integer> croissant(int[] donnees) {
        return IntStream.of(donnees).boxed().sorted().collect(Collectors.toList());
    }

    public int amplitude(int[] donnees) {
        List<Integer> ordonnees = croissant(donnees);
        if (ordonnees.isEmpty()) {
            return 0;
        }
        return ordonnees.get(ordonnees.size() - 1) - ordonnees.get(0);
    }

    public boolean present(int[] donnees, int cherche) {
        return IntStream.of(donnees).anyMatch(valeur -> valeur == cherche);
    }
}
`
