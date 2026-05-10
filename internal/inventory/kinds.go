// Package inventory defines the provider-agnostic types that every cloud
// implementation must speak. The store, TUI, and exporter consume these types;
// individual providers (gcp, aws, ...) produce them.
package inventory

// Kind enumerates the resource categories cloudcmder normalizes across clouds.
type Kind string

const (
	KindVM           Kind = "VM"
	KindDisk         Kind = "Disk"
	KindDatabase     Kind = "Database"
	KindBucket       Kind = "Bucket"
	KindLoadBalancer Kind = "LoadBalancer"
	KindNetwork      Kind = "Network"
	KindSubnet       Kind = "Subnet"
	KindFirewall     Kind = "Firewall"
	KindCluster      Kind = "Cluster"
	KindFunction     Kind = "Function"
	KindVertexAI     Kind = "VertexAI"

	// Stub-only Kinds — surfaced via Cloud Asset Inventory Phase 1 only.
	// No Phase-2 enricher; Detail is *StubDetail{Subtype, Region}.
	KindApigee           Kind = "Apigee"
	KindFirebase         Kind = "Firebase"
	KindAppEngine        Kind = "AppEngine"
	KindBigQuery         Kind = "BigQuery"
	KindDNS              Kind = "DNS"
	KindMemorystore      Kind = "Memorystore"
	KindArtifactRegistry Kind = "ArtifactRegistry"
	KindCloudScheduler   Kind = "CloudScheduler"
	KindPubSub           Kind = "PubSub"
	KindSpanner          Kind = "Spanner"
	KindBigtable         Kind = "Bigtable"
	KindKMS              Kind = "KMS"
	KindSecretManager    Kind = "SecretManager"
	KindDataflow         Kind = "Dataflow"
	KindDataproc         Kind = "Dataproc"
	KindComposer         Kind = "Composer"
	KindCloudTasks       Kind = "CloudTasks"
	KindMonitoring       Kind = "Monitoring"
	KindLogging          Kind = "Logging"
	KindOSConfig         Kind = "OSConfig"
	KindVPN              Kind = "VPN"
	KindRouter           Kind = "Router"
	KindCloudBuild       Kind = "CloudBuild"
)

// RefKind labels the directed edges in the interconnection graph.
type RefKind string

const (
	RefAttachedTo  RefKind = "AttachedTo"
	RefBackendOf   RefKind = "BackendOf"
	RefMemberOf    RefKind = "MemberOf"
	RefRoutesFrom  RefKind = "RoutesFrom"
	RefProtectedBy RefKind = "ProtectedBy"
	RefFrontendOf  RefKind = "FrontendOf"
)
