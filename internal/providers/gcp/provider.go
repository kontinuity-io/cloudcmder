package gcp

import (
	"context"
	"errors"
	"fmt"

	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// providerName is the short name returned by Provider.Name and embedded in
// every ResourceRef this package emits.
const providerName = "gcp"

// GCPProvider implements inventory.Provider for Google Cloud.
type GCPProvider struct {
	projects *resourcemanager.ProjectsClient
}

// New constructs a GCPProvider using Application Default Credentials.
// Pass option.ClientOption values to override the endpoint or auth — used by
// tests to point at an httptest server.
func New(ctx context.Context, opts ...option.ClientOption) (*GCPProvider, error) {
	if len(opts) == 0 {
		creds, err := NewCredentials(ctx)
		if err != nil {
			return nil, fmt.Errorf("gcp: resolve ADC: %w", err)
		}
		opts = []option.ClientOption{option.WithCredentials(creds)}
	}
	pc, err := resourcemanager.NewProjectsRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("gcp: new projects client: %w", err)
	}
	return &GCPProvider{projects: pc}, nil
}

// Close releases the underlying REST client.
func (p *GCPProvider) Close() error {
	if p.projects == nil {
		return nil
	}
	return p.projects.Close()
}

// Name returns the provider's short name.
func (p *GCPProvider) Name() string { return providerName }

// ListResources is implemented in M2; the channel is pre-closed so any caller
// that ranges over it terminates instead of deadlocking on a nil channel.
func (p *GCPProvider) ListResources(ctx context.Context, _ inventory.Scope, _ []inventory.Kind) (<-chan inventory.ResourceOrErr, error) {
	ch := make(chan inventory.ResourceOrErr)
	close(ch)
	return ch, errors.New("gcp: ListResources not implemented in M1")
}

// GetDetail is implemented in M5+ when per-kind enrichment lands.
func (p *GCPProvider) GetDetail(ctx context.Context, _ inventory.ResourceRef) (inventory.Resource, error) {
	return inventory.Resource{}, errors.New("gcp: GetDetail not implemented in M1")
}

// Telemetry returns nil in v1; v1.1 adds Cloud Monitoring overlays.
func (p *GCPProvider) Telemetry() inventory.Telemetry { return nil }
