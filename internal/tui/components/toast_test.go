package components

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestToastQueuePushStacksFIFO(t *testing.T) {
	var q ToastQueue
	q.Push("first", time.Hour)
	q.Push("second", time.Hour)
	q.Push("third", time.Hour)

	out := q.View(lipgloss.NewStyle())
	lines := strings.Split(out, "\n")
	assert.Len(t, lines, 3)
	assert.Equal(t, "first", lines[0])
	assert.Equal(t, "second", lines[1])
	assert.Equal(t, "third", lines[2])
}

func TestToastQueueTickExpiresOldEntries(t *testing.T) {
	var q ToastQueue
	now := time.Now()
	q.Push("expired", -time.Second) // already past
	q.Push("alive", time.Hour)

	q.Tick(now)
	out := q.View(lipgloss.NewStyle())
	assert.Equal(t, "alive", out, "Tick should drop expired entries")
}

func TestToastQueueIsEmptyAndViewClosed(t *testing.T) {
	var q ToastQueue
	assert.True(t, q.IsEmpty())
	assert.Equal(t, "", q.View(lipgloss.NewStyle()))

	q.Push("x", time.Hour)
	assert.False(t, q.IsEmpty())
}
