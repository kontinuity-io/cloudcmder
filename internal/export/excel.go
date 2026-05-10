package export

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/xuri/excelize/v2"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
)

// Sheet names — kept as constants so writers and tests agree on spelling.
const (
	sheetSummary         = "Summary"
	sheetScopes          = "Scopes"
	sheetVMs             = "VMs"
	sheetDisks           = "Disks"
	sheetNetworks        = "Networks"
	sheetSubnets         = "Subnets"
	sheetFirewalls       = "Firewalls"
	sheetLoadBalancers   = "LoadBalancers"
	sheetDatabases       = "Databases"
	sheetClusters        = "Clusters"
	sheetBuckets         = "Buckets"
	sheetFunctions       = "Functions"
	sheetVertexAI        = "VertexAI"
	sheetApigee          = "Apigee"
	sheetFirebase        = "Firebase"
	sheetAppEngine       = "AppEngine"
	sheetBigQuery        = "BigQuery"
	sheetDNS             = "DNS"
	sheetMemorystore     = "Memorystore"
	sheetArtifactReg     = "ArtifactRegistry"
	sheetCloudScheduler  = "CloudScheduler"
	sheetPubSub          = "PubSub"
	sheetSpanner         = "Spanner"
	sheetBigtable        = "Bigtable"
	sheetKMS             = "KMS"
	sheetSecretManager   = "SecretManager"
	sheetDataflow        = "Dataflow"
	sheetDataproc        = "Dataproc"
	sheetComposer        = "Composer"
	sheetCloudTasks      = "CloudTasks"
	sheetMonitoring      = "Monitoring"
	sheetLogging         = "Logging"
	sheetOSConfig        = "OSConfig"
	sheetVPN             = "VPN"
	sheetRouter          = "Router"
	sheetCloudBuild      = "CloudBuild"
	sheetEdges           = "Edges"
)

// kindSheets pairs each Kind with its destination sheet name. Order is the
// rendered tab order in the output workbook.
var kindSheets = []struct {
	kind  inventory.Kind
	sheet string
}{
	{inventory.KindVM, sheetVMs},
	{inventory.KindDisk, sheetDisks},
	{inventory.KindNetwork, sheetNetworks},
	{inventory.KindSubnet, sheetSubnets},
	{inventory.KindFirewall, sheetFirewalls},
	{inventory.KindLoadBalancer, sheetLoadBalancers},
	{inventory.KindDatabase, sheetDatabases},
	{inventory.KindCluster, sheetClusters},
	{inventory.KindBucket, sheetBuckets},
	{inventory.KindFunction, sheetFunctions},
	{inventory.KindVertexAI, sheetVertexAI},
	{inventory.KindApigee, sheetApigee},
	{inventory.KindFirebase, sheetFirebase},
	{inventory.KindAppEngine, sheetAppEngine},
	{inventory.KindBigQuery, sheetBigQuery},
	{inventory.KindDNS, sheetDNS},
	{inventory.KindMemorystore, sheetMemorystore},
	{inventory.KindArtifactRegistry, sheetArtifactReg},
	{inventory.KindCloudScheduler, sheetCloudScheduler},
	{inventory.KindPubSub, sheetPubSub},
	{inventory.KindSpanner, sheetSpanner},
	{inventory.KindBigtable, sheetBigtable},
	{inventory.KindKMS, sheetKMS},
	{inventory.KindSecretManager, sheetSecretManager},
	{inventory.KindDataflow, sheetDataflow},
	{inventory.KindDataproc, sheetDataproc},
	{inventory.KindComposer, sheetComposer},
	{inventory.KindCloudTasks, sheetCloudTasks},
	{inventory.KindMonitoring, sheetMonitoring},
	{inventory.KindLogging, sheetLogging},
	{inventory.KindOSConfig, sheetOSConfig},
	{inventory.KindVPN, sheetVPN},
	{inventory.KindRouter, sheetRouter},
	{inventory.KindCloudBuild, sheetCloudBuild},
}

// sheetForKind returns the Excel sheet name for a Kind, or the Kind string
// itself as a fallback for unknown kinds.
func sheetForKind(k inventory.Kind) string {
	for _, ks := range kindSheets {
		if ks.kind == k {
			return ks.sheet
		}
	}
	return string(k)
}

// WriteWorkbook materialises a run's resources and edges into a multi-tab
// .xlsx at outPath. Resource sheets stream row-by-row via excelize
// StreamWriter (constant memory); Summary/Scopes/Edges use plain
// SetCellValue (small, pre-known shape). All reads come from the store —
// no provider calls.
//
// Caller responsibilities: resolve the run before calling, and ensure the
// parent directory of outPath exists (DefaultPath + os.MkdirAll for the TUI
// path; main.go relies on the user's shell for the headless path).
func WriteWorkbook(ctx context.Context, st *store.Store, run store.RunSummary, outPath string) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// excelize creates "Sheet1" by default; rename to Summary so the workbook
	// opens on it.
	if err := f.SetSheetName("Sheet1", sheetSummary); err != nil {
		return fmt.Errorf("export: rename default sheet: %w", err)
	}
	if err := writeSummary(ctx, f, st, run); err != nil {
		return fmt.Errorf("export: summary: %w", err)
	}
	if err := writeScopes(ctx, f, st, run); err != nil {
		return fmt.Errorf("export: scopes: %w", err)
	}
	for _, ks := range kindSheets {
		if err := writeKindSheet(ctx, f, st, run, ks.kind, ks.sheet); err != nil {
			return fmt.Errorf("export: %s: %w", ks.sheet, err)
		}
	}
	if err := writeEdges(ctx, f, st, run); err != nil {
		return fmt.Errorf("export: edges: %w", err)
	}

	// Active tab back to Summary.
	if idx, err := f.GetSheetIndex(sheetSummary); err == nil {
		f.SetActiveSheet(idx)
	}

	if err := f.SaveAs(outPath); err != nil {
		return fmt.Errorf("export: save %s: %w", outPath, err)
	}
	return nil
}

// DefaultPath returns ~/.cloudcmder/exports/<scopeID>-<shortUUID>.xlsx.
// Used by the TUI's `e` handler so the user doesn't have to type a path.
func DefaultPath(run store.RunSummary) string {
	short := run.UUID
	if len(short) > 8 {
		short = short[:8]
	}
	name := fmt.Sprintf("%s-%s.xlsx", run.ScopeID, short)
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Clean(filepath.Join(".cloudcmder", "exports", name))
	}
	return filepath.Clean(filepath.Join(home, ".cloudcmder", "exports", name))
}

// --- writers ---------------------------------------------------------------

func writeSummary(ctx context.Context, f *excelize.File, st *store.Store, run store.RunSummary) error {
	rows := [][]any{
		{"Field", "Value"},
		{"UUID", run.UUID},
		{"Provider", run.Provider},
		{"ScopeID", run.ScopeID},
		{"ScopeName", run.ScopeName},
		{"Status", run.Status},
		{"StartedAt", run.StartedAt.Format(time.RFC3339)},
	}
	finished := ""
	if run.FinishedAt != nil {
		finished = run.FinishedAt.Format(time.RFC3339)
	}
	rows = append(rows, []any{"FinishedAt", finished})
	rows = append(rows, []any{"CloudcmderVersion", run.CloudcmderV})
	rows = append(rows, []any{"Notes", run.Notes})
	rows = append(rows, []any{"", ""})
	rows = append(rows, []any{"Kind", "Count"})

	counts, err := st.CountResourcesByKind(ctx, run.ID)
	if err != nil {
		return err
	}
	for _, ks := range kindSheets {
		rows = append(rows, []any{string(ks.kind), counts[ks.kind]})
	}

	for i, r := range rows {
		for j, v := range r {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
			if err := f.SetCellValue(sheetSummary, cell, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeScopes(ctx context.Context, f *excelize.File, st *store.Store, run store.RunSummary) error {
	if _, err := f.NewSheet(sheetScopes); err != nil {
		return err
	}
	headers := []any{"ScopeID", "DisplayName", "Parent", "Labels"}
	for j, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(j+1, 1)
		if err := f.SetCellValue(sheetScopes, cell, h); err != nil {
			return err
		}
	}
	scopes, err := st.LoadScopes(ctx, run.ID)
	if err != nil {
		return err
	}
	for i, s := range scopes {
		row := []any{s.ID, s.DisplayName, s.Parent, joinLabels(s.Labels)}
		for j, v := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			if err := f.SetCellValue(sheetScopes, cell, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeKindSheet uses a StreamWriter so resource sheets stay in constant
// memory regardless of row count. excelize's StreamWriter is strict that
// rows are written top-to-bottom in order — which is what LoadResources
// gives us via SQL ORDER BY kind, name.
func writeKindSheet(ctx context.Context, f *excelize.File, st *store.Store, run store.RunSummary, kind inventory.Kind, sheet string) error {
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

	// Header row.
	headers := make([]any, len(cols))
	for i, c := range cols {
		headers[i] = c.Header
	}
	if err := sw.SetRow("A1", headers); err != nil {
		return err
	}

	// Data rows.
	resources, err := st.LoadResources(ctx, run.ID, kind)
	if err != nil {
		return err
	}
	for i, res := range resources {
		raw, _ := res.Detail.(json.RawMessage)
		detail := decodeDetail(kind, raw)
		row := make([]any, len(cols))
		for j, c := range cols {
			row[j] = c.Extract(res, detail)
		}
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		if err := sw.SetRow(cell, row); err != nil {
			return err
		}
	}
	return sw.Flush()
}

func writeEdges(ctx context.Context, f *excelize.File, st *store.Store, run store.RunSummary) error {
	if _, err := f.NewSheet(sheetEdges); err != nil {
		return err
	}
	sw, err := f.NewStreamWriter(sheetEdges)
	if err != nil {
		return err
	}
	if err := sw.SetRow("A1", []any{"FromRef", "RefKind", "ToRef"}); err != nil {
		return err
	}
	edges, err := st.LoadEdges(ctx, run.ID)
	if err != nil {
		return err
	}
	for i, e := range edges {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		if err := sw.SetRow(cell, []any{e.FromRef, string(e.RefKind), e.ToRef}); err != nil {
			return err
		}
	}
	return sw.Flush()
}
