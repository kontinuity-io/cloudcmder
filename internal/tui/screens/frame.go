package screens

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
	"cloudcmder.com/internal/tui/style"
)

// PaneFocus identifies which side of the Frame currently has keyboard focus.
type PaneFocus int

const (
	focusLeft PaneFocus = iota
	focusRight
)

// Frame is the persistent commander shell pushed when a scope is selected.
// It owns the two-pane layout (left = Overview/ResourceList, right = Detail)
// plus a header showing run metadata and a footer with the focus indicator.
type Frame struct {
	ctx context.Context
	st  *store.Store
	run store.RunSummary

	left  LeftPane
	right *Detail // nil when no resource is selected

	// rightFor remembers which Resource the current right-pane Detail was
	// built for. Used to debounce re-creation: if the left-pane cursor
	// hasn't moved off this ref, don't rebuild Detail.
	rightFor string

	width  int
	height int
	focus  PaneFocus
	zoomed bool

	// keys
	tabKey   key.Binding
	enterKey key.Binding
	escKey   key.Binding
	graphKey key.Binding
}

// NewFrame builds a Frame for the given run with Overview as the default
// left pane.
func NewFrame(ctx context.Context, st *store.Store, run store.RunSummary) *Frame {
	f := &Frame{
		ctx: ctx, st: st, run: run,
		focus:    focusLeft,
		tabKey:   key.NewBinding(key.WithKeys("tab")),
		enterKey: key.NewBinding(key.WithKeys("enter")),
		escKey:   key.NewBinding(key.WithKeys("esc")),
		graphKey: key.NewBinding(key.WithKeys("g")),
	}
	f.left = NewOverview(ctx, st, run.ScopeID, run.UUID)
	return f
}

// Title satisfies core.Screen.
func (f *Frame) Title() string { return f.run.ScopeID }

// CurrentRun lets the App's :alias palette discover the active run.
func (f *Frame) CurrentRun() *store.RunSummary { return &f.run }

// Init kicks off the left pane's initial load.
func (f *Frame) Init() tea.Cmd {
	return f.left.Init()
}

// Update is the message router. Order:
//   1. Frame-scoped messages (SwapLeftPaneMsg, SwitchRunMsg) — always handled.
//   2. WindowSizeMsg — update own dimensions, broadcast to both panes.
//   3. If the left pane is absorbing keys (filter open) → forward unfiltered.
//   4. Frame keys (Tab, Enter, Esc, g) when not absorbed.
//   5. Otherwise dispatch to the focused pane.
//   6. Re-check left pane selection; rebuild right pane if it moved.
func (f *Frame) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case core.SwapLeftPaneMsg:
		f.left = NewResourceList(f.ctx, f.st, f.run, m.Kind)
		f.zoomed = false
		f.right = nil
		f.rightFor = ""
		return f, f.left.Init()
	case core.SwitchRunMsg:
		f.run = m.Run
		f.left = NewOverview(f.ctx, f.st, f.run.ScopeID, f.run.UUID)
		f.right = nil
		f.rightFor = ""
		f.zoomed = false
		return f, f.left.Init()
	case tea.WindowSizeMsg:
		f.width, f.height = m.Width, m.Height
	}

	// While the left pane absorbs keys (e.g., active filter input), don't let
	// Frame eat Tab/Enter/Esc — let the pane consume them.
	absorbing := f.left.AbsorbingKeys()

	if k, ok := msg.(tea.KeyMsg); ok && !absorbing {
		switch {
		case key.Matches(k, f.tabKey):
			f.toggleFocus()
			return f, nil
		case key.Matches(k, f.enterKey):
			if cmd := f.handleEnter(); cmd != nil {
				return f, cmd
			}
			return f, nil
		case key.Matches(k, f.escKey):
			if f.zoomed {
				f.zoomed = false
				return f, nil
			}
			return f, core.PopScreenCmd()
		case key.Matches(k, f.graphKey):
			if f.right != nil {
				return f, core.PushScreenCmd(NewGraphView(f.right.res, f.right.edges))
			}
		}
	}

	// Dispatch to the appropriate pane.
	var cmd tea.Cmd
	switch f.focus {
	case focusLeft:
		f.left, cmd = f.left.Update(msg)
	case focusRight:
		if f.right != nil {
			updated, c := f.right.Update(msg)
			f.right = updated.(*Detail)
			cmd = c
		}
	}

	f.syncRightWithSelection()
	return f, cmd
}

// handleEnter reacts to Enter based on what the left pane is selecting.
func (f *Frame) handleEnter() tea.Cmd {
	if f.focus != focusLeft {
		return nil
	}
	if k := f.left.SelectedKind(); k != nil {
		// Overview-style pane: swap the left pane to the kind's ResourceList.
		f.left = NewResourceList(f.ctx, f.st, f.run, *k)
		f.right = nil
		f.rightFor = ""
		return f.left.Init()
	}
	if r := f.left.SelectedResource(); r != nil {
		// ResourceList-style pane: zoom Detail to full width.
		f.zoomed = true
		return nil
	}
	return nil
}

// syncRightWithSelection rebuilds the right-pane Detail if the left pane's
// SelectedResource has changed since the last render.
func (f *Frame) syncRightWithSelection() {
	r := f.left.SelectedResource()
	if r == nil {
		f.right = nil
		f.rightFor = ""
		return
	}
	currentRef := r.res.Ref.String()
	if currentRef == f.rightFor && f.right != nil {
		return
	}
	f.right = NewDetail(f.ctx, f.st, f.run, r.res, r.detail)
	f.rightFor = currentRef
	// Kick off the edges load — its result will arrive as edgesLoadedMsg
	// routed to the focused pane. When focus is on the right, Update will
	// catch it; otherwise the load message is lost (we rebuild every
	// selection so a stale message wouldn't help anyway).
	if cmd := f.right.Init(); cmd != nil {
		// Best-effort: invoke the cmd synchronously so the goroutine starts.
		// The result still flows through tea.Msg; we just don't wait.
		go func() {
			_ = cmd()
		}()
	}
}

func (f *Frame) toggleFocus() {
	switch f.focus {
	case focusLeft:
		if f.right != nil {
			f.focus = focusRight
		}
	case focusRight:
		f.focus = focusLeft
	}
}

// View renders the Frame: header (always), body (split or zoomed),
// footer hint. Modals stack over this naturally — they're handled by App.
func (f *Frame) View() string {
	header := f.headerView()
	body := f.bodyView()
	footer := f.footerView()
	return strings.Join([]string{header, body, footer}, "\n")
}

func (f *Frame) headerView() string {
	parts := []string{
		style.Accent.Render(f.run.ScopeID),
		style.Dim.Render("·"),
		style.Dim.Render("run " + short(f.run.UUID)),
		style.Dim.Render("·"),
		style.Status(f.run.Status).Render(f.run.Status),
		style.Dim.Render("·"),
		style.Dim.Render(f.left.Title()),
	}
	return strings.Join(parts, " ")
}

func (f *Frame) bodyView() string {
	if f.zoomed && f.right != nil {
		return style.BorderActive.Render(f.right.View())
	}
	leftBox := f.borderFor(focusLeft).Render(f.left.View())
	if f.right == nil {
		hint := style.Dim.Render("\n  (move cursor onto a resource row to see details)\n")
		rightBox := style.BorderInactive.Render(hint)
		return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, " ", rightBox)
	}
	rightBox := f.borderFor(focusRight).Render(f.right.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, " ", rightBox)
}

func (f *Frame) borderFor(p PaneFocus) lipgloss.Style {
	if f.focus == p {
		return style.BorderActive
	}
	return style.BorderInactive
}

func (f *Frame) footerView() string {
	focusStr := "left"
	if f.focus == focusRight {
		focusStr = "right"
	}
	hints := []string{
		"focus: " + focusStr,
		"tab=swap",
		"enter=zoom/drill",
		":alias=jump",
		"H=runs",
		"g=graph",
	}
	return style.Dim.Render(strings.Join(hints, " · "))
}
