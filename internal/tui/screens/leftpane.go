package screens

import (
	tea "github.com/charmbracelet/bubbletea"

	"cloudcmder.com/internal/inventory"
)

// LeftPane is the contract for a swappable left-pane component inside Frame.
// Implementations: *Overview (kind-count rows, no per-resource selection) and
// *ResourceList (per-kind resource rows; cursor drives the right pane).
type LeftPane interface {
	Init() tea.Cmd
	Update(tea.Msg) (LeftPane, tea.Cmd)
	View() string
	Title() string

	// SelectedResource is the currently highlighted resource row, or nil if
	// the pane has no per-resource cursor (Overview). Frame uses this to
	// decide whether to render a Detail right pane and which row to render.
	SelectedResource() *rowData

	// SelectedKind is the highlighted kind in panes that operate on kinds
	// (Overview). Returns nil for ResourceList. Frame uses this to swap the
	// left pane on Enter from Overview.
	SelectedKind() *inventory.Kind

	// AbsorbingKeys returns true while the pane has a focused input — e.g.,
	// ResourceList's `/` filter textinput. Frame stops eating keys (Tab,
	// Enter, Esc, `:`, etc.) while this is true so the user can type freely.
	AbsorbingKeys() bool
}
