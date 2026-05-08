package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// StatusbarData is the slice of run-level metadata the status bar renders.
// Producers (Frame today; ScopesModal in a later batch) populate this once
// at construction time so the status bar stays cheap on every render.
type StatusbarData struct {
	ScopeID        string
	RunUUIDShort   string  // 8-char abbreviation of the run uuid
	RunStatus      string  // ok / partial / failed / running
	TotalResources int
	KindCount      int
	StartedAt      time.Time
}

// Statusbar is a stateless single-line ribbon. App constructs a transient
// instance per render — there's nothing to cache; layout is just style +
// data.
type Statusbar struct {
	accent  lipgloss.Style
	dim     lipgloss.Style
	healthy lipgloss.Style
	warn    lipgloss.Style
	err     lipgloss.Style
}

// NewStatusbar builds a Statusbar with the project palette. Pass the same
// styles used elsewhere in the TUI so the bar matches headers/borders.
func NewStatusbar(accent, dim, healthy, warn, err lipgloss.Style) Statusbar {
	return Statusbar{accent: accent, dim: dim, healthy: healthy, warn: warn, err: err}
}

// View renders one line: scope · run <short> · <status> · <count> resources / <kinds> kinds · scanned <relative>.
// Width is informational — long content is clipped by the terminal, but
// callers can pass it forward if they want to truncate explicitly.
func (s Statusbar) View(d StatusbarData) string {
	parts := []string{
		s.accent.Render(d.ScopeID),
		s.dim.Render("·"),
		s.dim.Render("run " + d.RunUUIDShort),
		s.dim.Render("·"),
		s.styleForStatus(d.RunStatus).Render(d.RunStatus),
		s.dim.Render("·"),
		s.dim.Render(fmt.Sprintf("%d resources / %d kinds",
			d.TotalResources, d.KindCount)),
		s.dim.Render("·"),
		s.dim.Render("scanned " + relativeTime(d.StartedAt)),
	}
	return strings.Join(parts, " ")
}

func (s Statusbar) styleForStatus(status string) lipgloss.Style {
	switch strings.ToLower(status) {
	case "ok":
		return s.healthy
	case "partial", "running":
		return s.warn
	case "failed":
		return s.err
	default:
		return s.dim
	}
}

// relativeTime returns "just now", "5m ago", "2h ago", "3d ago" — the
// granularity the user actually cares about for "when was this scanned".
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}
