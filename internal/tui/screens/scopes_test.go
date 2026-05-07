package screens

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/store"
)

func TestUniqueScopes(t *testing.T) {
	t1 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		in   []store.RunSummary
		want []ScopeSummary
	}{
		{
			name: "empty",
			in:   nil,
			want: []ScopeSummary{},
		},
		{
			name: "one scope, one run",
			in: []store.RunSummary{
				{UUID: "u1", ScopeID: "p1", ScopeName: "Project One", Status: "ok", StartedAt: t1},
			},
			want: []ScopeSummary{
				{ScopeID: "p1", DisplayName: "Project One", LatestUUID: "u1", LatestStartedAt: t1, LatestStatus: "ok"},
			},
		},
		{
			name: "one scope, three runs (newest wins, input is DESC)",
			in: []store.RunSummary{
				{UUID: "u3", ScopeID: "p1", Status: "ok", StartedAt: t3},
				{UUID: "u2", ScopeID: "p1", Status: "running", StartedAt: t2},
				{UUID: "u1", ScopeID: "p1", Status: "failed", StartedAt: t1},
			},
			want: []ScopeSummary{
				{ScopeID: "p1", LatestUUID: "u3", LatestStartedAt: t3, LatestStatus: "ok"},
			},
		},
		{
			name: "three scopes, mixed",
			in: []store.RunSummary{
				{UUID: "u3a", ScopeID: "p3", Status: "ok", StartedAt: t3},
				{UUID: "u2a", ScopeID: "p2", Status: "running", StartedAt: t2},
				{UUID: "u1a", ScopeID: "p1", Status: "ok", StartedAt: t1},
				{UUID: "u0a", ScopeID: "p1", Status: "ok", StartedAt: t1.Add(-time.Hour)},
			},
			want: []ScopeSummary{
				{ScopeID: "p3", LatestUUID: "u3a", LatestStartedAt: t3, LatestStatus: "ok"},
				{ScopeID: "p2", LatestUUID: "u2a", LatestStartedAt: t2, LatestStatus: "running"},
				{ScopeID: "p1", LatestUUID: "u1a", LatestStartedAt: t1, LatestStatus: "ok"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := uniqueScopes(tc.in)
			require.Equal(t, len(tc.want), len(got))
			for i := range tc.want {
				assert.Equal(t, tc.want[i].ScopeID, got[i].ScopeID)
				assert.Equal(t, tc.want[i].LatestUUID, got[i].LatestUUID)
				assert.Equal(t, tc.want[i].LatestStartedAt, got[i].LatestStartedAt)
				assert.Equal(t, tc.want[i].LatestStatus, got[i].LatestStatus)
			}
		})
	}
}
