package gcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"golang.org/x/sync/singleflight"
	"google.golang.org/api/option"
)

// machineTypesAPI is the subset of the Compute MachineTypesClient we exercise.
// Defined as an interface so tests can substitute a fake.
type machineTypesAPI interface {
	Get(ctx context.Context, req *computepb.GetMachineTypeRequest, opts ...gaxCallOption) (*computepb.MachineType, error)
	Close() error
}

type realMachineTypesClient struct {
	c *compute.MachineTypesClient
}

func (r *realMachineTypesClient) Get(ctx context.Context, req *computepb.GetMachineTypeRequest, _ ...gaxCallOption) (*computepb.MachineType, error) {
	return r.c.Get(ctx, req)
}

func (r *realMachineTypesClient) Close() error { return r.c.Close() }

type machineTypesFactory func(ctx context.Context, opts ...option.ClientOption) (machineTypesAPI, error)

// parseMachineType handles custom machine types directly so the API call can
// be skipped. Accepts both legacy `custom-N-M` and family-prefixed
// `<family>-custom-N-M` (e.g. `e2-custom-4-8192`). The memory part is MiB per
// GCP's quirky-but-documented usage of the field.
func parseMachineType(name string) (vCPU int32, memMiB int64, ok bool) {
	parts := strings.Split(name, "-")
	idx := -1
	for i, p := range parts {
		if p == "custom" {
			idx = i
			break
		}
	}
	if idx < 0 || idx+2 >= len(parts) {
		return 0, 0, false
	}
	vc, err := strconv.Atoi(parts[idx+1])
	if err != nil || vc <= 0 {
		return 0, 0, false
	}
	mem, err := strconv.Atoi(parts[idx+2])
	if err != nil || mem <= 0 {
		return 0, 0, false
	}
	return int32(vc), int64(mem), true
}

// machineTypeCache memoises predefined-machine-type lookups for the duration
// of one scan. singleflight collapses concurrent identical lookups into a
// single API call (matters when many VMs share `n2-standard-4`).
type machineTypeCache struct {
	sf    *singleflight.Group
	cache sync.Map // key "zone|mt" → mtEntry
}

func newMachineTypeCache() *machineTypeCache {
	return &machineTypeCache{sf: &singleflight.Group{}}
}

type mtEntry struct {
	v int32
	m int64
}

// Get returns (VCPUs, MemoryMiB) for a machine type, hitting the API at most
// once per (zone, mt) pair across the lifetime of the cache.
func (c *machineTypeCache) Get(ctx context.Context, mtClient machineTypesAPI, project, zone, mt string) (int32, int64, error) {
	key := zone + "|" + mt
	if v, ok := c.cache.Load(key); ok {
		e := v.(mtEntry)
		return e.v, e.m, nil
	}
	out, err, _ := c.sf.Do(key, func() (any, error) {
		res, err := mtClient.Get(ctx, &computepb.GetMachineTypeRequest{
			Project: project, Zone: zone, MachineType: mt,
		})
		if err != nil {
			return mtEntry{}, fmt.Errorf("get machine type %s/%s: %w", zone, mt, err)
		}
		e := mtEntry{v: res.GetGuestCpus(), m: int64(res.GetMemoryMb())}
		c.cache.Store(key, e)
		return e, nil
	})
	if err != nil {
		return 0, 0, err
	}
	e := out.(mtEntry)
	return e.v, e.m, nil
}
