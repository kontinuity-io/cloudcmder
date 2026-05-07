// Package core defines the Screen interface and the App-level messages that
// concrete screens emit. Splitting these out prevents an import cycle between
// internal/tui (the App) and internal/tui/screens (the implementations).
package core

import (
	tea "github.com/charmbracelet/bubbletea"

	"cloudcmder.com/internal/store"
)

// RunOwner is implemented by screens that hold a notion of the "current run"
// (Overview, ResourceList, Detail). The App walks its stack top-down looking
// for the first RunOwner so cmdbar `:alias` jumps know which run to open.
type RunOwner interface {
	CurrentRun() *store.RunSummary
}

// Screen is the contract every TUI screen satisfies. Update returns Screen
// (not tea.Model) so the App stack can hold concrete types without asserting.
type Screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (Screen, tea.Cmd)
	View() string
	Title() string
}

// PushScreenMsg pushes a new screen onto the App's stack.
type PushScreenMsg struct{ Screen Screen }

// PopScreenMsg pops the top screen; popping the last screen quits.
type PopScreenMsg struct{}

// ToastMsg displays a transient message at the bottom of the view for ~3s.
type ToastMsg struct{ Text string }

// PushScreenCmd is the conventional helper screens use to navigate forward.
func PushScreenCmd(s Screen) tea.Cmd {
	return func() tea.Msg { return PushScreenMsg{Screen: s} }
}

// PopScreenCmd is the conventional helper screens use to navigate back.
func PopScreenCmd() tea.Cmd {
	return func() tea.Msg { return PopScreenMsg{} }
}

// ToastCmd is the conventional helper for one-shot user-facing messages.
func ToastCmd(text string) tea.Cmd {
	return func() tea.Msg { return ToastMsg{Text: text} }
}
