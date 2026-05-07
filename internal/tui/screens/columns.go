package screens

import (
	"encoding/json"
	"fmt"
	"strings"

	"cloudcmder.com/internal/inventory"
)

// ColumnDef describes one column in a ResourceList table. Extract receives
// the already-decoded Detail (kind-specific struct) so each column doesn't
// pay the JSON-unmarshal cost separately.
type ColumnDef struct {
	Header  string
	Width   int
	Extract func(r inventory.Resource, detail any) string
}

// columnsFor returns the column set registered for the given Kind. The second
// return value reports whether a kind-specific column set exists; callers use
// it to decide between the real ResourceList and a stub. After M6 every kind
// is registered, so the stub fallback is unreachable.
func columnsFor(kind inventory.Kind) ([]ColumnDef, bool) {
	switch kind {
	case inventory.KindVM:
		return vmColumns(), true
	case inventory.KindDisk:
		return diskColumns(), true
	case inventory.KindNetwork:
		return networkColumns(), true
	case inventory.KindSubnet:
		return subnetColumns(), true
	case inventory.KindFirewall:
		return firewallColumns(), true
	case inventory.KindLoadBalancer:
		return loadBalancerColumns(), true
	case inventory.KindDatabase:
		return databaseColumns(), true
	case inventory.KindCluster:
		return clusterColumns(), true
	case inventory.KindBucket:
		return bucketColumns(), true
	case inventory.KindFunction:
		return functionColumns(), true
	default:
		return nil, false
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
	}
	return "", false
}

// --- VM --------------------------------------------------------------------

func vmColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 22, Extract: func(r inventory.Resource, _ any) string { return r.Name }},
		{Header: "ZONE", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil {
				return vm.Zone
			}
			return ""
		}},
		{Header: "MACHINE", Width: 14, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil {
				return vm.MachineType
			}
			return ""
		}},
		{Header: "VCPU", Width: 5, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil && vm.VCPUs > 0 {
				return fmt.Sprintf("%d", vm.VCPUs)
			}
			return ""
		}},
		{Header: "RAM GIB", Width: 8, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil && vm.MemoryMiB > 0 {
				return fmt.Sprintf("%.1f", float64(vm.MemoryMiB)/1024.0)
			}
			return ""
		}},
		{Header: "OS", Width: 12, Extract: func(_ inventory.Resource, d any) string {
			if vm := vmOf(d); vm != nil {
				return vm.OSFamily
			}
			return ""
		}},
		{Header: "STATUS", Width: 10, Extract: func(r inventory.Resource, _ any) string { return r.Status }},
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

// --- shared helpers --------------------------------------------------------

func nameOf(r inventory.Resource, _ any) string   { return r.Name }
func statusOf(r inventory.Resource, _ any) string { return r.Status }

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
