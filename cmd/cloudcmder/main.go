// cloudcmder — interactive cloud asset inventory TUI.
// Run inside GCP CloudShell (or any environment with Application Default Credentials)
// to browse and export your cloud estate without copying keys to a laptop.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"cloudcmder.com/internal/providers/gcp"
	"cloudcmder.com/internal/version"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		dbPath     string
		logLevel   string
		listScopes bool
	)

	root := &cobra.Command{
		Use:   "cloudcmder",
		Short: "Interactive cloud asset inventory TUI",
		Long: `cloudcmder (cloud commander) is an interactive TUI for inventorying
GCP resources. Run it inside GCP CloudShell with no additional setup — it uses
your existing credentials automatically.

Assessment data is stored in a local SQLite file so scans survive interruptions
and the file can be exported for offline analysis.`,
		// SilenceUsage prevents cobra from printing usage on every error.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if listScopes {
				return runListScopes(cmd)
			}
			fmt.Fprintln(os.Stderr, "TUI not yet implemented — use --help to see available flags")
			return nil
		},
	}

	root.PersistentFlags().StringVar(&dbPath, "db",
		defaultDBPath(), "path to the SQLite assessment database")
	root.PersistentFlags().StringVar(&logLevel, "log-level",
		"info", "log level: debug, info, warn, error (written to ~/.cloudcmder/cloudcmder.log)")
	root.Flags().BoolVar(&listScopes, "list-scopes", false,
		"list all GCP projects accessible to the current credentials and exit (JSON output)")

	// --version is handled by cobra's built-in Version field so it prints cleanly.
	root.Version = version.String()
	root.SetVersionTemplate("{{.Version}}\n")

	return root
}

// runListScopes prints every project the caller can see as a JSON array.
// Output goes through cmd.OutOrStdout so tests can capture it.
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

// defaultDBPath returns the default location for the SQLite assessment database.
// Uses $HOME so it works inside CloudShell where the home directory is ephemeral
// but consistent within a session.
func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cloudcmder/cloudcmder.db"
	}
	return home + "/.cloudcmder/cloudcmder.db"
}
