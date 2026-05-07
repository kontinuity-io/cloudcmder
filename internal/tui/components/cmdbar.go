package components

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CmdSubmitMsg is emitted when the user presses Enter inside the cmdbar.
// Alias is the entered text without the leading colon (e.g., "vm", "disk").
type CmdSubmitMsg struct{ Alias string }

// Cmdbar is a single-line ":alias" input. Hidden by default; opened with `:`.
type Cmdbar struct {
	in     textinput.Model
	open   bool
	prompt lipgloss.Style
	dim    lipgloss.Style
}

// NewCmdbar builds a cmdbar with sensible defaults.
func NewCmdbar(prompt, dim lipgloss.Style) Cmdbar {
	in := textinput.New()
	in.Prompt = ":"
	in.CharLimit = 32
	in.Width = 32
	return Cmdbar{in: in, prompt: prompt, dim: dim}
}

func (c Cmdbar) IsOpen() bool { return c.open }

func (c *Cmdbar) Open() {
	c.open = true
	c.in.SetValue("")
	c.in.Focus()
}

func (c *Cmdbar) Close() {
	c.open = false
	c.in.Blur()
}

// Update handles keypresses while the cmdbar is open. Returns the cmdbar plus
// any tea.Cmd (e.g., a CmdSubmitMsg dispatch).
func (c Cmdbar) Update(msg tea.Msg) (Cmdbar, tea.Cmd) {
	if !c.open {
		return c, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, key.NewBinding(key.WithKeys("esc"))):
			c.Close()
			return c, nil
		case key.Matches(k, key.NewBinding(key.WithKeys("enter"))):
			alias := c.in.Value()
			c.Close()
			return c, func() tea.Msg { return CmdSubmitMsg{Alias: alias} }
		}
	}
	var cmd tea.Cmd
	c.in, cmd = c.in.Update(msg)
	return c, cmd
}

// View renders the cmdbar; returns "" when closed.
func (c Cmdbar) View() string {
	if !c.open {
		return ""
	}
	return c.prompt.Render(c.in.View())
}
