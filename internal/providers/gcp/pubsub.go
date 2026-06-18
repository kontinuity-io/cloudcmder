package gcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"cloud.google.com/go/pubsub"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cloudcmder.com/internal/inventory"
)

// metricTopicSendRequestBytes is the Cloud Monitoring metric for bytes of
// publish requests sent to a topic. DELTA / INT64 on the pubsub_topic
// monitored resource (label topic_id). Best-effort: a disabled Monitoring
// API or a missing permission leaves PublishedBytes at 0.
//
// https://cloud.google.com/monitoring/api/metrics_gcp#gcp-pubsub
const metricTopicSendRequestBytes = "pubsub.googleapis.com/topic/send_request_count"

// psResource is the provider-internal projection of one Pub/Sub topic or
// subscription. It is the seam the build/enrich logic is tested against — the
// real client (untested, like every other realXClient) translates the Pub/Sub
// + Monitoring SDK types into this shape so tests never import the SDK.
type psResource struct {
	ID                string // last path segment of the full resource name
	Subtype           string // "Topic" | "Subscription"
	Region            string // best-effort; Pub/Sub is global, so usually ""
	DeliveryType      string // subscriptions only: "push" | "pull" | "bigquery" | "cloudstorage"
	SubscriptionCount int    // topics only: number of attached subscriptions
	MessageRetention  string // human duration, e.g. "7d" / "10m"
	PublishedBytes    int64  // topics only: published bytes over the metric window (best-effort)
}

// pubsubAPI is the seam between enrichPubSub and Cloud Pub/Sub. Tests inject a
// fake; production uses realPubSubClient.
type pubsubAPI interface {
	ListTopicsAndSubscriptions(ctx context.Context, projectID string) ([]psResource, error)
	Close() error
}

// realPubSubClient holds the credential options rather than a live client:
// pubsub.NewClient binds to a single project at construction, so a cached
// client would break --scan-all (one provider, many projects). We build a
// fresh per-project client inside ListTopicsAndSubscriptions instead.
type realPubSubClient struct {
	opts []option.ClientOption
}

func (r *realPubSubClient) ListTopicsAndSubscriptions(ctx context.Context, projectID string) ([]psResource, error) {
	c, err := pubsub.NewClient(ctx, projectID, r.opts...)
	if err != nil {
		return nil, fmt.Errorf("new pubsub client: %w", err)
	}
	defer func() { _ = c.Close() }()

	// Published bytes per topic from Cloud Monitoring. Best-effort: a nil map
	// (disabled API / missing permission) leaves every topic's PublishedBytes
	// at 0 rather than aborting the scan.
	bytesByTopic := r.loadTopicBytes(ctx, projectID)

	var out []psResource

	it := c.Topics(ctx)
	for {
		t, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list topics: %w", err)
		}
		ps := psResource{
			ID:             t.ID(),
			Subtype:        "Topic",
			PublishedBytes: bytesByTopic[t.ID()],
		}
		if cfg, err := t.Config(ctx); err == nil {
			ps.MessageRetention = topicRetention(cfg)
		}
		ps.SubscriptionCount = countSubscriptions(ctx, t)
		out = append(out, ps)
	}

	sit := c.Subscriptions(ctx)
	for {
		s, err := sit.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list subscriptions: %w", err)
		}
		ps := psResource{
			ID:           s.ID(),
			Subtype:      "Subscription",
			DeliveryType: "pull",
		}
		if cfg, err := s.Config(ctx); err == nil {
			ps.DeliveryType = subscriptionDeliveryType(cfg)
			ps.MessageRetention = humanDuration(cfg.RetentionDuration)
		}
		out = append(out, ps)
	}
	return out, nil
}

// countSubscriptions counts the subscriptions attached to a topic. Best-effort:
// any error (e.g. missing pubsub.topics.getIamPolicy on a single topic) yields
// the count gathered so far rather than aborting the whole topic scan.
func countSubscriptions(ctx context.Context, t *pubsub.Topic) int {
	var n int
	it := t.Subscriptions(ctx)
	for {
		_, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			break
		}
		n++
	}
	return n
}

// topicRetention renders a topic's optional message-retention duration as a
// short human string. Returns "" when the topic does not set retention (the
// SDK leaves the optional.Duration nil).
func topicRetention(cfg pubsub.TopicConfig) string {
	if cfg.RetentionDuration == nil {
		return ""
	}
	d, ok := cfg.RetentionDuration.(time.Duration)
	if !ok {
		return ""
	}
	return humanDuration(d)
}

// subscriptionDeliveryType classifies a subscription's delivery mechanism from
// its config. At most one of the push/BigQuery/CloudStorage configs is set; an
// empty set means classic pull.
func subscriptionDeliveryType(cfg pubsub.SubscriptionConfig) string {
	switch {
	case cfg.BigQueryConfig.Table != "":
		return "bigquery"
	case cfg.CloudStorageConfig.Bucket != "":
		return "cloudstorage"
	case cfg.PushConfig.Endpoint != "":
		return "push"
	default:
		return "pull"
	}
}

// loadTopicBytes returns published byte counts keyed by topic ID. A nil map
// means the Monitoring call did not yield data — typically the API is disabled,
// the caller lacks monitoring.timeSeries.list, or the project has no samples in
// the window. The scan continues regardless; affected topics show 0.
func (r *realPubSubClient) loadTopicBytes(ctx context.Context, projectID string) map[string]int64 {
	mc, err := monitoring.NewMetricClient(ctx, r.opts...)
	if err != nil {
		return nil
	}
	defer func() { _ = mc.Close() }()

	now := time.Now()
	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   "projects/" + projectID,
		Filter: fmt.Sprintf("metric.type = %q", metricTopicSendRequestBytes),
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamppb.New(now.Add(-26 * time.Hour)),
			EndTime:   timestamppb.New(now),
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:    durationpb.New(24 * time.Hour),
			PerSeriesAligner:   monitoringpb.Aggregation_ALIGN_MEAN,
			CrossSeriesReducer: monitoringpb.Aggregation_REDUCE_SUM,
			GroupByFields:      []string{"resource.label.topic_id"},
		},
		View: monitoringpb.ListTimeSeriesRequest_FULL,
	}
	out := map[string]int64{}
	it := mc.ListTimeSeries(ctx, req)
	for {
		ts, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil
		}
		if ts.GetResource() == nil || len(ts.GetPoints()) == 0 {
			continue
		}
		topic := ts.GetResource().GetLabels()["topic_id"]
		if topic == "" {
			continue
		}
		out[topic] += pointInt64(ts.GetPoints()[0])
	}
	return out
}

func (r *realPubSubClient) Close() error { return nil }

type pubsubFactory func(ctx context.Context, opts ...option.ClientOption) (pubsubAPI, error)

type pubsubClientState struct {
	once    sync.Once
	cli     pubsubAPI
	err     error
	factory pubsubFactory
}

func (p *GCPProvider) pubsubClient(ctx context.Context) (pubsubAPI, error) {
	p.pubsub.once.Do(func() {
		if p.pubsub.factory != nil {
			p.pubsub.cli, p.pubsub.err = p.pubsub.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.pubsub.err = fmt.Errorf("gcp: ADC for pubsub client: %w", err)
			return
		}
		p.pubsub.cli = &realPubSubClient{opts: []option.ClientOption{option.WithCredentials(creds)}}
	})
	if p.pubsub.err != nil {
		return nil, p.pubsub.err
	}
	return p.pubsub.cli, nil
}

func (p *GCPProvider) closePubSubClient() error {
	if p.pubsub.cli == nil {
		return nil
	}
	return p.pubsub.cli.Close()
}

// enrichPubSub emits PubSubDetail rows at the Topic and Subscription grain.
// These overwrite the CAI Phase-1 stub rows (matching Ref ID = last name
// segment). Schema/Snapshot stubs the enricher does not cover keep their
// Subtype-only StubDetail, which still decodes into PubSubDetail (shared
// Subtype/Region prefix).
func enrichPubSub(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	pc, err := p.pubsubClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: pubsub client: %w", err)})
		return
	}
	resources, err := pc.ListTopicsAndSubscriptions(ctx, scope.ID)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list pubsub topics/subscriptions: %w", err)})
		return
	}
	for _, ps := range resources {
		if ctx.Err() != nil {
			return
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildPubSubResource(scope.ID, ps)})
	}
}

func buildPubSubResource(scopeID string, ps psResource) inventory.Resource {
	detail := inventory.PubSubDetail{
		Subtype:           ps.Subtype,
		Region:            ps.Region,
		DeliveryType:      ps.DeliveryType,
		SubscriptionCount: ps.SubscriptionCount,
		MessageRetention:  ps.MessageRetention,
		PublishedBytes:    ps.PublishedBytes,
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindGCPPubSub, ID: ps.ID},
		Kind:   inventory.KindGCPPubSub,
		Name:   ps.ID,
		Region: ps.Region,
		Detail: &detail,
	}
}

// humanDuration renders a Go duration as a short human string: whole days as
// "Nd", whole hours as "Nh", whole minutes as "Nm", otherwise seconds. Returns
// "" for non-positive durations so unset retention renders as "—" downstream.
func humanDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	default:
		return fmt.Sprintf("%ds", d/time.Second)
	}
}
