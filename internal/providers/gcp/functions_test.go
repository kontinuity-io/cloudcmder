package gcp

import (
	"context"
	"testing"

	"cloud.google.com/go/functions/apiv2/functionspb"
	"cloud.google.com/go/run/apiv2/runpb"
	"google.golang.org/api/iterator"

	"cloudcmder.com/internal/inventory"
)

func TestParseMemory(t *testing.T) {
	cases := map[string]int64{
		"":         0,
		"512Mi":    512,
		"1Gi":      1024,
		"2Gi":      2048,
		"256M":     256,
		"1G":       1024,
		"50331648": 48, // 48 MiB in bytes
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := parseMemory(in); got != want {
				t.Errorf("parseMemory(%q) = %d, want %d", in, got, want)
			}
		})
	}
}

func TestParseCPUs(t *testing.T) {
	cases := map[string]float64{
		"":      0,
		"1000m": 1.0,
		"500m":  0.5,
		"2":     2.0,
		"0.25":  0.25,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := parseCPUs(in); got != want {
				t.Errorf("parseCPUs(%q) = %v, want %v", in, got, want)
			}
		})
	}
}

func TestRegionFromResourceName(t *testing.T) {
	got := regionFromResourceName("projects/p/locations/us-central1/services/foo")
	if got != "us-central1" {
		t.Errorf("got %q, want us-central1", got)
	}
	if regionFromResourceName("invalid/path") != "" {
		t.Errorf("expected empty for malformed path")
	}
}

func TestBuildRunServiceResource(t *testing.T) {
	s := &runpb.Service{
		Name: "projects/p1/locations/us-central1/services/my-svc",
		Template: &runpb.RevisionTemplate{
			Scaling: &runpb.RevisionScaling{MaxInstanceCount: 100},
			Containers: []*runpb.Container{
				{
					Image: "us-docker.pkg.dev/cloudrun/container/hello",
					Resources: &runpb.ResourceRequirements{
						Limits: map[string]string{"memory": "512Mi", "cpu": "1000m"},
					},
				},
			},
		},
	}
	r := buildRunServiceResource("p1", s, false)
	d := r.Detail.(*inventory.FunctionDetail)
	if d.MemoryMiB != 512 || d.CPUs != 1.0 || d.MaxInst != 100 {
		t.Errorf("detail = %+v", d)
	}
	if d.Trigger != "HTTP" || d.Region != "us-central1" {
		t.Errorf("detail = %+v", d)
	}
}

func TestBuildCloudFunctionResource(t *testing.T) {
	f := &functionspb.Function{
		Name: "projects/p1/locations/us-central1/functions/my-fn",
		BuildConfig: &functionspb.BuildConfig{
			Runtime: "go122",
		},
		ServiceConfig: &functionspb.ServiceConfig{
			AvailableMemory:  "256M",
			AvailableCpu:     "0.5",
			MaxInstanceCount: 50,
		},
	}
	r := buildCloudFunctionResource("p1", f, false)
	d := r.Detail.(*inventory.FunctionDetail)
	if d.Runtime != "go122" || d.MemoryMiB != 256 || d.CPUs != 0.5 || d.MaxInst != 50 {
		t.Errorf("detail = %+v", d)
	}
}

// --- fake run / functions clients -----------------------------------------

type fakeRunClient struct {
	items []*runpb.Service
}

func (f *fakeRunClient) List(_ context.Context, _ string) runServicesIterator {
	return &fakeRunIter{c: f}
}

func (f *fakeRunClient) Close() error { return nil }

type fakeRunIter struct {
	c   *fakeRunClient
	idx int
}

func (it *fakeRunIter) Next() (*runpb.Service, error) {
	if it.idx >= len(it.c.items) {
		return nil, iterator.Done
	}
	s := it.c.items[it.idx]
	it.idx++
	return s, nil
}

type fakeFunctionsClient struct {
	items []*functionspb.Function
}

func (f *fakeFunctionsClient) List(_ context.Context, _ string) functionsIterator {
	return &fakeFunctionsIter{c: f}
}

func (f *fakeFunctionsClient) Close() error { return nil }

type fakeFunctionsIter struct {
	c   *fakeFunctionsClient
	idx int
}

func (it *fakeFunctionsIter) Next() (*functionspb.Function, error) {
	if it.idx >= len(it.c.items) {
		return nil, iterator.Done
	}
	f := it.c.items[it.idx]
	it.idx++
	return f, nil
}
