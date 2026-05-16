package main

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	figure "github.com/common-nighthawk/go-figure"

	"cloudcmder.com/internal/tui/style"
)

// providerBanner returns a 3-row ANSI-styled wordmark for the given provider
// ID (e.g. "gcp" → coloured "GCP" with a ☁ icon prefix). Returns the
// providerID upper-cased on a single line for unknown providers.
func providerBanner(providerID string) string {
	switch providerID {
	case "gcp":
		return joinIconWordmark(style.GCPBlue, "GCP",
			[]color.Color{style.GCPBlue, style.GCPRed, style.GCPYellow})
	case "aws":
		return joinIconWordmark(style.AWSOrange, "AWS",
			[]color.Color{style.AWSOrange})
	default:
		return strings.ToUpper(providerID)
	}
}

func joinIconWordmark(iconColor color.Color, text string, letterColors []color.Color) string {
	icon := lipgloss.NewStyle().Foreground(iconColor).Render(" ☁ \n   \n   ")
	wm := coloredWordmark(text, letterColors)
	return lipgloss.JoinHorizontal(lipgloss.Top, icon, wm)
}

func coloredWordmark(text string, colors []color.Color) string {
	parts := make([]string, 0, len(text))
	for i, r := range text {
		c := colors[i%len(colors)]
		letter := figure.NewFigure(string(r), "threepoint", true).String()
		letter = strings.TrimRight(letter, "\n")
		parts = append(parts, lipgloss.NewStyle().Foreground(c).Render(letter))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
