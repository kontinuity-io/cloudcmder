package gcp

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

func TestBuildMemorystoreResource(t *testing.T) {
	cases := []struct {
		name    string
		in      msInstance
		wantID  string
		wantSub string
	}{
		{
			name: "redis",
			in: msInstance{
				Name:         "projects/p1/locations/us-central1/instances/cache-a",
				Subtype:      "Redis",
				ServiceType:  "Redis",
				Region:       "us-central1",
				Status:       "READY",
				Tier:         "STANDARD_HA",
				MemorySizeGB: 5,
				ReplicaCount: 2,
				Version:      "REDIS_7_0",
			},
			wantID:  "cache-a",
			wantSub: "Redis",
		},
		{
			name: "cluster",
			in: msInstance{
				Name:         "projects/p1/locations/us-east1/clusters/clstr-b",
				Subtype:      "RedisCluster",
				ServiceType:  "Redis Cluster",
				Region:       "us-east1",
				Status:       "ACTIVE",
				NodeType:     "REDIS_HIGHMEM_MEDIUM",
				MemorySizeGB: 26,
				ShardCount:   3,
				ReplicaCount: 1,
			},
			wantID:  "clstr-b",
			wantSub: "RedisCluster",
		},
		{
			name: "memcache",
			in: msInstance{
				Name:         "projects/p1/locations/europe-west1/instances/mc-c",
				Subtype:      "Memcache",
				ServiceType:  "Memcached",
				Region:       "europe-west1",
				Status:       "READY",
				NodeType:     "2vCPU/4096MB",
				MemorySizeGB: 8,
				Version:      "MEMCACHE_1_6_15",
			},
			wantID:  "mc-c",
			wantSub: "Memcache",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := buildMemorystoreResource("p1", tc.in)
			if r.Kind != inventory.KindGCPMemorystore {
				t.Fatalf("kind = %q", r.Kind)
			}
			// Ref.ID must equal the CAI stub id (last path segment) to overwrite.
			if r.Ref.ID != tc.wantID {
				t.Errorf("ref id = %q, want %q (must match CAI stub id)", r.Ref.ID, tc.wantID)
			}
			if r.Name != tc.wantID {
				t.Errorf("name = %q, want %q", r.Name, tc.wantID)
			}
			if r.Region != tc.in.Region || r.Status != tc.in.Status {
				t.Errorf("header region/status = %q/%q", r.Region, r.Status)
			}
			md, ok := r.Detail.(*inventory.MemorystoreDetail)
			if !ok {
				t.Fatalf("detail type = %T", r.Detail)
			}
			if md.Subtype != tc.wantSub {
				t.Errorf("subtype = %q, want %q", md.Subtype, tc.wantSub)
			}
			if md.ServiceType != tc.in.ServiceType || md.NodeType != tc.in.NodeType ||
				md.Tier != tc.in.Tier || md.MemorySizeGB != tc.in.MemorySizeGB ||
				md.ShardCount != tc.in.ShardCount || md.ReplicaCount != tc.in.ReplicaCount ||
				md.Version != tc.in.Version {
				t.Errorf("detail = %+v", md)
			}
		})
	}
}

// --- fake memorystoreAPI ---------------------------------------------------

type fakeMemorystoreClient struct {
	redis       []msInstance
	clusters    []msInstance
	memcache    []msInstance
	redisErr    error
	clusterErr  error
	memcacheErr error
}

func (f *fakeMemorystoreClient) ListRedis(_ context.Context, _ string) ([]msInstance, error) {
	return f.redis, f.redisErr
}

func (f *fakeMemorystoreClient) ListRedisClusters(_ context.Context, _ string) ([]msInstance, error) {
	return f.clusters, f.clusterErr
}

func (f *fakeMemorystoreClient) ListMemcache(_ context.Context, _ string) ([]msInstance, error) {
	return f.memcache, f.memcacheErr
}

func (f *fakeMemorystoreClient) Close() error { return nil }

func drainMemorystore(ch <-chan inventory.ResourceOrErr) ([]inventory.Resource, []error) {
	var res []inventory.Resource
	var errs []error
	for x := range ch {
		if x.Err != nil {
			errs = append(errs, x.Err)
			continue
		}
		res = append(res, x.Resource)
	}
	return res, errs
}

func TestEnrichMemorystoreAggregatesAllThree(t *testing.T) {
	p := &GCPProvider{}
	p.memorystore.factory = func(_ context.Context, _ ...option.ClientOption) (memorystoreAPI, error) {
		return &fakeMemorystoreClient{
			redis: []msInstance{
				{Name: "projects/p1/locations/us-central1/instances/r1", Subtype: "Redis", ServiceType: "Redis", Region: "us-central1"},
			},
			clusters: []msInstance{
				{Name: "projects/p1/locations/us-east1/clusters/c1", Subtype: "RedisCluster", ServiceType: "Redis Cluster", Region: "us-east1", ShardCount: 3},
			},
			memcache: []msInstance{
				{Name: "projects/p1/locations/europe-west1/instances/m1", Subtype: "Memcache", ServiceType: "Memcached", Region: "europe-west1"},
			},
		}, nil
	}

	ch := make(chan inventory.ResourceOrErr, 16)
	enrichMemorystore(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	got, errs := drainMemorystore(ch)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(got) != 3 {
		t.Fatalf("got %d resources, want 3", len(got))
	}

	bySub := map[string]inventory.Resource{}
	for _, r := range got {
		md, ok := r.Detail.(*inventory.MemorystoreDetail)
		if !ok {
			t.Fatalf("detail type = %T", r.Detail)
		}
		bySub[md.Subtype] = r
	}
	for _, want := range []string{"Redis", "RedisCluster", "Memcache"} {
		if _, ok := bySub[want]; !ok {
			t.Errorf("missing subtype %q in aggregated output", want)
		}
	}
	if bySub["Redis"].Ref.ID != "r1" || bySub["RedisCluster"].Ref.ID != "c1" || bySub["Memcache"].Ref.ID != "m1" {
		t.Errorf("ref ids = %q/%q/%q", bySub["Redis"].Ref.ID, bySub["RedisCluster"].Ref.ID, bySub["Memcache"].Ref.ID)
	}
}

// TestEnrichMemorystoreSkipsFailingSubservice verifies one sub-service erroring
// (e.g. its API disabled) surfaces an error but never aborts the others.
func TestEnrichMemorystoreSkipsFailingSubservice(t *testing.T) {
	p := &GCPProvider{}
	p.memorystore.factory = func(_ context.Context, _ ...option.ClientOption) (memorystoreAPI, error) {
		return &fakeMemorystoreClient{
			redis: []msInstance{
				{Name: "projects/p1/locations/us-central1/instances/r1", Subtype: "Redis"},
			},
			clusterErr: errors.New("redis cluster API disabled"),
			memcache: []msInstance{
				{Name: "projects/p1/locations/europe-west1/instances/m1", Subtype: "Memcache"},
			},
		}, nil
	}

	ch := make(chan inventory.ResourceOrErr, 16)
	enrichMemorystore(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	got, errs := drainMemorystore(ch)
	if len(errs) != 1 {
		t.Fatalf("got %d errs, want 1 (the failing sub-service)", len(errs))
	}
	if len(got) != 2 {
		t.Fatalf("got %d resources, want 2 (redis + memcache survive)", len(got))
	}
}

func TestEnrichMemorystoreClientError(t *testing.T) {
	p := &GCPProvider{}
	p.memorystore.factory = func(_ context.Context, _ ...option.ClientOption) (memorystoreAPI, error) {
		return nil, errors.New("boom")
	}

	ch := make(chan inventory.ResourceOrErr, 4)
	enrichMemorystore(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	_, errs := drainMemorystore(ch)
	if len(errs) != 1 {
		t.Fatalf("got %d errs, want 1", len(errs))
	}
}
