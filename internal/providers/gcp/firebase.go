package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"google.golang.org/api/firebase/v1beta1"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// firebaseProjectInfo is the provider-internal projection of a Firebase project
// returned by the realFirebaseClient. Using a projection struct keeps the
// interface independent of the SDK type so tests never need an SDK import.
type firebaseProjectInfo struct {
	ProjectID     string
	DisplayName   string
	ProjectNumber int64
	LocationID    string // from Resources.LocationId (deprecated but best available)
}

// firebaseAppCounts holds per-platform app counts for a Firebase project.
type firebaseAppCounts struct {
	Web     int
	Android int
	IOS     int
}

// firebaseAPI is the client seam for the Firebase Management REST API.
// Methods build a per-call SDK client internally so --scan-all across projects
// works without global state.
type firebaseAPI interface {
	// GetProject returns project metadata for the given projectID.
	// Returns (zero, nil) when the API is not enabled or permission is denied —
	// callers treat zero-value as "graceful unavailable".
	GetProject(ctx context.Context, projectID string) (firebaseProjectInfo, error)
	// ListApps counts Web, Android, and iOS apps for the given projectID.
	// Returns zeroed counts on any error (non-fatal).
	ListApps(ctx context.Context, projectID string) (firebaseAppCounts, error)
	Close() error
}

// realFirebaseClient implements firebaseAPI using google.golang.org/api/firebase/v1beta1.
// It creates a fresh SDK service per project call, which is stateless and safe
// across concurrent goroutines — mirroring the pattern in storage.go.
type realFirebaseClient struct {
	opts []option.ClientOption
}

func (r *realFirebaseClient) svc(ctx context.Context) (*firebase.Service, error) {
	return firebase.NewService(ctx, r.opts...)
}

func (r *realFirebaseClient) GetProject(ctx context.Context, projectID string) (firebaseProjectInfo, error) {
	svc, err := r.svc(ctx)
	if err != nil {
		return firebaseProjectInfo{}, fmt.Errorf("firebase: new service: %w", err)
	}
	proj, err := svc.Projects.Get("projects/" + projectID).Context(ctx).Do()
	if err != nil {
		return firebaseProjectInfo{}, fmt.Errorf("firebase: get project %s: %w", projectID, err)
	}
	info := firebaseProjectInfo{
		ProjectID:     proj.ProjectId,
		DisplayName:   proj.DisplayName,
		ProjectNumber: proj.ProjectNumber,
	}
	if proj.Resources != nil {
		info.LocationID = proj.Resources.LocationId
	}
	return info, nil
}

func (r *realFirebaseClient) ListApps(ctx context.Context, projectID string) (firebaseAppCounts, error) {
	svc, err := r.svc(ctx)
	if err != nil {
		return firebaseAppCounts{}, fmt.Errorf("firebase: new service: %w", err)
	}
	parent := "projects/" + projectID
	var counts firebaseAppCounts

	if err := svc.Projects.WebApps.List(parent).Context(ctx).Pages(ctx, func(page *firebase.ListWebAppsResponse) error {
		counts.Web += len(page.Apps)
		return nil
	}); err != nil {
		return firebaseAppCounts{}, fmt.Errorf("firebase: list web apps %s: %w", projectID, err)
	}
	if err := svc.Projects.AndroidApps.List(parent).Context(ctx).Pages(ctx, func(page *firebase.ListAndroidAppsResponse) error {
		counts.Android += len(page.Apps)
		return nil
	}); err != nil {
		return firebaseAppCounts{}, fmt.Errorf("firebase: list android apps %s: %w", projectID, err)
	}
	if err := svc.Projects.IosApps.List(parent).Context(ctx).Pages(ctx, func(page *firebase.ListIosAppsResponse) error {
		counts.IOS += len(page.Apps)
		return nil
	}); err != nil {
		return firebaseAppCounts{}, fmt.Errorf("firebase: list ios apps %s: %w", projectID, err)
	}
	return counts, nil
}

func (r *realFirebaseClient) Close() error { return nil }

// firebaseFactory is the constructor type injected during tests.
type firebaseFactory func(ctx context.Context, opts ...option.ClientOption) (firebaseAPI, error)

// firebaseClientState bundles the lazy-init fields for the Firebase client.
type firebaseClientState struct {
	once    sync.Once
	cli     firebaseAPI
	err     error
	factory firebaseFactory
}

// firebaseClient returns the cached Firebase API client, building one on first use.
func (p *GCPProvider) firebaseClient(ctx context.Context) (firebaseAPI, error) {
	p.firebase.once.Do(func() {
		if p.firebase.factory != nil {
			p.firebase.cli, p.firebase.err = p.firebase.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.firebase.err = fmt.Errorf("gcp: ADC for firebase client: %w", err)
			return
		}
		p.firebase.cli = &realFirebaseClient{opts: []option.ClientOption{option.WithCredentials(creds)}}
	})
	if p.firebase.err != nil {
		return nil, p.firebase.err
	}
	return p.firebase.cli, nil
}

// closeFirebaseClient releases the Firebase client if it was created.
func (p *GCPProvider) closeFirebaseClient() error {
	if p.firebase.cli == nil {
		return nil
	}
	return p.firebase.cli.Close()
}

// enrichFirebase emits FirebaseDetail rows for the Project grain only.
// AppInfo rows remain stubs (they carry no useful per-app enrichment from the
// Management API that isn't already surfaced via the Project row).
// Any API error (API not enabled, permission denied, etc.) is logged as a
// warning and the enricher returns 0 rows — the scan continues cleanly.
func enrichFirebase(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	fc, err := p.firebaseClient(ctx)
	if err != nil {
		// Client construction failure is unusual (ADC missing); log and bail.
		slog.Warn("gcp: firebase client unavailable; skipping Firebase enrichment",
			"scope", scope.ID, "error", err)
		return
	}

	// The CAI stub for the FirebaseProject grain uses the GCP project ID as
	// its resource ID (last path segment of the full resource name, which is
	// the projectId string). We call the Management API with that same ID.
	projectID := scope.ID

	info, err := fc.GetProject(ctx, projectID)
	if err != nil {
		// API not enabled, project not a Firebase project, or permission denied.
		// All are expected for many GCP projects — log at debug level and return.
		slog.Debug("gcp: firebase GetProject; project may not be a Firebase project",
			"scope", projectID, "error", err)
		return
	}

	// List apps. Failure here is non-fatal: emit the project row with zero
	// app counts rather than skipping the project entirely.
	counts, err := fc.ListApps(ctx, projectID)
	if err != nil {
		slog.Warn("gcp: firebase ListApps failed; app counts will be 0",
			"scope", projectID, "error", err)
	}

	r := buildFirebaseResource(scope.ID, projectID, info, counts)
	sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: r})
}

func buildFirebaseResource(scopeID, projectID string, info firebaseProjectInfo, counts firebaseAppCounts) inventory.Resource {
	total := counts.Web + counts.Android + counts.IOS
	detail := inventory.FirebaseDetail{
		Subtype:         "Project",
		Region:          info.LocationID,
		DisplayName:     info.DisplayName,
		ProjectNumber:   info.ProjectNumber,
		LocationID:      info.LocationID,
		WebAppCount:     counts.Web,
		AndroidAppCount: counts.Android,
		IOSAppCount:     counts.IOS,
		TotalApps:       total,
	}

	name := info.DisplayName
	if name == "" {
		name = projectID
	}

	region := info.LocationID
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindGCPFirebase, ID: projectID},
		Kind:   inventory.KindGCPFirebase,
		Name:   name,
		Region: region,
		Status: "ACTIVE",
		Detail: &detail,
	}
}
