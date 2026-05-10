package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	asset "cloud.google.com/go/asset/apiv1"
	"cloud.google.com/go/asset/apiv1/assetpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cloudcmder.com/internal/inventory"
)

// enrichConcurrency caps how many per-kind enrichers run in parallel during
// Phase 2 of a scan. Architecture.md §"Concurrency model" line 444–456
// specifies up to 4. A const (rather than a CLI flag) keeps the surface
// minimal for v1.0; promote to a flag if quota tuning becomes a real ask.
const enrichConcurrency = 4

// assetTypeToKind maps GCP asset type strings to cloudcmder Kinds, per
// architecture.md §"Discovery (asset.go)". Both Cloud Run Services and Cloud
// Functions normalize to KindFunction; GCP uses the same backend for both.
var assetTypeToKind = map[string]inventory.Kind{
	"compute.googleapis.com/Instance":         inventory.KindVM,
	"compute.googleapis.com/Disk":             inventory.KindDisk,
	"compute.googleapis.com/Network":          inventory.KindNetwork,
	"compute.googleapis.com/Subnetwork":       inventory.KindSubnet,
	"compute.googleapis.com/Firewall":         inventory.KindFirewall,
	"compute.googleapis.com/ForwardingRule":   inventory.KindLoadBalancer,
	"sqladmin.googleapis.com/Instance":        inventory.KindDatabase,
	"storage.googleapis.com/Bucket":           inventory.KindBucket,
	"container.googleapis.com/Cluster":        inventory.KindCluster,
	"run.googleapis.com/Service":              inventory.KindFunction,
	"cloudfunctions.googleapis.com/Function":  inventory.KindFunction,
	// --- Stub-only Kinds (CAI Phase 1 only; no Phase-2 enricher) ---------------
	// All entries below are routed through searchAssetPage(graceful=true).
	// Types not in CAI's searchable list silently return 0 rows.

	// Vertex AI
	"aiplatform.googleapis.com/BatchPredictionJob":           inventory.KindVertexAI,
	"aiplatform.googleapis.com/CachedContent":                inventory.KindVertexAI,
	"aiplatform.googleapis.com/CustomJob":                    inventory.KindVertexAI,
	"aiplatform.googleapis.com/Dataset":                      inventory.KindVertexAI,
	"aiplatform.googleapis.com/DeploymentResourcePool":       inventory.KindVertexAI,
	"aiplatform.googleapis.com/Endpoint":                     inventory.KindVertexAI,
	"aiplatform.googleapis.com/Featurestore":                 inventory.KindVertexAI,
	"aiplatform.googleapis.com/FeatureGroup":                 inventory.KindVertexAI,
	"aiplatform.googleapis.com/FeatureOnlineStore":           inventory.KindVertexAI,
	"aiplatform.googleapis.com/HyperparameterTuningJob":      inventory.KindVertexAI,
	"aiplatform.googleapis.com/Index":                        inventory.KindVertexAI,
	"aiplatform.googleapis.com/IndexEndpoint":                inventory.KindVertexAI,
	"aiplatform.googleapis.com/MetadataStore":                inventory.KindVertexAI,
	"aiplatform.googleapis.com/Model":                        inventory.KindVertexAI,
	"aiplatform.googleapis.com/ModelDeploymentMonitoringJob": inventory.KindVertexAI,
	"aiplatform.googleapis.com/NotebookRuntime":              inventory.KindVertexAI,
	"aiplatform.googleapis.com/NotebookRuntimeTemplate":      inventory.KindVertexAI,
	"aiplatform.googleapis.com/PipelineJob":                  inventory.KindVertexAI,
	"aiplatform.googleapis.com/ReasoningEngine":              inventory.KindVertexAI,
	"aiplatform.googleapis.com/Schedule":                     inventory.KindVertexAI,
	"aiplatform.googleapis.com/SpecialistPool":               inventory.KindVertexAI,
	"aiplatform.googleapis.com/Tensorboard":                  inventory.KindVertexAI,
	"aiplatform.googleapis.com/TrainingPipeline":             inventory.KindVertexAI,
	"aiplatform.googleapis.com/TuningJob":                    inventory.KindVertexAI,

	// Apigee
	"apigee.googleapis.com/ApiProxy":         inventory.KindApigee,
	"apigee.googleapis.com/ApiProxyRevision": inventory.KindApigee,
	"apigee.googleapis.com/Environment":      inventory.KindApigee,
	"apigee.googleapis.com/Instance":         inventory.KindApigee,
	"apigee.googleapis.com/Organization":     inventory.KindApigee,

	// Firebase
	"firebase.googleapis.com/FirebaseAppInfo": inventory.KindFirebase,
	"firebase.googleapis.com/FirebaseProject": inventory.KindFirebase,

	// App Engine
	"appengine.googleapis.com/Application": inventory.KindAppEngine,
	"appengine.googleapis.com/Service":     inventory.KindAppEngine,
	"appengine.googleapis.com/Version":     inventory.KindAppEngine,

	// BigQuery
	"bigquery.googleapis.com/Dataset":         inventory.KindBigQuery,
	"bigquery.googleapis.com/Model":           inventory.KindBigQuery,
	"bigquery.googleapis.com/Routine":         inventory.KindBigQuery,
	"bigquery.googleapis.com/RowAccessPolicy": inventory.KindBigQuery,
	"bigquery.googleapis.com/Table":           inventory.KindBigQuery,

	// Cloud DNS
	"dns.googleapis.com/ManagedZone":    inventory.KindDNS,
	"dns.googleapis.com/Policy":         inventory.KindDNS,
	"dns.googleapis.com/ResponsePolicy": inventory.KindDNS,

	// Memorystore
	"memcache.googleapis.com/Instance": inventory.KindMemorystore,
	"redis.googleapis.com/Cluster":     inventory.KindMemorystore,
	"redis.googleapis.com/Instance":    inventory.KindMemorystore,

	// Artifact Registry
	"artifactregistry.googleapis.com/DockerImage": inventory.KindArtifactRegistry,
	"artifactregistry.googleapis.com/Repository":  inventory.KindArtifactRegistry,

	// Cloud Scheduler
	"cloudscheduler.googleapis.com/Job": inventory.KindCloudScheduler,

	// Pub/Sub
	"pubsub.googleapis.com/Schema":       inventory.KindPubSub,
	"pubsub.googleapis.com/Snapshot":     inventory.KindPubSub,
	"pubsub.googleapis.com/Subscription": inventory.KindPubSub,
	"pubsub.googleapis.com/Topic":        inventory.KindPubSub,

	// Spanner
	"spanner.googleapis.com/Backup":   inventory.KindSpanner,
	"spanner.googleapis.com/Database": inventory.KindSpanner,
	"spanner.googleapis.com/Instance": inventory.KindSpanner,

	// Bigtable
	"bigtableadmin.googleapis.com/Backup":   inventory.KindBigtable,
	"bigtableadmin.googleapis.com/Cluster":  inventory.KindBigtable,
	"bigtableadmin.googleapis.com/Instance": inventory.KindBigtable,
	"bigtableadmin.googleapis.com/Table":    inventory.KindBigtable,

	// Cloud KMS
	"cloudkms.googleapis.com/CryptoKey": inventory.KindKMS,
	"cloudkms.googleapis.com/KeyRing":   inventory.KindKMS,

	// Secret Manager
	"secretmanager.googleapis.com/Secret": inventory.KindSecretManager,

	// Dataflow
	"dataflow.googleapis.com/Job": inventory.KindDataflow,

	// Dataproc
	"dataproc.googleapis.com/Cluster": inventory.KindDataproc,
	"dataproc.googleapis.com/Job":     inventory.KindDataproc,

	// Cloud Composer
	"composer.googleapis.com/Environment": inventory.KindComposer,

	// Cloud Tasks
	"cloudtasks.googleapis.com/Queue": inventory.KindCloudTasks,

	// Cloud Monitoring
	"monitoring.googleapis.com/AlertPolicy":         inventory.KindMonitoring,
	"monitoring.googleapis.com/NotificationChannel": inventory.KindMonitoring,
	"monitoring.googleapis.com/Snooze":              inventory.KindMonitoring,

	// Cloud Logging
	"logging.googleapis.com/LogBucket": inventory.KindLogging,
	"logging.googleapis.com/LogMetric": inventory.KindLogging,
	"logging.googleapis.com/LogSink":   inventory.KindLogging,

	// OS Config (VM Manager)
	"osconfig.googleapis.com/OSPolicyAssignment": inventory.KindOSConfig,
	"osconfig.googleapis.com/PatchDeployment":    inventory.KindOSConfig,

	// Cloud VPN (compute sub-resources)
	"compute.googleapis.com/ExternalVpnGateway": inventory.KindVPN,
	"compute.googleapis.com/VpnGateway":         inventory.KindVPN,
	"compute.googleapis.com/VpnTunnel":          inventory.KindVPN,

	// Cloud Router (compute sub-resource)
	"compute.googleapis.com/Router": inventory.KindRouter,

	// Cloud Build
	"cloudbuild.googleapis.com/Build":        inventory.KindCloudBuild,
	"cloudbuild.googleapis.com/BuildTrigger": inventory.KindCloudBuild,
}

// assetTypesForKinds returns the asset.googleapis.com filter strings for the
// requested Kinds. An empty kinds slice means "all known kinds".
func assetTypesForKinds(kinds []inventory.Kind) []string {
	if len(kinds) == 0 {
		out := make([]string, 0, len(assetTypeToKind))
		for at := range assetTypeToKind {
			out = append(out, at)
		}
		return out
	}
	want := make(map[inventory.Kind]struct{}, len(kinds))
	for _, k := range kinds {
		want[k] = struct{}{}
	}
	out := make([]string, 0)
	for at, k := range assetTypeToKind {
		if _, ok := want[k]; ok {
			out = append(out, at)
		}
	}
	return out
}

// assetClientFactory lets tests substitute a fake constructor without needing
// gRPC plumbing. Production paths leave it nil; the receiver lazily creates a
// real client via cloud.google.com/go/asset/apiv1.NewClient.
type assetClientFactory func(ctx context.Context, opts ...option.ClientOption) (assetSearcher, error)

// assetSearcher is the subset of *asset.Client we actually use. Defining it
// as an interface keeps ListResources testable without touching real gRPC.
type assetSearcher interface {
	SearchAllResources(ctx context.Context, req *assetpb.SearchAllResourcesRequest, opts ...gaxCallOption) resourceIterator
	Close() error
}

// resourceIterator is the iterator subset returned by SearchAllResources;
// matches both the real *asset.ResourceSearchResultIterator and a fake.
type resourceIterator interface {
	Next() (*assetpb.ResourceSearchResult, error)
}

// gaxCallOption is a marker alias so the assetSearcher interface signature
// stays type-clean across gax versions; we never pass any in M2.
type gaxCallOption any

// realAssetClient adapts *asset.Client to the assetSearcher interface.
type realAssetClient struct {
	c *asset.Client
}

func (r *realAssetClient) SearchAllResources(ctx context.Context, req *assetpb.SearchAllResourcesRequest, _ ...gaxCallOption) resourceIterator {
	return r.c.SearchAllResources(ctx, req)
}

func (r *realAssetClient) Close() error { return r.c.Close() }

// ListResources streams resources for the scope. Phase 1 emits stubs from
// Cloud Asset Inventory for every supported kind. Phase 2 then enriches VMs
// inline via the Compute API — emitting full VMDetail rows that overwrite the
// stubs (INSERT OR REPLACE) and disk-side stubs carrying the VM↔Disk edges.
// The channel closes when both phases complete or ctx is cancelled.
func (p *GCPProvider) ListResources(ctx context.Context, scope inventory.Scope, kinds []inventory.Kind) (<-chan inventory.ResourceOrErr, error) {
	if scope.ID == "" {
		return nil, errors.New("gcp: ListResources: scope.ID is required")
	}
	cli, err := p.assetClient(ctx)
	if err != nil {
		return nil, err
	}

	ch := make(chan inventory.ResourceOrErr, 64)
	go func() {
		defer close(ch)
		streamAssetStubs(ctx, cli, scope, kinds, ch)
		runEnrichers(ctx, p, scope, kinds, ch)
	}()
	return ch, nil
}

// kindEnricher pairs a Kind with the function that emits enriched Resources
// for it. The enrichers run sequentially in order; concurrent fan-out is M8
// polish per architecture.md.
type kindEnricher struct {
	kind inventory.Kind
	fn   func(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr)
}

// allEnrichers is the registered list of Phase 2 enrichers. M6 commits add to
// this slice; nothing else should need to change to wire a new kind.
var allEnrichers = []kindEnricher{
	{inventory.KindVM, enrichVMs},
	{inventory.KindDisk, enrichDisks},
	{inventory.KindNetwork, enrichNetworks},
	{inventory.KindSubnet, enrichSubnets},
	{inventory.KindFirewall, enrichFirewalls},
	{inventory.KindLoadBalancer, enrichLoadBalancers},
	{inventory.KindDatabase, enrichDatabases},
	{inventory.KindCluster, enrichClusters},
	{inventory.KindBucket, enrichBuckets},
	{inventory.KindFunction, enrichFunctions},
}

// runEnrichers is the production entry point — fans Phase 2 across the
// global allEnrichers slice with the architecture-mandated cap.
func runEnrichers(ctx context.Context, p *GCPProvider, scope inventory.Scope, kinds []inventory.Kind, ch chan<- inventory.ResourceOrErr) {
	runEnrichersWith(ctx, p, scope, kinds, ch, allEnrichers)
}

// runEnrichersWith fans the per-kind enrichment phase out across up to
// enrichConcurrency goroutines. The semaphore (`sem`) bounds in-flight work;
// the WaitGroup ensures every spawned goroutine completes before this call
// returns, so the outer ListResources goroutine's `defer close(ch)` only
// fires after every enricher has stopped writing — preventing send-on-
// closed-channel panics. Exposed (lower-case but referenced by asset_test.go
// in the same package) so tests can substitute a synthetic enricher slice
// for parallelism / cancellation assertions.
func runEnrichersWith(ctx context.Context, p *GCPProvider, scope inventory.Scope, kinds []inventory.Kind, ch chan<- inventory.ResourceOrErr, enrichers []kindEnricher) {
	sem := make(chan struct{}, enrichConcurrency)
	var wg sync.WaitGroup

	for _, e := range enrichers {
		if ctx.Err() != nil {
			break
		}
		if !wantsKind(kinds, e.kind) {
			continue
		}
		// Acquire a slot before spawning so the loop itself is bounded.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break
		}
		wg.Add(1)
		go func(e kindEnricher) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			e.fn(ctx, p, scope, ch)
		}(e)
	}
	wg.Wait()
}

// streamAssetStubs runs Phase 1 — the Cloud Asset Inventory listing — and
// emits one stub Resource per supported asset type encountered.
//
// Non-stub Kinds (the 10 original IaaS types) go in a single strict request.
// Each stub-only Kind gets its own graceful request so one unsupported CAI
// type never silences other Kinds — an InvalidArgument on that Kind's request
// is logged and skipped rather than aborting the whole scan.
func streamAssetStubs(ctx context.Context, cli assetSearcher, scope inventory.Scope, kinds []inventory.Kind, ch chan<- inventory.ResourceOrErr) {
	all := assetTypesForKinds(kinds)
	var strict []string
	stubByKind := map[inventory.Kind][]string{}
	for _, at := range all {
		if isStubKindAssetType(at) {
			k := assetTypeToKind[at]
			stubByKind[k] = append(stubByKind[k], at)
		} else {
			strict = append(strict, at)
		}
	}
	if len(strict) > 0 {
		searchAssetPage(ctx, cli, scope, strict, ch, false)
	}
	for _, types := range stubByKind {
		if ctx.Err() != nil {
			return
		}
		searchAssetPage(ctx, cli, scope, types, ch, true)
	}
}

// searchAssetPage issues one SearchAllResources call for the given asset types.
// When graceful is true, an InvalidArgument response (unsupported type) is
// logged and silently skipped instead of propagated as a scan error.
func searchAssetPage(ctx context.Context, cli assetSearcher, scope inventory.Scope, assetTypes []string, ch chan<- inventory.ResourceOrErr, graceful bool) {
	req := &assetpb.SearchAllResourcesRequest{
		Scope:      "projects/" + scope.ID,
		AssetTypes: assetTypes,
	}
	it := cli.SearchAllResources(ctx, req)
	for {
		res, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return
		}
		if err != nil {
			if graceful && status.Code(err) == codes.InvalidArgument {
				// One or more types not in CAI's searchable list; skip silently.
				return
			}
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{
				Err: fmt.Errorf("gcp: search-all-resources: %w", err),
			})
			return
		}
		r, ok := translateResult(scope.ID, res)
		if !ok {
			continue
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: r})
	}
}

// wantsKind reports whether the caller asked for the given Kind. An empty
// kinds slice is treated as "all supported kinds".
func wantsKind(kinds []inventory.Kind, k inventory.Kind) bool {
	if len(kinds) == 0 {
		return true
	}
	for _, want := range kinds {
		if want == k {
			return true
		}
	}
	return false
}

// translateResult maps one *assetpb.ResourceSearchResult to a stub Resource.
// Returns (zero, false) for asset types we do not recognise.
// For stub-only Kinds, Detail is pre-populated with *StubDetail so the Subtype
// label is available without a Phase-2 enricher.
func translateResult(scopeID string, res *assetpb.ResourceSearchResult) (inventory.Resource, bool) {
	kind, ok := assetTypeToKind[res.GetAssetType()]
	if !ok {
		return inventory.Resource{}, false
	}
	id := lastSegment(res.GetName())
	r := inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: kind, ID: id},
		Kind:   kind,
		Name:   nonEmpty(res.GetDisplayName(), id),
		Region: res.GetLocation(),
		Status: res.GetState(),
		Labels: res.GetLabels(),
	}
	if sd := stubDetailForKind(kind, res.GetAssetType()); sd != nil {
		sd.Region = res.GetLocation()
		r.Detail = sd
	}
	return r, true
}

// lastSegment returns the substring after the final slash, the GCP
// "full resource name" convention (e.g. //compute.../projects/p/zones/z/instances/foo → foo).
func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// --- asset client lifecycle -------------------------------------------------

// assetClient returns the cached asset searcher, building one on first use.
// We construct lazily because most invocations (e.g. --list-scopes) never
// touch Asset Inventory and therefore should not pay its setup cost.
func (p *GCPProvider) assetClient(ctx context.Context) (assetSearcher, error) {
	p.assetOnce.Do(func() {
		if p.assetFactory != nil {
			p.assetCli, p.assetErr = p.assetFactory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.assetErr = fmt.Errorf("gcp: resolve ADC for asset client: %w", err)
			return
		}
		c, err := asset.NewClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.assetErr = fmt.Errorf("gcp: new asset client: %w", err)
			return
		}
		p.assetCli = &realAssetClient{c: c}
	})
	if p.assetErr != nil {
		return nil, p.assetErr
	}
	return p.assetCli, nil
}

// closeAssetClient releases the asset client if it was created. Called from
// (*GCPProvider).Close.
func (p *GCPProvider) closeAssetClient() error {
	if p.assetCli == nil {
		return nil
	}
	return p.assetCli.Close()
}

// assetState bundles the receiver fields used by assetClient. Embedded into
// GCPProvider via a struct literal — keeps provider.go's struct lean while
// the asset wiring lives next to its consumers.
type assetState struct {
	assetOnce    sync.Once
	assetCli     assetSearcher
	assetErr     error
	assetFactory assetClientFactory
}
