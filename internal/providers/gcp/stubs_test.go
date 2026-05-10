package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/inventory"
)

func TestSubtypeMapsCoversAllStubKinds(t *testing.T) {
	for kind, m := range subtypeMaps {
		assert.NotEmpty(t, m, "subtypeMaps[%s] has no entries", kind)
	}
}

func TestStubDetailForKindKnownTypes(t *testing.T) {
	cases := []struct {
		name    string
		kind    inventory.Kind
		at      string
		wantSub string
	}{
		{"VertexAI Endpoint", inventory.KindVertexAI, "aiplatform.googleapis.com/Endpoint", "Endpoint"},
		{"VertexAI Model", inventory.KindVertexAI, "aiplatform.googleapis.com/Model", "Model"},
		{"Apigee Organization", inventory.KindApigee, "apigee.googleapis.com/Organization", "Organization"},
		{"Apigee ApiProxy", inventory.KindApigee, "apigee.googleapis.com/ApiProxy", "ApiProxy"},
		{"Firebase Project", inventory.KindFirebase, "firebase.googleapis.com/FirebaseProject", "Project"},
		{"AppEngine Application", inventory.KindAppEngine, "appengine.googleapis.com/Application", "Application"},
		{"BigQuery Dataset", inventory.KindBigQuery, "bigquery.googleapis.com/Dataset", "Dataset"},
		{"BigQuery Table", inventory.KindBigQuery, "bigquery.googleapis.com/Table", "Table"},
		{"DNS ManagedZone", inventory.KindDNS, "dns.googleapis.com/ManagedZone", "ManagedZone"},
		{"Memorystore Redis", inventory.KindMemorystore, "redis.googleapis.com/Instance", "Redis"},
		{"Memorystore Memcache", inventory.KindMemorystore, "memcache.googleapis.com/Instance", "Memcache"},
		{"ArtifactRegistry Repo", inventory.KindArtifactRegistry, "artifactregistry.googleapis.com/Repository", "Repository"},
		{"CloudScheduler Job", inventory.KindCloudScheduler, "cloudscheduler.googleapis.com/Job", "Job"},
		{"PubSub Topic", inventory.KindPubSub, "pubsub.googleapis.com/Topic", "Topic"},
		{"Spanner Instance", inventory.KindSpanner, "spanner.googleapis.com/Instance", "Instance"},
		{"Bigtable Instance", inventory.KindBigtable, "bigtableadmin.googleapis.com/Instance", "Instance"},
		{"KMS CryptoKey", inventory.KindKMS, "cloudkms.googleapis.com/CryptoKey", "CryptoKey"},
		{"SecretManager Secret", inventory.KindSecretManager, "secretmanager.googleapis.com/Secret", "Secret"},
		{"Dataflow Job", inventory.KindDataflow, "dataflow.googleapis.com/Job", "Job"},
		{"Dataproc Cluster", inventory.KindDataproc, "dataproc.googleapis.com/Cluster", "Cluster"},
		{"Composer Environment", inventory.KindComposer, "composer.googleapis.com/Environment", "Environment"},
		{"CloudTasks Queue", inventory.KindCloudTasks, "cloudtasks.googleapis.com/Queue", "Queue"},
		{"Monitoring AlertPolicy", inventory.KindMonitoring, "monitoring.googleapis.com/AlertPolicy", "AlertPolicy"},
		{"Logging LogSink", inventory.KindLogging, "logging.googleapis.com/LogSink", "LogSink"},
		{"OSConfig PatchDeployment", inventory.KindOSConfig, "osconfig.googleapis.com/PatchDeployment", "PatchDeployment"},
		{"VPN VpnTunnel", inventory.KindVPN, "compute.googleapis.com/VpnTunnel", "VpnTunnel"},
		{"Router", inventory.KindRouter, "compute.googleapis.com/Router", "Router"},
		{"CloudBuild BuildTrigger", inventory.KindCloudBuild, "cloudbuild.googleapis.com/BuildTrigger", "BuildTrigger"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stubDetailForKind(tc.kind, tc.at)
			require.NotNil(t, got)
			assert.Equal(t, tc.wantSub, got.Subtype)
		})
	}
}

func TestStubDetailForKindFallsBackToOther(t *testing.T) {
	// Known stub Kind + unknown asset type → Subtype "Other".
	got := stubDetailForKind(inventory.KindVertexAI, "aiplatform.googleapis.com/FutureResource")
	require.NotNil(t, got)
	assert.Equal(t, "Other", got.Subtype)
}

func TestStubDetailForKindRejectsNonStub(t *testing.T) {
	// Non-stub Kinds return nil.
	assert.Nil(t, stubDetailForKind(inventory.KindVM, "compute.googleapis.com/Instance"))
	assert.Nil(t, stubDetailForKind(inventory.KindDatabase, "sqladmin.googleapis.com/Instance"))
	assert.Nil(t, stubDetailForKind("", ""))
}

func TestIsStubKindAssetType(t *testing.T) {
	assert.True(t, isStubKindAssetType("aiplatform.googleapis.com/Endpoint"))
	assert.True(t, isStubKindAssetType("apigee.googleapis.com/Organization"))
	assert.True(t, isStubKindAssetType("bigquery.googleapis.com/Table"))
	assert.False(t, isStubKindAssetType("compute.googleapis.com/Instance"))
	assert.False(t, isStubKindAssetType("storage.googleapis.com/Bucket"))
	assert.False(t, isStubKindAssetType(""))
}

func TestStubKindsContainsAllEntries(t *testing.T) {
	got := stubKinds()
	assert.Len(t, got, len(subtypeMaps))
	// Every Kind in subtypeMaps appears in the output.
	gotSet := make(map[inventory.Kind]bool, len(got))
	for _, k := range got {
		gotSet[k] = true
	}
	for k := range subtypeMaps {
		assert.True(t, gotSet[k], "stubKinds() missing %s", k)
	}
}
