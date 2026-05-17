package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// runExportAll bundles the SQLite DB (+ WAL/SHM if present), log, and the
// most recent Excel export into a timestamped zip placed next to the binary.
func runExportAll(cmd *cobra.Command, dbPath string) error {
	outDir := binaryDir()

	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("--export-all: database %s: %w; run --scan first", dbPath, err)
	}

	dbDir := filepath.Dir(dbPath)
	bundleName := "cloudcmder-bundle-" + time.Now().UTC().Format("20060102-150405")
	outPath := filepath.Join(outDir, bundleName+".zip")

	entries := collectBundleEntries(dbPath, dbDir, bundleName)

	if err := writeZip(outPath, entries); err != nil {
		return fmt.Errorf("--export-all: write zip: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d file(s))\n", outPath, len(entries))
	return nil
}

type bundleEntry struct {
	zipPath string
	srcPath string
}

func collectBundleEntries(dbPath, dbDir, bundleName string) []bundleEntry {
	out := []bundleEntry{
		{bundleName + "/cloudcmder.db", dbPath},
	}
	for _, ext := range []string{"-wal", "-shm"} {
		if p := dbPath + ext; fileExists(p) {
			out = append(out, bundleEntry{bundleName + "/cloudcmder.db" + ext, p})
		}
	}
	if logPath := defaultLogPath(); fileExists(logPath) {
		out = append(out, bundleEntry{bundleName + "/cloudcmder.log", logPath})
	}
	// Search the default exports dir AND the binary's directory so that
	// relative-path --export / --export-multi outputs are found too.
	if xl := newestXLSXAny(filepath.Join(dbDir, "exports"), binaryDir()); xl != "" {
		out = append(out, bundleEntry{bundleName + "/exports/" + filepath.Base(xl), xl})
	}
	return out
}

// newestXLSXAny returns the path of the most recently modified .xlsx file
// found across all supplied directories. Returns "" if none exist.
func newestXLSXAny(dirs ...string) string {
	seen := make(map[string]bool)
	var best string
	var bestMod int64
	for _, dir := range dirs {
		if seen[dir] {
			continue
		}
		seen[dir] = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".xlsx" {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().UnixNano() > bestMod {
				bestMod = info.ModTime().UnixNano()
				best = filepath.Join(dir, e.Name())
			}
		}
	}
	return best
}

// newestXLSX is kept for test compatibility.
func newestXLSX(dir string) string { return newestXLSXAny(dir) }

func writeZip(outPath string, entries []bundleEntry) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		if err := addToZip(zw, e.zipPath, e.srcPath); err != nil {
			_ = zw.Close()
			_ = f.Close()
			return fmt.Errorf("add %s: %w", e.srcPath, err)
		}
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func addToZip(zw *zip.Writer, zipPath, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = zipPath
	hdr.Method = zip.Deflate

	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}

// binaryDir returns the directory containing the running binary, resolving
// symlinks so the zip lands next to the real binary rather than the link.
func binaryDir() string {
	exe, err := os.Executable()
	if err == nil {
		if resolved, err2 := filepath.EvalSymlinks(exe); err2 == nil {
			return filepath.Dir(resolved)
		}
		return filepath.Dir(exe)
	}
	slog.Warn("export-all: os.Executable failed; falling back to cwd", "err", err)
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
