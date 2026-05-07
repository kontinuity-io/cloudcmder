package gcp

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
	"google.golang.org/api/iterator"

	"cloudcmder.com/internal/inventory"
)

// ListScopes returns every active GCP project the caller has at least
// resourcemanager.projects.get on. Non-active projects (DELETE_REQUESTED,
// STATE_UNSPECIFIED) are filtered out to mirror `gcloud projects list`.
func (p *GCPProvider) ListScopes(ctx context.Context) ([]inventory.Scope, error) {
	if p.projects == nil {
		return nil, errors.New("gcp: provider not initialized")
	}
	it := p.projects.SearchProjects(ctx, &resourcemanagerpb.SearchProjectsRequest{})
	var out []inventory.Scope
	for {
		proj, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("gcp: search projects: %w", err)
		}
		if proj.GetState() != resourcemanagerpb.Project_ACTIVE {
			continue
		}
		out = append(out, inventory.Scope{
			ProviderID:  providerName,
			ID:          proj.GetProjectId(),
			DisplayName: proj.GetDisplayName(),
			Parent:      proj.GetParent(),
			Labels:      proj.GetLabels(),
		})
	}
	return out, nil
}
