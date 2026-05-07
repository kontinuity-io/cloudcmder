package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/components"
	"cloudcmder.com/internal/tui/core"
	"cloudcmder.com/internal/tui/screens"
	"cloudcmder.com/internal/tui/style"
	"cloudcmder.com/internal/version"
)

type clearToastMsg struct{}

// App is the root Bubble Tea model. The screen stack lives here; everything
// else (cmdbar, help, toast) wraps the active screen's view.
type App struct {
	ctx     context.Context
	st      *store.Store
	stack   []core.Screen
	width   int
	height  int
	keymap  Keymap
	cmdbar  components.Cmdbar
	help    components.Help
	helpOn  bool
	toast   string
	version string
}

// Run launches the TUI with the given store. Blocks until the user quits.
func Run(ctx context.Context, st *store.Store) error {
	// Force lipgloss to render hex colours as 24-bit truecolor regardless of
	// what termenv auto-detects from the terminal profile. Without this,
	// terminals with custom 256-colour palettes can quantise our Tokyo-Night
	// hex tokens to washed-out approximations of whatever the user's theme
	// maps them to.
	lipgloss.DefaultRenderer().SetColorProfile(termenv.TrueColor)

	app := newApp(ctx, st)
	prog := tea.NewProgram(app, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := prog.Run()
	return err
}

func newApp(ctx context.Context, st *store.Store) App {
	app := App{
		ctx:     ctx,
		st:      st,
		keymap:  DefaultKeymap(),
		cmdbar:  components.NewCmdbar(style.Accent, style.Dim),
		help:    components.NewHelp(),
		version: version.String(),
	}
	app.stack = []core.Screen{screens.NewScopes(ctx, st)}
	return app
}

// Init kicks off the first screen.
func (a App) Init() tea.Cmd { return a.stack[0].Init() }

// Update is the message router. Order matters: cmdbar consumes keys when open,
// then app-level messages, then global keys, then delegate to the top screen.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.help.Width(m.Width)
		// fall through so screens see the resize too
	case core.PushScreenMsg:
		// Newly pushed screens don't get a WindowSizeMsg from Bubble Tea
		// automatically — the runtime only emits one on init/resize for the
		// program-root model. Without this synthesis, every pushed screen
		// renders at width=0 until the user resizes the terminal, which
		// makes width-aware layouts (Frame's split-pane mode) collapse to
		// their narrow fallback.
		s := m.Screen
		initCmd := s.Init()
		var sizeCmd tea.Cmd
		if a.width > 0 && a.height > 0 {
			updated, c := s.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			s = updated
			sizeCmd = c
		}
		a.stack = append(a.stack, s)
		return a, tea.Batch(initCmd, sizeCmd)
	case core.PopScreenMsg:
		if len(a.stack) <= 1 {
			return a, tea.Quit
		}
		a.stack = a.stack[:len(a.stack)-1]
		return a, nil
	case core.ToastMsg:
		a.toast = m.Text
		return a, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearToastMsg{} })
	case clearToastMsg:
		a.toast = ""
		return a, nil
	case components.CmdSubmitMsg:
		kind, ok := screens.AliasToKind(m.Alias)
		if !ok {
			return a, core.ToastCmd("unknown alias: " + m.Alias)
		}
		if a.findCurrentRun() == nil {
			return a, core.ToastCmd("no current run — open a scope first (got :" + m.Alias + ")")
		}
		// Frame on the stack will catch this and swap its left pane in place.
		return a, core.SwapLeftPaneCmd(kind)
	}

	if a.cmdbar.IsOpen() {
		var cmd tea.Cmd
		a.cmdbar, cmd = a.cmdbar.Update(msg)
		return a, cmd
	}

	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, a.keymap.Quit):
			return a, tea.Quit
		case key.Matches(k, a.keymap.Help):
			a.helpOn = !a.helpOn
			return a, nil
		case key.Matches(k, a.keymap.Cmd):
			a.cmdbar.Open()
			return a, nil
		case key.Matches(k, a.keymap.Back):
			if len(a.stack) <= 1 {
				return a, tea.Quit
			}
			a.stack = a.stack[:len(a.stack)-1]
			return a, nil
		}
	}

	top := a.stack[len(a.stack)-1]
	updated, cmd := top.Update(msg)
	a.stack[len(a.stack)-1] = updated
	return a, cmd
}

// findCurrentRun walks the screen stack top-down looking for a screen that
// implements core.RunOwner. Returns nil if no screen on the stack holds a
// run (e.g., the user invoked `:` from the ScopeList).
func (a App) findCurrentRun() *store.RunSummary {
	for i := len(a.stack) - 1; i >= 0; i-- {
		if owner, ok := a.stack[i].(core.RunOwner); ok {
			if r := owner.CurrentRun(); r != nil {
				return r
			}
		}
	}
	return nil
}

// View composes breadcrumb + screen body + status/help/cmdbar/toast lines.
func (a App) View() string {
	if len(a.stack) == 0 {
		return ""
	}
	titles := make([]string, len(a.stack))
	for i, s := range a.stack {
		titles[i] = s.Title()
	}
	crumbs := components.Render(titles, a.width, style.Dim, style.Accent)
	body := a.stack[len(a.stack)-1].View()

	footer := ""
	switch {
	case a.cmdbar.IsOpen():
		footer = a.cmdbar.View()
	case a.helpOn:
		footer = a.help.View(a.keymap)
	case a.toast != "":
		footer = style.Toast.Render(a.toast)
	default:
		footer = style.Dim.Render("? help · q quit · " + a.version)
	}

	return strings.Join([]string{crumbs, body, footer}, "\n")
}
