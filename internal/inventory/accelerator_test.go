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
		{"single", []Accelerator{{Type: "nvidia-h100-80gb", Count: 8}}, "8×nvidia-h100-80gb"},
		{"mixed", []Accelerator{
			{Type: "nvidia-h100-80gb", Count: 8},
			{Type: "nvidia-l4", Count: 2},
		}, "8×nvidia-h100-80gb, 2×nvidia-l4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AcceleratorSummary(tt.in))
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
