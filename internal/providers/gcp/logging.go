package gcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	logging "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// logBucketRow is the provider-internal projection of a Cloud Logging LogBucket.
// Using an internal type keeps the loggingAPI interface free of SDK types so
// the fake in logging_test.go stays SDK-free.
type logBucketRow struct {
	Name             string // full resource name, e.g. projects/p/locations/global/buckets/_Default
	Location         string
	RetentionDays    int32
	Locked           bool
	AnalyticsEnabled bool
	LifecycleState   string
}

// loggingAPI is the seam between the Logging enricher and the Cloud Logging Config API.
type loggingAPI interface {
	ListBuckets(ctx context.Context, parent string) ([]logBucketRow, error)
	Close() error
}

type realLoggingClient struct {
	c *logging.ConfigClient
}

func (r *realLoggingClient) ListBuckets(ctx context.Context, parent string) ([]logBucketRow, error) {
	req := &loggingpb.ListBucketsRequest{Parent: parent}
	it := r.c.ListBuckets(ctx, req)
	var out []logBucketRow
	for {
		b, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, logBucketRow{
			Name:             b.GetName(),
			Location:         locationFromBucketName(b.GetName()),
			RetentionDays:    b.GetRetentionDays(),
			Locked:           b.GetLocked(),
			AnalyticsEnabled: b.GetAnalyticsEnabled(),
			LifecycleState:   b.GetLifecycleState().String(),
		})
	}
	return out, nil
}

func (r *realLoggingClient) Close() error { return r.c.Close() }

type loggingFactory func(ctx context.Context, opts ...option.ClientOption) (loggingAPI, error)

type loggingClientState struct {
	once    sync.Once
	cli     loggingAPI
	err     error
	factory loggingFactory
}

func (p *GCPProvider) loggingClient(ctx context.Context) (loggingAPI, error) {
	p.logging.once.Do(func() {
		if p.logging.factory != nil {
			p.logging.cli, p.logging.err = p.logging.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.logging.err = fmt.Errorf("gcp: ADC for logging client: %w", err)
			return
		}
		c, err := logging.NewConfigClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.logging.err = fmt.Errorf("gcp: new logging config client: %w", err)
			return
		}
		p.logging.cli = &realLoggingClient{c: c}
	})
	if p.logging.err != nil {
		return nil, p.logging.err
	}
	return p.logging.cli, nil
}

func (p *GCPProvider) closeLoggingClient() error {
	if p.logging.cli == nil {
		return nil
	}
	return p.logging.cli.Close()
}

// enrichLogBuckets is the Phase-2 enricher for KindGCPLogging / LogBucket
// grain. LogMetric and LogSink grains are left as stubs (no Phase-2 enricher
// for those subtypes — only LogBucket is enriched here).
func enrichLogBuckets(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	lc, err := p.loggingClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: logging client: %w", err)})
		return
	}

	// Cloud Logging organises buckets under
	//   projects/{project}/locations/{location}/buckets/{bucket}
	// The CAI stub uses the parent path "projects/{p}/locations/-" for
	// listing across all locations.
	parent := "projects/" + scope.ID + "/locations/-"
	buckets, err := lc.ListBuckets(ctx, parent)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list log buckets: %w", err)})
		return
	}

	// Pre-fetch storage bytes from Cloud Monitoring — best-effort; leave 0 on any error.
	storageMap := loadLogBucketStorageMetrics(ctx, p, scope.ID)

	for _, b := range buckets {
		id := lastSegment(b.Name)
		// storageMap is keyed by the bucket short name (the log_bucket metric
		// label value), not the full resource name.
		detail := inventory.LoggingDetail{
			Subtype:          "LogBucket",
			Region:           b.Location,
			RetentionDays:    b.RetentionDays,
			Locked:           b.Locked,
			AnalyticsEnabled: b.AnalyticsEnabled,
			LifecycleState:   b.LifecycleState,
			StorageBytes:     storageMap[id],
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{
			Resource: inventory.Resource{
				Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scope.ID, Kind: inventory.KindGCPLogging, ID: id},
				Kind:   inventory.KindGCPLogging,
				Name:   id,
				Region: b.Location,
				Status: b.LifecycleState,
				Detail: &detail,
			},
		})
	}
}

// loadLogBucketStorageMetrics fetches the bytes_ingested metric from Cloud
// Monitoring for all log buckets in the project. Best-effort: returns nil on
// any error so the enricher continues without storage data.
func loadLogBucketStorageMetrics(ctx context.Context, p *GCPProvider, projectID string) map[string]int64 {
	mc, err := p.metricsClient(ctx)
	if err != nil {
		slog.Warn("scan: monitoring client unavailable for log bucket storage",
			"project", projectID, "error", err)
		return nil
	}
	m, err := mc.ListLogBucketMetrics(ctx, projectID)
	if err != nil {
		slog.Warn("scan: log bucket metrics unavailable; storage = 0",
			"project", projectID, "error", err)
		return nil
	}
	return m
}

// locationFromBucketName extracts the location segment from a log bucket
// resource name like
//
//	projects/p/locations/us-central1/buckets/_Default
//	→ "us-central1"
func locationFromBucketName(name string) string {
	return regionFromResourceName(name)
}
