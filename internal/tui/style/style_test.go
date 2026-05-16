package style

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusBulletKnownStatusesProduceDistinctOutputs(t *testing.T) {
	// lipgloss v2 always emits ANSI escape codes regardless of terminal
	// profile — no forcing needed.

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
