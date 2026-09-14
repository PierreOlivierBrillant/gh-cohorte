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
		{Org: "acme", Session: "a26", Course: "5n6", Group: "01", Students: []roster.Person{
			{FullName: "Jean-Luc Picard", Username: "jlpicard"},
			{FullName: "Émilie Côté", Username: "emilie-cote", Also: []string{"emilie-perso"}, StudentID: "2100123"},
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
