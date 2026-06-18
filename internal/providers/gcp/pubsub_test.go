package gcp

import (
	"context"
	"testing"
	"time"

	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

func TestHumanDuration(t *testing.T) {
	cases := map[time.Duration]string{
		0:                  "",
		-time.Second:       "",
		7 * 24 * time.Hour: "7d",
		24 * time.Hour:     "1d",
		3 * time.Hour:      "3h",
		10 * time.Minute:   "10m",
		90 * time.Second:   "90s",
	}
	for in, want := range cases {
		if got := humanDuration(in); got != want {
			t.Errorf("humanDuration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildPubSubResource(t *testing.T) {
	// Topic grain.
	topic := psResource{
		ID:                "orders",
		Subtype:           "Topic",
		SubscriptionCount: 3,
		MessageRetention:  "7d",
		PublishedBytes:    4096,
	}
	r := buildPubSubResource("p1", topic)
	if r.Kind != inventory.KindGCPPubSub || r.Name != "orders" {
		t.Fatalf("resource header = %+v", r)
	}
	if r.Ref.ID != "orders" {
		t.Errorf("ref id = %q, want orders (must match CAI stub id to overwrite)", r.Ref.ID)
	}
	pd, ok := r.Detail.(*inventory.PubSubDetail)
	if !ok {
		t.Fatalf("detail type = %T", r.Detail)
	}
	if pd.Subtype != "Topic" || pd.SubscriptionCount != 3 || pd.MessageRetention != "7d" || pd.PublishedBytes != 4096 {
		t.Errorf("topic detail = %+v", pd)
	}
	if pd.DeliveryType != "" {
		t.Errorf("topic DeliveryType = %q, want empty", pd.DeliveryType)
	}

	// Subscription grain.
	sub := psResource{
		ID:               "orders-bq",
		Subtype:          "Subscription",
		DeliveryType:     "bigquery",
		MessageRetention: "10m",
	}
	rs := buildPubSubResource("p1", sub)
	if rs.Ref.ID != "orders-bq" {
		t.Errorf("ref id = %q, want orders-bq", rs.Ref.ID)
	}
	sd := rs.Detail.(*inventory.PubSubDetail)
	if sd.Subtype != "Subscription" || sd.DeliveryType != "bigquery" || sd.MessageRetention != "10m" {
		t.Errorf("subscription detail = %+v", sd)
	}
	if sd.SubscriptionCount != 0 {
		t.Errorf("subscription SubscriptionCount = %d, want 0", sd.SubscriptionCount)
	}
}

// --- fake pubsubAPI --------------------------------------------------------

type fakePubSubClient struct {
	resources []psResource
	err       error
}

func (f *fakePubSubClient) ListTopicsAndSubscriptions(_ context.Context, _ string) ([]psResource, error) {
	return f.resources, f.err
}

func (f *fakePubSubClient) Close() error { return nil }

func TestEnrichPubSub(t *testing.T) {
	p := &GCPProvider{}
	p.pubsub.factory = func(_ context.Context, _ ...option.ClientOption) (pubsubAPI, error) {
		return &fakePubSubClient{resources: []psResource{
			{ID: "t1", Subtype: "Topic", SubscriptionCount: 2, MessageRetention: "7d", PublishedBytes: 1024},
			{ID: "s1", Subtype: "Subscription", DeliveryType: "push", MessageRetention: "10m"},
		}}, nil
	}

	ch := make(chan inventory.ResourceOrErr, 8)
	enrichPubSub(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
	close(ch)

	var got []inventory.Resource
	for x := range ch {
		if x.Err != nil {
			t.Fatalf("unexpected err: %v", x.Err)
		}
		got = append(got, x.Resource)
	}
	if len(got) != 2 {
		t.Fatalf("got %d resources, want 2", len(got))
	}
	if got[0].Name != "t1" || got[1].Name != "s1" {
		t.Errorf("names = %q, %q", got[0].Name, got[1].Name)
	}
	d0 := got[0].Detail.(*inventory.PubSubDetail)
	if d0.Subtype != "Topic" || d0.SubscriptionCount != 2 || d0.PublishedBytes != 1024 {
		t.Errorf("topic detail = %+v", d0)
	}
	d1 := got[1].Detail.(*inventory.PubSubDetail)
	if d1.Subtype != "Subscription" || d1.DeliveryType != "push" {
		t.Errorf("subscription detail = %+v", d1)
	}
}
