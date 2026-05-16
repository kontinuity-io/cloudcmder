package tui

import "charm.land/bubbles/v2/key"

// Keymap groups the global keybindings declared in architecture.md. Per-screen
// keymaps embed this and add their own (e.g. enter/h/j/k/l on lists).
type Keymap struct {
	Quit      key.Binding
	Back      key.Binding
	Help      key.Binding
	Filter    key.Binding
	Cmd       key.Binding
	Export    key.Binding
	History   key.Binding
	Rescan    key.Binding
}

// DefaultKeymap returns the bindings used everywhere unless a screen overrides them.
func DefaultKeymap() Keymap {
	return Keymap{
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Back:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Cmd:     key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "jump")),
		Export:  key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "export (M7)")),
		History: key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "run history")),
		Rescan:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "rescan (M8)")),
	}
}

// ShortHelp / FullHelp satisfy bubbles/help.KeyMap so the help component
// can render the bindings without each screen knowing about help internals.
func (k Keymap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Back, k.Quit}
}

func (k Keymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Help, k.Back, k.Quit},
		{k.Filter, k.Cmd, k.History},
		{k.Export, k.Rescan},
		// Advisory — actual handling lives in screens/detail.go.
		{
			key.NewBinding(key.WithKeys("shift+left", "shift+right"), key.WithHelp("⇧←/⇧→", "prev/next tab")),
			key.NewBinding(key.WithKeys("1", "2", "3", "4"), key.WithHelp("1–4", "jump to tab")),
			key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "cycle tabs (alias)")),
		},
	}
}
