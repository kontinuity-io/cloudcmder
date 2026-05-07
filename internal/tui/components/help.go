package components

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

// Help wraps bubbles/help.Model so screens only need to pass a key.Map.
type Help struct {
	m    help.Model
	full bool
}

// NewHelp returns a help component starting in short (single-line) mode.
func NewHelp() Help {
	return Help{m: help.New()}
}

// Toggle flips between short and full views.
func (h *Help) Toggle() { h.full = !h.full }

// View renders the help line(s) for the given key.Map.
func (h *Help) View(km help.KeyMap) string {
	h.m.ShowAll = h.full
	return h.m.View(km)
}

// Width informs bubbles/help of the available render width so it can wrap.
func (h *Help) Width(w int) { h.m.Width = w }

// Compile-time check that DefaultKeymap (in package tui) satisfies help.KeyMap.
// We avoid the import cycle by accepting a help.KeyMap on View().
var _ key.Binding // keep imports tidy if unused
