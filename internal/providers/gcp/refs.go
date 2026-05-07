package gcp

import (
	"strings"

	"cloudcmder.com/internal/inventory"
)

// vmDiskRef parses a GCP disk source URL into a Disk ResourceRef.
// Returns the zero ResourceRef when sourceURL is empty so callers can skip.
// e.g. "projects/p/zones/us-central1-a/disks/my-disk" → ref{Kind:Disk, ID:my-disk}.
func vmDiskRef(scopeID, sourceURL string) inventory.ResourceRef {
	name := lastSegment(sourceURL)
	if name == "" {
		return inventory.ResourceRef{}
	}
	return inventory.ResourceRef{
		Provider: providerName, ScopeID: scopeID, Kind: inventory.KindDisk, ID: name,
	}
}

// vmSubnetRef parses a GCP subnetwork URL into a Subnet ResourceRef.
func vmSubnetRef(scopeID, subnetURL string) inventory.ResourceRef {
	name := lastSegment(subnetURL)
	if name == "" {
		return inventory.ResourceRef{}
	}
	return inventory.ResourceRef{
		Provider: providerName, ScopeID: scopeID, Kind: inventory.KindSubnet, ID: name,
	}
}

// parseLicenseURL extracts the license name from a GCP licenses URL.
// e.g. ".../licenses/debian-11" → "debian-11". Drops any query string first.
func parseLicenseURL(url string) string {
	if i := strings.Index(url, "?"); i >= 0 {
		url = url[:i]
	}
	return lastSegment(url)
}
