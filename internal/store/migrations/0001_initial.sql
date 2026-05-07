-- One row per discovery run. Many runs across many scopes live in one file.
CREATE TABLE IF NOT EXISTS runs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid         TEXT    NOT NULL UNIQUE,
    provider     TEXT    NOT NULL,
    scope_id     TEXT    NOT NULL,
    scope_name   TEXT,
    started_at   TEXT    NOT NULL,
    finished_at  TEXT,
    status       TEXT    NOT NULL,
    cloudcmder_v TEXT    NOT NULL,
    notes        TEXT
);
CREATE INDEX IF NOT EXISTS idx_runs_scope_started ON runs(scope_id, started_at DESC);

-- Scopes explicitly recorded per run (supports multi-scope runs in v2+).
CREATE TABLE IF NOT EXISTS scopes (
    run_id       INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    scope_id     TEXT    NOT NULL,
    display_name TEXT,
    parent       TEXT,
    labels_json  TEXT,
    PRIMARY KEY (run_id, scope_id)
);

-- All resource rows for a run. detail_json holds the typed struct as JSON.
CREATE TABLE IF NOT EXISTS resources (
    run_id       INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    ref          TEXT    NOT NULL,
    kind         TEXT    NOT NULL,
    scope_id     TEXT    NOT NULL,
    name         TEXT,
    region       TEXT,
    status       TEXT,
    labels_json  TEXT,
    detail_json  TEXT    NOT NULL,
    native_json  TEXT,
    PRIMARY KEY (run_id, ref)
);
CREATE INDEX IF NOT EXISTS idx_resources_kind  ON resources(run_id, kind);
CREATE INDEX IF NOT EXISTS idx_resources_scope ON resources(run_id, scope_id);

-- Directed edges between resources (the interconnection graph).
CREATE TABLE IF NOT EXISTS edges (
    run_id    INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    from_ref  TEXT    NOT NULL,
    ref_kind  TEXT    NOT NULL,
    to_ref    TEXT    NOT NULL,
    PRIMARY KEY (run_id, from_ref, ref_kind, to_ref)
);
CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(run_id, to_ref);

-- Schema version tracking for forward migrations.
CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT OR IGNORE INTO schema_meta VALUES ('schema_version', '1');
