package inventory

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// AcceleratorSummary returns a compact display string like "8×nvidia-h100-80gb (640GB)"
// for a single type or "8×nvidia-h100-80gb, 2×nvidia-l4 (688GB)" for mixed.
// The VRAM total is appended only when every type in the slice has a known
// VRAM value. Returns "" when the slice is empty.
func AcceleratorSummary(accs []Accelerator) string {
	if len(accs) == 0 {
		return ""
	}
	parts := make([]string, len(accs))
	for i, a := range accs {
		parts[i] = fmt.Sprintf("%d×%s", a.Count, a.Type)
	}
	s := strings.Join(parts, ", ")
	if total := AcceleratorTotalVRAMGB(accs); total > 0 {
		s += fmt.Sprintf(" (%dGB)", total)
	}
	return s
}

// AcceleratorTotalVRAMGB returns the total GPU VRAM across all entries (count ×
// per-card VRAM). Returns 0 if any type has unknown VRAM so the caller can
// omit the field rather than show a misleading partial total.
func AcceleratorTotalVRAMGB(accs []Accelerator) int32 {
	var total int32
	for _, a := range accs {
		v := gpuVRAMGB(a.Type)
		if v == 0 {
			return 0
		}
		total += v * a.Count
	}
	return total
}

// gpuVRAMGB returns per-card VRAM in GB for a known GPU type name.
// Tries to parse the value from the type name first (e.g. "nvidia-h100-80gb" → 80),
// then falls back to the static table. Returns 0 for unknown types.
func gpuVRAMGB(typeName string) int32 {
	if strings.HasSuffix(typeName, "gb") {
		parts := strings.Split(typeName, "-")
		last := parts[len(parts)-1]    // e.g. "80gb"
		s := last[:len(last)-2]        // strip "gb"
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return int32(n)
		}
	}
	return gpuVRAMTable[typeName]
}

// gpuVRAMTable holds per-card VRAM in GB for GPU types whose names don't
// encode the size. Source: https://cloud.google.com/compute/docs/gpus
var gpuVRAMTable = map[string]int32{
	"nvidia-l4":          24,
	"nvidia-tesla-t4":    16,
	"nvidia-tesla-v100":  16,
	"nvidia-tesla-a100":  40,
	"nvidia-tesla-p100":  16,
	"nvidia-tesla-p4":    8,
	"nvidia-tesla-p40":   24,
	"nvidia-tesla-k80":   12,
}

// AcceleratorTotalCount sums Count across all entries. Returns 0 for an empty
// slice. Used by the Excel GPUCount column when a VM has mixed GPU types.
func AcceleratorTotalCount(accs []Accelerator) int32 {
	var total int32
	for _, a := range accs {
		total += a.Count
	}
	return total
}

// AcceleratorTypeList returns a comma-joined, deduplicated, sorted list of type
// names — e.g. "nvidia-h100-80gb,nvidia-l4". Used by the Excel GPUType column.
// Returns "" for an empty slice.
func AcceleratorTypeList(accs []Accelerator) string {
	if len(accs) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(accs))
	for _, a := range accs {
		seen[a.Type] = struct{}{}
	}
	types := make([]string, 0, len(seen))
	for t := range seen {
		types = append(types, t)
	}
	sort.Strings(types)
	return strings.Join(types, ",")
}
