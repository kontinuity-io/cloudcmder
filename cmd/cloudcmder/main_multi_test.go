package main

import (
	"context"
	"testing"

	"cloudcmder.com/internal/store"
)

// openMemStore opens an in-memory store and registers cleanup.
func openMemStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// insertRun inserts a run row and returns its UUID.
func insertRun(t *testing.T, ctx context.Context, st *store.Store, scopeID string) string {
	t.Helper()
	_, uuid, err := st.OpenRun(ctx, "gcp", scopeID, scopeID, "test")
	if err != nil {
		t.Fatalf("OpenRun(%s): %v", scopeID, err)
	}
	if err := st.FinishRun(ctx, 0, "ok", ""); err != nil {
		// FinishRun needs the actual ID; get it from ListRuns.
		runs, lerr := st.ListRuns(ctx)
		if lerr != nil || len(runs) == 0 {
			t.Fatalf("FinishRun prep for %s: %v / %v", scopeID, err, lerr)
		}
		// Find our run.
		for _, r := range runs {
			if r.UUID == uuid {
				if err2 := st.FinishRun(ctx, r.ID, "ok", ""); err2 != nil {
					t.Fatalf("FinishRun(%s): %v", scopeID, err2)
				}
				return uuid
			}
		}
	}
	return uuid
}

func TestResolveRunsForExportPrecedence(t *testing.T) {
	ctx := context.Background()
	st := openMemStore(t)

	// Seed two scopes with one run each.
	uuid1 := insertRun(t, ctx, st, "proj-1")
	insertRun(t, ctx, st, "proj-2") // uuid2 not needed directly

	// Ensure both runs are accessible.
	allRuns, err := st.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(allRuns) != 2 {
		t.Fatalf("expected 2 runs in store, got %d", len(allRuns))
	}

	tests := []struct {
		name       string
		runsCSV    string
		scopesCSV  string
		wantCount  int
		wantScopes []string // non-empty means check first scope
	}{
		{
			name:      "explicit runs wins",
			runsCSV:   uuid1,
			scopesCSV: "proj-1,proj-2",
			wantCount: 1,
		},
		{
			name:      "scopes filter",
			runsCSV:   "",
			scopesCSV: "proj-1",
			wantCount: 1,
		},
		{
			name:      "neither defaults to latest per scope",
			runsCSV:   "",
			scopesCSV: "",
			wantCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runs, err := resolveRunsForExport(ctx, st, tc.runsCSV, tc.scopesCSV)
			if err != nil {
				t.Fatalf("resolveRunsForExport: %v", err)
			}
			if len(runs) != tc.wantCount {
				t.Errorf("got %d runs, want %d", len(runs), tc.wantCount)
			}
		})
	}
}

func TestResolveRunsForExportMissingUUID(t *testing.T) {
	ctx := context.Background()
	st := openMemStore(t)

	_, err := resolveRunsForExport(ctx, st, "does-not-exist", "")
	if err == nil {
		t.Fatal("expected error for unknown UUID, got nil")
	}
}

func TestResolveRunsForExportMissingScope(t *testing.T) {
	ctx := context.Background()
	st := openMemStore(t)

	// Unknown scope → warning but no error; returns empty slice.
	runs, err := resolveRunsForExport(ctx, st, "", "no-such-scope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs for unknown scope, got %d", len(runs))
	}
}

func TestResolveRunsForExportDefaultEmpty(t *testing.T) {
	ctx := context.Background()
	st := openMemStore(t)

	// No runs in store → empty result, no error.
	runs, err := resolveRunsForExport(ctx, st, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}
}
