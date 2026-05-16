package export

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
)

// seedRun writes a minimal resource batch for a scope and returns the RunSummary.
func seedRun(t *testing.T, ctx context.Context, st *store.Store, scopeID, status string, vms int) store.RunSummary {
	t.Helper()
	runID, runUUID, err := st.OpenRun(ctx, "gcp", scopeID, scopeID, "test")
	if err != nil {
		t.Fatalf("OpenRun(%s): %v", scopeID, err)
	}
	batch := make([]inventory.Resource, vms)
	for i := range batch {
		batch[i] = inventory.Resource{
			Ref:    inventory.ResourceRef{Provider: "gcp", ScopeID: scopeID, Kind: inventory.KindVM, ID: "vm-" + string(rune('a'+i))},
			Kind:   inventory.KindVM,
			Name:   "vm-" + string(rune('a'+i)),
			Region: "us-central1",
			Status: "RUNNING",
		}
	}
	if len(batch) > 0 {
		if err := st.WriteBatch(ctx, runID, batch); err != nil {
			t.Fatalf("WriteBatch(%s): %v", scopeID, err)
		}
	}
	if err := st.FinishRun(ctx, runID, status, ""); err != nil {
		t.Fatalf("FinishRun(%s): %v", scopeID, err)
	}
	r, err := st.FindRunByUUID(ctx, runUUID)
	if err != nil || r == nil {
		t.Fatalf("FindRunByUUID(%s): run=%v err=%v", scopeID, r, err)
	}
	return *r
}

func TestWriteMultiWorkbookShape(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	r1 := seedRun(t, ctx, st, "proj-a", "ok", 3)
	r2 := seedRun(t, ctx, st, "proj-b", "ok", 1)
	r3 := seedRun(t, ctx, st, "proj-c", "ok", 2)

	out := filepath.Join(t.TempDir(), "multi.xlsx")
	if err := WriteMultiWorkbook(ctx, st, []store.RunSummary{r1, r2, r3}, out); err != nil {
		t.Fatalf("WriteMultiWorkbook: %v", err)
	}

	wb, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	t.Cleanup(func() { _ = wb.Close() })

	// Expect same total sheet count as single-run: 34 kinds + Summary + Scopes + Edges = 37.
	sheets := wb.GetSheetList()
	if len(sheets) != 37 {
		t.Errorf("sheet count = %d, want 37: %v", len(sheets), sheets)
	}

	// Summary tab: header (row 1) + 3 project rows + TOTAL row = 5 rows.
	sumRows, err := wb.GetRows(sheetSummary)
	if err != nil {
		t.Fatalf("GetRows Summary: %v", err)
	}
	if len(sumRows) != 5 {
		t.Fatalf("Summary rows = %d, want 5", len(sumRows))
	}
	// Header should start with Scope, RunUUID, Status, StartedAt.
	hdr := sumRows[0]
	for i, want := range []string{"Scope", "RunUUID", "Status", "StartedAt"} {
		if i >= len(hdr) || hdr[i] != want {
			t.Errorf("Summary header col %d = %q, want %q", i, func() string {
				if i < len(hdr) {
					return hdr[i]
				}
				return "<missing>"
			}(), want)
		}
	}
	// Last row is TOTAL.
	lastRow := sumRows[len(sumRows)-1]
	if lastRow[0] != "TOTAL" {
		t.Errorf("Summary last row col 0 = %q, want TOTAL", lastRow[0])
	}

	// VMs sheet: header + 6 rows (3+1+2 vms).
	vmRows, err := wb.GetRows(sheetVMs)
	if err != nil {
		t.Fatalf("GetRows VMs: %v", err)
	}
	if len(vmRows) != 7 {
		t.Fatalf("VMs rows = %d, want 7 (header+6)", len(vmRows))
	}
	// First column of header should be "Scope".
	if vmRows[0][0] != "Scope" {
		t.Errorf("VMs header[0] = %q, want Scope", vmRows[0][0])
	}
	// Data rows carry the scope ID in col 0.
	scopesSeen := map[string]int{}
	for _, row := range vmRows[1:] {
		if len(row) > 0 {
			scopesSeen[row[0]]++
		}
	}
	if scopesSeen["proj-a"] != 3 || scopesSeen["proj-b"] != 1 || scopesSeen["proj-c"] != 2 {
		t.Errorf("VMs project distribution: %v", scopesSeen)
	}
}

func TestWriteMultiWorkbookEmpty(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	out := filepath.Join(t.TempDir(), "empty.xlsx")
	if err := WriteMultiWorkbook(ctx, st, nil, out); err == nil {
		t.Fatal("expected error for zero runs, got nil")
	}
}

func TestWriteMultiWorkbookFailedRun(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	r1 := seedRun(t, ctx, st, "proj-ok", "ok", 2)
	r2 := seedRun(t, ctx, st, "proj-fail", "failed", 0)

	out := filepath.Join(t.TempDir(), "partial.xlsx")
	if err := WriteMultiWorkbook(ctx, st, []store.RunSummary{r1, r2}, out); err != nil {
		t.Fatalf("WriteMultiWorkbook: %v", err)
	}

	wb, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	t.Cleanup(func() { _ = wb.Close() })

	sumRows, err := wb.GetRows(sheetSummary)
	if err != nil {
		t.Fatalf("GetRows Summary: %v", err)
	}
	// header + ok row + failed row + TOTAL = 4 rows.
	if len(sumRows) != 4 {
		t.Fatalf("Summary rows = %d, want 4", len(sumRows))
	}
	// Failed run row: col 2 = "failed", kind cols should be "-".
	failRow := sumRows[2]
	if failRow[2] != "failed" {
		t.Errorf("failed run Status cell = %q, want failed", failRow[2])
	}
	// Kind count cell (index 4) should be "-" for failed run.
	if failRow[4] != "-" {
		t.Errorf("failed run first kind cell = %q, want -", failRow[4])
	}

	// VMs sheet should have only 2 rows (header + 2 from ok run).
	vmRows, err := wb.GetRows(sheetVMs)
	if err != nil {
		t.Fatalf("GetRows VMs: %v", err)
	}
	if len(vmRows) != 3 {
		t.Fatalf("VMs rows = %d, want 3 (header+2)", len(vmRows))
	}
}
