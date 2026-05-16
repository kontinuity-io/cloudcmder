package export

import (
	"testing"

	"cloudcmder.com/internal/inventory"
)

func TestVMColumnsHandlesNilDetail(t *testing.T) {
	cols := vmColumns()
	r := inventory.Resource{Name: "vm-x", Region: "us-c1-a", Status: "RUNNING"}
	for _, c := range cols {
		// Should never panic on nil detail; basic fields still come from r.
		_ = c.Extract(r, nil)
	}
}

func TestVMColumnsPopulatedDetail(t *testing.T) {
	r := inventory.Resource{Name: "vm-a", Region: "us-c1-a", Status: "RUNNING"}
	d := &inventory.VMDetail{
		MachineType: "n2-standard-4",
		VCPUs:       4,
		MemoryMiB:   16384,
		OSFamily:    "debian-12",
		Zone:        "us-central1-a",
		BootDisk:    inventory.DiskRef{Name: "vm-boot", SizeGB: 100, Type: "pd-balanced"},
		AttachedDisks: []inventory.DiskRef{
			{Name: "data-1", SizeGB: 500, Type: "pd-ssd"},
			{Name: "data-2", SizeGB: 1000, Type: "pd-ssd"},
		},
		NICs: []inventory.NICDetail{
			{Subnetwork: "default", InternalIP: "10.0.0.1", ExternalIP: "35.1.2.3"},
		},
	}
	cols := vmColumns()
	want := map[string]string{
		"Name":           "vm-a",
		"MachineType":    "n2-standard-4",
		"vCPUs":          "4",
		"MemoryMiB":      "16384",
		"OSFamily":       "debian-12",
		"Zone":           "us-central1-a",
		"BootDiskName":   "vm-boot",
		"BootDiskSizeGB": "100",
		"AttachedDisks":  "data-1(pd-ssd,500G);data-2(pd-ssd,1000G)",
		"NICs":           "default@10.0.0.1→35.1.2.3",
	}
	for _, c := range cols {
		if expected, ok := want[c.Header]; ok {
			got := c.Extract(r, d)
			if got != expected {
				t.Errorf("%s = %q, want %q", c.Header, got, expected)
			}
		}
	}
}

func TestFirewallAllowedFormatting(t *testing.T) {
	r := inventory.Resource{Name: "fw-1"}
	d := &inventory.FirewallDetail{
		Direction:    "INGRESS",
		Priority:     1000,
		SourceRanges: []string{"0.0.0.0/0"},
		TargetTags:   []string{"web", "ssh"},
		Allowed: []inventory.FirewallRule{
			{Protocol: "tcp", Ports: []string{"80", "443"}},
			{Protocol: "icmp"},
		},
	}
	cols := firewallColumns()
	for _, c := range cols {
		switch c.Header {
		case "SourceRanges":
			if got := c.Extract(r, d); got != "0.0.0.0/0" {
				t.Errorf("SourceRanges = %q", got)
			}
		case "TargetTags":
			if got := c.Extract(r, d); got != "web;ssh" {
				t.Errorf("TargetTags = %q", got)
			}
		case "Allowed":
			if got := c.Extract(r, d); got != "tcp:80,443;icmp" {
				t.Errorf("Allowed = %q", got)
			}
		}
	}
}

func TestColumnsForCoversAllKinds(t *testing.T) {
	for _, k := range []inventory.Kind{
		inventory.KindVM, inventory.KindDisk, inventory.KindNetwork,
		inventory.KindSubnet, inventory.KindFirewall, inventory.KindLoadBalancer,
		inventory.KindDatabase, inventory.KindCluster, inventory.KindBucket,
		inventory.KindFunction,
		// stub-only Kinds
		inventory.KindGCPVertexAI, inventory.KindGCPApigee, inventory.KindGCPFirebase,
		inventory.KindGCPAppEngine, inventory.KindGCPBigQuery, inventory.KindGCPDNS,
		inventory.KindGCPMemorystore, inventory.KindGCPArtifactRegistry, inventory.KindGCPCloudScheduler,
		inventory.KindGCPPubSub, inventory.KindGCPSpanner, inventory.KindGCPBigtable,
		inventory.KindGCPKMS, inventory.KindGCPSecretManager, inventory.KindGCPDataflow,
		inventory.KindGCPDataproc, inventory.KindGCPComposer, inventory.KindGCPCloudTasks,
		inventory.KindGCPMonitoring, inventory.KindGCPLogging, inventory.KindGCPOSConfig,
		inventory.KindGCPVPN, inventory.KindGCPRouter, inventory.KindGCPCloudBuild,
	} {
		if cols := columnsFor(k); len(cols) == 0 {
			t.Errorf("no columns registered for kind %s", k)
		}
	}
}

func TestVMLicenseColumns(t *testing.T) {
	r := inventory.Resource{Name: "vm-f5"}
	d := &inventory.VMDetail{
		Licenses:       []string{"f5-bigip-best", "f5-addon"},
		MarketplaceProject: "f5-7626-networks-public",
		MarketplaceClass:   "marketplace",
	}
	cols := vmColumns()
	want := map[string]string{
		"Licenses":       "f5-bigip-best;f5-addon",
		"MarketplaceProject": "f5-7626-networks-public",
		"MarketplaceClass":   "marketplace",
	}
	for _, c := range cols {
		if expected, ok := want[c.Header]; ok {
			if got := c.Extract(r, d); got != expected {
				t.Errorf("%s = %q, want %q", c.Header, got, expected)
			}
		}
	}
}

func TestDiskLicenseColumns(t *testing.T) {
	r := inventory.Resource{Name: "disk-rhel"}
	d := &inventory.DiskDetail{
		Licenses:       []string{"rhel-9"},
		MarketplaceProject: "rhel-cloud",
		MarketplaceClass:   "google-paid",
	}
	cols := diskColumns()
	want := map[string]string{
		"Licenses":       "rhel-9",
		"MarketplaceProject": "rhel-cloud",
		"MarketplaceClass":   "google-paid",
	}
	for _, c := range cols {
		if expected, ok := want[c.Header]; ok {
			if got := c.Extract(r, d); got != expected {
				t.Errorf("%s = %q, want %q", c.Header, got, expected)
			}
		}
	}
}

func TestStubColumns(t *testing.T) {
	r := inventory.Resource{Name: "projects/p1/locations/us-central1/endpoints/123", Region: "us-central1", Status: "ACTIVE"}
	d := &inventory.StubDetail{Subtype: "Endpoint", Region: "us-central1"}
	cols := stubColumns()
	if len(cols) != 5 {
		t.Fatalf("stubColumns len = %d, want 5", len(cols))
	}
	want := map[string]string{
		"Name":    r.Name,
		"Region":  "us-central1",
		"Status":  "ACTIVE",
		"Subtype": "Endpoint",
	}
	for _, c := range cols {
		if expected, ok := want[c.Header]; ok {
			if got := c.Extract(r, d); got != expected {
				t.Errorf("%s = %q, want %q", c.Header, got, expected)
			}
		}
	}
}
