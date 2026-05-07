// Package store persists discovery runs to a single SQLite file.
// Schema and lifecycle are documented in architecture.md §"SQLite Store".
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"cloudcmder.com/internal/inventory"
)

// batchSize is the row count per WriteBatch transaction. Architecture sets
// this at 500; smaller batches dilute throughput, larger batches risk SQLITE
// transaction size warnings on resource-heavy projects.
const batchSize = 500

// Store wraps a SQLite database with cloudcmder's schema applied.
type Store struct {
	db *sql.DB
}

// Open returns a Store backed by the file at path, creating its parent
// directory and applying any pending migrations. ":memory:" is special-cased
// so tests can run hermetically.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("store: mkdir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	for _, p := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.ExecContext(context.Background(), p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("store: pragma %q: %w", p, err)
		}
	}
	if err := migrate(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// OpenRun records a new run row in status='running' and an accompanying scopes
// row. Returns the autoincrement ID and the generated UUID.
func (s *Store) OpenRun(ctx context.Context, provider, scopeID, scopeName, version string) (int64, string, error) {
	runUUID := uuid.NewString()
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", fmt.Errorf("store: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO runs (uuid, provider, scope_id, scope_name, started_at, status, cloudcmder_v)
		 VALUES (?, ?, ?, ?, ?, 'running', ?)`,
		runUUID, provider, scopeID, scopeName, startedAt, version)
	if err != nil {
		return 0, "", fmt.Errorf("store: insert run: %w", err)
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, "", fmt.Errorf("store: last insert id: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO scopes (run_id, scope_id, display_name) VALUES (?, ?, ?)`,
		runID, scopeID, scopeName,
	); err != nil {
		return 0, "", fmt.Errorf("store: insert scope: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, "", fmt.Errorf("store: commit run: %w", err)
	}
	return runID, runUUID, nil
}

// FinishRun stamps the run with finished_at, status, and notes. Status must be
// one of "ok", "partial", or "failed"; "running" is reserved for in-flight rows.
func (s *Store) FinishRun(ctx context.Context, runID int64, status, notes string) error {
	finishedAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`UPDATE runs SET finished_at = ?, status = ?, notes = ? WHERE id = ?`,
		finishedAt, status, notes, runID)
	if err != nil {
		return fmt.Errorf("store: finish run: %w", err)
	}
	return nil
}

// WriteBatch inserts resources in chunks of 500, one transaction per chunk.
// Re-inserting the same ref within the same run replaces the prior row so
// callers can safely overwrite stubs with enriched detail.
func (s *Store) WriteBatch(ctx context.Context, runID int64, batch []inventory.Resource) error {
	for start := 0; start < len(batch); start += batchSize {
		end := start + batchSize
		if end > len(batch) {
			end = len(batch)
		}
		if err := s.writeChunk(ctx, runID, batch[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) writeChunk(ctx context.Context, runID int64, chunk []inventory.Resource) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin chunk: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	resStmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO resources
		 (run_id, ref, kind, scope_id, name, region, status, labels_json, detail_json, native_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("store: prepare insert: %w", err)
	}
	defer func() { _ = resStmt.Close() }()

	// edges go through their own prepared statement; INSERT OR IGNORE so that
	// re-emitting the same Resource with the same Refs is idempotent within a
	// run (the composite PK on edges is run_id+from_ref+ref_kind+to_ref).
	edgeStmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO edges (run_id, from_ref, ref_kind, to_ref) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("store: prepare edge insert: %w", err)
	}
	defer func() { _ = edgeStmt.Close() }()

	for i := range chunk {
		r := &chunk[i]
		labelsJSON, err := marshalLabels(r.Labels)
		if err != nil {
			return fmt.Errorf("store: marshal labels for %s: %w", r.Ref.String(), err)
		}
		detailJSON, err := marshalDetail(r.Detail)
		if err != nil {
			return fmt.Errorf("store: marshal detail for %s: %w", r.Ref.String(), err)
		}
		nativeJSON, err := marshalNative(r.Native)
		if err != nil {
			return fmt.Errorf("store: marshal native for %s: %w", r.Ref.String(), err)
		}

		if _, err := resStmt.ExecContext(ctx,
			runID,
			r.Ref.String(),
			string(r.Kind),
			r.Ref.ScopeID,
			r.Name,
			r.Region,
			r.Status,
			labelsJSON,
			detailJSON,
			nativeJSON,
		); err != nil {
			return fmt.Errorf("store: insert resource %s: %w", r.Ref.String(), err)
		}

		from := r.Ref.String()
		for refKind, targets := range r.Refs {
			for _, t := range targets {
				if _, err := edgeStmt.ExecContext(ctx,
					runID, from, string(refKind), t.String(),
				); err != nil {
					return fmt.Errorf("store: insert edge %s -%s-> %s: %w",
						from, refKind, t.String(), err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit chunk: %w", err)
	}
	return nil
}

func marshalLabels(m map[string]string) (string, error) {
	if len(m) == 0 {
		return "", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func marshalDetail(d any) (string, error) {
	if d == nil {
		return "{}", nil
	}
	b, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func marshalNative(n any) (sql.NullString, error) {
	if n == nil {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(n)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}
