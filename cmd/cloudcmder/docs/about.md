# About cloudcmder

**cloudcmder** is an interactive terminal UI for inventorying, navigating, and exporting your Google Cloud Platform estate — without leaving your terminal.

## What it does

Run `cloudcmder` inside GCP CloudShell (or any environment with Application Default Credentials) and you get:

- A searchable, filterable view of every resource running across your GCP projects.
- A Commander split-pane layout: resource list on the left, live detail pane on the right.
- An `--single-view` 4-pane mode for wide terminals.
- Multi-tab Excel export (one sheet per resource kind) with `e` or `--export`.
- Multi-project fleet export with `--export-multi`.

## 34 resource kinds

**Core** (fully enriched with native API calls):
`VM` · `Disk` · `Network` · `Subnet` · `Firewall` · `LoadBalancer` · `Database` · `Cluster` · `Bucket` · `Function`

**Extended** (Cloud Asset Inventory stubs):
`VertexAI` · `Apigee` · `Firebase` · `AppEngine` · `BigQuery` · `DNS` · `Memorystore` · `ArtifactRegistry` · `CloudScheduler` · `PubSub` · `Spanner` · `Bigtable` · `KMS` · `SecretManager` · `Dataflow` · `Dataproc` · `Composer` · `CloudTasks` · `Monitoring` · `Logging` · `OSConfig` · `VPN` · `Router` · `CloudBuild`

## How it stores data

Every scan writes to a local SQLite file (`~/.cloudcmder/cloudcmder.db` by default). Scans are crash-safe: interrupt at any time and the rows already written stay on disk. The TUI and export commands read exclusively from the store — no live GCP calls during navigation.

## Single static binary

Built with `CGO_ENABLED=0` — no runtime dependencies, no setup. Drop it in CloudShell and run.

## Required IAM roles

Minimum read-only permissions:

- `roles/cloudasset.viewer` — Cloud Asset Inventory (all 34 kinds)
- `roles/compute.viewer` — VM, Disk, Network, Subnet, Firewall, LoadBalancer enrichment
- `roles/container.viewer` — GKE Cluster enrichment
- `roles/cloudfunctions.viewer` — Cloud Function enrichment
- `roles/cloudsql.viewer` — Cloud SQL Database enrichment
- `roles/storage.objectViewer` (project-level) — Cloud Storage Bucket enrichment
- `roles/monitoring.viewer` — Bucket size/object-count metrics

Run `cloudcmder --check` to audit which GCP APIs are enabled on your project.

## Key bindings (quick reference)

| Key | Action |
|-----|--------|
| `/` | Fuzzy filter |
| `:` | Command palette (kind aliases + resource jump) |
| `?` | Toggle help overlay |
| `e` | Export to Excel |
| `H` | Run history |
| `R` | Rescan |
| `Esc` | Back |
| `q` | Quit |
| `j` / `k` | Step down / up |
| `g` / `G` | Jump to first / last |
| `Ctrl-u` / `Ctrl-d` | Half-page up / down |
| `Tab` | Cycle focus (single-view mode) |
| `s` | Sort column |
| `m` | Cycle detail mode (Full / Connections / JSON / InlineGraph) |

For the full alias list see `cloudcmder support` or the docs at https://cloudcmder.com/docs.html.
