package screens

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
)

func newTestDetail() *Detail {
	return &Detail{
		ctx: context.Background(),
		run: store.RunSummary{ID: 1, UUID: "uuid-1234567890", ScopeID: "p"},
		res: inventory.Resource{
			Ref:    inventory.ResourceRef{Provider: "gcp", ScopeID: "p", Kind: inventory.KindVM, ID: "vm-a"},
			Kind:   inventory.KindVM,
			Name:   "vm-a",
			Region: "us-central1",
			Status: "RUNNING",
		},
		detail:     &inventory.VMDetail{MachineType: "e2-standard-2", Zone: "us-central1-a"},
		spin:       spinner.New(),
		loaded:     true,
		modeKey:    key.NewBinding(key.WithKeys("m")),
		prevTabKey: key.NewBinding(key.WithKeys("shift+left")),
		nextTabKey: key.NewBinding(key.WithKeys("shift+right")),
		jumpTabKey: key.NewBinding(key.WithKeys("1", "2", "3", "4")),
		graphKey:   key.NewBinding(key.WithKeys("g")),
	}
}

func TestCycleModeWrapsThroughAllModes(t *testing.T) {
	d := newTestDetail()
	require.Equal(t, DetailModeFull, d.mode)
	d.CycleMode()
	assert.Equal(t, DetailModeConnectionsOnly, d.mode)
	d.CycleMode()
	assert.Equal(t, DetailModeRawJSON, d.mode)
	d.CycleMode()
	assert.Equal(t, DetailModeInlineGraph, d.mode)
	d.CycleMode()
	assert.Equal(t, DetailModeFull, d.mode, "wraps back to Full")
}

func TestDetailViewSwitchesByMode(t *testing.T) {
	d := newTestDetail()

	d.mode = DetailModeFull
	full := d.View()
	assert.Contains(t, full, "Overview")    // active tab label
	assert.Contains(t, full, "Connections") // inactive tab label in strip

	d.mode = DetailModeConnectionsOnly
	d.contentDirty = true
	conn := d.View()
	assert.Contains(t, conn, "Connections") // active tab label
	assert.NotContains(t, conn, "DETAIL —") // header removed

	d.mode = DetailModeRawJSON
	raw := d.View()
	assert.Contains(t, raw, "RAW JSON")
	assert.True(t, strings.Contains(raw, "MachineType") || strings.Contains(raw, "(no"),
		"raw JSON should embed the detail struct or the no-data placeholder; got %q", raw)

	d.mode = DetailModeInlineGraph
	graph := d.View()
	// Graph header includes the resource name labelled as the focal node.
	assert.Contains(t, graph, "vm-a")
}

func TestDetailMKeyCyclesModeViaUpdate(t *testing.T) {
	d := newTestDetail()
	updated, _ := d.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	d2 := updated.(*Detail)
	assert.Equal(t, DetailModeConnectionsOnly, d2.mode)
}

// TestDetailViewportScrolls — Detail must forward navigation keystrokes
// (↓, PgDn, j, etc.) to its internal viewport so VM details with many
// disks/NICs are reachable when the pane is shorter than the content.
func TestDetailViewportScrolls(t *testing.T) {
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	run := store.RunSummary{ID: 1, UUID: "u", ScopeID: "p"}
	res := inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: "gcp", ScopeID: "p", Kind: inventory.KindVM, ID: "vm-tall"},
		Kind:   inventory.KindVM,
		Name:   "vm-tall",
		Status: "RUNNING",
	}
	d := NewDetail(context.Background(), st, run, res, nil)

	// Simulate the runtime: edges loaded so View() routes through the
	// viewport instead of the spinner branch.
	_, _ = d.Update(edgesLoadedMsg{edges: nil})
	// Tell Detail it has a 40-col × 5-row content area.
	_, _ = d.Update(tea.WindowSizeMsg{Width: 40, Height: 5})

	// Pump content larger than the viewport so scrolling has somewhere to
	// go. Direct SetContent keeps the test independent of which kind of
	// resource we built.
	d.vp.SetContent(strings.Repeat("filler line\n", 40))
	require.Equal(t, 0, d.vp.YOffset(), "starts at top")

	_, _ = d.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Greater(t, d.vp.YOffset(), 0, "down arrow must advance YOffset")
	before := d.vp.YOffset()

	_, _ = d.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	assert.Greater(t, d.vp.YOffset(), before, "pgdn must advance further")
}

// TestDetailShiftRightCyclesForward — shift+right advances mode the same as m.
func TestDetailShiftRightCyclesForward(t *testing.T) {
	d := newTestDetail()
	require.Equal(t, DetailModeFull, d.mode)
	updated, _ := d.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	assert.Equal(t, DetailModeConnectionsOnly, updated.(*Detail).mode)
}

// TestDetailShiftLeftCyclesBackward — shift+left decrements mode and wraps.
func TestDetailShiftLeftCyclesBackward(t *testing.T) {
	d := newTestDetail()
	require.Equal(t, DetailModeFull, d.mode)
	updated, _ := d.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})
	assert.Equal(t, DetailModeInlineGraph, updated.(*Detail).mode, "wraps from Full to InlineGraph")
}

// TestDetailNumberKeysJumpToTab — pressing 1–4 jumps directly to the
// matching tab without cycling through intermediate modes.
func TestDetailNumberKeysJumpToTab(t *testing.T) {
	tests := []struct {
		key  string
		want DetailMode
	}{
		{"1", DetailModeFull},
		{"2", DetailModeConnectionsOnly},
		{"3", DetailModeRawJSON},
		{"4", DetailModeInlineGraph},
	}
	for _, tt := range tests {
		t.Run("key_"+tt.key, func(t *testing.T) {
			d := newTestDetail()
			d.mode = DetailModeRawJSON // start somewhere other than Full
			updated, _ := d.Update(tea.KeyPressMsg{Code: rune(tt.key[0]), Text: tt.key})
			assert.Equal(t, tt.want, updated.(*Detail).mode)
		})
	}
}

// TestDetailModeCycleResetsScroll — pressing `m` to cycle modes must reset
// the scroll offset so the user sees the top of the new view instead of
// the scroll-position from the previous mode.
func TestDetailModeCycleResetsScroll(t *testing.T) {
	st, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	run := store.RunSummary{ID: 1, UUID: "u", ScopeID: "p"}
	res := inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: "gcp", ScopeID: "p", Kind: inventory.KindVM, ID: "vm"},
		Kind:   inventory.KindVM,
		Name:   "vm",
		Status: "RUNNING",
	}
	d := NewDetail(context.Background(), st, run, res, nil)
	_, _ = d.Update(edgesLoadedMsg{edges: nil})
	_, _ = d.Update(tea.WindowSizeMsg{Width: 40, Height: 5})

	d.vp.SetContent(strings.Repeat("x\n", 40))
	d.vp.SetYOffset(20)
	require.Equal(t, 20, d.vp.YOffset())

	_, _ = d.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	assert.Equal(t, 0, d.vp.YOffset(), "mode cycle must scroll back to top")
}
