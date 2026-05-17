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

// instancesAPI is the subset of compute.InstancesClient that we exercise.
type instancesAPI interface {
	AggregatedList(ctx context.Context, req *computepb.AggregatedListInstancesRequest, opts ...gaxCallOption) instancesIterator
	Close() error
}

// instancesIterator matches *compute.InstancesScopedListPairIterator.Next().
type instancesIterator interface {
	Next() (compute.InstancesScopedListPair, error)
}

type realInstancesClient struct {
	c *compute.InstancesClient
}

func (r *realInstancesClient) AggregatedList(ctx context.Context, req *computepb.AggregatedListInstancesRequest, _ ...gaxCallOption) instancesIterator {
	return r.c.AggregatedList(ctx, req)
}

func (r *realInstancesClient) Close() error { return r.c.Close() }

type instancesFactory func(ctx context.Context, opts ...option.ClientOption) (instancesAPI, error)

// computeState bundles the lazy compute clients embedded into GCPProvider.
type computeState struct {
	instancesOnce sync.Once
	instancesCli  instancesAPI
	instancesErr  error
	instancesFact instancesFactory

	mtOnce sync.Once
	mtCli  machineTypesAPI
	mtErr  error
	mtFact machineTypesFactory
}

// instancesClient returns the lazily-initialised Instances client.
func (p *GCPProvider) instancesClient(ctx context.Context) (instancesAPI, error) {
	p.instancesOnce.Do(func() {
		if p.instancesFact != nil {
			p.instancesCli, p.instancesErr = p.instancesFact(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.instancesErr = fmt.Errorf("gcp: resolve ADC for instances client: %w", err)
			return
		}
		c, err := compute.NewInstancesRESTClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.instancesErr = fmt.Errorf("gcp: new instances client: %w", err)
			return
		}
		p.instancesCli = &realInstancesClient{c: c}
	})
	if p.instancesErr != nil {
		return nil, p.instancesErr
	}
	return p.instancesCli, nil
}

// machineTypesClient returns the lazily-initialised MachineTypes client.
func (p *GCPProvider) machineTypesClient(ctx context.Context) (machineTypesAPI, error) {
	p.mtOnce.Do(func() {
		if p.mtFact != nil {
			p.mtCli, p.mtErr = p.mtFact(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.mtErr = fmt.Errorf("gcp: resolve ADC for machine types client: %w", err)
			return
		}
		c, err := compute.NewMachineTypesRESTClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.mtErr = fmt.Errorf("gcp: new machine types client: %w", err)
			return
		}
		p.mtCli = &realMachineTypesClient{c: c}
	})
	if p.mtErr != nil {
		return nil, p.mtErr
	}
	return p.mtCli, nil
}

func (p *GCPProvider) closeInstancesClient() error {
	if p.instancesCli == nil {
		return nil
	}
	return p.instancesCli.Close()
}

func (p *GCPProvider) closeMachineTypesClient() error {
	if p.mtCli == nil {
		return nil
	}
	return p.mtCli.Close()
}

// machineTypeResolver returns vCPU/MemoryMiB for a given (zone, machine type)
// pair. Custom types are parsed locally; predefined types hit the API via the
// supplied resolver function.
type machineTypeResolver func(ctx context.Context, zone, mt string) (vCPU int32, memMiB int64, err error)

// enrichVMs streams enriched VM Resources plus disk-side stub Resources onto
// the given channel. Disk stubs carry Refs[AttachedTo]=[VM] so the store
// persists the VM↔Disk edges; the resource row itself is INSERT-OR-REPLACE'd
// over the asset-listing stub with identical (name, region, status).
//
// Edge directionality follows architecture.md: edges are emitted from the
// subject of the verb. "Disk attached to VM" → from=Disk, to=VM.
func enrichVMs(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	ic, err := p.instancesClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: instances client: %w", err)})
		return
	}

	cache := newMachineTypeCache()
	resolveMT := func(c context.Context, zone, mt string) (int32, int64, error) {
		if v, m, ok := parseMachineType(mt); ok {
			return v, m, nil
		}
		mtClient, err := p.machineTypesClient(c)
		if err != nil {
			return 0, 0, err
		}
		return cache.Get(c, mtClient, scope.ID, zone, mt)
	}

	req := &computepb.AggregatedListInstancesRequest{Project: scope.ID}
	it := ic.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return
		}
		if err != nil {
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{
				Err: fmt.Errorf("gcp: aggregated list instances: %w", err),
			})
			return
		}
		if pair.Value == nil {
			continue
		}
		for _, inst := range pair.Value.Instances {
			vmRes := buildVMResource(ctx, scope.ID, inst, resolveMT, p.dumpNative)
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: vmRes})
			// Disk → VM AttachedTo edges are emitted from the disk side
			// in enrichDisks via Disk.Users(). Emitting a Disk stub here
			// (with no Detail) races against enrichDisks under the M8
			// concurrent fan-out and clobbers the disk's SizeGB/Type/etc
			// via INSERT OR REPLACE — the "all disks 0 GB" bug.
		}
	}
}

// buildVMResource is a pure mapper from one *Instance to a Resource. Exported
// (lowercase but referenced by the test file in the same package) so the
// happy-path can be unit-tested without spinning up an iterator fake.
func buildVMResource(ctx context.Context, scopeID string, inst *computepb.Instance, resolveMT machineTypeResolver, dumpNative bool) inventory.Resource {
	name := inst.GetName()
	zone := lastSegment(inst.GetZone())
	mt := lastSegment(inst.GetMachineType())

	detail := inventory.VMDetail{
		MachineType:  mt,
		Zone:         zone,
		CPUPlatform:  inst.GetCpuPlatform(),
		Accelerators: vmAccelerators(inst),
	}
	if v, m, err := resolveMT(ctx, zone, mt); err == nil {
		detail.VCPUs = v
		detail.MemoryMiB = m
	}
	if sched := inst.GetScheduling(); sched != nil {
		detail.Preemptible = sched.GetPreemptible()
		detail.Spot = sched.GetProvisioningModel() == "SPOT"
	}

	nics := make([]inventory.NICDetail, 0, len(inst.GetNetworkInterfaces()))
	subnetRefs := make([]inventory.ResourceRef, 0, len(inst.GetNetworkInterfaces()))
	for _, ni := range inst.GetNetworkInterfaces() {
		nic := inventory.NICDetail{
			Network:    lastSegment(ni.GetNetwork()),
			Subnetwork: lastSegment(ni.GetSubnetwork()),
			InternalIP: ni.GetNetworkIP(),
		}
		if acs := ni.GetAccessConfigs(); len(acs) > 0 {
			nic.ExternalIP = acs[0].GetNatIP()
		}
		nics = append(nics, nic)
		if ref := vmSubnetRef(scopeID, ni.GetSubnetwork()); ref.ID != "" {
			subnetRefs = append(subnetRefs, ref)
		}
	}
	detail.NICs = nics

	var attached []inventory.DiskRef
	var allLicenses []string
	for _, ad := range inst.GetDisks() {
		d := inventory.DiskRef{
			Name:   lastSegment(ad.GetSource()),
			SizeGB: ad.GetDiskSizeGb(),
			Type:   ad.GetType(),
		}
		diskLicenses := ad.GetLicenses()
		if ad.GetBoot() {
			detail.BootDisk = d
			// OSFamily is derived from the boot disk's first license only —
			// it is the human-readable OS label (e.g. "debian-11"), orthogonal
			// to the billing classification in LicenseClass.
			if len(diskLicenses) > 0 {
				detail.OSFamily = parseLicenseURL(diskLicenses[0])
			}
		} else {
			attached = append(attached, d)
		}
		allLicenses = append(allLicenses, diskLicenses...)
	}
	detail.AttachedDisks = attached
	// Aggregate license info across all disks: any-marketplace-wins precedence.
	detail.Licenses, detail.MarketplaceProject, detail.MarketplaceClass = licenseInfoFromURLs(allLicenses)

	refs := map[inventory.RefKind][]inventory.ResourceRef{}
	if len(subnetRefs) > 0 {
		refs[inventory.RefRoutesFrom] = subnetRefs
	}

	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindVM, ID: name},
		Kind:   inventory.KindVM,
		Name:   name,
		Region: zone,
		Status: inst.GetStatus(),
		Labels: inst.GetLabels(),
		Detail: &detail,
		Refs:   refs,
		Native: nativeFrom(dumpNative, inst),
	}
}

func sendOrCancel(ctx context.Context, ch chan<- inventory.ResourceOrErr, r inventory.ResourceOrErr) {
	select {
	case ch <- r:
	case <-ctx.Done():
	}
}
