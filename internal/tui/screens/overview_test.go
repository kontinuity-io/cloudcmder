package screens

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
)

func TestSortKindCounts(t *testing.T) {
	cases := []struct {
		name string
		in   map[inventory.Kind]int
		want []KindCount
	}{
		{
			name: "empty",
			in:   map[inventory.Kind]int{},
			want: []KindCount{},
		},
		{
			name: "single",
			in: map[inventory.Kind]int{
				inventory.KindVM: 3,
			},
			want: []KindCount{{Kind: inventory.KindVM, Count: 3}},
		},
		{
			name: "ties broken alphabetically (Bucket < Disk by string)",
			in: map[inventory.Kind]int{
				inventory.KindBucket: 5,
				inventory.KindDisk:   5,
			},
			want: []KindCount{
				{Kind: inventory.KindBucket, Count: 5},
				{Kind: inventory.KindDisk, Count: 5},
			},
		},
		{
			name: "typical mix matches M2 verify shape (descending count)",
			in: map[inventory.Kind]int{
				inventory.KindVM:       16,
				inventory.KindDisk:     15,
				inventory.KindSubnet:   12,
				inventory.KindFirewall: 10,
				inventory.KindNetwork:  9,
				inventory.KindBucket:   3,
				inventory.KindCluster:  2,
			},
			want: []KindCount{
				{Kind: inventory.KindVM, Count: 16},
				{Kind: inventory.KindDisk, Count: 15},
				{Kind: inventory.KindSubnet, Count: 12},
				{Kind: inventory.KindFirewall, Count: 10},
				{Kind: inventory.KindNetwork, Count: 9},
				{Kind: inventory.KindBucket, Count: 3},
				{Kind: inventory.KindCluster, Count: 2},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sortKindCounts(tc.in)
			require.Equal(t, len(tc.want), len(got))
			for i := range tc.want {
				assert.Equal(t, tc.want[i].Kind, got[i].Kind)
				assert.Equal(t, tc.want[i].Count, got[i].Count)
			}
		})
	}
}

func TestRenderCountBarWidthConsistency(t *testing.T) {
	// renderCountBar must always return exactly w visual characters (ANSI aside).
	cases := []struct{ count, max, w int }{
		{0, 10, 16}, {5, 10, 16}, {10, 10, 16}, {3, 3, 8}, {0, 0, 12},
	}
	for _, tc := range cases {
		bar := renderCountBar(tc.count, tc.max, tc.w)
		// Strip ANSI escape codes to measure visual width.
		plain := stripANSI(bar)
		assert.Equal(t, tc.w, len([]rune(plain)),
			"count=%d max=%d w=%d: got %q (visual width %d)",
			tc.count, tc.max, tc.w, plain, len([]rune(plain)))
	}
}

func TestRenderHealthDotsFiveGlyphs(t *testing.T) {
	cases := []store.KindStats{
		{Total: 0},
		{Total: 10, Healthy: 10},
		{Total: 10, Healthy: 5, Warning: 5},
		{Total: 10, Healthy: 3, Critical: 7},
	}
	for _, ks := range cases {
		dots := renderHealthDots(ks)
		plain := stripANSI(dots)
		// Count bullet/circle runes — must be exactly 5.
		count := strings.Count(plain, "●") + strings.Count(plain, "○")
		assert.Equal(t, 5, count, "expected 5 dot glyphs for %+v, got %q", ks, plain)
	}
}

func TestRenderStatusBadgeContent(t *testing.T) {
	ok := renderStatusBadge(store.KindStats{Total: 5, Healthy: 5})
	assert.Contains(t, stripANSI(ok), "[OK]")

	warn := renderStatusBadge(store.KindStats{Total: 5, Healthy: 3, Warning: 2})
	assert.Contains(t, stripANSI(warn), "[WARN: 2]")

	crit := renderStatusBadge(store.KindStats{Total: 5, Healthy: 1, Critical: 4})
	assert.Contains(t, stripANSI(crit), "[CRIT: 4]")
}

// stripANSI removes ANSI escape sequences so tests can measure plain content.
func stripANSI(s string) string {
	out := strings.Builder{}
	inSeq := false
	for _, r := range s {
		if inSeq {
			if r == 'm' {
				inSeq = false
			}
			continue
		}
		if r == '\x1b' {
			inSeq = true
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
