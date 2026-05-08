package store

import (
	"context"
	"path/filepath"
	"testing"

	"cloudcmder.com/internal/inventory"
)

func TestSchemaCreatesExpectedTables(t *testing.T) {
	s := openMemory(t)
	want := []string{"edges", "resources", "runs", "schema_meta", "scopes"}
	got := tables(t, s)
	if len(got) != len(want) {
		t.Fatalf("table count = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("tables[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestRoundTripResources(t *testing.T) {
	ctx := context.Background()
	s := openMemory(t)

	runID, runUUID, err := s.OpenRun(ctx, "gcp", "p1", "Project One", "test")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}
	if runUUID == "" {
		t.Fatalf("expected non-empty uuid")
	}

	batch := []inventory.Resource{
		{
			Ref:    inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindVM, ID: "vm-a"},
			Kind:   inventory.KindVM,
			Name:   "vm-a",
			Region: "us-central1",
			Status: "RUNNING",
			Labels: map[string]string{"env": "prod", "team": "infra"},
		},
		{
			Ref:    inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindBucket, ID: "b1"},
			Kind:   inventory.KindBucket,
			Name:   "b1",
			Region: "US",
			Status: "ACTIVE",
		},
	}
	if err := s.WriteBatch(ctx, runID, batch); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}
	if err := s.FinishRun(ctx, runID, "ok", ""); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}

	got, err := s.LoadResources(ctx, runID)
	if err != nil {
		t.Fatalf("LoadResources: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d, want 2", len(got))
	}

	// LoadResources orders by kind, name → Bucket("b1") < VM("vm-a").
	if got[0].Kind != inventory.KindBucket || got[0].Name != "b1" {
		t.Errorf("got[0] = %v", got[0])
	}
	if got[1].Kind != inventory.KindVM || got[1].Name != "vm-a" {
		t.Errorf("got[1] = %v", got[1])
	}
	if got[1].Labels["env"] != "prod" || got[1].Labels["team"] != "infra" {
		t.Errorf("vm labels lost: %v", got[1].Labels)
	}
	if got[1].Ref.String() != "gcp:p1:VM:vm-a" {
		t.Errorf("vm ref = %q, want gcp:p1:VM:vm-a", got[1].Ref.String())
	}

	// Filter by kind.
	onlyBuckets, err := s.LoadResources(ctx, runID, inventory.KindBucket)
	if err != nil {
		t.Fatalf("LoadResources kind filter: %v", err)
	}
	if len(onlyBuckets) != 1 || onlyBuckets[0].Kind != inventory.KindBucket {
		t.Errorf("kind filter returned %v", onlyBuckets)
	}

	counts, err := s.CountResourcesByKind(ctx, runID)
	if err != nil {
		t.Fatalf("CountResourcesByKind: %v", err)
	}
	if counts[inventory.KindVM] != 1 || counts[inventory.KindBucket] != 1 {
		t.Errorf("counts wrong: %v", counts)
	}
}

func TestWriteBatchChunksLargePayload(t *testing.T) {
	ctx := context.Background()
	s := openMemory(t)

	runID, _, err := s.OpenRun(ctx, "gcp", "p1", "P1", "test")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}
	const n = 1100 // exercises 3 chunks at batchSize=500.
	batch := make([]inventory.Resource, n)
	for i := range batch {
		id := "vm-" + itoa(i)
		batch[i] = inventory.Resource{
			Ref:  inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindVM, ID: id},
			Kind: inventory.KindVM,
			Name: id,
		}
	}
	if err := s.WriteBatch(ctx, runID, batch); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}
	got, err := s.LoadResources(ctx, runID)
	if err != nil {
		t.Fatalf("LoadResources: %v", err)
	}
	if len(got) != n {
		t.Fatalf("loaded %d rows, want %d", len(got), n)
	}
}

func TestCancelledContextWritesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled.

	s := openMemory(t)
	runID, _, err := s.OpenRun(context.Background(), "gcp", "p1", "P1", "test")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}
	batch := []inventory.Resource{
		{Ref: inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindVM, ID: "x"}, Kind: inventory.KindVM, Name: "x"},
	}
	if err := s.WriteBatch(ctx, runID, batch); err == nil {
		t.Fatalf("WriteBatch on cancelled ctx returned nil error")
	}
	got, err := s.LoadResources(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadResources: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected zero rows after cancelled write, got %d", len(got))
	}
}

func TestWriteBatchPersistsEdges(t *testing.T) {
	ctx := context.Background()
	s := openMemory(t)

	runID, _, err := s.OpenRun(ctx, "gcp", "p1", "P1", "test")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}

	vmRef := inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindVM, ID: "vm-a"}
	subnetRef := inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindSubnet, ID: "default-uc1"}
	diskRef := inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindDisk, ID: "vm-boot-pd"}

	batch := []inventory.Resource{
		{
			Ref: vmRef, Kind: inventory.KindVM, Name: "vm-a",
			Refs: map[inventory.RefKind][]inventory.ResourceRef{
				inventory.RefRoutesFrom: {subnetRef},
			},
		},
		{
			Ref: diskRef, Kind: inventory.KindDisk, Name: "vm-boot-pd",
			Refs: map[inventory.RefKind][]inventory.ResourceRef{
				inventory.RefAttachedTo: {vmRef},
			},
		},
		{Ref: subnetRef, Kind: inventory.KindSubnet, Name: "default-uc1"},
	}
	if err := s.WriteBatch(ctx, runID, batch); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}

	edges, err := s.LoadEdges(ctx, runID)
	if err != nil {
		t.Fatalf("LoadEdges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("got %d edges, want 2: %+v", len(edges), edges)
	}

	want := map[string]inventory.RefKind{
		"gcp:p1:VM:vm-a -> gcp:p1:Subnet:default-uc1":  inventory.RefRoutesFrom,
		"gcp:p1:Disk:vm-boot-pd -> gcp:p1:VM:vm-a":     inventory.RefAttachedTo,
	}
	for _, e := range edges {
		key := e.FromRef + " -> " + e.ToRef
		k, ok := want[key]
		if !ok {
			t.Errorf("unexpected edge %s (%s)", key, e.RefKind)
			continue
		}
		if e.RefKind != k {
			t.Errorf("edge %s kind=%s, want %s", key, e.RefKind, k)
		}
	}

	// Re-writing the same batch must be idempotent (INSERT OR IGNORE).
	if err := s.WriteBatch(ctx, runID, batch); err != nil {
		t.Fatalf("re-WriteBatch: %v", err)
	}
	edges2, err := s.LoadEdges(ctx, runID)
	if err != nil {
		t.Fatalf("LoadEdges 2: %v", err)
	}
	if len(edges2) != 2 {
		t.Errorf("after re-write got %d edges, want 2 (idempotent)", len(edges2))
	}
}

func TestLoadResourceIndex(t *testing.T) {
	ctx := context.Background()
	s := openMemory(t)

	runID, _, err := s.OpenRun(ctx, "gcp", "p1", "P1", "test")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}

	batch := []inventory.Resource{
		{Ref: inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindVM, ID: "vm-a"}, Kind: inventory.KindVM, Name: "vm-a"},
		{Ref: inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindVM, ID: "vm-b"}, Kind: inventory.KindVM, Name: "vm-b"},
		{Ref: inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindBucket, ID: "b1"}, Kind: inventory.KindBucket, Name: "b1"},
	}
	if err := s.WriteBatch(ctx, runID, batch); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}

	idx, err := s.LoadResourceIndex(ctx, runID)
	if err != nil {
		t.Fatalf("LoadResourceIndex: %v", err)
	}
	if len(idx) != 3 {
		t.Fatalf("got %d entries, want 3", len(idx))
	}
	// ORDER BY kind, name → Bucket("b1") < VM("vm-a") < VM("vm-b").
	want := []ResourceIndexEntry{
		{Kind: inventory.KindBucket, ID: "b1", Name: "b1"},
		{Kind: inventory.KindVM, ID: "vm-a", Name: "vm-a"},
		{Kind: inventory.KindVM, ID: "vm-b", Name: "vm-b"},
	}
	for i, w := range want {
		if idx[i] != w {
			t.Errorf("idx[%d] = %+v, want %+v", i, idx[i], w)
		}
	}
}

func TestRunWithoutFinishStaysRunning(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cc.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	_, runUUID, err := s.OpenRun(ctx, "gcp", "p1", "P1", "test")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}
	// Deliberately skip FinishRun.

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen and confirm status='running' survives.
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	got, err := s2.FindRunByUUID(ctx, runUUID)
	if err != nil || got == nil {
		t.Fatalf("FindRunByUUID: %v / %v", got, err)
	}
	if got.Status != "running" {
		t.Errorf("status = %q, want running", got.Status)
	}
	if got.FinishedAt != nil {
		t.Errorf("finished_at = %v, want nil", got.FinishedAt)
	}
}

func TestListRunsAndLatestForScope(t *testing.T) {
	ctx := context.Background()
	s := openMemory(t)

	id1, _, _ := s.OpenRun(ctx, "gcp", "p1", "P1", "test")
	_ = s.FinishRun(ctx, id1, "ok", "")
	id2, _, _ := s.OpenRun(ctx, "gcp", "p1", "P1", "test")
	_ = s.FinishRun(ctx, id2, "partial", "")
	id3, _, _ := s.OpenRun(ctx, "gcp", "p2", "P2", "test")
	_ = s.FinishRun(ctx, id3, "ok", "")

	all, err := s.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d runs, want 3", len(all))
	}

	latest, err := s.LatestRunForScope(ctx, "p1")
	if err != nil || latest == nil {
		t.Fatalf("LatestRunForScope: %v / %v", latest, err)
	}
	if latest.ID != id2 {
		t.Errorf("latest id = %d, want %d", latest.ID, id2)
	}

	missing, err := s.LatestRunForScope(ctx, "absent")
	if err != nil {
		t.Fatalf("LatestRunForScope absent: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for absent scope, got %v", missing)
	}
}

func openMemory(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func tables(t *testing.T, s *Store) []string {
	t.Helper()
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		// Skip the sqlite_sequence table that AUTOINCREMENT creates implicitly.
		if n == "sqlite_sequence" {
			continue
		}
		out = append(out, n)
	}
	return out
}

// itoa avoids strconv to keep the test file's import surface minimal.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
