package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"cloudcmder.com/internal/inventory"
	"cloudcmder.com/internal/store"
	"cloudcmder.com/internal/tui/style"
)

// Scan progress message types — emitted by the scan goroutine into the tea.Program.
type (
	scopeStartMsg   struct{ idx int }
	scanProgressMsg struct {
		idx  int
		kind inventory.Kind
	}
	scopeDoneMsg struct {
		idx     int
		runUUID string
		err     error
	}
	allDoneMsg struct{}
)

type rowStatus int

const (
	statusQueued rowStatus = iota
	statusRunning
	statusOK
	statusFailed
)

type scanRow struct {
	scope      inventory.Scope
	status     rowStatus
	count      int
	activeKind inventory.Kind
	startedAt  time.Time
	finishedAt time.Time
	err        error
	runUUID    string
}

const recentCap = 5

type scanModel struct {
	rows       []scanRow
	spin       spinner.Model
	pb         progress.Model
	startedAt  time.Time
	width      int
	height     int
	activeIdx  int    // index of currently-running scope, -1 if none
	recent     []int  // ring of last recentCap completed row indexes
	failed     []string
	cancel     context.CancelFunc
	providerID string
}

func newScanModel(scopes []inventory.Scope, cancel context.CancelFunc, providerID string) scanModel {
	rows := make([]scanRow, len(scopes))
	for i, s := range scopes {
		rows[i] = scanRow{scope: s, status: statusQueued}
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	pb := progress.New(progress.WithDefaultBlend(), progress.WithoutPercentage())
	return scanModel{
		rows:      rows,
		spin:      sp,
		pb:        pb,
		startedAt:  time.Now(),
		activeIdx:  -1,
		cancel:     cancel,
		providerID: providerID,
	}
}

func (m scanModel) Init() tea.Cmd {
	return m.spin.Tick
}

func (m scanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.pb.SetWidth(barWidth(m.width))
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case scopeStartMsg:
		if msg.idx >= 0 && msg.idx < len(m.rows) {
			m.rows[msg.idx].status = statusRunning
			m.rows[msg.idx].startedAt = time.Now()
			m.activeIdx = msg.idx
		}
		return m, nil

	case scanProgressMsg:
		if msg.idx >= 0 && msg.idx < len(m.rows) {
			m.rows[msg.idx].count++
			m.rows[msg.idx].activeKind = msg.kind
		}
		return m, nil

	case scopeDoneMsg:
		if msg.idx >= 0 && msg.idx < len(m.rows) {
			m.rows[msg.idx].finishedAt = time.Now()
			m.rows[msg.idx].runUUID = msg.runUUID
			if msg.err != nil {
				m.rows[msg.idx].status = statusFailed
				m.rows[msg.idx].err = msg.err
				m.failed = append(m.failed, m.rows[msg.idx].scope.ID)
			} else {
				m.rows[msg.idx].status = statusOK
			}
			m.recent = append(m.recent, msg.idx)
			if len(m.recent) > recentCap {
				m.recent = m.recent[len(m.recent)-recentCap:]
			}
			if m.activeIdx == msg.idx {
				m.activeIdx = -1
			}
		}
		return m, nil

	case allDoneMsg:
		return m, tea.Quit

	case tea.KeyPressMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.cancel()
			return m, tea.Quit
		}
	}
	return m, nil
}

// barWidth returns the progress bar character width for a given terminal width.
func barWidth(termWidth int) int {
	if termWidth == 0 {
		return 30
	}
	w := termWidth/2 - 4
	if w < 10 {
		return 10
	}
	if w > 40 {
		return 40
	}
	return w
}

// tailBudget returns the max number of recent-completion rows to render given
// the current terminal height. Returns recentCap when height is unknown (0).
func (m scanModel) tailBudget() int {
	if m.height == 0 {
		return recentCap
	}
	// fixed chrome: banner(6) blank(1) header(1) blank(1) bar(1) blank(1) active(1) blank+heading(2) blank(1) footer(1) = 16
	// plus 1 guard for terminal prompt = 17 reserved
	avail := m.height - 17
	if avail < 1 {
		return 1
	}
	if avail > recentCap {
		return recentCap
	}
	return avail
}

// truncScopeID truncates a scope ID to maxLen chars with a trailing ellipsis.
func truncScopeID(id string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 40
	}
	if len(id) > maxLen {
		return id[:maxLen-1] + "…"
	}
	return id
}

func (m scanModel) View() tea.View {
	elapsed := time.Since(m.startedAt).Truncate(time.Second)
	var sb strings.Builder

	okStyle  := lipgloss.NewStyle().Foreground(style.ColorHealthy)
	errStyle := lipgloss.NewStyle().Foreground(style.ColorError)

	// Provider brand banner
	if banner := providerBanner(m.providerID); banner != "" {
		sb.WriteString(banner + "\n\n")
	}

	// Count per-status
	var okN, failN, runN, queueN int
	for _, r := range m.rows {
		switch r.status {
		case statusOK:
			okN++
		case statusFailed:
			failN++
		case statusRunning:
			runN++
		case statusQueued:
			queueN++
		}
	}
	total := len(m.rows)

	// Header
	fmt.Fprintf(&sb, "scanning %d scope(s) — %s elapsed\n\n", total, elapsed)

	// Progress bar + aggregate counts
	pct := 0.0
	if total > 0 {
		pct = float64(okN+failN) / float64(total)
	}
	barStr := m.pb.ViewAs(pct)
	done := okN + failN
	counts := fmt.Sprintf("%s  %d/%d", barStr, done, total)
	if okN > 0 {
		counts += "   " + okStyle.Render(fmt.Sprintf("ok %d", okN))
	}
	if failN > 0 {
		counts += "   " + errStyle.Render(fmt.Sprintf("fail %d", failN))
	}
	if runN > 0 {
		counts += fmt.Sprintf("   running %d", runN)
	}
	if queueN > 0 {
		counts += "   " + style.Dim.Render(fmt.Sprintf("queued %d", queueN))
	}
	sb.WriteString("  " + counts + "\n")

	// Active scope row (blank line when nothing running)
	maxScopeW := 40
	if m.width > 0 {
		if w := m.width / 3; w < maxScopeW {
			maxScopeW = w
		}
	}
	sb.WriteString("\n")
	if m.activeIdx >= 0 && m.activeIdx < len(m.rows) {
		r := m.rows[m.activeIdx]
		kindHint := "scanning"
		if r.activeKind != "" {
			kindHint = "..." + string(r.activeKind)
		}
		dur := time.Since(r.startedAt).Truncate(time.Second)
		fmt.Fprintf(&sb, "  %s  %-*s    %-24s  %s\n",
			m.spin.View(), maxScopeW, truncScopeID(r.scope.ID, maxScopeW),
			kindHint, style.Dim.Render(dur.String()))
	} else {
		sb.WriteString("\n")
	}

	// Recent completions tail
	budget := m.tailBudget()
	tail := m.recent
	if len(tail) > budget {
		tail = tail[len(tail)-budget:]
	}
	if len(tail) > 0 {
		sb.WriteString("\n  recently completed:\n")
		for _, idx := range tail {
			r := m.rows[idx]
			sid := truncScopeID(r.scope.ID, maxScopeW)
			switch r.status {
			case statusOK:
				dur := r.finishedAt.Sub(r.startedAt).Truncate(time.Second)
				fmt.Fprintf(&sb, "    %s  %-*s  %5d resources   %s\n",
					okStyle.Render("✓"), maxScopeW, sid, r.count, style.Dim.Render(dur.String()))
			case statusFailed:
				errMsg := "failed"
				if r.err != nil {
					errMsg = r.err.Error()
					if len(errMsg) > 60 {
						errMsg = errMsg[:57] + "…"
					}
				}
				fmt.Fprintf(&sb, "    %s  %-*s  %s\n",
					errStyle.Render("✗"), maxScopeW, sid, errStyle.Render(errMsg))
			}
		}
	}

	// Footer
	fmt.Fprintf(&sb, "\n   %d/%d done   %s", done, total, style.Dim.Render("q quit"))

	return tea.NewView(sb.String())
}

// runScanManyUI runs the Bubble Tea scan progress view for a multi-scope scan.
// Scopes are scanned sequentially; the model updates live as each scope
// progresses. The provider must already have SetDumpNative configured.
func runScanManyUI(
	ctx context.Context,
	st *store.Store,
	p inventory.Provider,
	scopes []inventory.Scope,
	failFast bool,
) error {
	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	m := newScanModel(scopes, cancel, p.Name())
	prog := tea.NewProgram(m, tea.WithContext(ctx))

	go func() {
		for i, scope := range scopes {
			if scanCtx.Err() != nil {
				break
			}
			prog.Send(scopeStartMsg{idx: i})
			runUUID, err := scanOneScope(scanCtx, st, p, scope, func(kind inventory.Kind) {
				prog.Send(scanProgressMsg{idx: i, kind: kind})
			})
			prog.Send(scopeDoneMsg{idx: i, runUUID: runUUID, err: err})
			if err != nil && failFast {
				cancel()
				break
			}
		}
		prog.Send(allDoneMsg{})
	}()

	final, err := prog.Run()
	if err != nil {
		return fmt.Errorf("scan-ui: %w", err)
	}
	fm, ok := final.(scanModel)
	if !ok || len(fm.failed) == 0 {
		return nil
	}
	return fmt.Errorf("scan-all: %d/%d scope(s) failed: %s",
		len(fm.failed), len(fm.rows), strings.Join(fm.failed, ", "))
}
