# cloudcmder

**Cloud Commander** — an interactive TUI for inventorying your cloud resources.

> Pronounced "cloud commander". Runs as a single static binary inside GCP CloudShell.
> No database to install. No SQL to write. No credentials to copy.

## What it does

- **Commander dashboard** — k9s/lazygit-style two-pane layout. Cursor in the resource list drives a live detail pane next to it; `:disk` / `:db` / etc. swap the list in place; `Enter` zooms detail to full width.
- Navigate your GCP projects → resource overview → drill into VMs, disks, databases, networks, load balancers, GKE clusters, and more.
- See interconnections: which VM is on which subnet, behind which load balancer, attached to which disks — all on one screen.
- Export a full assessment as a multi-tab Excel workbook.
- Store every scan in a portable SQLite file you can copy out of CloudShell for offline analysis or audit.

## Quickstart (GCP CloudShell)

```sh
# Download the latest binary for linux/amd64
curl -Lo cloudcmder https://github.com/<org>/cloudcmder/releases/latest/download/cloudcmder_linux_amd64.tar.gz \
  | tar -xzO cloudcmder > cloudcmder && chmod +x cloudcmder

# Run — uses your existing CloudShell credentials automatically
./cloudcmder
```

## Required IAM roles

Assign these roles to the account you use with `gcloud auth application-default login`:

| Role | Purpose |
|---|---|
| `roles/viewer` | Read most resource types (compute, sql, gke, run, cloud functions, storage list) |
| `roles/cloudasset.viewer` | Cloud Asset Inventory discovery |
| `roles/storage.legacyBucketReader` *(optional)* | Accurate `PublicAccess` on Cloud Storage buckets — without it, the IAM check is skipped and buckets default to `PublicAccess=false` |

> Read-only. cloudcmder never modifies resources. Per-API enablement on the target project is also required (Compute, Cloud SQL Admin, Container, Cloud Run, Cloud Functions, GCS); a disabled API is logged as a warning and that kind is skipped — the rest of the scan still completes.

## Keybindings

| Key | Action |
|---|---|
| `Enter` | On a kind row: open that kind's resource list in the left pane. On a resource row: zoom Detail to full width. |
| `Esc` | Unzoom Detail; or go back one screen |
| `Tab` | Cycle focus between the list (left) and detail (right) panes |
| `q` | Quit |
| `?` | Toggle contextual help |
| `/` | Filter list by regex (case-insensitive; substring fallback if invalid) |
| `:vm` `:disk` `:db` `:lb` `:net` `:subnet` `:fw` `:bucket` `:gke` `:fn` | Swap the left pane to that kind's resource list |
| `g` | Open the ASCII connection-graph view for the focused resource |
| `H` | Run history modal — pick a different run for this scope |
| `e` | Export current run to Excel *(M7)* |
| `R` | Start a new scan from inside the TUI *(M8 — use `--scan` from CLI for now)* |

## CLI flags

```
cloudcmder [flags]

Flags:
  --db string          SQLite assessment database path (default ~/.cloudcmder/cloudcmder.db)
  --log-level string   debug, info, warn, error (default info)
  --list-scopes        list every accessible GCP project as JSON and exit
  --scan string        headless scan of a project; prints the run uuid on completion
  --list-runs          list every stored run as a table
  --show-run string    print resource counts grouped by kind for the given run uuid
  -v, --version        print version
```

`--export` (Excel) lands in M7. The interactive TUI is shipped — invoke `cloudcmder` with no flags.

## Development status

| Milestone | Status |
|---|---|
| M0 Skeleton | ✅ |
| M1 Inventory types + GCP auth | ✅ |
| M2 SQLite store + headless scan | ✅ |
| M3 Bubble Tea TUI shell | ✅ |
| M4 Overview screen | ✅ |
| M5 VM detail + interconnections | ✅ |
| M6 All resource kinds | ✅ |
| M6.5 Commander layout (split-pane, live detail) | ✅ |
| M7 Excel export | 🔲 |
| M8 Concurrency + polish | 🔲 |
| M9 Release v1.0.0 | 🔲 |
| v1.1 TUI Polish (lazydocker-rich) | 🔲 |

See `plan.md` for full milestone details and acceptance criteria.

## Architecture

See `architecture.md` for the full design: layer diagram, Provider interface, SQLite schema, GCP API choices, TUI screen flow, Excel layout, and the multi-cloud extension guide.

## Contributing

See `CLAUDE.md` for coding standards, the dependency rules between layers, and checklists for adding new resource kinds or cloud providers.

## License

Apache-2.0 — see [LICENSE](LICENSE).
