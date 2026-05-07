package gcp

import (
	"context"
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/iterator"

	"cloudcmder.com/internal/inventory"
)

func TestBuildFirewallResource(t *testing.T) {
	f := &computepb.Firewall{
		Name:         ptr("allow-ssh"),
		Direction:    ptr("INGRESS"),
		Priority:     ptr(int32(1000)),
		SourceRanges: []string{"0.0.0.0/0"},
		TargetTags:   []string{"web"},
		Allowed: []*computepb.Allowed{
			{IPProtocol: ptr("tcp"), Ports: []string{"22"}},
		},
	}
	r := buildFirewallResource("p1", f, false)
	if r.Ref.String() != "gcp:p1:Firewall:allow-ssh" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	d := r.Detail.(*inventory.FirewallDetail)
	if d.Direction != "INGRESS" || d.Priority != 1000 {
		t.Errorf("detail = %+v", d)
	}
	if len(d.Allowed) != 1 || d.Allowed[0].Protocol != "tcp" || len(d.Allowed[0].Ports) != 1 {
		t.Errorf("Allowed = %+v", d.Allowed)
	}
}

// --- fake firewalls client -------------------------------------------------

type fakeFirewallsClient struct {
	items []*computepb.Firewall
}

func (f *fakeFirewallsClient) List(_ context.Context, _ *computepb.ListFirewallsRequest, _ ...gaxCallOption) firewallsIterator {
	return &fakeFirewallsIter{c: f}
}

func (f *fakeFirewallsClient) Close() error { return nil }

type fakeFirewallsIter struct {
	c   *fakeFirewallsClient
	idx int
}

func (it *fakeFirewallsIter) Next() (*computepb.Firewall, error) {
	if it.idx >= len(it.c.items) {
		return nil, iterator.Done
	}
	f := it.c.items[it.idx]
	it.idx++
	return f, nil
}
