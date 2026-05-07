// Package version exposes build-time version information set via -ldflags.
package version

import "fmt"

// Set by goreleaser at link time:
//   -X cloudcmder.com/internal/version.Version=v1.0.0
//   -X cloudcmder.com/internal/version.Commit=abc1234
var (
    Version = "dev"
    Commit  = "none"
)

// String returns the human-readable version string shown in --version output.
func String() string {
    return fmt.Sprintf("cloudcmder %s (commit: %s)", Version, Commit)
}
