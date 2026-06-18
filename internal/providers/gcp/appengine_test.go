package gcp

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cloudcmder.com/internal/inventory"
)

// --- fake appEngineAPI client -----------------------------------------------

type fakeAppEngineClient struct {
	app      *appEngineInfo
	appErr   error
	svcCount int
	svcErr   error
}

func (f *fakeAppEngineClient) GetApplication(_ context.Context, _ string) (*appEngineInfo, error) {
	if f.appErr != nil {
		return nil, f.appErr
	}
	if f.app == nil {
		// Default: no App Engine application in this project.
		return nil, status.Error(codes.NotFound, "app engine application not found")
	}
	return f.app, nil
}

func (f *fakeAppEngineClient) CountServices(_ context.Context, _ string) (int, error) {
	return f.svcCount, f.svcErr
}

func (f *fakeAppEngineClient) Close() error { return nil }

// --- build function tests ---------------------------------------------------

func TestBuildAppEngineResourceFields(t *testing.T) {
	app := &appEngineInfo{
		ID:              "my-project",
		LocationID:      "us-central",
		DefaultHostname: "my-project.appspot.com",
		AuthDomain:      "gmail.com",
		ServingStatus:   "SERVING",
		DatabaseType:    "CLOUD_FIRESTORE",
	}
	r := buildAppEngineResource("my-project", app, 3, false)

	if r.Ref.String() != "gcp:my-project:AppEngine:my-project" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	if r.Kind != inventory.KindGCPAppEngine {
		t.Errorf("kind = %v, want AppEngine", r.Kind)
	}
	if r.Status != "SERVING" {
		t.Errorf("status = %q, want SERVING", r.Status)
	}
	if r.Region != "us-central" {
		t.Errorf("region = %q, want us-central", r.Region)
	}

	d, ok := r.Detail.(*inventory.AppEngineDetail)
	if !ok || d == nil {
		t.Fatalf("Detail is not *AppEngineDetail: %T", r.Detail)
	}
	if d.Subtype != "Application" {
		t.Errorf("Subtype = %q, want Application", d.Subtype)
	}
	if d.LocationID != "us-central" {
		t.Errorf("LocationID = %q, want us-central", d.LocationID)
	}
	if d.DefaultHostname != "my-project.appspot.com" {
		t.Errorf("DefaultHostname = %q, want my-project.appspot.com", d.DefaultHostname)
	}
	if d.AuthDomain != "gmail.com" {
		t.Errorf("AuthDomain = %q, want gmail.com", d.AuthDomain)
	}
	if d.DatabaseType != "CLOUD_FIRESTORE" {
		t.Errorf("DatabaseType = %q, want CLOUD_FIRESTORE", d.DatabaseType)
	}
	if d.ServiceCount != 3 {
		t.Errorf("ServiceCount = %d, want 3", d.ServiceCount)
	}
}

func TestBuildAppEngineResourceFallbackID(t *testing.T) {
	// When app.ID is empty (unusual but possible), fall back to scopeID.
	app := &appEngineInfo{ID: "", LocationID: "europe-west1"}
	r := buildAppEngineResource("fallback-proj", app, 0, false)
	if r.Ref.ID != "fallback-proj" {
		t.Errorf("Ref.ID = %q, want fallback-proj", r.Ref.ID)
	}
}

// --- enricher unit tests ---------------------------------------------------

func TestEnrichAppEngineEmitsResource(t *testing.T) {
	fake := &fakeAppEngineClient{
		app: &appEngineInfo{
			ID:              "proj1",
			LocationID:      "us-central",
			DefaultHostname: "proj1.appspot.com",
			ServingStatus:   "SERVING",
		},
		svcCount: 2,
	}
	p := newProviderWithFakeAsset(t, &fakeAssetClient{})
	p.appEngine.factory = func(_ context.Context, _ ...option.ClientOption) (appEngineAPI, error) {
		return fake, nil
	}

	ch := make(chan inventory.ResourceOrErr, 8)
	enrichAppEngine(context.Background(), p, inventory.Scope{ID: "proj1"}, ch)
	close(ch)

	var resources []inventory.Resource
	for x := range ch {
		if x.Err != nil {
			t.Errorf("unexpected error: %v", x.Err)
		}
		resources = append(resources, x.Resource)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}
	d, ok := resources[0].Detail.(*inventory.AppEngineDetail)
	if !ok {
		t.Fatalf("Detail type = %T, want *AppEngineDetail", resources[0].Detail)
	}
	if d.ServiceCount != 2 {
		t.Errorf("ServiceCount = %d, want 2", d.ServiceCount)
	}
}

func TestEnrichAppEngineNotFoundSilentlySkips(t *testing.T) {
	// A project without App Engine returns NotFound — should emit no rows and no errors.
	fake := &fakeAppEngineClient{
		appErr: status.Error(codes.NotFound, "no app engine application"),
	}
	p := newProviderWithFakeAsset(t, &fakeAssetClient{})
	p.appEngine.factory = func(_ context.Context, _ ...option.ClientOption) (appEngineAPI, error) {
		return fake, nil
	}

	ch := make(chan inventory.ResourceOrErr, 8)
	enrichAppEngine(context.Background(), p, inventory.Scope{ID: "proj-no-ae"}, ch)
	close(ch)

	for x := range ch {
		if x.Err != nil {
			t.Errorf("expected no errors for NotFound, got: %v", x.Err)
		} else {
			t.Errorf("expected no resources for NotFound, got: %v", x.Resource.Name)
		}
	}
}

func TestEnrichAppEngineAPIErrorPropagated(t *testing.T) {
	boom := errors.New("simulated 500")
	fake := &fakeAppEngineClient{appErr: boom}
	p := newProviderWithFakeAsset(t, &fakeAssetClient{})
	p.appEngine.factory = func(_ context.Context, _ ...option.ClientOption) (appEngineAPI, error) {
		return fake, nil
	}

	ch := make(chan inventory.ResourceOrErr, 8)
	enrichAppEngine(context.Background(), p, inventory.Scope{ID: "proj-err"}, ch)
	close(ch)

	var sawErr bool
	for x := range ch {
		if x.Err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Errorf("expected an error to be propagated")
	}
}

func TestEnrichAppEngineServiceCountErrorIsNonFatal(t *testing.T) {
	// CountServices fails → ServiceCount = 0, but resource is still emitted.
	fake := &fakeAppEngineClient{
		app: &appEngineInfo{
			ID:            "proj2",
			LocationID:    "us-east1",
			ServingStatus: "SERVING",
		},
		svcErr: errors.New("permission denied"),
	}
	p := newProviderWithFakeAsset(t, &fakeAssetClient{})
	p.appEngine.factory = func(_ context.Context, _ ...option.ClientOption) (appEngineAPI, error) {
		return fake, nil
	}

	ch := make(chan inventory.ResourceOrErr, 8)
	enrichAppEngine(context.Background(), p, inventory.Scope{ID: "proj2"}, ch)
	close(ch)

	var resources []inventory.Resource
	for x := range ch {
		if x.Err != nil {
			t.Errorf("svcErr should be non-fatal, got error: %v", x.Err)
		}
		resources = append(resources, x.Resource)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}
	d := resources[0].Detail.(*inventory.AppEngineDetail)
	if d.ServiceCount != 0 {
		t.Errorf("ServiceCount = %d, want 0 on svc error", d.ServiceCount)
	}
}
