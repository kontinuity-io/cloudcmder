package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLicenseProjectFromURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{
			"full googleapis URL",
			"https://www.googleapis.com/compute/v1/projects/debian-cloud/global/licenses/debian-11-bullseye",
			"debian-cloud",
		},
		{
			"short form",
			"projects/f5-7626-networks-public/global/licenses/f5-bigip-best",
			"f5-7626-networks-public",
		},
		{
			"URL with query string",
			"projects/rhel-cloud/global/licenses/rhel-8?key=val",
			"rhel-cloud",
		},
		{
			"empty",
			"",
			"",
		},
		{
			"no projects segment",
			"https://example.com/something/else",
			"",
		},
		{
			"malformed — project with no slash after",
			"projects/onlyone",
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, licenseProjectFromURL(tc.url))
		})
	}
}

func TestClassifyLicenseProject(t *testing.T) {
	cases := []struct {
		name    string
		project string
		want    string
	}{
		{"debian-cloud is free", "debian-cloud", "google-free"},
		{"ubuntu-os-cloud is free", "ubuntu-os-cloud", "google-free"},
		{"cos-cloud is free", "cos-cloud", "google-free"},
		{"rocky-linux-cloud is free", "rocky-linux-cloud", "google-free"},
		{"rhel-cloud is paid", "rhel-cloud", "google-paid"},
		{"rhel-sap-cloud is paid", "rhel-sap-cloud", "google-paid"},
		{"suse-cloud is paid", "suse-cloud", "google-paid"},
		{"windows-cloud is paid", "windows-cloud", "google-paid"},
		{"windows-sql-cloud is paid", "windows-sql-cloud", "google-paid"},
		{"ubuntu-os-pro-cloud is paid", "ubuntu-os-pro-cloud", "google-paid"},
		{"f5 is marketplace", "f5-7626-networks-public", "marketplace"},
		{"snowflake is marketplace", "snowflake-public", "marketplace"},
		{"fortigate is marketplace", "fortigcp-project-001", "marketplace"},
		{"unknown non-empty is marketplace", "some-isv-corp", "marketplace"},
		{"empty is empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyLicenseProject(tc.project))
		})
	}
}

func TestLicenseInfoFromURLs(t *testing.T) {
	debianURL := "projects/debian-cloud/global/licenses/debian-11"
	f5URL := "projects/f5-7626-networks-public/global/licenses/f5-bigip-best"
	rhelURL := "projects/rhel-cloud/global/licenses/rhel-8"

	cases := []struct {
		name        string
		urls        []string
		wantProject string
		wantClass   string
		wantNames   []string
	}{
		{
			"empty slice",
			nil,
			"", "",
			nil,
		},
		{
			"single google-free",
			[]string{debianURL},
			"debian-cloud", "google-free",
			[]string{"debian-11"},
		},
		{
			"single marketplace",
			[]string{f5URL},
			"f5-7626-networks-public", "marketplace",
			[]string{"f5-bigip-best"},
		},
		{
			"marketplace wins over free (any-marketplace-wins precedence)",
			[]string{debianURL, f5URL},
			"f5-7626-networks-public", "marketplace",
			[]string{"debian-11", "f5-bigip-best"},
		},
		{
			"paid wins over free",
			[]string{debianURL, rhelURL},
			"rhel-cloud", "google-paid",
			[]string{"debian-11", "rhel-8"},
		},
		{
			"marketplace wins over paid",
			[]string{rhelURL, f5URL},
			"f5-7626-networks-public", "marketplace",
			[]string{"rhel-8", "f5-bigip-best"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			names, project, class := licenseInfoFromURLs(tc.urls)
			assert.Equal(t, tc.wantProject, project)
			assert.Equal(t, tc.wantClass, class)
			assert.Equal(t, tc.wantNames, names)
		})
	}
}
