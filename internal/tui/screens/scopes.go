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
	case scopesLoadedMsg:
		s.loaded = true
		s.loadErr = m.err
		s.rows = m.rows
		s.tbl.SetRows(toTableRows(m.rows))
		return s, nil
	case tea.WindowSizeMsg:
		s.width = m.Width
		s.tbl.SetHeight(max(5, m.Height-6))
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
				return s, core.PushScreenCmd(NewOverview(s.ctx, s.st, row.ScopeID, row.LatestUUID))
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
		})
	}
	return out
}

func toTableRows(in []ScopeSummary) []table.Row {
	out := make([]table.Row, len(in))
	for i, s := range in {
		out[i] = table.Row{
			truncate(s.ScopeID, 30),
			short(s.LatestUUID),
			s.LatestStartedAt.Local().Format(time.RFC3339),
			style.Status(s.LatestStatus).Render(s.LatestStatus),
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
