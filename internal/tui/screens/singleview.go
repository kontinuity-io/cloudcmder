package screens

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
	"cloudcmder.com/internal/tui/style"
)

// singleViewMinWidth is the lower bound for rendering the 4-pane layout.
// Below it the layout becomes unreadable; SingleView prints a resize hint
// instead.
const singleViewMinWidth = 100

// SinglePaneFocus identifies which of the four panes has keyboard focus.
type SinglePaneFocus int

const (
	focusSVScopes SinglePaneFocus = iota
	focusSVOverview
	focusSVResources
	focusSVDetail
)

// SingleView is the alternative root screen, activated by `--single-view`.
// It renders a fixed 4-pane layout (Scopes | Overview / Resources | Detail)
// and cycles focus across all four panes via Tab / Shift+Tab. The existing
// screen-stack TUI is untouched — both modes coexist; the user picks one at
// app start.
type SingleView struct {
	ctx context.Context
	st  *store.Store

	scopes    *ScopeListPane
	overview  *Overview
	resources *ResourceList
	detail    *Detail

	currentRun       *store.RunSummary
	resourcesKindFor inventory.Kind
	detailFor        string

	// Cached for the statusbar; refreshed each time a scope is picked.
	totalResources int
	kindCount      int

	width, height int
	focus         SinglePaneFocus

	tabKey      key.Binding
	shiftTabKey key.Binding
	escKey      key.Binding
}

// NewSingleView returns a SingleView ready to be pushed as the App's root.
func NewSingleView(ctx context.Context, st *store.Store) *SingleView {
	return &SingleView{
		ctx:         ctx,
		st:          st,
		scopes:      NewScopeListPane(ctx, st),
		focus:       focusSVScopes,
		tabKey:      key.NewBinding(key.WithKeys("tab")),
		shiftTabKey: key.NewBinding(key.WithKeys("shift+tab")),
		escKey:      key.NewBinding(key.WithKeys("esc")),
	}
}

// Title satisfies core.Screen.
func (sv *SingleView) Title() string { return "single-view" }

// CurrentRun lets the App's :alias palette discover the active run.
func (sv *SingleView) CurrentRun() *store.RunSummary { return sv.currentRun }

// StatusbarData feeds the bottom status bar. Numbers come from
// CountResourcesByKind cached at scope-change time.
func (sv *SingleView) StatusbarData() core.RunStatusSnapshot {
	if sv.currentRun == nil {
		return core.RunStatusSnapshot{}
	}
	return core.RunStatusSnapshot{
		ScopeID:        sv.currentRun.ScopeID,
		RunUUIDShort:   short(sv.currentRun.UUID),
		RunStatus:      sv.currentRun.Status,
		TotalResources: sv.totalResources,
		KindCount:      sv.kindCount,
		StartedAt:      sv.currentRun.StartedAt,
	}
}

// Init kicks off the scopes pane's load. Auto-select of the first scope
// fires from syncFromSelections() once scopesLoadedMsg lands.
func (sv *SingleView) Init() tea.Cmd { return sv.scopes.Init() }

// Update routes messages and detects cascade-triggering cursor moves.
func (sv *SingleView) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		sv.width, sv.height = m.Width, m.Height
		return sv, sv.broadcastSize()
	case core.ScopeSelectedMsg:
		return sv.handleScopeSelected(m)
	case core.KindSelectedMsg:
		return sv.handleKindSelected(m)
	case core.ResourceSelectedMsg:
		return sv.handleResourceSelected(m)
	case core.SwapLeftPaneMsg:
		return sv.handleSwapLeftPane(m)
	}

	// Key dispatch — Tab/Shift+Tab/Esc are SingleView-level when the focused
	// pane is not absorbing keys. Anything else routes to the focused pane,
	// then we poll for cursor moves.
	if k, ok := msg.(tea.KeyMsg); ok {
		if !sv.activePaneAbsorbing() {
			switch {
			case key.Matches(k, sv.tabKey):
				sv.cycleFocus(true)
				return sv, nil
			case key.Matches(k, sv.shiftTabKey):
				sv.cycleFocus(false)
				return sv, nil
			case key.Matches(k, sv.escKey):
				return sv, core.PopScreenCmd()
			}
		}
		return sv, sv.dispatchKey(msg)
	}

	// Non-key (load completions, spinner ticks, etc.) → broadcast so panes
	// that have an outstanding async load receive the result regardless of
	// who currently holds focus.
	return sv, sv.broadcastNonKey(msg)
}

// handleScopeSelected replaces the Overview pane with one targeting the new
// run, refreshes the kind counts for the statusbar, and nulls out the
// downstream panes so a stale cursor can't bind to half-loaded state.
func (sv *SingleView) handleScopeSelected(m core.ScopeSelectedMsg) (core.Screen, tea.Cmd) {
	if m.Run == nil {
		sv.currentRun = nil
		sv.overview = nil
		sv.resources = nil
		sv.detail = nil
		sv.resourcesKindFor = ""
		sv.detailFor = ""
		sv.totalResources, sv.kindCount = 0, 0
		return sv, nil
	}
	sv.currentRun = m.Run
	if counts, err := sv.st.CountResourcesByKind(sv.ctx, m.Run.ID); err == nil {
		total := 0
		for _, n := range counts {
			total += n
		}
		sv.totalResources = total
		sv.kindCount = len(counts)
	} else {
		sv.totalResources, sv.kindCount = 0, 0
	}
	sv.overview = NewOverview(sv.ctx, sv.st, m.Run.ScopeID, m.Run.UUID)
	// Top-right pane is roughly half-height; the default chrome budget
	// (tuned for Frame's full-height left pane) leaves the kind table
	// starved here. 7 = chrome inside Overview (5 lines) + border (2).
	sv.overview.SetChromeBudget(7)
	sv.resources = nil
	sv.detail = nil
	sv.resourcesKindFor = ""
	sv.detailFor = ""
	return sv, sv.initPaneOverview()
}

// handleKindSelected replaces the ResourceList pane with one targeting the
// new kind, and nulls out the Detail pane.
func (sv *SingleView) handleKindSelected(m core.KindSelectedMsg) (core.Screen, tea.Cmd) {
	if sv.currentRun == nil {
		return sv, nil
	}
	sv.resourcesKindFor = m.Kind
	sv.resources = NewResourceList(sv.ctx, sv.st, *sv.currentRun, m.Kind)
	sv.detail = nil
	sv.detailFor = ""
	return sv, sv.initPaneResources()
}

// handleResourceSelected replaces the Detail pane with one targeting the new
// resource.
func (sv *SingleView) handleResourceSelected(m core.ResourceSelectedMsg) (core.Screen, tea.Cmd) {
	if sv.currentRun == nil {
		return sv, nil
	}
	sv.detailFor = m.Resource.Ref.String()
	sv.detail = NewDetail(sv.ctx, sv.st, *sv.currentRun, m.Resource, m.Detail)
	return sv, sv.initPaneDetail()
}

// handleSwapLeftPane reacts to a cmdbar `:alias` (or `:alias resource-name`)
// by rebuilding the ResourceList for the requested Kind under the current
// run and focusing it. Honours the JumpID by queueing it on the new
// ResourceList before Init so the cursor lands on the matching row.
func (sv *SingleView) handleSwapLeftPane(m core.SwapLeftPaneMsg) (core.Screen, tea.Cmd) {
	if sv.currentRun == nil {
		return sv, core.ToastCmd("no scope selected yet")
	}
	sv.resourcesKindFor = m.Kind
	rl := NewResourceList(sv.ctx, sv.st, *sv.currentRun, m.Kind)
	if m.JumpID != "" {
		rl.QueueJump(m.JumpID)
	}
	sv.resources = rl
	sv.detail = nil
	sv.detailFor = ""
	sv.focus = focusSVResources
	return sv, sv.initPaneResources()
}

// dispatchKey routes a keystroke to the currently focused pane and then
// polls for cursor-row changes that should fan out to downstream panes.
func (sv *SingleView) dispatchKey(msg tea.Msg) tea.Cmd {
	var paneCmd tea.Cmd
	switch sv.focus {
	case focusSVScopes:
		sv.scopes, paneCmd = sv.scopes.Update(msg)
	case focusSVOverview:
		if sv.overview != nil {
			updated, c := sv.overview.Update(msg)
			sv.overview = updated.(*Overview)
			paneCmd = c
		}
	case focusSVResources:
		if sv.resources != nil {
			updated, c := sv.resources.Update(msg)
			sv.resources = updated.(*ResourceList)
			paneCmd = c
		}
	case focusSVDetail:
		if sv.detail != nil {
			updated, c := sv.detail.Update(msg)
			sv.detail = updated.(*Detail)
			paneCmd = c
		}
	}
	cascade := sv.syncFromSelections()
	if paneCmd == nil {
		return cascade
	}
	if cascade == nil {
		return paneCmd
	}
	return tea.Sequence(paneCmd, cascade)
}

// broadcastNonKey forwards a non-key message to every non-nil pane.
// Used for spinner ticks and async load completions whose target pane is
// not necessarily focused. The scopes pane also drives auto-select via
// syncFromSelections() on the first scopesLoadedMsg.
func (sv *SingleView) broadcastNonKey(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	var c tea.Cmd
	sv.scopes, c = sv.scopes.Update(msg)
	if c != nil {
		cmds = append(cmds, c)
	}
	if sv.overview != nil {
		updated, oc := sv.overview.Update(msg)
		sv.overview = updated.(*Overview)
		if oc != nil {
			cmds = append(cmds, oc)
		}
	}
	if sv.resources != nil {
		updated, rc := sv.resources.Update(msg)
		sv.resources = updated.(*ResourceList)
		if rc != nil {
			cmds = append(cmds, rc)
		}
	}
	if sv.detail != nil {
		updated, dc := sv.detail.Update(msg)
		sv.detail = updated.(*Detail)
		if dc != nil {
			cmds = append(cmds, dc)
		}
	}
	if cascade := sv.syncFromSelections(); cascade != nil {
		cmds = append(cmds, cascade)
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// broadcastSize synthesises a WindowSizeMsg for each pane sized to its slot
// in the layout. Mirrors Frame's initLeftWithSize but for four panes.
func (sv *SingleView) broadcastSize() tea.Cmd {
	if sv.width == 0 || sv.height == 0 {
		return nil
	}
	dims := sv.computeDims()
	var cmds []tea.Cmd
	scopeMsg := tea.WindowSizeMsg{Width: dims.topL, Height: dims.topH}
	var c tea.Cmd
	sv.scopes, c = sv.scopes.Update(scopeMsg)
	if c != nil {
		cmds = append(cmds, c)
	}
	if sv.overview != nil {
		updated, oc := sv.overview.Update(tea.WindowSizeMsg{Width: dims.topR, Height: dims.topH})
		sv.overview = updated.(*Overview)
		if oc != nil {
			cmds = append(cmds, oc)
		}
	}
	if sv.resources != nil {
		updated, rc := sv.resources.Update(tea.WindowSizeMsg{Width: dims.botL, Height: dims.botH})
		sv.resources = updated.(*ResourceList)
		if rc != nil {
			cmds = append(cmds, rc)
		}
	}
	if sv.detail != nil {
		updated, dc := sv.detail.Update(tea.WindowSizeMsg{Width: dims.botR - 2, Height: dims.botH - 2})
		sv.detail = updated.(*Detail)
		if dc != nil {
			cmds = append(cmds, dc)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// initPaneOverview seeds the freshly-replaced Overview pane with size and
// fires its Init() load. Returned Cmd is sequenced (size before init) so
// the pane's first render sees a non-zero terminal.
func (sv *SingleView) initPaneOverview() tea.Cmd {
	if sv.overview == nil {
		return nil
	}
	initCmd := sv.overview.Init()
	if sv.width == 0 || sv.height == 0 {
		return initCmd
	}
	dims := sv.computeDims()
	updated, sizeCmd := sv.overview.Update(tea.WindowSizeMsg{Width: dims.topR, Height: dims.topH})
	sv.overview = updated.(*Overview)
	return tea.Batch(initCmd, sizeCmd)
}

func (sv *SingleView) initPaneResources() tea.Cmd {
	if sv.resources == nil {
		return nil
	}
	initCmd := sv.resources.Init()
	if sv.width == 0 || sv.height == 0 {
		return initCmd
	}
	dims := sv.computeDims()
	updated, sizeCmd := sv.resources.Update(tea.WindowSizeMsg{Width: dims.botL, Height: dims.botH})
	sv.resources = updated.(*ResourceList)
	return tea.Batch(initCmd, sizeCmd)
}

func (sv *SingleView) initPaneDetail() tea.Cmd {
	if sv.detail == nil {
		return nil
	}
	initCmd := sv.detail.Init()
	if sv.width == 0 || sv.height == 0 {
		return initCmd
	}
	dims := sv.computeDims()
	// Pass inner content dimensions (subtract border) so the viewport
	// inside Detail sizes to the actual rendering rect — viewport.View()
	// then fills the bordered box without lipgloss clipping its bottom
	// lines.
	updated, sizeCmd := sv.detail.Update(tea.WindowSizeMsg{Width: dims.botR - 2, Height: dims.botH - 2})
	sv.detail = updated.(*Detail)
	return tea.Batch(initCmd, sizeCmd)
}

// syncFromSelections polls each upstream pane for a cursor row change and
// emits the corresponding cascade message. Strict order via tea.Sequence so
// a Detail rebuild never races a half-built ResourceList.
func (sv *SingleView) syncFromSelections() tea.Cmd {
	var cmds []tea.Cmd
	if scope, moved := sv.scopes.SelectionMoved(); moved && scope != nil {
		cmds = append(cmds, sv.resolveScopeRunCmd(scope.ScopeID))
	}
	if sv.overview != nil {
		if k := sv.overview.SelectedKind(); k != nil && *k != sv.resourcesKindFor {
			cmds = append(cmds, core.KindSelectedCmd(*k))
		}
	}
	if sv.resources != nil {
		if r := sv.resources.SelectedResource(); r != nil {
			if ref := r.res.Ref.String(); ref != sv.detailFor {
				cmds = append(cmds, core.ResourceSelectedCmd(r.res, r.detail))
			}
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Sequence(cmds...)
}

// resolveScopeRunCmd returns a Cmd that fetches the latest RunSummary for
// the scope off the UI goroutine, then fires ScopeSelectedMsg.
func (sv *SingleView) resolveScopeRunCmd(scopeID string) tea.Cmd {
	return func() tea.Msg {
		run, _ := sv.st.LatestRunForScope(sv.ctx, scopeID)
		return core.ScopeSelectedMsg{ScopeID: scopeID, Run: run}
	}
}

// activePaneAbsorbing reports whether the currently focused pane is in an
// input mode that should consume Tab/Shift+Tab/Esc — currently true only
// for the ResourceList while its `/` filter is active.
func (sv *SingleView) activePaneAbsorbing() bool {
	switch sv.focus {
	case focusSVScopes:
		return sv.scopes.AbsorbingKeys()
	case focusSVOverview:
		return sv.overview != nil && sv.overview.AbsorbingKeys()
	case focusSVResources:
		return sv.resources != nil && sv.resources.AbsorbingKeys()
	case focusSVDetail:
		// Detail has no input field.
		return false
	}
	return false
}

// cycleFocus advances or reverses the focus across the four panes, skipping
// over panes that have not been built yet (overview / resources / detail
// stay nil until the upstream cascade has populated them).
func (sv *SingleView) cycleFocus(forward bool) {
	order := []SinglePaneFocus{focusSVScopes, focusSVOverview, focusSVResources, focusSVDetail}
	pos := 0
	for i, f := range order {
		if f == sv.focus {
			pos = i
			break
		}
	}
	step := 1
	if !forward {
		step = -1
	}
	for i := 0; i < len(order); i++ {
		pos = (pos + step + len(order)) % len(order)
		if sv.paneReady(order[pos]) {
			sv.focus = order[pos]
			return
		}
	}
}

func (sv *SingleView) paneReady(p SinglePaneFocus) bool {
	switch p {
	case focusSVScopes:
		return true
	case focusSVOverview:
		return sv.overview != nil
	case focusSVResources:
		return sv.resources != nil
	case focusSVDetail:
		return sv.detail != nil
	}
	return false
}

// --- layout ---

type svDims struct {
	topL, topR int // widths
	botL, botR int
	topH, botH int // heights
}

func (sv *SingleView) computeDims() svDims {
	bodyH := sv.height - 1 // 1 line footer hint
	if bodyH < 8 {
		bodyH = 8
	}
	topH := bodyH / 2
	botH := bodyH - topH
	inner := sv.width - 1 // 1-char vertical separator
	if inner < 20 {
		inner = 20
	}
	topL := inner * 25 / 100
	topR := inner - topL
	botL := inner * 70 / 100
	botR := inner - botL
	return svDims{topL: topL, topR: topR, botL: botL, botR: botR, topH: topH, botH: botH}
}

// View renders the 4-pane layout, or a resize hint when the terminal is
// too narrow.
func (sv *SingleView) View() string {
	if sv.width > 0 && sv.width < singleViewMinWidth {
		return style.Dim.Render(
			"terminal too narrow — resize to ≥100 cols to use --single-view",
		)
	}

	dims := sv.computeDims()

	topLeft := sv.borderFor(focusSVScopes).
		Width(dims.topL - 2).Height(dims.topH - 2).MaxHeight(dims.topH).
		Render(sv.scopes.View())

	topRight := sv.borderFor(focusSVOverview).
		Width(dims.topR - 2).Height(dims.topH - 2).MaxHeight(dims.topH).
		Render(sv.overviewView())

	botLeft := sv.borderFor(focusSVResources).
		Width(dims.botL - 2).Height(dims.botH - 2).MaxHeight(dims.botH).
		Render(sv.resourcesView())

	botRight := sv.borderFor(focusSVDetail).
		Width(dims.botR - 2).Height(dims.botH - 2).MaxHeight(dims.botH).
		Render(sv.detailView())

	top := lipgloss.JoinHorizontal(lipgloss.Top, topLeft, " ", topRight)
	bot := lipgloss.JoinHorizontal(lipgloss.Top, botLeft, " ", botRight)
	return strings.Join([]string{top, bot, sv.footerView()}, "\n")
}

func (sv *SingleView) overviewView() string {
	if sv.overview == nil {
		return style.Dim.Render("  select a scope to see\n  the kind summary")
	}
	sv.overview.SetInnerWidth(sv.computeDims().topR - 2)
	return sv.overview.View()
}

func (sv *SingleView) resourcesView() string {
	if sv.resources == nil {
		return style.Dim.Render("  pick a kind in the\n  summary to list its\n  resources here")
	}
	sv.resources.SetInnerWidth(sv.computeDims().botL - 2)
	return sv.resources.View()
}

func (sv *SingleView) detailView() string {
	if sv.detail == nil {
		return style.Dim.Render("  pick a resource to\n  see its details")
	}
	return sv.detail.View()
}

func (sv *SingleView) borderFor(p SinglePaneFocus) lipgloss.Style {
	if sv.focus == p {
		return style.BorderActive
	}
	return style.BorderInactive
}

func (sv *SingleView) footerView() string {
	hints := []string{
		"focus: " + focusName(sv.focus),
		"tab=cycle",
		"shift+tab=back",
		":=jump",
	}
	if sv.focus == focusSVDetail {
		hints = append(hints, "↑/↓/pgup/pgdn=scroll")
	}
	hints = append(hints, "q=quit", "esc=quit")
	return style.Dim.Render(strings.Join(hints, " · "))
}

func focusName(p SinglePaneFocus) string {
	switch p {
	case focusSVScopes:
		return "scopes"
	case focusSVOverview:
		return "summary"
	case focusSVResources:
		return "resources"
	case focusSVDetail:
		return "detail"
	}
	return "?"
}
