package screens

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/style"
)

// rowData pairs a stored Resource with its kind-specific decoded Detail so the
// per-column Extract calls don't re-unmarshal JSON.
type rowData struct {
	res    inventory.Resource
	detail any
}

type resourcesLoadedMsg struct {
	rows []rowData
	err  error
}

type resourcesKeymap struct {
	Filter key.Binding
}

// ResourceList is a kind-parameterised list pane. As of M6.5 it is a LeftPane
// component owned by Frame; Frame draws the surrounding border and is the
// host for Enter (Frame zooms the right-pane Detail to full width).
type ResourceList struct {
	ctx     context.Context
	st      *store.Store
	run     store.RunSummary
	kind    inventory.Kind
	cols    []ColumnDef
	tbl     table.Model
	spin    spinner.Model
	rows    []rowData
	visible []rowData
	loaded  bool
	loadErr error
	height  int

	filterOn   bool
	filterIn   textinput.Model
	regexCache map[string]*regexp.Regexp

	keymap resourcesKeymap
}

// NewResourceList returns a ResourceList for the given kind/run pair. Callers
// should have already verified `columnsFor(kind)` returned ok=true.
func NewResourceList(ctx context.Context, st *store.Store, run store.RunSummary, kind inventory.Kind) *ResourceList {
	cols, _ := columnsFor(kind)
	tCols := make([]table.Column, len(cols))
	for i, c := range cols {
		tCols[i] = table.Column{Title: c.Header, Width: c.Width}
	}
	tbl := table.New(
		table.WithColumns(tCols),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	in := textinput.New()
	in.Prompt = "/"
	in.CharLimit = 64
	in.Width = 32

	s := spinner.New()
	s.Spinner = spinner.Dot

	return &ResourceList{
		ctx: ctx, st: st, run: run, kind: kind, cols: cols, tbl: tbl, spin: s,
		filterIn:   in,
		regexCache: map[string]*regexp.Regexp{},
		keymap: resourcesKeymap{
			Filter: key.NewBinding(key.WithKeys("/")),
		},
	}
}

// Title satisfies LeftPane.
func (s *ResourceList) Title() string { return string(s.kind) }

// AbsorbingKeys reports true while the filter input is active so Frame stops
// eating Tab/Enter/Esc and lets the user type into the filter.
func (s *ResourceList) AbsorbingKeys() bool { return s.filterOn }

// SelectedResource returns the currently-highlighted row's resource+detail
// pair. Frame uses this to drive its right-pane Detail re-render.
func (s *ResourceList) SelectedResource() *rowData {
	if !s.loaded || len(s.visible) == 0 {
		return nil
	}
	cur := s.tbl.Cursor()
	if cur < 0 || cur >= len(s.visible) {
		return nil
	}
	return &s.visible[cur]
}

// SelectedKind is nil — ResourceList's selection is a Resource, not a Kind.
func (s *ResourceList) SelectedKind() *inventory.Kind { return nil }

// Init loads the kind-filtered resource set and kicks the spinner.
func (s *ResourceList) Init() tea.Cmd {
	load := func() tea.Msg {
		res, err := s.st.LoadResources(s.ctx, s.run.ID, s.kind)
		if err != nil {
			return resourcesLoadedMsg{err: err}
		}
		out := make([]rowData, 0, len(res))
		for _, r := range res {
			raw, _ := r.Detail.(json.RawMessage)
			out = append(out, rowData{res: r, detail: decodeDetail(s.kind, raw)})
		}
		return resourcesLoadedMsg{rows: out}
	}
	return tea.Batch(load, s.spin.Tick)
}

// Update routes messages either to the filter input (when open) or the table.
// Frame intercepts Enter at a higher level — ResourceList no longer pushes a
// Detail screen on its own.
func (s *ResourceList) Update(msg tea.Msg) (LeftPane, tea.Cmd) {
	switch m := msg.(type) {
	case resourcesLoadedMsg:
		s.loaded = true
		s.loadErr = m.err
		s.rows = m.rows
		s.applyFilter("")
		if s.height > 0 {
			s.tbl.SetHeight(tableHeight(len(s.visible), s.height))
		}
		return s, nil
	case tea.WindowSizeMsg:
		s.height = m.Height
		s.tbl.SetHeight(tableHeight(len(s.visible), m.Height))
		return s, nil
	case spinner.TickMsg:
		if !s.loaded {
			var cmd tea.Cmd
			s.spin, cmd = s.spin.Update(msg)
			return s, cmd
		}
		return s, nil
	}

	if s.filterOn {
		return s.updateFilter(msg)
	}

	if k, ok := msg.(tea.KeyMsg); ok {
		if key.Matches(k, s.keymap.Filter) {
			s.filterOn = true
			s.filterIn.SetValue("")
			s.filterIn.Focus()
			return s, nil
		}
	}

	var cmd tea.Cmd
	s.tbl, cmd = s.tbl.Update(msg)
	return s, cmd
}

func (s *ResourceList) updateFilter(msg tea.Msg) (LeftPane, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			s.filterOn = false
			s.filterIn.Blur()
			s.applyFilter("")
			return s, nil
		case "enter":
			s.filterOn = false
			s.filterIn.Blur()
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.filterIn, cmd = s.filterIn.Update(msg)
	s.applyFilter(s.filterIn.Value())
	return s, cmd
}

// applyFilter recomputes s.visible and pushes the matching rows into the
// table. Empty pattern clears the filter; invalid regex falls back to a
// case-insensitive substring match.
func (s *ResourceList) applyFilter(pattern string) {
	if pattern == "" {
		s.visible = s.rows
	} else {
		s.visible = s.matchRows(pattern)
	}
	s.tbl.SetRows(s.toTableRows(s.visible))
	if s.height > 0 {
		s.tbl.SetHeight(tableHeight(len(s.visible), s.height))
	}
}

func (s *ResourceList) matchRows(pattern string) []rowData {
	re := s.compileRegex(pattern)
	out := make([]rowData, 0, len(s.rows))
	for _, r := range s.rows {
		if re != nil {
			if re.MatchString(r.res.Name) {
				out = append(out, r)
			}
		} else {
			if strings.Contains(strings.ToLower(r.res.Name), strings.ToLower(pattern)) {
				out = append(out, r)
			}
		}
	}
	return out
}

func (s *ResourceList) compileRegex(pattern string) *regexp.Regexp {
	if re, ok := s.regexCache[pattern]; ok {
		return re
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		s.regexCache[pattern] = nil
		return nil
	}
	s.regexCache[pattern] = re
	return re
}

func (s *ResourceList) toTableRows(in []rowData) []table.Row {
	out := make([]table.Row, len(in))
	for i, rd := range in {
		row := make(table.Row, len(s.cols))
		for j, c := range s.cols {
			row[j] = truncate(c.Extract(rd.res, rd.detail), c.Width)
		}
		out[i] = row
	}
	return out
}

// View renders the table; appends the filter overlay when active. Frame draws
// the outer border around this content.
func (s *ResourceList) View() string {
	switch {
	case !s.loaded:
		return s.spin.View() + style.Dim.Render(" loading "+string(s.kind)+"s…")
	case s.loadErr != nil:
		return lipgloss.NewStyle().Foreground(style.ColorError).
			Render("error loading resources: " + s.loadErr.Error())
	case len(s.rows) == 0:
		return style.Dim.Render("no resources of kind " + string(s.kind) + " in this run")
	}

	body := s.tbl.View()
	if s.filterOn {
		body = lipgloss.JoinVertical(lipgloss.Left, body,
			style.Accent.Render(s.filterIn.View()))
	} else if pat := s.filterIn.Value(); pat != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body,
			style.Dim.Render("filter: "+pat+"  ("+matchCount(len(s.visible), len(s.rows))+")"))
	}
	return body
}

func matchCount(visible, total int) string {
	switch {
	case visible == total:
		return "all"
	default:
		return itoaCount(visible) + " of " + itoaCount(total)
	}
}

func itoaCount(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
