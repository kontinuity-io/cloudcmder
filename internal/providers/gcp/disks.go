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

// disksAPI is the slice of *compute.DisksClient we use.
type disksAPI interface {
	AggregatedList(ctx context.Context, req *computepb.AggregatedListDisksRequest, opts ...gaxCallOption) disksIterator
	Close() error
}

type disksIterator interface {
	Next() (compute.DisksScopedListPair, error)
}

type realDisksClient struct {
	c *compute.DisksClient
}

func (r *realDisksClient) AggregatedList(ctx context.Context, req *computepb.AggregatedListDisksRequest, _ ...gaxCallOption) disksIterator {
	return r.c.AggregatedList(ctx, req)
}

func (r *realDisksClient) Close() error { return r.c.Close() }

type disksFactory func(ctx context.Context, opts ...option.ClientOption) (disksAPI, error)

type disksClientState struct {
	once    sync.Once
	cli     disksAPI
	err     error
	factory disksFactory
}

func (p *GCPProvider) disksClient(ctx context.Context) (disksAPI, error) {
	p.disks.once.Do(func() {
		if p.disks.factory != nil {
			p.disks.cli, p.disks.err = p.disks.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.disks.err = fmt.Errorf("gcp: ADC for disks client: %w", err)
			return
		}
		c, err := compute.NewDisksRESTClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.disks.err = fmt.Errorf("gcp: new disks client: %w", err)
			return
		}
		p.disks.cli = &realDisksClient{c: c}
	})
	if p.disks.err != nil {
		return nil, p.disks.err
	}
	return p.disks.cli, nil
}

func (p *GCPProvider) closeDisksClient() error {
	if p.disks.cli == nil {
		return nil
	}
	return p.disks.cli.Close()
}

// enrichDisks streams enriched Disk Resources, populating DiskDetail and
// re-emitting the Disk → VM (RefAttachedTo) edges from the disk side. M5
// already emitted those edges from the VM perspective; INSERT OR IGNORE on
// the edges PK keeps re-emission idempotent.
func enrichDisks(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	dc, err := p.disksClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: disks client: %w", err)})
		return
	}
	it := dc.AggregatedList(ctx, &computepb.AggregatedListDisksRequest{Project: scope.ID})
	for {
		pair, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return
		}
		if err != nil {
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{
				Err: fmt.Errorf("gcp: aggregated list disks: %w", err),
			})
			return
		}
		if pair.Value == nil {
			continue
		}
		for _, d := range pair.Value.Disks {
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildDiskResource(scope.ID, d)})
		}
	}
}

func buildDiskResource(scopeID string, d *computepb.Disk) inventory.Resource {
	zone := lastSegment(d.GetZone())
	users := make([]inventory.ResourceRef, 0, len(d.GetUsers()))
	for _, u := range d.GetUsers() {
		// User URLs look like .../zones/Z/instances/<name>
		name := lastSegment(u)
		if name == "" {
			continue
		}
		users = append(users, inventory.ResourceRef{
			Provider: providerName, ScopeID: scopeID, Kind: inventory.KindVM, ID: name,
		})
	}

	detail := inventory.DiskDetail{
		SizeGB:  d.GetSizeGb(),
		Type:    lastSegment(d.GetType()),
		Zone:    zone,
		InUseBy: users,
	}

	refs := map[inventory.RefKind][]inventory.ResourceRef{}
	if len(users) > 0 {
		refs[inventory.RefAttachedTo] = users
	}

	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindDisk, ID: d.GetName()},
		Kind:   inventory.KindDisk,
		Name:   d.GetName(),
		Region: zone,
		Status: d.GetStatus(),
		Labels: d.GetLabels(),
		Detail: &detail,
		Refs:   refs,
	}
}
