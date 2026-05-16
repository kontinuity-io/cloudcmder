package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

// suggestion is one row in the dropdown below the input. Either an alias
// (kind="" — submit emits CmdSubmitMsg) or a resource (kind!="" — submit
// emits CmdJumpResourceMsg).
type suggestion struct {
	label string
	kind  inventory.Kind
	id    string
	hint  string // shown faintly to the right of the label (e.g., "vm")
}

// suggestionKey returns the stable identity of a suggestion. Resources are
// keyed by kind+id (label collisions across kinds are common for, e.g.,
// disk vs vm sharing a name); aliases are keyed by their label.
func (s suggestion) suggestionKey() string {
	if s.kind != "" {
		return string(s.kind) + ":" + s.id
	}
	return "alias:" + s.label
}

// visibleWindow is the number of suggestion rows rendered at once. The
// total render height is constant (1 input + visibleWindow slots) so the
// body's effective height doesn't shift while the user types — but the
// pool of matches kept can be much larger; arrow keys scroll a viewport
// across that pool.
const visibleWindow = 4

// maxFuzzyResults caps the underlying match pool. Keeping a larger
// reservoir than the visible window lets the user scroll into matches
// that didn't make the first screenful — the previous behaviour silently
// dropped any match past slot 4.
const maxFuzzyResults = 50

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

	// transient: rebuilt on every keystroke. suggestions holds up to
	// maxFuzzyResults matches; selected is the absolute index inside it.
	// offset is the start of the visibleWindow viewport — when the cursor
	// moves past the visible bottom or above the top, offset adjusts so
	// the highlighted entry is always on-screen.
	suggestions []suggestion
	selected    int
	offset      int
}

// NewCmdbar builds a cmdbar with sensible defaults. Corpus is empty until
// the App calls SetCorpus on the first Frame push.
func NewCmdbar(prompt, dim lipgloss.Style) Cmdbar {
	in := textinput.New()
	in.Prompt = ":"
	in.CharLimit = 64
	in.SetWidth(48)
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
	c.offset = 0
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
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(k, key.NewBinding(key.WithKeys("esc"))):
			c.Close()
			return c, nil
		case key.Matches(k, key.NewBinding(key.WithKeys("up"))):
			if c.selected > 0 {
				c.selected--
				if c.selected < c.offset {
					c.offset = c.selected
				}
			}
			return c, nil
		case key.Matches(k, key.NewBinding(key.WithKeys("down"))):
			if c.selected < len(c.suggestions)-1 {
				c.selected++
				if c.selected >= c.offset+visibleWindow {
					c.offset = c.selected - visibleWindow + 1
				}
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
//
// Selection is preserved across recomputes when the previously-selected
// entry is still in the new list — without this, every refining keystroke
// would yank the cursor back to the top mid-navigation.
func (c *Cmdbar) recomputeSuggestions() {
	pat := c.in.Value()
	// Only preserve selection across recomputes when the user has
	// actively navigated (selected > 0). At the default selected=0, the
	// "best match" identity drifts as the query refines — preserving it
	// would silently drag the cursor away from the new top match.
	prevKey := ""
	if c.selected > 0 && c.selected < len(c.suggestions) {
		prevKey = c.suggestions[c.selected].suggestionKey()
	}

	if pat == "" {
		c.suggestions = nil
		c.selected = 0
		c.offset = 0
		return
	}

	out := make([]suggestion, 0, maxFuzzyResults)

	for _, m := range fuzzy.Find(pat, c.aliases) {
		out = append(out, suggestion{label: c.aliases[m.Index], hint: "kind"})
		if len(out) >= maxFuzzyResults {
			break
		}
	}

	if len(out) < maxFuzzyResults {
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
			if len(out) >= maxFuzzyResults {
				break
			}
		}
	}

	c.suggestions = out
	c.selected = 0
	c.offset = 0
	if prevKey == "" {
		return
	}
	for i, s := range out {
		if s.suggestionKey() == prevKey {
			c.selected = i
			// Keep the previously-picked entry on-screen by sliding the
			// viewport so it lands inside the visibleWindow.
			if c.selected >= visibleWindow {
				c.offset = c.selected - visibleWindow + 1
			}
			return
		}
	}
}

// RenderHeight returns the constant vertical footprint of the cmdbar: 0
// when closed, 1+maxSuggestions when open. The constant is critical — if
// the height varied with the typed query, App would have to re-emit a
// WindowSizeMsg on every keystroke, cascading through Frame and both
// panes' tables. Padding the dropdown with empty lines keeps the body's
// effective height stable for the duration of a cmdbar session.
func (c Cmdbar) RenderHeight() int {
	if !c.open {
		return 0
	}
	return 1 + visibleWindow
}

// View renders the input line at the top (k9s-style header) followed by a
// fixed-height windowed suggestion dropdown. The window slides over a
// larger pool — match indicators (`↑`/`↓`) at the top/bottom signal that
// more entries exist above/below the visible slice. Total output is
// always RenderHeight() lines when open.
func (c Cmdbar) View() string {
	if !c.open {
		return ""
	}
	lines := make([]string, 0, 1+visibleWindow)
	lines = append(lines, c.prompt.Render(c.in.View()))
	end := c.offset + visibleWindow
	if end > len(c.suggestions) {
		end = len(c.suggestions)
	}
	for slot := 0; slot < visibleWindow; slot++ {
		i := c.offset + slot
		if i >= end {
			lines = append(lines, "")
			continue
		}
		s := c.suggestions[i]
		marker := "  "
		switch {
		case slot == 0 && c.offset > 0:
			// More matches above the visible window.
			marker = "↑ "
		case slot == visibleWindow-1 && end < len(c.suggestions):
			// More matches below the visible window.
			marker = "↓ "
		}
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
	return strings.Join(lines, "\n")
}
