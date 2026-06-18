package gcp

import (
	"context"
	"errors"
	"fmt"

	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// providerName is the short name returned by Provider.Name and embedded in
// every ResourceRef this package emits.
const providerName = "gcp"

// GCPProvider implements inventory.Provider for Google Cloud.
type GCPProvider struct {
	projects *resourcemanager.ProjectsClient
	assetState
	computeState
	serviceUsageState

	// M6 lazy clients — one named field per kind family (no embedding to
	// avoid name collisions across the per-client (once, cli, err, factory)
	// quartets).
	disks            disksClientState
	networks         networksClientState
	subnets          subnetsClientState
	firewalls        firewallsClientState
	gfwd             globalForwardingRulesClientState
	rfwd             forwardingRulesClientState
	sql              sqlClientState
	gke              gkeClientState
	buckets          bucketsClientState
	metrics          metricsClientState
	runsvc           runClientState
	funcs            functionsClientState
	bq               bigQueryClientState
	pubsub           pubsubClientState
	memorystore      memorystoreClientState
	artifactRegistry artifactRegistryClientState
	secretManager    secretManagerClientState
	appEngine        appEngineClientState
	firebase         firebaseClientState
	logging          loggingClientState
	monitoringAlerts monitoringAlertsClientState

	// dumpNative controls whether raw provider API payloads are stored in
	// resources.native_json. Off by default — roughly doubles DB size.
	dumpNative bool
}

// SetDumpNative enables or disables raw native payload capture.
// Must be called before the first ListResources call.
func (p *GCPProvider) SetDumpNative(v bool) { p.dumpNative = v }

// New constructs a GCPProvider using Application Default Credentials.
// Pass option.ClientOption values to override the endpoint or auth — used by
// tests to point at an httptest server.
func New(ctx context.Context, opts ...option.ClientOption) (*GCPProvider, error) {
	if len(opts) == 0 {
		creds, err := NewCredentials(ctx)
		if err != nil {
			return nil, fmt.Errorf("gcp: resolve ADC: %w", err)
		}
		opts = []option.ClientOption{option.WithCredentials(creds)}
	}
	pc, err := resourcemanager.NewProjectsRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("gcp: new projects client: %w", err)
	}
	return &GCPProvider{projects: pc}, nil
}

// Close releases the underlying clients.
func (p *GCPProvider) Close() error {
	var errs []error
	if p.projects != nil {
		if err := p.projects.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := p.closeAssetClient(); err != nil {
		errs = append(errs, err)
	}
	if err := p.closeInstancesClient(); err != nil {
		errs = append(errs, err)
	}
	if err := p.closeMachineTypesClient(); err != nil {
		errs = append(errs, err)
	}
	for _, closer := range []func() error{
		p.closeDisksClient,
		p.closeNetworksClient,
		p.closeSubnetsClient,
		p.closeFirewallsClient,
		p.closeGlobalForwardingRulesClient,
		p.closeForwardingRulesClient,
		p.closeSQLClient,
		p.closeGKEClient,
		p.closeBucketsClient,
		p.closeMetricsClient,
		p.closeRunClient,
		p.closeFunctionsClient,
		p.closeBigQueryClient,
		p.closePubSubClient,
		p.closeMemorystoreClient,
		p.closeArtifactRegistryClient,
		p.closeSecretManagerClient,
		p.closeAppEngineClient,
		p.closeFirebaseClient,
		p.closeLoggingClient,
		p.closeMonitoringAlertsClient,
	} {
		if err := closer(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Name returns the provider's short name.
func (p *GCPProvider) Name() string { return providerName }

// GetDetail is reserved for on-demand re-enrichment (post-M5 polish). Today
// every scan writes the full Detail inline, so the TUI never needs to call
// back into the provider — it reads from the store.
func (p *GCPProvider) GetDetail(ctx context.Context, _ inventory.ResourceRef) (inventory.Resource, error) {
	return inventory.Resource{}, errors.New("gcp: GetDetail not implemented; re-run --scan instead")
}

// Telemetry returns nil in v1; v1.1 adds Cloud Monitoring overlays.
func (p *GCPProvider) Telemetry() inventory.Telemetry { return nil }
