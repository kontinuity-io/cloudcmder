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

	// leftHistory is the stack of previous left panes — one entry pushed
	// every time the user swaps the left pane via Enter on Overview or via
	// the cmdbar `:alias` palette. Esc pops one entry and restores it,
	// giving k9s-style "back through context" navigation. The Frame itself
	// is only popped (returning to ScopeList) once leftHistory is empty.
	leftHistory []LeftPane

	// rightFor remembers which Resource the current right-pane Detail was
	// built for. Used to debounce re-creation: if the left-pane cursor
	// hasn't moved off this ref, don't rebuild Detail.
	rightFor string

	width  int
	height int
	focus  PaneFocus
	zoomed bool

	// keys
	tabKey     key.Binding
	enterKey   key.Binding
	escKey     key.Binding
	graphKey   key.Binding
	historyKey key.Binding
}

// NewFrame builds a Frame for the given run with Overview as the default
// left pane.
func NewFrame(ctx context.Context, st *store.Store, run store.RunSummary) *Frame {
	f := &Frame{
		ctx: ctx, st: st, run: run,
		focus:      focusLeft,
		tabKey:     key.NewBinding(key.WithKeys("tab")),
		enterKey:   key.NewBinding(key.WithKeys("enter")),
		escKey:     key.NewBinding(key.WithKeys("esc")),
		graphKey:   key.NewBinding(key.WithKeys("g")),
		historyKey: key.NewBinding(key.WithKeys("H")),
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
		f.leftHistory = append(f.leftHistory, f.left)
		f.left = NewResourceList(f.ctx, f.st, f.run, m.Kind)
		f.zoomed = false
		f.right = nil
		f.rightFor = ""
		return f, f.left.Init()
	case core.SwitchRunMsg:
		f.run = m.Run
		f.left = NewOverview(f.ctx, f.st, f.run.ScopeID, f.run.UUID)
		f.leftHistory = nil // a new run resets the navigation stack
		f.right = nil
		f.rightFor = ""
		f.zoomed = false
		return f, f.left.Init()
	case tea.WindowSizeMsg:
		f.width, f.height = m.Width, m.Height
		// Broadcast to both panes so each can size its internal table/text.
		var lc, rc tea.Cmd
		f.left, lc = f.left.Update(msg)
		if f.right != nil {
			updated, c := f.right.Update(msg)
			f.right = updated.(*Detail)
			rc = c
		}
		return f, tea.Batch(lc, rc)
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
			if len(f.leftHistory) > 0 {
				// Pop one step in the left-pane history — restore the
				// previous pane without re-initialising it (its load is
				// already done from the first time we built it).
				idx := len(f.leftHistory) - 1
				f.left = f.leftHistory[idx]
				f.leftHistory = f.leftHistory[:idx]
				f.right = nil
				f.rightFor = ""
				return f, nil
			}
			// Empty history at the root pane: stay put. Esc never exits
			// the Frame in commander mode — `q` quits the program; that's
			// the only way back to a fresh scope picker for v1.0.
			return f, nil
		case key.Matches(k, f.graphKey):
			if f.right != nil {
				return f, core.PushScreenCmd(NewGraphView(f.right.res, f.right.edges))
			}
		case key.Matches(k, f.historyKey):
			return f, core.PushScreenCmd(NewRunHistory(f.ctx, f.st, f.run.ScopeID))
		}
	}

	// Dispatch the message:
	//   - tea.KeyMsg → focused pane only (so cursor moves don't double-trigger)
	//   - everything else (load events, etc.) → both panes so async load
	//     results from Init() reach their owner regardless of focus.
	var cmds []tea.Cmd
	if _, isKey := msg.(tea.KeyMsg); isKey {
		switch f.focus {
		case focusLeft:
			var c tea.Cmd
			f.left, c = f.left.Update(msg)
			cmds = append(cmds, c)
		case focusRight:
			if f.right != nil {
				updated, c := f.right.Update(msg)
				f.right = updated.(*Detail)
				cmds = append(cmds, c)
			}
		}
	} else {
		var lc tea.Cmd
		f.left, lc = f.left.Update(msg)
		cmds = append(cmds, lc)
		if f.right != nil {
			u, rc := f.right.Update(msg)
			f.right = u.(*Detail)
			cmds = append(cmds, rc)
		}
	}

	if c := f.syncRightWithSelection(); c != nil {
		cmds = append(cmds, c)
	}
	return f, tea.Batch(cmds...)
}

// handleEnter reacts to Enter based on what the left pane is selecting.
func (f *Frame) handleEnter() tea.Cmd {
	if f.focus != focusLeft {
		return nil
	}
	if k := f.left.SelectedKind(); k != nil {
		// Overview-style pane: push current onto history, swap the left
		// pane to the kind's ResourceList. Esc later pops back.
		f.leftHistory = append(f.leftHistory, f.left)
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
// SelectedResource has changed since the last render. Returns Detail.Init()'s
// load Cmd so Bubble Tea can run it and route the result back to the right
// pane via the non-key broadcast path in Update.
func (f *Frame) syncRightWithSelection() tea.Cmd {
	r := f.left.SelectedResource()
	if r == nil {
		f.right = nil
		f.rightFor = ""
		return nil
	}
	currentRef := r.res.Ref.String()
	if currentRef == f.rightFor && f.right != nil {
		return nil
	}
	f.right = NewDetail(f.ctx, f.st, f.run, r.res, r.detail)
	f.rightFor = currentRef
	return f.right.Init()
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
	bodyH := f.bodyHeight()
	if f.zoomed && f.right != nil {
		w := f.width
		if w < 4 {
			w = 80
		}
		return style.BorderActive.Width(w - 2).Height(bodyH - 2).Render(f.right.View())
	}

	// Empty-state placeholder for the right pane.
	rightBody := f.right // capture
	rightContent := ""
	if rightBody == nil {
		rightContent = style.Dim.Render("  move cursor onto a resource row\n  to see details here")
	} else {
		rightContent = rightBody.View()
	}

	if f.width >= 100 {
		// Side-by-side, explicit 60/40 split with fixed heights so cursor
		// moves don't reflow the whole screen.
		inner := f.width - 1 // account for the 1-char separator
		leftW := inner * 60 / 100
		rightW := inner - leftW
		leftBox := f.borderFor(focusLeft).
			Width(leftW - 2).Height(bodyH - 2).
			Render(f.left.View())
		rightBox := f.borderFor(focusRight).
			Width(rightW - 2).Height(bodyH - 2).
			Render(rightContent)
		return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, " ", rightBox)
	}
	// Narrow: stacked top/bottom, each gets ~half the body height.
	half := (bodyH - 1) / 2
	leftBox := f.borderFor(focusLeft).
		Width(f.width - 2).Height(half - 2).
		Render(f.left.View())
	rightBox := f.borderFor(focusRight).
		Width(f.width - 2).Height(bodyH - half - 2).
		Render(rightContent)
	return lipgloss.JoinVertical(lipgloss.Left, leftBox, rightBox)
}

// bodyHeight is the vertical room available for the two-pane body — the
// total terminal height minus the header (1 line) and the two footer lines.
func (f *Frame) bodyHeight() int {
	h := f.height - 3
	if h < 8 {
		h = 8
	}
	return h
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
		"esc=back",
		":alias=jump",
		"H=runs",
		"g=graph",
		"q=quit",
	}
	return style.Dim.Render(strings.Join(hints, " · "))
}
