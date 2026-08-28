package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
)

// Progress affiche l'avancement d'une série d'appels à l'API. Sur un terminal,
// la barre se réécrit sur place ; ailleurs, seules quelques lignes discrètes
// sont émises, sans retour chariot ni séquence d'échappement.
type Progress struct {
	console *Console
	label   string
	total   int
	bar     progress.Model
	last    int
	active  bool
}

// NewProgress prépare une barre de progression.
func NewProgress(console *Console, label string, total int) *Progress {
	bar := progress.New(progress.WithDefaultGradient(), progress.WithWidth(30))
	bar.ShowPercentage = false
	return &Progress{console: console, label: label, total: total, bar: bar, last: -1}
}

// Update affiche l'avancement ; suffix précise l'élément en cours.
func (p *Progress) Update(done int, suffix string) {
	if p.total <= 0 {
		return
	}
	if done > p.total {
		done = p.total
	}
	percent := done * 100 / p.total
	if !p.console.TTY {
		// Hors terminal : une ligne par tranche de 25 %, jamais davantage.
		if p.last >= 0 && percent/25 == p.last/25 && done != p.total {
			return
		}
		p.last = percent
		p.console.Printf("  %s %d/%d (%d %%)", p.label, done, p.total, percent)
		return
	}
	p.active = true
	ratio := float64(done) / float64(p.total)
	line := fmt.Sprintf("  %s %s %d/%d  %d %%", p.label, p.bar.ViewAs(ratio), done, p.total, percent)
	if suffix != "" {
		line += "  " + p.console.Dim(truncate(suffix, 30))
	}
	fmt.Fprintf(p.console.Out, "\r\033[K%s", line)
}

// Finish efface la barre et laisse éventuellement un message final.
func (p *Progress) Finish(message string) {
	if p.console.TTY && p.active {
		fmt.Fprint(p.console.Out, "\r\033[K")
	}
	p.active = false
	if message != "" {
		p.console.Print(message)
	}
}

// Clear efface la barre sans rien afficher.
func (p *Progress) Clear() { p.Finish("") }

func truncate(text string, width int) string {
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

// Spinnerless indique une progression sans total connu (chargement de pages).
func (p *Progress) Line(text string) {
	if p.console.TTY {
		fmt.Fprintf(p.console.Out, "\r\033[K  %s", p.console.Dim(text))
		p.active = true
		return
	}
	if strings.TrimSpace(text) != "" {
		p.console.Note("%s", text)
	}
}
