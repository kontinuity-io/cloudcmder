package screens

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/inventory"
)

func TestColumnsForReturnsKnownKinds(t *testing.T) {
	for _, k := range []inventory.Kind{
		inventory.KindVM, inventory.KindDisk, inventory.KindNetwork,
		inventory.KindSubnet, inventory.KindFirewall, inventory.KindLoadBalancer,
		inventory.KindDatabase, inventory.KindCluster, inventory.KindBucket,
		inventory.KindFunction, inventory.KindVertexAI,
	} {
		cols, ok := columnsFor(k, 0)
		assert.True(t, ok, "kind %s should be registered", k)
		assert.NotEmpty(t, cols, "kind %s should have columns", k)
	}
	_, ok := columnsFor("Unknown", 0)
	assert.False(t, ok)
}

func TestFitColumnWidthsLeavesNaturalWhenItFits(t *testing.T) {
	cols := []ColumnDef{
		{Header: "A", Width: 10},
		{Header: "B", Width: 20},
		{Header: "C", Width: 30},
	}
	// 60 + 6 padding = 66. Available 100 — fits with room.
	fitColumnWidths(cols, 100)
	assert.Equal(t, 10, cols[0].Width)
	assert.Equal(t, 20, cols[1].Width)
	assert.Equal(t, 30, cols[2].Width)
}

func TestFitColumnWidthsShrinksProportionally(t *testing.T) {
	cols := []ColumnDef{
		{Header: "A", Width: 20},
		{Header: "B", Width: 40},
		{Header: "C", Width: 40},
	}
	// 100 + 6 padding = 106 natural. Available 56 → budget 50.
	fitColumnWidths(cols, 56)
	total := cols[0].Width + cols[1].Width + cols[2].Width
	assert.LessOrEqual(t, total, 50, "widths should fit in budget")
	// Proportions roughly preserved (B and C should be similar).
	assert.Equal(t, cols[1].Width, cols[2].Width)
	assert.Less(t, cols[0].Width, cols[1].Width)
}

func TestFitColumnWidthsHonoursMinimumFloor(t *testing.T) {
	cols := []ColumnDef{
		{Header: "A", Width: 10},
		{Header: "B", Width: 100},
	}
	// Very narrow available — should not shrink either column below 4.
	fitColumnWidths(cols, 20)
	for _, c := range cols {
		assert.GreaterOrEqual(t, c.Width, 4, "min floor")
	}
}

func TestColumnsForAdaptsToWidth(t *testing.T) {
	natural, _ := columnsFor(inventory.KindVM, 0)
	naturalSum := 0
	for _, c := range natural {
		naturalSum += c.Width
	}

	narrow, _ := columnsFor(inventory.KindVM, 60)
	narrowSum := 0
	for _, c := range narrow {
		narrowSum += c.Width
	}
	require.Equal(t, len(natural), len(narrow))
	assert.Less(t, narrowSum, naturalSum, "narrow terminal should produce smaller columns")
}
