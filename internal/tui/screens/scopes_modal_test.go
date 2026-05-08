package screens

import (
	"context"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
)

func openMemStoreT(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestNewScopesModalIsModalFlagged(t *testing.T) {
	s := NewScopesModal(context.Background(), openMemStoreT(t))
	assert.True(t, s.modal)
	assert.Equal(t, "Scopes (modal)", s.Title())
}

func TestScopesModalEscPopsScreen(t *testing.T) {
	s := NewScopesModal(context.Background(), openMemStoreT(t))
	updated, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)
	got := cmd()
	_, ok := got.(core.PopScreenMsg)
	assert.True(t, ok, "Esc in modal should emit PopScreenMsg, got %T", got)
	_ = updated
}

func TestScopesNonModalIgnoresEsc(t *testing.T) {
	// At the root, Scopes leaves Esc alone so future bindings can claim it.
	s := NewScopes(context.Background(), openMemStoreT(t))
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	// May or may not produce a cmd via the table, but it must NOT be a
	// PopScreenMsg — that would crash the program at the root screen.
	if cmd != nil {
		got := cmd()
		// Anything other than PopScreenMsg is fine.
		_, isPop := got.(core.PopScreenMsg)
		assert.False(t, isPop, "non-modal Scopes must not pop on Esc; got %s", reflect.TypeOf(got))
	}
}
