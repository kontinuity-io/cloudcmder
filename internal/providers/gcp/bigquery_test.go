package gcp

import (
	"context"
	"testing"

	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

func TestBigQueryLocationType(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"US":          "multi-region",
		"EU":          "multi-region",
		"us-central1": "region",
		"europe-west4": "region",
	}
	for in, want := range cases {
		if got := bigQueryLocationType(in); got != want {
			t.Errorf("bigQueryLocationType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildBigQueryResource(t *testing.T) {
	d := bqDataset{
		ID:           "analytics",
		Location:     "US",
		StorageBytes: 1 << 30,
		TableCount:   12,
		Edition:      "ENTERPRISE",
		Slots:        500,
	}
	r := buildBigQueryResource("p1", d)

	if r.Kind != inventory.KindGCPBigQuery || r.Name != "analytics" || r.Region != "US" {
		t.Fatalf("resource header = %+v", r)
	}
	if r.Ref.ID != "analytics" {
		t.Errorf("ref id = %q, want analytics (must match CAI stub id to overwrite)", r.Ref.ID)
	}
	bd, ok := r.Detail.(*inventory.BigQueryDetail)
	if !ok {
		t.Fatalf("detail type = %T", r.Detail)
	}
	if bd.Subtype != "Dataset" || bd.LocationType != "multi-region" {
		t.Errorf("detail = %+v", bd)
	}
	if bd.StorageBytes != 1<<30 || bd.TableCount != 12 || bd.Edition != "ENTERPRISE" || bd.Slots != 500 {
		t.Errorf("detail = %+v", bd)
	}
}

// --- fake bigQueryAPI ------------------------------------------------------

type fakeBigQueryClient struct {
	datasets []bqDataset
	err      error
}

func (f *fakeBigQueryClient) ListDatasets(_ context.Context, _ string) ([]bqDataset, error) {
	return f.datasets, f.err
}

func (f *fakeBigQueryClient) Close() error { return nil }

func TestEnrichBigQuery(t *testing.T) {
	p := &GCPProvider{}
	p.bq.factory = func(_ context.Context, _ ...option.ClientOption) (bigQueryAPI, error) {
		return &fakeBigQueryClient{datasets: []bqDataset{
			{ID: "ds1", Location: "EU", TableCount: 3},
			{ID: "ds2", Location: "us-central1", StorageBytes: 2048, TableCount: 1},
		}}, nil
	}

	ch := make(chan inventory.ResourceOrErr, 8)
	enrichBigQuery(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
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
	if got[0].Name != "ds1" || got[1].Name != "ds2" {
		t.Errorf("names = %q, %q", got[0].Name, got[1].Name)
	}
	d1 := got[0].Detail.(*inventory.BigQueryDetail)
	if d1.LocationType != "multi-region" || d1.TableCount != 3 {
		t.Errorf("ds1 detail = %+v", d1)
	}
}
