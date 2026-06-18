package gcp

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// --- fake logging client ---------------------------------------------------

type fakeLoggingClient struct {
	buckets []logBucketRow
	err     error
}

func (f *fakeLoggingClient) ListBuckets(_ context.Context, _ string) ([]logBucketRow, error) {
	return f.buckets, f.err
}

func (f *fakeLoggingClient) Close() error { return nil }

// --- fake metrics client that also satisfies ListLogBucketMetrics ----------

type fakeMetricsClientWithLogging struct {
	logBytes map[string]int64
}

func (f *fakeMetricsClientWithLogging) ListBucketMetrics(_ context.Context, _ string) (map[string]bucketMetrics, error) {
	return nil, nil
}

func (f *fakeMetricsClientWithLogging) ListLogBucketMetrics(_ context.Context, _ string) (map[string]int64, error) {
	return f.logBytes, nil
}

func (f *fakeMetricsClientWithLogging) Close() error { return nil }

// --- tests -----------------------------------------------------------------

func TestEnrichLogBuckets_HappyPath(t *testing.T) {
	buckets := []logBucketRow{
		{
			Name:             "projects/p1/locations/us-central1/buckets/_Default",
			Location:         "us-central1",
			RetentionDays:    30,
			Locked:           false,
			AnalyticsEnabled: true,
			LifecycleState:   "ACTIVE",
		},
		{
			Name:             "projects/p1/locations/global/buckets/_Required",
			Location:         "global",
			RetentionDays:    400,
			Locked:           true,
			AnalyticsEnabled: false,
			LifecycleState:   "ACTIVE",
		},
	}

	ch := make(chan inventory.ResourceOrErr, 16)
	p := &GCPProvider{}
	p.logging.factory = func(_ context.Context, _ ...option.ClientOption) (loggingAPI, error) {
		return &fakeLoggingClient{buckets: buckets}, nil
	}
	p.metrics.factory = func(_ context.Context, _ ...option.ClientOption) (metricsAPI, error) {
		return &fakeMetricsClientWithLogging{}, nil
	}

	enrichLogBuckets(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	var results []inventory.Resource
	for x := range ch {
		if x.Err != nil {
			t.Fatalf("unexpected error: %v", x.Err)
		}
		results = append(results, x.Resource)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Check first bucket (_Default)
	r0 := results[0]
	if r0.Ref.ID != "_Default" {
		t.Errorf("ID = %q, want _Default", r0.Ref.ID)
	}
	if r0.Ref.Kind != inventory.KindGCPLogging {
		t.Errorf("Kind = %v, want KindGCPLogging", r0.Ref.Kind)
	}
	d0, ok := r0.Detail.(*inventory.LoggingDetail)
	if !ok {
		t.Fatalf("Detail type = %T, want *LoggingDetail", r0.Detail)
	}
	if d0.Subtype != "LogBucket" {
		t.Errorf("Subtype = %q, want LogBucket", d0.Subtype)
	}
	if d0.Region != "us-central1" {
		t.Errorf("Region = %q, want us-central1", d0.Region)
	}
	if d0.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", d0.RetentionDays)
	}
	if d0.Locked {
		t.Errorf("Locked = true, want false")
	}
	if !d0.AnalyticsEnabled {
		t.Errorf("AnalyticsEnabled = false, want true")
	}

	// Check second bucket (_Required)
	r1 := results[1]
	d1, ok := r1.Detail.(*inventory.LoggingDetail)
	if !ok {
		t.Fatalf("Detail type = %T, want *LoggingDetail", r1.Detail)
	}
	if !d1.Locked {
		t.Errorf("Locked = false, want true")
	}
	if d1.RetentionDays != 400 {
		t.Errorf("RetentionDays = %d, want 400", d1.RetentionDays)
	}
}

func TestEnrichLogBuckets_EmptyList(t *testing.T) {
	ch := make(chan inventory.ResourceOrErr, 4)
	p := &GCPProvider{}
	p.logging.factory = func(_ context.Context, _ ...option.ClientOption) (loggingAPI, error) {
		return &fakeLoggingClient{buckets: nil}, nil
	}
	p.metrics.factory = func(_ context.Context, _ ...option.ClientOption) (metricsAPI, error) {
		return &fakeMetricsClientWithLogging{}, nil
	}

	enrichLogBuckets(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	for x := range ch {
		t.Errorf("expected no results, got: %+v", x)
	}
}

func TestEnrichLogBuckets_ClientError(t *testing.T) {
	wantErr := errors.New("API unavailable")
	ch := make(chan inventory.ResourceOrErr, 4)
	p := &GCPProvider{}
	p.logging.factory = func(_ context.Context, _ ...option.ClientOption) (loggingAPI, error) {
		return &fakeLoggingClient{err: wantErr}, nil
	}
	p.metrics.factory = func(_ context.Context, _ ...option.ClientOption) (metricsAPI, error) {
		return &fakeMetricsClientWithLogging{}, nil
	}

	enrichLogBuckets(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	var sawErr bool
	for x := range ch {
		if x.Err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Errorf("expected error to be propagated")
	}
}

func TestEnrichLogBuckets_StorageBytesFromMonitoring(t *testing.T) {
	buckets := []logBucketRow{
		{
			Name:           "projects/p1/locations/us-central1/buckets/my-bucket",
			Location:       "us-central1",
			LifecycleState: "ACTIVE",
		},
	}

	ch := make(chan inventory.ResourceOrErr, 4)
	p := &GCPProvider{}
	p.logging.factory = func(_ context.Context, _ ...option.ClientOption) (loggingAPI, error) {
		return &fakeLoggingClient{buckets: buckets}, nil
	}
	p.metrics.factory = func(_ context.Context, _ ...option.ClientOption) (metricsAPI, error) {
		return &fakeMetricsClientWithLogging{
			logBytes: map[string]int64{
				"my-bucket": 512 * 1024 * 1024, // 512 MiB
			},
		}, nil
	}

	enrichLogBuckets(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	for x := range ch {
		if x.Err != nil {
			t.Fatalf("unexpected error: %v", x.Err)
		}
		d, ok := x.Resource.Detail.(*inventory.LoggingDetail)
		if !ok {
			t.Fatalf("Detail type = %T", x.Resource.Detail)
		}
		if d.StorageBytes != 512*1024*1024 {
			t.Errorf("StorageBytes = %d, want %d", d.StorageBytes, 512*1024*1024)
		}
	}
}

func TestEnrichLogBuckets_MonitoringUnavailable(t *testing.T) {
	// When the metrics client is unavailable, StorageBytes should be 0 and
	// the enricher should still emit the bucket row.
	buckets := []logBucketRow{
		{
			Name:           "projects/p1/locations/us-central1/buckets/b1",
			Location:       "us-central1",
			LifecycleState: "ACTIVE",
		},
	}

	ch := make(chan inventory.ResourceOrErr, 4)
	p := &GCPProvider{}
	p.logging.factory = func(_ context.Context, _ ...option.ClientOption) (loggingAPI, error) {
		return &fakeLoggingClient{buckets: buckets}, nil
	}
	p.metrics.factory = func(_ context.Context, _ ...option.ClientOption) (metricsAPI, error) {
		return nil, errors.New("monitoring API disabled")
	}

	enrichLogBuckets(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	var count int
	for x := range ch {
		if x.Err != nil {
			t.Errorf("unexpected error: %v", x.Err)
			continue
		}
		count++
		d := x.Resource.Detail.(*inventory.LoggingDetail)
		if d.StorageBytes != 0 {
			t.Errorf("StorageBytes should be 0 when monitoring unavailable, got %d", d.StorageBytes)
		}
	}
	if count != 1 {
		t.Errorf("expected 1 resource, got %d", count)
	}
}
