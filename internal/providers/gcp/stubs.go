package gcp

// subtypeMaps is the lookup table for all stub-only Kinds.
// subtypeMaps[Kind][assetType] = Subtype label shown in the TUI.
// Unknown asset types within a known Kind fall through to "Other" so future
// CAI additions surface automatically without a code change.
//
// Searchability reference: https://cloud.google.com/asset-inventory/docs/supported-asset-types
// Only types listed as searchable in SearchAllResources are included.
// Types not in that list silently return zero rows (graceful path).

import "cloudcmder.com/internal/inventory"

var subtypeMaps = map[inventory.Kind]map[string]string{
	// VertexAI — 24 aiplatform.googleapis.com/* types.
	inventory.KindVertexAI: {
		"aiplatform.googleapis.com/BatchPredictionJob":           "BatchPredictionJob",
		"aiplatform.googleapis.com/CachedContent":                "CachedContent",
		"aiplatform.googleapis.com/CustomJob":                    "CustomJob",
		"aiplatform.googleapis.com/Dataset":                      "Dataset",
		"aiplatform.googleapis.com/DeploymentResourcePool":       "DeploymentResourcePool",
		"aiplatform.googleapis.com/Endpoint":                     "Endpoint",
		"aiplatform.googleapis.com/Featurestore":                 "Featurestore",
		"aiplatform.googleapis.com/FeatureGroup":                 "FeatureGroup",
		"aiplatform.googleapis.com/FeatureOnlineStore":           "FeatureOnlineStore",
		"aiplatform.googleapis.com/HyperparameterTuningJob":      "HyperparameterTuningJob",
		"aiplatform.googleapis.com/Index":                        "Index",
		"aiplatform.googleapis.com/IndexEndpoint":                "IndexEndpoint",
		"aiplatform.googleapis.com/MetadataStore":                "MetadataStore",
		"aiplatform.googleapis.com/Model":                        "Model",
		"aiplatform.googleapis.com/ModelDeploymentMonitoringJob": "ModelDeploymentMonitoringJob",
		"aiplatform.googleapis.com/NotebookRuntime":              "NotebookRuntime",
		"aiplatform.googleapis.com/NotebookRuntimeTemplate":      "NotebookRuntimeTemplate",
		"aiplatform.googleapis.com/PipelineJob":                  "PipelineJob",
		"aiplatform.googleapis.com/ReasoningEngine":              "ReasoningEngine",
		"aiplatform.googleapis.com/Schedule":                     "Schedule",
		"aiplatform.googleapis.com/SpecialistPool":               "SpecialistPool",
		"aiplatform.googleapis.com/Tensorboard":                  "Tensorboard",
		"aiplatform.googleapis.com/TrainingPipeline":             "TrainingPipeline",
		"aiplatform.googleapis.com/TuningJob":                    "TuningJob",
	},
	// Apigee — API management platform.
	inventory.KindApigee: {
		"apigee.googleapis.com/ApiProxy":         "ApiProxy",
		"apigee.googleapis.com/ApiProxyRevision": "ApiProxyRevision",
		"apigee.googleapis.com/Environment":      "Environment",
		"apigee.googleapis.com/Instance":         "Instance",
		"apigee.googleapis.com/Organization":     "Organization",
	},
	// Firebase — firebase.googleapis.com stub types. Hosting, Firestore, and
	// Realtime DB are not searchable in CAI; only FirebaseProject and
	// FirebaseAppInfo are confirmed searchable.
	inventory.KindFirebase: {
		"firebase.googleapis.com/FirebaseAppInfo":   "AppInfo",
		"firebase.googleapis.com/FirebaseProject":   "Project",
	},
	// App Engine — appengine.googleapis.com.
	inventory.KindAppEngine: {
		"appengine.googleapis.com/Application": "Application",
		"appengine.googleapis.com/Service":     "Service",
		"appengine.googleapis.com/Version":     "Version",
	},
	// BigQuery — bigquery.googleapis.com.
	inventory.KindBigQuery: {
		"bigquery.googleapis.com/Dataset":         "Dataset",
		"bigquery.googleapis.com/Model":           "Model",
		"bigquery.googleapis.com/Routine":         "Routine",
		"bigquery.googleapis.com/RowAccessPolicy": "RowAccessPolicy",
		"bigquery.googleapis.com/Table":           "Table",
	},
	// Cloud DNS — dns.googleapis.com.
	inventory.KindDNS: {
		"dns.googleapis.com/ManagedZone":    "ManagedZone",
		"dns.googleapis.com/Policy":         "Policy",
		"dns.googleapis.com/ResponsePolicy": "ResponsePolicy",
	},
	// Memorystore — Redis and Memcache managed caches.
	inventory.KindMemorystore: {
		"memcache.googleapis.com/Instance": "Memcache",
		"redis.googleapis.com/Cluster":     "RedisCluster",
		"redis.googleapis.com/Instance":    "Redis",
	},
	// Artifact Registry — artifactregistry.googleapis.com.
	inventory.KindArtifactRegistry: {
		"artifactregistry.googleapis.com/DockerImage": "DockerImage",
		"artifactregistry.googleapis.com/Repository":  "Repository",
	},
	// Cloud Scheduler — cloudscheduler.googleapis.com.
	// Searchability unconfirmed; graceful path returns 0 rows if not supported.
	inventory.KindCloudScheduler: {
		"cloudscheduler.googleapis.com/Job": "Job",
	},
	// Pub/Sub — pubsub.googleapis.com.
	// Searchability unconfirmed; graceful path returns 0 rows if not supported.
	inventory.KindPubSub: {
		"pubsub.googleapis.com/Schema":        "Schema",
		"pubsub.googleapis.com/Snapshot":      "Snapshot",
		"pubsub.googleapis.com/Subscription":  "Subscription",
		"pubsub.googleapis.com/Topic":         "Topic",
	},
	// Spanner — spanner.googleapis.com.
	// Searchability unconfirmed; graceful path returns 0 rows if not supported.
	inventory.KindSpanner: {
		"spanner.googleapis.com/Backup":   "Backup",
		"spanner.googleapis.com/Database": "Database",
		"spanner.googleapis.com/Instance": "Instance",
	},
	// Bigtable — bigtableadmin.googleapis.com.
	// Searchability unconfirmed; graceful path returns 0 rows if not supported.
	inventory.KindBigtable: {
		"bigtableadmin.googleapis.com/Backup":   "Backup",
		"bigtableadmin.googleapis.com/Cluster":  "Cluster",
		"bigtableadmin.googleapis.com/Instance": "Instance",
		"bigtableadmin.googleapis.com/Table":    "Table",
	},
	// Cloud KMS — cloudkms.googleapis.com.
	inventory.KindKMS: {
		"cloudkms.googleapis.com/CryptoKey":     "CryptoKey",
		"cloudkms.googleapis.com/KeyRing":       "KeyRing",
	},
	// Secret Manager — secretmanager.googleapis.com.
	// Searchability unconfirmed; graceful path returns 0 rows if not supported.
	inventory.KindSecretManager: {
		"secretmanager.googleapis.com/Secret": "Secret",
	},
	// Dataflow — dataflow.googleapis.com.
	inventory.KindDataflow: {
		"dataflow.googleapis.com/Job": "Job",
	},
	// Dataproc — dataproc.googleapis.com.
	inventory.KindDataproc: {
		"dataproc.googleapis.com/Cluster":          "Cluster",
		"dataproc.googleapis.com/Job":              "Job",
	},
	// Cloud Composer — composer.googleapis.com.
	inventory.KindComposer: {
		"composer.googleapis.com/Environment": "Environment",
	},
	// Cloud Tasks — cloudtasks.googleapis.com.
	// Searchability unconfirmed; graceful path returns 0 rows if not supported.
	inventory.KindCloudTasks: {
		"cloudtasks.googleapis.com/Queue": "Queue",
	},
	// Cloud Monitoring — monitoring.googleapis.com.
	inventory.KindMonitoring: {
		"monitoring.googleapis.com/AlertPolicy":         "AlertPolicy",
		"monitoring.googleapis.com/NotificationChannel": "NotificationChannel",
		"monitoring.googleapis.com/Snooze":              "Snooze",
	},
	// Cloud Logging — logging.googleapis.com.
	inventory.KindLogging: {
		"logging.googleapis.com/LogBucket": "LogBucket",
		"logging.googleapis.com/LogMetric": "LogMetric",
		"logging.googleapis.com/LogSink":   "LogSink",
	},
	// OS Config (VM Manager) — osconfig.googleapis.com.
	// Searchability unconfirmed; graceful path returns 0 rows if not supported.
	inventory.KindOSConfig: {
		"osconfig.googleapis.com/OSPolicyAssignment": "OSPolicyAssignment",
		"osconfig.googleapis.com/PatchDeployment":    "PatchDeployment",
	},
	// Cloud VPN — compute.googleapis.com VPN sub-resources.
	inventory.KindVPN: {
		"compute.googleapis.com/ExternalVpnGateway": "ExternalVpnGateway",
		"compute.googleapis.com/VpnGateway":         "VpnGateway",
		"compute.googleapis.com/VpnTunnel":          "VpnTunnel",
	},
	// Cloud Router — compute.googleapis.com/Router.
	inventory.KindRouter: {
		"compute.googleapis.com/Router": "Router",
	},
	// Cloud Build — cloudbuild.googleapis.com.
	inventory.KindCloudBuild: {
		"cloudbuild.googleapis.com/Build":        "Build",
		"cloudbuild.googleapis.com/BuildTrigger": "BuildTrigger",
	},
}

// stubAssetTypes is a reverse index: assetType → true for every type in subtypeMaps.
// Built once at package init; used by isStubKindAssetType.
var stubAssetTypes = func() map[string]bool {
	m := make(map[string]bool)
	for _, inner := range subtypeMaps {
		for at := range inner {
			m[at] = true
		}
	}
	return m
}()

// isStubKindAssetType reports whether at belongs to any stub-only Kind.
func isStubKindAssetType(at string) bool {
	return stubAssetTypes[at]
}

// stubDetailForKind returns a *inventory.StubDetail for any (Kind, assetType)
// pair where the Kind is stub-only. Returns nil for non-stub Kinds or when
// kind has no subtypeMap entry. The Subtype falls back to "Other" for unknown
// future asset types within a known stub Kind.
func stubDetailForKind(kind inventory.Kind, at string) *inventory.StubDetail {
	m, ok := subtypeMaps[kind]
	if !ok {
		return nil
	}
	sub, ok := m[at]
	if !ok {
		sub = "Other"
	}
	return &inventory.StubDetail{Subtype: sub}
}

// stubKinds returns every Kind that has a subtypeMap entry, i.e., every
// stub-only Kind. Order is not guaranteed (map iteration). Used by
// streamAssetStubs to route stub types through the graceful CAI search path.
func stubKinds() []inventory.Kind {
	out := make([]inventory.Kind, 0, len(subtypeMaps))
	for k := range subtypeMaps {
		out = append(out, k)
	}
	return out
}
