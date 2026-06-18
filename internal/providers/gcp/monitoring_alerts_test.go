package gcp

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// --- fake monitoring alerts client -----------------------------------------

type fakeMonitoringAlertsClient struct {
	policies []alertPolicyRow
	err      error
}

func (f *fakeMonitoringAlertsClient) ListAlertPolicies(_ context.Context, _ string) ([]alertPolicyRow, error) {
	return f.policies, f.err
}

func (f *fakeMonitoringAlertsClient) Close() error { return nil }

// --- tests -----------------------------------------------------------------

func TestEnrichAlertPolicies_HappyPath(t *testing.T) {
	policies := []alertPolicyRow{
		{
			Name:                     "projects/p1/alertPolicies/123456",
			Enabled:                  true,
			ConditionCount:           2,
			Combiner:                 "AND",
			NotificationChannelCount: 3,
		},
		{
			Name:                     "projects/p1/alertPolicies/789012",
			Enabled:                  false,
			ConditionCount:           1,
			Combiner:                 "OR",
			NotificationChannelCount: 0,
		},
	}

	ch := make(chan inventory.ResourceOrErr, 16)
	p := &GCPProvider{}
	p.monitoringAlerts.factory = func(_ context.Context, _ ...option.ClientOption) (monitoringAlertsAPI, error) {
		return &fakeMonitoringAlertsClient{policies: policies}, nil
	}

	enrichAlertPolicies(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	var results []inventory.Resource
	for x := range ch {
		if x.Err != nil {
			t.Fatalf("unexpected error: %v", x.Err)
		}
		results = append(results, x.Resource)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First policy
	r0 := results[0]
	if r0.Ref.ID != "123456" {
		t.Errorf("ID = %q, want 123456", r0.Ref.ID)
	}
	if r0.Ref.Kind != inventory.KindGCPMonitoring {
		t.Errorf("Kind = %v, want KindGCPMonitoring", r0.Ref.Kind)
	}
	d0, ok := r0.Detail.(*inventory.MonitoringDetail)
	if !ok {
		t.Fatalf("Detail type = %T, want *MonitoringDetail", r0.Detail)
	}
	if d0.Subtype != "AlertPolicy" {
		t.Errorf("Subtype = %q, want AlertPolicy", d0.Subtype)
	}
	if d0.Region != "global" {
		t.Errorf("Region = %q, want global", d0.Region)
	}
	if !d0.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if d0.ConditionCount != 2 {
		t.Errorf("ConditionCount = %d, want 2", d0.ConditionCount)
	}
	if d0.Combiner != "AND" {
		t.Errorf("Combiner = %q, want AND", d0.Combiner)
	}
	if d0.NotificationChannelCount != 3 {
		t.Errorf("NotificationChannelCount = %d, want 3", d0.NotificationChannelCount)
	}

	// Second policy
	r1 := results[1]
	d1, ok := r1.Detail.(*inventory.MonitoringDetail)
	if !ok {
		t.Fatalf("Detail type = %T, want *MonitoringDetail", r1.Detail)
	}
	if d1.Enabled {
		t.Errorf("Enabled = true, want false")
	}
	if d1.NotificationChannelCount != 0 {
		t.Errorf("NotificationChannelCount = %d, want 0", d1.NotificationChannelCount)
	}
}

func TestEnrichAlertPolicies_EmptyList(t *testing.T) {
	ch := make(chan inventory.ResourceOrErr, 4)
	p := &GCPProvider{}
	p.monitoringAlerts.factory = func(_ context.Context, _ ...option.ClientOption) (monitoringAlertsAPI, error) {
		return &fakeMonitoringAlertsClient{policies: nil}, nil
	}

	enrichAlertPolicies(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	for x := range ch {
		t.Errorf("expected no results, got: %+v", x)
	}
}

func TestEnrichAlertPolicies_ClientError(t *testing.T) {
	wantErr := errors.New("permission denied")
	ch := make(chan inventory.ResourceOrErr, 4)
	p := &GCPProvider{}
	p.monitoringAlerts.factory = func(_ context.Context, _ ...option.ClientOption) (monitoringAlertsAPI, error) {
		return &fakeMonitoringAlertsClient{err: wantErr}, nil
	}

	enrichAlertPolicies(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	var sawErr bool
	for x := range ch {
		if x.Err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Errorf("expected error to be propagated")
	}
}

func TestEnrichAlertPolicies_FactoryError(t *testing.T) {
	ch := make(chan inventory.ResourceOrErr, 4)
	p := &GCPProvider{}
	p.monitoringAlerts.factory = func(_ context.Context, _ ...option.ClientOption) (monitoringAlertsAPI, error) {
		return nil, errors.New("factory failed")
	}

	enrichAlertPolicies(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	var sawErr bool
	for x := range ch {
		if x.Err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Errorf("expected error when factory fails")
	}
}
