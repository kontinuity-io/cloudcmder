// Package tui hosts the Bubble Tea screen stack and reusable components.
// All colour and border choices route through this file so a future re-skin
// is a single-file change.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	ColorHealthy = lipgloss.Color("#00d4aa")
	ColorWarning = lipgloss.Color("#f5a623")
	ColorError   = lipgloss.Color("#e74c3c")
	ColorUnknown = lipgloss.Color("#888888")
	ColorDim     = lipgloss.Color("#555555")
	ColorAccent  = lipgloss.Color("#7aa2f7")
)

var (
	BorderActive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent)

	BorderInactive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorDim)

	StyleDim    = lipgloss.NewStyle().Foreground(ColorDim)
	StyleAccent = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleToast  = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
)

// StatusStyle maps a run/resource status string to a coloured style. Inputs
// are normalised to upper-case before matching so callers don't need to care
// whether the provider returned "ok" or "OK" or "Running".
func StatusStyle(status string) lipgloss.Style {
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
