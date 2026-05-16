// cloudcmder — interactive cloud asset inventory TUI.
// Run inside CloudShell (or any environment with Application Default Credentials)
// to browse and export your cloud estate without copying keys to a laptop.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"cloudcmder.com/internal/export"
	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/providers/gcp"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui"
	"cloudcmder.com/internal/version"
)

// scanBatchSize is the channel-drain batch size before flushing to the store.
// Smaller than store.batchSize so a Ctrl-C during a slow Asset listing still
// commits recent rows rather than holding them in memory.
const scanBatchSize = 200

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		dbPath       string
		logLevel     string
		providerName string
		checkFlag    bool
		checkProject string
		listScopes   bool
		scanProject  string
		scanAll      bool
		scanProjects string
		scanFailFast bool
		listRunsFlag bool
		showRunUUID  string
		exportPath   string
		exportRunID  string
		exportMulti  string
		exportRunIDs string
		exportScopes string
		dumpNative   bool
		singleView   bool
	)

	root := &cobra.Command{
		Use:   "cloudcmder",
		Short: "Interactive cloud asset inventory TUI",
		Long: `cloudcmder (cloud commander) is an interactive TUI for inventorying
cloud resources. Run it inside CloudShell with no additional setup — it uses
your existing credentials automatically.

Assessment data is stored in a local SQLite file so scans survive interruptions
and the file can be exported for offline analysis.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return setupLogging(logLevel)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case checkFlag:
				return runCheck(cmd, providerName, checkProject)
			case listScopes:
				return runListScopes(cmd, providerName)
			case scanAll:
				return runScanMany(cmd, dbPath, providerName, "", dumpNative, scanFailFast)
			case scanProjects != "":
				return runScanMany(cmd, dbPath, providerName, scanProjects, dumpNative, scanFailFast)
			case scanProject != "":
				return runScan(cmd, dbPath, providerName, scanProject, dumpNative)
			case listRunsFlag:
				return runListRuns(cmd, dbPath)
			case showRunUUID != "":
				return runShowRun(cmd, dbPath, showRunUUID)
			case exportMulti != "":
				return runExportMulti(cmd, dbPath, exportMulti, exportRunIDs, exportScopes)
			case exportPath != "":
				return runExport(cmd, dbPath, exportPath, exportRunID)
			default:
				return runTUI(cmd, dbPath, singleView)
			}
		},
	}

	root.PersistentFlags().StringVar(&dbPath, "db",
		defaultDBPath(), "path to the SQLite assessment database")
	root.PersistentFlags().StringVar(&logLevel, "log-level",
		"info", "log level: debug, info, warn, error (written to ~/.cloudcmder/cloudcmder.log)")
	root.PersistentFlags().StringVar(&providerName, "provider",
		"gcp", "cloud provider to use: gcp, aws")
	root.PersistentFlags().BoolVar(&singleView, "single-view", false,
		"open the alternative single-screen 4-pane TUI layout (only meaningful for the default TUI action)")

	root.Flags().BoolVar(&checkFlag, "check", false,
		"check that required APIs are enabled; prints missing ones and the command to enable them (read-only; GCP only)")
	root.Flags().StringVar(&checkProject, "project", "",
		"limit --check to a single scope ID (default: all accessible scopes)")
	root.Flags().BoolVar(&listScopes, "list-scopes", false,
		"list all scopes accessible to the current credentials and exit (JSON output)")
	root.Flags().StringVar(&scanProject, "scan", "",
		"discover all resources in the given scope and write them to the store")
	root.Flags().StringVar(&scanProject, "scope", "",
		"alias for --scan")
	root.Flags().BoolVar(&scanAll, "scan-all", false,
		"scan every accessible scope sequentially (one run per scope)")
	root.Flags().StringVar(&scanProjects, "scan-projects", "",
		"scan a comma-separated list of scope IDs sequentially")
	root.Flags().StringVar(&scanProjects, "scan-scopes", "",
		"alias for --scan-projects")
	root.Flags().BoolVar(&scanFailFast, "fail-fast", false,
		"abort --scan-all/--scan-projects on the first project that errors (default: continue)")
	root.Flags().BoolVar(&listRunsFlag, "list-runs", false,
		"list every run recorded in the store as a table")
	root.Flags().StringVar(&showRunUUID, "show-run", "",
		"print resource counts grouped by kind for the run with the given uuid")
	root.Flags().StringVar(&exportPath, "export", "",
		"write a multi-tab Excel workbook for a stored run to the given path")
	root.Flags().StringVar(&exportRunID, "run", "",
		"run uuid to export (with --export); defaults to the most recent run")
	root.Flags().StringVar(&exportMulti, "export-multi", "",
		"write a combined multi-project workbook to the given path")
	root.Flags().StringVar(&exportRunIDs, "runs", "",
		"comma-separated run UUIDs to include in --export-multi")
	root.Flags().StringVar(&exportScopes, "scopes", "",
		"comma-separated scope IDs for --export-multi (uses latest run per scope)")
	root.Flags().BoolVar(&dumpNative, "dump-native", false,
		"store raw GCP API payloads in native_json (off by default; roughly doubles DB size)")

	var versionFlag bool
	root.Flags().BoolVarP(&versionFlag, "version", "v", false, "Print version banner and exit")

	root.AddCommand(newVersionCmd(), newAboutCmd(), newSupportCmd())

	// Promote --version flag to print the full banner (same output as `cloudcmder version`).
	origRunE := root.RunE
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if versionFlag {
			renderBanner(cmd.OutOrStdout())
			return nil
		}
		return origRunE(cmd, args)
	}

	return root
}

// newProvider returns the inventory.Provider for the given provider name.
// Only "gcp" is fully implemented; "aws" returns a stub error until v2.
func newProvider(ctx context.Context, name string) (inventory.Provider, error) {
	switch name {
	case "gcp":
		return gcp.New(ctx)
	case "aws":
		return nil, fmt.Errorf("aws provider not yet implemented — coming in v2")
	default:
		return nil, fmt.Errorf("unknown provider %q — supported: gcp, aws", name)
	}
}

// runListScopes prints every scope the caller can see as a JSON array.
func runListScopes(cmd *cobra.Command, providerName string) error {
	ctx := cmd.Context()
	p, err := newProvider(ctx, providerName)
	if err != nil {
		return err
	}
	defer func() { _ = p.Close() }()

	scopes, err := p.ListScopes(ctx)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(scopes)
}

// runCheck calls Service Usage to diff required vs enabled APIs per scope
// and prints a copy-paste-ready enable command for any that are not yet
// enabled. Exits non-zero when any are missing — composable with &&.
// Currently only supported for the GCP provider.
func runCheck(cmd *cobra.Command, providerName, scopeFilter string) error {
	ctx := cmd.Context()
	p, err := newProvider(ctx, providerName)
	if err != nil {
		return err
	}
	defer func() { _ = p.Close() }()

	gp, ok := p.(*gcp.GCPProvider)
	if !ok {
		return fmt.Errorf("--check is only supported for --provider gcp")
	}

	scopes, err := p.ListScopes(ctx)
	if err != nil {
		return fmt.Errorf("preflight: list scopes: %w", err)
	}
	if scopeFilter != "" {
		var filtered []inventory.Scope
		for _, s := range scopes {
			if s.ID == scopeFilter {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("preflight: scope %q not in accessible scopes", scopeFilter)
		}
		scopes = filtered
	}

	w := cmd.OutOrStdout()
	var totalMissing int
	for _, scope := range scopes {
		r, err := gp.Preflight(ctx, scope)
		if err != nil {
			fmt.Fprintf(w, "Scope: %s\n  ERROR: %v\n\n", scope.ID, err)
			continue
		}
		enabledOfRequired := len(r.Required) - len(r.Missing)
		if len(r.Missing) == 0 {
			fmt.Fprintf(w, "Scope: %s\n  All %d required APIs enabled. ✓\n\n", scope.ID, len(r.Required))
		} else {
			fmt.Fprintf(w, "Scope: %s\n  %d of %d required APIs enabled — %d not enabled:\n",
				scope.ID, enabledOfRequired, len(r.Required), len(r.Missing))
			for _, m := range r.Missing {
				fmt.Fprintf(w, "    - %s\n", m)
			}
			fmt.Fprintf(w, "\n  Run this to enable them:\n    %s\n\n", r.GcloudEnableCommand())
		}
		totalMissing += len(r.Missing)
	}
	if totalMissing > 0 {
		return fmt.Errorf("preflight: %d required API(s) not enabled — enable them and re-run --check",
			totalMissing)
	}
	return nil
}

// runScan opens the store, opens a run, and streams provider results
// into 200-row WriteBatch calls. On Ctrl-C the run row stays at status='running'
// with whatever rows the chunked transactions had committed; that's the
// crash-safety contract from architecture.md.
func runScan(cmd *cobra.Command, dbPath, providerName, scopeID string, dumpNative bool) error {
	ctx := cmd.Context()

	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	p, err := newProvider(ctx, providerName)
	if err != nil {
		return err
	}
	defer func() { _ = p.Close() }()
	if d, ok := p.(interface{ SetDumpNative(bool) }); ok {
		d.SetDumpNative(dumpNative)
	}

	runID, runUUID, err := st.OpenRun(ctx, p.Name(), scopeID, scopeID, version.Version)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", runUUID)

	ch, err := p.ListResources(ctx, inventory.Scope{ProviderID: p.Name(), ID: scopeID}, nil)
	if err != nil {
		_ = st.FinishRun(context.Background(), runID, "failed", err.Error())
		return err
	}

	scanErr := drainAndWrite(ctx, st, runID, ch)
	switch {
	case errors.Is(scanErr, context.Canceled):
		// Leave status='running' so a future "resume" feature can pick it up.
		// The verify step `kill -INT` relies on this exact behavior.
		return scanErr
	case scanErr != nil:
		_ = st.FinishRun(context.Background(), runID, "failed", scanErr.Error())
		return scanErr
	default:
		return st.FinishRun(ctx, runID, "ok", "")
	}
}

func drainAndWrite(ctx context.Context, st *store.Store, runID int64, ch <-chan inventory.ResourceOrErr) error {
	batch := make([]inventory.Resource, 0, scanBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := st.WriteBatch(ctx, runID, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			_ = flush()
			return ctx.Err()
		case x, ok := <-ch:
			if !ok {
				return flush()
			}
			if x.Err != nil {
				if gcp.IsRecoverableScanErr(x.Err) {
					// Architecture line 543: skip the kind, keep scanning.
					// The most common trigger is a GCP API not being
					// enabled on the target project.
					slog.Warn("scan: kind enrichment skipped",
						"error", x.Err)
					continue
				}
				return x.Err
			}
			batch = append(batch, x.Resource)
			if len(batch) >= scanBatchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
}

// runListRuns prints all runs as a fixed-width table.
func runListRuns(cmd *cobra.Command, dbPath string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	runs, err := st.ListRuns(cmd.Context())
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "UUID\tSCOPE\tSTARTED\tSTATUS")
	for _, r := range runs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			r.UUID, r.ScopeID, r.StartedAt.Local().Format(time.RFC3339), r.Status)
	}
	return tw.Flush()
}

// runShowRun prints kind/count rows for a given run.
func runShowRun(cmd *cobra.Command, dbPath, runUUID string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	run, err := st.FindRunByUUID(cmd.Context(), runUUID)
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("no run found with uuid %s", runUUID)
	}
	counts, err := st.CountResourcesByKind(cmd.Context(), run.ID)
	if err != nil {
		return err
	}

	kinds := make([]string, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, string(k))
	}
	sort.Slice(kinds, func(i, j int) bool { return counts[inventory.Kind(kinds[i])] > counts[inventory.Kind(kinds[j])] })

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tCOUNT")
	for _, k := range kinds {
		fmt.Fprintf(tw, "%s\t%d\n", k, counts[inventory.Kind(k)])
	}
	return tw.Flush()
}

// runExport writes a multi-tab Excel workbook for a run from the store.
// If runUUID is empty, the most recent run across all scopes is used.
func runExport(cmd *cobra.Command, dbPath, outPath, runUUID string) error {
	ctx := cmd.Context()

	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	var run *store.RunSummary
	if runUUID != "" {
		run, err = st.FindRunByUUID(ctx, runUUID)
		if err != nil {
			return err
		}
		if run == nil {
			return fmt.Errorf("no run found with uuid %s", runUUID)
		}
	} else {
		runs, err := st.ListRuns(ctx)
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			return errors.New("no runs in store — run --scan first")
		}
		run = &runs[0] // ListRuns is newest-first.
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("export: create parent dir: %w", err)
	}
	if err := export.WriteWorkbook(ctx, st, *run, outPath); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", outPath)
	return nil
}

// runScanMany scans each scope in scopesCSV (comma-separated) sequentially,
// or every accessible scope when scopesCSV is empty. Each scope gets its
// own run row. On per-scope error: logs a warning and continues unless
// failFast is true. Returns non-zero when any scope failed.
func runScanMany(cmd *cobra.Command, dbPath, providerName, scopesCSV string, dumpNative, failFast bool) error {
	ctx := cmd.Context()

	p, err := newProvider(ctx, providerName)
	if err != nil {
		return err
	}
	defer func() { _ = p.Close() }()

	var scopes []inventory.Scope
	if scopesCSV != "" {
		for _, id := range strings.Split(scopesCSV, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			scopes = append(scopes, inventory.Scope{ProviderID: p.Name(), ID: id, DisplayName: id})
		}
	} else {
		scopes, err = p.ListScopes(ctx)
		if err != nil {
			return fmt.Errorf("scan-all: list scopes: %w", err)
		}
	}
	if len(scopes) == 0 {
		return errors.New("scan-all: no scopes to scan")
	}

	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if d, ok := p.(interface{ SetDumpNative(bool) }); ok {
		d.SetDumpNative(dumpNative)
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "scanning %d scope(s)…\n", len(scopes))

	var failed []string
	for i, scope := range scopes {
		fmt.Fprintf(w, "[%d/%d] %s … ", i+1, len(scopes), scope.ID)

		runID, runUUID, err := st.OpenRun(ctx, p.Name(), scope.ID, scope.DisplayName, version.Version)
		if err != nil {
			fmt.Fprintf(w, "open: %v\n", err)
			slog.Warn("scan-many: open run failed", "scope", scope.ID, "error", err)
			failed = append(failed, scope.ID)
			if failFast {
				return err
			}
			continue
		}

		ch, err := p.ListResources(ctx, scope, nil)
		if err != nil {
			_ = st.FinishRun(context.Background(), runID, "failed", err.Error())
			fmt.Fprintf(w, "list: %v\n", err)
			slog.Warn("scan-many: list resources failed", "scope", scope.ID, "error", err)
			failed = append(failed, scope.ID)
			if failFast {
				return err
			}
			continue
		}

		scanErr := drainAndWrite(ctx, st, runID, ch)
		switch {
		case errors.Is(scanErr, context.Canceled):
			// Leave status='running' per crash-safety contract.
			return scanErr
		case scanErr != nil:
			_ = st.FinishRun(context.Background(), runID, "failed", scanErr.Error())
			fmt.Fprintf(w, "scan: %v\n", scanErr)
			slog.Warn("scan-many: drain failed", "scope", scope.ID, "error", scanErr)
			failed = append(failed, scope.ID)
			if failFast {
				return scanErr
			}
			continue
		}

		_ = st.FinishRun(ctx, runID, "ok", "")
		fmt.Fprintf(w, "ok (run %s)\n", runUUID)
	}

	if len(failed) > 0 {
		return fmt.Errorf("scan-all: %d/%d scope(s) failed: %s",
			len(failed), len(scopes), strings.Join(failed, ", "))
	}
	return nil
}

// runExportMulti writes a combined multi-project workbook. Runs are selected
// by --runs (UUIDs), --scopes (latest run per scope), or neither (latest run
// per every scope that has at least one run).
func runExportMulti(cmd *cobra.Command, dbPath, outPath, runsCSV, scopesCSV string) error {
	ctx := cmd.Context()

	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	runs, err := resolveRunsForExport(ctx, st, runsCSV, scopesCSV)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return errors.New("export-multi: no runs found — run --scan-all or --scan first")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("export-multi: create parent dir: %w", err)
	}
	if err := export.WriteMultiWorkbook(ctx, st, runs, outPath); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d scope(s))\n", outPath, len(runs))
	return nil
}

// resolveRunsForExport builds the run slice, in precedence order:
//  1. --runs UUID,UUID  → FindRunByUUID per entry
//  2. --scopes A,B,C   → LatestRunForScope per id
//  3. neither          → LatestRunForScope for every distinct scope_id in ListRuns
func resolveRunsForExport(ctx context.Context, st *store.Store, runsCSV, scopesCSV string) ([]store.RunSummary, error) {
	if runsCSV != "" {
		var out []store.RunSummary
		for _, u := range strings.Split(runsCSV, ",") {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			r, err := st.FindRunByUUID(ctx, u)
			if err != nil {
				return nil, fmt.Errorf("export-multi: resolve run %s: %w", u, err)
			}
			if r == nil {
				return nil, fmt.Errorf("export-multi: run not found: %s", u)
			}
			out = append(out, *r)
		}
		return out, nil
	}

	if scopesCSV != "" {
		var out []store.RunSummary
		for _, id := range strings.Split(scopesCSV, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			r, err := st.LatestRunForScope(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("export-multi: latest run for scope %s: %w", id, err)
			}
			if r == nil {
				slog.Warn("export-multi: no run found for scope", "scope", id)
				continue
			}
			out = append(out, *r)
		}
		return out, nil
	}

	// Default: latest run per distinct scope_id across all runs.
	allRuns, err := st.ListRuns(ctx)
	if err != nil {
		return nil, fmt.Errorf("export-multi: list runs: %w", err)
	}
	seen := make(map[string]bool)
	var out []store.RunSummary
	for _, r := range allRuns {
		if seen[r.ScopeID] {
			continue // ListRuns is newest-first; first seen = latest
		}
		seen[r.ScopeID] = true
		out = append(out, r)
	}
	return out, nil
}

// runTUI opens the store and hands it to the Bubble Tea app. The TUI never
// imports providers/* — it reads only from the store, per architecture.md.
// singleView selects the alternative 4-pane single-screen layout.
func runTUI(cmd *cobra.Command, dbPath string, singleView bool) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if singleView {
		return tui.RunSingleView(cmd.Context(), st)
	}
	return tui.Run(cmd.Context(), st)
}

// setupLogging routes slog to ~/.cloudcmder/cloudcmder.log so debug output
// never corrupts the alt-screen TUI. Falls back to stderr only if the file
// cannot be opened (e.g., read-only home directory).
func setupLogging(level string) error {
	lvl := parseLevel(level)
	logPath := defaultLogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// Last-resort fallback so a disk-full doesn't break --version etc.
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
		return nil
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: lvl})))
	return nil
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// defaultDBPath returns the default location for the SQLite assessment database.
func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cloudcmder/cloudcmder.db"
	}
	return home + "/.cloudcmder/cloudcmder.db"
}

func defaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cloudcmder/cloudcmder.log"
	}
	return home + "/.cloudcmder/cloudcmder.log"
}
