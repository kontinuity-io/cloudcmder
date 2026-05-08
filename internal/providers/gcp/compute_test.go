package gcp

import (
	"context"
	"errors"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

func TestBuildVMResourceShape(t *testing.T) {
	inst := &computepb.Instance{
		Name:        ptr("vm-a"),
		Zone:        ptr("https://www.googleapis.com/compute/v1/projects/p1/zones/us-central1-a"),
		MachineType: ptr("https://www.googleapis.com/compute/v1/projects/p1/zones/us-central1-a/machineTypes/custom-4-8192"),
		Status:      ptr("RUNNING"),
		CpuPlatform: ptr("Intel Cascade Lake"),
		Scheduling: &computepb.Scheduling{
			Preemptible:       ptr(false),
			ProvisioningModel: ptr("STANDARD"),
		},
		Disks: []*computepb.AttachedDisk{
			{
				Boot:       ptr(true),
				Source:     ptr("https://www.googleapis.com/compute/v1/projects/p1/zones/us-central1-a/disks/vm-a-boot"),
				DiskSizeGb: ptr(int64(100)),
				Type:       ptr("PERSISTENT"),
				Licenses: []string{
					"https://www.googleapis.com/compute/v1/projects/debian-cloud/global/licenses/debian-11",
				},
			},
			{
				Boot:       ptr(false),
				Source:     ptr("https://www.googleapis.com/compute/v1/projects/p1/zones/us-central1-a/disks/vm-a-data"),
				DiskSizeGb: ptr(int64(500)),
			},
		},
		NetworkInterfaces: []*computepb.NetworkInterface{
			{
				Network:    ptr("https://www.googleapis.com/compute/v1/projects/p1/global/networks/default"),
				Subnetwork: ptr("https://www.googleapis.com/compute/v1/projects/p1/regions/us-central1/subnetworks/default-uc1"),
				NetworkIP:  ptr("10.128.0.5"),
				AccessConfigs: []*computepb.AccessConfig{
					{NatIP: ptr("35.1.2.3")},
				},
			},
		},
		Labels: map[string]string{"env": "prod"},
	}

	resolveCustom := func(ctx context.Context, zone, mt string) (int32, int64, error) {
		// parseMachineType handles `custom-4-8192` directly; resolver should not be called.
		t.Errorf("resolver called unexpectedly for custom MT %q", mt)
		return 0, 0, errors.New("unexpected")
	}
	// Wrap the resolver so the parseMachineType branch is the actual one used:
	resolve := func(ctx context.Context, zone, mt string) (int32, int64, error) {
		if v, m, ok := parseMachineType(mt); ok {
			return v, m, nil
		}
		return resolveCustom(ctx, zone, mt)
	}

	r := buildVMResource(context.Background(), "p1", inst, resolve, false)

	if r.Ref.String() != "gcp:p1:VM:vm-a" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	if r.Region != "us-central1-a" || r.Status != "RUNNING" {
		t.Errorf("region/status: %+v", r)
	}
	d, ok := r.Detail.(*inventory.VMDetail)
	if !ok {
		t.Fatalf("Detail not *VMDetail: %T", r.Detail)
	}
	if d.MachineType != "custom-4-8192" || d.VCPUs != 4 || d.MemoryMiB != 8192 {
		t.Errorf("MT mismatch: %+v", d)
	}
	if d.OSFamily != "debian-11" {
		t.Errorf("OSFamily = %q, want debian-11", d.OSFamily)
	}
	if d.BootDisk.Name != "vm-a-boot" || d.BootDisk.SizeGB != 100 {
		t.Errorf("BootDisk = %+v", d.BootDisk)
	}
	if len(d.AttachedDisks) != 1 || d.AttachedDisks[0].Name != "vm-a-data" {
		t.Errorf("AttachedDisks = %+v", d.AttachedDisks)
	}
	if len(d.NICs) != 1 || d.NICs[0].InternalIP != "10.128.0.5" || d.NICs[0].ExternalIP != "35.1.2.3" {
		t.Errorf("NICs = %+v", d.NICs)
	}
	if d.Zone != "us-central1-a" || d.CPUPlatform != "Intel Cascade Lake" {
		t.Errorf("Zone/CPUPlatform: %+v", d)
	}

	subnetRefs := r.Refs[inventory.RefRoutesFrom]
	if len(subnetRefs) != 1 || subnetRefs[0].ID != "default-uc1" {
		t.Errorf("Refs[RoutesFrom] = %+v", subnetRefs)
	}
}

func TestEnrichVMsStreamsResources(t *testing.T) {
	pages := []compute.InstancesScopedListPair{
		{
			Key: "us-central1-a",
			Value: &computepb.InstancesScopedList{
				Instances: []*computepb.Instance{
					{
						Name:        ptr("vm-1"),
						Zone:        ptr("zones/us-central1-a"),
						MachineType: ptr("zones/us-central1-a/machineTypes/custom-2-4096"),
						Status:      ptr("RUNNING"),
						Disks: []*computepb.AttachedDisk{
							{Boot: ptr(true), Source: ptr("zones/us-central1-a/disks/vm-1-boot")},
						},
						NetworkInterfaces: []*computepb.NetworkInterface{
							{Subnetwork: ptr("regions/us-central1/subnetworks/default-uc1")},
						},
					},
				},
			},
		},
		{Key: "us-east1-b", Value: &computepb.InstancesScopedList{}},
		{Key: "us-east1-c", Value: nil},
	}
	p := newProviderWithFakeInstances(t, &fakeInstancesClient{pairs: pages})

	ctx := context.Background()
	ch := make(chan inventory.ResourceOrErr, 8)
	go func() {
		defer close(ch)
		enrichVMs(ctx, p, inventory.Scope{ID: "p1"}, ch)
	}()

	var vms, disks int
	for x := range ch {
		if x.Err != nil {
			t.Fatalf("stream err: %v", x.Err)
		}
		switch x.Resource.Kind {
		case inventory.KindVM:
			vms++
		case inventory.KindDisk:
			disks++
		}
	}
	if vms != 1 {
		t.Errorf("got %d VMs, want 1", vms)
	}
	// enrichVMs no longer emits Disk stubs — the Disk → VM AttachedTo
	// edge is captured from the disk side via enrichDisks (Disk.Users()).
	if disks != 0 {
		t.Errorf("got %d disk stubs, want 0 (disk side now emits the edge)", disks)
	}
}

func TestEnrichVMsPropagatesError(t *testing.T) {
	p := newProviderWithFakeInstances(t, &fakeInstancesClient{
		pairs:    []compute.InstancesScopedListPair{},
		errAfter: errors.New("simulated 503"),
	})

	ctx := context.Background()
	ch := make(chan inventory.ResourceOrErr, 4)
	go func() {
		defer close(ch)
		enrichVMs(ctx, p, inventory.Scope{ID: "p1"}, ch)
	}()
	var sawErr bool
	for x := range ch {
		if x.Err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Error("expected error to be emitted")
	}
}

// --- fake instances client ----------------------------------------------------

type fakeInstancesClient struct {
	pairs    []compute.InstancesScopedListPair
	errAfter error
}

func (f *fakeInstancesClient) AggregatedList(ctx context.Context, _ *computepb.AggregatedListInstancesRequest, _ ...gaxCallOption) instancesIterator {
	return &fakeInstancesIter{c: f}
}

func (f *fakeInstancesClient) Close() error { return nil }

type fakeInstancesIter struct {
	c   *fakeInstancesClient
	idx int
}

func (it *fakeInstancesIter) Next() (compute.InstancesScopedListPair, error) {
	if it.idx >= len(it.c.pairs) {
		if it.c.errAfter != nil {
			err := it.c.errAfter
			it.c.errAfter = nil
			return compute.InstancesScopedListPair{}, err
		}
		return compute.InstancesScopedListPair{}, iterator.Done
	}
	p := it.c.pairs[it.idx]
	it.idx++
	return p, nil
}

func newProviderWithFakeInstances(t *testing.T, fake instancesAPI) *GCPProvider {
	t.Helper()
	p, err := New(context.Background(),
		option.WithEndpoint("http://localhost"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.instancesFact = func(_ context.Context, _ ...option.ClientOption) (instancesAPI, error) {
		return fake, nil
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func ptr[T any](v T) *T { return &v }
