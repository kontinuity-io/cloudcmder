package gcp

import (
	"context"
	"fmt"
	"sync"

	container "cloud.google.com/go/container/apiv1"
	"cloud.google.com/go/container/apiv1/containerpb"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

type gkeAPI interface {
	ListClusters(ctx context.Context, parent string) ([]*containerpb.Cluster, error)
	Close() error
}

type realGKEClient struct {
	c *container.ClusterManagerClient
}

func (r *realGKEClient) ListClusters(ctx context.Context, parent string) ([]*containerpb.Cluster, error) {
	resp, err := r.c.ListClusters(ctx, &containerpb.ListClustersRequest{Parent: parent})
	if err != nil {
		return nil, err
	}
	return resp.GetClusters(), nil
}

func (r *realGKEClient) Close() error { return r.c.Close() }

type gkeFactory func(ctx context.Context, opts ...option.ClientOption) (gkeAPI, error)

type gkeClientState struct {
	once    sync.Once
	cli     gkeAPI
	err     error
	factory gkeFactory
}

func (p *GCPProvider) gkeClient(ctx context.Context) (gkeAPI, error) {
	p.gke.once.Do(func() {
		if p.gke.factory != nil {
			p.gke.cli, p.gke.err = p.gke.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.gke.err = fmt.Errorf("gcp: ADC for gke client: %w", err)
			return
		}
		c, err := container.NewClusterManagerClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.gke.err = fmt.Errorf("gcp: new gke client: %w", err)
			return
		}
		p.gke.cli = &realGKEClient{c: c}
	})
	if p.gke.err != nil {
		return nil, p.gke.err
	}
	return p.gke.cli, nil
}

func (p *GCPProvider) closeGKEClient() error {
	if p.gke.cli == nil {
		return nil
	}
	return p.gke.cli.Close()
}

func enrichClusters(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	gc, err := p.gkeClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: gke client: %w", err)})
		return
	}
	// "-" means all locations.
	clusters, err := gc.ListClusters(ctx, "projects/"+scope.ID+"/locations/-")
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list gke clusters: %w", err)})
		return
	}
	for _, c := range clusters {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildClusterResource(scope.ID, c, p.dumpNative)})
	}
}

func buildClusterResource(scopeID string, c *containerpb.Cluster, dumpNative bool) inventory.Resource {
	detail := inventory.ClusterDetail{
		Version:   c.GetCurrentMasterVersion(),
		NodeCount: c.GetCurrentNodeCount(),
		Serverless: c.GetAutopilot().GetEnabled(),
		Location:  c.GetLocation(),
	}
	if pools := c.GetNodePools(); len(pools) > 0 {
		if cfg := pools[0].GetConfig(); cfg != nil {
			detail.NodeMachine = cfg.GetMachineType()
			detail.NodeDiskGB = int64(cfg.GetDiskSizeGb())
		}
		detail.Accelerators = nodePoolAccelerators(pools)
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindCluster, ID: c.GetName()},
		Kind:   inventory.KindCluster,
		Name:   c.GetName(),
		Region: c.GetLocation(),
		Status: c.GetStatus().String(),
		Labels: c.GetResourceLabels(),
		Detail: &detail,
		Native: nativeFrom(dumpNative, c),
	}
}
