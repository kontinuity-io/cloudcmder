package main

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"cloudcmder.com/internal/inventory"
)

var testScopes = []inventory.Scope{
	{ID: "scope-a", ProviderID: "gcp"},
	{ID: "scope-b", ProviderID: "gcp"},
	{ID: "scope-c", ProviderID: "gcp"},
}

func applyMsgs(m scanModel, msgs ...tea.Msg) scanModel {
	for _, msg := range msgs {
		updated, _ := m.Update(msg)
		m = updated.(scanModel)
	}
	return m
}

func TestScanModelScopeStart(t *testing.T) {
	m := newScanModel(testScopes, func() {})
	m = applyMsgs(m, scopeStartMsg{idx: 0})

	if m.rows[0].status != statusRunning {
		t.Errorf("rows[0].status = %v, want statusRunning", m.rows[0].status)
	}
	if m.rows[0].startedAt.IsZero() {
		t.Error("rows[0].startedAt not set after scopeStartMsg")
	}
	if m.activeIdx != 0 {
		t.Errorf("activeIdx = %d, want 0", m.activeIdx)
	}
	if m.rows[1].status != statusQueued {
		t.Errorf("rows[1].status = %v, want statusQueued (unaffected)", m.rows[1].status)
	}
}

func TestScanModelProgress(t *testing.T) {
	m := newScanModel(testScopes, func() {})
	m = applyMsgs(m,
		scopeStartMsg{idx: 0},
		scanProgressMsg{idx: 0, kind: inventory.KindVM},
		scanProgressMsg{idx: 0, kind: inventory.KindDisk},
		scanProgressMsg{idx: 0, kind: inventory.KindVM},
	)

	if m.rows[0].count != 3 {
		t.Errorf("count = %d, want 3", m.rows[0].count)
	}
	if m.rows[0].activeKind != inventory.KindVM {
		t.Errorf("activeKind = %q, want %q", m.rows[0].activeKind, inventory.KindVM)
	}
}

func TestScanModelScopeDoneOK(t *testing.T) {
	m := newScanModel(testScopes, func() {})
	m = applyMsgs(m,
		scopeStartMsg{idx: 0},
		scanProgressMsg{idx: 0, kind: inventory.KindVM},
		scopeDoneMsg{idx: 0, runUUID: "test-uuid", err: nil},
	)

	if m.rows[0].status != statusOK {
		t.Errorf("status = %v, want statusOK", m.rows[0].status)
	}
	if m.rows[0].runUUID != "test-uuid" {
		t.Errorf("runUUID = %q, want test-uuid", m.rows[0].runUUID)
	}
	if len(m.failed) != 0 {
		t.Errorf("failed = %v, want empty", m.failed)
	}
	if m.rows[0].finishedAt.IsZero() {
		t.Error("finishedAt not set on ok completion")
	}
}

func TestScanModelScopeDoneFailed(t *testing.T) {
	m := newScanModel(testScopes, func() {})
	m = applyMsgs(m,
		scopeStartMsg{idx: 1},
		scopeDoneMsg{idx: 1, err: fmt.Errorf("access denied")},
	)

	if m.rows[1].status != statusFailed {
		t.Errorf("status = %v, want statusFailed", m.rows[1].status)
	}
	if len(m.failed) != 1 || m.failed[0] != "scope-b" {
		t.Errorf("failed = %v, want [scope-b]", m.failed)
	}
}

func TestScanModelAllDone(t *testing.T) {
	m := newScanModel(testScopes, func() {})
	_, cmd := m.Update(allDoneMsg{})
	if cmd == nil {
		t.Error("allDoneMsg should return tea.Quit cmd (non-nil)")
	}
}

func TestScanModelQuitKey(t *testing.T) {
	var cancelled bool
	m := newScanModel(testScopes, func() { cancelled = true })
	_, cmd := m.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	if !cancelled {
		t.Error("q key should have called cancel()")
	}
	if cmd == nil {
		t.Error("q key should return tea.Quit cmd (non-nil)")
	}
}

func TestScanModelWindowSize(t *testing.T) {
	m := newScanModel(testScopes, func() {})
	m = applyMsgs(m, tea.WindowSizeMsg{Width: 120, Height: 24})
	if m.height != 24 {
		t.Errorf("height = %d, want 24", m.height)
	}
	if m.width != 120 {
		t.Errorf("width = %d, want 120", m.width)
	}
}

func TestScanModelRecentRingCap(t *testing.T) {
	// Build 8 scopes and complete them all; recent should cap at recentCap.
	scopes := make([]inventory.Scope, 8)
	for i := range scopes {
		scopes[i] = inventory.Scope{ID: fmt.Sprintf("s%d", i), ProviderID: "gcp"}
	}
	m := newScanModel(scopes, func() {})
	for i := range scopes {
		m = applyMsgs(m, scopeStartMsg{idx: i}, scopeDoneMsg{idx: i})
	}
	if len(m.recent) != recentCap {
		t.Errorf("recent len = %d, want %d", len(m.recent), recentCap)
	}
	// Should hold last recentCap indexes: 3,4,5,6,7
	want := []int{3, 4, 5, 6, 7}
	for i, idx := range m.recent {
		if idx != want[i] {
			t.Errorf("recent[%d] = %d, want %d", i, idx, want[i])
		}
	}
}

func TestScanModelActiveCleared(t *testing.T) {
	m := newScanModel(testScopes, func() {})
	m = applyMsgs(m, scopeStartMsg{idx: 0})
	if m.activeIdx != 0 {
		t.Errorf("activeIdx = %d after start, want 0", m.activeIdx)
	}
	m = applyMsgs(m, scopeDoneMsg{idx: 0})
	if m.activeIdx != -1 {
		t.Errorf("activeIdx = %d after done, want -1", m.activeIdx)
	}
}

func TestScanModelViewContains(t *testing.T) {
	tests := []struct {
		name    string
		msgs    []tea.Msg
		want    []string
		notwant []string
	}{
		{
			name:    "all queued on start",
			msgs:    nil,
			want:    []string{"queued 3", "0/3"},
			notwant: []string{"recently completed"},
		},
		{
			name: "running row shows kind hint",
			msgs: []tea.Msg{
				scopeStartMsg{idx: 0},
				scanProgressMsg{idx: 0, kind: inventory.KindVM},
			},
			want:    []string{"...VM", "0/3", "running 1"},
			notwant: []string{"recently completed"},
		},
		{
			name: "ok completion appears in tail",
			msgs: []tea.Msg{
				scopeStartMsg{idx: 0},
				scanProgressMsg{idx: 0, kind: inventory.KindVM},
				scanProgressMsg{idx: 0, kind: inventory.KindDisk},
				scopeDoneMsg{idx: 0, runUUID: "u1"},
			},
			want: []string{"scope-a", "2 resources", "1/3", "recently completed"},
		},
		{
			name: "failed completion appears in tail",
			msgs: []tea.Msg{
				scopeStartMsg{idx: 0},
				scopeDoneMsg{idx: 0, err: fmt.Errorf("permission denied")},
			},
			want: []string{"permission denied", "1/3", "recently completed"},
		},
		{
			name: "no recently-completed heading when nothing done",
			msgs: []tea.Msg{
				scopeStartMsg{idx: 0},
			},
			notwant: []string{"recently completed"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newScanModel(testScopes, func() {})
			m = applyMsgs(m, tc.msgs...)
			view := m.View().Content
			for _, s := range tc.want {
				if !strings.Contains(view, s) {
					t.Errorf("view missing %q\nfull view:\n%s", s, view)
				}
			}
			for _, s := range tc.notwant {
				if strings.Contains(view, s) {
					t.Errorf("view should not contain %q\nfull view:\n%s", s, view)
				}
			}
		})
	}
}

func TestTailBudget(t *testing.T) {
	tests := []struct {
		height int
		want   int
	}{
		{0, recentCap},
		{8, 1},
		{10, 1},
		{11, 1},
		{14, 4},
		{15, 5},
		{24, 5},
		{80, 5},
	}
	for _, tc := range tests {
		m := newScanModel(testScopes, func() {})
		m.height = tc.height
		got := m.tailBudget()
		if got != tc.want {
			t.Errorf("tailBudget(height=%d) = %d, want %d", tc.height, got, tc.want)
		}
	}
}
