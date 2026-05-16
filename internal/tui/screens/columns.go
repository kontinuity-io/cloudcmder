package screens

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/tui/style"
)

// ColumnDef describes one column in a ResourceList table. Extract receives
// the already-decoded Detail (kind-specific struct) so each column doesn't
// pay the JSON-unmarshal cost separately.
type ColumnDef struct {
	Header  string
	Width   int
	Extract func(r inventory.Resource, detail any) string
}

// columnsFor returns the column set registered for the given Kind, with
// each column's Width adapted to fit availableWidth. The hardcoded widths
// in the per-kind functions are treated as relative weights — at narrow
// terminals they shrink proportionally; at wide terminals they fit
// naturally. Pass availableWidth=0 to skip the fit step (tests, exporter
// reuse). The second return value reports whether a kind-specific column
// set exists.
func columnsFor(kind inventory.Kind, availableWidth int) ([]ColumnDef, bool) {
	var cols []ColumnDef
	switch kind {
	case inventory.KindVM:
		cols = vmColumns()
	case inventory.KindDisk:
		cols = diskColumns()
	case inventory.KindNetwork:
		cols = networkColumns()
	case inventory.KindSubnet:
		cols = subnetColumns()
	case inventory.KindFirewall:
		cols = firewallColumns()
	case inventory.KindLoadBalancer:
		cols = loadBalancerColumns()
	case inventory.KindDatabase:
		cols = databaseColumns()
	case inventory.KindCluster:
		cols = clusterColumns()
	case inventory.KindBucket:
		cols = bucketColumns()
	case inventory.KindFunction:
		cols = functionColumns()
	case inventory.KindVertexAI,
		inventory.KindApigee,
		inventory.KindFirebase,
		inventory.KindAppEngine,
		inventory.KindBigQuery,
		inventory.KindDNS,
		inventory.KindMemorystore,
		inventory.KindArtifactRegistry,
		inventory.KindCloudScheduler,
		inventory.KindPubSub,
		inventory.KindSpanner,
		inventory.KindBigtable,
		inventory.KindKMS,
		inventory.KindSecretManager,
		inventory.KindDataflow,
		inventory.KindDataproc,
		inventory.KindComposer,
		inventory.KindCloudTasks,
		inventory.KindMonitoring,
		inventory.KindLogging,
		inventory.KindOSConfig,
		inventory.KindVPN,
		inventory.KindRouter,
		inventory.KindCloudBuild:
		cols = stubColumns()
	default:
		return nil, false
	}
	if availableWidth > 0 {
		fitColumnWidths(cols, availableWidth)
	}
	return cols, true
}

// fitColumnWidths shrinks the per-column Width values so the rendered
// table fits inside availableWidth. Natural widths that already fit are
// left alone; narrow terminals shrink each column proportionally with a
// 4-rune floor.
func fitColumnWidths(cols []ColumnDef, availableWidth int) {
	if len(cols) == 0 {
		return
	}
	const minWidth = 4
	const cellPadding = 2
	budget := availableWidth - cellPadding*len(cols)
	if budget <= minWidth*len(cols) {
		// Terminal too narrow to shrink fairly — leave defaults; bubbles/
		// table will horizontally clip on render.
		return
	}
	sum := 0
	for _, c := range cols {
		sum += c.Width
	}
	if sum <= budget {
		return
	}
	used := 0
	for i := range cols {
		scaled := cols[i].Width * budget / sum
		if scaled < minWidth {
			scaled = minWidth
		}
		cols[i].Width = scaled
		used += scaled
	}
	// Spend any remainder on the first column (typically NAME) so the
	// total matches the budget within rounding error.
	if r := budget - used; r > 0 {
		cols[0].Width += r
	}
}

// decodeDetail unmarshals the json.RawMessage that LoadResources places in
// Resource.Detail into the kind-specific struct. Returns nil for unknown
// kinds — extractors must guard against that.
func decodeDetail(kind inventory.Kind, raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	switch kind {
	case inventory.KindVM:
		return unmarshalOrNil(raw, &inventory.VMDetail{})
	case inventory.KindDisk:
		return unmarshalOrNil(raw, &inventory.DiskDetail{})
	case inventory.KindNetwork:
		return unmarshalOrNil(raw, &inventory.NetworkDetail{})
	case inventory.KindSubnet:
		return unmarshalOrNil(raw, &inventory.SubnetDetail{})
	case inventory.KindFirewall:
		return unmarshalOrNil(raw, &inventory.FirewallDetail{})
	case inventory.KindLoadBalancer:
		return unmarshalOrNil(raw, &inventory.LoadBalancerDetail{})
	case inventory.KindDatabase:
		return unmarshalOrNil(raw, &inventory.DatabaseDetail{})
	case inventory.KindCluster:
		return unmarshalOrNil(raw, &inventory.ClusterDetail{})
	case inventory.KindBucket:
		return unmarshalOrNil(raw, &inventory.BucketDetail{})
	case inventory.KindFunction:
		return unmarshalOrNil(raw, &inventory.FunctionDetail{})
	case inventory.KindVertexAI,
		inventory.KindApigee,
		inventory.KindFirebase,
		inventory.KindAppEngine,
		inventory.KindBigQuery,
		inventory.KindDNS,
		inventory.KindMemorystore,
		inventory.KindArtifactRegistry,
		inventory.KindCloudScheduler,
		inventory.KindPubSub,
		inventory.KindSpanner,
		inventory.KindBigtable,
		inventory.KindKMS,
		inventory.KindSecretManager,
		inventory.KindDataflow,
		inventory.KindDataproc,
		inventory.KindComposer,
		inventory.KindCloudTasks,
		inventory.KindMonitoring,
		inventory.KindLogging,
		inventory.KindOSConfig,
		inventory.KindVPN,
		inventory.KindRouter,
		inventory.KindCloudBuild:
		return unmarshalOrNil(raw, &inventory.StubDetail{})
	}
	return nil
}

func unmarshalOrNil(raw json.RawMessage, into any) any {
	if err := json.Unmarshal(raw, into); err != nil {
		return nil
	}
	return into
}

// AliasToKind maps the cmdbar `:alias` strings (per architecture.md line 484)
// to inventory.Kind values. ok=false for unknown aliases.
func AliasToKind(alias string) (inventory.Kind, bool) {
	switch strings.ToLower(alias) {
	case "vm":
		return inventory.KindVM, true
	case "disk":
		return inventory.KindDisk, true
	case "db":
		return inventory.KindDatabase, true
	case "lb":
		return inventory.KindLoadBalancer, true
	case "net":
		return inventory.KindNetwork, true
	case "subnet":
		return inventory.KindSubnet, true
	case "fw":
		return inventory.KindFirewall, true
	case "gke":
		return inventory.KindCluster, true
	case "bucket":
		return inventory.KindBucket, true
	case "fn":
		return inventory.KindFunction, true
	case "vertex", "ai":
		return inventory.KindVertexAI, true
	case "apigee":
		return inventory.KindApigee, true
	case "firebase", "fb":
		return inventory.KindFirebase, true
	case "appengine", "gae", "ae":
		return inventory.KindAppEngine, true
	case "bigquery", "bq":
		return inventory.KindBigQuery, true
	case "dns":
		return inventory.KindDNS, true
	case "memorystore", "redis", "memcache":
		return inventory.KindMemorystore, true
	case "artifactregistry", "ar":
		return inventory.KindArtifactRegistry, true
	case "scheduler", "cron":
		return inventory.KindCloudScheduler, true
	case "pubsub", "ps":
		return inventory.KindPubSub, true
	case "spanner":
		return inventory.KindSpanner, true
	case "bigtable", "bt":
		return inventory.KindBigtable, true
	case "kms":
		return inventory.KindKMS, true
	case "secrets", "sm":
		return inventory.KindSecretManager, true
	case "dataflow", "df":
		return inventory.KindDataflow, true
	case "dataproc", "dp":
		return inventory.KindDataproc, true
	case "composer", "airflow":
		return inventory.KindComposer, true
	case "tasks":
		return inventory.KindCloudTasks, true
	case "monitoring", "stackdriver":
		return inventory.KindMonitoring, true
	case "logging", "logs":
		return inventory.KindLogging, true
	case "osconfig", "vmm":
		return inventory.KindOSConfig, true
	case "vpn":
		return inventory.KindVPN, true
	case "router":
		return inventory.KindRouter, true
	case "build", "cb":
		return inventory.KindCloudBuild, true
	}
	return "", false
}

// AllAliases returns every cmdbar alias in declaration order. Used by the
// cmdbar to seed its fuzzy-suggestion corpus. "scopes" doesn't map to a
// Kind — App.Update special-cases it and pushes the ScopesModal instead.
func AllAliases() []string {
	return []string{
		"ae", "apigee", "ar", "ai",
		"bq", "bigtable", "bt", "bucket", "build",
		"cb", "cron", "composer",
		"dataflow", "dataproc", "db", "df", "disk", "dns", "dp",
		"fb", "firebase", "fn", "fw",
		"gae", "gke",
		"kms",
		"lb", "logging", "logs",
		"memcache", "memorystore", "monitoring",
		"net",
		"osconfig",
		"ps", "pubsub",
		"redis", "router",
		"scheduler", "secrets", "sm", "spanner", "stackdriver", "subnet",
		"tasks",
		"vertex", "vmm", "vpn",
		"scopes",
	}
}

// --- VM --------------------------------------------------------------------

func vmColumns() []ColumnDef {
	// Widths are relative weights; columnsFor scales them to the actual
	// pane width via fitColumnWidths. At ≥120-col terminals the natural
	// widths fit; narrower terminals get proportionally shrunken cells.
	return []ColumnDef{
		{Header: "NAME", Width: 20, Extract: func(r inventory.Resource, _ any) string { return r.Name }},
		{Header: "ZONE", Width: 14, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil {
				return vm.Zone
			}
			return ""
		}},
		{Header: "MACHINE", Width: 13, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil {
				return vm.MachineType
			}
			return ""
		}},
		{Header: "vCPU", Width: 5, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil && vm.VCPUs > 0 {
				return fmt.Sprintf("%d", vm.VCPUs)
			}
			return ""
		}},
		{Header: "RAM", Width: 6, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil && vm.MemoryMiB > 0 {
				return fmt.Sprintf("%.1f", float64(vm.MemoryMiB)/1024.0)
			}
			return ""
		}},
		{Header: "OS", Width: 10, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil {
				return vm.OSFamily
			}
			return ""
		}},
		{Header: "MARKETPLACE", Width: 12, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil {
				return vm.LicenseClass
			}
			return ""
		}},
		{Header: "STATUS", Width: 10, Extract: statusOf},
	}
}

func vmOf(d any) *inventory.VMDetail {
	if vm, ok := d.(*inventory.VMDetail); ok {
		return vm
	}
	return nil
}

// --- Disk ------------------------------------------------------------------

func diskColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 24, Extract: nameOf},
		{Header: "ZONE", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if dd, ok := d.(*inventory.DiskDetail); ok && dd != nil {
				return dd.Zone
			}
			return ""
		}},
		{Header: "TYPE", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if dd, ok := d.(*inventory.DiskDetail); ok && dd != nil {
				return dd.Type
			}
			return ""
		}},
		{Header: "SIZE GB", Width: 8, Extract: func(_ inventory.Resource, d any) string {
			if dd, ok := d.(*inventory.DiskDetail); ok && dd != nil && dd.SizeGB > 0 {
				return fmt.Sprintf("%d", dd.SizeGB)
			}
			return ""
		}},
		{Header: "OS", Width: 10, Extract: func(_ inventory.Resource, d any) string {
			if dd, ok := d.(*inventory.DiskDetail); ok && dd != nil {
				return dd.LicenseProject
			}
			return ""
		}},
		{Header: "MARKETPLACE", Width: 12, Extract: func(_ inventory.Resource, d any) string {
			if dd, ok := d.(*inventory.DiskDetail); ok && dd != nil {
				return dd.LicenseClass
			}
			return ""
		}},
		{Header: "ATTACHED", Width: 8, Extract: func(_ inventory.Resource, d any) string {
			if dd, ok := d.(*inventory.DiskDetail); ok && dd != nil {
				return fmt.Sprintf("%d", len(dd.InUseBy))
			}
			return ""
		}},
		{Header: "STATUS", Width: 10, Extract: statusOf},
	}
}

// --- Network ---------------------------------------------------------------

func networkColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 28, Extract: nameOf},
		{Header: "AUTO SUBNET", Width: 12, Extract: func(_ inventory.Resource, d any) string {
			if nd, ok := d.(*inventory.NetworkDetail); ok && nd != nil {
				return boolStr(nd.AutoSubnet)
			}
			return ""
		}},
		{Header: "IPV4", Width: 18, Extract: func(_ inventory.Resource, d any) string {
			if nd, ok := d.(*inventory.NetworkDetail); ok && nd != nil {
				return nd.IPv4Range
			}
			return ""
		}},
	}
}

// --- Subnet ----------------------------------------------------------------

func subnetColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 24, Extract: nameOf},
		{Header: "REGION", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if sd, ok := d.(*inventory.SubnetDetail); ok && sd != nil {
				return sd.Region
			}
			return ""
		}},
		{Header: "NETWORK", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if sd, ok := d.(*inventory.SubnetDetail); ok && sd != nil {
				return sd.Network
			}
			return ""
		}},
		{Header: "CIDR", Width: 18, Extract: func(_ inventory.Resource, d any) string {
			if sd, ok := d.(*inventory.SubnetDetail); ok && sd != nil {
				return sd.CIDR
			}
			return ""
		}},
		{Header: "PRIVATE", Width: 8, Extract: func(_ inventory.Resource, d any) string {
			if sd, ok := d.(*inventory.SubnetDetail); ok && sd != nil {
				return boolStr(sd.Private)
			}
			return ""
		}},
	}
}

// --- Firewall --------------------------------------------------------------

func firewallColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 24, Extract: nameOf},
		{Header: "DIR", Width: 8, Extract: func(_ inventory.Resource, d any) string {
			if fd, ok := d.(*inventory.FirewallDetail); ok && fd != nil {
				return fd.Direction
			}
			return ""
		}},
		{Header: "PRI", Width: 6, Extract: func(_ inventory.Resource, d any) string {
			if fd, ok := d.(*inventory.FirewallDetail); ok && fd != nil {
				return fmt.Sprintf("%d", fd.Priority)
			}
			return ""
		}},
		{Header: "SOURCES", Width: 20, Extract: func(_ inventory.Resource, d any) string {
			if fd, ok := d.(*inventory.FirewallDetail); ok && fd != nil {
				return strings.Join(fd.SourceRanges, ",")
			}
			return ""
		}},
		{Header: "TAGS", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if fd, ok := d.(*inventory.FirewallDetail); ok && fd != nil {
				return strings.Join(fd.TargetTags, ",")
			}
			return ""
		}},
	}
}

// --- LoadBalancer ----------------------------------------------------------

func loadBalancerColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 24, Extract: nameOf},
		{Header: "REGION", Width: 14, Extract: func(r inventory.Resource, _ any) string { return r.Region }},
		{Header: "SCHEME", Width: 18, Extract: func(_ inventory.Resource, d any) string {
			if lb, ok := d.(*inventory.LoadBalancerDetail); ok && lb != nil {
				return lb.Scheme
			}
			return ""
		}},
		{Header: "PROTO", Width: 7, Extract: func(_ inventory.Resource, d any) string {
			if lb, ok := d.(*inventory.LoadBalancerDetail); ok && lb != nil {
				return lb.Protocol
			}
			return ""
		}},
		{Header: "IP", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if lb, ok := d.(*inventory.LoadBalancerDetail); ok && lb != nil {
				return lb.IPAddress
			}
			return ""
		}},
	}
}

// --- Database --------------------------------------------------------------

func databaseColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 22, Extract: nameOf},
		{Header: "REGION", Width: 12, Extract: func(r inventory.Resource, _ any) string { return r.Region }},
		{Header: "ENGINE", Width: 14, Extract: func(_ inventory.Resource, d any) string {
			if dd, ok := d.(*inventory.DatabaseDetail); ok && dd != nil {
				return dd.Engine
			}
			return ""
		}},
		{Header: "TIER", Width: 18, Extract: func(_ inventory.Resource, d any) string {
			if dd, ok := d.(*inventory.DatabaseDetail); ok && dd != nil {
				return dd.Tier
			}
			return ""
		}},
		{Header: "STORAGE GB", Width: 10, Extract: func(_ inventory.Resource, d any) string {
			if dd, ok := d.(*inventory.DatabaseDetail); ok && dd != nil && dd.StorageGB > 0 {
				return fmt.Sprintf("%d", dd.StorageGB)
			}
			return ""
		}},
		{Header: "STATUS", Width: 10, Extract: statusOf},
	}
}

// --- Cluster ---------------------------------------------------------------

func clusterColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 22, Extract: nameOf},
		{Header: "LOCATION", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if cd, ok := d.(*inventory.ClusterDetail); ok && cd != nil {
				return cd.Location
			}
			return ""
		}},
		{Header: "VERSION", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if cd, ok := d.(*inventory.ClusterDetail); ok && cd != nil {
				return cd.Version
			}
			return ""
		}},
		{Header: "NODES", Width: 6, Extract: func(_ inventory.Resource, d any) string {
			if cd, ok := d.(*inventory.ClusterDetail); ok && cd != nil {
				return fmt.Sprintf("%d", cd.NodeCount)
			}
			return ""
		}},
		{Header: "AUTOPILOT", Width: 10, Extract: func(_ inventory.Resource, d any) string {
			if cd, ok := d.(*inventory.ClusterDetail); ok && cd != nil {
				return boolStr(cd.Autopilot)
			}
			return ""
		}},
		{Header: "STATUS", Width: 10, Extract: statusOf},
	}
}

// --- Bucket ----------------------------------------------------------------

func bucketColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 32, Extract: nameOf},
		{Header: "LOCATION", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if bd, ok := d.(*inventory.BucketDetail); ok && bd != nil {
				return bd.Location
			}
			return ""
		}},
		{Header: "CLASS", Width: 10, Extract: func(_ inventory.Resource, d any) string {
			if bd, ok := d.(*inventory.BucketDetail); ok && bd != nil {
				return bd.StorageClass
			}
			return ""
		}},
		{Header: "PUBLIC", Width: 8, Extract: func(_ inventory.Resource, d any) string {
			if bd, ok := d.(*inventory.BucketDetail); ok && bd != nil {
				return boolStr(bd.PublicAccess)
			}
			return ""
		}},
		{Header: "VERSION", Width: 8, Extract: func(_ inventory.Resource, d any) string {
			if bd, ok := d.(*inventory.BucketDetail); ok && bd != nil {
				return boolStr(bd.Versioning)
			}
			return ""
		}},
		{Header: "SIZE", Width: 9, Extract: func(_ inventory.Resource, d any) string {
			if bd, ok := d.(*inventory.BucketDetail); ok && bd != nil {
				return formatBytes(bd.SizeBytes)
			}
			return ""
		}},
		{Header: "OBJECTS", Width: 10, Extract: func(_ inventory.Resource, d any) string {
			if bd, ok := d.(*inventory.BucketDetail); ok && bd != nil {
				return formatCount(bd.ObjectCount)
			}
			return ""
		}},
	}
}

// --- Function --------------------------------------------------------------

func functionColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 24, Extract: nameOf},
		{Header: "REGION", Width: 12, Extract: func(r inventory.Resource, _ any) string { return r.Region }},
		{Header: "RUNTIME", Width: 14, Extract: func(_ inventory.Resource, d any) string {
			if fd, ok := d.(*inventory.FunctionDetail); ok && fd != nil {
				return fd.Runtime
			}
			return ""
		}},
		{Header: "MEM MIB", Width: 8, Extract: func(_ inventory.Resource, d any) string {
			if fd, ok := d.(*inventory.FunctionDetail); ok && fd != nil && fd.MemoryMiB > 0 {
				return fmt.Sprintf("%d", fd.MemoryMiB)
			}
			return ""
		}},
		{Header: "CPU", Width: 5, Extract: func(_ inventory.Resource, d any) string {
			if fd, ok := d.(*inventory.FunctionDetail); ok && fd != nil && fd.CPUs > 0 {
				return fmt.Sprintf("%g", fd.CPUs)
			}
			return ""
		}},
		{Header: "TRIGGER", Width: 14, Extract: func(_ inventory.Resource, d any) string {
			if fd, ok := d.(*inventory.FunctionDetail); ok && fd != nil {
				return fd.Trigger
			}
			return ""
		}},
	}
}

// --- Stub-only Kinds (VertexAI, Apigee, Firebase, BigQuery, …) ----------------

// stubColumns returns the standard 4-column layout shared by all stub-only Kinds:
// NAME · SUBTYPE · REGION · STATUS.
func stubColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 24, Extract: nameOf},
		{Header: "SUBTYPE", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if sd, ok := d.(*inventory.StubDetail); ok && sd != nil {
				return sd.Subtype
			}
			return ""
		}},
		{Header: "REGION", Width: 14, Extract: func(r inventory.Resource, _ any) string { return r.Region }},
		{Header: "STATUS", Width: 10, Extract: statusOf},
	}
}

// selectedRowStyles returns the bubbles/table styles used by every left
// pane in v1.2 — accent-on-dark for the selected row, dim for the header
// rule. Centralised so the row contrast looks consistent across panes.
func selectedRowStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		Foreground(style.ColorAccent).
		Bold(true).
		BorderForeground(style.ColorDim)
	s.Selected = s.Selected.
		Foreground(style.ColorAccent).
		Background(style.ColorSelectedBg).
		Bold(true)
	return s
}

// --- shared helpers --------------------------------------------------------

func nameOf(r inventory.Resource, _ any) string { return r.Name }

func statusOf(r inventory.Resource, _ any) string {
	return style.StatusBullet(r.Status) + " " + r.Status
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// formatBytes renders byte counts as "1.2 TB", "256 MB", etc. Returns "—"
// for zero — Cloud Monitoring values are daily aggregates, so a bucket
// younger than the first sample window legitimately has no data; the dash
// signals "not yet measured" rather than "zero bytes."
func formatBytes(n int64) string {
	if n <= 0 {
		return "—"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	// IEC units to match cloud-console display conventions.
	suffixes := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < len(suffixes)-1; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), suffixes[exp])
}

// formatCount renders integer counts as 1.2K / 3.4M / 5.6B. Same em-dash
// fallback for zero as formatBytes.
func formatCount(n int64) string {
	if n <= 0 {
		return "—"
	}
	switch {
	case n < 1_000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	case n < 1_000_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	default:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	}
}
