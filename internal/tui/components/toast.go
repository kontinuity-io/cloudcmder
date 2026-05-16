package components

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// toastEntry is one queued message with its expiry timestamp. Internal —
// callers only see ToastQueue's API.
type toastEntry struct {
	text    string
	expires time.Time
}

// ToastQueue holds a list of transient messages each with an independent
// TTL. Replaces the single App.toast string so back-to-back actions
// (multiple `e` exports, repeated toasts) stack instead of clobbering.
type ToastQueue struct {
	entries []toastEntry
}

// Push appends a new toast with the given TTL. Order is FIFO; render
// order is oldest-first (top), most-recent at the bottom.
func (q *ToastQueue) Push(text string, ttl time.Duration) {
	q.entries = append(q.entries, toastEntry{
		text:    text,
		expires: time.Now().Add(ttl),
	})
}

// Tick prunes expired entries. Cheap to call from the App's expiry
// scheduler — re-uses the slice header (zero allocations on the steady
// state).
func (q *ToastQueue) Tick(now time.Time) {
	out := q.entries[:0]
	for _, e := range q.entries {
		if e.expires.After(now) {
			out = append(out, e)
		}
	}
	q.entries = out
}

// IsEmpty reports whether the queue has any live entries. Used by App to
// decide between rendering the toast stack and the default version footer.
func (q ToastQueue) IsEmpty() bool {
	return len(q.entries) == 0
}

// View renders one entry per line, oldest first. Style is applied per-line.
func (q ToastQueue) View(style lipgloss.Style) string {
	if len(q.entries) == 0 {
		return ""
	}
	lines := make([]string, len(q.entries))
	for i, e := range q.entries {
		lines[i] = style.Render(e.text)
	}
	return strings.Join(lines, "\n")
}
