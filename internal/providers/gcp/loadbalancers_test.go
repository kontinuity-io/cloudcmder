package gcp

import (
	"context"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/iterator"

	"cloudcmder.com/internal/inventory"
)

func TestBuildLBResourceFromGlobalRule(t *testing.T) {
	fr := &computepb.ForwardingRule{
		Name:                 ptr("lb-frontend"),
		IPAddress:            ptr("35.1.2.3"),
		IPProtocol:           ptr("TCP"),
		LoadBalancingScheme:  ptr("EXTERNAL_MANAGED"),
		PortRange:            ptr("443"),
	}
	r := buildLBResource("p1", fr, "global", false)
	if r.Ref.String() != "gcp:p1:LoadBalancer:lb-frontend" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	d := r.Detail.(*inventory.LoadBalancerDetail)
	if d.IPAddress != "35.1.2.3" || d.Protocol != "TCP" || d.Scheme != "EXTERNAL_MANAGED" {
		t.Errorf("detail = %+v", d)
	}
	if len(d.Ports) != 1 || d.Ports[0] != "443" {
		t.Errorf("Ports = %+v", d.Ports)
	}
	if d.BackendCount != 0 {
		t.Errorf("BackendCount = %d, want 0 (M6 minimum-viable)", d.BackendCount)
	}
}

// --- fake forwarding rules clients ----------------------------------------

type fakeGlobalForwardingRulesClient struct {
	items []*computepb.ForwardingRule
}

func (f *fakeGlobalForwardingRulesClient) List(_ context.Context, _ *computepb.ListGlobalForwardingRulesRequest, _ ...gaxCallOption) forwardingRulesGlobalIterator {
	return &fakeGfwdIter{c: f}
}

func (f *fakeGlobalForwardingRulesClient) Close() error { return nil }

type fakeGfwdIter struct {
	c   *fakeGlobalForwardingRulesClient
	idx int
}

func (it *fakeGfwdIter) Next() (*computepb.ForwardingRule, error) {
	if it.idx >= len(it.c.items) {
		return nil, iterator.Done
	}
	r := it.c.items[it.idx]
	it.idx++
	return r, nil
}

type fakeForwardingRulesClient struct {
	pairs []compute.ForwardingRulesScopedListPair
}

func (f *fakeForwardingRulesClient) AggregatedList(_ context.Context, _ *computepb.AggregatedListForwardingRulesRequest, _ ...gaxCallOption) forwardingRulesRegionalIterator {
	return &fakeRfwdIter{c: f}
}

func (f *fakeForwardingRulesClient) Close() error { return nil }

type fakeRfwdIter struct {
	c   *fakeForwardingRulesClient
	idx int
}

func (it *fakeRfwdIter) Next() (compute.ForwardingRulesScopedListPair, error) {
	if it.idx >= len(it.c.pairs) {
		return compute.ForwardingRulesScopedListPair{}, iterator.Done
	}
	p := it.c.pairs[it.idx]
	it.idx++
	return p, nil
}
