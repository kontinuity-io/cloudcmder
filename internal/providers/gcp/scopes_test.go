package gcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

func TestListScopes(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		wantIDs  []string
		wantErr  bool
		wantSize int
	}{
		{
			name: "empty result",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			},
			wantIDs: nil,
		},
		{
			name: "filters out non-active projects",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// state: 1 = ACTIVE, 2 = DELETE_REQUESTED. The REST client requests
				// $alt=json;enum-encoding=int so int values are the canonical form.
				_, _ = w.Write([]byte(`{
					"projects": [
						{"name":"projects/100","projectId":"alpha","displayName":"Alpha","state":1,"parent":"folders/1","labels":{"env":"prod"}},
						{"name":"projects/200","projectId":"beta","displayName":"Beta","state":2}
					]
				}`))
			},
			wantIDs: []string{"alpha"},
		},
		{
			name: "stitches paginated pages",
			handler: pagedHandler(
				`{"projects":[{"projectId":"p1","displayName":"P1","state":1}],"nextPageToken":"tok2"}`,
				`{"projects":[{"projectId":"p2","displayName":"P2","state":1}]}`,
			),
			wantIDs: []string{"p1", "p2"},
		},
		{
			name: "surfaces server errors",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)

			ctx := context.Background()
			p, err := New(ctx,
				option.WithEndpoint(srv.URL),
				option.WithoutAuthentication(),
				option.WithHTTPClient(srv.Client()),
			)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(func() { _ = p.Close() })

			got, err := p.ListScopes(ctx)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ListScopes: %v", err)
			}

			gotIDs := make([]string, len(got))
			for i, s := range got {
				gotIDs[i] = s.ID
				if s.ProviderID != providerName {
					t.Errorf("scope[%d].ProviderID = %q, want %q", i, s.ProviderID, providerName)
				}
			}
			if !equalStrings(gotIDs, tc.wantIDs) {
				t.Errorf("scope ids = %v, want %v", gotIDs, tc.wantIDs)
			}

			if tc.name == "filters out non-active projects" && len(got) == 1 {
				if got[0].DisplayName != "Alpha" || got[0].Parent != "folders/1" || got[0].Labels["env"] != "prod" {
					t.Errorf("alpha mapping wrong: %+v", got[0])
				}
			}
		})
	}
}

// pagedHandler returns the responses in order, advancing on each request.
// Subsequent calls (beyond the supplied list) return the last page so a buggy
// iterator does not deadlock the test.
func pagedHandler(pages ...string) http.HandlerFunc {
	var calls int
	return func(w http.ResponseWriter, r *http.Request) {
		idx := calls
		if idx >= len(pages) {
			idx = len(pages) - 1
		}
		calls++

		// pageToken correctness check: the second request must include it.
		if idx > 0 && !strings.Contains(r.URL.RawQuery, "pageToken=") {
			http.Error(w, "missing pageToken on follow-up", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(pages[idx]))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Compile-time assurance that GCPProvider satisfies the inventory.Provider
// interface. Failing this catches drift before runtime.
var _ inventory.Provider = (*GCPProvider)(nil)
