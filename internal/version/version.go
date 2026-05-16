// Package version exposes build-time version information set via -ldflags.
package version

import (
	"fmt"
	"runtime"
)

// Set by goreleaser at link time:
//   -X cloudcmder.com/internal/version.Version=v1.0.0
//   -X cloudcmder.com/internal/version.Commit=abc1234
//   -X cloudcmder.com/internal/version.Date=2026-05-16T10:00:00Z
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info carries all build and runtime metadata surfaced by the version banner.
type Info struct {
	Version string
	Commit  string
	Date    string
	GoVer   string
	OS      string
	Arch    string
}

// Get returns a populated Info using the ldflag-injected vars plus runtime values.
func Get() Info {
	return Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
		GoVer:   runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}
}

// String returns the human-readable version string shown in --version output.
// Kept intentionally terse so cli_version rows in the store remain parseable.
func String() string {
	return fmt.Sprintf("cloudcmder %s (commit: %s)", Version, Commit)
}
