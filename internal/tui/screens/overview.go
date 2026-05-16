package screens

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/style"
)

// KindCount is kept for backward compat with sortKindCounts (used in tests).
type KindCount struct {
	Kind  inventory.Kind
	Count int
}

type overviewLoadedMsg struct {
	run   *store.RunSummary
	stats []store.KindStats
	err   error
}

// Overview shows run metadata and a per-Kind health dashboard.
type Overview struct {
	ctx     context.Context
	st      *store.Store
	scopeID string
	runUUID string

	spin   spinner.Model
	width  int
	height int

	loaded  bool
	loadErr error
	run     *store.RunSummary
	stats   []store.KindStats
	cursor  int

	chromeBudget int
}

// NewOverview builds an Overview pane for the given run UUID.
func NewOverview(ctx context.Context, st *store.Store, scopeID, runUUID string) *Overview {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return &Overview{ctx: ctx, st: st, scopeID: scopeID, runUUID: runUUID, spin: s}
}

func (o *Overview) Title() string         { return "Overview" }
func (o *Overview) AbsorbingKeys() bool   { return false }
func (o *Overview) SetInnerWidth(_ int)   {}
func (o *Overview) SelectedResource() *rowData { return nil }

func (o *Overview) SetChromeBudget(n int) {
	o.chromeBudget = n
}

func (o *Overview) effectiveChrome() int {
	if o.chromeBudget > 0 {
		return o.chromeBudget
	}
	return 12
}

// SelectedKind returns the highlighted Kind, or nil if no rows.
func (o *Overview) SelectedKind() *inventory.Kind {
	if len(o.stats) == 0 {
		return nil
	}
	if o.cursor < 0 || o.cursor >= len(o.stats) {
		return nil
	}
	k := o.stats[o.cursor].Kind
	return &k
}

// Init loads run metadata + kind stats in one goroutine round trip.
func (o *Overview) Init() tea.Cmd {
	load := func() tea.Msg {
		run, err := o.st.FindRunByUUID(o.ctx, o.runUUID)
		if err != nil {
			return overviewLoadedMsg{err: err}
		}
		if run == nil {
			return overviewLoadedMsg{err: fmt.Errorf("run %s not found in store", o.runUUID)}
		}
		stats, err := o.st.KindStats(o.ctx, run.ID)
		if err != nil {
			return overviewLoadedMsg{run: run, err: err}
		}
		return overviewLoadedMsg{run: run, stats: stats}
	}
	return tea.Batch(load, o.spin.Tick)
}

// Update handles load completion, resize, and cursor navigation.
func (o *Overview) Update(msg tea.Msg) (LeftPane, tea.Cmd) {
	switch m := msg.(type) {
	case overviewLoadedMsg:
		o.loaded = true
		o.loadErr = m.err
		o.run = m.run
		o.stats = m.stats
		o.cursor = 0
		return o, nil
	case tea.WindowSizeMsg:
		o.width = m.Width
		o.height = m.Height
		return o, nil
	case spinner.TickMsg:
		if !o.loaded {
			var cmd tea.Cmd
			o.spin, cmd = o.spin.Update(msg)
			return o, cmd
		}
		return o, nil
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		n := len(o.stats)
		switch k.String() {
		case "g", "home":
			o.cursor = 0
		case "G", "end":
			if n > 0 {
				o.cursor = n - 1
			}
		case "j", "down":
			if o.cursor < n-1 {
				o.cursor++
			}
		case "k", "up":
			if o.cursor > 0 {
				o.cursor--
			}
		}
	}
	return o, nil
}

// View renders the health dashboard + run metadata header.
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

	var body string
	if len(o.stats) == 0 {
		body = style.Dim.Render("  no resources captured yet for this run")
	} else {
		body = o.renderTable()
	}

	parts := []string{header, sep, body}
	if len(o.stats) > 0 {
		total := 0
		for _, ks := range o.stats {
			total += ks.Total
		}
		parts = append(parts, sep,
			style.Dim.Render(fmt.Sprintf("Total: %d resources across %d kinds", total, len(o.stats))))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (o *Overview) renderTable() string {
	maxCount := 0
	for _, ks := range o.stats {
		if ks.Total > maxCount {
			maxCount = ks.Total
		}
	}

	// Decide which columns to show based on available width.
	narrow := o.width > 0 && o.width < 90

	// Header.
	hdr := style.Dim.Render(o.renderHeader(narrow))
	lines := make([]string, 0, 1+len(o.stats))
	lines = append(lines, hdr)

	for i, ks := range o.stats {
		row := o.renderRow(ks, maxCount, i == o.cursor, narrow)
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

func (o *Overview) renderHeader(narrow bool) string {
	if narrow {
		return fmt.Sprintf("%-20s %6s  %-5s  %s", "KIND", "COUNT", "HLTH", "STATUS")
	}
	return fmt.Sprintf("%-20s %6s  %-16s  %-5s  %s", "KIND", "COUNT", "USAGE", "HLTH", "STATUS")
}

func (o *Overview) renderRow(ks store.KindStats, maxCount int, selected, narrow bool) string {
	kindCell := fmt.Sprintf("%-20s", string(ks.Kind))
	countCell := fmt.Sprintf("%6d", ks.Total)
	dotsCell := renderHealthDots(ks)
	badgeCell := renderStatusBadge(ks)

	var row string
	if narrow {
		row = kindCell + " " + countCell + "  " + dotsCell + "  " + badgeCell
	} else {
		barCell := renderCountBar(ks.Total, maxCount, 16)
		row = kindCell + " " + countCell + "  " + barCell + "  " + dotsCell + "  " + badgeCell
	}
	if selected {
		row = lipgloss.NewStyle().Background(style.ColorSelectedBg).Render(row)
	} else {
		row = style.Dim.Render(row)
	}
	return row
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

// renderCountBar renders a ████░░░░ progress bar of width w.
func renderCountBar(count, maxCount, w int) string {
	if w <= 0 {
		return ""
	}
	filled := 0
	if maxCount > 0 {
		filled = int(math.Round(float64(count) / float64(maxCount) * float64(w)))
	}
	if filled > w {
		filled = w
	}
	return style.Accent.Render(strings.Repeat("█", filled)) +
		style.Dim.Render(strings.Repeat("░", w-filled))
}

// renderHealthDots renders 5 dots (●/○) colored by health ratio.
func renderHealthDots(ks store.KindStats) string {
	if ks.Total == 0 {
		return style.Dim.Render("○○○○○")
	}
	filled := int(math.Round(float64(ks.Healthy) / float64(ks.Total) * 5))
	var dotStyle lipgloss.Style
	switch {
	case ks.Critical > 0:
		dotStyle = lipgloss.NewStyle().Foreground(style.ColorError)
	case ks.Warning > 0:
		dotStyle = lipgloss.NewStyle().Foreground(style.ColorWarning)
	default:
		dotStyle = lipgloss.NewStyle().Foreground(style.ColorHealthy)
	}
	return dotStyle.Render(strings.Repeat("●", filled)) +
		style.Dim.Render(strings.Repeat("○", 5-filled))
}

// renderStatusBadge renders [OK], [WARN: N], or [CRIT: N].
func renderStatusBadge(ks store.KindStats) string {
	switch {
	case ks.Critical > 0:
		return lipgloss.NewStyle().Foreground(style.ColorError).
			Render(fmt.Sprintf("[CRIT: %d]", ks.Critical))
	case ks.Warning > 0:
		return lipgloss.NewStyle().Foreground(style.ColorWarning).
			Render(fmt.Sprintf("[WARN: %d]", ks.Warning))
	default:
		return lipgloss.NewStyle().Foreground(style.ColorHealthy).Render("[OK]")
	}
}

// sortKindCounts converts the map returned by store.CountResourcesByKind into
// a slice ordered by descending count, with alphabetical tie-breaking. Pure
// function so the overview unit test can stay cheap.
func sortKindCounts(in map[inventory.Kind]int) []KindCount {
	out := make([]KindCount, 0, len(in))
	for k, c := range in {
		out = append(out, KindCount{Kind: k, Count: c})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].Count > out[j-1].Count ||
			(out[j].Count == out[j-1].Count && string(out[j].Kind) < string(out[j-1].Kind))); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
