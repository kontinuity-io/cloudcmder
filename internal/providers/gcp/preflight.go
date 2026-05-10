package gcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"google.golang.org/api/option"
	"google.golang.org/api/serviceusage/v1"

	"cloudcmder.com/internal/inventory"
)

// alwaysRequired lists APIs cloudcmder needs regardless of which Kinds are
// scanned: project enumeration, asset listing, and the preflight check itself.
var alwaysRequired = []string{
	"cloudresourcemanager.googleapis.com",
	"cloudasset.googleapis.com",
	"serviceusage.googleapis.com",
}

// RequiredAPIs returns the deduplicated, sorted list of GCP APIs cloudcmder
// needs to enrich every supported Kind. Derived from assetTypeToKind so adding
// a new asset type automatically updates this list without touching preflight.go.
func RequiredAPIs() []string {
	set := make(map[string]struct{}, len(assetTypeToKind)+len(alwaysRequired))
	for _, a := range alwaysRequired {
		set[a] = struct{}{}
	}
	for at := range assetTypeToKind {
		if i := strings.Index(at, "/"); i > 0 {
			set[at[:i]] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// PreflightResult is the per-project diff between required and enabled APIs.
type PreflightResult struct {
	Scope    inventory.Scope
	Required []string
	Enabled  []string
	Missing  []string
}

// GcloudEnableCommand returns the ready-to-run `gcloud services enable …`
// command to fix all missing APIs in r.Scope. Returns empty string if nothing
// is missing.
func (r PreflightResult) GcloudEnableCommand() string {
	if len(r.Missing) == 0 {
		return ""
	}
	return fmt.Sprintf("gcloud services enable %s --project=%s",
		strings.Join(r.Missing, " "), r.Scope.ID)
}

// Preflight checks one project's enabled APIs against RequiredAPIs() and
// returns the diff. Requires roles/viewer (or roles/serviceusage.serviceUsageViewer).
func (p *GCPProvider) Preflight(ctx context.Context, scope inventory.Scope) (PreflightResult, error) {
	enabled, err := p.listEnabledServices(ctx, scope.ID)
	if err != nil {
		return PreflightResult{}, err
	}
	enabledSet := make(map[string]struct{}, len(enabled))
	for _, s := range enabled {
		enabledSet[s] = struct{}{}
	}
	required := RequiredAPIs()
	var missing []string
	for _, a := range required {
		if _, ok := enabledSet[a]; !ok {
			missing = append(missing, a)
		}
	}
	return PreflightResult{
		Scope:    scope,
		Required: required,
		Enabled:  enabled,
		Missing:  missing,
	}, nil
}

func (p *GCPProvider) listEnabledServices(ctx context.Context, projectID string) ([]string, error) {
	svc, err := p.serviceUsage(ctx)
	if err != nil {
		return nil, err
	}
	parent := "projects/" + projectID
	var enabled []string
	call := svc.Services.List(parent).Filter("state:ENABLED").PageSize(200)
	err = call.Pages(ctx, func(page *serviceusage.ListServicesResponse) error {
		for _, s := range page.Services {
			// s.Name is the full resource name: projects/NUM/services/X.googleapis.com
			if i := strings.LastIndex(s.Name, "/"); i >= 0 {
				name := s.Name[i+1:]
				if strings.HasSuffix(name, ".googleapis.com") {
					enabled = append(enabled, name)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("gcp: list enabled services for %s: %w", projectID, err)
	}
	sort.Strings(enabled)
	return enabled, nil
}

// serviceUsageState holds the lazy-initialised Service Usage REST client,
// mirroring the assetState pattern in asset.go.
type serviceUsageState struct {
	suOnce    sync.Once
	suSvc     *serviceusage.Service
	suErr     error
	suFactory func(ctx context.Context) (*serviceusage.Service, error)
}

func (p *GCPProvider) serviceUsage(ctx context.Context) (*serviceusage.Service, error) {
	p.suOnce.Do(func() {
		if p.suFactory != nil {
			p.suSvc, p.suErr = p.suFactory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.suErr = fmt.Errorf("gcp: resolve ADC for serviceusage: %w", err)
			return
		}
		svc, err := serviceusage.NewService(ctx, option.WithCredentials(creds))
		if err != nil {
			p.suErr = fmt.Errorf("gcp: new serviceusage client: %w", err)
			return
		}
		p.suSvc = svc
	})
	if p.suErr != nil {
		return nil, p.suErr
	}
	return p.suSvc, nil
}
