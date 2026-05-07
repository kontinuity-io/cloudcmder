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
