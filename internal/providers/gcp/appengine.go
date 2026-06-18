package gcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	appengine "cloud.google.com/go/appengine/apiv1"
	"cloud.google.com/go/appengine/apiv1/appenginepb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cloudcmder.com/internal/inventory"
)

// --- App Engine client seam ------------------------------------------------

// appEngineInfo is the projection struct the enricher works with.
// Using projection (rather than raw SDK types) keeps the interface narrow
// and the fake client simple.
type appEngineInfo struct {
	ID              string
	LocationID      string
	DefaultHostname string
	AuthDomain      string
	ServingStatus   string
	DatabaseType    string
}

// appEngineAPI is the seam between the enricher and the App Engine Admin API.
// Tests inject a fake; production uses realAppEngineClient.
type appEngineAPI interface {
	GetApplication(ctx context.Context, projectID string) (*appEngineInfo, error)
	CountServices(ctx context.Context, projectID string) (int, error)
	Close() error
}

// realAppEngineClient implements appEngineAPI against the live SDK.
// Clients are built per-call (inside the method) so that --scan-all across
// multiple projects reuses the same factory without sharing mutable state.
type realAppEngineClient struct {
	opts []option.ClientOption
}

func (r *realAppEngineClient) GetApplication(ctx context.Context, projectID string) (*appEngineInfo, error) {
	cli, err := appengine.NewApplicationsClient(ctx, r.opts...)
	if err != nil {
		return nil, fmt.Errorf("new appengine applications client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	app, err := cli.GetApplication(ctx, &appenginepb.GetApplicationRequest{
		Name: "apps/" + projectID,
	})
	if err != nil {
		return nil, err
	}
	return &appEngineInfo{
		ID:              app.GetId(),
		LocationID:      app.GetLocationId(),
		DefaultHostname: app.GetDefaultHostname(),
		AuthDomain:      app.GetAuthDomain(),
		ServingStatus:   app.GetServingStatus().String(),
		DatabaseType:    app.GetDatabaseType().String(),
	}, nil
}

func (r *realAppEngineClient) CountServices(ctx context.Context, projectID string) (int, error) {
	cli, err := appengine.NewServicesClient(ctx, r.opts...)
	if err != nil {
		return 0, fmt.Errorf("new appengine services client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	it := cli.ListServices(ctx, &appenginepb.ListServicesRequest{
		Parent: "apps/" + projectID,
	})
	var count int
	for {
		_, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func (r *realAppEngineClient) Close() error { return nil }

// --- client state / lazy getter --------------------------------------------

type appEngineFactory func(ctx context.Context, opts ...option.ClientOption) (appEngineAPI, error)

type appEngineClientState struct {
	once    sync.Once
	cli     appEngineAPI
	err     error
	factory appEngineFactory
}

func (p *GCPProvider) appEngineClient(ctx context.Context) (appEngineAPI, error) {
	p.appEngine.once.Do(func() {
		if p.appEngine.factory != nil {
			p.appEngine.cli, p.appEngine.err = p.appEngine.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.appEngine.err = fmt.Errorf("gcp: ADC for appengine client: %w", err)
			return
		}
		p.appEngine.cli = &realAppEngineClient{opts: []option.ClientOption{option.WithCredentials(creds)}}
	})
	if p.appEngine.err != nil {
		return nil, p.appEngine.err
	}
	return p.appEngine.cli, nil
}

func (p *GCPProvider) closeAppEngineClient() error {
	if p.appEngine.cli == nil {
		return nil
	}
	return p.appEngine.cli.Close()
}

// --- enricher --------------------------------------------------------------

// enrichAppEngine emits one AppEngineDetail row for the Application grain.
// Service and Version grains remain as CAI stubs (they have no Phase-2 enricher).
func enrichAppEngine(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	cli, err := p.appEngineClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: appengine client: %w", err)})
		return
	}

	app, err := cli.GetApplication(ctx, scope.ID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Project has no App Engine application — silently skip.
			return
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{
			Err: fmt.Errorf("gcp: get appengine application %s: %w", scope.ID, err),
		})
		return
	}

	// Count services best-effort — failure is non-fatal.
	svcCount, svcErr := cli.CountServices(ctx, scope.ID)
	if svcErr != nil {
		slog.Warn("scan: appengine service count unavailable; defaulting to 0",
			"project", scope.ID, "error", svcErr)
		svcCount = 0
	}

	sendOrCancel(ctx, ch, inventory.ResourceOrErr{
		Resource: buildAppEngineResource(scope.ID, app, svcCount, p.dumpNative),
	})
}

func buildAppEngineResource(scopeID string, app *appEngineInfo, svcCount int, _ bool) inventory.Resource {
	detail := inventory.AppEngineDetail{
		Subtype:         "Application",
		Region:          app.LocationID,
		ServingStatus:   app.ServingStatus,
		LocationID:      app.LocationID,
		DefaultHostname: app.DefaultHostname,
		AuthDomain:      app.AuthDomain,
		DatabaseType:    app.DatabaseType,
		ServiceCount:    svcCount,
	}
	// The CAI stub ID for an Application is the project ID (last segment of
	// "apps/{project}"). We use the project ID directly so INSERT OR REPLACE
	// overwrites the stub.
	id := app.ID
	if id == "" {
		id = scopeID
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindGCPAppEngine, ID: id},
		Kind:   inventory.KindGCPAppEngine,
		Name:   id,
		Region: app.LocationID,
		Status: app.ServingStatus,
		Detail: &detail,
	}
}
