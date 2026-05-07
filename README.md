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

Active in the TUI today (M3): `Esc`, `q`, `?`, `/`, `:`, `H`. Others land in the milestone noted.

| Key | Action |
|---|---|
| `Enter` | Drill into selected item |
| `Esc` | Go back |
| `q` | Quit |
| `?` | Show contextual help |
| `/` | Filter list by regex |
| `:vm` `:disk` `:db` `:lb` `:net` `:bucket` `:gke` `:fn` | Jump to resource type *(M4+)* |
| `e` | Export current run to Excel *(M7)* |
| `H` | Run history for current scope |
| `R` | Start a new scan of the current project *(M8)* |

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

`--export` (Excel) and the interactive TUI land in later milestones.

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
