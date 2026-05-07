// Package screens hosts the concrete TUI screens that read from the store.
// Per the architecture dependency rule, this package must NEVER import
// internal/providers/*; only internal/store and internal/inventory are allowed.
package screens

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
	"cloudcmder.com/internal/tui/style"
)

// ScopeSummary is one row in the ScopeList screen.
type ScopeSummary struct {
	ScopeID         string
	DisplayName     string
	LatestUUID      string
	LatestStartedAt time.Time
	LatestStatus    string
	// LatestRun carries the full RunSummary so Frame can be opened directly
	// without an extra FindRunByUUID call.
	LatestRun store.RunSummary
}

type scopesLoadedMsg struct {
	rows []ScopeSummary
	err  error
}

// Scopes is the first screen presented to the user.
type Scopes struct {
	ctx     context.Context
	st      *store.Store
	tbl     table.Model
	rows    []ScopeSummary
	loaded  bool
	loadErr error
	width   int
	height  int
	keymap  scopesKeymap
}

type scopesKeymap struct {
	Open    key.Binding
	History key.Binding
	Rescan  key.Binding
	Export  key.Binding
}

// NewScopes returns a Scopes screen ready to be pushed onto the App stack.
func NewScopes(ctx context.Context, st *store.Store) *Scopes {
	tbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "SCOPE ID", Width: 30},
			{Title: "LAST RUN", Width: 10},
			{Title: "STARTED", Width: 20},
			{Title: "STATUS", Width: 12},
		}),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	return &Scopes{
		ctx: ctx, st: st, tbl: tbl,
		keymap: scopesKeymap{
			Open:    key.NewBinding(key.WithKeys("enter")),
			History: key.NewBinding(key.WithKeys("H")),
			Rescan:  key.NewBinding(key.WithKeys("R")),
			Export:  key.NewBinding(key.WithKeys("e")),
		},
	}
}

// Title satisfies the tui.Screen contract.
func (s *Scopes) Title() string { return "Scopes" }

// Init triggers the initial store query.
func (s *Scopes) Init() tea.Cmd { return s.loadCmd() }

func (s *Scopes) loadCmd() tea.Cmd {
	return func() tea.Msg {
		runs, err := s.st.ListRuns(s.ctx)
		if err != nil {
			return scopesLoadedMsg{err: err}
		}
		return scopesLoadedMsg{rows: uniqueScopes(runs)}
	}
}

// Update handles loading completion, resize, and key navigation.
func (s *Scopes) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case core.SwitchRunMsg:
		// RunHistory was pushed straight from Scopes (no Frame below). After
		// the modal pops, the SwitchRunMsg lands here — open a Frame for
		// the picked run.
		return s, core.PushScreenCmd(NewFrame(s.ctx, s.st, m.Run))
	case scopesLoadedMsg:
		s.loaded = true
		s.loadErr = m.err
		s.rows = m.rows
		s.tbl.SetRows(toTableRows(m.rows))
		if s.height > 0 {
			s.tbl.SetHeight(tableHeight(len(s.rows), s.height))
		}
		return s, nil
	case tea.WindowSizeMsg:
		s.width, s.height = m.Width, m.Height
		s.tbl.SetHeight(tableHeight(len(s.rows), m.Height))
		return s, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(m, s.keymap.Open):
			if len(s.rows) == 0 {
				return s, nil
			}
			cur := s.tbl.Cursor()
			if cur >= 0 && cur < len(s.rows) {
				row := s.rows[cur]
				return s, core.PushScreenCmd(NewFrame(s.ctx, s.st, row.LatestRun))
			}
		case key.Matches(m, s.keymap.History):
			if len(s.rows) == 0 {
				return s, nil
			}
			cur := s.tbl.Cursor()
			if cur >= 0 && cur < len(s.rows) {
				row := s.rows[cur]
				return s, core.PushScreenCmd(NewRunHistory(s.ctx, s.st, row.ScopeID))
			}
		case key.Matches(m, s.keymap.Rescan):
			return s, core.ToastCmd("use cloudcmder --scan <project-id> from CLI for now (M8 wires this in TUI)")
		case key.Matches(m, s.keymap.Export):
			return s, core.ToastCmd("export lands in M7")
		}
	}
	var cmd tea.Cmd
	s.tbl, cmd = s.tbl.Update(msg)
	return s, cmd
}

// View renders the table or the empty-state hint.
func (s *Scopes) View() string {
	switch {
	case !s.loaded:
		return style.Dim.Render("loading scopes…")
	case s.loadErr != nil:
		return lipgloss.NewStyle().Foreground(style.ColorError).
			Render("error loading scopes: " + s.loadErr.Error())
	case len(s.rows) == 0:
		return style.Dim.Render(
			"\n  no scans yet — quit (q) and run:\n    cloudcmder --scan <project-id>\n")
	default:
		return style.BorderActive.Render(s.tbl.View())
	}
}

// uniqueScopes walks newest-first runs and keeps the latest entry per scope_id.
// Pure function, exported for unit tests.
func uniqueScopes(runs []store.RunSummary) []ScopeSummary {
	seen := make(map[string]struct{}, len(runs))
	out := make([]ScopeSummary, 0, len(runs))
	for _, r := range runs {
		if _, ok := seen[r.ScopeID]; ok {
			continue
		}
		seen[r.ScopeID] = struct{}{}
		out = append(out, ScopeSummary{
			ScopeID:         r.ScopeID,
			DisplayName:     r.ScopeName,
			LatestUUID:      r.UUID,
			LatestStartedAt: r.StartedAt,
			LatestStatus:    r.Status,
			LatestRun:       r,
		})
	}
	return out
}

func toTableRows(in []ScopeSummary) []table.Row {
	out := make([]table.Row, len(in))
	for i, s := range in {
		// Status stays plain: bubbles/table v1's selected-row style swallows
		// embedded ANSI and erases the cell content. Status colour still
		// shows in the Overview run header where the badge sits outside any
		// table.
		out[i] = table.Row{
			truncate(s.ScopeID, 30),
			short(s.LatestUUID),
			s.LatestStartedAt.Local().Format(time.RFC3339),
			s.LatestStatus,
		}
	}
	return out
}

func short(uuid string) string {
	if len(uuid) >= 8 {
		return uuid[:8]
	}
	return uuid
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return s[:n-1] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// tableHeight returns the desired bubbles/table height: just enough to fit
// the rows, but never more than the terminal can actually show. Floor at 3
// (header + at least one row) so the table is still recognisable when empty.
func tableHeight(rowCount, termHeight int) int {
	natural := rowCount + 1 // header
	if natural < 3 {
		natural = 3
	}
	cap := termHeight - 6 // breadcrumb + footer + outer border
	if cap < 3 {
		cap = 3
	}
	if natural > cap {
		return cap
	}
	return natural
}
