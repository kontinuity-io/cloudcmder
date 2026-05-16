package export

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
)

// WriteMultiWorkbook writes a single .xlsx combining N runs. Per-kind sheets
// gain a leading "Scope" column (populated with run.ScopeID). The Summary
// tab has one row per run with per-kind counts plus a TOTAL row. Failed runs
// show "-" in kind count cells and their rows are skipped from kind sheets.
func WriteMultiWorkbook(ctx context.Context, st *store.Store, runs []store.RunSummary, outPath string) error {
	if len(runs) == 0 {
		return fmt.Errorf("export-multi: no runs provided")
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// Pre-compute per-run kind counts; failed runs get nil map.
	counts := make([]map[inventory.Kind]int, len(runs))
	for i, r := range runs {
		if r.Status != "ok" {
			continue
		}
		cs, err := st.CountResourcesByKind(ctx, r.ID)
		if err != nil {
			return fmt.Errorf("export-multi: count for run %s: %w", r.UUID, err)
		}
		counts[i] = cs
	}

	if err := f.SetSheetName("Sheet1", sheetSummary); err != nil {
		return fmt.Errorf("export-multi: rename default sheet: %w", err)
	}
	if err := writeMultiSummary(f, runs, counts); err != nil {
		return fmt.Errorf("export-multi: summary: %w", err)
	}
	if err := writeMultiScopes(ctx, f, st, runs); err != nil {
		return fmt.Errorf("export-multi: scopes: %w", err)
	}
	for _, ks := range kindSheets {
		if err := writeMultiKindSheet(ctx, f, st, runs, ks.kind); err != nil {
			return fmt.Errorf("export-multi: %s: %w", ks.sheet, err)
		}
	}
	if err := writeMultiEdges(ctx, f, st, runs); err != nil {
		return fmt.Errorf("export-multi: edges: %w", err)
	}

	if idx, err := f.GetSheetIndex(sheetSummary); err == nil {
		f.SetActiveSheet(idx)
	}
	if err := f.SaveAs(outPath); err != nil {
		return fmt.Errorf("export-multi: save %s: %w", outPath, err)
	}
	return nil
}

// writeMultiSummary writes the Summary sheet with one row per run (Scope |
// RunUUID | Status | StartedAt | <K1 count> | … | Total) and a final TOTAL
// row. Failed runs show "-" in all count cells.
func writeMultiSummary(f *excelize.File, runs []store.RunSummary, counts []map[inventory.Kind]int) error {
	// Build header: fixed cols + one per kind + Total.
	headers := []any{"Scope", "RunUUID", "Status", "StartedAt"}
	for _, ks := range kindSheets {
		headers = append(headers, string(ks.kind))
	}
	headers = append(headers, "Total")

	setCells := func(sheet string, rowIdx int, cells []any) error {
		for j, v := range cells {
			cell, _ := excelize.CoordinatesToCellName(j+1, rowIdx)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return err
			}
		}
		return nil
	}

	if err := setCells(sheetSummary, 1, headers); err != nil {
		return err
	}

	// Per-run rows.
	totals := make(map[inventory.Kind]int)
	for i, r := range runs {
		started := r.StartedAt.UTC().Format(time.RFC3339)
		row := []any{r.ScopeID, r.UUID, r.Status, started}

		if counts[i] == nil {
			// Failed run — all "-".
			for range kindSheets {
				row = append(row, "-")
			}
			row = append(row, "-")
		} else {
			rowTotal := 0
			for _, ks := range kindSheets {
				n := counts[i][ks.kind]
				row = append(row, n)
				totals[ks.kind] += n
				rowTotal += n
			}
			row = append(row, rowTotal)
		}

		if err := setCells(sheetSummary, i+2, row); err != nil {
			return err
		}
	}

	// TOTAL row.
	grandTotal := 0
	totalRow := []any{"TOTAL", "", "", ""}
	for _, ks := range kindSheets {
		n := totals[ks.kind]
		totalRow = append(totalRow, n)
		grandTotal += n
	}
	totalRow = append(totalRow, grandTotal)
	return setCells(sheetSummary, len(runs)+2, totalRow)
}

// writeMultiScopes writes the Scopes sheet as a union of every run's scope rows.
func writeMultiScopes(ctx context.Context, f *excelize.File, st *store.Store, runs []store.RunSummary) error {
	if _, err := f.NewSheet(sheetScopes); err != nil {
		return err
	}
	headers := []any{"Scope", "ScopeID", "DisplayName", "Parent", "Labels"}
	for j, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(j+1, 1)
		if err := f.SetCellValue(sheetScopes, cell, h); err != nil {
			return err
		}
	}
	rowIdx := 2
	for _, r := range runs {
		scopes, err := st.LoadScopes(ctx, r.ID)
		if err != nil {
			return err
		}
		for _, s := range scopes {
			row := []any{r.ScopeID, s.ID, s.DisplayName, s.Parent, joinLabels(s.Labels)}
			for j, v := range row {
				cell, _ := excelize.CoordinatesToCellName(j+1, rowIdx)
				if err := f.SetCellValue(sheetScopes, cell, v); err != nil {
					return err
				}
			}
			rowIdx++
		}
	}
	return nil
}

// writeMultiKindSheet streams all resources of the given kind across every
// (non-failed) run into one sheet with a leading Project column.
func writeMultiKindSheet(ctx context.Context, f *excelize.File, st *store.Store, runs []store.RunSummary, kind inventory.Kind) error {
	sheet := sheetForKind(kind)
	if _, err := f.NewSheet(sheet); err != nil {
		return err
	}
	cols := columnsFor(kind)
	if len(cols) == 0 {
		return fmt.Errorf("no columns registered for kind %s", kind)
	}

	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return err
	}

	headers := append([]any{"Scope"}, headersOf(cols)...)
	if err := sw.SetRow("A1", headers); err != nil {
		return err
	}

	rowIdx := 2
	for _, r := range runs {
		if r.Status != "ok" {
			continue
		}
		resources, err := st.LoadResources(ctx, r.ID, kind)
		if err != nil {
			return err
		}
		for _, res := range resources {
			raw, _ := res.Detail.(json.RawMessage)
			detail := decodeDetail(kind, raw)
			row := make([]any, 0, 1+len(cols))
			row = append(row, r.ScopeID)
			for _, c := range cols {
				row = append(row, c.Extract(res, detail))
			}
			cell, _ := excelize.CoordinatesToCellName(1, rowIdx)
			if err := sw.SetRow(cell, row); err != nil {
				return err
			}
			rowIdx++
		}
	}
	return sw.Flush()
}

// writeMultiEdges streams edges from all runs into one Edges sheet with a
// leading Project column.
func writeMultiEdges(ctx context.Context, f *excelize.File, st *store.Store, runs []store.RunSummary) error {
	if _, err := f.NewSheet(sheetEdges); err != nil {
		return err
	}
	sw, err := f.NewStreamWriter(sheetEdges)
	if err != nil {
		return err
	}
	if err := sw.SetRow("A1", []any{"Scope", "FromRef", "RefKind", "ToRef"}); err != nil {
		return err
	}

	rowIdx := 2
	for _, r := range runs {
		edges, err := st.LoadEdges(ctx, r.ID)
		if err != nil {
			return err
		}
		for _, e := range edges {
			cell, _ := excelize.CoordinatesToCellName(1, rowIdx)
			if err := sw.SetRow(cell, []any{r.ScopeID, e.FromRef, string(e.RefKind), e.ToRef}); err != nil {
				return err
			}
			rowIdx++
		}
	}
	return sw.Flush()
}
