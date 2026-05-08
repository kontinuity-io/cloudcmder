package screens

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
		detail:   &inventory.VMDetail{MachineType: "e2-standard-2", Zone: "us-central1-a"},
		spin:     spinner.New(),
		loaded:   true,
		modeKey:  key.NewBinding(key.WithKeys("m")),
		graphKey: key.NewBinding(key.WithKeys("g")),
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
	assert.Contains(t, full, "DETAIL")
	assert.Contains(t, full, "CONNECTIONS")

	d.mode = DetailModeConnectionsOnly
	conn := d.View()
	assert.Contains(t, conn, "CONNECTIONS")
	assert.NotContains(t, conn, "DETAIL —")

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
	updated, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	d2 := updated.(*Detail)
	assert.Equal(t, DetailModeConnectionsOnly, d2.mode)
}
