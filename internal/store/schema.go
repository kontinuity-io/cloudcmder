package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate applies any embedded migrations whose numeric prefix exceeds the
// schema_version recorded in schema_meta. The schema_meta table itself is
// created by 0001_initial.sql, so we tolerate it being absent on a fresh DB.
func migrate(ctx context.Context, db *sql.DB) error {
	files, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".sql") {
			names = append(names, f.Name())
		}
	}
	sort.Strings(names)

	current, err := currentSchemaVersion(ctx, db)
	if err != nil {
		return err
	}

	for _, name := range names {
		v, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %q: bad numeric prefix: %w", name, err)
		}
		if v <= current {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", name, err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %q: %w", name, err)
		}
		if _, err := db.ExecContext(ctx,
			`INSERT OR REPLACE INTO schema_meta (key, value) VALUES ('schema_version', ?)`,
			strconv.Itoa(v),
		); err != nil {
			return fmt.Errorf("update schema_version after %q: %w", name, err)
		}
	}
	return nil
}

func currentSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	// schema_meta may not exist on a brand-new file; treat that as version 0.
	var v string
	row := db.QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE key = 'schema_version'`)
	switch err := row.Scan(&v); {
	case err == sql.ErrNoRows:
		return 0, nil
	case err != nil:
		// Most likely "no such table"; the first migration creates it.
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("parse schema_version %q: %w", v, err)
	}
	return n, nil
}
