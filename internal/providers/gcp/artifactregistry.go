package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	artifactregistry "cloud.google.com/go/artifactregistry/apiv1"
	"cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// arRepository is the provider-internal projection of one Artifact Registry
// repository. It is the seam the build/enrich logic is tested against — the
// real client (untested, like every other realXClient) translates the
// artifactregistrypb.Repository SDK type into this shape.
type arRepository struct {
	Name      string // full GCP resource name (last segment = repo id)
	Region    string // location component of Name
	Format    string // DOCKER | MAVEN | NPM | … ("" when unspecified)
	Mode      string // STANDARD_REPOSITORY | REMOTE_REPOSITORY | … ("" when unspecified)
	SizeBytes int64
}

// artifactRegistryAPI is the seam between enrichArtifactRegistry and Cloud
// Artifact Registry. Tests inject a fake; production uses
// realArtifactRegistryClient.
type artifactRegistryAPI interface {
	ListRepositories(ctx context.Context, projectID string) ([]arRepository, error)
	Close() error
}

// realArtifactRegistryClient holds the credential options rather than a live
// client: artifactregistry.NewClient is not project-bound, but we keep the
// same per-call construction pattern as the BigQuery client so a cached client
// never gets shared across projects during --scan-all. We build a fresh client
// inside ListRepositories instead.
type realArtifactRegistryClient struct {
	opts []option.ClientOption
}

func (r *realArtifactRegistryClient) ListRepositories(ctx context.Context, projectID string) ([]arRepository, error) {
	c, err := artifactregistry.NewClient(ctx, r.opts...)
	if err != nil {
		return nil, fmt.Errorf("new artifact registry client: %w", err)
	}
	defer func() { _ = c.Close() }()

	// "-" location wildcard lists repositories across every region in one call.
	parent := "projects/" + projectID + "/locations/-"
	var out []arRepository
	it := c.ListRepositories(ctx, &artifactregistrypb.ListRepositoriesRequest{Parent: parent})
	for {
		repo, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list repositories: %w", err)
		}
		out = append(out, arRepository{
			Name:      repo.GetName(),
			Region:    regionFromResourceName(repo.GetName()),
			Format:    artifactRegistryFormat(repo.GetFormat()),
			Mode:      artifactRegistryMode(repo.GetMode()),
			SizeBytes: repo.GetSizeBytes(),
		})
	}
	return out, nil
}

func (r *realArtifactRegistryClient) Close() error { return nil }

// artifactRegistryFormat renders a repository format enum as its short string,
// collapsing the UNSPECIFIED sentinel to "" so the TUI shows "—" rather than a
// misleading value.
func artifactRegistryFormat(f artifactregistrypb.Repository_Format) string {
	if f == artifactregistrypb.Repository_FORMAT_UNSPECIFIED {
		return ""
	}
	return f.String()
}

// artifactRegistryMode renders a repository mode enum as its short string,
// collapsing the UNSPECIFIED sentinel to "".
func artifactRegistryMode(m artifactregistrypb.Repository_Mode) string {
	if m == artifactregistrypb.Repository_MODE_UNSPECIFIED {
		return ""
	}
	return m.String()
}

type artifactRegistryFactory func(ctx context.Context, opts ...option.ClientOption) (artifactRegistryAPI, error)

type artifactRegistryClientState struct {
	once    sync.Once
	cli     artifactRegistryAPI
	err     error
	factory artifactRegistryFactory
}

func (p *GCPProvider) artifactRegistryClient(ctx context.Context) (artifactRegistryAPI, error) {
	p.artifactRegistry.once.Do(func() {
		if p.artifactRegistry.factory != nil {
			p.artifactRegistry.cli, p.artifactRegistry.err = p.artifactRegistry.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.artifactRegistry.err = fmt.Errorf("gcp: ADC for artifact registry client: %w", err)
			return
		}
		p.artifactRegistry.cli = &realArtifactRegistryClient{opts: []option.ClientOption{option.WithCredentials(creds)}}
	})
	if p.artifactRegistry.err != nil {
		return nil, p.artifactRegistry.err
	}
	return p.artifactRegistry.cli, nil
}

func (p *GCPProvider) closeArtifactRegistryClient() error {
	if p.artifactRegistry.cli == nil {
		return nil
	}
	return p.artifactRegistry.cli.Close()
}

// enrichArtifactRegistry emits ArtifactRegistryDetail rows at the repository
// grain. These overwrite the CAI Phase-1 stub rows (matching Ref ID = repo id).
// DockerImage stubs the enricher does not cover keep their Subtype-only
// StubDetail, which still decodes into ArtifactRegistryDetail (shared
// Subtype/Region prefix).
func enrichArtifactRegistry(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	ac, err := p.artifactRegistryClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: artifact registry client: %w", err)})
		return
	}
	repos, err := ac.ListRepositories(ctx, scope.ID)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list artifact registry repositories: %w", err)})
		return
	}
	for _, repo := range repos {
		if ctx.Err() != nil {
			return
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildArtifactRegistryResource(scope.ID, repo)})
	}
}

func buildArtifactRegistryResource(scopeID string, repo arRepository) inventory.Resource {
	id := lastSegment(repo.Name)
	region := repo.Region
	if region == "" {
		region = regionFromResourceName(repo.Name)
	}
	detail := inventory.ArtifactRegistryDetail{
		Subtype:   "Repository",
		Region:    region,
		Format:    strings.ToUpper(repo.Format),
		Mode:      strings.ToUpper(repo.Mode),
		SizeBytes: repo.SizeBytes,
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindGCPArtifactRegistry, ID: id},
		Kind:   inventory.KindGCPArtifactRegistry,
		Name:   id,
		Region: region,
		Detail: &detail,
	}
}
