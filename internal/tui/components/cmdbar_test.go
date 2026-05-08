package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloudcmder.com/internal/inventory"
)

func newTestCmdbar() Cmdbar {
	return NewCmdbar(lipgloss.NewStyle(), lipgloss.NewStyle())
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeInto(c Cmdbar, text string) Cmdbar {
	for _, r := range text {
		c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return c
}

func TestCmdbarAliasFuzzyTopHit(t *testing.T) {
	c := newTestCmdbar()
	c.SetCorpus([]string{"vm", "bucket", "disk"}, nil)
	c.Open()

	c = typeInto(c, "buc")
	require.NotEmpty(t, c.suggestions)
	assert.Equal(t, "bucket", c.suggestions[0].label)
	assert.Equal(t, inventory.Kind(""), c.suggestions[0].kind, "alias suggestions carry no kind")
}

func TestCmdbarAliasEnterEmitsCmdSubmit(t *testing.T) {
	c := newTestCmdbar()
	c.SetCorpus([]string{"vm", "bucket"}, nil)
	c.Open()

	c = typeInto(c, "vm")
	c, cmd := c.Update(keyMsg("enter"))
	require.NotNil(t, cmd)
	got := cmd()
	sub, ok := got.(CmdSubmitMsg)
	require.True(t, ok, "expected CmdSubmitMsg, got %T", got)
	assert.Equal(t, "vm", sub.Alias)
	assert.False(t, c.IsOpen(), "Enter should close the cmdbar")
}

func TestCmdbarResourceFuzzyEmitsJump(t *testing.T) {
	c := newTestCmdbar()
	c.SetCorpus(
		[]string{"vm", "bucket"},
		[]ResourceEntry{
			{Kind: inventory.KindVM, ID: "my-prod-vm-1", Name: "my-prod-vm-1"},
			{Kind: inventory.KindVM, ID: "vm-staging", Name: "vm-staging"},
			{Kind: inventory.KindBucket, ID: "logs-bucket", Name: "logs-bucket"},
		},
	)
	c.Open()

	c = typeInto(c, "my-prod")
	require.NotEmpty(t, c.suggestions)
	// First suggestion should be the prod VM resource (alias "vm" wouldn't
	// fuzzy-match "my-prod" so resource hits dominate the dropdown).
	first := c.suggestions[0]
	assert.Equal(t, "my-prod-vm-1", first.label)
	assert.Equal(t, inventory.KindVM, first.kind)
	assert.Equal(t, "my-prod-vm-1", first.id)

	_, cmd := c.Update(keyMsg("enter"))
	require.NotNil(t, cmd)
	got := cmd()
	jump, ok := got.(CmdJumpResourceMsg)
	require.True(t, ok, "expected CmdJumpResourceMsg, got %T", got)
	assert.Equal(t, inventory.KindVM, jump.Kind)
	assert.Equal(t, "my-prod-vm-1", jump.ID)
}

func TestCmdbarArrowKeysMoveSelection(t *testing.T) {
	c := newTestCmdbar()
	c.SetCorpus([]string{"vm", "subnet", "bucket", "disk"}, nil)
	c.Open()

	c = typeInto(c, "u") // matches "subnet", "bucket"
	require.GreaterOrEqual(t, len(c.suggestions), 2)
	assert.Equal(t, 0, c.selected)

	c, _ = c.Update(keyMsg("down"))
	assert.Equal(t, 1, c.selected)
	c, _ = c.Update(keyMsg("up"))
	assert.Equal(t, 0, c.selected)
}

func TestCmdbarEscClosesWithoutSubmit(t *testing.T) {
	c := newTestCmdbar()
	c.Open()
	c, cmd := c.Update(keyMsg("esc"))
	assert.False(t, c.IsOpen())
	assert.Nil(t, cmd)
}

func TestCmdbarSelectedSurvivesRefiningKeystrokes(t *testing.T) {
	c := newTestCmdbar()
	c.SetCorpus(
		nil,
		[]ResourceEntry{
			{Kind: inventory.KindVM, ID: "vm-prod-api", Name: "vm-prod-api"},
			{Kind: inventory.KindVM, ID: "vm-prod-db", Name: "vm-prod-db"},
			{Kind: inventory.KindVM, ID: "vm-prod-cache", Name: "vm-prod-cache"},
		},
	)
	c.Open()

	c = typeInto(c, "vm")
	require.GreaterOrEqual(t, len(c.suggestions), 3)
	c, _ = c.Update(keyMsg("down"))
	c, _ = c.Update(keyMsg("down"))
	require.Equal(t, 2, c.selected)
	picked := c.suggestions[c.selected]

	// Refining keystroke ("p" → narrows to all -prod-*) should keep the
	// cursor on the same resource if it's still in the new list, instead
	// of yanking back to selected=0.
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	found := -1
	for i, s := range c.suggestions {
		if s.suggestionKey() == picked.suggestionKey() {
			found = i
			break
		}
	}
	require.GreaterOrEqual(t, found, 0, "previously-selected suggestion should still be in the list")
	assert.Equal(t, found, c.selected, "selected should track the previously-picked entry")
}

func TestCmdbarRenderHeightTracksDropdown(t *testing.T) {
	c := newTestCmdbar()
	c.SetCorpus([]string{"vm", "bucket"}, nil)
	assert.Equal(t, 0, c.RenderHeight(), "closed cmdbar takes no vertical room")

	c.Open()
	c = typeInto(c, "v")
	// Open + 1 input line + N suggestions.
	assert.Equal(t, 1+len(c.suggestions), c.RenderHeight())

	c.Close()
	assert.Equal(t, 0, c.RenderHeight())
}

func TestCmdbarEmptySuggestionsFallsBackToAliasSubmit(t *testing.T) {
	c := newTestCmdbar()
	c.SetCorpus([]string{"vm"}, nil)
	c.Open()

	c = typeInto(c, "zzz") // matches nothing
	assert.Empty(t, c.suggestions)
	_, cmd := c.Update(keyMsg("enter"))
	got := cmd()
	sub, ok := got.(CmdSubmitMsg)
	require.True(t, ok, "fallback should be CmdSubmitMsg even with no suggestion")
	assert.Equal(t, "zzz", sub.Alias)
}
