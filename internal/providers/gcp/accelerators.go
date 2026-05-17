package gcp

import (
	"sort"
	"strconv"
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/container/apiv1/containerpb"

	"cloudcmder.com/internal/inventory"
)

// vmAccelerators returns explicit (GuestAccelerators field) or implicit
// (machine-type-implied) accelerators for an instance. GCE only allows
// attaching explicit GPUs to N1 variants; A2/A3/G2 machine types have GPUs
// baked into the machine type and leave GuestAccelerators empty.
func vmAccelerators(inst *computepb.Instance) []inventory.Accelerator {
	explicit := explicitAccelerators(inst.GetGuestAccelerators())
	if len(explicit) > 0 {
		return explicit
	}
	return implicitAccelerators(lastSegment(inst.GetMachineType()))
}

func explicitAccelerators(in []*computepb.AcceleratorConfig) []inventory.Accelerator {
	out := make([]inventory.Accelerator, 0, len(in))
	for _, a := range in {
		out = append(out, inventory.Accelerator{
			Type:  lastSegment(a.GetAcceleratorType()),
			Count: a.GetAcceleratorCount(),
		})
	}
	return out
}

// implicitAccelerators derives the GPU complement from accelerator-optimized
// machine families (a2, a3, g2). Returns nil for CPU-only families.
// Source: https://cloud.google.com/compute/docs/gpus
func implicitAccelerators(machineType string) []inventory.Accelerator {
	if strings.HasPrefix(machineType, "g2-") {
		if n, ok := g2L4Counts[machineType]; ok {
			return []inventory.Accelerator{{Type: "nvidia-l4", Count: n}}
		}
		return nil
	}

	// a2 / a3: <generation>-<variant>-<count>g  e.g. a3-highgpu-8g
	// Only known variants are valid; unknown substrings return nil so future
	// families don't produce phantom GPU counts.
	var gpuType string
	switch {
	case strings.HasPrefix(machineType, "a2-ultragpu-"):
		gpuType = "nvidia-a100-80gb"
	case strings.HasPrefix(machineType, "a2-highgpu-"),
		strings.HasPrefix(machineType, "a2-megagpu-"):
		gpuType = "nvidia-tesla-a100"
	case strings.HasPrefix(machineType, "a3-ultragpu-"):
		gpuType = "nvidia-h200-141gb"
	case strings.HasPrefix(machineType, "a3-highgpu-"),
		strings.HasPrefix(machineType, "a3-megagpu-"),
		strings.HasPrefix(machineType, "a3-edgegpu-"):
		gpuType = "nvidia-h100-80gb"
	default:
		return nil
	}

	// Extract the GPU count from the trailing "-<N>g" suffix.
	parts := strings.Split(machineType, "-")
	last := parts[len(parts)-1] // e.g. "8g"
	if !strings.HasSuffix(last, "g") {
		return nil
	}
	n, err := strconv.Atoi(last[:len(last)-1])
	if err != nil || n <= 0 {
		return nil
	}
	return []inventory.Accelerator{{Type: gpuType, Count: int32(n)}}
}

// g2L4Counts maps g2 machine type name → L4 GPU count.
// Source: https://cloud.google.com/compute/docs/gpus/gpu-regions-zones
var g2L4Counts = map[string]int32{
	"g2-standard-4":  1,
	"g2-standard-8":  1,
	"g2-standard-12": 1,
	"g2-standard-16": 1,
	"g2-standard-24": 2,
	"g2-standard-32": 1,
	"g2-standard-48": 4,
	"g2-standard-96": 8,
}

// nodePoolAccelerators aggregates accelerator configs across all node pools,
// summing counts of identical types weighted by InitialNodeCount per pool.
// For pools whose Config has no explicit Accelerators entry (A2/A3/G2
// machine types), implicit GPU derivation is applied.
func nodePoolAccelerators(pools []*containerpb.NodePool) []inventory.Accelerator {
	totals := map[string]int32{}
	for _, p := range pools {
		cfg := p.GetConfig()
		if cfg == nil {
			continue
		}
		n := p.GetInitialNodeCount()
		if n <= 0 {
			n = 1
		}
		if accs := cfg.GetAccelerators(); len(accs) > 0 {
			for _, a := range accs {
				// AcceleratorCount is int64 in containerpb; cast to int32 — counts
				// are bounded by the hardware limits of a single node pool.
				totals[lastSegment(a.GetAcceleratorType())] += int32(a.GetAcceleratorCount()) * n
			}
		} else {
			// Baked-in GPU machine types (A2/A3/G2) have no explicit Accelerators.
			for _, ia := range implicitAccelerators(cfg.GetMachineType()) {
				totals[ia.Type] += ia.Count * n
			}
		}
	}
	if len(totals) == 0 {
		return nil
	}
	types := make([]string, 0, len(totals))
	for t := range totals {
		types = append(types, t)
	}
	sort.Strings(types)
	out := make([]inventory.Accelerator, len(types))
	for i, t := range types {
		out[i] = inventory.Accelerator{Type: t, Count: totals[t]}
	}
	return out
}
