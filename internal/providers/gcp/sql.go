package gcp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"

	"cloudcmder.com/internal/inventory"
)

// sqlAPI is our minimal Cloud SQL surface. Returns all instances flattened
// across pages so the caller doesn't need to know about pageToken plumbing.
type sqlAPI interface {
	ListInstances(ctx context.Context, projectID string) ([]*sqladmin.DatabaseInstance, error)
	Close() error
}

type realSQLClient struct {
	svc *sqladmin.Service
}

func (r *realSQLClient) ListInstances(ctx context.Context, projectID string) ([]*sqladmin.DatabaseInstance, error) {
	var out []*sqladmin.DatabaseInstance
	err := r.svc.Instances.List(projectID).Pages(ctx, func(page *sqladmin.InstancesListResponse) error {
		out = append(out, page.Items...)
		return nil
	})
	return out, err
}

// sqladmin's autogen Service has no Close — keep the interface symmetric with
// the rest of our clients by returning nil.
func (r *realSQLClient) Close() error { return nil }

type sqlFactory func(ctx context.Context, opts ...option.ClientOption) (sqlAPI, error)

type sqlClientState struct {
	once    sync.Once
	cli     sqlAPI
	err     error
	factory sqlFactory
}

func (p *GCPProvider) sqlClient(ctx context.Context) (sqlAPI, error) {
	p.sql.once.Do(func() {
		if p.sql.factory != nil {
			p.sql.cli, p.sql.err = p.sql.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.sql.err = fmt.Errorf("gcp: ADC for sql client: %w", err)
			return
		}
		svc, err := sqladmin.NewService(ctx, option.WithCredentials(creds))
		if err != nil {
			p.sql.err = fmt.Errorf("gcp: new sql admin service: %w", err)
			return
		}
		p.sql.cli = &realSQLClient{svc: svc}
	})
	if p.sql.err != nil {
		return nil, p.sql.err
	}
	return p.sql.cli, nil
}

func (p *GCPProvider) closeSQLClient() error {
	if p.sql.cli == nil {
		return nil
	}
	return p.sql.cli.Close()
}

func enrichDatabases(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	sc, err := p.sqlClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: sql client: %w", err)})
		return
	}
	insts, err := sc.ListInstances(ctx, scope.ID)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list sql instances: %w", err)})
		return
	}
	for _, inst := range insts {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildDatabaseResource(scope.ID, inst, p.dumpNative)})
	}
}

func buildDatabaseResource(scopeID string, inst *sqladmin.DatabaseInstance, dumpNative bool) inventory.Resource {
	settings := inst.Settings
	detail := inventory.DatabaseDetail{
		Engine: normaliseEngine(inst.DatabaseVersion),
	}
	if settings != nil {
		detail.Tier = settings.Tier
		detail.StorageGB = settings.DataDiskSizeGb
		detail.StorageType = settings.DataDiskType
		detail.HighAvailability = settings.AvailabilityType == "REGIONAL"
		if mw := settings.MaintenanceWindow; mw != nil {
			detail.MaintenanceWindow = formatMaintenanceWindow(mw)
		}
	}
	// VCPUs/MemoryMiB stay zero — Cloud SQL Tier→shape table is M8 polish
	// per the M6 plan; rendered as `—` in the TUI.

	region := inst.Region
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindDatabase, ID: inst.Name},
		Kind:   inventory.KindDatabase,
		Name:   inst.Name,
		Region: region,
		Status: inst.State,
		Detail: &detail,
		Native: nativeFrom(dumpNative, inst),
	}
}

// normaliseEngine turns "MYSQL_8_0" into "mysql-8.0", "POSTGRES_15" into
// "postgres-15", etc., matching the architecture's example values.
func normaliseEngine(version string) string {
	if version == "" {
		return ""
	}
	v := strings.ToLower(version)
	parts := strings.SplitN(v, "_", 2)
	if len(parts) != 2 {
		return v
	}
	return parts[0] + "-" + strings.ReplaceAll(parts[1], "_", ".")
}

func formatMaintenanceWindow(mw *sqladmin.MaintenanceWindow) string {
	if mw == nil {
		return ""
	}
	return fmt.Sprintf("day=%d hour=%d", mw.Day, mw.Hour)
}
