# Support

## Reporting bugs

Open an issue or send feedback via https://cloudcmder.com

Please include:

- The output of `cloudcmder version --short`
- Your OS and shell (`uname -a`)
- Relevant lines from `~/.cloudcmder/cloudcmder.log` (set `--log-level debug` to get more detail)
- Steps to reproduce

## Preflight check

Before scanning, verify that all required GCP APIs are enabled:

```
cloudcmder --check --project YOUR_PROJECT_ID
```

This calls the Service Usage API and prints a copy-paste `gcloud services enable` command for anything missing.

## Diagnostic flags

| Flag | Effect |
|------|--------|
| `--log-level debug` | Verbose logging to `~/.cloudcmder/cloudcmder.log` |
| `--dump-native` | Store raw GCP API payloads in `native_json` column (doubles DB size) |
| `--db /tmp/test.db` | Use a throwaway database for a clean test run |

## Required IAM roles

Minimum read-only set:

- `roles/cloudasset.viewer`
- `roles/compute.viewer`
- `roles/container.viewer`
- `roles/cloudfunctions.viewer`
- `roles/cloudsql.viewer`
- `roles/storage.objectViewer` (project-level)
- `roles/monitoring.viewer`

## Kind aliases (all 34)

`:vm` `:disk` `:db` `:lb` `:net` `:subnet` `:fw` `:bucket` `:gke` `:fn`
`:vertex` `:apigee` `:firebase`/`:fb` `:ae`/`:appengine`/`:gae` `:bq`/`:bigquery`
`:dns` `:redis`/`:memcache`/`:memorystore` `:ar`/`:artifactregistry`
`:pubsub`/`:ps` `:spanner` `:bigtable`/`:bt` `:kms` `:secrets`/`:sm`
`:dataflow`/`:df` `:dataproc`/`:dp` `:composer`/`:airflow` `:tasks`
`:monitoring`/`:stackdriver` `:logging`/`:logs` `:osconfig`/`:vmm`
`:vpn` `:router` `:build`/`:cb` `:scopes` `:cron`/`:scheduler`

## Docs

https://cloudcmder.com/docs.html
