package export

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
)

func TestWriteWorkbookSheetsAndContent(t *testing.T) {
	ctx := context.Background()

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	runID, runUUID, err := st.OpenRun(ctx, "gcp", "p1", "Project One", "test")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}

	vmRef := inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindVM, ID: "vm-a"}
	subnetRef := inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindSubnet, ID: "default"}
	diskRef := inventory.ResourceRef{Provider: "gcp", ScopeID: "p1", Kind: inventory.KindDisk, ID: "vm-a-boot"}

	batch := []inventory.Resource{
		{
			Ref: vmRef, Kind: inventory.KindVM, Name: "vm-a",
			Region: "us-central1-a", Status: "RUNNING",
			Detail: &inventory.VMDetail{
				MachineType: "n2-standard-4", VCPUs: 4, MemoryMiB: 16384,
				OSFamily: "debian-12", Zone: "us-central1-a",
				BootDisk: inventory.DiskRef{Name: "vm-a-boot", SizeGB: 100, Type: "pd-balanced"},
			},
			Refs: map[inventory.RefKind][]inventory.ResourceRef{
				inventory.RefRoutesFrom: {subnetRef},
			},
		},
		{
			Ref: diskRef, Kind: inventory.KindDisk, Name: "vm-a-boot",
			Region: "us-central1-a", Status: "READY",
			Detail: &inventory.DiskDetail{SizeGB: 100, Type: "pd-balanced", Zone: "us-central1-a"},
			Refs: map[inventory.RefKind][]inventory.ResourceRef{
				inventory.RefAttachedTo: {vmRef},
			},
		},
	}
	if err := st.WriteBatch(ctx, runID, batch); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}
	if err := st.FinishRun(ctx, runID, "ok", ""); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}

	run, err := st.FindRunByUUID(ctx, runUUID)
	if err != nil || run == nil {
		t.Fatalf("FindRunByUUID: run=%v err=%v", run, err)
	}

	out := filepath.Join(t.TempDir(), "out.xlsx")
	if err := WriteWorkbook(ctx, st, *run, out); err != nil {
		t.Fatalf("WriteWorkbook: %v", err)
	}

	wb, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	t.Cleanup(func() { _ = wb.Close() })

	// 34 kind sheets + Summary + Scopes + Edges = 37 total.
	wantSheets := make([]string, 0, 37)
	wantSheets = append(wantSheets, sheetSummary, sheetScopes)
	for _, ks := range kindSheets {
		wantSheets = append(wantSheets, ks.sheet)
	}
	wantSheets = append(wantSheets, sheetEdges)
	gotSheets := wb.GetSheetList()
	if len(gotSheets) != len(wantSheets) {
		t.Errorf("got %d sheets, want %d: %v", len(gotSheets), len(wantSheets), gotSheets)
	}
	gotSet := map[string]bool{}
	for _, s := range gotSheets {
		gotSet[s] = true
	}
	for _, w := range wantSheets {
		if !gotSet[w] {
			t.Errorf("missing sheet %q", w)
		}
	}

	// Summary first row should be the header pair.
	if v, _ := wb.GetCellValue(sheetSummary, "A1"); v != "Field" {
		t.Errorf("Summary A1 = %q, want Field", v)
	}
	if v, _ := wb.GetCellValue(sheetSummary, "B2"); v != runUUID {
		t.Errorf("Summary B2 = %q, want %q", v, runUUID)
	}

	// VMs sheet has 1 header row + 1 data row.
	rows, err := wb.GetRows(sheetVMs)
	if err != nil {
		t.Fatalf("GetRows VMs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("VMs row count = %d, want 2 (header+1 vm)", len(rows))
	}
	if rows[0][0] != "Name" {
		t.Errorf("VMs header[0] = %q, want Name", rows[0][0])
	}
	if rows[1][0] != "vm-a" {
		t.Errorf("VMs data[0] = %q, want vm-a", rows[1][0])
	}

	// Edges sheet: header + 2 edges.
	edges, err := wb.GetRows(sheetEdges)
	if err != nil {
		t.Fatalf("GetRows Edges: %v", err)
	}
	if len(edges) != 3 {
		t.Errorf("Edges row count = %d, want 3 (header + 2 edges)", len(edges))
	}
}

func TestVMGPUColumnsRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	runID, runUUID, err := st.OpenRun(ctx, "gcp", "p-gpu", "GPU Project", "test")
	if err != nil {
		t.Fatalf("OpenRun: %v", err)
	}

	batch := []inventory.Resource{{
		Ref:    inventory.ResourceRef{Provider: "gcp", ScopeID: "p-gpu", Kind: inventory.KindVM, ID: "gpu-vm"},
		Kind:   inventory.KindVM,
		Name:   "gpu-vm",
		Region: "us-central1-a",
		Status: "RUNNING",
		Detail: &inventory.VMDetail{
			MachineType: "n1-standard-8",
			CPUPlatform: "Intel Skylake",
			Accelerators: []inventory.Accelerator{
				{Type: "nvidia-l4", Count: 2},
			},
		},
	}}
	if err := st.WriteBatch(ctx, runID, batch); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}
	if err := st.FinishRun(ctx, runID, "ok", ""); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	run, err := st.FindRunByUUID(ctx, runUUID)
	if err != nil || run == nil {
		t.Fatalf("FindRunByUUID: run=%v err=%v", run, err)
	}

	// --- single-run export ---
	out := filepath.Join(t.TempDir(), "gpu.xlsx")
	if err := WriteWorkbook(ctx, st, *run, out); err != nil {
		t.Fatalf("WriteWorkbook: %v", err)
	}
	wb, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	t.Cleanup(func() { _ = wb.Close() })

	hdr, _ := wb.GetRows(sheetVMs)
	if len(hdr) < 2 {
		t.Fatalf("VMs: want at least 2 rows (header+data), got %d", len(hdr))
	}
	colIdx := map[string]int{}
	for i, h := range hdr[0] {
		colIdx[h] = i
	}
	gpuTypeIdx, ok1 := colIdx["GPUType"]
	gpuCountIdx, ok2 := colIdx["GPUCount"]
	if !ok1 || !ok2 {
		t.Fatalf("GPUType/GPUCount not in header: %v", hdr[0])
	}
	dataRow := hdr[1]
	if gpuTypeIdx >= len(dataRow) || dataRow[gpuTypeIdx] != "nvidia-l4" {
		t.Errorf("GPUType = %q, want nvidia-l4", func() string {
			if gpuTypeIdx < len(dataRow) {
				return dataRow[gpuTypeIdx]
			}
			return "<missing>"
		}())
	}
	if gpuCountIdx >= len(dataRow) || dataRow[gpuCountIdx] != "2" {
		t.Errorf("GPUCount = %q, want 2", func() string {
			if gpuCountIdx < len(dataRow) {
				return dataRow[gpuCountIdx]
			}
			return "<missing>"
		}())
	}
	// nvidia-l4 = 24 GB/card × 2 = 48 GB total
	gpuMemIdx, ok3 := colIdx["GPUMemGB"]
	if !ok3 {
		t.Fatal("GPUMemGB not in header")
	}
	if gpuMemIdx >= len(dataRow) || dataRow[gpuMemIdx] != "48" {
		t.Errorf("GPUMemGB = %q, want 48", func() string {
			if gpuMemIdx < len(dataRow) {
				return dataRow[gpuMemIdx]
			}
			return "<missing>"
		}())
	}

	// --- multi-run export uses the same WriteMultiWorkbook path ---
	outMulti := filepath.Join(t.TempDir(), "gpu-multi.xlsx")
	if err := WriteMultiWorkbook(ctx, st, []store.RunSummary{*run}, outMulti); err != nil {
		t.Fatalf("WriteMultiWorkbook: %v", err)
	}
	wbm, err := excelize.OpenFile(outMulti)
	if err != nil {
		t.Fatalf("OpenFile multi: %v", err)
	}
	t.Cleanup(func() { _ = wbm.Close() })

	mRows, _ := wbm.GetRows(sheetVMs)
	if len(mRows) < 2 {
		t.Fatalf("multi VMs: want ≥2 rows, got %d", len(mRows))
	}
	mColIdx := map[string]int{}
	for i, h := range mRows[0] {
		mColIdx[h] = i
	}
	if _, ok := mColIdx["GPUType"]; !ok {
		t.Fatalf("multi GPUType not in header: %v", mRows[0])
	}
	mData := mRows[1]
	mGPUType := mData[mColIdx["GPUType"]]
	mGPUCount := mData[mColIdx["GPUCount"]]
	mGPUMem := mData[mColIdx["GPUMemGB"]]
	if mGPUType != "nvidia-l4" {
		t.Errorf("multi GPUType = %q, want nvidia-l4", mGPUType)
	}
	if mGPUCount != "2" {
		t.Errorf("multi GPUCount = %q, want 2", mGPUCount)
	}
	if mGPUMem != "48" {
		t.Errorf("multi GPUMemGB = %q, want 48", mGPUMem)
	}
}

func TestDefaultPathContainsScopeAndShortUUID(t *testing.T) {
	run := store.RunSummary{
		UUID:    "be603380-aaaa-bbbb-cccc-dddddddddddd",
		ScopeID: "fazalullah-lab",
	}
	got := DefaultPath(run)
	if !filepath.IsAbs(got) && got != filepath.Clean(".cloudcmder/exports/fazalullah-lab-be603380.xlsx") {
		t.Errorf("DefaultPath = %q", got)
	}
}
