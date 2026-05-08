package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestAppQuitAlwaysQuitsEvenWhenCmdbarOpen(t *testing.T) {
	app := newApp(context.Background(), openMemStore(t))

	// Sanity: q quits with cmdbar closed.
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)
	assert.Equal(t, tea.Quit(), cmd(), "q should quit when cmdbar is closed")

	// Open the cmdbar and confirm q STILL quits — it must not be trapped
	// inside the textinput when the user wants out.
	app.cmdbar.Open()
	require.True(t, app.cmdbar.IsOpen())
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)
	assert.Equal(t, tea.Quit(), cmd(), "q must still quit while cmdbar is open")
}

func TestAppCtrlCQuits(t *testing.T) {
	app := newApp(context.Background(), openMemStore(t))
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
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
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	app = model.(App)
	assert.True(t, app.cmdbar.IsOpen())
	openedShrink := app.lastBodyShrink
	assert.Greater(t, openedShrink, 0, "open should bump shrink")

	// Type three characters into cmdbar — shrink must NOT change.
	for _, r := range []rune{'v', 'm', 's'} {
		model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		app = model.(App)
		assert.Equal(t, openedShrink, app.lastBodyShrink,
			"shrink must stay constant while typing (no per-keystroke cascade)")
	}

	// Close via Esc — shrink should drop back to 0.
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = model.(App)
	assert.False(t, app.cmdbar.IsOpen())
	assert.Equal(t, 0, app.lastBodyShrink, "close should reset shrink")
}
