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

// ArtifactRegistryDetail captures one Artifact Registry repository's format,
// mode, and storage size. Subtype+Region lead the struct so it overwrites the
// CAI Phase-1 stub (StubDetail) by sharing the same field prefix on decode.
type ArtifactRegistryDetail struct {
	Subtype   string // "Repository"
	Region    string
	Format    string // DOCKER | MAVEN | NPM | PYTHON | APT | YUM | GO | KFP | ...
	Mode      string // STANDARD_REPOSITORY | REMOTE_REPOSITORY | VIRTUAL_REPOSITORY
	SizeBytes int64  // repository size in bytes (Repository.SizeBytes)
}

// AppEngineDetail captures the enriched spec for a GCP App Engine Application.
// Subtype and Region MUST be the first two fields so a StubDetail JSON payload
// decodes losslessly into this struct (Phase-1 stub overwrite compatibility).
type AppEngineDetail struct {
	Subtype string // always "Application" for enriched rows
	Region  string // GCP region, e.g. "us-central1"
	// App Engine–specific fields populated by Phase-2 enrichment via GetApplication.
	ServingStatus   string // e.g. "SERVING", "DISABLED", "USER_DISABLED"
	LocationID      string // App Engine location id, e.g. "us-central"
	DefaultHostname string // e.g. "myapp.appspot.com"
	AuthDomain      string // Google Apps domain controlling access, e.g. "gmail.com"
	DatabaseType    string // e.g. "CLOUD_FIRESTORE", "CLOUD_DATASTORE_COMPATIBILITY"
	ServiceCount    int    // number of services (from ListServices)
}

// FirebaseDetail captures the enriched fields for a Firebase project.
// Subtype and Region MUST remain the first two fields so a StubDetail JSON
// blob decodes losslessly into FirebaseDetail (INSERT OR REPLACE overwrite).
type FirebaseDetail struct {
	Subtype         string // always "Project" for the Project grain
	Region          string // resource location ID (e.g. "us-central1"), may be empty
	DisplayName     string // user-assigned display name of the Firebase project
	ProjectNumber   int64  // globally unique Google-assigned project number
	LocationID      string // same as Region; kept for explicit naming clarity
	WebAppCount     int    // number of Web apps registered with this project
	AndroidAppCount int    // number of Android apps registered with this project
	IOSAppCount     int    // number of iOS apps registered with this project
	TotalApps       int    // WebAppCount + AndroidAppCount + IOSAppCount
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

// MemorystoreDetail describes a Memorystore cache instance. The three GCP
// sub-services (classic Redis, Redis Cluster, Memcache) all normalize to
// KindGCPMemorystore; Subtype distinguishes them and which fields are
// populated. Fields that a given sub-service does not expose stay zero/empty
// and render as "—".
type MemorystoreDetail struct {
	Subtype      string // "Redis" | "RedisCluster" | "Memcache"
	Region       string
	ServiceType  string // same family label, e.g. "Redis", "Redis Cluster", "Memcached"
	NodeType     string // cluster node type / memcache nodeConfig; empty if N/A
	Tier         string // BASIC | STANDARD_HA for classic Redis; empty otherwise
	MemorySizeGB int    // provisioned memory (sum for memcache nodes * memory)
	ShardCount   int32  // cluster shard count; 0 for classic Redis/memcache
	ReplicaCount int32  // replicas per shard (cluster); 0 otherwise
	Version      string // redis/memcache engine version
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

// SecretManagerDetail describes a Secret Manager secret: its replication policy,
// active version count, and rotation configuration. Enriched at the secret
// grain (matching the CAI stub ID = secret short name).
type SecretManagerDetail struct {
	Subtype          string // "Secret"
	Region           string // "" for automatic (global) replication; region for user-managed
	ActiveVersions   int    // count of ENABLED versions across all locations
	Replication      string // "automatic" | "user-managed"
	RotationPeriod   string // human duration, e.g. "30d"; empty if no rotation
	RotationTopic    string // Pub/Sub topic for rotation notifications; empty if none
	AccessOperations int64  // access-version operations over the metric window (best-effort via monitoring; 0 if unavailable)
}

// LoggingDetail holds enriched Cloud Logging log-bucket fields. Subtype and
// Region MUST remain the first two fields so the Phase-2 overwrite pattern
// (INSERT OR REPLACE keyed on ResourceRef) works correctly.
type LoggingDetail struct {
	Subtype          string // "LogBucket" for enriched rows; other subtypes stay StubDetail
	Region           string // bucket location
	RetentionDays    int32  // configured retention; 0 = API default (30 days)
	Locked           bool   // locked bucket — retention cannot be shortened
	AnalyticsEnabled bool   // Log Analytics enabled on this bucket
	LifecycleState   string // "ACTIVE", "DELETE_REQUESTED", "CREATING", etc.
	StorageBytes     int64  // best-effort from Cloud Monitoring; 0 if unavailable
}

// MonitoringDetail holds enriched Cloud Monitoring alert-policy fields.
// Subtype and Region MUST remain the first two fields.
type MonitoringDetail struct {
	Subtype                  string // "AlertPolicy" for enriched rows
	Region                   string // always "global" for alert policies
	Enabled                  bool   // alert policy is enabled
	ConditionCount           int    // number of conditions on the policy
	Combiner                 string // "AND" | "OR" | "AND_WITH_MATCHING_RESOURCE"
	NotificationChannelCount int    // number of notification channels attached
}

// ProjectDetail captures project-level metadata surfaced once per scan as a
// synthetic KindGCPProject row (ID = project ID). It currently focuses on the
// Cloud Billing association. Subtype/Region lead the struct for convention
// parity with the other GCP Detail types — there is no CAI stub to overwrite.
type ProjectDetail struct {
	Subtype            string // always "Project"
	Region             string // always "global" (projects are not regional)
	BillingAccountID   string // short ID, e.g. "01ABCD-234567-89EFGH"; empty if not associated
	BillingAccountName string // billing account display name (best-effort; empty without billing.accounts.get)
	BillingEnabled     bool   // whether billing is enabled on the project
}
