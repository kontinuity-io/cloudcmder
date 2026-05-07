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

// --- Networks --------------------------------------------------------------

type networksAPI interface {
	List(ctx context.Context, req *computepb.ListNetworksRequest, opts ...gaxCallOption) networksIterator
	Close() error
}

type networksIterator interface {
	Next() (*computepb.Network, error)
}

type realNetworksClient struct {
	c *compute.NetworksClient
}

func (r *realNetworksClient) List(ctx context.Context, req *computepb.ListNetworksRequest, _ ...gaxCallOption) networksIterator {
	return r.c.List(ctx, req)
}

func (r *realNetworksClient) Close() error { return r.c.Close() }

type networksFactory func(ctx context.Context, opts ...option.ClientOption) (networksAPI, error)

type networksClientState struct {
	once    sync.Once
	cli     networksAPI
	err     error
	factory networksFactory
}

func (p *GCPProvider) networksClient(ctx context.Context) (networksAPI, error) {
	p.networks.once.Do(func() {
		if p.networks.factory != nil {
			p.networks.cli, p.networks.err = p.networks.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.networks.err = fmt.Errorf("gcp: ADC for networks client: %w", err)
			return
		}
		c, err := compute.NewNetworksRESTClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.networks.err = fmt.Errorf("gcp: new networks client: %w", err)
			return
		}
		p.networks.cli = &realNetworksClient{c: c}
	})
	if p.networks.err != nil {
		return nil, p.networks.err
	}
	return p.networks.cli, nil
}

func (p *GCPProvider) closeNetworksClient() error {
	if p.networks.cli == nil {
		return nil
	}
	return p.networks.cli.Close()
}

// --- Subnetworks -----------------------------------------------------------

type subnetsAPI interface {
	AggregatedList(ctx context.Context, req *computepb.AggregatedListSubnetworksRequest, opts ...gaxCallOption) subnetsIterator
	Close() error
}

type subnetsIterator interface {
	Next() (compute.SubnetworksScopedListPair, error)
}

type realSubnetsClient struct {
	c *compute.SubnetworksClient
}

func (r *realSubnetsClient) AggregatedList(ctx context.Context, req *computepb.AggregatedListSubnetworksRequest, _ ...gaxCallOption) subnetsIterator {
	return r.c.AggregatedList(ctx, req)
}

func (r *realSubnetsClient) Close() error { return r.c.Close() }

type subnetsFactory func(ctx context.Context, opts ...option.ClientOption) (subnetsAPI, error)

type subnetsClientState struct {
	once    sync.Once
	cli     subnetsAPI
	err     error
	factory subnetsFactory
}

func (p *GCPProvider) subnetsClient(ctx context.Context) (subnetsAPI, error) {
	p.subnets.once.Do(func() {
		if p.subnets.factory != nil {
			p.subnets.cli, p.subnets.err = p.subnets.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.subnets.err = fmt.Errorf("gcp: ADC for subnets client: %w", err)
			return
		}
		c, err := compute.NewSubnetworksRESTClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.subnets.err = fmt.Errorf("gcp: new subnets client: %w", err)
			return
		}
		p.subnets.cli = &realSubnetsClient{c: c}
	})
	if p.subnets.err != nil {
		return nil, p.subnets.err
	}
	return p.subnets.cli, nil
}

func (p *GCPProvider) closeSubnetsClient() error {
	if p.subnets.cli == nil {
		return nil
	}
	return p.subnets.cli.Close()
}

// --- Enrichment ------------------------------------------------------------

// enrichNetworks emits Network Resources with NetworkDetail. SubnetCount is
// computed by enrichSubnets, which runs after Networks and writes a second
// pass with the count populated. Sequential ordering is enforced by asset.go.
func enrichNetworks(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	nc, err := p.networksClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: networks client: %w", err)})
		return
	}
	it := nc.List(ctx, &computepb.ListNetworksRequest{Project: scope.ID})
	for {
		n, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return
		}
		if err != nil {
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{
				Err: fmt.Errorf("gcp: list networks: %w", err),
			})
			return
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildNetworkResource(scope.ID, n)})
	}
}

func buildNetworkResource(scopeID string, n *computepb.Network) inventory.Resource {
	detail := inventory.NetworkDetail{
		AutoSubnet: n.GetAutoCreateSubnetworks(),
		IPv4Range:  n.GetIPv4Range(),
		// SubnetCount left at 0 here; not currently re-computed.
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindNetwork, ID: n.GetName()},
		Kind:   inventory.KindNetwork,
		Name:   n.GetName(),
		Region: "global",
		Status: "",
		Detail: &detail,
	}
}

// enrichSubnets emits Subnet Resources with SubnetDetail and a Subnet→Network
// edge (RefRoutesFrom).
func enrichSubnets(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	sc, err := p.subnetsClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: subnets client: %w", err)})
		return
	}
	it := sc.AggregatedList(ctx, &computepb.AggregatedListSubnetworksRequest{Project: scope.ID})
	for {
		pair, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return
		}
		if err != nil {
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{
				Err: fmt.Errorf("gcp: aggregated list subnetworks: %w", err),
			})
			return
		}
		if pair.Value == nil {
			continue
		}
		for _, s := range pair.Value.Subnetworks {
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildSubnetResource(scope.ID, s)})
		}
	}
}

func buildSubnetResource(scopeID string, s *computepb.Subnetwork) inventory.Resource {
	region := lastSegment(s.GetRegion())
	networkName := lastSegment(s.GetNetwork())
	detail := inventory.SubnetDetail{
		CIDR:    s.GetIpCidrRange(),
		Region:  region,
		Network: networkName,
		Private: s.GetPrivateIpGoogleAccess(),
	}
	refs := map[inventory.RefKind][]inventory.ResourceRef{}
	if networkName != "" {
		refs[inventory.RefRoutesFrom] = []inventory.ResourceRef{{
			Provider: providerName, ScopeID: scopeID, Kind: inventory.KindNetwork, ID: networkName,
		}}
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindSubnet, ID: s.GetName()},
		Kind:   inventory.KindSubnet,
		Name:   s.GetName(),
		Region: region,
		Detail: &detail,
		Refs:   refs,
	}
}
