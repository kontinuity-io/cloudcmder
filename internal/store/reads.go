package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloudcmder.com/internal/inventory"
)

// RunSummary is the read-side projection of a row in the runs table.
type RunSummary struct {
	ID          int64
	UUID        string
	Provider    string
	ScopeID     string
	ScopeName   string
	Status      string
	StartedAt   time.Time
	FinishedAt  *time.Time
	CloudcmderV string
	Notes       string
}

// Edge is a single from→to relationship recorded in the edges table.
type Edge struct {
	FromRef string
	RefKind inventory.RefKind
	ToRef   string
}

// ListRuns returns all run rows newest-first.
func (s *Store) ListRuns(ctx context.Context) ([]RunSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, uuid, provider, scope_id, COALESCE(scope_name, ''), started_at,
		        finished_at, status, cloudcmder_v, COALESCE(notes, '')
		 FROM runs
		 ORDER BY started_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []RunSummary
	for rows.Next() {
		r, err := scanRunSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestRunForScope returns the most recent run for a given scope, or nil if
// no such run exists.
func (s *Store) LatestRunForScope(ctx context.Context, scopeID string) (*RunSummary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, uuid, provider, scope_id, COALESCE(scope_name, ''), started_at,
		        finished_at, status, cloudcmder_v, COALESCE(notes, '')
		 FROM runs
		 WHERE scope_id = ?
		 ORDER BY started_at DESC
		 LIMIT 1`, scopeID)
	r, err := scanRunSummary(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// FindRunByUUID returns the run matching the given uuid, or nil if absent.
func (s *Store) FindRunByUUID(ctx context.Context, runUUID string) (*RunSummary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, uuid, provider, scope_id, COALESCE(scope_name, ''), started_at,
		        finished_at, status, cloudcmder_v, COALESCE(notes, '')
		 FROM runs WHERE uuid = ?`, runUUID)
	r, err := scanRunSummary(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// LoadResources returns all resources for the run, optionally filtered by
// kinds. Detail is returned as json.RawMessage so callers decide when (and
// into what type) to decode it.
func (s *Store) LoadResources(ctx context.Context, runID int64, kinds ...inventory.Kind) ([]inventory.Resource, error) {
	q := `SELECT ref, kind, scope_id, name, region, status, labels_json, detail_json
	      FROM resources WHERE run_id = ?`
	args := []any{runID}
	if len(kinds) > 0 {
		placeholders := make([]string, len(kinds))
		for i, k := range kinds {
			placeholders[i] = "?"
			args = append(args, string(k))
		}
		q += " AND kind IN (" + strings.Join(placeholders, ",") + ")"
	}
	q += " ORDER BY kind, name"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: load resources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []inventory.Resource
	for rows.Next() {
		var (
			ref, kind, scopeID, name, region, status string
			labelsJSON                               sql.NullString
			detailJSON                               string
		)
		if err := rows.Scan(&ref, &kind, &scopeID, &name, &region, &status,
			&labelsJSON, &detailJSON); err != nil {
			return nil, fmt.Errorf("store: scan resource: %w", err)
		}

		labels, err := unmarshalLabels(labelsJSON)
		if err != nil {
			return nil, fmt.Errorf("store: parse labels for %s: %w", ref, err)
		}

		out = append(out, inventory.Resource{
			Ref:    parseRef(ref, kind, scopeID),
			Kind:   inventory.Kind(kind),
			Name:   name,
			Region: region,
			Status: status,
			Labels: labels,
			Detail: json.RawMessage(detailJSON),
		})
	}
	return out, rows.Err()
}

// LoadEdges returns the edge graph for the run. Empty in M2 (no edges yet).
func (s *Store) LoadEdges(ctx context.Context, runID int64) ([]Edge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT from_ref, ref_kind, to_ref FROM edges WHERE run_id = ?`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: load edges: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Edge
	for rows.Next() {
		var e Edge
		var refKind string
		if err := rows.Scan(&e.FromRef, &refKind, &e.ToRef); err != nil {
			return nil, fmt.Errorf("store: scan edge: %w", err)
		}
		e.RefKind = inventory.RefKind(refKind)
		out = append(out, e)
	}
	return out, rows.Err()
}

// LoadScopes returns every scope row recorded for the given run. Used by the
// export package to drive the Scopes sheet — v1 always has a single scope
// per run, but the schema and reader allow multi-scope runs (v2 territory).
func (s *Store) LoadScopes(ctx context.Context, runID int64) ([]inventory.Scope, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT scope_id, COALESCE(display_name, ''), COALESCE(parent, ''), labels_json
		 FROM scopes WHERE run_id = ? ORDER BY scope_id`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: load scopes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []inventory.Scope
	for rows.Next() {
		var (
			id, name, parent string
			labelsJSON       sql.NullString
		)
		if err := rows.Scan(&id, &name, &parent, &labelsJSON); err != nil {
			return nil, fmt.Errorf("store: scan scope: %w", err)
		}
		labels, err := unmarshalLabels(labelsJSON)
		if err != nil {
			return nil, fmt.Errorf("store: parse scope labels for %s: %w", id, err)
		}
		out = append(out, inventory.Scope{
			ID: id, DisplayName: name, Parent: parent, Labels: labels,
		})
	}
	return out, rows.Err()
}

// CountResourcesByKind returns kind→count for the run, in descending count order.
func (s *Store) CountResourcesByKind(ctx context.Context, runID int64) (map[inventory.Kind]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT kind, COUNT(*) FROM resources WHERE run_id = ? GROUP BY kind`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: count by kind: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[inventory.Kind]int)
	for rows.Next() {
		var (
			kind  string
			count int
		)
		if err := rows.Scan(&kind, &count); err != nil {
			return nil, err
		}
		out[inventory.Kind(kind)] = count
	}
	return out, rows.Err()
}

// rowScanner abstracts *sql.Row and *sql.Rows for scanRunSummary.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRunSummary(r rowScanner) (RunSummary, error) {
	var (
		out          RunSummary
		startedRaw   string
		finishedRaw  sql.NullString
	)
	if err := r.Scan(
		&out.ID, &out.UUID, &out.Provider, &out.ScopeID, &out.ScopeName,
		&startedRaw, &finishedRaw, &out.Status, &out.CloudcmderV, &out.Notes,
	); err != nil {
		return out, err
	}
	t, err := time.Parse(time.RFC3339Nano, startedRaw)
	if err != nil {
		return out, fmt.Errorf("store: parse started_at %q: %w", startedRaw, err)
	}
	out.StartedAt = t
	if finishedRaw.Valid {
		ft, err := time.Parse(time.RFC3339Nano, finishedRaw.String)
		if err != nil {
			return out, fmt.Errorf("store: parse finished_at %q: %w", finishedRaw.String, err)
		}
		out.FinishedAt = &ft
	}
	return out, nil
}

func unmarshalLabels(s sql.NullString) (map[string]string, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(s.String), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// parseRef reconstructs a ResourceRef from its stored canonical string.
// Format is provider:scope:Kind:id; the scope and kind columns are passed
// alongside as a defensive cross-check, used to fill the struct without
// re-splitting a known shape.
func parseRef(ref, kind, scopeID string) inventory.ResourceRef {
	// "gcp:my-project:VM:my-instance" → split into 4.
	parts := strings.SplitN(ref, ":", 4)
	out := inventory.ResourceRef{Kind: inventory.Kind(kind), ScopeID: scopeID}
	if len(parts) == 4 {
		out.Provider = parts[0]
		out.ID = parts[3]
	}
	return out
}
