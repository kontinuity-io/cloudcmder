package main

import (
	"os"
	"strings"

	"github.com/arsham/figurine/figurine"
)

// providerBanner returns an ANSI-styled block-font wordmark for the given
// provider ID rendered in figurine's rainbow gradient, matching the style of
// the cloudcmder version banner. Falls back to plain upper-cased text when
// NO_COLOR is set or the provider is unrecognised.
func providerBanner(providerID string) string {
	text := strings.ToUpper(providerID)
	switch providerID {
	case "gcp", "aws":
		if os.Getenv("NO_COLOR") != "" {
			return text
		}
		var buf strings.Builder
		_ = figurine.Write(&buf, text, "ANSI Regular.flf")
		// ANSI Regular renders 6 rows; keep only the first 4 for a compact banner.
		// Each line carries independent ANSI escape codes so line-splitting is safe.
		output := strings.TrimRight(buf.String(), "\n")
		lines := strings.Split(output, "\n")
		if len(lines) > 4 {
			lines = lines[:4]
		}
		return strings.Join(lines, "\n")
	default:
		return text
	}
}
