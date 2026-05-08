// Package style holds the lipgloss colour tokens, borders, and helpers used
// across the TUI. Lives as a leaf package so both internal/tui (App) and
// internal/tui/screens can import it without creating a cycle.
package style

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	ColorHealthy = lipgloss.Color("#00d4aa")
	ColorWarning = lipgloss.Color("#f5a623")
	ColorError   = lipgloss.Color("#e74c3c")
	ColorUnknown = lipgloss.Color("#a0a4b0")
	ColorDim     = lipgloss.Color("#9aa0b0")
	ColorAccent  = lipgloss.Color("#7aa2f7")
)

var (
	BorderActive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent)

	BorderInactive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorDim)

	Dim    = lipgloss.NewStyle().Foreground(ColorDim)
	Accent = lipgloss.NewStyle().Foreground(ColorAccent)
	Toast  = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
)

// Status maps a run/resource status string to a coloured style. Inputs are
// upper-cased before matching so callers don't need to care whether the
// provider returned "ok" or "OK" or "Running".
func Status(status string) lipgloss.Style {
	switch strings.ToUpper(status) {
	case "OK", "ACTIVE", "RUNNING", "READY":
		return lipgloss.NewStyle().Foreground(ColorHealthy)
	case "PARTIAL", "PENDING", "PROVISIONING", "DEGRADED":
		return lipgloss.NewStyle().Foreground(ColorWarning)
	case "FAILED", "ERROR", "STOPPED", "TERMINATED", "PERMISSION_DENIED":
		return lipgloss.NewStyle().Foreground(ColorError)
	default:
		return lipgloss.NewStyle().Foreground(ColorUnknown)
	}
}

// StatusBullet returns a coloured ● glyph reflecting status. Empty string
// in → empty string out (no bullet for resources without status). Used in
// table cells to save horizontal space and read at a glance.
func StatusBullet(status string) string {
	if status == "" {
		return ""
	}
	return Status(status).Render("●")
}

// Separator returns a dim horizontal rule of the given width — used inside
// bordered containers to divide sections.
func Separator(width int) string {
	if width <= 0 {
		return ""
	}
	return Dim.Render(strings.Repeat("─", width))
}
