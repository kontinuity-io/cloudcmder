// Package export materialises a stored run into a multi-tab .xlsx workbook.
// Reads only from internal/store and decodes Detail JSON into the kind-
// specific structs in internal/inventory; never imports internal/providers/*.
package export

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cloudcmder.com/internal/inventory"
)

// ColumnDef describes one column in an export sheet. Extract receives the
// already-decoded Detail (kind-specific struct) so each column doesn't pay
// the JSON-unmarshal cost separately. Returns a string — analyst-grade
// numeric formulas can re-coerce in the consumer.
type ColumnDef struct {
	Header  string
	Extract func(r inventory.Resource, detail any) string
}

// columnsFor returns the export-side columns for a Kind. Richer than the TUI
// equivalent in internal/tui/screens/columns.go: every Detail field that the
// architecture documents lands on a sheet, slice/map fields rendered as
// semicolon-separated strings.
func columnsFor(kind inventory.Kind) []ColumnDef {
	switch kind {
	case inventory.KindVM:
		return vmColumns()
	case inventory.KindDisk:
		return diskColumns()
	case inventory.KindNetwork:
		return networkColumns()
	case inventory.KindSubnet:
		return subnetColumns()
	case inventory.KindFirewall:
		return firewallColumns()
	case inventory.KindLoadBalancer:
		return loadBalancerColumns()
	case inventory.KindDatabase:
		return databaseColumns()
	case inventory.KindCluster:
		return clusterColumns()
	case inventory.KindBucket:
		return bucketColumns()
	case inventory.KindFunction:
		return functionColumns()
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
		return stubColumns()
	}
	return nil
}

// decodeDetail unmarshals the json.RawMessage that LoadResources places in
// Resource.Detail into the kind-specific struct. Returns nil for unknown
// kinds; column extractors guard against that and emit empty strings.
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

// headersOf extracts the Header strings from a ColumnDef slice as an []any
// suitable for passing to excelize's SetRow.
func headersOf(cols []ColumnDef) []any {
	out := make([]any, len(cols))
	for i, c := range cols {
		out[i] = c.Header
	}
	return out
}

// --- shared helpers --------------------------------------------------------

func nameOf(r inventory.Resource, _ any) string   { return r.Name }
func regionOf(r inventory.Resource, _ any) string { return r.Region }
func statusOf(r inventory.Resource, _ any) string { return r.Status }
func labelsOf(r inventory.Resource, _ any) string { return joinLabels(r.Labels) }

func joinLabels(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(m))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ";")
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// --- VMs -------------------------------------------------------------------

func vmColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "Region", Extract: regionOf},
		{Header: "Status", Extract: statusOf},
		{Header: "MachineType", Extract: vmField(func(d *inventory.VMDetail) string { return d.MachineType })},
		{Header: "vCPUs", Extract: vmField(func(d *inventory.VMDetail) string {
			if d.VCPUs == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.VCPUs)
		})},
		{Header: "MemoryMiB", Extract: vmField(func(d *inventory.VMDetail) string {
			if d.MemoryMiB == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.MemoryMiB)
		})},
		{Header: "CPUPlatform", Extract: vmField(func(d *inventory.VMDetail) string { return d.CPUPlatform })},
		{Header: "OSFamily", Extract: vmField(func(d *inventory.VMDetail) string { return d.OSFamily })},
		{Header: "OSImage", Extract: vmField(func(d *inventory.VMDetail) string { return d.OSImage })},
		{Header: "Licenses", Extract: vmField(func(d *inventory.VMDetail) string { return strings.Join(d.Licenses, ";") })},
		{Header: "MarketplaceProject", Extract: vmField(func(d *inventory.VMDetail) string { return d.MarketplaceProject })},
		{Header: "MarketplaceClass", Extract: vmField(func(d *inventory.VMDetail) string { return d.MarketplaceClass })},
		{Header: "Preemptible", Extract: vmField(func(d *inventory.VMDetail) string { return boolStr(d.Preemptible) })},
		{Header: "Spot", Extract: vmField(func(d *inventory.VMDetail) string { return boolStr(d.Spot) })},
		{Header: "Zone", Extract: vmField(func(d *inventory.VMDetail) string { return d.Zone })},
		{Header: "BootDiskName", Extract: vmField(func(d *inventory.VMDetail) string { return d.BootDisk.Name })},
		{Header: "BootDiskSizeGB", Extract: vmField(func(d *inventory.VMDetail) string {
			if d.BootDisk.SizeGB == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.BootDisk.SizeGB)
		})},
		{Header: "BootDiskType", Extract: vmField(func(d *inventory.VMDetail) string { return d.BootDisk.Type })},
		{Header: "AttachedDisks", Extract: vmField(func(d *inventory.VMDetail) string {
			if len(d.AttachedDisks) == 0 {
				return ""
			}
			parts := make([]string, len(d.AttachedDisks))
			for i, ad := range d.AttachedDisks {
				parts[i] = fmt.Sprintf("%s(%s,%dG)", ad.Name, ad.Type, ad.SizeGB)
			}
			return strings.Join(parts, ";")
		})},
		{Header: "NICs", Extract: vmField(func(d *inventory.VMDetail) string {
			if len(d.NICs) == 0 {
				return ""
			}
			parts := make([]string, len(d.NICs))
			for i, n := range d.NICs {
				s := n.Subnetwork + "@" + n.InternalIP
				if n.ExternalIP != "" {
					s += "→" + n.ExternalIP
				}
				parts[i] = s
			}
			return strings.Join(parts, ";")
		})},
		{Header: "Labels", Extract: labelsOf},
	}
}

func vmField(get func(*inventory.VMDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		vm, ok := d.(*inventory.VMDetail)
		if !ok || vm == nil {
			return ""
		}
		return get(vm)
	}
}

// --- Disks -----------------------------------------------------------------

func diskColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "Region", Extract: regionOf},
		{Header: "Status", Extract: statusOf},
		{Header: "SizeGB", Extract: diskField(func(d *inventory.DiskDetail) string {
			if d.SizeGB == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.SizeGB)
		})},
		{Header: "Type", Extract: diskField(func(d *inventory.DiskDetail) string { return d.Type })},
		{Header: "Zone", Extract: diskField(func(d *inventory.DiskDetail) string { return d.Zone })},
		{Header: "InUseBy", Extract: diskField(func(d *inventory.DiskDetail) string {
			if len(d.InUseBy) == 0 {
				return ""
			}
			parts := make([]string, len(d.InUseBy))
			for i, ref := range d.InUseBy {
				parts[i] = ref.ID
			}
			return strings.Join(parts, ";")
		})},
		{Header: "Snapshot", Extract: diskField(func(d *inventory.DiskDetail) string { return d.Snapshot })},
		{Header: "Licenses", Extract: diskField(func(d *inventory.DiskDetail) string { return strings.Join(d.Licenses, ";") })},
		{Header: "MarketplaceProject", Extract: diskField(func(d *inventory.DiskDetail) string { return d.MarketplaceProject })},
		{Header: "MarketplaceClass", Extract: diskField(func(d *inventory.DiskDetail) string { return d.MarketplaceClass })},
		{Header: "Labels", Extract: labelsOf},
	}
}

func diskField(get func(*inventory.DiskDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		dd, ok := d.(*inventory.DiskDetail)
		if !ok || dd == nil {
			return ""
		}
		return get(dd)
	}
}

// --- Networks --------------------------------------------------------------

func networkColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "AutoSubnet", Extract: netField(func(d *inventory.NetworkDetail) string { return boolStr(d.AutoSubnet) })},
		{Header: "IPv4Range", Extract: netField(func(d *inventory.NetworkDetail) string { return d.IPv4Range })},
		{Header: "SubnetCount", Extract: netField(func(d *inventory.NetworkDetail) string {
			if d.SubnetCount == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.SubnetCount)
		})},
	}
}

func netField(get func(*inventory.NetworkDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		n, ok := d.(*inventory.NetworkDetail)
		if !ok || n == nil {
			return ""
		}
		return get(n)
	}
}

// --- Subnets ---------------------------------------------------------------

func subnetColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "Region", Extract: subField(func(d *inventory.SubnetDetail) string { return d.Region })},
		{Header: "CIDR", Extract: subField(func(d *inventory.SubnetDetail) string { return d.CIDR })},
		{Header: "Network", Extract: subField(func(d *inventory.SubnetDetail) string { return d.Network })},
		{Header: "Private", Extract: subField(func(d *inventory.SubnetDetail) string { return boolStr(d.Private) })},
	}
}

func subField(get func(*inventory.SubnetDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		s, ok := d.(*inventory.SubnetDetail)
		if !ok || s == nil {
			return ""
		}
		return get(s)
	}
}

// --- Firewalls -------------------------------------------------------------

func firewallColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "Direction", Extract: fwField(func(d *inventory.FirewallDetail) string { return d.Direction })},
		{Header: "Priority", Extract: fwField(func(d *inventory.FirewallDetail) string {
			if d.Priority == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.Priority)
		})},
		{Header: "SourceRanges", Extract: fwField(func(d *inventory.FirewallDetail) string { return strings.Join(d.SourceRanges, ";") })},
		{Header: "TargetTags", Extract: fwField(func(d *inventory.FirewallDetail) string { return strings.Join(d.TargetTags, ";") })},
		{Header: "Allowed", Extract: fwField(func(d *inventory.FirewallDetail) string {
			if len(d.Allowed) == 0 {
				return ""
			}
			parts := make([]string, len(d.Allowed))
			for i, a := range d.Allowed {
				if len(a.Ports) == 0 {
					parts[i] = a.Protocol
				} else {
					parts[i] = a.Protocol + ":" + strings.Join(a.Ports, ",")
				}
			}
			return strings.Join(parts, ";")
		})},
		{Header: "Labels", Extract: labelsOf},
	}
}

func fwField(get func(*inventory.FirewallDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		f, ok := d.(*inventory.FirewallDetail)
		if !ok || f == nil {
			return ""
		}
		return get(f)
	}
}

// --- LoadBalancers ---------------------------------------------------------

func loadBalancerColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "Region", Extract: regionOf},
		{Header: "Scheme", Extract: lbField(func(d *inventory.LoadBalancerDetail) string { return d.Scheme })},
		{Header: "Protocol", Extract: lbField(func(d *inventory.LoadBalancerDetail) string { return d.Protocol })},
		{Header: "IPAddress", Extract: lbField(func(d *inventory.LoadBalancerDetail) string { return d.IPAddress })},
		{Header: "Ports", Extract: lbField(func(d *inventory.LoadBalancerDetail) string { return strings.Join(d.Ports, ";") })},
		{Header: "BackendCount", Extract: lbField(func(d *inventory.LoadBalancerDetail) string { return fmt.Sprintf("%d", d.BackendCount) })},
	}
}

func lbField(get func(*inventory.LoadBalancerDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		lb, ok := d.(*inventory.LoadBalancerDetail)
		if !ok || lb == nil {
			return ""
		}
		return get(lb)
	}
}

// --- Databases -------------------------------------------------------------

func databaseColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "Region", Extract: regionOf},
		{Header: "Status", Extract: statusOf},
		{Header: "Engine", Extract: dbField(func(d *inventory.DatabaseDetail) string { return d.Engine })},
		{Header: "Tier", Extract: dbField(func(d *inventory.DatabaseDetail) string { return d.Tier })},
		{Header: "vCPUs", Extract: dbField(func(d *inventory.DatabaseDetail) string {
			if d.VCPUs == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.VCPUs)
		})},
		{Header: "MemoryMiB", Extract: dbField(func(d *inventory.DatabaseDetail) string {
			if d.MemoryMiB == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.MemoryMiB)
		})},
		{Header: "StorageGB", Extract: dbField(func(d *inventory.DatabaseDetail) string {
			if d.StorageGB == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.StorageGB)
		})},
		{Header: "StorageType", Extract: dbField(func(d *inventory.DatabaseDetail) string { return d.StorageType })},
		{Header: "HighAvailability", Extract: dbField(func(d *inventory.DatabaseDetail) string { return boolStr(d.HighAvailability) })},
		{Header: "MaintenanceWindow", Extract: dbField(func(d *inventory.DatabaseDetail) string { return d.MaintenanceWindow })},
	}
}

func dbField(get func(*inventory.DatabaseDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		db, ok := d.(*inventory.DatabaseDetail)
		if !ok || db == nil {
			return ""
		}
		return get(db)
	}
}

// --- Clusters --------------------------------------------------------------

func clusterColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "Location", Extract: clField(func(d *inventory.ClusterDetail) string { return d.Location })},
		{Header: "Status", Extract: statusOf},
		{Header: "Version", Extract: clField(func(d *inventory.ClusterDetail) string { return d.Version })},
		{Header: "NodeCount", Extract: clField(func(d *inventory.ClusterDetail) string {
			if d.NodeCount == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.NodeCount)
		})},
		{Header: "NodeMachine", Extract: clField(func(d *inventory.ClusterDetail) string { return d.NodeMachine })},
		{Header: "NodeDiskGB", Extract: clField(func(d *inventory.ClusterDetail) string {
			if d.NodeDiskGB == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.NodeDiskGB)
		})},
		{Header: "Serverless", Extract: clField(func(d *inventory.ClusterDetail) string { return boolStr(d.Serverless) })},
		{Header: "Labels", Extract: labelsOf},
	}
}

func clField(get func(*inventory.ClusterDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		c, ok := d.(*inventory.ClusterDetail)
		if !ok || c == nil {
			return ""
		}
		return get(c)
	}
}

// --- Buckets ---------------------------------------------------------------

func bucketColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "Location", Extract: bkField(func(d *inventory.BucketDetail) string { return d.Location })},
		{Header: "StorageClass", Extract: bkField(func(d *inventory.BucketDetail) string { return d.StorageClass })},
		{Header: "PublicAccess", Extract: bkField(func(d *inventory.BucketDetail) string { return boolStr(d.PublicAccess) })},
		{Header: "Versioning", Extract: bkField(func(d *inventory.BucketDetail) string { return boolStr(d.Versioning) })},
		// Raw integers so Excel SUM / sort works without parsing display strings.
		{Header: "SizeBytes", Extract: bkField(func(d *inventory.BucketDetail) string { return fmt.Sprintf("%d", d.SizeBytes) })},
		{Header: "ObjectCount", Extract: bkField(func(d *inventory.BucketDetail) string { return fmt.Sprintf("%d", d.ObjectCount) })},
		{Header: "Labels", Extract: labelsOf},
	}
}

func bkField(get func(*inventory.BucketDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		b, ok := d.(*inventory.BucketDetail)
		if !ok || b == nil {
			return ""
		}
		return get(b)
	}
}

// --- Functions -------------------------------------------------------------

func functionColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "Region", Extract: regionOf},
		{Header: "Status", Extract: statusOf},
		{Header: "Runtime", Extract: fnField(func(d *inventory.FunctionDetail) string { return d.Runtime })},
		{Header: "Trigger", Extract: fnField(func(d *inventory.FunctionDetail) string { return d.Trigger })},
		{Header: "MemoryMiB", Extract: fnField(func(d *inventory.FunctionDetail) string {
			if d.MemoryMiB == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.MemoryMiB)
		})},
		{Header: "CPUs", Extract: fnField(func(d *inventory.FunctionDetail) string {
			if d.CPUs == 0 {
				return ""
			}
			return fmt.Sprintf("%g", d.CPUs)
		})},
		{Header: "MaxInst", Extract: fnField(func(d *inventory.FunctionDetail) string {
			if d.MaxInst == 0 {
				return ""
			}
			return fmt.Sprintf("%d", d.MaxInst)
		})},
		{Header: "Labels", Extract: labelsOf},
	}
}

func fnField(get func(*inventory.FunctionDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		f, ok := d.(*inventory.FunctionDetail)
		if !ok || f == nil {
			return ""
		}
		return get(f)
	}
}

// --- Stub-only Kinds (VertexAI, Apigee, Firebase, BigQuery, …) ----------------

// stubColumns returns the standard 5-column Excel layout shared by all stub-only Kinds.
func stubColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "Name", Extract: nameOf},
		{Header: "Region", Extract: regionOf},
		{Header: "Status", Extract: statusOf},
		{Header: "Subtype", Extract: stubField(func(d *inventory.StubDetail) string { return d.Subtype })},
		{Header: "Labels", Extract: labelsOf},
	}
}

func stubField(get func(*inventory.StubDetail) string) func(inventory.Resource, any) string {
	return func(_ inventory.Resource, d any) string {
		sd, ok := d.(*inventory.StubDetail)
		if !ok || sd == nil {
			return ""
		}
		return get(sd)
	}
}
