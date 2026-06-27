package gcp

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

func TestBuildBucketResourcePublicAccessTruthTable(t *testing.T) {
	cases := []struct {
		name      string
		pap       storage.PublicAccessPrevention
		publicIAM bool
		iamKnown  bool
		want      bool
		wantState string
	}{
		{name: "enforced + IAM allUsers → still not public", pap: storage.PublicAccessPreventionEnforced, publicIAM: true, iamKnown: true, want: false, wantState: "not_public"},
		{name: "enforced + private IAM → not public", pap: storage.PublicAccessPreventionEnforced, publicIAM: false, iamKnown: true, want: false, wantState: "not_public"},
		{name: "inherited + IAM allUsers → public", pap: storage.PublicAccessPreventionInherited, publicIAM: true, iamKnown: true, want: true, wantState: "public"},
		{name: "inherited + private IAM → not public (the M6 bug case)", pap: storage.PublicAccessPreventionInherited, publicIAM: false, iamKnown: true, want: false, wantState: "not_public"},
		{name: "unknown + private IAM → not public", pap: storage.PublicAccessPreventionUnknown, publicIAM: false, iamKnown: true, want: false, wantState: "not_public"},
		{name: "IAM unreadable → public access unknown", pap: storage.PublicAccessPreventionInherited, publicIAM: false, iamKnown: false, want: false, wantState: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &storage.BucketAttrs{
				Name:                   "b",
				Location:               "US",
				PublicAccessPrevention: tc.pap,
			}
			r := buildBucketResource("p1", b, tc.publicIAM, tc.iamKnown, bucketMetrics{}, false)
			d := r.Detail.(*inventory.BucketDetail)
			if d.PublicAccess != tc.want {
				t.Errorf("PublicAccess = %v, want %v", d.PublicAccess, tc.want)
			}
			if d.PublicAccessState != tc.wantState {
				t.Errorf("PublicAccessState = %q, want %q", d.PublicAccessState, tc.wantState)
			}
		})
	}
}

func TestBuildBucketResourceFields(t *testing.T) {
	b := &storage.BucketAttrs{
		Name:              "my-bucket",
		Location:          "US",
		StorageClass:      "STANDARD",
		VersioningEnabled: true,
	}
	r := buildBucketResource("p1", b, false, true, bucketMetrics{SizeBytes: 1024, ObjectCount: 4}, false)
	if r.Ref.String() != "gcp:p1:Bucket:my-bucket" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	d := r.Detail.(*inventory.BucketDetail)
	if d.Location != "US" || d.StorageClass != "STANDARD" || !d.Versioning {
		t.Errorf("detail = %+v", d)
	}
	if d.SizeBytes != 1024 || d.ObjectCount != 4 {
		t.Errorf("metrics not plumbed through: SizeBytes=%d ObjectCount=%d", d.SizeBytes, d.ObjectCount)
	}
}

func TestEnrichBucketsIAMErrorMarksPublicAccessUnknown(t *testing.T) {
	p := &GCPProvider{
		buckets: bucketsClientState{factory: func(context.Context, ...option.ClientOption) (bucketsAPI, error) {
			return &fakeBucketsClient{
				items: []*storage.BucketAttrs{{
					Name:                   "restricted-bucket",
					Location:               "US",
					PublicAccessPrevention: storage.PublicAccessPreventionInherited,
				}},
				publicIAMErr: map[string]error{"restricted-bucket": errors.New("permission denied")},
			}, nil
		}},
		metrics: metricsClientState{factory: func(context.Context, ...option.ClientOption) (metricsAPI, error) {
			return &fakeMetricsClient{}, nil
		}},
	}
	ch := make(chan inventory.ResourceOrErr, 1)
	enrichBuckets(context.Background(), p, inventory.Scope{ID: "p1"}, ch)

	got := <-ch
	if got.Err != nil {
		t.Fatalf("unexpected error: %v", got.Err)
	}
	d := got.Resource.Detail.(*inventory.BucketDetail)
	if d.PublicAccessState != "unknown" {
		t.Fatalf("PublicAccessState = %q, want unknown", d.PublicAccessState)
	}
	if d.PublicAccess {
		t.Fatal("PublicAccess = true, want false when IAM state is unknown")
	}
}

// --- fake storage client ---------------------------------------------------

type fakeBucketsClient struct {
	items        []*storage.BucketAttrs
	publicIAM    map[string]bool
	publicIAMErr map[string]error
}

func (f *fakeBucketsClient) List(_ context.Context, _ string) bucketsIterator {
	return &fakeBucketsIter{c: f}
}

func (f *fakeBucketsClient) HasPublicIAM(_ context.Context, name string) (bool, error) {
	if err := f.publicIAMErr[name]; err != nil {
		return false, err
	}
	if f.publicIAM == nil {
		return false, nil
	}
	return f.publicIAM[name], nil
}

type fakeMetricsClient struct{}

func (f *fakeMetricsClient) ListBucketMetrics(context.Context, string) (map[string]bucketMetrics, error) {
	return nil, nil
}

func (f *fakeMetricsClient) Close() error { return nil }

func (f *fakeBucketsClient) Close() error { return nil }

type fakeBucketsIter struct {
	c   *fakeBucketsClient
	idx int
}

func (it *fakeBucketsIter) Next() (*storage.BucketAttrs, error) {
	if it.idx >= len(it.c.items) {
		return nil, iterator.Done
	}
	b := it.c.items[it.idx]
	it.idx++
	return b, nil
}
