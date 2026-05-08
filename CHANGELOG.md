# Changelog

All notable changes to **cloudcmder** are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
