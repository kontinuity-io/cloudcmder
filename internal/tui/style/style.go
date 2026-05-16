// Package style holds the lipgloss colour tokens, borders, and helpers used
// across the TUI. Lives as a leaf package so both internal/tui (App) and
// internal/tui/screens can import it without creating a cycle.
package style

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Catppuccin Mocha palette tokens.
var (
	ColorHealthy    = lipgloss.Color("#a6e3a1") // Green
	ColorWarning    = lipgloss.Color("#f9e2af") // Yellow
	ColorError      = lipgloss.Color("#f38ba8") // Red
	ColorUnknown    = lipgloss.Color("#9399b2") // Overlay2
	ColorDim        = lipgloss.Color("#6c7086") // Overlay0
	ColorAccent     = lipgloss.Color("#89b4fa") // Blue
	ColorSelectedBg = lipgloss.Color("#313244") // Surface0
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
