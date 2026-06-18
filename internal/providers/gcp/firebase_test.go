package gcp

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// fakeFirebaseClient is a test double for firebaseAPI.
// The zero-value simulates a project that is not a Firebase project:
// GetProject returns a "not found" error so the enricher emits 0 rows.
// Tests that want a successful response must set info and clear projErr.
type fakeFirebaseClient struct {
	info    firebaseProjectInfo
	counts  firebaseAppCounts
	projErr error
	appsErr error
	// noProjectErr disables the default not-found error for tests that want 0
	// rows without configuring an explicit projErr. When false (the zero-value),
	// GetProject returns a synthetic "not a Firebase project" error.
	// Set noProjectErr=false and leave projErr nil to get the default-fail behavior,
	// or set projErr explicitly for custom error messages.
	// Actually: we just make the zero-value emit an error via a sentinel.
	active bool // when true, GetProject returns info+nil; when false, returns notFoundErr
}

func (f *fakeFirebaseClient) GetProject(_ context.Context, _ string) (firebaseProjectInfo, error) {
	if !f.active && f.projErr == nil {
		// Zero-value: simulate "this GCP project is not a Firebase project"
		return firebaseProjectInfo{}, errors.New("googleapi: Error 404: not a Firebase project (fake)")
	}
	return f.info, f.projErr
}

func (f *fakeFirebaseClient) ListApps(_ context.Context, _ string) (firebaseAppCounts, error) {
	return f.counts, f.appsErr
}

func (f *fakeFirebaseClient) Close() error { return nil }

// TestBuildFirebaseResource verifies that buildFirebaseResource maps all
// projection fields into FirebaseDetail correctly.
func TestBuildFirebaseResource(t *testing.T) {
	info := firebaseProjectInfo{
		ProjectID:     "my-proj",
		DisplayName:   "My Firebase Project",
		ProjectNumber: 123456789,
		LocationID:    "us-central1",
	}
	counts := firebaseAppCounts{Web: 3, Android: 2, IOS: 1}

	r := buildFirebaseResource("my-proj", "my-proj", info, counts)

	if r.Kind != inventory.KindGCPFirebase {
		t.Errorf("Kind = %v, want %v", r.Kind, inventory.KindGCPFirebase)
	}
	if r.Ref.ID != "my-proj" {
		t.Errorf("Ref.ID = %q, want %q", r.Ref.ID, "my-proj")
	}
	if r.Name != "My Firebase Project" {
		t.Errorf("Name = %q, want display name", r.Name)
	}
	if r.Region != "us-central1" {
		t.Errorf("Region = %q, want us-central1", r.Region)
	}

	detail, ok := r.Detail.(*inventory.FirebaseDetail)
	if !ok || detail == nil {
		t.Fatalf("Detail is not *FirebaseDetail: %T", r.Detail)
	}
	if detail.Subtype != "Project" {
		t.Errorf("Subtype = %q, want Project", detail.Subtype)
	}
	if detail.Region != "us-central1" {
		t.Errorf("detail.Region = %q, want us-central1", detail.Region)
	}
	if detail.DisplayName != "My Firebase Project" {
		t.Errorf("DisplayName = %q, want My Firebase Project", detail.DisplayName)
	}
	if detail.ProjectNumber != 123456789 {
		t.Errorf("ProjectNumber = %d, want 123456789", detail.ProjectNumber)
	}
	if detail.LocationID != "us-central1" {
		t.Errorf("LocationID = %q, want us-central1", detail.LocationID)
	}
	if detail.WebAppCount != 3 {
		t.Errorf("WebAppCount = %d, want 3", detail.WebAppCount)
	}
	if detail.AndroidAppCount != 2 {
		t.Errorf("AndroidAppCount = %d, want 2", detail.AndroidAppCount)
	}
	if detail.IOSAppCount != 1 {
		t.Errorf("IOSAppCount = %d, want 1", detail.IOSAppCount)
	}
	if detail.TotalApps != 6 {
		t.Errorf("TotalApps = %d, want 6", detail.TotalApps)
	}
}

// TestBuildFirebaseResourceNameFallback verifies that when DisplayName is empty
// the resource Name falls back to the projectID.
func TestBuildFirebaseResourceNameFallback(t *testing.T) {
	info := firebaseProjectInfo{ProjectID: "sparse-proj"}
	r := buildFirebaseResource("sparse-proj", "sparse-proj", info, firebaseAppCounts{})
	if r.Name != "sparse-proj" {
		t.Errorf("Name = %q, want projectID fallback %q", r.Name, "sparse-proj")
	}
	d, ok := r.Detail.(*inventory.FirebaseDetail)
	if !ok || d == nil {
		t.Fatalf("Detail is not *FirebaseDetail")
	}
	if d.TotalApps != 0 {
		t.Errorf("TotalApps = %d, want 0", d.TotalApps)
	}
}

// TestEnrichFirebaseEmitsRow verifies that enrichFirebase sends exactly one
// resource row for a successful project + app listing.
func TestEnrichFirebaseEmitsRow(t *testing.T) {
	fake := &fakeFirebaseClient{
		active: true,
		info: firebaseProjectInfo{
			ProjectID:     "proj-a",
			DisplayName:   "Project A",
			ProjectNumber: 42,
			LocationID:    "europe-west1",
		},
		counts: firebaseAppCounts{Web: 1, Android: 1, IOS: 0},
	}

	p, err := New(context.Background(),
		option.WithEndpoint("http://localhost"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	p.firebase.factory = func(_ context.Context, _ ...option.ClientOption) (firebaseAPI, error) {
		return fake, nil
	}

	ch := make(chan inventory.ResourceOrErr, 16)
	enrichFirebase(context.Background(), p, inventory.Scope{ID: "proj-a"}, ch)
	close(ch)

	var resources []inventory.Resource
	var errs []error
	for x := range ch {
		if x.Err != nil {
			errs = append(errs, x.Err)
		} else {
			resources = append(resources, x.Resource)
		}
	}
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}
	r := resources[0]
	if r.Kind != inventory.KindGCPFirebase {
		t.Errorf("Kind = %v, want Firebase", r.Kind)
	}
	if r.Ref.ID != "proj-a" {
		t.Errorf("Ref.ID = %q, want proj-a", r.Ref.ID)
	}
	d, ok := r.Detail.(*inventory.FirebaseDetail)
	if !ok || d == nil {
		t.Fatalf("Detail not *FirebaseDetail")
	}
	if d.TotalApps != 2 {
		t.Errorf("TotalApps = %d, want 2", d.TotalApps)
	}
}

// TestEnrichFirebaseGracefulOnGetProjectError verifies that when GetProject
// returns an error (e.g. project is not a Firebase project, API not enabled,
// or permission denied) the enricher emits 0 rows and no error — the scan
// must remain healthy.
func TestEnrichFirebaseGracefulOnGetProjectError(t *testing.T) {
	fake := &fakeFirebaseClient{
		projErr: errors.New("googleapi: Error 403: Firebase Management API has not been enabled"),
	}

	p, err := New(context.Background(),
		option.WithEndpoint("http://localhost"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	p.firebase.factory = func(_ context.Context, _ ...option.ClientOption) (firebaseAPI, error) {
		return fake, nil
	}

	ch := make(chan inventory.ResourceOrErr, 16)
	enrichFirebase(context.Background(), p, inventory.Scope{ID: "plain-gcp-proj"}, ch)
	close(ch)

	for x := range ch {
		if x.Err != nil {
			t.Errorf("unexpected error: %v", x.Err)
		} else {
			t.Errorf("unexpected resource: %+v", x.Resource)
		}
	}
}

// TestEnrichFirebaseAppsErrorNonFatal verifies that when ListApps fails the
// enricher still emits the project row (with zero app counts).
func TestEnrichFirebaseAppsErrorNonFatal(t *testing.T) {
	fake := &fakeFirebaseClient{
		active: true,
		info: firebaseProjectInfo{
			ProjectID:   "proj-b",
			DisplayName: "Project B",
		},
		appsErr: errors.New("googleapi: Error 403: firebase.projects.apps.list permission denied"),
	}

	p, err := New(context.Background(),
		option.WithEndpoint("http://localhost"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	p.firebase.factory = func(_ context.Context, _ ...option.ClientOption) (firebaseAPI, error) {
		return fake, nil
	}

	ch := make(chan inventory.ResourceOrErr, 16)
	enrichFirebase(context.Background(), p, inventory.Scope{ID: "proj-b"}, ch)
	close(ch)

	var resources []inventory.Resource
	for x := range ch {
		if x.Err != nil {
			t.Errorf("unexpected error (should be non-fatal): %v", x.Err)
		} else {
			resources = append(resources, x.Resource)
		}
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1 (project row even without app counts)", len(resources))
	}
	d, ok := resources[0].Detail.(*inventory.FirebaseDetail)
	if !ok || d == nil {
		t.Fatalf("Detail not *FirebaseDetail")
	}
	if d.TotalApps != 0 {
		t.Errorf("TotalApps = %d, want 0 when ListApps fails", d.TotalApps)
	}
}

// TestEnrichFirebaseClientConstructionError verifies that when the factory
// itself fails, enrichFirebase returns 0 rows and 0 errors — not a panic.
func TestEnrichFirebaseClientConstructionError(t *testing.T) {
	p, err := New(context.Background(),
		option.WithEndpoint("http://localhost"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	p.firebase.factory = func(_ context.Context, _ ...option.ClientOption) (firebaseAPI, error) {
		return nil, errors.New("ADC not configured")
	}

	ch := make(chan inventory.ResourceOrErr, 16)
	enrichFirebase(context.Background(), p, inventory.Scope{ID: "any-proj"}, ch)
	close(ch)

	for x := range ch {
		t.Errorf("expected empty channel; got: %+v", x)
	}
}
