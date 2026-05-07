package screens

import (
	"encoding/json"
	"fmt"

	"cloudcmder.com/internal/inventory"
)

// ColumnDef describes one column in a ResourceList table. Extract receives the
// already-decoded Detail (kind-specific struct) so each column doesn't pay the
// JSON-unmarshal cost separately.
type ColumnDef struct {
	Header  string
	Width   int
	Extract func(r inventory.Resource, detail any) string
}

// columnsFor returns the column set registered for the given Kind. The second
// return value reports whether a kind-specific column set exists; callers use
// it to decide between the real ResourceList and the M5-era ResourceListStub.
func columnsFor(kind inventory.Kind) ([]ColumnDef, bool) {
	switch kind {
	case inventory.KindVM:
		return vmColumns(), true
	default:
		return nil, false
	}
}

// decodeDetail unmarshals the json.RawMessage that LoadResources places in
// Resource.Detail into the kind-specific struct. Returns nil for unknown
// kinds — extractors must guard against that.
func decodeDetail(kind inventory.Kind, raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	switch kind {
	case inventory.KindVM:
		var d inventory.VMDetail
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil
		}
		return &d
	default:
		return nil
	}
}

func vmColumns() []ColumnDef {
	return []ColumnDef{
		{Header: "NAME", Width: 22, Extract: func(r inventory.Resource, _ any) string { return r.Name }},
		{Header: "ZONE", Width: 16, Extract: func(_ inventory.Resource, d any) string {
			if vm, ok := d.(*inventory.VMDetail); ok && vm != nil {
				return vm.Zone
			}
			return ""
		}},
		{Header: "MACHINE", Width: 14, Extract: func(_ inventory.Resource, d any) string {
			if vm, ok := d.(*inventory.VMDetail); ok && vm != nil {
				return vm.MachineType
			}
			return ""
		}},
		{Header: "VCPU", Width: 5, Extract: func(_ inventory.Resource, d any) string {
			if vm, ok := d.(*inventory.VMDetail); ok && vm != nil && vm.VCPUs > 0 {
				return fmt.Sprintf("%d", vm.VCPUs)
			}
			return ""
		}},
		{Header: "RAM GIB", Width: 8, Extract: func(_ inventory.Resource, d any) string {
			if vm, ok := d.(*inventory.VMDetail); ok && vm != nil && vm.MemoryMiB > 0 {
				return fmt.Sprintf("%.1f", float64(vm.MemoryMiB)/1024.0)
			}
			return ""
		}},
		{Header: "OS", Width: 12, Extract: func(_ inventory.Resource, d any) string {
			if vm, ok := d.(*inventory.VMDetail); ok && vm != nil {
				return vm.OSFamily
			}
			return ""
		}},
		{Header: "STATUS", Width: 10, Extract: func(r inventory.Resource, _ any) string { return r.Status }},
	}
}
