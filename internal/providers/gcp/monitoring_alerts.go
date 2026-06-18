package gcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// alertPolicyRow is the provider-internal projection of a Cloud Monitoring
// AlertPolicy. Using an internal type keeps the monitoringAlertsAPI interface
// free of raw SDK types so the fake in monitoring_alerts_test.go stays SDK-free.
type alertPolicyRow struct {
	Name                     string // full resource name: projects/{p}/alertPolicies/{id}
	Enabled                  bool
	ConditionCount           int
	Combiner                 string
	NotificationChannelCount int
}

// monitoringAlertsAPI is the seam between the Monitoring enricher and the
// Cloud Monitoring AlertPolicy API.
type monitoringAlertsAPI interface {
	ListAlertPolicies(ctx context.Context, projectName string) ([]alertPolicyRow, error)
	Close() error
}

type realMonitoringAlertsClient struct {
	c *monitoring.AlertPolicyClient
}

func (r *realMonitoringAlertsClient) ListAlertPolicies(ctx context.Context, projectName string) ([]alertPolicyRow, error) {
	req := &monitoringpb.ListAlertPoliciesRequest{Name: projectName}
	it := r.c.ListAlertPolicies(ctx, req)
	var out []alertPolicyRow
	for {
		p, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		row := alertPolicyRow{
			Name:                     p.GetName(),
			Enabled:                  p.GetEnabled().GetValue(),
			ConditionCount:           len(p.GetConditions()),
			Combiner:                 p.GetCombiner().String(),
			NotificationChannelCount: len(p.GetNotificationChannels()),
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *realMonitoringAlertsClient) Close() error { return r.c.Close() }

type monitoringAlertsFactory func(ctx context.Context, opts ...option.ClientOption) (monitoringAlertsAPI, error)

type monitoringAlertsClientState struct {
	once    sync.Once
	cli     monitoringAlertsAPI
	err     error
	factory monitoringAlertsFactory
}

func (p *GCPProvider) monitoringAlertsClient(ctx context.Context) (monitoringAlertsAPI, error) {
	p.monitoringAlerts.once.Do(func() {
		if p.monitoringAlerts.factory != nil {
			p.monitoringAlerts.cli, p.monitoringAlerts.err = p.monitoringAlerts.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.monitoringAlerts.err = fmt.Errorf("gcp: ADC for monitoring alerts client: %w", err)
			return
		}
		c, err := monitoring.NewAlertPolicyClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.monitoringAlerts.err = fmt.Errorf("gcp: new monitoring alerts client: %w", err)
			return
		}
		p.monitoringAlerts.cli = &realMonitoringAlertsClient{c: c}
	})
	if p.monitoringAlerts.err != nil {
		return nil, p.monitoringAlerts.err
	}
	return p.monitoringAlerts.cli, nil
}

func (p *GCPProvider) closeMonitoringAlertsClient() error {
	if p.monitoringAlerts.cli == nil {
		return nil
	}
	return p.monitoringAlerts.cli.Close()
}

// enrichAlertPolicies is the Phase-2 enricher for KindGCPMonitoring /
// AlertPolicy grain. NotificationChannel and Snooze grains are left as stubs.
func enrichAlertPolicies(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	mac, err := p.monitoringAlertsClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: monitoring alerts client: %w", err)})
		return
	}

	policies, err := mac.ListAlertPolicies(ctx, "projects/"+scope.ID)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list alert policies: %w", err)})
		return
	}

	for _, pol := range policies {
		id := lastSegment(pol.Name)
		detail := inventory.MonitoringDetail{
			Subtype:                  "AlertPolicy",
			Region:                   "global",
			Enabled:                  pol.Enabled,
			ConditionCount:           pol.ConditionCount,
			Combiner:                 pol.Combiner,
			NotificationChannelCount: pol.NotificationChannelCount,
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{
			Resource: inventory.Resource{
				Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scope.ID, Kind: inventory.KindGCPMonitoring, ID: id},
				Kind:   inventory.KindGCPMonitoring,
				Name:   id,
				Region: "global",
				Status: "",
				Detail: &detail,
			},
		})
	}
}
