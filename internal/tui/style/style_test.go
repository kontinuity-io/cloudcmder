package style

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

func TestStatusBulletKnownStatusesProduceDistinctOutputs(t *testing.T) {
	// Force a true-color profile so lipgloss actually emits ANSI codes —
	// the default renderer detects no-tty during `go test` and strips
	// colours, which would make every bullet identical and defeat the
	// test.
	prev := lipgloss.DefaultRenderer().ColorProfile()
	lipgloss.DefaultRenderer().SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.DefaultRenderer().SetColorProfile(prev) })

	healthy := StatusBullet("RUNNING")
	error_ := StatusBullet("TERMINATED")
	warn := StatusBullet("PARTIAL")
	unknown := StatusBullet("???WHATEVER")

	for _, b := range []string{healthy, error_, warn, unknown} {
		assert.Contains(t, b, "●")
		assert.NotEqual(t, "●", b, "should carry an ANSI colour wrapper, not bare glyph")
	}
	// Pairwise distinct — different status colours render different ANSI.
	assert.NotEqual(t, healthy, error_)
	assert.NotEqual(t, healthy, warn)
	assert.NotEqual(t, error_, unknown)
}

func TestStatusBulletEmptyIn(t *testing.T) {
	assert.Equal(t, "", StatusBullet(""))
}
