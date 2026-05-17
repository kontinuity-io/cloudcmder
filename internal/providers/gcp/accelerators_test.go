package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"cloudcmder.com/internal/inventory"
)

func TestImplicitAccelerators(t *testing.T) {
	tests := []struct {
		machineType string
		want        []inventory.Accelerator
	}{
		// A2 explicit-GPU family
		{"a2-highgpu-1g", []inventory.Accelerator{{Type: "nvidia-tesla-a100", Count: 1}}},
		{"a2-highgpu-8g", []inventory.Accelerator{{Type: "nvidia-tesla-a100", Count: 8}}},
		{"a2-megagpu-16g", []inventory.Accelerator{{Type: "nvidia-tesla-a100", Count: 16}}},
		// A2 ultra (80 GB A100)
		{"a2-ultragpu-1g", []inventory.Accelerator{{Type: "nvidia-a100-80gb", Count: 1}}},
		{"a2-ultragpu-4g", []inventory.Accelerator{{Type: "nvidia-a100-80gb", Count: 4}}},
		// A3
		{"a3-highgpu-8g", []inventory.Accelerator{{Type: "nvidia-h100-80gb", Count: 8}}},
		{"a3-megagpu-8g", []inventory.Accelerator{{Type: "nvidia-h100-80gb", Count: 8}}},
		{"a3-edgegpu-8g", []inventory.Accelerator{{Type: "nvidia-h100-80gb", Count: 8}}},
		// A3 ultra (H200)
		{"a3-ultragpu-8g", []inventory.Accelerator{{Type: "nvidia-h200-141gb", Count: 8}}},
		// G2 / L4
		{"g2-standard-4", []inventory.Accelerator{{Type: "nvidia-l4", Count: 1}}},
		{"g2-standard-24", []inventory.Accelerator{{Type: "nvidia-l4", Count: 2}}},
		{"g2-standard-96", []inventory.Accelerator{{Type: "nvidia-l4", Count: 8}}},
		// CPU-only families → nil
		{"n1-standard-4", nil},
		{"e2-medium", nil},
		{"n2-standard-8", nil},
		{"c3-standard-4", nil},
		// Unknown a2/a3 suffix → nil
		{"a2-fake-99g", nil},
	}
	for _, tt := range tests {
		t.Run(tt.machineType, func(t *testing.T) {
			got := implicitAccelerators(tt.machineType)
			assert.Equal(t, tt.want, got)
		})
	}
}
