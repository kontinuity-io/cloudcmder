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
