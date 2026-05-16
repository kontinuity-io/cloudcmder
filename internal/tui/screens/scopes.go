// Package screens hosts the concrete TUI screens that read from the store.
// Per the architecture dependency rule, this package must NEVER import
// internal/providers/*; only internal/store and internal/inventory are allowed.
package screens

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
	spin    spinner.Model
	rows    []ScopeSummary
	loaded  bool
	loadErr error
	width   int
	height  int
	keymap  scopesKeymap
	// modal is true when this Scopes was pushed via `:scopes` over an
	// existing Frame. In that mode, picking a scope emits SwitchRunMsg
	// (the Frame underneath swaps in place) and pops the modal — instead
	// of the default behaviour of pushing a new Frame on top.
	modal bool
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
		table.WithWidth(80),
		table.WithStyles(selectedRowStyles()),
	)
	s := spinner.New()
	s.Spinner = spinner.Dot
	return &Scopes{
		ctx: ctx, st: st, tbl: tbl, spin: s,
		keymap: scopesKeymap{
			Open:    key.NewBinding(key.WithKeys("enter")),
			History: key.NewBinding(key.WithKeys("H")),
			Rescan:  key.NewBinding(key.WithKeys("R")),
			Export:  key.NewBinding(key.WithKeys("e")),
		},
	}
}

// NewScopesModal returns a Scopes screen configured for "switch project
// without exiting the current Frame" — pushed by App on `:scopes`. Esc
// pops the modal; Enter on a row emits SwitchRunMsg + PopScreenMsg so
// the Frame underneath swaps to the picked scope.
func NewScopesModal(ctx context.Context, st *store.Store) *Scopes {
	s := NewScopes(ctx, st)
	s.modal = true
	return s
}

// Title satisfies the tui.Screen contract.
func (s *Scopes) Title() string {
	if s.modal {
		return "Scopes (modal)"
	}
	return "Scopes"
}

// Init triggers the initial store query and kicks the spinner.
func (s *Scopes) Init() tea.Cmd {
	return tea.Batch(s.loadCmd(), s.spin.Tick)
}

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
		s.tbl.SetWidth(m.Width)
		s.tbl.SetHeight(tableHeight(len(s.rows), m.Height))
		return s, nil
	case spinner.TickMsg:
		if !s.loaded {
			var cmd tea.Cmd
			s.spin, cmd = s.spin.Update(msg)
			return s, cmd
		}
		return s, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(m, s.keymap.Open):
			if len(s.rows) == 0 {
				return s, nil
			}
			cur := s.tbl.Cursor()
			if cur >= 0 && cur < len(s.rows) {
				row := s.rows[cur]
				if s.modal {
					// Modal mode: pop the modal FIRST so the Frame underneath
					// is the active screen when SwitchRunMsg lands. tea.Batch
					// would race; tea.Sequence guarantees order.
					return s, tea.Sequence(
						core.PopScreenCmd(),
						core.SwitchRunCmd(row.LatestRun),
					)
				}
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
			if len(s.rows) == 0 {
				return s, nil
			}
			cur := s.tbl.Cursor()
			if cur >= 0 && cur < len(s.rows) {
				return s, exportRunCmd(s.ctx, s.st, s.rows[cur].LatestRun)
			}
		}
		// Esc in modal mode pops the modal without switching scope. Only
		// honoured for modal mode — at the root, Esc has no defined action
		// and we leave it alone so future bindings aren't blocked.
		if s.modal && m.Code == tea.KeyEsc {
			return s, core.PopScreenCmd()
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
		return s.spin.View() + style.Dim.Render(" loading scopes…")
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

// truncate clips s to a visible width of n columns, ANSI-aware. Cells
// that are already styled (e.g., status bullets) round-trip without
// corrupting the escape sequences. Falls back to the un-styled byte
// truncation when the input has no ANSI (cheap fast-path).
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return ansi.Truncate(s, n-1, "") + "…"
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
