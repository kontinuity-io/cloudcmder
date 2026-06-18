package gcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	memcache "cloud.google.com/go/memcache/apiv1"
	"cloud.google.com/go/memcache/apiv1/memcachepb"
	redis "cloud.google.com/go/redis/apiv1"
	"cloud.google.com/go/redis/apiv1/redispb"
	rediscluster "cloud.google.com/go/redis/cluster/apiv1"
	"cloud.google.com/go/redis/cluster/apiv1/clusterpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// msInstance is the provider-internal projection of one Memorystore cache —
// classic Redis, Redis Cluster, or Memcache flattened into a single shape.
// It is the seam the build/enrich logic is tested against; the real client
// (untested, like every other realXClient) translates the three SDK proto
// types into this projection.
type msInstance struct {
	Name         string // full GCP resource name; Ref.ID is its last segment
	Subtype      string // "Redis" | "RedisCluster" | "Memcache"
	ServiceType  string // family label: "Redis" | "Redis Cluster" | "Memcached"
	Region       string
	Status       string
	NodeType     string
	Tier         string
	MemorySizeGB int
	ShardCount   int32
	ReplicaCount int32
	Version      string
	Labels       map[string]string
}

// memorystoreAPI is the seam between enrichMemorystore and the three GCP
// Memorystore SDKs. Each method lists one sub-service; one failing (e.g. its
// API disabled) returns an error the enricher logs and skips without aborting
// the others. Tests inject a fake; production uses realMemorystoreClient.
type memorystoreAPI interface {
	ListRedis(ctx context.Context, projectID string) ([]msInstance, error)
	ListRedisClusters(ctx context.Context, projectID string) ([]msInstance, error)
	ListMemcache(ctx context.Context, projectID string) ([]msInstance, error)
	Close() error
}

// realMemorystoreClient holds credential options rather than live clients:
// each Memorystore SDK client binds to its own service, and --scan-all reuses
// one provider across many projects, so we build fresh per-project clients
// inside each list method (mirroring realBigQueryClient).
type realMemorystoreClient struct {
	opts []option.ClientOption
}

func (r *realMemorystoreClient) ListRedis(ctx context.Context, projectID string) ([]msInstance, error) {
	c, err := redis.NewCloudRedisClient(ctx, r.opts...)
	if err != nil {
		return nil, fmt.Errorf("new redis client: %w", err)
	}
	defer func() { _ = c.Close() }()

	parent := "projects/" + projectID + "/locations/-"
	it := c.ListInstances(ctx, &redispb.ListInstancesRequest{Parent: parent})
	var out []msInstance
	for {
		in, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list redis instances: %w", err)
		}
		out = append(out, redisProjection(in))
	}
	return out, nil
}

func (r *realMemorystoreClient) ListRedisClusters(ctx context.Context, projectID string) ([]msInstance, error) {
	c, err := rediscluster.NewCloudRedisClusterClient(ctx, r.opts...)
	if err != nil {
		return nil, fmt.Errorf("new redis cluster client: %w", err)
	}
	defer func() { _ = c.Close() }()

	parent := "projects/" + projectID + "/locations/-"
	it := c.ListClusters(ctx, &clusterpb.ListClustersRequest{Parent: parent})
	var out []msInstance
	for {
		cl, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list redis clusters: %w", err)
		}
		out = append(out, redisClusterProjection(cl))
	}
	return out, nil
}

func (r *realMemorystoreClient) ListMemcache(ctx context.Context, projectID string) ([]msInstance, error) {
	c, err := memcache.NewCloudMemcacheClient(ctx, r.opts...)
	if err != nil {
		return nil, fmt.Errorf("new memcache client: %w", err)
	}
	defer func() { _ = c.Close() }()

	parent := "projects/" + projectID + "/locations/-"
	it := c.ListInstances(ctx, &memcachepb.ListInstancesRequest{Parent: parent})
	var out []msInstance
	for {
		in, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list memcache instances: %w", err)
		}
		out = append(out, memcacheProjection(in))
	}
	return out, nil
}

func (r *realMemorystoreClient) Close() error { return nil }

// redisProjection flattens a classic Redis instance. Region comes from the
// resource name path; ShardCount stays 0 (classic Redis is single-shard).
func redisProjection(in *redispb.Instance) msInstance {
	return msInstance{
		Name:         in.GetName(),
		Subtype:      "Redis",
		ServiceType:  "Redis",
		Region:       regionFromResourceName(in.GetName()),
		Status:       in.GetState().String(),
		Tier:         in.GetTier().String(),
		MemorySizeGB: int(in.GetMemorySizeGb()),
		ReplicaCount: in.GetReplicaCount(),
		Version:      in.GetRedisVersion(),
		Labels:       in.GetLabels(),
	}
}

// redisClusterProjection flattens a Redis Cluster. The cluster proto exposes
// no engine version, so Version stays empty; Tier is N/A for clusters.
func redisClusterProjection(cl *clusterpb.Cluster) msInstance {
	return msInstance{
		Name:         cl.GetName(),
		Subtype:      "RedisCluster",
		ServiceType:  "Redis Cluster",
		Region:       regionFromResourceName(cl.GetName()),
		Status:       cl.GetState().String(),
		NodeType:     cl.GetNodeType().String(),
		MemorySizeGB: int(cl.GetSizeGb()),
		ShardCount:   cl.GetShardCount(),
		ReplicaCount: cl.GetReplicaCount(),
	}
}

// memcacheProjection flattens a Memcache instance. Provisioned memory is the
// sum across nodes (nodeCount * per-node MemorySizeMb), converted to GB.
// NodeType carries the per-node cpu/memory shape since Memcache has no enum
// node type. Tier/Shard/Replica are N/A for Memcache.
func memcacheProjection(in *memcachepb.Instance) msInstance {
	memGB := 0
	nodeType := ""
	if nc := in.GetNodeConfig(); nc != nil {
		memGB = int(in.GetNodeCount()) * int(nc.GetMemorySizeMb()) / 1024
		nodeType = fmt.Sprintf("%dvCPU/%dMB", nc.GetCpuCount(), nc.GetMemorySizeMb())
	}
	version := in.GetMemcacheVersion().String()
	if fv := in.GetMemcacheFullVersion(); fv != "" {
		version = fv
	}
	return msInstance{
		Name:         in.GetName(),
		Subtype:      "Memcache",
		ServiceType:  "Memcached",
		Region:       regionFromResourceName(in.GetName()),
		Status:       in.GetState().String(),
		NodeType:     nodeType,
		MemorySizeGB: memGB,
		Version:      version,
		Labels:       in.GetLabels(),
	}
}

type memorystoreFactory func(ctx context.Context, opts ...option.ClientOption) (memorystoreAPI, error)

type memorystoreClientState struct {
	once    sync.Once
	cli     memorystoreAPI
	err     error
	factory memorystoreFactory
}

func (p *GCPProvider) memorystoreClient(ctx context.Context) (memorystoreAPI, error) {
	p.memorystore.once.Do(func() {
		if p.memorystore.factory != nil {
			p.memorystore.cli, p.memorystore.err = p.memorystore.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.memorystore.err = fmt.Errorf("gcp: ADC for memorystore client: %w", err)
			return
		}
		p.memorystore.cli = &realMemorystoreClient{opts: []option.ClientOption{option.WithCredentials(creds)}}
	})
	if p.memorystore.err != nil {
		return nil, p.memorystore.err
	}
	return p.memorystore.cli, nil
}

func (p *GCPProvider) closeMemorystoreClient() error {
	if p.memorystore.cli == nil {
		return nil
	}
	return p.memorystore.cli.Close()
}

// enrichMemorystore emits MemorystoreDetail rows for all three Memorystore
// sub-services — classic Redis, Redis Cluster, and Memcache — all of which map
// to KindGCPMemorystore. Each emitted row overwrites the matching CAI Phase-1
// stub (Ref.ID = last segment of the resource name). If one sub-service errors
// (commonly its API being disabled) it surfaces but never aborts the others.
func enrichMemorystore(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	mc, err := p.memorystoreClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: memorystore client: %w", err)})
		return
	}

	lists := []struct {
		label string
		fn    func(context.Context, string) ([]msInstance, error)
	}{
		{"redis instances", mc.ListRedis},
		{"redis clusters", mc.ListRedisClusters},
		{"memcache instances", mc.ListMemcache},
	}

	for _, l := range lists {
		if ctx.Err() != nil {
			return
		}
		insts, err := l.fn(ctx, scope.ID)
		if err != nil {
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list %s: %w", l.label, err)})
			continue
		}
		for _, in := range insts {
			if ctx.Err() != nil {
				return
			}
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildMemorystoreResource(scope.ID, in)})
		}
	}
}

func buildMemorystoreResource(scopeID string, in msInstance) inventory.Resource {
	id := lastSegment(in.Name)
	detail := inventory.MemorystoreDetail{
		Subtype:      in.Subtype,
		Region:       in.Region,
		ServiceType:  in.ServiceType,
		NodeType:     in.NodeType,
		Tier:         in.Tier,
		MemorySizeGB: in.MemorySizeGB,
		ShardCount:   in.ShardCount,
		ReplicaCount: in.ReplicaCount,
		Version:      in.Version,
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindGCPMemorystore, ID: id},
		Kind:   inventory.KindGCPMemorystore,
		Name:   id,
		Region: in.Region,
		Status: in.Status,
		Labels: in.Labels,
		Detail: &detail,
	}
}
