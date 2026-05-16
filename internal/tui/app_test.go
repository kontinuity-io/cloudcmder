package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/store"
)

// openMemStore returns a :memory: SQLite store; tests don't need real data,
// just a non-nil *store.Store to construct App.
func openMemStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestQQuitsOnlyWhenCmdbarClosed(t *testing.T) {
	// Locks in the fix for the "q exits while typing 'mysql' in :"
	// regression. q is a normal keymap binding, NOT a global preempt —
	// when the cmdbar is open, q types into the search input.
	app := newApp(context.Background(), openMemStore(t))

	// Cmdbar closed → q quits.
	_, cmd := app.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	require.NotNil(t, cmd)
	assert.Equal(t, tea.Quit(), cmd(), "q should quit when cmdbar is closed")

	// Cmdbar open → q must NOT quit; it lands on the textinput instead.
	updated, _ := app.Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	app = updated.(App)
	require.True(t, app.cmdbar.IsOpen(), "':' should have opened the cmdbar")

	updated, cmd = app.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	app = updated.(App)
	if cmd != nil {
		got := cmd()
		require.NotEqual(t, tea.Quit(), got,
			"q must NOT produce tea.Quit while cmdbar is open; got %T", got)
	}
	assert.True(t, app.cmdbar.IsOpen(), "cmdbar should still be open after typing q")
}

func TestAppCtrlCQuits(t *testing.T) {
	app := newApp(context.Background(), openMemStore(t))
	_, cmd := app.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'c'})
	require.NotNil(t, cmd)
	assert.Equal(t, tea.Quit(), cmd())
}

// TestCmdbarBodyShrinkOnlyFiresOnTransitions guards the load-bearing
// invariant from Batch 1: opening the cmdbar emits exactly one body
// resize, typing into it emits zero, closing it emits exactly one. If
// per-keystroke emits ever return, the original unresponsive-TUI bug
// (commit 8d055af) is back.
func TestCmdbarBodyShrinkOnlyFiresOnTransitions(t *testing.T) {
	app := newApp(context.Background(), openMemStore(t))
	// Seed App with a known terminal size so syncBodyShrink can compute.
	model, _ := app.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	app = model.(App)

	initialShrink := app.lastBodyShrink
	require.Equal(t, 0, initialShrink, "cmdbar starts closed")

	// Open via `:` — should bump lastBodyShrink to the constant.
	model, _ = app.Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	app = model.(App)
	assert.True(t, app.cmdbar.IsOpen())
	openedShrink := app.lastBodyShrink
	assert.Greater(t, openedShrink, 0, "open should bump shrink")

	// Type three characters into cmdbar — shrink must NOT change.
	for _, r := range []rune{'v', 'm', 's'} {
		model, _ = app.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		app = model.(App)
		assert.Equal(t, openedShrink, app.lastBodyShrink,
			"shrink must stay constant while typing (no per-keystroke cascade)")
	}

	// Close via Esc — shrink should drop back to 0.
	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	app = model.(App)
	assert.False(t, app.cmdbar.IsOpen())
	assert.Equal(t, 0, app.lastBodyShrink, "close should reset shrink")
}
