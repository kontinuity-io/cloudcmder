package screens

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/style"
)

// rowData pairs a stored Resource with its kind-specific decoded Detail so the
// per-column Extract calls don't re-unmarshal JSON. matchedIndexes are byte
// offsets into rowCorpus(res) populated when the row arrived via fuzzy
// matching — used by highlightName to bold the matched runes inside the
// NAME cell. Empty when the row is shown without a filter active.
type rowData struct {
	res            inventory.Resource
	detail         any
	matchedIndexes []int
}

type resourcesLoadedMsg struct {
	rows []rowData
	err  error
}

type resourcesKeymap struct {
	Filter   key.Binding
	Sort     key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Down     key.Binding
	Up       key.Binding
	HalfDown key.Binding
	HalfUp   key.Binding
}

// sortState tracks the active column-sort ordering in a ResourceList.
// column < 0 means "no sort applied" (rows render in their natural order
// from the store, and fuzzy-matched rows render in score order).
type sortState struct {
	column int
	desc   bool
}

// ResourceList is a kind-parameterised list pane. As of M6.5 it is a LeftPane
// component owned by Frame; Frame draws the surrounding border and is the
// host for Enter (Frame zooms the right-pane Detail to full width).
type ResourceList struct {
	ctx        context.Context
	st         *store.Store
	run        store.RunSummary
	kind       inventory.Kind
	cols       []ColumnDef
	tbl        table.Model
	spin       spinner.Model
	rows       []rowData
	visible    []rowData
	loaded     bool
	loadErr    error
	height     int
	innerWidth int

	filterOn bool
	filterIn textinput.Model

	// pendingJumpID is non-empty when the cmdbar issued a JumpToResourceMsg
	// before our async load finished; applied as soon as resourcesLoadedMsg
	// arrives.
	pendingJumpID string

	sort sortState

	keymap resourcesKeymap
}

// NewResourceList returns a ResourceList for the given kind/run pair. Callers
// should have already verified `columnsFor(kind, …)` returned ok=true.
// Columns start at their natural widths; Frame calls SetInnerWidth before the
// first render so they can fit the actual pane.
func NewResourceList(ctx context.Context, st *store.Store, run store.RunSummary, kind inventory.Kind) *ResourceList {
	cols, _ := columnsFor(kind, 0)
	tCols := make([]table.Column, len(cols))
	for i, c := range cols {
		tCols[i] = table.Column{Title: c.Header, Width: c.Width}
	}
	tbl := table.New(
		table.WithColumns(tCols),
		table.WithFocused(true),
		table.WithHeight(15),
		table.WithStyles(selectedRowStyles()),
	)
	in := textinput.New()
	in.Prompt = "/"
	in.CharLimit = 64
	in.Width = 32

	s := spinner.New()
	s.Spinner = spinner.Dot

	return &ResourceList{
		ctx: ctx, st: st, run: run, kind: kind, cols: cols, tbl: tbl, spin: s,
		filterIn: in,
		sort:     sortState{column: -1},
		keymap: resourcesKeymap{
			Filter:   key.NewBinding(key.WithKeys("/")),
			Sort:     key.NewBinding(key.WithKeys("s")),
			Top:      key.NewBinding(key.WithKeys("g", "home")),
			Bottom:   key.NewBinding(key.WithKeys("G", "end")),
			Down:     key.NewBinding(key.WithKeys("j")),
			Up:       key.NewBinding(key.WithKeys("k")),
			HalfDown: key.NewBinding(key.WithKeys("ctrl+d")),
			HalfUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		},
	}
}

// SetInnerWidth recomputes column widths to fit the given pane budget,
// then re-renders the visible rows with the new widths. Called by Frame
// when it knows the actual leftW. No-op if width is unchanged.
func (s *ResourceList) SetInnerWidth(w int) {
	if s.innerWidth == w || w <= 0 {
		return
	}
	s.innerWidth = w
	cols, _ := columnsFor(s.kind, w)
	s.cols = cols
	tCols := make([]table.Column, len(cols))
	for i, c := range cols {
		tCols[i] = table.Column{Title: c.Header, Width: c.Width}
	}
	s.tbl.SetColumns(tCols)
	s.tbl.SetRows(s.toTableRows(s.visible))
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

// JumpTo positions the cursor on the row whose Resource.Ref.ID matches.
// No-op if the id is absent (e.g., user filtered it out before jumping).
func (s *ResourceList) JumpTo(id string) {
	for i, r := range s.visible {
		if r.res.Ref.ID == id {
			s.tbl.SetCursor(i)
			return
		}
	}
}

// QueueJump records a target ID to position the cursor on once the async
// load completes. Frame calls this immediately after constructing the
// pane via SwapLeftPaneMsg, so the jump fires atomically with the load
// rather than racing in via a separate message.
func (s *ResourceList) QueueJump(id string) {
	s.pendingJumpID = id
}

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
		if s.pendingJumpID != "" {
			s.JumpTo(s.pendingJumpID)
			s.pendingJumpID = ""
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
		switch {
		case key.Matches(k, s.keymap.Filter):
			s.filterOn = true
			s.filterIn.SetValue("")
			s.filterIn.Focus()
			return s, nil
		case key.Matches(k, s.keymap.Sort):
			s.cycleSort()
			return s, nil
		case key.Matches(k, s.keymap.Top):
			s.tbl.SetCursor(0)
			return s, nil
		case key.Matches(k, s.keymap.Bottom):
			if n := len(s.visible); n > 0 {
				s.tbl.SetCursor(n - 1)
			}
			return s, nil
		case key.Matches(k, s.keymap.Down):
			n := len(s.visible)
			if c := s.tbl.Cursor(); c < n-1 {
				s.tbl.SetCursor(c + 1)
			}
			return s, nil
		case key.Matches(k, s.keymap.Up):
			if c := s.tbl.Cursor(); c > 0 {
				s.tbl.SetCursor(c - 1)
			}
			return s, nil
		case key.Matches(k, s.keymap.HalfDown):
			s.tbl.SetCursor(clampCursor(s.tbl.Cursor()+s.halfPage(), len(s.visible)))
			return s, nil
		case key.Matches(k, s.keymap.HalfUp):
			s.tbl.SetCursor(clampCursor(s.tbl.Cursor()-s.halfPage(), len(s.visible)))
			return s, nil
		}
	}

	var cmd tea.Cmd
	s.tbl, cmd = s.tbl.Update(msg)
	return s, cmd
}

// halfPage returns the half-page jump distance for Ctrl+u/Ctrl+d. Uses the
// table's current viewport height when known, falling back to a reasonable
// default for unsized panes (e.g., immediately after construction).
func (s *ResourceList) halfPage() int {
	h := s.height
	if h <= 0 {
		h = 20
	}
	half := h / 2
	if half < 1 {
		half = 1
	}
	return half
}

func clampCursor(c, n int) int {
	if c < 0 {
		return 0
	}
	if c >= n {
		return n - 1
	}
	return c
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
// table. Empty pattern restores the natural order from the store, then
// applies any active sort. Non-empty pattern fuzzy-ranks rows by score —
// fuzzy ordering wins over column sort because the user is asking for
// "what matches", not "ordered by X".
func (s *ResourceList) applyFilter(pattern string) {
	if pattern == "" {
		// Copy so we don't reorder s.rows under sort.
		visible := make([]rowData, len(s.rows))
		copy(visible, s.rows)
		s.visible = visible
		if s.sort.column >= 0 {
			s.applySort()
		}
	} else {
		s.visible = s.matchRows(pattern)
	}
	s.tbl.SetRows(s.toTableRows(s.visible))
	if s.height > 0 {
		s.tbl.SetHeight(tableHeight(len(s.visible), s.height))
	}
}

// cycleSort advances the sort state through every column in both
// directions, wrapping back to "no sort" after the last cycle:
//
//   none → col0 asc → col0 desc → col1 asc → col1 desc → ... → none
//
// Re-runs applyFilter so the visible rows pick up the new ordering. Sort
// only takes effect when the filter is empty (fuzzy ranking wins
// otherwise) — but we still cycle the state so the next empty-filter
// view picks it up.
func (s *ResourceList) cycleSort() {
	switch {
	case s.sort.column < 0:
		s.sort = sortState{column: 0, desc: false}
	case !s.sort.desc:
		s.sort.desc = true
	case s.sort.column < len(s.cols)-1:
		s.sort = sortState{column: s.sort.column + 1, desc: false}
	default:
		s.sort = sortState{column: -1}
	}
	s.applyFilter(s.filterIn.Value())
}

// applySort reorders s.visible by the active column extractor. Comparison
// is lexical on the rendered cell text — adequate for name/region/zone
// columns; numeric columns (SIZE GB, vCPU) sort by string which puts
// "100" before "20"; that's a known v1.2 limitation, fixable later via
// per-column sort keys.
func (s *ResourceList) applySort() {
	if s.sort.column < 0 || s.sort.column >= len(s.cols) {
		return
	}
	extract := s.cols[s.sort.column].Extract
	sort.SliceStable(s.visible, func(i, j int) bool {
		a := extract(s.visible[i].res, s.visible[i].detail)
		b := extract(s.visible[j].res, s.visible[j].detail)
		if s.sort.desc {
			return a > b
		}
		return a < b
	})
}

// matchRows fuzzy-scores rows against pattern. The corpus per row is
// "name|region|status|label-vals" so a label or region typo still surfaces
// the resource. Output is ordered by descending fuzzy score; each entry
// carries the matched byte indices into the corpus so the renderer can
// highlight the matched runes inside the NAME cell.
func (s *ResourceList) matchRows(pattern string) []rowData {
	corpus := make([]string, len(s.rows))
	for i, r := range s.rows {
		corpus[i] = rowCorpus(r.res)
	}
	matches := fuzzy.Find(pattern, corpus)
	out := make([]rowData, len(matches))
	for i, m := range matches {
		rd := s.rows[m.Index]
		rd.matchedIndexes = m.MatchedIndexes
		out[i] = rd
	}
	return out
}

// rowCorpus serialises the searchable surface of a Resource for fuzzy
// matching. Pipe is unlikely to appear inside any of these fields and gives
// the scorer a soft separator that breaks bridging matches across columns.
func rowCorpus(r inventory.Resource) string {
	parts := make([]string, 0, 3+len(r.Labels))
	parts = append(parts, r.Name, r.Region, r.Status)
	for _, v := range r.Labels {
		parts = append(parts, v)
	}
	return strings.Join(parts, "|")
}

func (s *ResourceList) toTableRows(in []rowData) []table.Row {
	nameCol := -1
	for j, c := range s.cols {
		if strings.EqualFold(c.Header, "NAME") {
			nameCol = j
			break
		}
	}
	out := make([]table.Row, len(in))
	for i, rd := range in {
		row := make(table.Row, len(s.cols))
		for j, c := range s.cols {
			cell := c.Extract(rd.res, rd.detail)
			if j == nameCol && len(rd.matchedIndexes) > 0 {
				cell = highlightName(cell, rd.matchedIndexes)
			}
			row[j] = truncate(cell, c.Width)
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
