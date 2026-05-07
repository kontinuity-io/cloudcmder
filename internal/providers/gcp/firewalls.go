package gcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

type firewallsAPI interface {
	List(ctx context.Context, req *computepb.ListFirewallsRequest, opts ...gaxCallOption) firewallsIterator
	Close() error
}

type firewallsIterator interface {
	Next() (*computepb.Firewall, error)
}

type realFirewallsClient struct {
	c *compute.FirewallsClient
}

func (r *realFirewallsClient) List(ctx context.Context, req *computepb.ListFirewallsRequest, _ ...gaxCallOption) firewallsIterator {
	return r.c.List(ctx, req)
}

func (r *realFirewallsClient) Close() error { return r.c.Close() }

type firewallsFactory func(ctx context.Context, opts ...option.ClientOption) (firewallsAPI, error)

type firewallsClientState struct {
	once    sync.Once
	cli     firewallsAPI
	err     error
	factory firewallsFactory
}

func (p *GCPProvider) firewallsClient(ctx context.Context) (firewallsAPI, error) {
	p.firewalls.once.Do(func() {
		if p.firewalls.factory != nil {
			p.firewalls.cli, p.firewalls.err = p.firewalls.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.firewalls.err = fmt.Errorf("gcp: ADC for firewalls client: %w", err)
			return
		}
		c, err := compute.NewFirewallsRESTClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.firewalls.err = fmt.Errorf("gcp: new firewalls client: %w", err)
			return
		}
		p.firewalls.cli = &realFirewallsClient{c: c}
	})
	if p.firewalls.err != nil {
		return nil, p.firewalls.err
	}
	return p.firewalls.cli, nil
}

func (p *GCPProvider) closeFirewallsClient() error {
	if p.firewalls.cli == nil {
		return nil
	}
	return p.firewalls.cli.Close()
}

func enrichFirewalls(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	fc, err := p.firewallsClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: firewalls client: %w", err)})
		return
	}
	it := fc.List(ctx, &computepb.ListFirewallsRequest{Project: scope.ID})
	for {
		f, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return
		}
		if err != nil {
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{
				Err: fmt.Errorf("gcp: list firewalls: %w", err),
			})
			return
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildFirewallResource(scope.ID, f)})
	}
}

func buildFirewallResource(scopeID string, f *computepb.Firewall) inventory.Resource {
	allowed := make([]inventory.FirewallRule, 0, len(f.GetAllowed()))
	for _, a := range f.GetAllowed() {
		allowed = append(allowed, inventory.FirewallRule{
			Protocol: a.GetIPProtocol(),
			Ports:    a.GetPorts(),
		})
	}
	detail := inventory.FirewallDetail{
		Direction:    f.GetDirection(),
		Priority:     f.GetPriority(),
		SourceRanges: f.GetSourceRanges(),
		TargetTags:   f.GetTargetTags(),
		Allowed:      allowed,
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindFirewall, ID: f.GetName()},
		Kind:   inventory.KindFirewall,
		Name:   f.GetName(),
		Region: "global",
		Status: "",
		Detail: &detail,
	}
}
