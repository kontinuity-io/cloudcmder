package gcp

import "strings"

// googleFreeProjects are GCP first-party image projects whose license URLs
// carry no ISV billing. Everything outside googleFreeProjects and
// googlePaidProjects is treated as a Marketplace ISV charge.
var googleFreeProjects = map[string]bool{
	"debian-cloud":       true,
	"ubuntu-os-cloud":    true,
	"cos-cloud":          true, // Container-Optimized OS
	"rocky-linux-cloud":  true,
	"fedora-cloud":       true,
	"centos-cloud":       true,
	"freebsd-org-cloud":  true,
	"gce-uefi-images":    true, // UEFI-compatible base images
	"confidential-vm-images": true,
}

// googlePaidProjects carry per-hour OS license fees billed by Google (RHEL,
// SLES, Windows) but are NOT Marketplace ISV products.
var googlePaidProjects = map[string]bool{
	"rhel-cloud":          true,
	"rhel-sap-cloud":      true,
	"suse-cloud":          true,
	"suse-sap-cloud":      true,
	"windows-cloud":       true,
	"windows-sql-cloud":   true,
	"ubuntu-os-pro-cloud": true,
}

// licenseProjectFromURL extracts the image-project segment from a GCP license
// URL. Accepts both the full googleapis.com form and the short path form:
//
//	"https://www.googleapis.com/compute/v1/projects/<PROJECT>/global/licenses/<NAME>"
//	"projects/<PROJECT>/global/licenses/<NAME>"
//
// Returns "" if the URL does not contain a recognisable /projects/<project>/
// segment.
func licenseProjectFromURL(url string) string {
	// Strip optional query string — some older CAI payloads append "?key=val"
	if i := strings.IndexByte(url, '?'); i >= 0 {
		url = url[:i]
	}
	// Normalise: find the "projects/" segment regardless of leading content.
	// Use LastIndex so we always land on the innermost occurrence (e.g. the
	// googleapis.com full path has /projects/ in the middle).
	const needle = "projects/"
	idx := strings.LastIndex(url, needle)
	if idx < 0 {
		return ""
	}
	rest := url[idx+len(needle):]
	// rest = "<project>/global/licenses/<name>"
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		return ""
	}
	return rest[:slash]
}

// classifyLicenseProject returns "google-free", "google-paid", "marketplace",
// or "" (empty project or unknown shape). Decision rule:
//   - googleFreeProjects map → "google-free"
//   - googlePaidProjects map → "google-paid"
//   - non-empty otherwise   → "marketplace"
//   - empty                 → ""
func classifyLicenseProject(project string) string {
	if project == "" {
		return ""
	}
	if googleFreeProjects[project] {
		return "google-free"
	}
	if googlePaidProjects[project] {
		return "google-paid"
	}
	return "marketplace"
}

// licenseInfoFromURLs aggregates the license URLs from one or more disks and
// derives (names, project, class). The precedence rule is
// marketplace > google-paid > google-free — so a VM whose boot disk is
// Debian but has an attached F5 appliance disk classifies as "marketplace"
// with project "f5-7626-networks-public".
//
// names: last-segment of each unique URL.
// project: the image-project of the "winning" URL per the precedence rule.
// class: one of "marketplace", "google-paid", "google-free", or "" (none).
func licenseInfoFromURLs(urls []string) (names []string, project string, class string) {
	// Collect last-segment names (deduplicated) and track per-class candidates.
	seen := make(map[string]bool, len(urls))
	var marketplaceProject, paidProject, freeProject string

	for _, u := range urls {
		seg := lastSegment(strings.SplitN(u, "?", 2)[0])
		if seg != "" && !seen[seg] {
			seen[seg] = true
			names = append(names, seg)
		}
		proj := licenseProjectFromURL(u)
		cls := classifyLicenseProject(proj)
		switch cls {
		case "marketplace":
			if marketplaceProject == "" {
				marketplaceProject = proj
			}
		case "google-paid":
			if paidProject == "" {
				paidProject = proj
			}
		case "google-free":
			if freeProject == "" {
				freeProject = proj
			}
		}
	}

	switch {
	case marketplaceProject != "":
		return names, marketplaceProject, "marketplace"
	case paidProject != "":
		return names, paidProject, "google-paid"
	case freeProject != "":
		return names, freeProject, "google-free"
	default:
		return names, "", ""
	}
}
