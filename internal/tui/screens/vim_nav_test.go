package screens

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"cloudcmder.com/internal/inventory"
)

// keyEvent constructs a tea.KeyPressMsg from a string the same way bubbles/key
// would interpret it. Single rune keys carry Code+Text; ctrl combos carry Mod.
func keyEvent(s string) tea.KeyPressMsg {
	switch s {
	case "ctrl+u":
		return tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'u'}
	case "ctrl+d":
		return tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'd'}
	}
	runes := []rune(s)
	if len(runes) == 1 {
		return tea.KeyPressMsg{Code: runes[0], Text: s}
	}
	return tea.KeyPressMsg{Text: s}
}

func newTestResourceList(visible []rowData) *ResourceList {
	rl := &ResourceList{
		kind:    inventory.KindVM,
		rows:    visible,
		visible: visible,
		cols:    []ColumnDef{{Header: "NAME", Width: 12, Extract: nameOf}},
		sort:    sortState{column: -1},
	}
	rl.keymap = resourcesKeymap{
		Filter:   key.NewBinding(key.WithKeys("/")),
		Sort:     key.NewBinding(key.WithKeys("s")),
		Top:      key.NewBinding(key.WithKeys("g", "home")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end")),
		Down:     key.NewBinding(key.WithKeys("j")),
		Up:       key.NewBinding(key.WithKeys("k")),
		HalfDown: key.NewBinding(key.WithKeys("ctrl+d")),
		HalfUp:   key.NewBinding(key.WithKeys("ctrl+u")),
	}
	rl.tbl = table.New(table.WithColumns([]table.Column{{Title: "NAME", Width: 12}}))
	rows := make([]table.Row, len(visible))
	for i, rd := range visible {
		rows[i] = table.Row{rd.res.Name}
	}
	rl.tbl.SetRows(rows)
	rl.height = 20
	return rl
}

func makeRows(names ...string) []rowData {
	out := make([]rowData, len(names))
	for i, n := range names {
		out[i] = rowData{res: inventory.Resource{Ref: inventory.ResourceRef{ID: n}, Name: n}}
	}
	return out
}

func TestVimNavGandG(t *testing.T) {
	rl := newTestResourceList(makeRows("a", "b", "c", "d", "e"))
	// G → bottom.
	updated, _ := rl.Update(keyEvent("G"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 4, rl.tbl.Cursor())
	// g → top.
	updated, _ = rl.Update(keyEvent("g"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 0, rl.tbl.Cursor())
}

func TestVimNavJandK(t *testing.T) {
	rl := newTestResourceList(makeRows("a", "b", "c"))
	// j → 1
	updated, _ := rl.Update(keyEvent("j"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 1, rl.tbl.Cursor())
	// j → 2
	updated, _ = rl.Update(keyEvent("j"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 2, rl.tbl.Cursor())
	// j at end → stays at 2 (no wrap).
	updated, _ = rl.Update(keyEvent("j"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 2, rl.tbl.Cursor())
	// k → 1
	updated, _ = rl.Update(keyEvent("k"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 1, rl.tbl.Cursor())
}

func TestVimNavHalfPage(t *testing.T) {
	// 30 rows, height=20 → halfPage=10.
	names := make([]string, 30)
	for i := range names {
		names[i] = string(rune('a'+i%26)) + "x"
	}
	rl := newTestResourceList(makeRows(names...))
	// Ctrl+d → cursor 0 + 10 = 10.
	updated, _ := rl.Update(keyEvent("ctrl+d"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 10, rl.tbl.Cursor())
	// Ctrl+d again → 20.
	updated, _ = rl.Update(keyEvent("ctrl+d"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 20, rl.tbl.Cursor())
	// Ctrl+u → 10.
	updated, _ = rl.Update(keyEvent("ctrl+u"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 10, rl.tbl.Cursor())
}

func TestSortCycleAdvancesThroughColumns(t *testing.T) {
	rl := newTestResourceList(makeRows("c-thing", "a-thing", "b-thing"))
	// Two columns so the cycle has somewhere to go.
	rl.cols = []ColumnDef{
		{Header: "NAME", Width: 12, Extract: nameOf},
		{Header: "REGION", Width: 12, Extract: func(r inventory.Resource, _ any) string { return r.Region }},
	}
	rl.tbl.SetColumns([]table.Column{{Title: "NAME", Width: 12}, {Title: "REGION", Width: 12}})

	// First `s` → col0 asc.
	updated, _ := rl.Update(keyEvent("s"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 0, rl.sort.column)
	assert.False(t, rl.sort.desc)
	assert.Equal(t, "a-thing", rl.visible[0].res.Name, "asc on NAME")
	assert.Equal(t, "c-thing", rl.visible[2].res.Name)

	// Second `s` → col0 desc.
	updated, _ = rl.Update(keyEvent("s"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 0, rl.sort.column)
	assert.True(t, rl.sort.desc)
	assert.Equal(t, "c-thing", rl.visible[0].res.Name, "desc on NAME")

	// Third `s` → col1 asc.
	updated, _ = rl.Update(keyEvent("s"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 1, rl.sort.column)
	assert.False(t, rl.sort.desc)

	// Fourth → col1 desc.
	updated, _ = rl.Update(keyEvent("s"))
	rl = updated.(*ResourceList)
	assert.Equal(t, 1, rl.sort.column)
	assert.True(t, rl.sort.desc)

	// Fifth → wraps back to "no sort".
	updated, _ = rl.Update(keyEvent("s"))
	rl = updated.(*ResourceList)
	assert.Equal(t, -1, rl.sort.column)
}
