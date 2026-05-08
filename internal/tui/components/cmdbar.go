package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"cloudcmder.com/internal/inventory"
)

// CmdSubmitMsg is emitted when the user picks an alias suggestion (or types
// raw text matching no resource). Alias is the entered text without the
// leading colon (e.g., "vm", "disk").
type CmdSubmitMsg struct{ Alias string }

// CmdJumpResourceMsg is emitted when the user picks a resource suggestion.
// The App turns this into a Frame swap (to Kind's ResourceList) plus a
// JumpToResourceCmd that positions the cursor on ID once the new pane loads.
type CmdJumpResourceMsg struct {
	Kind inventory.Kind
	ID   string
}

// ResourceEntry is the cmdbar's view of a Resource — just enough to fuzzy
// match its name and dispatch a jump. Loaded by App via store.LoadResourceIndex
// when a Frame is pushed onto the stack.
type ResourceEntry struct {
	Kind inventory.Kind
	ID   string
	Name string
}

// suggestion is one row in the dropdown above the input. Either an alias
// (kind="" — submit emits CmdSubmitMsg) or a resource (kind!="" — submit
// emits CmdJumpResourceMsg).
type suggestion struct {
	label  string
	kind   inventory.Kind
	id     string
	hint   string // shown faintly to the right of the label (e.g., "vm")
}

// maxSuggestions caps the dropdown so a 1000-resource project doesn't
// stretch the palette across the screen. The user keeps typing to narrow.
const maxSuggestions = 6

// Cmdbar is the `:` palette: a single-line input with a fuzzy suggestion
// dropdown above it. Suggestions span both kind aliases and live resources
// in the current run.
type Cmdbar struct {
	in     textinput.Model
	open   bool
	prompt lipgloss.Style
	dim    lipgloss.Style

	// corpus
	aliases   []string
	resources []ResourceEntry

	// transient: rebuilt on every keystroke
	suggestions []suggestion
	selected    int
}

// NewCmdbar builds a cmdbar with sensible defaults. Corpus is empty until
// the App calls SetCorpus on the first Frame push.
func NewCmdbar(prompt, dim lipgloss.Style) Cmdbar {
	in := textinput.New()
	in.Prompt = ":"
	in.CharLimit = 64
	in.Width = 48
	return Cmdbar{in: in, prompt: prompt, dim: dim}
}

func (c Cmdbar) IsOpen() bool { return c.open }

func (c *Cmdbar) Open() {
	c.open = true
	c.in.SetValue("")
	c.in.Focus()
	c.recomputeSuggestions()
}

func (c *Cmdbar) Close() {
	c.open = false
	c.in.Blur()
	c.suggestions = nil
	c.selected = 0
}

// SetCorpus replaces the alias and resource search targets. Cheap to call
// every time the active run changes; the next Open() rebuilds suggestions.
func (c *Cmdbar) SetCorpus(aliases []string, resources []ResourceEntry) {
	c.aliases = aliases
	c.resources = resources
}

// Update handles keypresses while the cmdbar is open.
func (c Cmdbar) Update(msg tea.Msg) (Cmdbar, tea.Cmd) {
	if !c.open {
		return c, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, key.NewBinding(key.WithKeys("esc"))):
			c.Close()
			return c, nil
		case key.Matches(k, key.NewBinding(key.WithKeys("up"))):
			if c.selected > 0 {
				c.selected--
			}
			return c, nil
		case key.Matches(k, key.NewBinding(key.WithKeys("down"))):
			if c.selected < len(c.suggestions)-1 {
				c.selected++
			}
			return c, nil
		case key.Matches(k, key.NewBinding(key.WithKeys("enter"))):
			cmd := c.commit()
			c.Close()
			return c, cmd
		}
	}
	var cmd tea.Cmd
	c.in, cmd = c.in.Update(msg)
	c.recomputeSuggestions()
	return c, cmd
}

// commit returns the tea.Cmd appropriate for the highlighted suggestion. If
// the dropdown is empty (user typed gibberish), fall back to the legacy
// alias path so :foo still emits a CmdSubmitMsg the App can toast as
// "unknown alias: foo".
func (c Cmdbar) commit() tea.Cmd {
	if len(c.suggestions) == 0 {
		alias := c.in.Value()
		return func() tea.Msg { return CmdSubmitMsg{Alias: alias} }
	}
	s := c.suggestions[c.selected]
	if s.kind != "" {
		return func() tea.Msg { return CmdJumpResourceMsg{Kind: s.kind, ID: s.id} }
	}
	return func() tea.Msg { return CmdSubmitMsg{Alias: s.label} }
}

// recomputeSuggestions rebuilds the dropdown after every keystroke. Aliases
// and resource names are fuzzy-matched against the typed pattern; results
// are merged with aliases first (kind keywords are usually shorter and
// almost always what the user meant when typing 2-3 chars).
func (c *Cmdbar) recomputeSuggestions() {
	pat := c.in.Value()
	c.selected = 0
	if pat == "" {
		c.suggestions = nil
		return
	}

	out := make([]suggestion, 0, maxSuggestions)

	for _, m := range fuzzy.Find(pat, c.aliases) {
		out = append(out, suggestion{label: c.aliases[m.Index], hint: "kind"})
		if len(out) >= maxSuggestions {
			c.suggestions = out
			return
		}
	}

	if len(out) < maxSuggestions {
		names := make([]string, len(c.resources))
		for i, r := range c.resources {
			names[i] = r.Name
		}
		for _, m := range fuzzy.Find(pat, names) {
			r := c.resources[m.Index]
			out = append(out, suggestion{
				label: r.Name,
				kind:  r.Kind,
				id:    r.ID,
				hint:  string(r.Kind),
			})
			if len(out) >= maxSuggestions {
				break
			}
		}
	}

	c.suggestions = out
}

// View renders the dropdown stacked above the input line. Empty when closed.
func (c Cmdbar) View() string {
	if !c.open {
		return ""
	}
	lines := make([]string, 0, len(c.suggestions)+1)
	for i, s := range c.suggestions {
		marker := "  "
		if i == c.selected {
			marker = "▸ "
		}
		row := marker + s.label
		if s.hint != "" {
			row += "  " + c.dim.Render("("+s.hint+")")
		}
		if i == c.selected {
			row = c.prompt.Render(row)
		} else {
			row = c.dim.Render(row)
		}
		lines = append(lines, row)
	}
	lines = append(lines, c.prompt.Render(c.in.View()))
	return strings.Join(lines, "\n")
}
