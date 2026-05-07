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

## Quickstart

### Build from source

The released binary ships with v1.0.0 (M9 milestone). Until then, build from source:

```sh
# Prereqs: Go 1.25+, gcloud CLI (for credentials)
git clone https://github.com/kontinuity-io/cloudcmder
cd cloudcmder

# 1. Authenticate to GCP via Application Default Credentials.
#    Skip this step inside CloudShell — ADC is already set up.
gcloud auth application-default login

# 2. Build a static binary (CGO_ENABLED=0 keeps modernc.org/sqlite happy
#    and produces a portable binary you can copy anywhere).
CGO_ENABLED=0 go build -o cloudcmder ./cmd/cloudcmder

# 3. Verify auth — prints every project the credential can see, as JSON.
./cloudcmder --list-scopes

# 4. Scan a project (writes ~/.cloudcmder/cloudcmder.db).
./cloudcmder --scan <your-project-id>

# 5. Browse the scan in the commander TUI.
./cloudcmder
```

### Release binary (post-v1.0.0)

Once v1.0.0 ships you can skip the build step:

```sh
curl -Lo cloudcmder https://github.com/kontinuity-io/cloudcmder/releases/latest/download/cloudcmder_linux_amd64.tar.gz \
  | tar -xzO cloudcmder > cloudcmder && chmod +x cloudcmder
./cloudcmder --scan <your-project-id>
./cloudcmder
```

### Day-to-day usage

```sh
# Refresh data — re-run the scan whenever you want a fresh snapshot.
# (Typically completes in ~5s; the 10 enrichment phases run concurrently
#  with a 4-goroutine cap.)
./cloudcmder --scan my-project

# Inspect the runs the store has (no GCP calls).
./cloudcmder --list-runs

# See the kind/count breakdown for a specific run.
./cloudcmder --show-run <uuid>

# Open the TUI on a different db (e.g. one a teammate shared).
./cloudcmder --db /path/to/their.db

# Power-user: query the SQLite directly.
sqlite3 ~/.cloudcmder/cloudcmder.db "SELECT kind, COUNT(*) FROM resources GROUP BY kind;"

# Export the most recent run to a multi-tab .xlsx for analysts/auditors.
./cloudcmder --export ~/Desktop/assessment.xlsx

# Export a specific run by uuid.
./cloudcmder --export /tmp/old-snapshot.xlsx --run a5f1880b-8225-4ab1-915e-8461f6a21ee8
# (Inside the TUI, press `e` to export the current run to
#  ~/.cloudcmder/exports/<scope>-<short-uuid>.xlsx.)
```

> **First-run gotcha:** if your project has APIs disabled (e.g. Cloud Functions or GKE never used), the scan logs a warning per kind and skips it. The rest of the scan still completes. To enrich every kind, enable the corresponding API once via the Cloud Console or `gcloud services enable …`.

## Required IAM roles

Assign these roles to the account you use with `gcloud auth application-default login`:

| Role | Purpose |
|---|---|
| `roles/viewer` | Read most resource types (compute, sql, gke, run, cloud functions, storage list) |
| `roles/cloudasset.viewer` | Cloud Asset Inventory discovery |
| `roles/storage.legacyBucketReader` *(optional)* | Accurate `PublicAccess` on Cloud Storage buckets — without it, the IAM check is skipped and buckets default to `PublicAccess=false` |

> Read-only. cloudcmder never modifies resources. The list of APIs that must be enabled on the target project is in [Troubleshooting](#troubleshooting); a disabled API is logged as a warning and that kind is skipped — the rest of the scan still completes.

## Keybindings

| Key | Action |
|---|---|
| `Enter` | On a kind row: open that kind's resource list in the left pane. On a resource row: zoom Detail to full width. |
| `Esc` | Unzoom Detail; or walk back through left-pane history. No-op at the root pane — `q` is the only way out of the Frame. |
| `Tab` | Cycle focus between the list (left) and detail (right) panes |
| `q` | Quit |
| `?` | Toggle contextual help |
| `/` | Filter list by regex (case-insensitive; substring fallback if invalid) |
| `:vm` `:disk` `:db` `:lb` `:net` `:subnet` `:fw` `:bucket` `:gke` `:fn` | Swap the left pane to that kind's resource list |
| `g` | Open the ASCII connection-graph view for the focused resource |
| `H` | Run history modal — pick a different run for this scope |
| `e` | Export current run to Excel — lands in `~/.cloudcmder/exports/<scope>-<short-uuid>.xlsx` |
| `R` | Start a new scan from inside the TUI *(deferred to v1.1 — use `--scan` from CLI for now)* |

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
  --export string      write a multi-tab Excel workbook for a stored run to the given path
  --run string         run uuid to export (with --export); defaults to the most recent run
  -v, --version        print version
```

The interactive TUI is shipped — invoke `cloudcmder` with no flags.

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
| M7 Excel export | ✅ |
| M8 Concurrency + polish | ✅ |
| M9 Release v1.0.0 | 🔲 |
| v1.1 TUI Polish (lazydocker-rich) | 🔲 |

See `plan.md` for full milestone details and acceptance criteria.

## Troubleshooting

### Enable the GCP APIs cloudcmder reads

Each Kind needs its provider API enabled on the target project. A disabled API isn't fatal — the scan logs a warning and skips that kind — but for full enrichment, enable all of them:

```sh
PROJECT=<your-project-id>
gcloud services enable \
  cloudresourcemanager.googleapis.com \
  cloudasset.googleapis.com \
  compute.googleapis.com \
  sqladmin.googleapis.com \
  container.googleapis.com \
  storage.googleapis.com \
  run.googleapis.com \
  cloudfunctions.googleapis.com \
  --project=$PROJECT
```

| API | Used for |
|---|---|
| `cloudresourcemanager.googleapis.com` | `--list-scopes`; project listing |
| `cloudasset.googleapis.com` | Cloud Asset Inventory discovery (Phase 1 of every scan) |
| `compute.googleapis.com` | VMs, Disks, Networks, Subnets, Firewalls, Load Balancers |
| `sqladmin.googleapis.com` | Cloud SQL Databases |
| `container.googleapis.com` | GKE Clusters |
| `storage.googleapis.com` | GCS Buckets (list + IAM check for `PublicAccess`) |
| `run.googleapis.com` | Cloud Run services (rendered as `Function`) |
| `cloudfunctions.googleapis.com` | Cloud Functions Gen2 (rendered as `Function`) |

To turn them off again (no resources are deleted; the project just loses API access):

```sh
gcloud services disable \
  cloudasset.googleapis.com \
  compute.googleapis.com \
  sqladmin.googleapis.com \
  container.googleapis.com \
  storage.googleapis.com \
  run.googleapis.com \
  cloudfunctions.googleapis.com \
  --project=$PROJECT
```

> `cloudresourcemanager.googleapis.com` is intentionally omitted from the disable list — disabling it can lock you out of project metadata. Leave it on.

### Common issues

- **`--list-scopes` returns nothing** — your ADC credential can't see any projects. Re-auth with `gcloud auth application-default login`, or check `gcloud projects list` returns at least one project.
- **`PermissionDenied: API has not been used in project …`** — that one API isn't enabled. The scan continues for the other kinds; enable the API per the table above and re-scan.
- **All `Function` rows have empty Detail** — both `run.googleapis.com` and `cloudfunctions.googleapis.com` need to be enabled (Cloud Run + Cloud Functions Gen2 both back the `Function` kind).
- **Bucket `PublicAccess` always shows `no`** — your credential lacks `storage.buckets.getIamPolicy`. Grant `roles/storage.legacyBucketReader` (or any role including that permission) and re-scan. Without it cloudcmder defaults to "not public" to avoid false alarms.
- **TUI rendering looks corrupted** — check `~/.cloudcmder/cloudcmder.log`. Debug output is routed there so it can't trash the alt-screen; if anything went sideways the trace is in that file.
- **Stuck on `loading…` forever** — usually a slow GCP API call. `q` to quit cleanly, then `tail ~/.cloudcmder/cloudcmder.log`.
- **Start fresh** — `rm ~/.cloudcmder/cloudcmder.db` deletes every stored run; the next `--scan` rebuilds from scratch.
- **Open the SQLite directly** — `sqlite3 ~/.cloudcmder/cloudcmder.db`. Schema is documented in `architecture.md`.

## Architecture

See `architecture.md` for the full design: layer diagram, Provider interface, SQLite schema, GCP API choices, TUI screen flow, Excel layout, and the multi-cloud extension guide.

## Contributing

See `CLAUDE.md` for coding standards, the dependency rules between layers, and checklists for adding new resource kinds or cloud providers.

## License

Apache-2.0 — see [LICENSE](LICENSE).
