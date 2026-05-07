package gcp

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"cloudcmder.com/internal/inventory"
)

func TestBuildBucketResourcePublicAccessTruthTable(t *testing.T) {
	cases := []struct {
		name      string
		pap       storage.PublicAccessPrevention
		publicIAM bool
		want      bool
	}{
		{name: "enforced + IAM allUsers → still not public", pap: storage.PublicAccessPreventionEnforced, publicIAM: true, want: false},
		{name: "enforced + private IAM → not public", pap: storage.PublicAccessPreventionEnforced, publicIAM: false, want: false},
		{name: "inherited + IAM allUsers → public", pap: storage.PublicAccessPreventionInherited, publicIAM: true, want: true},
		{name: "inherited + private IAM → not public (the M6 bug case)", pap: storage.PublicAccessPreventionInherited, publicIAM: false, want: false},
		{name: "unknown + private IAM → not public", pap: storage.PublicAccessPreventionUnknown, publicIAM: false, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &storage.BucketAttrs{
				Name:                   "b",
				Location:               "US",
				PublicAccessPrevention: tc.pap,
			}
			r := buildBucketResource("p1", b, tc.publicIAM, false)
			d := r.Detail.(*inventory.BucketDetail)
			if d.PublicAccess != tc.want {
				t.Errorf("PublicAccess = %v, want %v", d.PublicAccess, tc.want)
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
	r := buildBucketResource("p1", b, false, false)
	if r.Ref.String() != "gcp:p1:Bucket:my-bucket" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	d := r.Detail.(*inventory.BucketDetail)
	if d.Location != "US" || d.StorageClass != "STANDARD" || !d.Versioning {
		t.Errorf("detail = %+v", d)
	}
}

// --- fake storage client ---------------------------------------------------

type fakeBucketsClient struct {
	items     []*storage.BucketAttrs
	publicIAM map[string]bool
}

func (f *fakeBucketsClient) List(_ context.Context, _ string) bucketsIterator {
	return &fakeBucketsIter{c: f}
}

func (f *fakeBucketsClient) HasPublicIAM(_ context.Context, name string) (bool, error) {
	if f.publicIAM == nil {
		return false, nil
	}
	return f.publicIAM[name], nil
}

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
