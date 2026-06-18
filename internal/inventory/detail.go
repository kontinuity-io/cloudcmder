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
	Location     string
	StorageClass string
	PublicAccess bool
	Versioning   bool
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
// Apigee, and the other CAI-listed Kinds without a Phase-2 enricher. Detail
// carries only the Subtype label derived from the CAI asset type string.
//
// Enriched GCP Kinds (BigQuery, PubSub, …) embed the same Subtype/Region pair
// at the top of their own Detail struct so a CAI Phase-1 stub row (serialized
// as {"Subtype":…,"Region":…}) decodes losslessly into the richer struct when
// the Phase-2 enricher did not cover that particular asset subtype.
type StubDetail struct {
	Subtype string // type suffix, e.g. "Endpoint", "Dataset", "Topic", "Organization", …, "Other"
	Region  string
}

// BigQueryDetail describes a BigQuery dataset and the project's reservation
// capacity. Enriched at the dataset grain; Table/Model/Routine stub rows keep
// only Subtype/Region (the remaining fields stay zero and render as "—").
type BigQueryDetail struct {
	Subtype      string
	Region       string
	LocationType string // "multi-region" | "region"
	StorageBytes int64  // total logical bytes across the dataset's tables
	TableCount   int
	Edition      string // STANDARD | ENTERPRISE | ENTERPRISE_PLUS (project reservation; best-effort)
	Slots        int64  // reservation slot capacity for the project (best-effort)
}

// PubSubDetail describes a Pub/Sub topic or subscription. Enriched at the
// Topic and Subscription grain in one pass; Schema/Snapshot stub rows keep
// only Subtype/Region (the remaining fields stay zero and render as "—").
type PubSubDetail struct {
	Subtype           string // "Topic" | "Subscription"
	Region            string
	DeliveryType      string // subscriptions: "push" | "pull" | "bigquery" | "cloudstorage"; empty for topics
	SubscriptionCount int    // topics: number of attached subscriptions
	MessageRetention  string // human duration, e.g. "7d" / "10m" (topic or subscription retention)
	PublishedBytes    int64  // topics: published bytes over the metric window (best-effort via monitoring; 0 if unavailable)
}
