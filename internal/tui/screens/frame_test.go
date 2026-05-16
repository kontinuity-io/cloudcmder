package screens

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/core"
)

func TestFrameEscBehaviorByContext(t *testing.T) {
	cases := []struct {
		name       string
		zoomed     bool
		historyLen int
		wantPopMsg bool // true => Cmd resolves to core.PopScreenMsg
		wantZoomed bool // expected f.zoomed AFTER the Esc
	}{
		{"root pane → pop screen", false, 0, true, false},
		{"with history → pop pane, not screen", false, 1, false, false},
		{"zoomed → un-zoom, not pop", true, 0, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := openMemStoreT(t)
			f := NewFrame(context.Background(), st, store.RunSummary{})
			f.zoomed = tc.zoomed
			for i := 0; i < tc.historyLen; i++ {
				f.leftHistory = append(f.leftHistory, f.left)
			}

			_, cmd := f.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

			if tc.wantPopMsg {
				require.NotNil(t, cmd, "root-pane Esc must emit a PopScreenCmd")
				_, ok := cmd().(core.PopScreenMsg)
				assert.True(t, ok, "root-pane Esc must resolve to PopScreenMsg")
			} else if cmd != nil {
				_, isPop := cmd().(core.PopScreenMsg)
				assert.False(t, isPop, "non-root Esc must not pop the Frame")
			}
			assert.Equal(t, tc.wantZoomed, f.zoomed)
		})
	}
}
