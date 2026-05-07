// Package components holds reusable Bubble Tea fragments used by multiple screens.
package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const separator = " › "

// Render joins a stack of screen titles into a single breadcrumb line and
// truncates to width with an ellipsis from the left so the most relevant
// (right-most) crumb is always visible.
func Render(titles []string, width int, dim, accent lipgloss.Style) string {
	if len(titles) == 0 || width <= 0 {
		return ""
	}
	parts := make([]string, len(titles))
	for i, t := range titles {
		if i == len(titles)-1 {
			parts[i] = accent.Render(t)
		} else {
			parts[i] = dim.Render(t)
		}
	}
	line := strings.Join(parts, dim.Render(separator))
	if lipgloss.Width(line) <= width {
		return line
	}
	plain := strings.Join(titles, separator)
	if width <= 1 {
		return "…"
	}
	keep := plain[len(plain)-(width-1):]
	return dim.Render("…") + accent.Render(keep)
}
