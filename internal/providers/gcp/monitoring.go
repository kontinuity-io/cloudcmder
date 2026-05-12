package gcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Cloud Monitoring metric types exposed by GCS. Both are GAUGE / DOUBLE per
// https://cloud.google.com/monitoring/api/metrics_gcp#gcp-storage — sampled
// once per day, ~24h after the bucket is observed. A bucket newer than one
// sample window has no time series at all.
const (
	metricBucketTotalBytes  = "storage.googleapis.com/storage/total_bytes"
	metricBucketObjectCount = "storage.googleapis.com/storage/object_count"
)

// bucketMetrics is the (size, count) tuple Monitoring returns for one bucket.
type bucketMetrics struct {
	SizeBytes   int64
	ObjectCount int64
}

// metricsAPI is the seam between the bucket enrich loop and Cloud Monitoring.
// Tests inject a fake; production uses realMetricsClient.
type metricsAPI interface {
	ListBucketMetrics(ctx context.Context, projectID string) (map[string]bucketMetrics, error)
	Close() error
}

type realMetricsClient struct {
	c *monitoring.MetricClient
}

func (r *realMetricsClient) ListBucketMetrics(ctx context.Context, projectID string) (map[string]bucketMetrics, error) {
	// Query both metrics in a single ListTimeSeries call. The "starts_with"
	// filter narrows the response on the server side so we don't pay for
	// unrelated GCS metrics.
	now := time.Now()
	req := &monitoringpb.ListTimeSeriesRequest{
		Name: "projects/" + projectID,
		Filter: fmt.Sprintf(
			`metric.type = starts_with("storage.googleapis.com/storage/") AND (metric.type = %q OR metric.type = %q)`,
			metricBucketTotalBytes, metricBucketObjectCount),
		Interval: &monitoringpb.TimeInterval{
			// 26h window gives the 24h-delayed daily sample headroom even if
			// the metric pipeline is slightly behind.
			StartTime: timestamppb.New(now.Add(-26 * time.Hour)),
			EndTime:   timestamppb.New(now),
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:  durationpb.New(24 * time.Hour),
			PerSeriesAligner: monitoringpb.Aggregation_ALIGN_MEAN,
		},
		View: monitoringpb.ListTimeSeriesRequest_FULL,
	}
	var series []*monitoringpb.TimeSeries
	it := r.c.ListTimeSeries(ctx, req)
	for {
		ts, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		series = append(series, ts)
	}
	return parseBucketTimeSeries(series), nil
}

func (r *realMetricsClient) Close() error { return r.c.Close() }

// parseBucketTimeSeries collapses the per-(bucket, storage_class) time series
// into one bucketMetrics row per bucket name. Buckets with Autoclass produce
// one series per storage class, so size and object_count are summed across
// classes for the same bucket. Buckets younger than the first sample window
// (~24h) are absent — callers treat missing entries as zeros.
func parseBucketTimeSeries(series []*monitoringpb.TimeSeries) map[string]bucketMetrics {
	out := make(map[string]bucketMetrics, len(series))
	for _, ts := range series {
		if ts == nil || ts.GetResource() == nil || ts.GetMetric() == nil {
			continue
		}
		bucket := ts.GetResource().GetLabels()["bucket_name"]
		if bucket == "" || len(ts.GetPoints()) == 0 {
			continue
		}
		v := pointInt64(ts.GetPoints()[0])
		m := out[bucket]
		switch ts.GetMetric().GetType() {
		case metricBucketTotalBytes:
			m.SizeBytes += v
		case metricBucketObjectCount:
			m.ObjectCount += v
		default:
			continue
		}
		out[bucket] = m
	}
	return out
}

// pointInt64 returns the int64 view of a Monitoring point. The storage
// metrics are exposed as DOUBLE but pre-aligned values from
// ListTimeSeries can carry either typed value depending on aligner —
// accept both shapes and treat anything else as 0.
func pointInt64(p *monitoringpb.Point) int64 {
	if p == nil || p.GetValue() == nil {
		return 0
	}
	switch v := p.GetValue().GetValue().(type) {
	case *monitoringpb.TypedValue_Int64Value:
		return v.Int64Value
	case *monitoringpb.TypedValue_DoubleValue:
		return int64(v.DoubleValue)
	default:
		return 0
	}
}

// --- lazy client glue (mirrors bucketsClientState pattern in storage.go) ---

type metricsFactory func(ctx context.Context, opts ...option.ClientOption) (metricsAPI, error)

type metricsClientState struct {
	once    sync.Once
	cli     metricsAPI
	err     error
	factory metricsFactory
}

func (p *GCPProvider) metricsClient(ctx context.Context) (metricsAPI, error) {
	p.metrics.once.Do(func() {
		if p.metrics.factory != nil {
			p.metrics.cli, p.metrics.err = p.metrics.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.metrics.err = fmt.Errorf("gcp: ADC for monitoring client: %w", err)
			return
		}
		c, err := monitoring.NewMetricClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.metrics.err = fmt.Errorf("gcp: new monitoring client: %w", err)
			return
		}
		p.metrics.cli = &realMetricsClient{c: c}
	})
	if p.metrics.err != nil {
		return nil, p.metrics.err
	}
	return p.metrics.cli, nil
}

func (p *GCPProvider) closeMetricsClient() error {
	if p.metrics.cli == nil {
		return nil
	}
	return p.metrics.cli.Close()
}
