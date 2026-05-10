// cloudcmder — interactive cloud asset inventory TUI.
// Run inside GCP CloudShell (or any environment with Application Default Credentials)
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
		checkFlag    bool
		checkProject string
		listScopes   bool
		scanProject  string
		listRunsFlag bool
		showRunUUID  string
		exportPath   string
		exportRunID  string
		dumpNative   bool
	)

	root := &cobra.Command{
		Use:   "cloudcmder",
		Short: "Interactive cloud asset inventory TUI",
		Long: `cloudcmder (cloud commander) is an interactive TUI for inventorying
GCP resources. Run it inside GCP CloudShell with no additional setup — it uses
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
				return runCheck(cmd, checkProject)
			case listScopes:
				return runListScopes(cmd)
			case scanProject != "":
				return runScan(cmd, dbPath, scanProject, dumpNative)
			case listRunsFlag:
				return runListRuns(cmd, dbPath)
			case showRunUUID != "":
				return runShowRun(cmd, dbPath, showRunUUID)
			case exportPath != "":
				return runExport(cmd, dbPath, exportPath, exportRunID)
			default:
				return runTUI(cmd, dbPath)
			}
		},
	}

	root.PersistentFlags().StringVar(&dbPath, "db",
		defaultDBPath(), "path to the SQLite assessment database")
	root.PersistentFlags().StringVar(&logLevel, "log-level",
		"info", "log level: debug, info, warn, error (written to ~/.cloudcmder/cloudcmder.log)")

	root.Flags().BoolVar(&checkFlag, "check", false,
		"check that required GCP APIs are enabled; prints missing ones and the gcloud command to enable them (read-only)")
	root.Flags().StringVar(&checkProject, "project", "",
		"limit --check to a single project ID (default: all accessible projects)")
	root.Flags().BoolVar(&listScopes, "list-scopes", false,
		"list all GCP projects accessible to the current credentials and exit (JSON output)")
	root.Flags().StringVar(&scanProject, "scan", "",
		"discover all resources in the given GCP project and write them to the store")
	root.Flags().BoolVar(&listRunsFlag, "list-runs", false,
		"list every run recorded in the store as a table")
	root.Flags().StringVar(&showRunUUID, "show-run", "",
		"print resource counts grouped by kind for the run with the given uuid")
	root.Flags().StringVar(&exportPath, "export", "",
		"write a multi-tab Excel workbook for a stored run to the given path")
	root.Flags().StringVar(&exportRunID, "run", "",
		"run uuid to export (with --export); defaults to the most recent run")
	root.Flags().BoolVar(&dumpNative, "dump-native", false,
		"store raw GCP API payloads in native_json (off by default; roughly doubles DB size)")

	root.Version = version.String()
	root.SetVersionTemplate("{{.Version}}\n")

	return root
}

// runListScopes prints every project the caller can see as a JSON array.
func runListScopes(cmd *cobra.Command) error {
	ctx := cmd.Context()
	p, err := gcp.New(ctx)
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

// runCheck calls Service Usage to diff required vs enabled APIs per project
// and prints a copy-paste-ready gcloud enable command for any that are missing.
// Exits with an error (non-zero) if any APIs are missing — composable with &&.
func runCheck(cmd *cobra.Command, projectFilter string) error {
	ctx := cmd.Context()
	p, err := gcp.New(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = p.Close() }()

	scopes, err := p.ListScopes(ctx)
	if err != nil {
		return fmt.Errorf("preflight: list scopes: %w", err)
	}
	if projectFilter != "" {
		var filtered []inventory.Scope
		for _, s := range scopes {
			if s.ID == projectFilter {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("preflight: project %q not in accessible scopes", projectFilter)
		}
		scopes = filtered
	}

	w := cmd.OutOrStdout()
	var totalMissing int
	for _, scope := range scopes {
		r, err := p.Preflight(ctx, scope)
		if err != nil {
			fmt.Fprintf(w, "Project: %s\n  ERROR: %v\n\n", scope.ID, err)
			continue
		}
		fmt.Fprintf(w, "Project: %s\n  Required: %d  Enabled: %d  Missing: %d\n",
			scope.ID, len(r.Required), len(r.Enabled), len(r.Missing))
		for _, m := range r.Missing {
			fmt.Fprintf(w, "    - %s\n", m)
		}
		if enable := r.GcloudEnableCommand(); enable != "" {
			fmt.Fprintf(w, "\n  To enable missing APIs:\n    %s\n", enable)
		}
		fmt.Fprintln(w)
		totalMissing += len(r.Missing)
	}
	if totalMissing > 0 {
		return fmt.Errorf("preflight: %d API(s) missing across %d project(s) — enable them and re-run --check",
			totalMissing, len(scopes))
	}
	return nil
}

// runScan opens the store, opens a run, and streams Asset Inventory results
// into 200-row WriteBatch calls. On Ctrl-C the run row stays at status='running'
// with whatever rows the chunked transactions had committed; that's the
// crash-safety contract from architecture.md.
func runScan(cmd *cobra.Command, dbPath, projectID string, dumpNative bool) error {
	ctx := cmd.Context()

	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	p, err := gcp.New(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = p.Close() }()
	p.SetDumpNative(dumpNative)

	runID, runUUID, err := st.OpenRun(ctx, "gcp", projectID, projectID, version.Version)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", runUUID)

	ch, err := p.ListResources(ctx, inventory.Scope{ProviderID: "gcp", ID: projectID}, nil)
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

// runTUI opens the store and hands it to the Bubble Tea app. The TUI never
// imports providers/* — it reads only from the store, per architecture.md.
func runTUI(cmd *cobra.Command, dbPath string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
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
