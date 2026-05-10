package gcp

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/asset/apiv1/assetpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

func TestAssetTypeToKindCovers34Kinds(t *testing.T) {
	// 11 non-stub + 24 VertexAI + 57 new stub types = 92 total.
	// Unique kinds: 10 original + 1 VertexAI + 23 new stub = 34.
	if len(assetTypeToKind) != 92 {
		t.Errorf("len(assetTypeToKind) = %d, want 92", len(assetTypeToKind))
	}

	uniqueKinds := map[inventory.Kind]struct{}{}
	for _, k := range assetTypeToKind {
		uniqueKinds[k] = struct{}{}
	}
	if len(uniqueKinds) != 34 {
		t.Errorf("unique kinds = %d, want 34", len(uniqueKinds))
	}
}

func TestTranslateResultPopulatesStubDetail(t *testing.T) {
	cases := []struct {
		name      string
		assetType string
		wantKind  inventory.Kind
		wantSub   string
	}{
		{"VertexAI Endpoint", "aiplatform.googleapis.com/Endpoint", inventory.KindVertexAI, "Endpoint"},
		{"Apigee Organization", "apigee.googleapis.com/Organization", inventory.KindApigee, "Organization"},
		{"BigQuery Dataset", "bigquery.googleapis.com/Dataset", inventory.KindBigQuery, "Dataset"},
		{"PubSub Topic", "pubsub.googleapis.com/Topic", inventory.KindPubSub, "Topic"},
		{"KMS CryptoKey", "cloudkms.googleapis.com/CryptoKey", inventory.KindKMS, "CryptoKey"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := &assetpb.ResourceSearchResult{
				Name:        "//x.googleapis.com/projects/p1/locations/us-central1/resources/r1",
				AssetType:   tc.assetType,
				DisplayName: "r1",
				Location:    "us-central1",
				State:       "ACTIVE",
			}
			r, ok := translateResult("p1", res)
			if !ok {
				t.Fatalf("translateResult ok = false for %s", tc.assetType)
			}
			if r.Kind != tc.wantKind {
				t.Errorf("kind = %v, want %v", r.Kind, tc.wantKind)
			}
			sd, ok := r.Detail.(*inventory.StubDetail)
			if !ok || sd == nil {
				t.Fatalf("Detail is not *StubDetail: %T", r.Detail)
			}
			if sd.Subtype != tc.wantSub {
				t.Errorf("Subtype = %q, want %q", sd.Subtype, tc.wantSub)
			}
			if sd.Region != "us-central1" {
				t.Errorf("Region = %q, want %q", sd.Region, "us-central1")
			}
		})
	}
}

func TestAssetTypesForKinds(t *testing.T) {
	all := assetTypesForKinds(nil)
	if len(all) != len(assetTypeToKind) {
		t.Errorf("nil kinds → %d types, want %d", len(all), len(assetTypeToKind))
	}

	got := assetTypesForKinds([]inventory.Kind{inventory.KindFunction})
	sort.Strings(got)
	want := []string{
		"cloudfunctions.googleapis.com/Function",
		"run.googleapis.com/Service",
	}
	if len(got) != len(want) {
		t.Fatalf("Function filter → %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTranslateResultMapsFields(t *testing.T) {
	res := &assetpb.ResourceSearchResult{
		Name:        "//compute.googleapis.com/projects/p1/zones/us-central1-a/instances/vm-a",
		AssetType:   "compute.googleapis.com/Instance",
		DisplayName: "vm-a",
		Location:    "us-central1-a",
		State:       "RUNNING",
		Labels:      map[string]string{"env": "prod"},
	}
	r, ok := translateResult("p1", res)
	if !ok {
		t.Fatalf("translateResult ok = false, want true")
	}
	if r.Kind != inventory.KindVM {
		t.Errorf("kind = %v, want VM", r.Kind)
	}
	if r.Ref.ID != "vm-a" || r.Ref.ScopeID != "p1" || r.Ref.Provider != "gcp" {
		t.Errorf("ref = %+v", r.Ref)
	}
	if r.Region != "us-central1-a" || r.Status != "RUNNING" {
		t.Errorf("region/status mismatch: %+v", r)
	}
	if r.Labels["env"] != "prod" {
		t.Errorf("labels lost: %v", r.Labels)
	}
}

func TestTranslateResultUnknownAssetType(t *testing.T) {
	_, ok := translateResult("p1", &assetpb.ResourceSearchResult{
		Name:      "//something.else/foo",
		AssetType: "something.else/Resource",
	})
	if ok {
		t.Errorf("unknown asset type should map to ok=false")
	}
}

func TestTranslateResultDisplayNameFallback(t *testing.T) {
	r, ok := translateResult("p1", &assetpb.ResourceSearchResult{
		Name:      "//storage.googleapis.com/projects/_/buckets/no-display",
		AssetType: "storage.googleapis.com/Bucket",
	})
	if !ok {
		t.Fatalf("expected mapping ok")
	}
	if r.Name != "no-display" {
		t.Errorf("name = %q, want %q (last-segment fallback)", r.Name, "no-display")
	}
}

func TestListResourcesStreamsAndFiltersUnknown(t *testing.T) {
	pages := [][]*assetpb.ResourceSearchResult{
		{
			{Name: "//.../instances/vm1", AssetType: "compute.googleapis.com/Instance", DisplayName: "vm1"},
			{Name: "//.../buckets/b1", AssetType: "storage.googleapis.com/Bucket", DisplayName: "b1"},
			{Name: "//.../mystery", AssetType: "skipped.googleapis.com/Thing"},
		},
		{
			{Name: "//.../instances/vm2", AssetType: "compute.googleapis.com/Instance", DisplayName: "vm2"},
		},
	}
	p := newProviderWithFakeAsset(t, &fakeAssetClient{pages: pages})

	ctx := context.Background()
	ch, err := p.ListResources(ctx, inventory.Scope{ID: "p1"}, nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	var names []string
	for x := range ch {
		if x.Err != nil {
			t.Fatalf("stream error: %v", x.Err)
		}
		names = append(names, x.Resource.Name)
	}
	sort.Strings(names)
	want := []string{"b1", "vm1", "vm2"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestListResourcesPropagatesIteratorError(t *testing.T) {
	p := newProviderWithFakeAsset(t, &fakeAssetClient{
		pages: [][]*assetpb.ResourceSearchResult{
			{{Name: "//.../instances/vm1", AssetType: "compute.googleapis.com/Instance", DisplayName: "vm1"}},
		},
		errAfter: errors.New("simulated 503"),
	})

	ctx := context.Background()
	ch, err := p.ListResources(ctx, inventory.Scope{ID: "p1"}, nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	var sawResource, sawErr bool
	for x := range ch {
		switch {
		case x.Err != nil:
			sawErr = true
		default:
			sawResource = true
		}
	}
	if !sawResource || !sawErr {
		t.Errorf("expected one resource and one error, got resource=%v err=%v", sawResource, sawErr)
	}
}

func TestListResourcesRequiresScope(t *testing.T) {
	p, err := New(context.Background(),
		option.WithEndpoint("http://localhost"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	if _, err := p.ListResources(context.Background(), inventory.Scope{}, nil); err == nil {
		t.Errorf("expected error when scope.ID is empty")
	}
}

// --- fake asset client used by tests ---------------------------------------

type fakeAssetClient struct {
	pages    [][]*assetpb.ResourceSearchResult
	errAfter error
}

// SearchAllResources mirrors real CAI behaviour: only results whose AssetType
// appears in req.AssetTypes are returned (empty AssetTypes = all).
func (f *fakeAssetClient) SearchAllResources(_ context.Context, req *assetpb.SearchAllResourcesRequest, _ ...gaxCallOption) resourceIterator {
	want := make(map[string]bool, len(req.AssetTypes))
	for _, at := range req.AssetTypes {
		want[at] = true
	}
	var filtered [][]*assetpb.ResourceSearchResult
	for _, page := range f.pages {
		var fp []*assetpb.ResourceSearchResult
		for _, r := range page {
			if len(want) == 0 || want[r.AssetType] {
				fp = append(fp, r)
			}
		}
		if len(fp) > 0 {
			filtered = append(filtered, fp)
		}
	}
	return &fakeIter{pages: filtered, errAfter: f.errAfter}
}

func (f *fakeAssetClient) Close() error { return nil }

type fakeIter struct {
	pages    [][]*assetpb.ResourceSearchResult
	errAfter error
	page     int
	idx      int
}

func (it *fakeIter) Next() (*assetpb.ResourceSearchResult, error) {
	for it.page < len(it.pages) && it.idx >= len(it.pages[it.page]) {
		it.page++
		it.idx = 0
	}
	if it.page >= len(it.pages) {
		if it.errAfter != nil {
			err := it.errAfter
			it.errAfter = nil
			return nil, err
		}
		return nil, iterator.Done
	}
	res := it.pages[it.page][it.idx]
	it.idx++
	return res, nil
}

func TestRunEnrichersIsConcurrent(t *testing.T) {
	// Four fake enrichers, each sleeping 80ms. Sequential = 320ms,
	// concurrent (cap=4) = ~80ms. Asserting <200ms gives generous slack
	// for CI scheduler jitter while still failing if the loop went serial.
	const each = 80 * time.Millisecond
	enrichers := []kindEnricher{
		{kind: inventory.Kind("FakeA"), fn: sleeper(each)},
		{kind: inventory.Kind("FakeB"), fn: sleeper(each)},
		{kind: inventory.Kind("FakeC"), fn: sleeper(each)},
		{kind: inventory.Kind("FakeD"), fn: sleeper(each)},
	}
	ch := make(chan inventory.ResourceOrErr, 16)
	start := time.Now()
	runEnrichersWith(context.Background(), nil, inventory.Scope{ID: "p1"}, nil, ch, enrichers)
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("runEnrichers ran serially (%v); expected concurrent (<200ms)", elapsed)
	}
}

func TestRunEnrichersWithKindFilter(t *testing.T) {
	var ran int32
	enrichers := []kindEnricher{
		{kind: inventory.Kind("FakeA"), fn: counter(&ran)},
		{kind: inventory.Kind("FakeB"), fn: counter(&ran)},
	}
	ch := make(chan inventory.ResourceOrErr, 8)
	runEnrichersWith(context.Background(), nil, inventory.Scope{ID: "p1"},
		[]inventory.Kind{inventory.Kind("FakeA")}, ch, enrichers)
	if got := atomic.LoadInt32(&ran); got != 1 {
		t.Errorf("ran = %d, want 1 (kind filter should skip FakeB)", got)
	}
}

func sleeper(d time.Duration) func(context.Context, *GCPProvider, inventory.Scope, chan<- inventory.ResourceOrErr) {
	return func(_ context.Context, _ *GCPProvider, _ inventory.Scope, _ chan<- inventory.ResourceOrErr) {
		time.Sleep(d)
	}
}

func counter(c *int32) func(context.Context, *GCPProvider, inventory.Scope, chan<- inventory.ResourceOrErr) {
	return func(_ context.Context, _ *GCPProvider, _ inventory.Scope, _ chan<- inventory.ResourceOrErr) {
		atomic.AddInt32(c, 1)
	}
}

// newProviderWithFakeAsset returns a GCPProvider whose asset client is the
// supplied fake. The resourcemanager client is created against an unreachable
// endpoint — fine because tests in this file never call ListScopes. After M5
// the instances client is also stubbed with an empty fake so the new VM
// enrichment phase does not try to reach the real Compute API.
func newProviderWithFakeAsset(t *testing.T, fake assetSearcher) *GCPProvider {
	t.Helper()
	p, err := New(context.Background(),
		option.WithEndpoint("http://localhost"),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.assetFactory = func(_ context.Context, _ ...option.ClientOption) (assetSearcher, error) {
		return fake, nil
	}
	p.instancesFact = func(_ context.Context, _ ...option.ClientOption) (instancesAPI, error) {
		return &fakeInstancesClient{}, nil
	}
	// Stub every M6 compute client too so Phase 2 enrichers find no work
	// without trying to reach the real API.
	p.disks.factory = func(_ context.Context, _ ...option.ClientOption) (disksAPI, error) {
		return &fakeDisksClient{}, nil
	}
	p.networks.factory = func(_ context.Context, _ ...option.ClientOption) (networksAPI, error) {
		return &fakeNetworksClient{}, nil
	}
	p.subnets.factory = func(_ context.Context, _ ...option.ClientOption) (subnetsAPI, error) {
		return &fakeSubnetsClient{}, nil
	}
	p.firewalls.factory = func(_ context.Context, _ ...option.ClientOption) (firewallsAPI, error) {
		return &fakeFirewallsClient{}, nil
	}
	p.gfwd.factory = func(_ context.Context, _ ...option.ClientOption) (globalForwardingRulesAPI, error) {
		return &fakeGlobalForwardingRulesClient{}, nil
	}
	p.rfwd.factory = func(_ context.Context, _ ...option.ClientOption) (forwardingRulesAPI, error) {
		return &fakeForwardingRulesClient{}, nil
	}
	p.sql.factory = func(_ context.Context, _ ...option.ClientOption) (sqlAPI, error) {
		return &fakeSQLClient{}, nil
	}
	p.gke.factory = func(_ context.Context, _ ...option.ClientOption) (gkeAPI, error) {
		return &fakeGKEClient{}, nil
	}
	p.buckets.factory = func(_ context.Context, _ ...option.ClientOption) (bucketsAPI, error) {
		return &fakeBucketsClient{}, nil
	}
	p.runsvc.factory = func(_ context.Context, _ ...option.ClientOption) (runServicesAPI, error) {
		return &fakeRunClient{}, nil
	}
	p.funcs.factory = func(_ context.Context, _ ...option.ClientOption) (functionsAPI, error) {
		return &fakeFunctionsClient{}, nil
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}
