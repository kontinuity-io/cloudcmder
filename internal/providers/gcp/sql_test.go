package gcp

import (
	"context"
	"testing"

	sqladmin "google.golang.org/api/sqladmin/v1beta4"

	"cloudcmder.com/internal/inventory"
)

func TestNormaliseEngine(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"MYSQL_8_0", "mysql-8.0"},
		{"POSTGRES_15", "postgres-15"},
		{"SQLSERVER_2019_WEB", "sqlserver-2019.web"},
		{"", ""},
		{"MYSQL", "mysql"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normaliseEngine(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildDatabaseResource(t *testing.T) {
	inst := &sqladmin.DatabaseInstance{
		Name:            "my-db",
		Region:          "us-central1",
		State:           "RUNNABLE",
		DatabaseVersion: "POSTGRES_15",
		Settings: &sqladmin.Settings{
			Tier:             "db-custom-2-7680",
			DataDiskSizeGb:   100,
			DataDiskType:     "PD_SSD",
			AvailabilityType: "REGIONAL",
		},
	}
	r := buildDatabaseResource("p1", inst, false)
	if r.Ref.String() != "gcp:p1:Database:my-db" {
		t.Errorf("ref = %s", r.Ref.String())
	}
	d := r.Detail.(*inventory.DatabaseDetail)
	if d.Engine != "postgres-15" || d.Tier != "db-custom-2-7680" || d.StorageGB != 100 ||
		d.StorageType != "PD_SSD" || !d.HighAvailability {
		t.Errorf("detail = %+v", d)
	}
}

// --- fake SQL client -------------------------------------------------------

type fakeSQLClient struct {
	items []*sqladmin.DatabaseInstance
	err   error
}

func (f *fakeSQLClient) ListInstances(_ context.Context, _ string) ([]*sqladmin.DatabaseInstance, error) {
	return f.items, f.err
}

func (f *fakeSQLClient) Close() error { return nil }
