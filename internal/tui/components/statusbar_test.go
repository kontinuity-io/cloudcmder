package components

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func newTestStatusbar() Statusbar {
	plain := lipgloss.NewStyle()
	return NewStatusbar(plain, plain, plain, plain, plain)
}

func TestStatusbarRendersAllFields(t *testing.T) {
	s := newTestStatusbar()
	out := s.View(StatusbarData{
		ScopeID:        "fazalullah-lab",
		RunUUIDShort:   "17a233a4",
		RunStatus:      "ok",
		TotalResources: 80,
		KindCount:      8,
		StartedAt:      time.Now().Add(-2 * time.Hour),
	})
	for _, want := range []string{"fazalullah-lab", "17a233a4", "ok", "80 resources / 8 kinds", "2h ago"} {
		assert.Contains(t, out, want)
	}
}

func TestRelativeTimeBuckets(t *testing.T) {
	now := time.Now()
	cases := []struct {
		t    time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-50 * time.Hour), "2d ago"},
	}
	for _, tc := range cases {
		got := relativeTime(tc.t)
		assert.Equal(t, tc.want, got, "input %v", tc.t)
	}

	// Zero time → em dash.
	assert.Equal(t, "—", relativeTime(time.Time{}))
}

func TestStatusbarStatusStyling(t *testing.T) {
	healthy := lipgloss.NewStyle().Foreground(lipgloss.Color("#00d4aa"))
	dim := lipgloss.NewStyle()
	err := lipgloss.NewStyle().Foreground(lipgloss.Color("#e74c3c"))
	s := NewStatusbar(dim, dim, healthy, dim, err)

	// Render "ok" twice — once via the styler, once direct — and confirm
	// the status segment carries the healthy ANSI escape.
	out := s.View(StatusbarData{ScopeID: "p", RunUUIDShort: "abc12345", RunStatus: "ok", StartedAt: time.Now()})
	wantPrefix := healthy.Render("ok")
	assert.True(t, strings.Contains(out, wantPrefix), "ok status should carry healthy style; got %q", out)
}
