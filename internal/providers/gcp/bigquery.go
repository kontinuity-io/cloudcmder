package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	bigquery "cloud.google.com/go/bigquery"
	reservation "cloud.google.com/go/bigquery/reservation/apiv1"
	"cloud.google.com/go/bigquery/reservation/apiv1/reservationpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// bqDataset is the provider-internal projection of one BigQuery dataset plus
// the project's reservation capacity. It is the seam the build/enrich logic is
// tested against — the real client (untested, like every other realXClient)
// translates the BigQuery + Reservation SDK types into this shape.
type bqDataset struct {
	ID           string
	Location     string
	StorageBytes int64
	TableCount   int
	Edition      string // reservation edition for the dataset's location (best-effort)
	Slots        int64  // reservation slot capacity for the dataset's location (best-effort)
}

// bigQueryAPI is the seam between enrichBigQuery and Cloud BigQuery. Tests
// inject a fake; production uses realBigQueryClient.
type bigQueryAPI interface {
	ListDatasets(ctx context.Context, projectID string) ([]bqDataset, error)
	Close() error
}

// realBigQueryClient holds the credential options rather than a live client:
// bigquery.NewClient binds to a single project at construction, so a cached
// client would break --scan-all (one provider, many projects). We build a
// fresh per-project client inside ListDatasets instead.
type realBigQueryClient struct {
	opts []option.ClientOption
}

func (r *realBigQueryClient) ListDatasets(ctx context.Context, projectID string) ([]bqDataset, error) {
	c, err := bigquery.NewClient(ctx, projectID, r.opts...)
	if err != nil {
		return nil, fmt.Errorf("new bigquery client: %w", err)
	}
	defer func() { _ = c.Close() }()

	var out []bqDataset
	locSlots := map[string]int64{}
	locEdition := map[string]string{}

	it := c.Datasets(ctx)
	for {
		ds, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list datasets: %w", err)
		}
		d := bqDataset{ID: ds.DatasetID}
		if md, err := ds.Metadata(ctx); err == nil {
			d.Location = md.Location
		}
		d.StorageBytes, d.TableCount = datasetStorage(ctx, ds)

		// Reservation capacity is per-location; fetch each location once.
		if d.Location != "" {
			if _, seen := locSlots[d.Location]; !seen {
				ed, sl := r.locationReservation(ctx, projectID, d.Location)
				locEdition[d.Location] = ed
				locSlots[d.Location] = sl
			}
			d.Edition = locEdition[d.Location]
			d.Slots = locSlots[d.Location]
		}
		out = append(out, d)
	}
	return out, nil
}

// datasetStorage sums logical bytes across the dataset's tables and counts
// them. Per-table Metadata is best-effort: an unreadable table is counted but
// contributes 0 bytes rather than aborting the dataset.
func datasetStorage(ctx context.Context, ds *bigquery.Dataset) (int64, int) {
	var bytes int64
	var count int
	tit := ds.Tables(ctx)
	for {
		t, err := tit.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			break
		}
		count++
		if md, err := t.Metadata(ctx); err == nil {
			bytes += md.NumBytes
		}
	}
	return bytes, count
}

// locationReservation returns the edition and summed slot capacity for the
// project's reservations in one location. Best-effort: any error yields
// ("", 0) so a project without reservations (on-demand pricing) or a missing
// permission never aborts the BigQuery scan.
func (r *realBigQueryClient) locationReservation(ctx context.Context, projectID, location string) (string, int64) {
	rc, err := reservation.NewClient(ctx, r.opts...)
	if err != nil {
		return "", 0
	}
	defer func() { _ = rc.Close() }()

	parent := fmt.Sprintf("projects/%s/locations/%s", projectID, location)
	it := rc.ListReservations(ctx, &reservationpb.ListReservationsRequest{Parent: parent})
	var slots int64
	var edition string
	for {
		res, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return "", 0
		}
		slots += res.GetSlotCapacity()
		if e := res.GetEdition().String(); e != "" && e != "EDITION_UNSPECIFIED" {
			edition = e
		}
	}
	return edition, slots
}

func (r *realBigQueryClient) Close() error { return nil }

type bigQueryFactory func(ctx context.Context, opts ...option.ClientOption) (bigQueryAPI, error)

type bigQueryClientState struct {
	once    sync.Once
	cli     bigQueryAPI
	err     error
	factory bigQueryFactory
}

func (p *GCPProvider) bigQueryClient(ctx context.Context) (bigQueryAPI, error) {
	p.bq.once.Do(func() {
		if p.bq.factory != nil {
			p.bq.cli, p.bq.err = p.bq.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.bq.err = fmt.Errorf("gcp: ADC for bigquery client: %w", err)
			return
		}
		p.bq.cli = &realBigQueryClient{opts: []option.ClientOption{option.WithCredentials(creds)}}
	})
	if p.bq.err != nil {
		return nil, p.bq.err
	}
	return p.bq.cli, nil
}

func (p *GCPProvider) closeBigQueryClient() error {
	if p.bq.cli == nil {
		return nil
	}
	return p.bq.cli.Close()
}

// enrichBigQuery emits BigQueryDetail rows at the dataset grain. These overwrite
// the CAI Phase-1 stub rows (matching Ref ID = dataset ID). Table/Model/Routine
// stubs the enricher does not cover keep their Subtype-only StubDetail, which
// still decodes into BigQueryDetail (shared Subtype/Region prefix).
func enrichBigQuery(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	bc, err := p.bigQueryClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: bigquery client: %w", err)})
		return
	}
	datasets, err := bc.ListDatasets(ctx, scope.ID)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list bigquery datasets: %w", err)})
		return
	}
	for _, d := range datasets {
		if ctx.Err() != nil {
			return
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildBigQueryResource(scope.ID, d)})
	}
}

func buildBigQueryResource(scopeID string, d bqDataset) inventory.Resource {
	detail := inventory.BigQueryDetail{
		Subtype:      "Dataset",
		Region:       d.Location,
		LocationType: bigQueryLocationType(d.Location),
		StorageBytes: d.StorageBytes,
		TableCount:   d.TableCount,
		Edition:      d.Edition,
		Slots:        d.Slots,
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindGCPBigQuery, ID: d.ID},
		Kind:   inventory.KindGCPBigQuery,
		Name:   d.ID,
		Region: d.Location,
		Detail: &detail,
	}
}

// bigQueryLocationType classifies a BigQuery dataset location as a multi-region
// ("US", "EU") or a single region ("us-central1"). Multi-regions are the
// uppercase, hyphen-free location tokens BigQuery exposes.
func bigQueryLocationType(loc string) string {
	if loc == "" {
		return ""
	}
	if strings.Contains(loc, "-") {
		return "region"
	}
	return "multi-region"
}
