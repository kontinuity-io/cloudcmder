package screens

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/inventory"
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
