# Changelog

All notable changes to **cloudcmder** are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### v1.5.6 — Scan UI scaling

- Scan progress view now uses a fixed-height compact aggregate layout: progress bar + ok/fail/running/queued counts + active scope row + rolling tail of last 5 completions. Fixes top-row truncation and flicker on terminals with fewer rows than scopes (confirmed failure mode at 94 scopes on a 40-row terminal). Tail depth auto-scales with terminal height; layout is stable at any terminal width ≥ 50 columns.
- `charm.land/bubbles/v2/progress` and `github.com/charmbracelet/harmonica` added as direct dependencies (were already in the module graph transitively).

#### v1.5.5 — Scan progress UI

- `--scan-all` / `--scan-scopes` now show a live Bubble Tea progress view on interactive terminals: spinner on the active scope, per-scope resource counter, elapsed time per scope, ✓/✗ status glyphs on completion. Non-TTY output (CI, piped, `> file`) is byte-identical to the previous plain-text `[i/N] scope … ok (run UUID)` format.
- `golang.org/x/term` promoted to a direct dependency (was already a transitive dep via Bubble Tea).

#### v1.5 — Cross-cloud decoupling (pre-AWS gate)

- `--provider {gcp,aws}` PersistentFlag added (default: `gcp` — fully backward-compatible). `--provider aws` returns a clear "not yet implemented" error until v2 ships.
- `--scope` alias for `--scan`; `--scan-scopes` alias for `--scan-projects`.
- `inventory.Provider` interface gains `Close() error` (every provider should release connections).
- GCP-shape field renames in `internal/inventory/detail.go` — **user-visible Excel column rename**:
  - `LicenseProject` → `MarketplaceProject` (VM + Disk sheets)
  - `LicenseClass` → `MarketplaceClass` (VM + Disk sheets)
  - `ClusterDetail.Autopilot` → `Serverless` (GKE Autopilot → generic serverless-mode flag for future EKS Fargate etc.; `AUTOPILOT` table column → `SERVERLESS`)
- 24 GCP-branded `Kind` identifiers prefixed with `KindGCP` (e.g. `KindApigee` → `KindGCPApigee`). Wire-format string values unchanged — existing DBs load correctly without migration.
- Excel multi-workbook `"Project"` first-column header renamed to `"Scope"` across Summary, Scopes, per-kind, and Edges sheets.
- TUI scope-screen hints updated: `--scan <project-id>` → `--scan <scope-id>`.
- `.gitignore` extended: goreleaser `--snapshot` tarballs, `website/downloads/`, scan-output xlsx files.

#### v1.4 — Tabbed Detail chrome

- Replaced the hidden `m` mode cycle in the right-pane Detail with a visible
  4-tab strip: **Overview · Connections · JSON · Graph**. The active tab uses a
  custom lipgloss border (`Bottom: " "`) so it visually "opens into" the content
  below; a `TabGap` extends the bottom rule to the pane edge. Pattern ported from
  `GustavoCaso/docker-dash`.
- `shift+←` / `shift+→` cycle the active tab; `1`–`4` jump directly; `m` kept as
  a muscle-memory alias. All three binding groups advertised in the global `?`
  overlay via `internal/tui/keymap.go`.
- Tab strip pinned above the scrollable viewport — content scrolls; strip stays
  fixed. Viewport height reduced by 3 rows (strip height) via the existing
  `WindowSizeMsg` path so pane budget arithmetic is unchanged.
- `DETAIL —` and `CONNECTIONS` accent headers removed from the respective content
  panes (tab labels now carry that context).
- `style.ActiveTabBorder`, `style.TabBorder`, `style.TabActive`, `style.TabInactive`,
  `style.TabGap` added to `internal/tui/style/style.go`. Reuses the Catppuccin
  Mocha Mauve / Accent / Dim tokens already present.

#### `cloudcmder version` colorful banner + CLI help commands

- `cloudcmder version` prints a figurine figlet wordmark (ANSI Regular font with
  per-glyph color gradient) followed by a lipgloss-bordered metadata box: version,
  commit, build date, OS/arch, Go version, docs link. `--short` emits the terse
  one-liner for scripting. `NO_COLOR=1` suppresses the figlet entirely.
- `cloudcmder --version` routes through the same `renderBanner()` as the
  subcommand; cobra's auto-version template removed.
- `cloudcmder about` renders `cmd/cloudcmder/docs/about.md` (one-page tool
  summary, resource kinds, SQLite contract, quickstart) via glamour.
- `cloudcmder support` renders `cmd/cloudcmder/docs/support.md` (bug reporting,
  preflight check, diagnostic flags, required IAM roles, all 34 kind aliases)
  via glamour.
- Both `about` and `support` honour `NO_COLOR` and terminal dark/light mode via
  glamour's `WithAutoStyle()`.
- New deps: `github.com/arsham/figurine v1.3.0`,
  `github.com/charmbracelet/glamour v1.0.0`. Binary size +12% vs v1.0 (figurine
  pulls `spf13/viper`; informal +8% bound accepted as one-time cost).

#### Charm v2 migration + Catppuccin Mocha overhaul

- Migrated `bubbletea`, `bubbles`, `lipgloss` to v2 under the
  `charm.land/...` import paths. Dropped `muesli/termenv`.
- Catppuccin Mocha palette applied across every screen via tokens in
  `internal/tui/style/style.go`. Selected-row uses Surface0 (`#313244`)
  so the cursor reads at a glance.
- Status `●` bullets re-enabled in row tables (`bubbles/v2/table` is
  ANSI-aware in row width math; v1 limitation removed).
- Overview rewritten to a hand-rolled lipgloss health dashboard:
  per-Kind count bars (`████░░░░`), 5-dot health indicators (`●●●○○`),
  gradient counts, `[OK]` / `[CRIT: N]` status badges.
- New `store.KindStats(ctx, runID)` query aggregates per-Kind health
  buckets (healthy/warning/critical/unknown) in one `GROUP BY` pass.
- Binary size delta +1.8% vs v1.0 release (DoD was ≤ +15%).

#### `--single-view` 4-pane TUI mode

- New CLI flag activates an alternative TUI layout that renders Scopes,
  kind Summary, Resources, and Detail simultaneously in a 2×2 grid (top
  25/75, bottom 70/30, body height 50/50). `Tab` cycles focus forward
  through the four panes; `Shift+Tab` cycles back; nil downstream panes
  are skipped so cycling works before the first scope is resolved.
- Cursor moves in any pane cascade downstream automatically: scope →
  summary → resources → detail. Cascades fire as atomic core messages
  (`ScopeSelectedMsg`, `KindSelectedMsg`, `ResourceSelectedMsg`)
  sequenced via `tea.Sequence` so a downstream pane never binds to a
  half-built upstream pane.
- Cmdbar `:alias` (`:vm`, `:bucket`, …) still works inside single-view:
  jumps the bottom-left ResourceList to the requested kind and focuses
  it, with the JumpID queued before Init for atomic swap-and-jump.
- `Esc` and `q` both quit. Requires ≥100-column terminal; narrower
  terminals show a resize hint.
- The existing screen-stack TUI is untouched; both modes coexist and
  the user picks one at app start via the `--single-view` flag.

#### Scrollable Detail pane

- Detail content now renders through a `bubbles/viewport` so resources
  whose detail exceeds the pane height stay reachable instead of
  clipping at the bottom border. VMs with many attached disks/NICs are
  the obvious case; narrow Detail columns in single-view hit it too.
- Scrolling keys: `↑` `↓` `j` `k` `PgUp` `PgDn` `Ctrl-u` `Ctrl-d` `f`
  `b` `Home` `End` (viewport's default pager bindings). Horizontal
  bindings disabled — kv-line content has nothing to scroll
  horizontally. `m` (mode cycle) resets scroll to top.
- Applies to both screen-stack and `--single-view` modes.

#### Overview `SetChromeBudget(int)`

- New method lets callers override the kind-count table's chrome
  assumption. Default 12 unchanged (Frame's full-height left pane
  keeps existing behaviour); single-view's half-height top-right
  pane calls with 7 so the table fills the box rather than starving
  at 5 rows.

#### GCS bucket size + object count (Cloud Monitoring)

- `BucketDetail` gains `SizeBytes` and `ObjectCount`, populated from Cloud
  Monitoring's `storage/total_bytes` and `storage/object_count` metrics.
  Autoclass buckets (which emit one series per storage class) are summed
  per bucket. TUI Bucket list adds `SIZE` and `OBJECTS` columns; right-pane
  Detail shows both with a daily-aggregate hint when both are 0. Excel
  bucket sheet gains raw-integer `SizeBytes` and `ObjectCount` columns.
- New required API: `monitoring.googleapis.com`. `--check` flags it if missing.
- Failure is recoverable per the scan contract: if Monitoring rejects (API
  disabled, no IAM, project too young), warn and fall back to 0 — scan
  continues.
- Implementation note pinned by `TestBucketMetricTypesIterableSingly`:
  `ListTimeSeries` only returns one metric.type per call, so the fetch
  loops one call per metric. Earlier attempts (multi-metric `OR`,
  `one_of(...)`) were rejected with `InvalidArgument`.

### Fixed

- **TUI: Esc at Frame root now returns to the Scopes picker.** Previously
  `Esc` on a Frame with no pane-history was a no-op, trapping the user on
  the resources summary view when multiple scopes existed. `Esc` now reads
  consistently as "back through context" all the way out; `q` remains the
  program-level quit.

#### Multi-project scan + combined workbook

- `--scan-all` — sequentially scans every accessible GCP project. Each project
  gets its own `runs` row. Prints `[i/N] <project> … ok (run <uuid>)` progress.
- `--scan-projects=a,b,c` — scan only the named comma-separated project IDs.
- `--fail-fast` — abort `--scan-all`/`--scan-projects` on the first project
  error (default: log a warning and continue; exit non-zero if any failed).
- `--export-multi <out.xlsx>` — write a combined workbook covering N projects.
  Defaults to the latest run per distinct scope across all stored runs. Narrow
  with `--scopes proj-a,proj-b` (latest run per scope) or
  `--runs uuid1,uuid2` (exact runs).
- Combined workbook layout: **Summary** tab with one row per project, one column
  per resource Kind, and a TOTAL row (TUI Overview-style); **per-kind tabs**
  (VMs, Disks, Networks, …, all 34 Kinds) each gain a leading `Project` column;
  **Scopes** and **Edges** tabs are unioned across all included runs.

#### Excel export — stub-kind sheets (bug fix)

All 24 stub-only Kinds (VertexAI, Apigee, Firebase, AppEngine, BigQuery, DNS,
Memorystore, ArtifactRegistry, CloudScheduler, PubSub, Spanner, Bigtable, KMS,
SecretManager, Dataflow, Dataproc, Composer, CloudTasks, Monitoring, Logging,
OSConfig, VPN, Router, CloudBuild) now appear as dedicated sheets in both the
single-run `--export` workbook and the new `--export-multi` workbook. Previously
only the 10 core Kinds had sheets despite `columnsFor()` supporting all 34.

#### `--check` preflight command

- `--check` — read-only preflight that calls Service Usage API, diffs required
  vs enabled APIs per project, and prints a copy-paste-ready
  `gcloud services enable … --project=ID` command for anything missing.
  Exits 0 when clean; exits non-zero when any APIs are absent — composable with
  `cloudcmder --check && cloudcmder --scan <project>`.
- `--project` — optional flag to limit `--check` to a single project ID
  (default: all projects accessible to the current credentials).
- `gcp.RequiredAPIs()` — pure function that derives the required-API list from
  `assetTypeToKind` at runtime, so adding a new asset type automatically updates
  the preflight without touching the preflight code.

#### Stub-only Kind expansion — 23 new GCP services via Cloud Asset Inventory

- Generalised the VertexAI stub pattern into a shared `inventory.StubDetail{Subtype, Region}`
  and a `subtypeMaps[Kind][assetType]` lookup table (`internal/providers/gcp/stubs.go`).
  All 24 stub-only Kinds (VertexAI + 23 new) reuse the same Detail struct.
- 23 new mega-Kinds, all stub-only, all backed by `roles/cloudasset.viewer` only:
  **Apigee**, **Firebase**, **AppEngine**, **BigQuery**, **DNS**,
  **Memorystore** (Redis + Memcache), **ArtifactRegistry**, **CloudScheduler**,
  **PubSub**, **Spanner**, **Bigtable**, **KMS**, **SecretManager**,
  **Dataflow**, **Dataproc**, **Composer**, **CloudTasks**, **Monitoring**,
  **Logging**, **OSConfig** (VM Manager), **VPN**, **Router**, **CloudBuild**.
- TUI cmdbar aliases: `:apigee` `:firebase`/`:fb` `:appengine`/`:ae`/`:gae`
  `:bigquery`/`:bq` `:dns` `:memorystore`/`:redis`/`:memcache`
  `:artifactregistry`/`:ar` `:scheduler`/`:cron` `:pubsub`/`:ps`
  `:spanner` `:bigtable`/`:bt` `:kms` `:secrets`/`:sm` `:dataflow`/`:df`
  `:dataproc`/`:dp` `:composer`/`:airflow` `:tasks` `:monitoring`/`:stackdriver`
  `:logging`/`:logs` `:osconfig`/`:vmm` `:vpn` `:router` `:build`/`:cb`.
- Each new Kind surfaces in the Overview count, TUI list (NAME/SUBTYPE/REGION/STATUS),
  right-pane Detail, and Excel export (one sheet per Kind with Name/Region/Status/Subtype/Labels).
- Each stub asset type gets its own graceful CAI request — an unsupported
  type silences only that type's rows, not the entire Kind.

#### Vertex AI / Gemini coverage (existing, now using StubDetail)

- `KindVertexAI` with 24 `aiplatform.googleapis.com/*` asset types. No Phase-2
  enricher; `roles/cloudasset.viewer` sufficient. Unknown future types → `Subtype="Other"`.

### Fixed

- **Vertex AI (and all stub Kinds) returning 0 rows.** `SearchAllResources`
  is all-or-nothing per batch: one unsupported asset type in the request
  causes the entire batch to return `InvalidArgument`, silently wiping every
  result for that Kind. Stub asset types now each get their own single-type
  request, so a newly-GA'd type that CAI hasn't added to the searchable list
  yet (e.g. `DeploymentResourcePool`, GA 2026-04-28) loses only its own rows
  rather than all 24 VertexAI rows. Skipped types are now logged as
  `slog.Warn` entries in `~/.cloudcmder/cloudcmder.log` instead of being
  swallowed silently.

### Changed

- `inventory.VertexDetail` renamed to `inventory.StubDetail`. Field shape
  (`Subtype string`, `Region string`) and JSON wire format are unchanged.
  All stored scan data remains readable without migration.

#### Marketplace ISV attribution

- License URI classification (`marketplace.go`): extracts the image project
  from `compute.googleapis.com/Instance` and `compute.googleapis.com/Disk`
  license URLs and classifies as `"marketplace"` | `"google-paid"` |
  `"google-free"` | `""` with any-marketplace-wins precedence across all
  attached disks.
- `VMDetail` and `DiskDetail` gain three new fields: `Licenses []string`,
  `LicenseProject string`, `LicenseClass string`.
- TUI VM list adds a `MARKETPLACE` column; Disk list adds `OS` (license
  project) and `MARKETPLACE` columns. Detail panes show license fields when
  non-empty.
- Excel VM and Disk sheets gain `Licenses`, `LicenseProject`, `LicenseClass`
  columns.

## [1.0.0] - 2026-05-08

First stable release. Single cloud (GCP); 10 normalized resource kinds; SQLite
store; Bubble Tea TUI; Excel export.

### Added

#### Core (M0–M2)

- Project skeleton: cobra CLI, version package, `~/.cloudcmder/cloudcmder.log`
  structured logging via `slog`.
- Normalized inventory types (`internal/inventory`): `Resource`, `ResourceRef`,
  `Scope`, `Provider` interface. 10 kinds (VM / Disk / Network / Subnet /
  Firewall / LoadBalancer / Database / Cluster / Bucket / Function).
- GCP authentication via Application Default Credentials; project listing
  via Resource Manager v3.
- SQLite store (`modernc.org/sqlite` — pure Go, `CGO_ENABLED=0`): `runs`,
  `resources`, `scopes`, `edges` tables. WAL mode + INSERT OR REPLACE for
  crash-safe scans. `--scan`, `--list-runs`, `--show-run` CLI flags.

#### TUI (M3–M6.5)

- Bubble Tea TUI shell with screen stack, breadcrumbs, cmdbar (`:`), and
  toggleable help overlay (`?`). Tokyo Night palette via lipgloss.
- ScopeList screen — every accessible GCP project from the latest run.
- Run history modal (`H`) — switches the active Frame to a different run
  for the same scope.
- Overview screen — kind-count breakdown, run metadata header.
- Per-kind ResourceList with `/` filter, kind-specific column packs.
- VM detail with full enrichment: machine type (vCPUs / memory MiB
  resolved via Compute machineTypes API + singleflight cache), zone,
  status, OS family, boot/attached disks, NICs.
- All 9 remaining resource kinds enriched with their kind-specific Detail
  structs; Cloud Asset Inventory drives Phase 1 stub discovery.
- Interconnection edges: Disk ↔ VM (AttachedTo), VM ↔ Subnet (RoutesFrom).
  ASCII connection-tree view (`g`) per resource.
- Commander split-pane Frame layout (`internal/tui/screens/frame.go`):
  60/40 left/right split at ≥100 cols; stacked top/bottom otherwise. Live
  Detail rebuild as the cursor moves over the resource list.

#### Export & concurrency (M7–M8)

- Excel multi-sheet export via `excelize.StreamWriter` — one sheet per
  kind plus a Summary sheet. CLI `--export <file> [--run <uuid>]` and
  TUI keybinding `e`.
- Concurrent enrichment fan-out: 4 per-kind goroutines (architecture-
  bounded by a semaphore). A typical 80-resource project scans in ~5 s.
- `--dump-native` flag stores raw GCP API payloads in
  `resources.native_json` for diagnostics; off by default (doubles DB
  size).
- Per-pane `bubbles/spinner` while data loads.

#### Fuzzy command palette (v1.1)

- `:` opens a fuzzy palette over kind aliases AND every Resource name in
  the current run. Resource picks emit a single atomic `SwapLeftPaneMsg`
  carrying the JumpID — Frame queues the jump on the new pane before
  Init, so the cursor lands on the matched row as soon as the load
  completes. Library: `github.com/sahilm/fuzzy`.
- Per-pane `/` filter is fuzzy across name + region + status + label
  values; rows reorder by descending fuzzy score.
- New store API `LoadResourceIndex(runID)` for the cmdbar's flat
  `{Kind, ID, Name}` corpus.

#### TUI polish (v1.2)

- Cmdbar renders at the top (k9s-style header) with constant render
  height when open (`1 + visibleWindow`) so typing doesn't cascade
  WindowSizeMsg through the body.
- Cmdbar dropdown viewport: holds up to 50 fuzzy matches; `↑`/`↓` scrolls
  a 4-row window over the pool; `↑`/`↓` glyph markers when more matches
  exist outside the visible window.
- Adaptive column widths — `LeftPane.SetInnerWidth(int)` interface;
  `columnsFor(kind, width)` scales per-column weights to fit any
  terminal width with a 4-rune floor.
- `truncate` is now ANSI-aware via `charmbracelet/x/ansi.Truncate`.
- Bottom status bar (always visible when a Frame is active): scope · run
  uuid · run status · resource counts · scanned <relative time>.
- Toast queue replacing the single-toast clobber; per-entry TTL,
  oldest-first stacking.
- Inline fuzzy match highlight — matched rune indices from
  `fuzzy.Match.MatchedIndexes` are bolded + underlined inside the NAME
  cell on non-selected rows.
- Vim navigation: `j`/`k` step, `g`/`G` jump to ends, `Ctrl+u`/`Ctrl+d`
  half-page on ResourceList and Overview.
- Sortable columns — `s` cycles through every column in both directions,
  then wraps back to "no sort". Sort active only when the filter is
  empty (fuzzy ranking wins otherwise).
- Right-pane modes — `m` cycles Detail / ConnectionsOnly / RawJSON /
  InlineGraph (the InlineGraph mode embeds `graph.go`'s ASCII tree).
- Scopes modal — `:scopes` opens ScopeList over the active Frame;
  picking a scope emits `SwitchRunMsg` + `PopScreen` via `tea.Sequence`,
  swapping the underlying Frame in place without exiting cloudcmder.
- Selected-row contrast — accent foreground on a dark background; bold
  header row.

### Known limitations

- **Status bullets render in Detail panes only.** `bubbles/table` v1's
  row width calculation gets confused by ANSI escape codes in cell
  content; the Charm v2 upgrade (M9.5) unblocks bullets in tables.
- **Detail pane content longer than the right pane's body height clips
  visually at the bottom.** Press `m` to switch to Raw JSON mode (which
  may be shorter) or `Enter` to zoom Detail to full width. A scrollable
  Detail viewport is parked for v1.3 polish.
- **Sort comparator is lexical on rendered cell text** — numeric
  columns (SIZE GB, vCPU, RAM) sort as strings (`"100"` < `"20"`).
  Per-column sort keys are out of scope for v1.0.
- **No row striping.** `bubbles/table` v1 lacks the per-row Styles hook
  needed; revisited after v2 upgrade.

[Unreleased]: https://github.com/kontinuity-io/cloudcmder/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/kontinuity-io/cloudcmder/releases/tag/v1.0.0
