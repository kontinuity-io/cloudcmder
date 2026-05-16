package screens

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"cloudcmder.com/internal/inventory"
)

func TestHighlightNameWrapsMatchedRunes(t *testing.T) {
	// lipgloss v2 always emits ANSI — no profile forcing needed.
	out := highlightName("vm-prod-1", []int{3, 4, 5, 6})
	// "prod" should be wrapped in ANSI codes; "vm-" and "-1" plain.
	assert.Contains(t, out, "vm-")
	assert.Contains(t, out, "-1")
	// Bold (1) and underline (4) escape codes appear inside.
	assert.Contains(t, out, "\x1b[")
}

func TestHighlightNameSkipsIndexesPastNameRange(t *testing.T) {
	// matchedIndexes 9, 10 fall in "|region" portion of the corpus —
	// should NOT highlight anything inside the name.
	out := highlightName("vm-prod-1", []int{9, 10})
	plain := highlightName("vm-prod-1", nil)
	assert.Equal(t, plain, out, "indexes past name length must not produce highlights")
}

func TestHighlightNameEmptyInputs(t *testing.T) {
	assert.Equal(t, "", highlightName("", []int{0, 1}))
	assert.Equal(t, "name", highlightName("name", nil))
}

func TestMatchRowsCarriesMatchedIndexes(t *testing.T) {
	rl := &ResourceList{
		rows: []rowData{
			{res: inventory.Resource{Name: "alpha-prod"}},
			{res: inventory.Resource{Name: "beta-dev"}},
		},
	}
	got := rl.matchRows("prod")
	assert.NotEmpty(t, got)
	for _, g := range got {
		// At least one matched index for the matched query.
		assert.NotEmpty(t, g.matchedIndexes,
			"matchedIndexes must propagate from fuzzy.Find")
	}
}

func TestToTableRowsAppliesHighlightOnNameColumn(t *testing.T) {
	rl := &ResourceList{
		kind: inventory.KindBucket,
		cols: []ColumnDef{
			{Header: "NAME", Width: 24, Extract: nameOf},
			{Header: "REGION", Width: 12, Extract: func(r inventory.Resource, _ any) string { return r.Region }},
		},
	}
	rows := []rowData{
		{
			res:            inventory.Resource{Name: "my-bucket", Region: "us"},
			matchedIndexes: []int{0, 1},
		},
		{
			res: inventory.Resource{Name: "other-bucket", Region: "us"},
			// no matches — should render plain.
		},
	}
	out := rl.toTableRows(rows)
	assert.Len(t, out, 2)
	// First row's NAME cell carries ANSI; second row's NAME is plain.
	assert.True(t, strings.Contains(out[0][0], "\x1b["),
		"highlighted row should contain ANSI escapes; got %q", out[0][0])
	assert.False(t, strings.Contains(out[1][0], "\x1b["),
		"unhighlighted row should be plain; got %q", out[1][0])
}
