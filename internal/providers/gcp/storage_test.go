package gcp

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"cloudcmder.com/internal/inventory"
)

func TestBuildBucketResource(t *testing.T) {
	b := &storage.BucketAttrs{
		Name:                   "my-bucket",
		Location:               "US",
		StorageClass:           "STANDARD",
		PublicAccessPrevention: storage.PublicAccessPreventionEnforced,
		VersioningEnabled:      true,
	}
	r := buildBucketResource("p1", b)
	if r.Ref.String() != "gcp:p1:Bucket:my-bucket" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	d := r.Detail.(*inventory.BucketDetail)
	if d.Location != "US" || d.StorageClass != "STANDARD" || !d.Versioning {
		t.Errorf("detail = %+v", d)
	}
	if d.PublicAccess {
		t.Errorf("PublicAccess = true, want false (enforced)")
	}
}

func TestBuildBucketResourceInherited(t *testing.T) {
	b := &storage.BucketAttrs{
		Name:                   "open-bucket",
		Location:               "US",
		PublicAccessPrevention: storage.PublicAccessPreventionInherited,
	}
	r := buildBucketResource("p1", b)
	d := r.Detail.(*inventory.BucketDetail)
	if !d.PublicAccess {
		t.Errorf("PublicAccess = false, want true (inherited)")
	}
}

// --- fake storage client ---------------------------------------------------

type fakeBucketsClient struct {
	items []*storage.BucketAttrs
}

func (f *fakeBucketsClient) List(_ context.Context, _ string) bucketsIterator {
	return &fakeBucketsIter{c: f}
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
