package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAcceleratorSummary(t *testing.T) {
	tests := []struct {
		name string
		in   []Accelerator
		want string
	}{
		{"empty", nil, ""},
		// 8 × H100-80GB = 640 GB
		{"single known vram", []Accelerator{{Type: "nvidia-h100-80gb", Count: 8}}, "8×nvidia-h100-80gb (640GB)"},
		// 8×H100-80GB (640) + 2×L4 (48) = 688 GB
		{"mixed known vram", []Accelerator{
			{Type: "nvidia-h100-80gb", Count: 8},
			{Type: "nvidia-l4", Count: 2},
		}, "8×nvidia-h100-80gb, 2×nvidia-l4 (688GB)"},
		// unknown type → no VRAM suffix
		{"unknown type", []Accelerator{{Type: "nvidia-future-x", Count: 4}}, "4×nvidia-future-x"},
		// mixed where one type is unknown → no VRAM suffix
		{"mixed one unknown", []Accelerator{
			{Type: "nvidia-l4", Count: 2},
			{Type: "nvidia-future-x", Count: 1},
		}, "2×nvidia-l4, 1×nvidia-future-x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AcceleratorSummary(tt.in))
		})
	}
}

func TestAcceleratorTotalVRAMGB(t *testing.T) {
	tests := []struct {
		name string
		in   []Accelerator
		want int32
	}{
		{"empty", nil, 0},
		// name-encoded: nvidia-h100-80gb → 80 × 8 = 640
		{"h100 parsed from name", []Accelerator{{Type: "nvidia-h100-80gb", Count: 8}}, 640},
		// table lookup: nvidia-l4 → 24 × 2 = 48
		{"l4 from table", []Accelerator{{Type: "nvidia-l4", Count: 2}}, 48},
		// mixed: 640 + 48 = 688
		{"mixed", []Accelerator{
			{Type: "nvidia-h100-80gb", Count: 8},
			{Type: "nvidia-l4", Count: 2},
		}, 688},
		// unknown type returns 0 (can't compute partial total)
		{"unknown", []Accelerator{{Type: "nvidia-future-x", Count: 4}}, 0},
		// mixed with unknown → 0
		{"mixed with unknown", []Accelerator{
			{Type: "nvidia-l4", Count: 2},
			{Type: "nvidia-future-x", Count: 1},
		}, 0},
		// a100-80gb parsed from name: 80 × 4 = 320
		{"a100-80gb parsed", []Accelerator{{Type: "nvidia-a100-80gb", Count: 4}}, 320},
		// nvidia-tesla-a100 from table: 40 × 2 = 80
		{"tesla-a100 from table", []Accelerator{{Type: "nvidia-tesla-a100", Count: 2}}, 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AcceleratorTotalVRAMGB(tt.in))
		})
	}
}

func TestAcceleratorTotalCount(t *testing.T) {
	tests := []struct {
		name string
		in   []Accelerator
		want int32
	}{
		{"empty", nil, 0},
		{"single", []Accelerator{{Type: "nvidia-h100-80gb", Count: 8}}, 8},
		{"mixed", []Accelerator{
			{Type: "nvidia-h100-80gb", Count: 8},
			{Type: "nvidia-l4", Count: 2},
		}, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AcceleratorTotalCount(tt.in))
		})
	}
}

func TestAcceleratorTypeList(t *testing.T) {
	tests := []struct {
		name string
		in   []Accelerator
		want string
	}{
		{"empty", nil, ""},
		{"single", []Accelerator{{Type: "nvidia-l4", Count: 4}}, "nvidia-l4"},
		{"mixed sorted", []Accelerator{
			{Type: "nvidia-l4", Count: 2},
			{Type: "nvidia-h100-80gb", Count: 8},
		}, "nvidia-h100-80gb,nvidia-l4"},
		{"dedup", []Accelerator{
			{Type: "nvidia-l4", Count: 4},
			{Type: "nvidia-l4", Count: 2},
		}, "nvidia-l4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AcceleratorTypeList(tt.in))
		})
	}
}
