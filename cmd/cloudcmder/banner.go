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
		_ = figurine.Write(&buf, text, "Cybersmall.flf")
		return strings.TrimRight(buf.String(), "\n")
	default:
		return text
	}
}
