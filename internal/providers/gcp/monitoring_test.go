package gcp

import (
	"testing"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/stretchr/testify/assert"
	googlemetric "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"
)

// timeSeriesFor builds a minimal TimeSeries for one (bucket, metric, value)
// triple. Tests assemble fixtures by concatenating these.
func timeSeriesFor(bucket, metricType string, value int64) *monitoringpb.TimeSeries {
	return &monitoringpb.TimeSeries{
		Metric: &googlemetric.Metric{Type: metricType},
		Resource: &monitoredres.MonitoredResource{
			Type:   "gcs_bucket",
			Labels: map[string]string{"bucket_name": bucket},
		},
		Points: []*monitoringpb.Point{
			{Value: &monitoringpb.TypedValue{
				Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: float64(value)},
			}},
		},
	}
}

func TestParseBucketTimeSeries(t *testing.T) {
	cases := []struct {
		name   string
		series []*monitoringpb.TimeSeries
		want   map[string]bucketMetrics
	}{
		{
			name: "single bucket, both metrics",
			series: []*monitoringpb.TimeSeries{
				timeSeriesFor("alpha", metricBucketTotalBytes, 1024),
				timeSeriesFor("alpha", metricBucketObjectCount, 7),
			},
			want: map[string]bucketMetrics{
				"alpha": {SizeBytes: 1024, ObjectCount: 7},
			},
		},
		{
			name: "autoclass — multi-storage-class sums per bucket",
			series: []*monitoringpb.TimeSeries{
				timeSeriesFor("beta", metricBucketTotalBytes, 100),
				timeSeriesFor("beta", metricBucketTotalBytes, 250),
				timeSeriesFor("beta", metricBucketObjectCount, 3),
				timeSeriesFor("beta", metricBucketObjectCount, 5),
			},
			want: map[string]bucketMetrics{
				"beta": {SizeBytes: 350, ObjectCount: 8},
			},
		},
		{
			name: "multiple buckets isolated",
			series: []*monitoringpb.TimeSeries{
				timeSeriesFor("a", metricBucketTotalBytes, 10),
				timeSeriesFor("b", metricBucketTotalBytes, 20),
				timeSeriesFor("a", metricBucketObjectCount, 1),
			},
			want: map[string]bucketMetrics{
				"a": {SizeBytes: 10, ObjectCount: 1},
				"b": {SizeBytes: 20, ObjectCount: 0},
			},
		},
		{
			name:   "empty input → empty map",
			series: nil,
			want:   map[string]bucketMetrics{},
		},
		{
			name: "missing bucket_name label → skipped",
			series: []*monitoringpb.TimeSeries{
				{
					Metric:   &googlemetric.Metric{Type: metricBucketTotalBytes},
					Resource: &monitoredres.MonitoredResource{Type: "gcs_bucket", Labels: map[string]string{}},
					Points: []*monitoringpb.Point{{Value: &monitoringpb.TypedValue{
						Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 999},
					}}},
				},
			},
			want: map[string]bucketMetrics{},
		},
		{
			name: "zero points → skipped",
			series: []*monitoringpb.TimeSeries{
				{
					Metric:   &googlemetric.Metric{Type: metricBucketTotalBytes},
					Resource: &monitoredres.MonitoredResource{Labels: map[string]string{"bucket_name": "empty"}},
					Points:   nil,
				},
			},
			want: map[string]bucketMetrics{},
		},
		{
			name: "unknown metric type → ignored",
			series: []*monitoringpb.TimeSeries{
				timeSeriesFor("c", "storage.googleapis.com/network/sent_bytes_count", 999),
				timeSeriesFor("c", metricBucketTotalBytes, 50),
			},
			want: map[string]bucketMetrics{
				"c": {SizeBytes: 50},
			},
		},
		{
			name: "int64 value type also handled",
			series: []*monitoringpb.TimeSeries{
				{
					Metric:   &googlemetric.Metric{Type: metricBucketObjectCount},
					Resource: &monitoredres.MonitoredResource{Labels: map[string]string{"bucket_name": "d"}},
					Points: []*monitoringpb.Point{{Value: &monitoringpb.TypedValue{
						Value: &monitoringpb.TypedValue_Int64Value{Int64Value: 42},
					}}},
				},
			},
			want: map[string]bucketMetrics{
				"d": {ObjectCount: 42},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBucketTimeSeries(tc.series)
			assert.Equal(t, tc.want, got)
		})
	}
}
