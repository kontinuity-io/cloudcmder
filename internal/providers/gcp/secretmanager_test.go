package gcp

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"

	"cloudcmder.com/internal/inventory"
)

func TestSecretRotationPeriodDurations(t *testing.T) {
	cases := []struct {
		secs int64
		want string
	}{
		{30 * 24 * 3600, "30d"}, // 30 days
		{2 * 3600, "2h"},        // 2 hours (not a whole day)
		{90 * 60, "90m"},        // 90 minutes (not a whole hour)
		{45, "45s"},             // 45 seconds
		{7 * 24 * 3600, "7d"},   // 1 week → days
	}
	for _, c := range cases {
		rot := &secretmanagerpb.Rotation{RotationPeriod: durationpb.New(time.Duration(c.secs) * time.Second)}
		got := secretRotationPeriod(rot)
		if got != c.want {
			t.Errorf("secretRotationPeriod(%ds) = %q, want %q", c.secs, got, c.want)
		}
	}
}

func TestSecretReplication(t *testing.T) {
	// Automatic (global) → no region.
	auto := &secretmanagerpb.Replication{
		Replication: &secretmanagerpb.Replication_Automatic_{
			Automatic: &secretmanagerpb.Replication_Automatic{},
		},
	}
	if pol, region := secretReplication(auto); pol != "automatic" || region != "" {
		t.Errorf("automatic = (%q,%q), want (automatic,\"\")", pol, region)
	}

	// User-managed → first replica location.
	um := &secretmanagerpb.Replication{
		Replication: &secretmanagerpb.Replication_UserManaged_{
			UserManaged: &secretmanagerpb.Replication_UserManaged{
				Replicas: []*secretmanagerpb.Replication_UserManaged_Replica{
					{Location: "us-east1"},
					{Location: "europe-west4"},
				},
			},
		},
	}
	if pol, region := secretReplication(um); pol != "user-managed" || region != "us-east1" {
		t.Errorf("user-managed = (%q,%q), want (user-managed,us-east1)", pol, region)
	}

	// nil replication → empty.
	if pol, region := secretReplication(nil); pol != "" || region != "" {
		t.Errorf("nil = (%q,%q), want (\"\",\"\")", pol, region)
	}
}

func TestSecretRotationAndTopic(t *testing.T) {
	if got := secretRotationPeriod(nil); got != "" {
		t.Errorf("nil rotation = %q, want empty", got)
	}
	rot := &secretmanagerpb.Rotation{RotationPeriod: durationpb.New(30 * 24 * time.Hour)}
	if got := secretRotationPeriod(rot); got != "30d" {
		t.Errorf("rotation period = %q, want 30d", got)
	}
	// Rotation present but no period.
	if got := secretRotationPeriod(&secretmanagerpb.Rotation{}); got != "" {
		t.Errorf("rotation no-period = %q, want empty", got)
	}

	if got := firstTopic(nil); got != "" {
		t.Errorf("firstTopic(nil) = %q, want empty", got)
	}
	topics := []*secretmanagerpb.Topic{
		{Name: "projects/p/topics/rotate-me"},
		{Name: "projects/p/topics/other"},
	}
	if got := firstTopic(topics); got != "rotate-me" {
		t.Errorf("firstTopic = %q, want rotate-me", got)
	}
}

func TestBuildSecretManagerResource(t *testing.T) {
	s := smSecret{
		ID:               "db-password",
		Replication:      "user-managed",
		Region:           "us-east1",
		ActiveVersions:   3,
		RotationPeriod:   "30d",
		RotationTopic:    "rotate-me",
		AccessOperations: 42,
	}
	r := buildSecretManagerResource("p1", s)

	if r.Kind != inventory.KindGCPSecretManager || r.Name != "db-password" || r.Region != "us-east1" {
		t.Fatalf("resource header = %+v", r)
	}
	if r.Ref.ID != "db-password" {
		t.Errorf("ref id = %q, want db-password (must match CAI stub id to overwrite)", r.Ref.ID)
	}
	sd, ok := r.Detail.(*inventory.SecretManagerDetail)
	if !ok {
		t.Fatalf("detail type = %T", r.Detail)
	}
	if sd.Subtype != "Secret" || sd.Replication != "user-managed" || sd.Region != "us-east1" {
		t.Errorf("detail = %+v", sd)
	}
	if sd.ActiveVersions != 3 || sd.RotationPeriod != "30d" || sd.RotationTopic != "rotate-me" || sd.AccessOperations != 42 {
		t.Errorf("detail = %+v", sd)
	}
}

// countEnabledVersions logic mirror: an enricher-side helper would loop the
// SDK iterator; here we assert the ENABLED-only counting rule against the
// proto state enum so a future SDK bump that renames the constant is caught.
func TestEnabledVersionCounting(t *testing.T) {
	versions := []*secretmanagerpb.SecretVersion{
		{State: secretmanagerpb.SecretVersion_ENABLED},
		{State: secretmanagerpb.SecretVersion_DISABLED},
		{State: secretmanagerpb.SecretVersion_ENABLED},
		{State: secretmanagerpb.SecretVersion_DESTROYED},
		{State: secretmanagerpb.SecretVersion_STATE_UNSPECIFIED},
	}
	var n int
	for _, v := range versions {
		if v.GetState() == secretmanagerpb.SecretVersion_ENABLED {
			n++
		}
	}
	if n != 2 {
		t.Errorf("enabled count = %d, want 2", n)
	}
}

// --- fake secretManagerAPI -------------------------------------------------

type fakeSecretManagerClient struct {
	secrets []smSecret
	err     error
}

func (f *fakeSecretManagerClient) ListSecrets(_ context.Context, _ string) ([]smSecret, error) {
	return f.secrets, f.err
}

func (f *fakeSecretManagerClient) Close() error { return nil }

func TestEnrichSecretManager(t *testing.T) {
	p := &GCPProvider{}
	p.secretManager.factory = func(_ context.Context, _ ...option.ClientOption) (secretManagerAPI, error) {
		return &fakeSecretManagerClient{secrets: []smSecret{
			{ID: "api-key", Replication: "automatic", ActiveVersions: 1},
			{ID: "db-password", Replication: "user-managed", Region: "us-east1", ActiveVersions: 2, RotationPeriod: "30d", RotationTopic: "rotate-me"},
		}}, nil
	}

	ch := make(chan inventory.ResourceOrErr, 8)
	enrichSecretManager(context.Background(), p, inventory.Scope{ID: "p1"}, ch)
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
	if got[0].Name != "api-key" || got[1].Name != "db-password" {
		t.Errorf("names = %q, %q", got[0].Name, got[1].Name)
	}
	d0 := got[0].Detail.(*inventory.SecretManagerDetail)
	if d0.Replication != "automatic" || d0.Region != "" || d0.ActiveVersions != 1 {
		t.Errorf("api-key detail = %+v", d0)
	}
	d1 := got[1].Detail.(*inventory.SecretManagerDetail)
	if d1.Replication != "user-managed" || d1.Region != "us-east1" || d1.RotationPeriod != "30d" || d1.RotationTopic != "rotate-me" {
		t.Errorf("db-password detail = %+v", d1)
	}
}
