package inventory

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
	SizeBytes    int64
}

// LoadBalancerDetail flattens GCP's multi-resource LB composition into one row.
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
	Version     string
	NodeCount   int32
	NodeMachine string
	NodeDiskGB  int64
	Autopilot   bool
	Location    string
}

// FunctionDetail normalizes Cloud Run services and Cloud Functions gen2.
type FunctionDetail struct {
	Runtime   string
	Trigger   string
	MemoryMiB int64
	CPUs      float64
	MaxInst   int32
	Region    string
}

// VertexDetail describes a Vertex AI resource surfaced via Cloud Asset Inventory.
// cloudcmder lists these as stubs (name/region/status from CAI) — no Phase-2
// enricher is registered, so Detail carries only the subtype label.
type VertexDetail struct {
	Subtype string // Endpoint | Model | Dataset | Index | IndexEndpoint | PipelineJob | TrainingPipeline | Featurestore | FeatureGroup | NotebookRuntime | ... | Other
	Region  string
}
