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
	r := buildClusterResource("p1", c)
	if r.Ref.String() != "gcp:p1:Cluster:my-cluster" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	d := r.Detail.(*inventory.ClusterDetail)
	if d.Version != "1.30.0-gke.100" || d.NodeCount != 3 ||
		d.NodeMachine != "e2-medium" || d.NodeDiskGB != 100 || d.Autopilot {
		t.Errorf("detail = %+v", d)
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
