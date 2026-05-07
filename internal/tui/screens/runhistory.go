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

type runsLoadedMsg struct {
	rows []store.RunSummary
	err  error
}

// RunHistory shows every run for a single scope.
type RunHistory struct {
	ctx     context.Context
	st      *store.Store
	scopeID string
	tbl     table.Model
	rows    []store.RunSummary
	loaded  bool
	loadErr error
	height  int
	open    key.Binding
}

// NewRunHistory builds the screen for the given scope id.
func NewRunHistory(ctx context.Context, st *store.Store, scopeID string) *RunHistory {
	tbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "UUID", Width: 10},
			{Title: "STARTED", Width: 20},
			{Title: "FINISHED", Width: 20},
			{Title: "STATUS", Width: 10},
			{Title: "NOTES", Width: 25},
		}),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	return &RunHistory{
		ctx: ctx, st: st, scopeID: scopeID, tbl: tbl,
		open: key.NewBinding(key.WithKeys("enter")),
	}
}

// Title returns the breadcrumb label.
func (r *RunHistory) Title() string { return "RunHistory: " + r.scopeID }

// Init kicks off the store query.
func (r *RunHistory) Init() tea.Cmd {
	return func() tea.Msg {
		runs, err := r.st.ListRuns(r.ctx)
		if err != nil {
			return runsLoadedMsg{err: err}
		}
		filtered := make([]store.RunSummary, 0, len(runs))
		for _, run := range runs {
			if run.ScopeID == r.scopeID {
				filtered = append(filtered, run)
			}
		}
		return runsLoadedMsg{rows: filtered}
	}
}

// Update handles load completion, resize, and enter to open a run.
func (r *RunHistory) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case runsLoadedMsg:
		r.loaded = true
		r.loadErr = m.err
		r.rows = m.rows
		r.tbl.SetRows(runRows(m.rows))
		if r.height > 0 {
			r.tbl.SetHeight(tableHeight(len(r.rows), r.height))
		}
		return r, nil
	case tea.WindowSizeMsg:
		r.height = m.Height
		r.tbl.SetHeight(tableHeight(len(r.rows), m.Height))
		return r, nil
	case tea.KeyMsg:
		if key.Matches(m, r.open) && len(r.rows) > 0 {
			cur := r.tbl.Cursor()
			if cur >= 0 && cur < len(r.rows) {
				row := r.rows[cur]
				// If a Frame is already on the stack (RunHistory was pushed
				// over it), pop the modal and ask the Frame to switch runs
				// in place. Otherwise (RunHistory pushed straight from
				// ScopeList), push a new Frame.
				return r, tea.Sequence(
					core.PopScreenCmd(),
					core.SwitchRunCmd(row),
				)
			}
		}
	}
	var cmd tea.Cmd
	r.tbl, cmd = r.tbl.Update(msg)
	return r, cmd
}

// View renders the table or load/empty state.
func (r *RunHistory) View() string {
	switch {
	case !r.loaded:
		return style.Dim.Render("loading runs…")
	case r.loadErr != nil:
		return lipgloss.NewStyle().Foreground(style.ColorError).
			Render("error loading runs: " + r.loadErr.Error())
	case len(r.rows) == 0:
		return style.Dim.Render("no runs found for this scope")
	default:
		return style.BorderActive.Render(r.tbl.View())
	}
}

func runRows(in []store.RunSummary) []table.Row {
	out := make([]table.Row, len(in))
	for i, run := range in {
		finished := ""
		if run.FinishedAt != nil {
			finished = run.FinishedAt.Local().Format(time.RFC3339)
		}
		// Status stays plain — bubbles/table v1's selected-row style strips
		// embedded ANSI. See note in scopes.go::toTableRows.
		out[i] = table.Row{
			short(run.UUID),
			run.StartedAt.Local().Format(time.RFC3339),
			finished,
			run.Status,
			truncate(run.Notes, 25),
		}
	}
	return out
}
