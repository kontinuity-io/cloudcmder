package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cloudcmder.com/internal/tui/core"
)

// OverviewStub is a placeholder pushed when the user picks a scope or run.
// M4 replaces this file with the real Overview screen.
type OverviewStub struct {
	scopeID string
	runUUID string
}

// NewOverviewStub builds the placeholder screen.
func NewOverviewStub(scopeID, runUUID string) *OverviewStub {
	return &OverviewStub{scopeID: scopeID, runUUID: runUUID}
}

// Title satisfies the screen contract.
func (o *OverviewStub) Title() string { return "Overview: " + o.scopeID }

// Init is a no-op for the stub.
func (o *OverviewStub) Init() tea.Cmd { return nil }

// Update ignores everything; the App's global keys handle Esc/q.
func (o *OverviewStub) Update(msg tea.Msg) (core.Screen, tea.Cmd) { return o, nil }

// View renders a single explanatory line.
func (o *OverviewStub) View() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
		Render("\n  M4 lands the resource overview for run " + o.runUUID + "\n  scope: " + o.scopeID + "\n")
}
