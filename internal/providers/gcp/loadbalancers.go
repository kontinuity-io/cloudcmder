package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// --- Global forwarding rules ----------------------------------------------

type globalForwardingRulesAPI interface {
	List(ctx context.Context, req *computepb.ListGlobalForwardingRulesRequest, opts ...gaxCallOption) forwardingRulesGlobalIterator
	Close() error
}

type forwardingRulesGlobalIterator interface {
	Next() (*computepb.ForwardingRule, error)
}

type realGlobalForwardingRulesClient struct {
	c *compute.GlobalForwardingRulesClient
}

func (r *realGlobalForwardingRulesClient) List(ctx context.Context, req *computepb.ListGlobalForwardingRulesRequest, _ ...gaxCallOption) forwardingRulesGlobalIterator {
	return r.c.List(ctx, req)
}

func (r *realGlobalForwardingRulesClient) Close() error { return r.c.Close() }

type globalForwardingRulesFactory func(ctx context.Context, opts ...option.ClientOption) (globalForwardingRulesAPI, error)

type globalForwardingRulesClientState struct {
	once    sync.Once
	cli     globalForwardingRulesAPI
	err     error
	factory globalForwardingRulesFactory
}

func (p *GCPProvider) globalForwardingRulesClient(ctx context.Context) (globalForwardingRulesAPI, error) {
	p.gfwd.once.Do(func() {
		if p.gfwd.factory != nil {
			p.gfwd.cli, p.gfwd.err = p.gfwd.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.gfwd.err = fmt.Errorf("gcp: ADC for global fwd rules client: %w", err)
			return
		}
		c, err := compute.NewGlobalForwardingRulesRESTClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.gfwd.err = fmt.Errorf("gcp: new global fwd rules client: %w", err)
			return
		}
		p.gfwd.cli = &realGlobalForwardingRulesClient{c: c}
	})
	if p.gfwd.err != nil {
		return nil, p.gfwd.err
	}
	return p.gfwd.cli, nil
}

func (p *GCPProvider) closeGlobalForwardingRulesClient() error {
	if p.gfwd.cli == nil {
		return nil
	}
	return p.gfwd.cli.Close()
}

// --- Regional forwarding rules (aggregated) -------------------------------

type forwardingRulesAPI interface {
	AggregatedList(ctx context.Context, req *computepb.AggregatedListForwardingRulesRequest, opts ...gaxCallOption) forwardingRulesRegionalIterator
	Close() error
}

type forwardingRulesRegionalIterator interface {
	Next() (compute.ForwardingRulesScopedListPair, error)
}

type realForwardingRulesClient struct {
	c *compute.ForwardingRulesClient
}

func (r *realForwardingRulesClient) AggregatedList(ctx context.Context, req *computepb.AggregatedListForwardingRulesRequest, _ ...gaxCallOption) forwardingRulesRegionalIterator {
	return r.c.AggregatedList(ctx, req)
}

func (r *realForwardingRulesClient) Close() error { return r.c.Close() }

type forwardingRulesFactory func(ctx context.Context, opts ...option.ClientOption) (forwardingRulesAPI, error)

type forwardingRulesClientState struct {
	once    sync.Once
	cli     forwardingRulesAPI
	err     error
	factory forwardingRulesFactory
}

func (p *GCPProvider) forwardingRulesClient(ctx context.Context) (forwardingRulesAPI, error) {
	p.rfwd.once.Do(func() {
		if p.rfwd.factory != nil {
			p.rfwd.cli, p.rfwd.err = p.rfwd.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.rfwd.err = fmt.Errorf("gcp: ADC for fwd rules client: %w", err)
			return
		}
		c, err := compute.NewForwardingRulesRESTClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.rfwd.err = fmt.Errorf("gcp: new fwd rules client: %w", err)
			return
		}
		p.rfwd.cli = &realForwardingRulesClient{c: c}
	})
	if p.rfwd.err != nil {
		return nil, p.rfwd.err
	}
	return p.rfwd.cli, nil
}

func (p *GCPProvider) closeForwardingRulesClient() error {
	if p.rfwd.cli == nil {
		return nil
	}
	return p.rfwd.cli.Close()
}

// --- Enrichment ------------------------------------------------------------

// enrichLoadBalancers walks both global and regional forwarding rules and
// emits LoadBalancer Resources from the rule fields alone (architecture-spec
// "minimum-viable" — full BackendService composition is a v1.1 task).
func enrichLoadBalancers(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	if err := emitGlobalLBs(ctx, p, scope, ch); err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: err})
		return
	}
	if err := emitRegionalLBs(ctx, p, scope, ch); err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: err})
		return
	}
}

func emitGlobalLBs(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) error {
	gc, err := p.globalForwardingRulesClient(ctx)
	if err != nil {
		return fmt.Errorf("gcp: global fwd rules client: %w", err)
	}
	it := gc.List(ctx, &computepb.ListGlobalForwardingRulesRequest{Project: scope.ID})
	for {
		fr, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("gcp: list global forwarding rules: %w", err)
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildLBResource(scope.ID, fr, "global")})
	}
}

func emitRegionalLBs(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) error {
	rc, err := p.forwardingRulesClient(ctx)
	if err != nil {
		return fmt.Errorf("gcp: fwd rules client: %w", err)
	}
	it := rc.AggregatedList(ctx, &computepb.AggregatedListForwardingRulesRequest{Project: scope.ID})
	for {
		pair, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("gcp: aggregated list forwarding rules: %w", err)
		}
		if pair.Value == nil {
			continue
		}
		region := pair.Key
		if i := strings.LastIndex(region, "/"); i >= 0 {
			region = region[i+1:]
		}
		for _, fr := range pair.Value.ForwardingRules {
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildLBResource(scope.ID, fr, region)})
		}
	}
}

func buildLBResource(scopeID string, fr *computepb.ForwardingRule, region string) inventory.Resource {
	ports := fr.GetPorts()
	if len(ports) == 0 && fr.GetPortRange() != "" {
		ports = []string{fr.GetPortRange()}
	}
	detail := inventory.LoadBalancerDetail{
		Scheme:       fr.GetLoadBalancingScheme(),
		Protocol:     fr.GetIPProtocol(),
		IPAddress:    fr.GetIPAddress(),
		Ports:        ports,
		BackendCount: 0,
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindLoadBalancer, ID: fr.GetName()},
		Kind:   inventory.KindLoadBalancer,
		Name:   fr.GetName(),
		Region: region,
		Status: "",
		Detail: &detail,
	}
}
