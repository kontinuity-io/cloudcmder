package gcp

import (
	"context"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

func TestBuildDiskResource(t *testing.T) {
	d := &computepb.Disk{
		Name:   ptr("disk-a"),
		Zone:   ptr("zones/us-central1-a"),
		SizeGb: ptr(int64(100)),
		Type:   ptr("zones/us-central1-a/diskTypes/pd-balanced"),
		Status: ptr("READY"),
		Users: []string{
			"projects/p1/zones/us-central1-a/instances/vm-a",
		},
		Labels: map[string]string{"env": "prod"},
	}
	r := buildDiskResource("p1", d, false)
	if r.Ref.String() != "gcp:p1:Disk:disk-a" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	dd, ok := r.Detail.(*inventory.DiskDetail)
	if !ok {
		t.Fatalf("detail not *DiskDetail: %T", r.Detail)
	}
	if dd.SizeGB != 100 || dd.Type != "pd-balanced" || dd.Zone != "us-central1-a" {
		t.Errorf("detail = %+v", dd)
	}
	if len(dd.InUseBy) != 1 || dd.InUseBy[0].ID != "vm-a" {
		t.Errorf("InUseBy = %+v", dd.InUseBy)
	}
	if len(r.Refs[inventory.RefAttachedTo]) != 1 {
		t.Errorf("Refs[AttachedTo] = %+v", r.Refs)
	}
	// No licenses → empty class.
	if dd.LicenseClass != "" {
		t.Errorf("LicenseClass = %q, want empty (no licenses on this disk)", dd.LicenseClass)
	}
}

func TestBuildDiskResourceWithMarketplaceLicense(t *testing.T) {
	d := &computepb.Disk{
		Name:   ptr("disk-f5"),
		Zone:   ptr("zones/us-central1-a"),
		SizeGb: ptr(int64(100)),
		Type:   ptr("zones/us-central1-a/diskTypes/pd-ssd"),
		Status: ptr("READY"),
		Licenses: []string{
			"projects/f5-7626-networks-public/global/licenses/f5-bigip-best",
		},
	}
	r := buildDiskResource("p1", d, false)
	dd, ok := r.Detail.(*inventory.DiskDetail)
	if !ok {
		t.Fatalf("detail not *DiskDetail: %T", r.Detail)
	}
	if dd.LicenseClass != "marketplace" {
		t.Errorf("LicenseClass = %q, want marketplace", dd.LicenseClass)
	}
	if dd.LicenseProject != "f5-7626-networks-public" {
		t.Errorf("LicenseProject = %q, want f5-7626-networks-public", dd.LicenseProject)
	}
	if len(dd.Licenses) != 1 || dd.Licenses[0] != "f5-bigip-best" {
		t.Errorf("Licenses = %v, want [f5-bigip-best]", dd.Licenses)
	}
}

func TestEnrichDisksStreams(t *testing.T) {
	pages := []compute.DisksScopedListPair{
		{
			Key: "us-central1-a",
			Value: &computepb.DisksScopedList{
				Disks: []*computepb.Disk{
					{Name: ptr("d1"), Zone: ptr("zones/us-central1-a"), SizeGb: ptr(int64(50))},
					{Name: ptr("d2"), Zone: ptr("zones/us-central1-a"), SizeGb: ptr(int64(75))},
				},
			},
		},
		{Key: "us-east1-b", Value: &computepb.DisksScopedList{}},
	}
	p := newProviderWithFakeAsset(t, &fakeAssetClient{})
	p.disks.factory = func(_ context.Context, _ ...option.ClientOption) (disksAPI, error) {
		return &fakeDisksClient{pairs: pages}, nil
	}

	ctx := context.Background()
	ch := make(chan inventory.ResourceOrErr, 8)
	go func() {
		defer close(ch)
		enrichDisks(ctx, p, inventory.Scope{ID: "p1"}, ch)
	}()
	var n int
	for x := range ch {
		if x.Err != nil {
			t.Fatalf("err: %v", x.Err)
		}
		if x.Resource.Kind == inventory.KindDisk {
			n++
		}
	}
	if n != 2 {
		t.Errorf("got %d disks, want 2", n)
	}
}

// --- fake disks client ----------------------------------------------------

type fakeDisksClient struct {
	pairs []compute.DisksScopedListPair
}

func (f *fakeDisksClient) AggregatedList(_ context.Context, _ *computepb.AggregatedListDisksRequest, _ ...gaxCallOption) disksIterator {
	return &fakeDisksIter{c: f}
}

func (f *fakeDisksClient) Close() error { return nil }

type fakeDisksIter struct {
	c   *fakeDisksClient
	idx int
}

func (it *fakeDisksIter) Next() (compute.DisksScopedListPair, error) {
	if it.idx >= len(it.c.pairs) {
		return compute.DisksScopedListPair{}, iterator.Done
	}
	p := it.c.pairs[it.idx]
	it.idx++
	return p, nil
}
