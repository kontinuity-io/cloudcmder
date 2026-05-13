package screens

import (
	"context"
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
)

// reflectSliceOfCmds returns msg as []tea.Cmd if its underlying type is
// such — covers tea.sequenceMsg (unexported, defined as []Cmd) and any
// custom alias. Returns nil for anything else.
func reflectSliceOfCmds(msg tea.Msg) []tea.Cmd {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice {
		return nil
	}
	if v.Type().Elem().Kind() != reflect.Func {
		return nil
	}
	out := make([]tea.Cmd, v.Len())
	for i := 0; i < v.Len(); i++ {
		f, ok := v.Index(i).Interface().(tea.Cmd)
		if !ok {
			return nil
		}
		out[i] = f
	}
	return out
}

// drainCmd evaluates a Cmd, routes whatever it produces back through
// SingleView.Update, and recursively drains any follow-up Cmds. Handles
// tea.BatchMsg (slice of Cmds) and tea.Sequence's unexported sequenceMsg
// (also a []Cmd under the hood, accessed via reflection).
//
// This is a minimal stand-in for the bubbletea runtime — adequate to drive
// the SingleView cascade in tests without spinning up a tea.Program. Each
// Cmd is fully drained before the next is processed; the real runtime
// would fan tea.Batch children out concurrently, but for our cascade the
// final state is identical either way.
func drainCmd(t *testing.T, sv *SingleView, cmd tea.Cmd, remaining int) {
	t.Helper()
	if cmd == nil || remaining <= 0 {
		return
	}
	msg := cmd()
	if msg == nil {
		return
	}
	// Batch — drain each child completely.
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			drainCmd(t, sv, c, remaining-1)
		}
		return
	}
	// Sequence — sequenceMsg is unexported but is []Cmd under the hood.
	if rv := reflectSliceOfCmds(msg); rv != nil {
		for _, c := range rv {
			drainCmd(t, sv, c, remaining-1)
		}
		return
	}
	updated, next := sv.Update(msg)
	sv2, ok := updated.(*SingleView)
	require.True(t, ok)
	*sv = *sv2
	drainCmd(t, sv, next, remaining-1)
}

func drainSV(t *testing.T, sv *SingleView, cmd tea.Cmd) {
	t.Helper()
	drainCmd(t, sv, cmd, 256)
}

// seedRun creates a finished run for scopeID containing the supplied
// resources and returns its RunSummary. Uses the live store so the same
// SQL paths the TUI calls in production are exercised in tests.
func seedRun(t *testing.T, st *store.Store, scopeID string, resources []inventory.Resource) store.RunSummary {
	t.Helper()
	ctx := context.Background()
	runID, runUUID, err := st.OpenRun(ctx, "gcp", scopeID, scopeID, "test")
	require.NoError(t, err)
	if len(resources) > 0 {
		require.NoError(t, st.WriteBatch(ctx, runID, resources))
	}
	require.NoError(t, st.FinishRun(ctx, runID, "ok", ""))
	run, err := st.FindRunByUUID(ctx, runUUID)
	require.NoError(t, err)
	require.NotNil(t, run)
	return *run
}

func sampleVM(scopeID, name string) inventory.Resource {
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: "gcp", ScopeID: scopeID, Kind: inventory.KindVM, ID: name},
		Kind:   inventory.KindVM,
		Name:   name,
		Region: "us-central1",
		Status: "RUNNING",
	}
}

func sampleBucket(scopeID, name string) inventory.Resource {
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: "gcp", ScopeID: scopeID, Kind: inventory.KindBucket, ID: name},
		Kind:   inventory.KindBucket,
		Name:   name,
		Region: "US",
		Status: "ACTIVE",
	}
}

// TestSingleViewFocusCycle pins the Tab / Shift+Tab cycle order. Panes
// downstream of Scopes are pre-populated so the cycle reaches all four.
func TestSingleViewFocusCycle(t *testing.T) {
	st := openMemStoreT(t)
	sv := NewSingleView(context.Background(), st)

	// Force every pane to "ready" without going through the cascade so the
	// cycle test isolates focus logic from async loads.
	sv.overview = NewOverview(context.Background(), st, "proj", "uuid")
	sv.resources = NewResourceList(context.Background(), st, store.RunSummary{}, inventory.KindVM)
	sv.detail = NewDetail(context.Background(), st, store.RunSummary{}, inventory.Resource{}, nil)

	cases := []struct {
		start  SinglePaneFocus
		key    tea.KeyType
		expect SinglePaneFocus
	}{
		{focusSVScopes, tea.KeyTab, focusSVOverview},
		{focusSVOverview, tea.KeyTab, focusSVResources},
		{focusSVResources, tea.KeyTab, focusSVDetail},
		{focusSVDetail, tea.KeyTab, focusSVScopes},
		{focusSVScopes, tea.KeyShiftTab, focusSVDetail},
		{focusSVDetail, tea.KeyShiftTab, focusSVResources},
		{focusSVResources, tea.KeyShiftTab, focusSVOverview},
		{focusSVOverview, tea.KeyShiftTab, focusSVScopes},
	}
	for _, tc := range cases {
		sv.focus = tc.start
		updated, _ := sv.Update(tea.KeyMsg{Type: tc.key})
		got := updated.(*SingleView)
		assert.Equal(t, tc.expect, got.focus,
			"start=%v key=%v want=%v", tc.start, tc.key, tc.expect)
	}
}

// TestSingleViewFocusCycleSkipsNilPanes verifies the cycle skips panes that
// haven't been built yet (e.g., before the first scope is selected).
func TestSingleViewFocusCycleSkipsNilPanes(t *testing.T) {
	st := openMemStoreT(t)
	sv := NewSingleView(context.Background(), st)
	// Only the scopes pane exists — overview/resources/detail are nil.
	sv.focus = focusSVScopes
	updated, _ := sv.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := updated.(*SingleView)
	// All downstream panes are nil, so cycling forward returns to scopes.
	assert.Equal(t, focusSVScopes, got.focus)
}

// TestSingleViewEscQuits — Esc at SingleView (the only screen on the stack)
// must emit PopScreenMsg so App's "len(stack)<=1 → tea.Quit" branch fires.
func TestSingleViewEscQuits(t *testing.T) {
	st := openMemStoreT(t)
	sv := NewSingleView(context.Background(), st)
	_, cmd := sv.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)
	_, ok := cmd().(core.PopScreenMsg)
	assert.True(t, ok, "Esc must emit PopScreenMsg")
}

// TestSingleViewNarrowTerminalHint — width below the threshold must render
// the resize hint and skip the 4-pane layout.
func TestSingleViewNarrowTerminalHint(t *testing.T) {
	st := openMemStoreT(t)
	sv := NewSingleView(context.Background(), st)
	sv.width = 80
	sv.height = 24
	got := sv.View()
	assert.Contains(t, got, "too narrow")
}

// TestSingleViewScopeSelectedBuildsOverview — handling ScopeSelectedMsg must
// (a) cache the run, (b) refresh kind counts, (c) construct an Overview,
// (d) null out downstream panes.
func TestSingleViewScopeSelectedBuildsOverview(t *testing.T) {
	st := openMemStoreT(t)
	run := seedRun(t, st, "proj-a", []inventory.Resource{sampleVM("proj-a", "vm-a"), sampleBucket("proj-a", "b-a")})
	sv := NewSingleView(context.Background(), st)
	sv.width, sv.height = 200, 40

	// Pre-poison downstream panes; the handler must null them out.
	sv.resources = NewResourceList(context.Background(), st, run, inventory.KindVM)
	sv.detail = NewDetail(context.Background(), st, run, sampleVM("proj-a", "vm-a"), nil)
	sv.resourcesKindFor = inventory.KindVM
	sv.detailFor = "stale"

	_, _ = sv.Update(core.ScopeSelectedMsg{ScopeID: run.ScopeID, Run: &run})

	require.NotNil(t, sv.currentRun)
	assert.Equal(t, run.UUID, sv.currentRun.UUID)
	require.NotNil(t, sv.overview, "overview must be built")
	assert.Nil(t, sv.resources, "downstream resources pane must be nulled")
	assert.Nil(t, sv.detail, "downstream detail pane must be nulled")
	assert.Equal(t, inventory.Kind(""), sv.resourcesKindFor)
	assert.Equal(t, "", sv.detailFor)
	assert.Equal(t, 2, sv.totalResources)
	assert.Equal(t, 2, sv.kindCount)
}

// TestSingleViewScopeSelectedNilRunClears — when the scope has no recorded
// runs, ScopeSelectedMsg{Run: nil} must clear all downstream state.
func TestSingleViewScopeSelectedNilRunClears(t *testing.T) {
	st := openMemStoreT(t)
	sv := NewSingleView(context.Background(), st)
	sv.currentRun = &store.RunSummary{UUID: "old"}
	sv.overview = NewOverview(context.Background(), st, "p", "uuid")
	sv.totalResources, sv.kindCount = 5, 3

	_, _ = sv.Update(core.ScopeSelectedMsg{ScopeID: "empty", Run: nil})

	assert.Nil(t, sv.currentRun)
	assert.Nil(t, sv.overview)
	assert.Equal(t, 0, sv.totalResources)
	assert.Equal(t, 0, sv.kindCount)
}

// TestSingleViewKindSelectedBuildsResources — handling KindSelectedMsg
// builds a ResourceList for the new kind and nulls Detail.
func TestSingleViewKindSelectedBuildsResources(t *testing.T) {
	st := openMemStoreT(t)
	run := seedRun(t, st, "p", []inventory.Resource{sampleVM("p", "vm-x")})
	sv := NewSingleView(context.Background(), st)
	sv.width, sv.height = 200, 40
	sv.currentRun = &run
	sv.detail = NewDetail(context.Background(), st, run, sampleVM("p", "vm-x"), nil)
	sv.detailFor = "old"

	_, _ = sv.Update(core.KindSelectedMsg{Kind: inventory.KindBucket})

	require.NotNil(t, sv.resources)
	assert.Equal(t, inventory.KindBucket, sv.resourcesKindFor)
	assert.Nil(t, sv.detail)
	assert.Equal(t, "", sv.detailFor)
}

// TestSingleViewKindSelectedNoCurrentRunIsNoOp — guard against firing
// before a scope has resolved.
func TestSingleViewKindSelectedNoCurrentRunIsNoOp(t *testing.T) {
	st := openMemStoreT(t)
	sv := NewSingleView(context.Background(), st)
	_, _ = sv.Update(core.KindSelectedMsg{Kind: inventory.KindVM})
	assert.Nil(t, sv.resources)
	assert.Equal(t, inventory.Kind(""), sv.resourcesKindFor)
}

// TestSingleViewResourceSelectedBuildsDetail — handling ResourceSelectedMsg
// builds a Detail for the new resource.
func TestSingleViewResourceSelectedBuildsDetail(t *testing.T) {
	st := openMemStoreT(t)
	run := seedRun(t, st, "p", []inventory.Resource{sampleVM("p", "vm-y")})
	sv := NewSingleView(context.Background(), st)
	sv.width, sv.height = 200, 40
	sv.currentRun = &run

	vm := sampleVM("p", "vm-y")
	_, _ = sv.Update(core.ResourceSelectedMsg{Resource: vm, Detail: nil})

	require.NotNil(t, sv.detail)
	assert.Equal(t, vm.Ref.String(), sv.detailFor)
}

// TestSingleViewSwapLeftPaneBuildsResourcesAndFocuses — `:vm` from the
// cmdbar must rebuild the resources pane for KindVM, queue the JumpID,
// and shift focus to the resources pane.
func TestSingleViewSwapLeftPaneBuildsResourcesAndFocuses(t *testing.T) {
	st := openMemStoreT(t)
	run := seedRun(t, st, "p", []inventory.Resource{sampleVM("p", "vm-q")})
	sv := NewSingleView(context.Background(), st)
	sv.width, sv.height = 200, 40
	sv.currentRun = &run
	sv.focus = focusSVScopes

	_, _ = sv.Update(core.SwapLeftPaneMsg{Kind: inventory.KindVM, JumpID: "vm-q"})

	require.NotNil(t, sv.resources)
	assert.Equal(t, inventory.KindVM, sv.resourcesKindFor)
	assert.Equal(t, focusSVResources, sv.focus)
}

// TestSingleViewSwapLeftPaneNoRunToasts — `:vm` before any scope resolves
// must surface a toast (no nil-deref crash).
func TestSingleViewSwapLeftPaneNoRunToasts(t *testing.T) {
	st := openMemStoreT(t)
	sv := NewSingleView(context.Background(), st)
	_, cmd := sv.Update(core.SwapLeftPaneMsg{Kind: inventory.KindVM})
	require.NotNil(t, cmd)
	msg := cmd()
	_, isToast := msg.(core.ToastMsg)
	assert.True(t, isToast, "must produce a ToastMsg when no run is active, got %T", msg)
}

// TestSingleViewCascadeEndToEnd — feeds the SingleView through the
// scopes-load → auto-select → overview-load → kind-select → resources-load
// → resource-select chain using the real store, asserting each pane is
// built with the right backing state.
func TestSingleViewCascadeEndToEnd(t *testing.T) {
	st := openMemStoreT(t)
	run := seedRun(t, st, "proj-a", []inventory.Resource{
		sampleVM("proj-a", "vm-a"),
		sampleBucket("proj-a", "b-a"),
	})

	sv := NewSingleView(context.Background(), st)
	updated, sizeCmd := sv.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	sv = updated.(*SingleView)
	drainSV(t, sv, sizeCmd)

	// Drive the scopes load.
	drainSV(t, sv, sv.Init())

	// Auto-select fires from the cursor-move poll: after rows land, the
	// first cursor (0) differs from the -1 sentinel, so SelectionMoved
	// returns true → ScopeSelectedMsg → Overview built.
	require.NotNil(t, sv.currentRun, "currentRun must resolve via auto-select")
	assert.Equal(t, run.UUID, sv.currentRun.UUID)
	require.NotNil(t, sv.overview, "overview must be built by auto-select")

	// Simulate the user pressing 'j' on the overview cursor to land on a
	// Kind row. Since the count table is sorted descending and there are
	// two singletons, alphabetical tie-break makes Bucket first.
	sv.focus = focusSVOverview
	// Run the overview's load to populate its rows.
	drainSV(t, sv, sv.overview.Init())
	require.NotNil(t, sv.overview.SelectedKind(), "overview must have a kind to select")

	// One key tick fans out to the focused pane and then runs the
	// selection poll; that emits KindSelectedMsg.
	updated, cmd := sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	sv = updated.(*SingleView)
	drainSV(t, sv, cmd)
	require.NotNil(t, sv.resources, "resources pane must be built after kind selection")

	// Pump the resources load.
	drainSV(t, sv, sv.resources.Init())
	require.NotNil(t, sv.resources.SelectedResource(), "resources must have a row to select")

	// One more focus-and-tick to land in resources and fire the
	// resource-selected cascade.
	sv.focus = focusSVResources
	updated, cmd = sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	sv = updated.(*SingleView)
	drainSV(t, sv, cmd)
	require.NotNil(t, sv.detail, "detail pane must be built after resource selection")
}

// TestScopeListPaneSelectionMovedSentinel — first call after load returns
// moved=true so SingleView's auto-select fires; subsequent calls without
// cursor movement return moved=false.
func TestScopeListPaneSelectionMovedSentinel(t *testing.T) {
	st := openMemStoreT(t)
	seedRun(t, st, "p1", nil)
	pane := NewScopeListPane(context.Background(), st)

	// Pump the load.
	cmd := pane.loadCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	pane, _ = pane.Update(msg)

	require.True(t, pane.loaded)
	require.Len(t, pane.rows, 1)

	scope, moved := pane.SelectionMoved()
	require.True(t, moved, "first call after load must fire (cursor 0 != -1 sentinel)")
	require.NotNil(t, scope)
	assert.Equal(t, "p1", scope.ScopeID)

	scope, moved = pane.SelectionMoved()
	assert.False(t, moved, "second call without cursor change must return moved=false")
	assert.Nil(t, scope)
}

// silence unused-import warnings when running this file in isolation.
var _ = time.Time{}
