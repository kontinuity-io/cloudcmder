package gcp

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// fakeBillingClient is a test double for billingAPI. The zero-value returns an
// empty projectBillingInfo with no error (a project with billing not enabled).
// Set info for a successful association, or err to simulate API unavailability.
type fakeBillingClient struct {
	info projectBillingInfo
	err  error
}

func (f *fakeBillingClient) GetProjectBilling(_ context.Context, _ string) (projectBillingInfo, error) {
	return f.info, f.err
}

func (f *fakeBillingClient) Close() error { return nil }

// TestBuildProjectResource verifies buildProjectResource maps projection fields
// into ProjectDetail and sets the synthetic project Ref correctly.
func TestBuildProjectResource(t *testing.T) {
	scope := inventory.Scope{ID: "my-proj", DisplayName: "My Project"}
	info := projectBillingInfo{
		BillingAccountID:   "01ABCD-234567-89EFGH",
		BillingAccountName: "My Billing Account",
		BillingEnabled:     true,
	}

	r := buildProjectResource(scope, info)

	if r.Kind != inventory.KindGCPProject {
		t.Errorf("Kind = %v, want %v", r.Kind, inventory.KindGCPProject)
	}
	if r.Ref.ID != "my-proj" {
		t.Errorf("Ref.ID = %q, want my-proj", r.Ref.ID)
	}
	if r.Name != "My Project" {
		t.Errorf("Name = %q, want display name", r.Name)
	}
	if r.Region != "global" {
		t.Errorf("Region = %q, want global", r.Region)
	}

	d, ok := r.Detail.(*inventory.ProjectDetail)
	if !ok || d == nil {
		t.Fatalf("Detail is not *ProjectDetail: %T", r.Detail)
	}
	if d.Subtype != "Project" {
		t.Errorf("Subtype = %q, want Project", d.Subtype)
	}
	if d.BillingAccountID != "01ABCD-234567-89EFGH" {
		t.Errorf("BillingAccountID = %q, want 01ABCD-234567-89EFGH", d.BillingAccountID)
	}
	if d.BillingAccountName != "My Billing Account" {
		t.Errorf("BillingAccountName = %q, want My Billing Account", d.BillingAccountName)
	}
	if !d.BillingEnabled {
		t.Errorf("BillingEnabled = false, want true")
	}
}

// TestBuildProjectResourceNameFallback verifies the resource Name falls back to
// the project ID when the scope has no display name.
func TestBuildProjectResourceNameFallback(t *testing.T) {
	r := buildProjectResource(inventory.Scope{ID: "sparse-proj"}, projectBillingInfo{})
	if r.Name != "sparse-proj" {
		t.Errorf("Name = %q, want projectID fallback", r.Name)
	}
	d, ok := r.Detail.(*inventory.ProjectDetail)
	if !ok || d == nil {
		t.Fatalf("Detail not *ProjectDetail")
	}
	if d.BillingEnabled {
		t.Errorf("BillingEnabled = true, want false for zero-value info")
	}
}

func newBillingTestProvider(t *testing.T, fake billingAPI, factoryErr error) *GCPProvider {
	t.Helper()
	p, err := New(context.Background(),
		option.WithEndpoint("http://localhost"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	p.billing.factory = func(_ context.Context, _ ...option.ClientOption) (billingAPI, error) {
		return fake, factoryErr
	}
	return p
}

// TestEnrichProjectBillingEmitsRow verifies a successful billing lookup emits
// exactly one KindGCPProject row.
func TestEnrichProjectBillingEmitsRow(t *testing.T) {
	fake := &fakeBillingClient{info: projectBillingInfo{
		BillingAccountID: "01ABCD-234567-89EFGH",
		BillingEnabled:   true,
	}}
	p := newBillingTestProvider(t, fake, nil)

	ch := make(chan inventory.ResourceOrErr, 16)
	enrichProjectBilling(context.Background(), p, inventory.Scope{ID: "proj-a"}, ch)
	close(ch)

	var resources []inventory.Resource
	for x := range ch {
		if x.Err != nil {
			t.Errorf("unexpected error: %v", x.Err)
		} else {
			resources = append(resources, x.Resource)
		}
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}
	d, ok := resources[0].Detail.(*inventory.ProjectDetail)
	if !ok || d == nil {
		t.Fatalf("Detail not *ProjectDetail")
	}
	if d.BillingAccountID != "01ABCD-234567-89EFGH" {
		t.Errorf("BillingAccountID = %q, want 01ABCD-234567-89EFGH", d.BillingAccountID)
	}
}

// TestEnrichProjectBillingGracefulOnError verifies that a GetProjectBilling
// error (Billing API disabled, permission denied) yields 0 rows and 0 errors
// so the scan stays healthy.
func TestEnrichProjectBillingGracefulOnError(t *testing.T) {
	fake := &fakeBillingClient{err: errors.New("googleapi: Error 403: Cloud Billing API has not been used")}
	p := newBillingTestProvider(t, fake, nil)

	ch := make(chan inventory.ResourceOrErr, 16)
	enrichProjectBilling(context.Background(), p, inventory.Scope{ID: "proj-b"}, ch)
	close(ch)

	for x := range ch {
		if x.Err != nil {
			t.Errorf("unexpected error: %v", x.Err)
		} else {
			t.Errorf("unexpected resource: %+v", x.Resource)
		}
	}
}

// TestEnrichProjectBillingClientConstructionError verifies a factory failure
// yields 0 rows and no panic.
func TestEnrichProjectBillingClientConstructionError(t *testing.T) {
	p := newBillingTestProvider(t, nil, errors.New("ADC not configured"))

	ch := make(chan inventory.ResourceOrErr, 16)
	enrichProjectBilling(context.Background(), p, inventory.Scope{ID: "any-proj"}, ch)
	close(ch)

	for x := range ch {
		t.Errorf("expected empty channel; got: %+v", x)
	}
}
