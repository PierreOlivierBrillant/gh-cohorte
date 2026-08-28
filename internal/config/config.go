// Package config gère les réglages de génération, persistés entre deux exécutions.
// Le jeton d'accès n'y figure jamais.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/PierreOlivierBrillant/gh-cohorte/internal/valid"
)

// Permissions acceptées par l'API pour un collaborateur de dépôt.
var Permissions = []string{"pull", "triage", "push", "maintain", "admin"}

// Visibilities énumère les visibilités possibles d'un dépôt.
var Visibilities = []string{"private", "public"}

// PermissionLabels décrit chaque droit en français, pour les menus.
var PermissionLabels = map[string]string{
	"pull":     "pull — lecture seule",
	"triage":   "triage — lecture et gestion des tickets",
	"push":     "push — lecture et écriture (recommandé)",
	"maintain": "maintain — gestion du dépôt sans réglages sensibles",
	"admin":    "admin — contrôle total",
}

// Valeurs par défaut des gabarits et du message de commit.
const (
	DefaultNamePattern        = "{assignment}-{username}"
	DefaultDescriptionPattern = "{assignment} — {fullname}"
	DefaultCommitMessage      = "Fichiers de départ"
	DefaultDelaySeconds       = 1.0
)

// Settings rassemble les paramètres d'une campagne de génération.
type Settings struct {
	Org                string  `json:"org"`
	Assignment         string  `json:"assignment"`
	NamePattern        string  `json:"name_pattern"`
	DescriptionPattern string  `json:"description_pattern"`
	Template           string  `json:"template"` // « owner/repo » ; vide = dépôt neuf
	Visibility         string  `json:"visibility"`
	Permission         string  `json:"permission"`
	AddCollaborator    bool    `json:"add_collaborator"`
	VerifyAccounts     bool    `json:"verify_accounts"`
	IncludeAllBranches bool    `json:"include_all_branches"`
	DelaySeconds       float64 `json:"delay_seconds"` // marge entre deux créations
	RosterPath         string  `json:"roster_path"`
	StarterDir         string  `json:"starter_dir"`
	CloneDir           string  `json:"clone_dir"`
	CommitMessage      string  `json:"commit_message"`
}

// Default renvoie des réglages neufs.
func Default() Settings {
	return Settings{
		NamePattern:        DefaultNamePattern,
		DescriptionPattern: DefaultDescriptionPattern,
		Visibility:         "private",
		Permission:         "push",
		AddCollaborator:    true,
		VerifyAccounts:     true,
		DelaySeconds:       DefaultDelaySeconds,
		CommitMessage:      DefaultCommitMessage,
	}
}

// Private indique si les dépôts seront créés en privé.
func (s Settings) Private() bool { return s.Visibility != "public" }

// Path renvoie l'emplacement du fichier de réglages, conforme au standard XDG.
func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".", "cohorte", "config.json")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "cohorte", "config.json")
}

// Save enregistre les réglages réutilisables ; le jeton n'y figure jamais.
func (s Settings) Save(path string) error {
	if path == "" {
		path = Path()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return err
	}
	// Un fichier déjà présent conserverait ses anciennes permissions sans cela.
	return os.Chmod(path, 0o600)
}

// Load recharge les réglages ; toute anomalie renvoie les valeurs par défaut.
func Load(path string) Settings {
	if path == "" {
		path = Path()
	}
	settings := Default()
	content, err := os.ReadFile(path)
	if err != nil {
		return settings
	}
	if err := json.Unmarshal(content, &settings); err != nil {
		return Default()
	}
	return settings.normalized()
}

// normalized remplace les valeurs absentes ou aberrantes par les valeurs par défaut.
func (s Settings) normalized() Settings {
	base := Default()
	if s.NamePattern == "" {
		s.NamePattern = base.NamePattern
	}
	if s.DescriptionPattern == "" {
		s.DescriptionPattern = base.DescriptionPattern
	}
	if !contains(Visibilities, s.Visibility) {
		s.Visibility = base.Visibility
	}
	if !contains(Permissions, s.Permission) {
		s.Permission = base.Permission
	}
	if s.CommitMessage == "" {
		s.CommitMessage = base.CommitMessage
	}
	if s.DelaySeconds < 0 {
		s.DelaySeconds = base.DelaySeconds
	}
	return s
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// ValidatePermission vérifie qu'un droit est reconnu par l'API.
func ValidatePermission(value string) (string, error) {
	if !contains(Permissions, value) {
		return "", valid.Errorf("Droit : « %s » est inconnu (attendus : pull, triage, push, maintain, admin).", value)
	}
	return value, nil
}

// ValidateVisibility vérifie qu'une visibilité est reconnue.
func ValidateVisibility(value string) (string, error) {
	if !contains(Visibilities, value) {
		return "", valid.Errorf("Visibilité : « %s » est inconnue (attendues : private, public).", value)
	}
	return value, nil
}
