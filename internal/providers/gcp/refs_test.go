package gcp

import (
	"testing"

	"cloudcmder.com/internal/inventory"
)

func TestVMDiskRef(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{name: "full URL", url: "https://www.googleapis.com/compute/v1/projects/p/zones/us-central1-a/disks/my-disk", want: "my-disk"},
		{name: "relative", url: "projects/p/zones/us-central1-a/disks/another", want: "another"},
		{name: "empty", url: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := vmDiskRef("p1", tc.url)
			if got.ID != tc.want {
				t.Errorf("ID = %q, want %q", got.ID, tc.want)
			}
			if tc.want != "" {
				if got.Kind != inventory.KindDisk || got.Provider != "gcp" || got.ScopeID != "p1" {
					t.Errorf("ref = %+v", got)
				}
			}
		})
	}
}

func TestVMSubnetRef(t *testing.T) {
	got := vmSubnetRef("p1",
		"https://www.googleapis.com/compute/v1/projects/p/regions/us-central1/subnetworks/default-uc1")
	if got.ID != "default-uc1" || got.Kind != inventory.KindSubnet {
		t.Errorf("ref = %+v", got)
	}
	if vmSubnetRef("p1", "").ID != "" {
		t.Errorf("expected zero ref for empty URL")
	}
}

func TestParseLicenseURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{name: "full URL", url: "https://www.googleapis.com/compute/v1/projects/debian-cloud/global/licenses/debian-11", want: "debian-11"},
		{name: "with query string", url: "https://x/licenses/ubuntu-2204?token=abc", want: "ubuntu-2204"},
		{name: "empty", url: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseLicenseURL(tc.url); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
