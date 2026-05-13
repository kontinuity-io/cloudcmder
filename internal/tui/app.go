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

// toastExpireMsg fires once per Toast TTL; carries the timestamp so the
// queue can prune anything that's expired by then. time.Time directly so
// the message has zero overhead beyond the wall clock.
type toastExpireMsg time.Time

const toastTTL = 3 * time.Second

// App is the root Bubble Tea model. The screen stack lives here; everything
// else (cmdbar, help, toast) wraps the active screen's view.
type App struct {
	ctx       context.Context
	st        *store.Store
	stack     []core.Screen
	width     int
	height    int
	keymap    Keymap
	cmdbar    components.Cmdbar
	statusbar components.Statusbar
	help      components.Help
	helpOn    bool
	toasts    components.ToastQueue
	version   string

	// lastBodyShrink is the number of vertical lines the cmdbar (when open)
	// is currently asking the body to give up. Tracked so syncBodyShrink
	// only re-emits a synthesized WindowSizeMsg on actual transitions —
	// since RenderHeight is now constant while the cmdbar is open, the
	// state is binary (0 ↔ 1+maxSuggestions) and emit fires twice per
	// cmdbar session.
	lastBodyShrink int
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

// RunSingleView is the alternative entry point used when `--single-view` is
// passed on the CLI. Identical to Run() but seeds the stack with the
// SingleView 4-pane layout instead of the standard Scopes picker.
func RunSingleView(ctx context.Context, st *store.Store) error {
	lipgloss.DefaultRenderer().SetColorProfile(termenv.TrueColor)
	app := newSingleViewApp(ctx, st)
	prog := tea.NewProgram(app, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := prog.Run()
	return err
}

// newAppCore builds the App with all chrome (cmdbar, statusbar, help, etc.)
// but no root screen on the stack. The two entry-point constructors below
// add the appropriate root.
func newAppCore(ctx context.Context, st *store.Store) App {
	return App{
		ctx:    ctx,
		st:     st,
		keymap: DefaultKeymap(),
		cmdbar: components.NewCmdbar(style.Accent, style.Dim),
		help:   components.NewHelp(),
		statusbar: components.NewStatusbar(
			style.Accent,
			style.Dim,
			lipgloss.NewStyle().Foreground(style.ColorHealthy),
			lipgloss.NewStyle().Foreground(style.ColorWarning),
			lipgloss.NewStyle().Foreground(style.ColorError),
		),
		version: version.String(),
	}
}

func newApp(ctx context.Context, st *store.Store) App {
	app := newAppCore(ctx, st)
	app.stack = []core.Screen{screens.NewScopes(ctx, st)}
	return app
}

func newSingleViewApp(ctx context.Context, st *store.Store) App {
	app := newAppCore(ctx, st)
	app.stack = []core.Screen{screens.NewSingleView(ctx, st)}
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
		a.lastBodyShrink = a.cmdbar.RenderHeight()
		// Reserve App-level chrome before forwarding to the active screen
		// — without this, Frame thinks it owns the full terminal and the
		// crumb + statusbar + footer push the top of the body off-screen.
		forwarded := tea.WindowSizeMsg{Width: m.Width, Height: a.bodyBudget()}
		top := a.stack[len(a.stack)-1]
		updated, cmd := top.Update(forwarded)
		a.stack[len(a.stack)-1] = updated
		return a, cmd
	case core.PushScreenMsg:
		// Newly pushed screens don't get a WindowSizeMsg from Bubble Tea
		// automatically — the runtime only emits one on init/resize for the
		// program-root model. Without this synthesis, every pushed screen
		// renders at width=0 until the user resizes the terminal, which
		// makes width-aware layouts (Frame's split-pane mode) collapse to
		// their narrow fallback.
		s := m.Screen
		// Push first so bodyBudget can see the new screen when computing
		// the statusbar reservation (RunOwner contributes a line).
		a.stack = append(a.stack, s)
		initCmd := s.Init()
		var sizeCmd tea.Cmd
		if a.width > 0 && a.height > 0 {
			updated, c := s.Update(tea.WindowSizeMsg{Width: a.width, Height: a.bodyBudget()})
			a.stack[len(a.stack)-1] = updated
			sizeCmd = c
		}
		// Refresh the cmdbar's fuzzy corpus whenever a RunOwner lands on
		// top — keeps the `:` palette pointing at the active run's resources.
		if owner, ok := a.stack[len(a.stack)-1].(core.RunOwner); ok {
			if r := owner.CurrentRun(); r != nil {
				a.refreshCmdbarCorpus(*r)
			}
		}
		return a, tea.Batch(initCmd, sizeCmd)
	case core.PopScreenMsg:
		if len(a.stack) <= 1 {
			return a, tea.Quit
		}
		a.stack = a.stack[:len(a.stack)-1]
		return a, nil
	case core.SwitchRunMsg:
		// A modal (RunHistory or ScopesModal) is asking the Frame
		// underneath to swap its run. Refresh the cmdbar corpus first so
		// `:` after the swap points at the new scope's resources, THEN
		// forward to Frame so it can swap in place.
		a.refreshCmdbarCorpus(m.Run)
		top := a.stack[len(a.stack)-1]
		updated, cmd := top.Update(msg)
		a.stack[len(a.stack)-1] = updated
		return a, cmd
	case core.ToastMsg:
		a.toasts.Push(m.Text, toastTTL)
		// One expiry tick per push — multiple stacked toasts each get
		// their own scheduled prune, and Tick is idempotent so duplicate
		// fires are harmless.
		return a, tea.Tick(toastTTL, func(t time.Time) tea.Msg {
			return toastExpireMsg(t)
		})
	case toastExpireMsg:
		a.toasts.Tick(time.Time(m))
		return a, nil
	case components.CmdSubmitMsg:
		// `:scopes` is a special-case alias that doesn't map to a Kind —
		// open the ScopeList as a modal over the current Frame.
		if strings.EqualFold(m.Alias, "scopes") {
			return a, core.PushScreenCmd(screens.NewScopesModal(a.ctx, a.st))
		}
		kind, ok := screens.AliasToKind(m.Alias)
		if !ok {
			return a, core.ToastCmd("unknown alias: " + m.Alias)
		}
		if a.findCurrentRun() == nil {
			return a, core.ToastCmd("no current run — open a scope first (got :" + m.Alias + ")")
		}
		// Frame on the stack will catch this and swap its left pane in place.
		return a, core.SwapLeftPaneCmd(kind)
	case components.CmdJumpResourceMsg:
		if a.findCurrentRun() == nil {
			return a, core.ToastCmd("no current run — open a scope first")
		}
		// Single atomic message — Frame builds the new ResourceList and
		// sets pendingJumpID before Init, so the cursor positions on the
		// matched row as soon as the async load completes. tea.Batch can't
		// be used here because its commands fire concurrently — the jump
		// would race ahead and arrive at the OLD pane.
		return a, core.SwapAndJumpCmd(m.Kind, m.ID)
	}

	// Cmdbar gets first crack at any keystroke when open — so the user can
	// type queries containing `q`, `?`, etc., without Quit/Help intercepts
	// stealing them. Esc and Enter inside the cmdbar always close cleanly
	// (constant-height invariant means no stuck-cmdbar trap).
	if a.cmdbar.IsOpen() {
		var cmd tea.Cmd
		a.cmdbar, cmd = a.cmdbar.Update(msg)
		// The cmdbar may have closed via Esc/Enter; rebalance the body's
		// effective height if so. RenderHeight is constant while open, so
		// this is a no-op during typing and only fires on the close edge.
		sizeCmd := a.syncBodyShrink()
		return a, tea.Batch(cmd, sizeCmd)
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
			sizeCmd := a.syncBodyShrink()
			return a, sizeCmd
		}
		// Esc is intentionally NOT handled here. Each screen (Frame,
		// RunHistory, GraphView) decides what Esc means in its own
		// context — Frame walks pane history; modals close themselves
		// via core.PopScreenCmd. Eating Esc at the App layer would
		// short-circuit Frame's pane-history navigation.
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

// bodyBudget returns the vertical room available to the active screen
// once App-level chrome (crumbs, cmdbar, statusbar, footer) is reserved.
// Single source of truth so the WindowSizeMsg path and the cmdbar
// open/close path produce identical sizes — preventing one-off rendering
// glitches when the cmdbar toggles.
func (a App) bodyBudget() int {
	chrome := 1 + a.cmdbar.RenderHeight() + 1 // crumbs + cmdbar + footer
	if a.statusbarLine() != "" {
		chrome++
	}
	h := a.height - chrome
	if h < 5 {
		h = 5
	}
	return h
}

// syncBodyShrink emits a WindowSizeMsg to the top screen whenever the
// cmdbar transitions between closed and open states. RenderHeight is
// constant while the cmdbar is open (1+maxSuggestions), so this only
// fires on the open and close edges — never on every keystroke. The
// keystroke cascade was the original cause of the unresponsive TUI in
// the failed move-to-top attempt (commit 8d055af).
func (a *App) syncBodyShrink() tea.Cmd {
	if a.width == 0 || a.height == 0 || len(a.stack) == 0 {
		return nil
	}
	shrink := a.cmdbar.RenderHeight()
	if shrink == a.lastBodyShrink {
		return nil
	}
	a.lastBodyShrink = shrink
	top := a.stack[len(a.stack)-1]
	updated, cmd := top.Update(tea.WindowSizeMsg{
		Width:  a.width,
		Height: a.bodyBudget(),
	})
	a.stack[len(a.stack)-1] = updated
	return cmd
}

// refreshCmdbarCorpus reloads the cmdbar's fuzzy corpus for the given run.
// Failures degrade gracefully: the alias tier still works, only the
// resource-jump tier is missing.
func (a *App) refreshCmdbarCorpus(run store.RunSummary) {
	idx, err := a.st.LoadResourceIndex(a.ctx, run.ID)
	if err != nil {
		a.cmdbar.SetCorpus(screens.AllAliases(), nil)
		return
	}
	entries := make([]components.ResourceEntry, len(idx))
	for i, e := range idx {
		entries[i] = components.ResourceEntry{Kind: e.Kind, ID: e.ID, Name: e.Name}
	}
	a.cmdbar.SetCorpus(screens.AllAliases(), entries)
}

// View composes breadcrumb + (cmdbar when open) + screen body + footer.
// Cmdbar renders ABOVE the body — k9s-style — with a constant footprint
// so the body's effective height stays stable for the duration of a
// cmdbar session (height is updated via syncBodyShrink only on open/close
// transitions).
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
	case a.helpOn:
		footer = a.help.View(a.keymap)
	case !a.toasts.IsEmpty():
		footer = a.toasts.View(style.Toast)
	default:
		footer = style.Dim.Render("? help · q quit · " + a.version)
	}

	parts := []string{crumbs}
	if a.cmdbar.IsOpen() {
		parts = append(parts, a.cmdbar.View())
	}
	parts = append(parts, body)
	if bar := a.statusbarLine(); bar != "" {
		parts = append(parts, bar)
	}
	parts = append(parts, footer)
	return strings.Join(parts, "\n")
}

// statusbarLine returns the rendered status bar when the active screen is
// a RunOwner — otherwise empty (e.g., on ScopeList where no run is active).
func (a App) statusbarLine() string {
	for i := len(a.stack) - 1; i >= 0; i-- {
		owner, ok := a.stack[i].(core.RunOwner)
		if !ok {
			continue
		}
		if owner.CurrentRun() == nil {
			continue
		}
		snap := owner.StatusbarData()
		return a.statusbar.View(components.StatusbarData{
			ScopeID:        snap.ScopeID,
			RunUUIDShort:   snap.RunUUIDShort,
			RunStatus:      snap.RunStatus,
			TotalResources: snap.TotalResources,
			KindCount:      snap.KindCount,
			StartedAt:      snap.StartedAt,
		})
	}
	return ""
}
