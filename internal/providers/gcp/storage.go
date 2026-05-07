package gcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

type bucketsAPI interface {
	List(ctx context.Context, projectID string) bucketsIterator
	Close() error
}

type bucketsIterator interface {
	Next() (*storage.BucketAttrs, error)
}

type realBucketsClient struct {
	c *storage.Client
}

func (r *realBucketsClient) List(ctx context.Context, projectID string) bucketsIterator {
	return r.c.Buckets(ctx, projectID)
}

func (r *realBucketsClient) Close() error { return r.c.Close() }

type bucketsFactory func(ctx context.Context, opts ...option.ClientOption) (bucketsAPI, error)

type bucketsClientState struct {
	once    sync.Once
	cli     bucketsAPI
	err     error
	factory bucketsFactory
}

func (p *GCPProvider) bucketsClient(ctx context.Context) (bucketsAPI, error) {
	p.buckets.once.Do(func() {
		if p.buckets.factory != nil {
			p.buckets.cli, p.buckets.err = p.buckets.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.buckets.err = fmt.Errorf("gcp: ADC for storage client: %w", err)
			return
		}
		c, err := storage.NewClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.buckets.err = fmt.Errorf("gcp: new storage client: %w", err)
			return
		}
		p.buckets.cli = &realBucketsClient{c: c}
	})
	if p.buckets.err != nil {
		return nil, p.buckets.err
	}
	return p.buckets.cli, nil
}

func (p *GCPProvider) closeBucketsClient() error {
	if p.buckets.cli == nil {
		return nil
	}
	return p.buckets.cli.Close()
}

func enrichBuckets(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	bc, err := p.bucketsClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: storage client: %w", err)})
		return
	}
	it := bc.List(ctx, scope.ID)
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return
		}
		if err != nil {
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{
				Err: fmt.Errorf("gcp: list buckets: %w", err),
			})
			return
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildBucketResource(scope.ID, attrs)})
	}
}

func buildBucketResource(scopeID string, b *storage.BucketAttrs) inventory.Resource {
	// PublicAccess is best-effort: a bucket can still be public via IAM if
	// PublicAccessPrevention is not "enforced". Treat enforcement as the
	// definitive "not public" signal.
	publicAccess := b.PublicAccessPrevention != storage.PublicAccessPreventionEnforced
	detail := inventory.BucketDetail{
		Location:     b.Location,
		StorageClass: b.StorageClass,
		PublicAccess: publicAccess,
		Versioning:   b.VersioningEnabled,
		// SizeBytes populated in v1.1 via Cloud Monitoring; 0 in v1.
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindBucket, ID: b.Name},
		Kind:   inventory.KindBucket,
		Name:   b.Name,
		Region: b.Location,
		Status: "",
		Labels: b.Labels,
		Detail: &detail,
	}
}
