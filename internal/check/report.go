package check

import (
	"fmt"
	"io"
	"strings"
)

// Status grades a finding.
type Status int

const (
	StatusInfo Status = iota
	StatusOK
	StatusWarn
	StatusFail
)

// Finding is one line of the diagnosis, with optional hints below it.
type Finding struct {
	Status Status
	Text   string
	Hints  []string
}

// Section groups findings under a heading.
type Section struct {
	Title    string
	Findings []Finding
}

// Report is the complete diagnosis.
type Report struct {
	Sections []Section
	// Commands are ready-to-copy shell commands for missing pieces.
	Commands []string
}

// Count returns how many findings have the given status.
func (r Report) Count(status Status) int {
	n := 0
	for _, s := range r.Sections {
		for _, f := range s.Findings {
			if f.Status == status {
				n++
			}
		}
	}
	return n
}

// Renderer prints a Report for humans.
type Renderer struct {
	Color bool
}

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiCyan   = "\033[36m"
)

// Render writes the report as a German terminal text.
func (r Renderer) Render(w io.Writer, report Report) error {
	var b strings.Builder
	b.WriteString(r.paint(ansiBold, "Splash-Check") + "\n")
	for _, s := range report.Sections {
		b.WriteString("\n" + r.paint(ansiBold, s.Title) + "\n")
		for _, f := range s.Findings {
			r.writeFinding(&b, f)
		}
	}
	r.writeCommands(&b, report.Commands)
	b.WriteString(fmt.Sprintf("\nErgebnis: %d Fehler, %d Warnungen\n", report.Count(StatusFail), report.Count(StatusWarn)))
	_, err := io.WriteString(w, b.String())
	return err
}

func (r Renderer) writeFinding(b *strings.Builder, f Finding) {
	b.WriteString("  " + r.marker(f.Status) + " " + f.Text + "\n")
	for _, hint := range f.Hints {
		b.WriteString("      → " + hint + "\n")
	}
}

func (r Renderer) writeCommands(b *strings.Builder, commands []string) {
	if len(commands) == 0 {
		return
	}
	b.WriteString("\n" + r.paint(ansiBold, "Zum Kopieren") + "\n")
	for _, c := range commands {
		b.WriteString("  " + r.paint(ansiCyan, c) + "\n")
	}
}

func (r Renderer) marker(s Status) string {
	switch s {
	case StatusOK:
		return r.paint(ansiGreen, "✓")
	case StatusWarn:
		return r.paint(ansiYellow, "!")
	case StatusFail:
		return r.paint(ansiRed, "✗")
	default:
		return "·"
	}
}

func (r Renderer) paint(code, text string) string {
	if !r.Color {
		return text
	}
	return code + text + ansiReset
}

// formatGB prints a size with one decimal and a German decimal comma.
func formatGB(gb float64) string {
	return strings.Replace(fmt.Sprintf("%.1f GB", gb), ".", ",", 1)
}

// formatRoundGB prints an estimate in whole GB.
func formatRoundGB(gb float64) string {
	return fmt.Sprintf("%.0f GB", gb)
}
