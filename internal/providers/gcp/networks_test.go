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

func TestBuildNetworkResource(t *testing.T) {
	n := &computepb.Network{
		Name:                  ptr("default"),
		AutoCreateSubnetworks: ptr(true),
	}
	r := buildNetworkResource("p1", n)
	if r.Ref.String() != "gcp:p1:Network:default" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	nd, ok := r.Detail.(*inventory.NetworkDetail)
	if !ok {
		t.Fatalf("detail not *NetworkDetail: %T", r.Detail)
	}
	if !nd.AutoSubnet {
		t.Errorf("AutoSubnet not set")
	}
}

func TestBuildSubnetResource(t *testing.T) {
	s := &computepb.Subnetwork{
		Name:                  ptr("default-uc1"),
		Region:                ptr("regions/us-central1"),
		Network:               ptr("global/networks/default"),
		IpCidrRange:           ptr("10.128.0.0/20"),
		PrivateIpGoogleAccess: ptr(true),
	}
	r := buildSubnetResource("p1", s)
	sd := r.Detail.(*inventory.SubnetDetail)
	if sd.CIDR != "10.128.0.0/20" || sd.Region != "us-central1" || sd.Network != "default" || !sd.Private {
		t.Errorf("detail = %+v", sd)
	}
	rf := r.Refs[inventory.RefRoutesFrom]
	if len(rf) != 1 || rf[0].ID != "default" || rf[0].Kind != inventory.KindNetwork {
		t.Errorf("Refs[RoutesFrom] = %+v", rf)
	}
}

func TestEnrichNetworksStreams(t *testing.T) {
	netList := []*computepb.Network{
		{Name: ptr("default"), AutoCreateSubnetworks: ptr(true)},
		{Name: ptr("vpc-a"), AutoCreateSubnetworks: ptr(false)},
	}
	p := newProviderWithFakeAsset(t, &fakeAssetClient{})
	p.networks.factory = func(_ context.Context, _ ...option.ClientOption) (networksAPI, error) {
		return &fakeNetworksClient{items: netList}, nil
	}
	ctx := context.Background()
	ch := make(chan inventory.ResourceOrErr, 4)
	go func() {
		defer close(ch)
		enrichNetworks(ctx, p, inventory.Scope{ID: "p1"}, ch)
	}()
	var n int
	for x := range ch {
		if x.Err != nil {
			t.Fatalf("err: %v", x.Err)
		}
		n++
	}
	if n != 2 {
		t.Errorf("got %d networks, want 2", n)
	}
}

// --- fake clients ---------------------------------------------------------

type fakeNetworksClient struct {
	items []*computepb.Network
}

func (f *fakeNetworksClient) List(_ context.Context, _ *computepb.ListNetworksRequest, _ ...gaxCallOption) networksIterator {
	return &fakeNetworksIter{c: f}
}

func (f *fakeNetworksClient) Close() error { return nil }

type fakeNetworksIter struct {
	c   *fakeNetworksClient
	idx int
}

func (it *fakeNetworksIter) Next() (*computepb.Network, error) {
	if it.idx >= len(it.c.items) {
		return nil, iterator.Done
	}
	n := it.c.items[it.idx]
	it.idx++
	return n, nil
}

type fakeSubnetsClient struct {
	pairs []compute.SubnetworksScopedListPair
}

func (f *fakeSubnetsClient) AggregatedList(_ context.Context, _ *computepb.AggregatedListSubnetworksRequest, _ ...gaxCallOption) subnetsIterator {
	return &fakeSubnetsIter{c: f}
}

func (f *fakeSubnetsClient) Close() error { return nil }

type fakeSubnetsIter struct {
	c   *fakeSubnetsClient
	idx int
}

func (it *fakeSubnetsIter) Next() (compute.SubnetworksScopedListPair, error) {
	if it.idx >= len(it.c.pairs) {
		return compute.SubnetworksScopedListPair{}, iterator.Done
	}
	p := it.c.pairs[it.idx]
	it.idx++
	return p, nil
}
