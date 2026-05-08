// Package core defines the Screen interface and the App-level messages that
// concrete screens emit. Splitting these out prevents an import cycle between
// internal/tui (the App) and internal/tui/screens (the implementations).
package core

import (
	tea "github.com/charmbracelet/bubbletea"

	"cloudcmder.com/internal/inventory"
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

// SwapLeftPaneMsg asks any Frame on the stack to replace its left pane with
// the kind-specific resource list. JumpID is optional: when non-empty, the
// new ResourceList queues a jump so the cursor lands on the matching row as
// soon as its async load completes — used by the `:` palette to "search a
// resource name from anywhere → land on it".
//
// Why this is one message instead of SwapLeftPaneCmd + JumpToResourceCmd:
// tea.Batch runs commands concurrently, so the jump message could arrive
// at the OLD pane before the new pane finishes constructing. Carrying the
// jump target inside the swap message makes the combined operation atomic.
type SwapLeftPaneMsg struct {
	Kind   inventory.Kind
	JumpID string
}

// SwitchRunMsg asks any Frame on the stack to load a different run. Emitted
// by RunHistory when the user picks a run while a Frame is already on the
// stack — replaces the Frame's run in place instead of pushing a new Frame.
type SwitchRunMsg struct{ Run store.RunSummary }


// SwapLeftPaneCmd is the conventional helper for alias-only swaps
// (`:vm`, `:bucket`, …) where there's no specific resource to jump to.
func SwapLeftPaneCmd(kind inventory.Kind) tea.Cmd {
	return func() tea.Msg { return SwapLeftPaneMsg{Kind: kind} }
}

// SwapAndJumpCmd is the helper for fuzzy-palette resource picks: swap
// the left pane to the kind's ResourceList AND position the cursor on
// the row matching jumpID once the load completes.
func SwapAndJumpCmd(kind inventory.Kind, jumpID string) tea.Cmd {
	return func() tea.Msg { return SwapLeftPaneMsg{Kind: kind, JumpID: jumpID} }
}

// SwitchRunCmd is the conventional helper.
func SwitchRunCmd(run store.RunSummary) tea.Cmd {
	return func() tea.Msg { return SwitchRunMsg{Run: run} }
}
