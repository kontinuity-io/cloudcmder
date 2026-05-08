package screens

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
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
// As of M6.5 it is a LeftPane component owned by Frame; Frame draws the
// surrounding border and is the host for Enter (Frame inspects SelectedKind
// and swaps the left pane to a ResourceList).
type Overview struct {
	ctx     context.Context
	st      *store.Store
	scopeID string
	runUUID string

	tbl    table.Model
	spin   spinner.Model
	width  int
	height int

	loaded  bool
	loadErr error
	run     *store.RunSummary
	rows    []KindCount
}

// NewOverview builds an Overview pane for the given run UUID.
func NewOverview(ctx context.Context, st *store.Store, scopeID, runUUID string) *Overview {
	tbl := table.New(
		table.WithColumns([]table.Column{
			{Title: "KIND", Width: 16},
			{Title: "COUNT", Width: 8},
		}),
		table.WithFocused(true),
		table.WithHeight(10),
		table.WithStyles(selectedRowStyles()),
	)
	s := spinner.New()
	s.Spinner = spinner.Dot
	return &Overview{
		ctx: ctx, st: st, scopeID: scopeID, runUUID: runUUID, tbl: tbl, spin: s,
	}
}

// Title satisfies LeftPane.
func (o *Overview) Title() string { return "Overview" }

// AbsorbingKeys reports false — Overview has no input field.
func (o *Overview) AbsorbingKeys() bool { return false }

// SetInnerWidth is a no-op for Overview: its KIND/COUNT columns are
// fixed-shape and small enough to fit any reasonable terminal. The pane
// width controls border drawing, not table layout.
func (o *Overview) SetInnerWidth(_ int) {}

// SelectedResource is always nil — Overview operates on Kinds, not resources.
func (o *Overview) SelectedResource() *rowData { return nil }

// SelectedKind returns the highlighted Kind, or nil if no rows.
func (o *Overview) SelectedKind() *inventory.Kind {
	if len(o.rows) == 0 {
		return nil
	}
	cur := o.tbl.Cursor()
	if cur < 0 || cur >= len(o.rows) {
		return nil
	}
	k := o.rows[cur].Kind
	return &k
}

// Init loads run metadata + counts in a single goroutine round trip and
// also kicks the spinner so the pane shows visible progress while loading.
func (o *Overview) Init() tea.Cmd {
	load := func() tea.Msg {
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
	return tea.Batch(load, o.spin.Tick)
}

// Update handles load completion, resize, and table cursor moves. Frame
// intercepts Enter — Overview no longer pushes a screen on its own.
func (o *Overview) Update(msg tea.Msg) (LeftPane, tea.Cmd) {
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
	case spinner.TickMsg:
		if !o.loaded {
			var cmd tea.Cmd
			o.spin, cmd = o.spin.Update(msg)
			return o, cmd
		}
		return o, nil
	}
	var cmd tea.Cmd
	o.tbl, cmd = o.tbl.Update(msg)
	return o, cmd
}

// View renders the count table plus run metadata header. Frame draws the
// outer border around this content.
func (o *Overview) View() string {
	switch {
	case !o.loaded:
		return o.spin.View() + style.Dim.Render(" loading overview…")
	case o.loadErr != nil:
		return lipgloss.NewStyle().Foreground(style.ColorError).
			Render("error loading overview: " + o.loadErr.Error())
	}

	header := o.headerView()
	sep := style.Separator(40)
	var body, total string
	if len(o.rows) == 0 {
		body = style.Dim.Render("  no resources captured yet for this run")
	} else {
		body = o.tbl.View()
		total = style.Dim.Render(
			fmt.Sprintf("Total: %d resources across %d kinds",
				totalResources(o.rows), len(o.rows)))
	}
	parts := []string{header, sep, body}
	if total != "" {
		parts = append(parts, sep, total)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (o *Overview) headerView() string {
	if o.run == nil {
		return ""
	}
	r := o.run
	finished := "—"
	if r.FinishedAt != nil {
		finished = r.FinishedAt.Local().Format("15:04:05")
	}
	line1 := lipgloss.JoinHorizontal(lipgloss.Top,
		style.Accent.Render("Run "+short(r.UUID)),
		style.Dim.Render(" · "),
		style.Status(r.Status).Render(r.Status),
	)
	line2 := style.Dim.Render(fmt.Sprintf(
		"started: %s   finished: %s",
		r.StartedAt.Local().Format(time.RFC3339), finished,
	))
	return lipgloss.JoinVertical(lipgloss.Left, line1, line2)
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
