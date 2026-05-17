package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewestXLSX(t *testing.T) {
	t.Run("empty dir returns empty", func(t *testing.T) {
		d := t.TempDir()
		assert.Equal(t, "", newestXLSX(d))
	})

	t.Run("missing dir returns empty", func(t *testing.T) {
		assert.Equal(t, "", newestXLSX("/no/such/dir/xyz"))
	})

	t.Run("single xlsx returned", func(t *testing.T) {
		d := t.TempDir()
		p := filepath.Join(d, "scope-abc.xlsx")
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
		assert.Equal(t, p, newestXLSX(d))
	})

	t.Run("returns newest by mtime", func(t *testing.T) {
		d := t.TempDir()
		old := filepath.Join(d, "old.xlsx")
		mid := filepath.Join(d, "mid.xlsx")
		newest := filepath.Join(d, "newest.xlsx")
		require.NoError(t, os.WriteFile(old, []byte("a"), 0o644))
		require.NoError(t, os.WriteFile(mid, []byte("b"), 0o644))
		require.NoError(t, os.WriteFile(newest, []byte("c"), 0o644))

		now := time.Now()
		require.NoError(t, os.Chtimes(old, now.Add(-2*time.Hour), now.Add(-2*time.Hour)))
		require.NoError(t, os.Chtimes(mid, now.Add(-1*time.Hour), now.Add(-1*time.Hour)))
		require.NoError(t, os.Chtimes(newest, now, now))

		assert.Equal(t, newest, newestXLSX(d))
	})

	t.Run("ignores non-xlsx files", func(t *testing.T) {
		d := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(d, "report.csv"), []byte("x"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(d, "notes.txt"), []byte("x"), 0o644))
		assert.Equal(t, "", newestXLSX(d))
	})
}

func TestCollectBundleEntries(t *testing.T) {
	base := t.TempDir()
	dbPath := filepath.Join(base, "cloudcmder.db")
	exportsDir := filepath.Join(base, "exports")
	require.NoError(t, os.MkdirAll(exportsDir, 0o755))

	require.NoError(t, os.WriteFile(dbPath, []byte("db"), 0o644))
	require.NoError(t, os.WriteFile(dbPath+"-wal", []byte("wal"), 0o644))
	require.NoError(t, os.WriteFile(dbPath+"-shm", []byte("shm"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(exportsDir, "scope-abc.xlsx"), []byte("xl"), 0o644))

	const bundle = "cloudcmder-bundle-test"
	entries := collectBundleEntries(dbPath, base, bundle)

	zipPaths := make([]string, len(entries))
	for i, e := range entries {
		zipPaths[i] = e.zipPath
	}

	assert.Contains(t, zipPaths, bundle+"/cloudcmder.db")
	assert.Contains(t, zipPaths, bundle+"/cloudcmder.db-wal")
	assert.Contains(t, zipPaths, bundle+"/cloudcmder.db-shm")
	assert.Contains(t, zipPaths, bundle+"/exports/scope-abc.xlsx")
}

func TestCollectBundleEntriesOptionalMissing(t *testing.T) {
	base := t.TempDir()
	dbPath := filepath.Join(base, "cloudcmder.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("db"), 0o644))

	const bundle = "cloudcmder-bundle-test"
	entries := collectBundleEntries(dbPath, base, bundle)

	zipPaths := make([]string, len(entries))
	for i, e := range entries {
		zipPaths[i] = e.zipPath
	}
	assert.Contains(t, zipPaths, bundle+"/cloudcmder.db")
	for _, zp := range zipPaths {
		assert.False(t, strings.HasSuffix(zp, "-wal"), "unexpected wal entry: %s", zp)
		assert.False(t, strings.HasSuffix(zp, "-shm"), "unexpected shm entry: %s", zp)
		assert.False(t, strings.Contains(zp, ".xlsx"), "unexpected xlsx entry: %s", zp)
	}
}

func TestWriteZip(t *testing.T) {
	base := t.TempDir()
	src1 := filepath.Join(base, "a.txt")
	src2 := filepath.Join(base, "b.txt")
	require.NoError(t, os.WriteFile(src1, []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(src2, []byte("world"), 0o644))

	outPath := filepath.Join(base, "out.zip")
	entries := []bundleEntry{
		{"prefix/a.txt", src1},
		{"prefix/b.txt", src2},
	}
	require.NoError(t, writeZip(outPath, entries))

	r, err := zip.OpenReader(outPath)
	require.NoError(t, err)
	defer r.Close()

	names := make(map[string]bool, len(r.File))
	for _, f := range r.File {
		names[f.Name] = true
	}
	assert.True(t, names["prefix/a.txt"])
	assert.True(t, names["prefix/b.txt"])
}

func TestRunExportAllMissingDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "no-such.db")
	err := runExportAll(nil, dbPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--export-all")
}
