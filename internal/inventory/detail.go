package inventory

// Accelerator captures one accelerator type and count attached to a VM or
// node pool. A slice allows for multi-type configurations.
type Accelerator struct {
	Type  string // short type name, e.g. "nvidia-tesla-t4", "nvidia-l4"
	Count int32
}

// VMDetail captures the compute-instance fields the TUI and exporter render.
type VMDetail struct {
	MachineType   string
	VCPUs         int32
	MemoryMiB     int64
	CPUPlatform   string
	OSFamily      string
	OSImage       string
	Preemptible   bool
	Spot          bool
	BootDisk      DiskRef
	AttachedDisks []DiskRef
	NICs          []NICDetail
	Zone          string
	// Marketplace fields — populated from attached disk license metadata.
	// MarketplaceClass precedence: "marketplace" > "paid" > "free" > "".
	Licenses           []string // license names aggregated across all attached disks
	MarketplaceProject string   // image project of the highest-precedence license
	MarketplaceClass   string   // "marketplace" | "paid" | "free" | ""
	Accelerators       []Accelerator
}

// DiskRef is a lightweight pointer to a disk used inside VMDetail; the full
// DiskDetail is fetched separately when the user drills into the disk.
type DiskRef struct {
	Name   string
	SizeGB int64
	Type   string
}

// NICDetail describes a single network interface attached to a VM.
type NICDetail struct {
	Network    string
	Subnetwork string
	InternalIP string
	ExternalIP string
}

// DiskDetail describes a persistent disk and the VMs it is attached to.
type DiskDetail struct {
	SizeGB   int64
	Type     string
	Zone     string
	InUseBy  []ResourceRef
	Snapshot string
	// Marketplace fields — same three-state classification as VMDetail.
	Licenses           []string
	MarketplaceProject string
	MarketplaceClass   string
}

// DatabaseDetail normalizes managed-database compute and storage shape.
type DatabaseDetail struct {
	Engine            string
	Tier              string
	VCPUs             int32
	MemoryMiB         int64
	StorageGB         int64
	StorageType       string
	HighAvailability  bool
	MaintenanceWindow string
}

// BucketDetail describes an object-storage bucket.
type BucketDetail struct {
	Location          string
	StorageClass      string
	PublicAccess      bool
	PublicAccessState string // "public", "not_public", "unknown"; empty for legacy rows
	Versioning        bool
	// SizeBytes and ObjectCount come from the provider's monitoring API (the
	// object-store API itself does not expose either). The metric is a daily
	// aggregate so freshly created buckets show 0 for ~24h until first sample.
	SizeBytes   int64
	ObjectCount int64
}

// LoadBalancerDetail flattens a provider's multi-resource LB composition into one row.
type LoadBalancerDetail struct {
	Scheme       string
	Protocol     string
	IPAddress    string
	Ports        []string
	BackendCount int
}

// NetworkDetail describes a VPC.
type NetworkDetail struct {
	AutoSubnet  bool
	IPv4Range   string
	SubnetCount int
}

// SubnetDetail describes a regional subnet inside a VPC.
type SubnetDetail struct {
	CIDR    string
	Region  string
	Network string
	Private bool
}

// FirewallDetail describes a firewall rule and its allow-list.
type FirewallDetail struct {
	Direction    string
	Priority     int32
	SourceRanges []string
	TargetTags   []string
	Allowed      []FirewallRule
}

// FirewallRule is one protocol+ports allow entry inside a firewall.
type FirewallRule struct {
	Protocol string
	Ports    []string
}

// ClusterDetail describes a managed Kubernetes cluster.
type ClusterDetail struct {
	Version      string
	NodeCount    int32
	NodeMachine  string
	NodeDiskGB   int64
	Serverless   bool // managed/serverless mode (GKE Autopilot, EKS Fargate, etc.)
	Location     string
	Accelerators []Accelerator // aggregated across all node pools
}

// FunctionDetail normalizes function-platform services (Cloud Run, Lambda, etc.).
type FunctionDetail struct {
	Runtime   string
	Trigger   string
	MemoryMiB int64
	CPUs      float64
	MaxInst   int32
	Region    string
}

// StubDetail is the shared Detail type for all stub-only Kinds: VertexAI,
// Apigee, Firebase, AppEngine, BigQuery, and the other 19 CAI-listed Kinds.
// No Phase-2 enricher is registered for any stub Kind; Detail carries only
// the Subtype label derived from the CAI asset type string.
type StubDetail struct {
	Subtype string // type suffix, e.g. "Endpoint", "Dataset", "Topic", "Organization", …, "Other"
	Region  string
}
