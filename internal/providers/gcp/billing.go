package gcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	billing "cloud.google.com/go/billing/apiv1"
	"cloud.google.com/go/billing/apiv1/billingpb"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// projectBillingInfo is the provider-internal projection of a project's Cloud
// Billing association returned by the realBillingClient. Using a projection
// keeps the interface free of SDK types so tests never need an SDK import.
type projectBillingInfo struct {
	BillingAccountID   string // short ID extracted from "billingAccounts/{id}"; empty if none
	BillingAccountName string // billing account display name (best-effort)
	BillingEnabled     bool
}

// billingAPI is the client seam for the Cloud Billing API. The single method
// builds a per-call SDK client internally so --scan-all across projects works
// without global state.
type billingAPI interface {
	// GetProjectBilling returns the billing association for projectID.
	GetProjectBilling(ctx context.Context, projectID string) (projectBillingInfo, error)
	Close() error
}

// realBillingClient implements billingAPI using cloud.google.com/go/billing/apiv1.
// It creates a fresh SDK client per call, which is stateless and safe across
// concurrent goroutines — mirroring the seam pattern in firebase.go.
type realBillingClient struct {
	opts []option.ClientOption
}

func (r *realBillingClient) GetProjectBilling(ctx context.Context, projectID string) (projectBillingInfo, error) {
	c, err := billing.NewCloudBillingClient(ctx, r.opts...)
	if err != nil {
		return projectBillingInfo{}, fmt.Errorf("billing: new client: %w", err)
	}
	defer func() { _ = c.Close() }()

	pbi, err := c.GetProjectBillingInfo(ctx, &billingpb.GetProjectBillingInfoRequest{
		Name: "projects/" + projectID,
	})
	if err != nil {
		return projectBillingInfo{}, fmt.Errorf("billing: get project billing info %s: %w", projectID, err)
	}

	out := projectBillingInfo{
		BillingEnabled:   pbi.GetBillingEnabled(),
		BillingAccountID: strings.TrimPrefix(pbi.GetBillingAccountName(), "billingAccounts/"),
	}

	// Best-effort: resolve the billing account display name. Requires
	// billing.accounts.get, which many project-scoped callers lack — treat any
	// error as "name unavailable" and keep the ID.
	if name := pbi.GetBillingAccountName(); name != "" {
		if acct, aerr := c.GetBillingAccount(ctx, &billingpb.GetBillingAccountRequest{Name: name}); aerr == nil {
			out.BillingAccountName = acct.GetDisplayName()
		}
	}
	return out, nil
}

func (r *realBillingClient) Close() error { return nil }

// billingFactory is the constructor type injected during tests.
type billingFactory func(ctx context.Context, opts ...option.ClientOption) (billingAPI, error)

// billingClientState bundles the lazy-init fields for the billing client.
type billingClientState struct {
	once    sync.Once
	cli     billingAPI
	err     error
	factory billingFactory
}

// billingClient returns the cached billing API client, building one on first use.
func (p *GCPProvider) billingClient(ctx context.Context) (billingAPI, error) {
	p.billing.once.Do(func() {
		if p.billing.factory != nil {
			p.billing.cli, p.billing.err = p.billing.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.billing.err = fmt.Errorf("gcp: ADC for billing client: %w", err)
			return
		}
		p.billing.cli = &realBillingClient{opts: []option.ClientOption{option.WithCredentials(creds)}}
	})
	if p.billing.err != nil {
		return nil, p.billing.err
	}
	return p.billing.cli, nil
}

// closeBillingClient releases the billing client if it was created.
func (p *GCPProvider) closeBillingClient() error {
	if p.billing.cli == nil {
		return nil
	}
	return p.billing.cli.Close()
}

// enrichProjectBilling emits a single KindGCPProject row per scan carrying the
// project's Cloud Billing association. There is no CAI Phase-1 stub for this
// synthetic kind, so the row is a clean insert (not a stub overwrite). Any API
// error (Billing API disabled, permission denied) is logged and the enricher
// returns 0 rows so the scan continues cleanly.
func enrichProjectBilling(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	bc, err := p.billingClient(ctx)
	if err != nil {
		slog.Warn("gcp: billing client unavailable; skipping project billing enrichment",
			"scope", scope.ID, "error", err)
		return
	}

	info, err := bc.GetProjectBilling(ctx, scope.ID)
	if err != nil {
		// Billing API not enabled or permission denied — expected for many
		// projects. Log at debug and skip the project-summary row.
		slog.Debug("gcp: billing GetProjectBilling failed; skipping project row",
			"scope", scope.ID, "error", err)
		return
	}

	sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildProjectResource(scope, info)})
}

func buildProjectResource(scope inventory.Scope, info projectBillingInfo) inventory.Resource {
	detail := inventory.ProjectDetail{
		Subtype:            "Project",
		Region:             "global",
		BillingAccountID:   info.BillingAccountID,
		BillingAccountName: info.BillingAccountName,
		BillingEnabled:     info.BillingEnabled,
	}
	name := scope.DisplayName
	if name == "" {
		name = scope.ID
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scope.ID, Kind: inventory.KindGCPProject, ID: scope.ID},
		Kind:   inventory.KindGCPProject,
		Name:   name,
		Region: "global",
		Status: "ACTIVE",
		Detail: &detail,
	}
}
