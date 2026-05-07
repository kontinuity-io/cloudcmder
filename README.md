# cloudcmder

**Cloud Commander** — an interactive TUI for inventorying your cloud resources.

> Pronounced "cloud commander". Runs as a single static binary inside GCP CloudShell.
> No database to install. No SQL to write. No credentials to copy.

## What it does

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
| `roles/viewer` | List most resource types |
| `roles/cloudasset.viewer` | Cloud Asset Inventory discovery |

> Read-only. cloudcmder never modifies resources.

## Keybindings

| Key | Action |
|---|---|
| `Enter` | Drill into selected item |
| `Esc` | Go back |
| `q` | Quit |
| `?` | Show contextual help |
| `/` | Filter list by regex |
| `:vm` `:disk` `:db` `:lb` `:net` `:bucket` `:gke` `:fn` | Jump to resource type |
| `e` | Export current run to Excel |
| `H` | Run history for current scope |
| `R` | Start a new scan of the current project |

## CLI flags

```
cloudcmder [flags]

Flags:
  --db string          SQLite assessment database path (default ~/.cloudcmder/cloudcmder.db)
  --log-level string   debug, info, warn, error (default info)
  --scan string        headless scan of a project; prints run uuid on completion
  --list-runs          list all stored assessment runs
  --export string      export a run to Excel (.xlsx)
  --run string         run uuid to export (default: latest)
  -v, --version        print version
```

## Development status

| Milestone | Status |
|---|---|
| M0 Skeleton | ✅ |
| M1 Inventory types + GCP auth | 🔲 |
| M2 SQLite store + headless scan | 🔲 |
| M3 Bubble Tea TUI shell | 🔲 |
| M4 Overview screen | 🔲 |
| M5 VM detail + interconnections | 🔲 |
| M6 All resource kinds | 🔲 |
| M7 Excel export | 🔲 |
| M8 Concurrency + polish | 🔲 |
| M9 Release v1.0.0 | 🔲 |

See `plan.md` for full milestone details and acceptance criteria.

## Architecture

See `architecture.md` for the full design: layer diagram, Provider interface, SQLite schema, GCP API choices, TUI screen flow, Excel layout, and the multi-cloud extension guide.

## Contributing

See `CLAUDE.md` for coding standards, the dependency rules between layers, and checklists for adding new resource kinds or cloud providers.

## License

Apache-2.0 — see [LICENSE](LICENSE).
