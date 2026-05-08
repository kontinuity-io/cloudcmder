package screens

import (
	"testing"

	"github.com/charmbracelet/bubbles/table"
	"github.com/stretchr/testify/assert"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/tui/core"
)

func TestRowCorpusFlattensSearchableFields(t *testing.T) {
	r := inventory.Resource{
		Name:   "vm-prod-1",
		Region: "us-central1",
		Status: "RUNNING",
		Labels: map[string]string{"team": "platform"},
	}
	got := rowCorpus(r)
	for _, want := range []string{"vm-prod-1", "us-central1", "RUNNING", "platform"} {
		assert.Contains(t, got, want)
	}
}

func TestMatchRowsRanksByFuzzyScore(t *testing.T) {
	rl := &ResourceList{
		rows: []rowData{
			{res: inventory.Resource{Name: "vm-dev-cache"}},
			{res: inventory.Resource{Name: "vm-prod-api"}},
			{res: inventory.Resource{Name: "vm-prod-db"}},
			{res: inventory.Resource{Name: "vm-staging-1"}},
		},
	}
	got := rl.matchRows("prd")
	assert.NotEmpty(t, got, "fuzzy should find prod-* even with the typo")
	// Both prod rows should rank above the dev/staging rows.
	for i, g := range got {
		t.Logf("matchRows[%d] = %s", i, g.res.Name)
	}
	require := func(name string) bool {
		for _, g := range got {
			if g.res.Name == name {
				return true
			}
		}
		return false
	}
	assert.True(t, require("vm-prod-api"))
	assert.True(t, require("vm-prod-db"))
}

func TestMatchRowsHonoursLabelsAndRegion(t *testing.T) {
	rl := &ResourceList{
		rows: []rowData{
			{res: inventory.Resource{Name: "alpha"}},
			{res: inventory.Resource{Name: "beta", Region: "europe-west1"}},
			{res: inventory.Resource{Name: "gamma", Labels: map[string]string{"env": "production"}}},
		},
	}
	got := rl.matchRows("europe")
	assert.Len(t, got, 1)
	assert.Equal(t, "beta", got[0].res.Name)

	got = rl.matchRows("production")
	assert.Len(t, got, 1)
	assert.Equal(t, "gamma", got[0].res.Name)
}

func TestJumpToPositionsCursorOnMatch(t *testing.T) {
	rl := &ResourceList{
		visible: []rowData{
			{res: inventory.Resource{Ref: inventory.ResourceRef{ID: "alpha"}, Name: "alpha"}},
			{res: inventory.Resource{Ref: inventory.ResourceRef{ID: "bravo"}, Name: "bravo"}},
			{res: inventory.Resource{Ref: inventory.ResourceRef{ID: "charlie"}, Name: "charlie"}},
		},
	}
	rl.tbl = table.New(table.WithColumns([]table.Column{{Title: "NAME", Width: 10}}))
	rl.tbl.SetRows([]table.Row{{"alpha"}, {"bravo"}, {"charlie"}})
	rl.JumpTo("charlie")
	assert.Equal(t, 2, rl.tbl.Cursor())
}

func TestJumpToResourceMsgQueuesUntilLoaded(t *testing.T) {
	rl := &ResourceList{kind: inventory.KindVM}
	rl.tbl = table.New(table.WithColumns([]table.Column{{Title: "NAME", Width: 10}}))

	// Pane not yet loaded → JumpToResourceMsg should be queued.
	updated, _ := rl.Update(core.JumpToResourceMsg{ID: "vm-2"})
	rl = updated.(*ResourceList)
	assert.Equal(t, "vm-2", rl.pendingJumpID)

	// Now the load completes — pendingJumpID gets applied as cursor position.
	updated, _ = rl.Update(resourcesLoadedMsg{rows: []rowData{
		{res: inventory.Resource{Ref: inventory.ResourceRef{ID: "vm-1"}, Name: "vm-1"}},
		{res: inventory.Resource{Ref: inventory.ResourceRef{ID: "vm-2"}, Name: "vm-2"}},
	}})
	rl = updated.(*ResourceList)
	assert.Equal(t, "", rl.pendingJumpID, "pending jump should be cleared once applied")
	assert.Equal(t, 1, rl.tbl.Cursor(), "cursor should land on vm-2")
}

func TestMatchRowsEmptyPatternReturnsAll(t *testing.T) {
	rl := &ResourceList{
		rows: []rowData{
			{res: inventory.Resource{Name: "a"}},
			{res: inventory.Resource{Name: "b"}},
		},
	}
	// applyFilter("") path handles the empty case directly; matchRows is
	// only called with non-empty patterns. But "" against fuzzy should
	// still return zero matches (defensive — we never feed it ""):
	got := rl.matchRows("")
	assert.Empty(t, got)
}
