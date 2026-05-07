package screens

import (
	tea "github.com/charmbracelet/bubbletea"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
	"cloudcmder.com/internal/tui/style"
)

// ResourceListStub is the placeholder pushed when the user drills into a Kind
// from Overview. M5 replaces this file with the real ResourceList screen
// parameterised by Kind.
type ResourceListStub struct {
	kind inventory.Kind
	run  store.RunSummary
}

// NewResourceListStub builds the placeholder for the given kind+run.
func NewResourceListStub(kind inventory.Kind, run store.RunSummary) *ResourceListStub {
	return &ResourceListStub{kind: kind, run: run}
}

// Title satisfies core.Screen.
func (r *ResourceListStub) Title() string { return "Resources: " + string(r.kind) }

// Init is a no-op for the stub.
func (r *ResourceListStub) Init() tea.Cmd { return nil }

// Update ignores everything; the App's global keys handle Esc/q.
func (r *ResourceListStub) Update(msg tea.Msg) (core.Screen, tea.Cmd) { return r, nil }

// View renders an explanatory placeholder.
func (r *ResourceListStub) View() string {
	return style.Dim.Render(
		"\n  M5 lists " + string(r.kind) + " resources for run " + short(r.run.UUID) + " here.\n" +
			"  scope: " + r.run.ScopeID + "\n",
	)
}
