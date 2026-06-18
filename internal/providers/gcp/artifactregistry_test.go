package gcp

import (
	"context"
	"testing"

	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

func TestBuildArtifactRegistryResource(t *testing.T) {
	repo := arRepository{
		Name:      "projects/p1/locations/us-central1/repositories/my-repo",
		Region:    "us-central1",
		Format:    "DOCKER",
		Mode:      "STANDARD_REPOSITORY",
		SizeBytes: 1 << 30,
	}
	r := buildArtifactRegistryResource("p1", repo)

	if r.Kind != inventory.KindGCPArtifactRegistry || r.Name != "my-repo" || r.Region != "us-central1" {
		t.Fatalf("resource header = %+v", r)
	}
	if r.Ref.ID != "my-repo" {
		t.Errorf("ref id = %q, want my-repo (must match CAI stub id to overwrite)", r.Ref.ID)
	}
	ad, ok := r.Detail.(*inventory.ArtifactRegistryDetail)
	if !ok {
		t.Fatalf("detail type = %T", r.Detail)
	}
	if ad.Subtype != "Repository" || ad.Region != "us-central1" {
		t.Errorf("detail = %+v", ad)
	}
	if ad.Format != "DOCKER" || ad.Mode != "STANDARD_REPOSITORY" || ad.SizeBytes != 1<<30 {
		t.Errorf("detail = %+v", ad)
	}
}

// TestBuildArtifactRegistryResourceRegionFromName confirms the region is
// recovered from the resource name when the projection left it empty.
func TestBuildArtifactRegistryResourceRegionFromName(t *testing.T) {
	repo := arRepository{
		Name: "projects/p1/locations/europe-west4/repositories/r2",
	}
	r := buildArtifactRegistryResource("p1", repo)
	if r.Region != "europe-west4" {
		t.Errorf("region = %q, want europe-west4", r.Region)
	}
	if r.Ref.ID != "r2" {
		t.Errorf("ref id = %q, want r2", r.Ref.ID)
	}
}

// --- fake artifactRegistryAPI ----------------------------------------------

type fakeArtifactRegistryClient struct {
	repos []arRepository
	err   error
}

func (f *fakeArtifactRegistryClient) ListRepositories(_ context.Context, _ string) ([]arRepository, error) {
	return f.repos, f.err
}

func (f *fakeArtifactRegistryClient) Close() error { return nil }

func TestEnrichArtifactRegistry(t *testing.T) {
	p := &GCPProvider{}
	p.artifactRegistry.factory = func(_ context.Context, _ ...option.ClientOption) (artifactRegistryAPI, error) {
		return &fakeArtifactRegistryClient{repos: []arRepository{
			{Name: "projects/p1/locations/us-central1/repositories/docker-repo", Region: "us-central1", Format: "DOCKER", Mode: "STANDARD_REPOSITORY", SizeBytes: 2048},
			{Name: "projects/p1/locations/europe-west1/repositories/maven-repo", Region: "europe-west1", Format: "MAVEN", Mode: "REMOTE_REPOSITORY"},
		}}, nil
	}

	ch := make(chan inventory.ResourceOrErr, 8)
	enrichArtifactRegistry(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	var got []inventory.Resource
	for x := range ch {
		if x.Err != nil {
			t.Fatalf("unexpected err: %v", x.Err)
		}
		got = append(got, x.Resource)
	}
	if len(got) != 2 {
		t.Fatalf("got %d resources, want 2", len(got))
	}
	if got[0].Name != "docker-repo" || got[1].Name != "maven-repo" {
		t.Errorf("names = %q, %q", got[0].Name, got[1].Name)
	}
	d0 := got[0].Detail.(*inventory.ArtifactRegistryDetail)
	if d0.Format != "DOCKER" || d0.Mode != "STANDARD_REPOSITORY" || d0.SizeBytes != 2048 {
		t.Errorf("docker-repo detail = %+v", d0)
	}
	d1 := got[1].Detail.(*inventory.ArtifactRegistryDetail)
	if d1.Format != "MAVEN" || d1.Mode != "REMOTE_REPOSITORY" || d1.Region != "europe-west1" {
		t.Errorf("maven-repo detail = %+v", d1)
	}
}

func TestEnrichArtifactRegistryClientError(t *testing.T) {
	p := &GCPProvider{}
	p.artifactRegistry.factory = func(_ context.Context, _ ...option.ClientOption) (artifactRegistryAPI, error) {
		return &fakeArtifactRegistryClient{err: context.DeadlineExceeded}, nil
	}
	ch := make(chan inventory.ResourceOrErr, 4)
	enrichArtifactRegistry(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	var sawErr bool
	for x := range ch {
		if x.Err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatalf("expected an error to be emitted when ListRepositories fails")
	}
}
