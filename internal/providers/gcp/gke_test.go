package gcp

import (
	"context"
	"testing"

	"cloud.google.com/go/container/apiv1/containerpb"

	"cloudcmder.com/internal/inventory"
)

func TestBuildClusterResource(t *testing.T) {
	c := &containerpb.Cluster{
		Name:                 "my-cluster",
		Location:             "us-central1",
		CurrentMasterVersion: "1.30.0-gke.100",
		CurrentNodeCount:     3,
		Status:               containerpb.Cluster_RUNNING,
		Autopilot:            &containerpb.Autopilot{Enabled: false},
		NodePools: []*containerpb.NodePool{
			{
				Name: "default-pool",
				Config: &containerpb.NodeConfig{
					MachineType: "e2-medium",
					DiskSizeGb:  100,
				},
			},
		},
	}
	r := buildClusterResource("p1", c, false)
	if r.Ref.String() != "gcp:p1:Cluster:us-central1/my-cluster" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	d := r.Detail.(*inventory.ClusterDetail)
	if d.Version != "1.30.0-gke.100" || d.NodeCount != 3 ||
		d.NodeMachine != "e2-medium" || d.NodeDiskGB != 100 || d.Serverless {
		t.Errorf("detail = %+v", d)
	}
}

func TestBuildClusterResourceRefIncludesLocation(t *testing.T) {
	clusters := []*containerpb.Cluster{
		{Name: "shared", Location: "us-central1"},
		{Name: "shared", Location: "europe-west1"},
	}
	got := map[string]bool{}
	for _, c := range clusters {
		r := buildClusterResource("p1", c, false)
		got[r.Ref.String()] = true
	}
	for _, want := range []string{
		"gcp:p1:Cluster:us-central1/shared",
		"gcp:p1:Cluster:europe-west1/shared",
	} {
		if !got[want] {
			t.Fatalf("missing ref %q in %v", want, got)
		}
	}
	if len(got) != 2 {
		t.Fatalf("refs collided: %v", got)
	}
}

func TestBuildClusterResourceAccelerators(t *testing.T) {
	tests := []struct {
		name  string
		pools []*containerpb.NodePool
		want  []inventory.Accelerator
	}{
		{
			name:  "no GPU pools",
			pools: []*containerpb.NodePool{{Config: &containerpb.NodeConfig{MachineType: "e2-medium"}}},
			want:  nil,
		},
		{
			name: "explicit accelerator config",
			pools: []*containerpb.NodePool{{
				InitialNodeCount: 2,
				Config: &containerpb.NodeConfig{
					MachineType: "n1-standard-8",
					Accelerators: []*containerpb.AcceleratorConfig{
						{AcceleratorType: "nvidia-tesla-t4", AcceleratorCount: 1},
					},
				},
			}},
			// 1 GPU × 2 nodes = 2 total
			want: []inventory.Accelerator{{Type: "nvidia-tesla-t4", Count: 2}},
		},
		{
			name: "implicit A3 pool",
			pools: []*containerpb.NodePool{{
				InitialNodeCount: 4,
				Config:           &containerpb.NodeConfig{MachineType: "a3-highgpu-8g"},
			}},
			// 8 GPUs × 4 nodes = 32 total
			want: []inventory.Accelerator{{Type: "nvidia-h100-80gb", Count: 32}},
		},
		{
			name: "mixed pools aggregated",
			pools: []*containerpb.NodePool{
				{
					InitialNodeCount: 2,
					Config: &containerpb.NodeConfig{
						MachineType: "n1-standard-8",
						Accelerators: []*containerpb.AcceleratorConfig{
							{AcceleratorType: "nvidia-tesla-a100", AcceleratorCount: 2},
						},
					},
				},
				{
					InitialNodeCount: 3,
					Config:           &containerpb.NodeConfig{MachineType: "g2-standard-4"},
				},
			},
			// A100: 2×2=4, L4: 1×3=3; sorted by type
			want: []inventory.Accelerator{
				{Type: "nvidia-l4", Count: 3},
				{Type: "nvidia-tesla-a100", Count: 4},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nodePoolAccelerators(tt.pools)
			if len(tt.want) == 0 {
				if len(got) != 0 {
					t.Errorf("got %v, want nil/empty", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] got %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// --- fake GKE client -------------------------------------------------------

type fakeGKEClient struct {
	items []*containerpb.Cluster
	err   error
}

func (f *fakeGKEClient) ListClusters(_ context.Context, _ string) ([]*containerpb.Cluster, error) {
	return f.items, f.err
}

func (f *fakeGKEClient) Close() error { return nil }
