package screens

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
	"cloudcmder.com/internal/tui/style"
)

// KindCount is one row in the Overview's count table.
type KindCount struct {
	Kind  inventory.Kind
	Count int
}

type overviewLoadedMsg struct {
	run    *store.RunSummary
	counts []KindCount
	err    error
}

// Overview shows run metadata and a per-Kind count breakdown for one run.
type Overview struct {
	ctx     context.Context
	st      *store.Store
	scopeID string
	runUUID string

	tbl     table.Model
	width   int
	height  int

	loaded  bool
	loadErr error
	run     *store.RunSummary
	rows    []KindCount
	open    key.Binding
}

// NewOverview builds an Overview screen for the given run UUID.
func NewOverview(ctx context.Context, st *store.Store, scopeID, runUUID string) *Overview {
	tbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "KIND", Width: 16},
			{Title: "COUNT", Width: 8},
		}),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	return &Overview{
		ctx: ctx, st: st, scopeID: scopeID, runUUID: runUUID, tbl: tbl,
		open: key.NewBinding(key.WithKeys("enter")),
	}
}

// Title satisfies core.Screen.
func (o *Overview) Title() string { return "Overview: " + o.scopeID }

// Init loads run metadata + counts in a single goroutine round trip.
func (o *Overview) Init() tea.Cmd {
	return func() tea.Msg {
		run, err := o.st.FindRunByUUID(o.ctx, o.runUUID)
		if err != nil {
			return overviewLoadedMsg{err: err}
		}
		if run == nil {
			return overviewLoadedMsg{err: fmt.Errorf("run %s not found in store", o.runUUID)}
		}
		counts, err := o.st.CountResourcesByKind(o.ctx, run.ID)
		if err != nil {
			return overviewLoadedMsg{run: run, err: err}
		}
		return overviewLoadedMsg{run: run, counts: sortKindCounts(counts)}
	}
}

// Update handles load completion, resize, and kind drill-down.
func (o *Overview) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case overviewLoadedMsg:
		o.loaded = true
		o.loadErr = m.err
		o.run = m.run
		o.rows = m.counts
		o.tbl.SetRows(kindCountRows(m.counts))
		return o, nil
	case tea.WindowSizeMsg:
		o.width = m.Width
		o.height = m.Height
		o.tbl.SetHeight(max(5, m.Height-12))
		return o, nil
	case tea.KeyMsg:
		if key.Matches(m, o.open) && len(o.rows) > 0 && o.run != nil {
			cur := o.tbl.Cursor()
			if cur >= 0 && cur < len(o.rows) {
				kind := o.rows[cur].Kind
				if _, ok := columnsFor(kind); ok {
					return o, core.PushScreenCmd(NewResourceList(o.ctx, o.st, *o.run, kind))
				}
				return o, core.PushScreenCmd(NewResourceListStub(kind, *o.run))
			}
		}
	}
	var cmd tea.Cmd
	o.tbl, cmd = o.tbl.Update(msg)
	return o, cmd
}

// View renders the run header, count table, and totals inside a single rounded border.
func (o *Overview) View() string {
	switch {
	case !o.loaded:
		return style.Dim.Render("loading overview…")
	case o.loadErr != nil:
		return lipgloss.NewStyle().Foreground(style.ColorError).
			Render("error loading overview: " + o.loadErr.Error())
	}

	innerWidth := o.contentWidth()
	header := o.headerView(innerWidth)

	var body, total string
	if len(o.rows) == 0 {
		body = style.Dim.Render("  no resources captured yet for this run")
		total = ""
	} else {
		body = o.tbl.View()
		total = style.Dim.Render(
			fmt.Sprintf("Total: %d resources across %d kinds",
				totalResources(o.rows), len(o.rows)))
	}

	sep := style.Separator(innerWidth)
	parts := []string{header, sep, body}
	if total != "" {
		parts = append(parts, sep, total)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return style.BorderActive.Width(innerWidth).Render(content)
}

func (o *Overview) headerView(innerWidth int) string {
	r := o.run
	statusBadge := style.Status(r.Status).Render(r.Status)
	line1 := lipgloss.JoinHorizontal(lipgloss.Top,
		style.Accent.Render("Run "+short(r.UUID)),
		style.Dim.Render(" · "),
		statusBadge,
	)
	line2 := style.Dim.Render("scope: " + r.ScopeID)
	finished := "—"
	if r.FinishedAt != nil {
		finished = r.FinishedAt.Local().Format("15:04:05")
	}
	line3 := style.Dim.Render(fmt.Sprintf(
		"started: %s   finished: %s",
		r.StartedAt.Local().Format(time.RFC3339), finished,
	))
	return lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3)
}

func (o *Overview) contentWidth() int {
	w := o.width
	if w <= 0 {
		w = 100
	}
	if w > 120 {
		w = 120
	}
	return w - 2 // leave room for the outer rounded border
}

// sortKindCounts converts the map returned by store.CountResourcesByKind into
// a slice ordered by descending count, with alphabetical tie-breaking. Pure
// function so the unit test doesn't need to spin up bubbletea.
func sortKindCounts(in map[inventory.Kind]int) []KindCount {
	out := make([]KindCount, 0, len(in))
	for k, c := range in {
		out = append(out, KindCount{Kind: k, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return string(out[i].Kind) < string(out[j].Kind)
	})
	return out
}

func kindCountRows(in []KindCount) []table.Row {
	out := make([]table.Row, len(in))
	for i, kc := range in {
		out[i] = table.Row{string(kc.Kind), fmt.Sprintf("%d", kc.Count)}
	}
	return out
}

func totalResources(rows []KindCount) int {
	t := 0
	for _, r := range rows {
		t += r.Count
	}
	return t
}
