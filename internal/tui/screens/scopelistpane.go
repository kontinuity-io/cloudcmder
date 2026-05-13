package screens

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/style"
)

// ScopeListPane is the top-left pane of SingleView: a thin wrapper around the
// same uniqueScopes() + bubbles/table data path the standalone Scopes screen
// uses, with a cursor-tracking helper that SingleView polls after each Update
// to detect scope-row changes. The existing Scopes screen is left untouched.
type ScopeListPane struct {
	ctx     context.Context
	st      *store.Store
	tbl     table.Model
	spin    spinner.Model
	rows    []ScopeSummary
	loaded  bool
	loadErr error
	width   int
	height  int

	// lastCursor seeds at -1 so the first SelectionMoved call after the
	// initial load fires (cursor 0 != -1) — that's what kicks off the
	// auto-select cascade on launch.
	lastCursor int
}

// NewScopeListPane returns a pane ready to be embedded inside SingleView.
func NewScopeListPane(ctx context.Context, st *store.Store) *ScopeListPane {
	tbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "SCOPE", Width: 20},
			{Title: "STATUS", Width: 10},
		}),
		table.WithFocused(true),
		table.WithHeight(10),
		table.WithStyles(selectedRowStyles()),
	)
	s := spinner.New()
	s.Spinner = spinner.Dot
	return &ScopeListPane{
		ctx: ctx, st: st, tbl: tbl, spin: s,
		lastCursor: -1,
	}
}

// Title satisfies the pane contract; SingleView uses it for the pane header.
func (s *ScopeListPane) Title() string { return "Scopes" }

// AbsorbingKeys reports false — no filter input in v1.
func (s *ScopeListPane) AbsorbingKeys() bool { return false }

// SetInnerWidth resizes the table columns to fit the pane's inner width.
// Keeps the STATUS column at a fixed 10 chars; SCOPE takes the remainder.
func (s *ScopeListPane) SetInnerWidth(w int) {
	if w <= 0 {
		return
	}
	scopeW := w - 11 // 10 for STATUS + 1 char of separator
	if scopeW < 6 {
		scopeW = 6
	}
	s.tbl.SetColumns([]table.Column{
		{Title: "SCOPE", Width: scopeW},
		{Title: "STATUS", Width: 10},
	})
}

// SelectedScope returns the row at the cursor, or nil when not loaded / empty.
func (s *ScopeListPane) SelectedScope() *ScopeSummary {
	if !s.loaded || len(s.rows) == 0 {
		return nil
	}
	cur := s.tbl.Cursor()
	if cur < 0 || cur >= len(s.rows) {
		return nil
	}
	return &s.rows[cur]
}

// SelectionMoved returns (row, true) the first time after a cursor row
// change (and updates the bookkeeping); (nil, false) otherwise. SingleView
// calls this after every Update to drive the scope→summary cascade.
func (s *ScopeListPane) SelectionMoved() (*ScopeSummary, bool) {
	if !s.loaded || len(s.rows) == 0 {
		return nil, false
	}
	cur := s.tbl.Cursor()
	if cur < 0 || cur >= len(s.rows) {
		return nil, false
	}
	if cur == s.lastCursor {
		return nil, false
	}
	s.lastCursor = cur
	row := s.rows[cur]
	return &row, true
}

// Init triggers the store query and kicks the spinner.
func (s *ScopeListPane) Init() tea.Cmd {
	return tea.Batch(s.loadCmd(), s.spin.Tick)
}

func (s *ScopeListPane) loadCmd() tea.Cmd {
	return func() tea.Msg {
		runs, err := s.st.ListRuns(s.ctx)
		if err != nil {
			return scopesLoadedMsg{err: err}
		}
		return scopesLoadedMsg{rows: uniqueScopes(runs)}
	}
}

// Update handles load completion, resize, and key navigation.
func (s *ScopeListPane) Update(msg tea.Msg) (*ScopeListPane, tea.Cmd) {
	switch m := msg.(type) {
	case scopesLoadedMsg:
		s.loaded = true
		s.loadErr = m.err
		s.rows = m.rows
		s.tbl.SetRows(toScopePaneRows(m.rows))
		if s.height > 0 {
			s.tbl.SetHeight(tableHeight(len(s.rows), s.height))
		}
		return s, nil
	case tea.WindowSizeMsg:
		s.width, s.height = m.Width, m.Height
		s.tbl.SetHeight(tableHeight(len(s.rows), m.Height))
		return s, nil
	case spinner.TickMsg:
		if !s.loaded {
			var cmd tea.Cmd
			s.spin, cmd = s.spin.Update(msg)
			return s, cmd
		}
		return s, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		n := len(s.rows)
		switch k.String() {
		case "g", "home":
			s.tbl.SetCursor(0)
			return s, nil
		case "G", "end":
			if n > 0 {
				s.tbl.SetCursor(n - 1)
			}
			return s, nil
		case "j", "down":
			if c := s.tbl.Cursor(); c < n-1 {
				s.tbl.SetCursor(c + 1)
			}
			return s, nil
		case "k", "up":
			if c := s.tbl.Cursor(); c > 0 {
				s.tbl.SetCursor(c - 1)
			}
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.tbl, cmd = s.tbl.Update(msg)
	return s, cmd
}

// View renders the table, the loading spinner, or the error / empty state.
// SingleView draws the outer border around this content.
func (s *ScopeListPane) View() string {
	switch {
	case !s.loaded:
		return s.spin.View() + style.Dim.Render(" loading scopes…")
	case s.loadErr != nil:
		return lipgloss.NewStyle().Foreground(style.ColorError).
			Render("error: " + s.loadErr.Error())
	case len(s.rows) == 0:
		return style.Dim.Render("no scans yet — run\ncloudcmder --scan <project>")
	default:
		return s.tbl.View()
	}
}

// toScopePaneRows shapes ScopeSummary into the two-column SCOPE/STATUS view.
// Pane cells stay plain text — bubbles/table v1 is not ANSI-aware in row
// width calculation (see CLAUDE.md). Status colour will appear in the pane
// detail row of the Overview itself, not here.
func toScopePaneRows(in []ScopeSummary) []table.Row {
	out := make([]table.Row, len(in))
	for i, s := range in {
		out[i] = table.Row{
			truncate(s.ScopeID, 60),
			s.LatestStatus,
		}
	}
	return out
}

