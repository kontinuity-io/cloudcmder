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
	// KindGCP* identifiers use the GCP prefix to distinguish them from the
	// cloud-neutral Kinds above. Wire format (the string stored in the DB)
	// is kept stable so existing DBs load correctly after this rename.
	KindGCPVertexAI Kind = "VertexAI"

	// Stub-only GCP Kinds — surfaced via Cloud Asset Inventory Phase 1 only.
	// No Phase-2 enricher; Detail is *StubDetail{Subtype, Region}.
	// AWS-specific Kinds will follow the same KindAWS* naming convention.
	KindGCPApigee           Kind = "Apigee"
	KindGCPFirebase         Kind = "Firebase"
	KindGCPAppEngine        Kind = "AppEngine"
	KindGCPBigQuery         Kind = "BigQuery"
	KindGCPDNS              Kind = "DNS"
	KindGCPMemorystore      Kind = "Memorystore"
	KindGCPArtifactRegistry Kind = "ArtifactRegistry"
	KindGCPCloudScheduler   Kind = "CloudScheduler"
	KindGCPPubSub           Kind = "PubSub"
	KindGCPSpanner          Kind = "Spanner"
	KindGCPBigtable         Kind = "Bigtable"
	KindGCPKMS              Kind = "KMS"
	KindGCPSecretManager    Kind = "SecretManager"
	KindGCPDataflow         Kind = "Dataflow"
	KindGCPDataproc         Kind = "Dataproc"
	KindGCPComposer         Kind = "Composer"
	KindGCPCloudTasks       Kind = "CloudTasks"
	KindGCPMonitoring       Kind = "Monitoring"
	KindGCPLogging          Kind = "Logging"
	KindGCPOSConfig         Kind = "OSConfig"
	KindGCPVPN              Kind = "VPN"
	KindGCPRouter           Kind = "Router"
	KindGCPCloudBuild       Kind = "CloudBuild"
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
