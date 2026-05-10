package gcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/serviceusage/v1"

	"cloudcmder.com/internal/inventory"
)

func TestRequiredAPIsCoversAssetTypeToKind(t *testing.T) {
	required := make(map[string]struct{})
	for _, a := range RequiredAPIs() {
		required[a] = struct{}{}
	}
	for at := range assetTypeToKind {
		i := strings.Index(at, "/")
		if i <= 0 {
			continue
		}
		api := at[:i]
		if _, ok := required[api]; !ok {
			t.Errorf("RequiredAPIs() missing %q (from assetTypeToKind key %q)", api, at)
		}
	}
}

func TestRequiredAPIsContainsAlwaysRequired(t *testing.T) {
	required := make(map[string]struct{})
	for _, a := range RequiredAPIs() {
		required[a] = struct{}{}
	}
	for _, want := range alwaysRequired {
		if _, ok := required[want]; !ok {
			t.Errorf("RequiredAPIs() missing always-required API %q", want)
		}
	}
}

func TestPreflightResultMissing(t *testing.T) {
	cases := []struct {
		name    string
		req     []string
		enabled []string
		want    []string
	}{
		{"all enabled", []string{"a.com", "b.com"}, []string{"a.com", "b.com"}, nil},
		{"one missing", []string{"a.com", "b.com"}, []string{"a.com"}, []string{"b.com"}},
		{"all missing", []string{"a.com", "b.com"}, nil, []string{"a.com", "b.com"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enabledSet := make(map[string]struct{}, len(tc.enabled))
			for _, s := range tc.enabled {
				enabledSet[s] = struct{}{}
			}
			var missing []string
			for _, a := range tc.req {
				if _, ok := enabledSet[a]; !ok {
					missing = append(missing, a)
				}
			}
			if len(missing) != len(tc.want) {
				t.Fatalf("missing = %v, want %v", missing, tc.want)
			}
			for i := range tc.want {
				if missing[i] != tc.want[i] {
					t.Errorf("missing[%d] = %q, want %q", i, missing[i], tc.want[i])
				}
			}
		})
	}
}

func TestPreflightResultGcloudEnableCommand(t *testing.T) {
	cases := []struct {
		name    string
		result  PreflightResult
		wantCmd string
	}{
		{
			"nothing missing",
			PreflightResult{Scope: inventory.Scope{ID: "my-proj"}, Missing: nil},
			"",
		},
		{
			"one missing",
			PreflightResult{
				Scope:   inventory.Scope{ID: "my-proj"},
				Missing: []string{"apigee.googleapis.com"},
			},
			"gcloud services enable apigee.googleapis.com --project=my-proj",
		},
		{
			"multiple missing",
			PreflightResult{
				Scope:   inventory.Scope{ID: "proj-123"},
				Missing: []string{"apigee.googleapis.com", "bigquery.googleapis.com"},
			},
			"gcloud services enable apigee.googleapis.com bigquery.googleapis.com --project=proj-123",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.result.GcloudEnableCommand()
			if got != tc.wantCmd {
				t.Errorf("GcloudEnableCommand() = %q, want %q", got, tc.wantCmd)
			}
		})
	}
}

func TestPreflightDiffsAgainstFakeClient(t *testing.T) {
	// Fake Service Usage API that returns a fixed set of enabled services.
	enabledAPIs := []string{
		"cloudasset.googleapis.com",
		"cloudresourcemanager.googleapis.com",
		"compute.googleapis.com",
		"serviceusage.googleapis.com",
		"storage.googleapis.com",
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var svcs []*serviceusage.GoogleApiServiceusageV1Service
		for _, api := range enabledAPIs {
			svcs = append(svcs, &serviceusage.GoogleApiServiceusageV1Service{
				Name:  "projects/my-proj/services/" + api,
				State: "ENABLED",
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"services": svcs})
	}))
	defer ts.Close()

	p := &GCPProvider{}
	p.suFactory = func(ctx context.Context) (*serviceusage.Service, error) {
		return serviceusage.NewService(ctx,
			option.WithEndpoint(ts.URL),
			option.WithoutAuthentication(),
		)
	}

	r, err := p.Preflight(context.Background(), inventory.Scope{ID: "my-proj", ProviderID: "gcp"})
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if len(r.Missing) == 0 {
		t.Error("expected some Missing APIs since not all are enabled in fake; got none")
	}
	// Verify that the five enabled APIs are NOT in Missing.
	missingSet := make(map[string]struct{}, len(r.Missing))
	for _, m := range r.Missing {
		missingSet[m] = struct{}{}
	}
	for _, api := range enabledAPIs {
		if _, ok := missingSet[api]; ok {
			t.Errorf("API %q reported as missing but was in enabled list", api)
		}
	}
	// GcloudEnableCommand should name only missing APIs and reference project.
	cmd := r.GcloudEnableCommand()
	if cmd == "" {
		t.Fatal("GcloudEnableCommand() empty but Missing is non-empty")
	}
	if !strings.Contains(cmd, "--project=my-proj") {
		t.Errorf("command missing --project= flag: %q", cmd)
	}
}
