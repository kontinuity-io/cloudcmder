package inventory

import (
	"fmt"
	"sort"
	"strings"
)

// AcceleratorSummary returns a compact display string like "8×nvidia-h100-80gb"
// for a single type or "8×nvidia-h100-80gb, 2×nvidia-l4" for mixed. Returns ""
// when the slice is empty.
func AcceleratorSummary(accs []Accelerator) string {
	if len(accs) == 0 {
		return ""
	}
	parts := make([]string, len(accs))
	for i, a := range accs {
		parts[i] = fmt.Sprintf("%d×%s", a.Count, a.Type)
	}
	return strings.Join(parts, ", ")
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
