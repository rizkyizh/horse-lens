package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	if len(s) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	}
	return tea.KeyMsg{Type: tea.KeyEsc}
}

func send(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(Model)
	}
	return m
}

func twoRows() Model {
	return New([]Row{
		{Name: "auth", Dir: "/ws/auth", Links: 2},
		{Name: "web", Dir: "/ws/web", Links: 1, Drift: 1},
	}, func(string) error { return nil })
}

func TestCursorMovementIsClamped(t *testing.T) {
	m := send(twoRows(), "k", "k")
	if m.cursor != 0 {
		t.Errorf("cursor = %d after moving up at the top, want 0", m.cursor)
	}
	m = send(m, "j", "j", "j")
	if m.cursor != 1 {
		t.Errorf("cursor = %d after moving down past the end, want 1", m.cursor)
	}
}

// Entering must not happen inside the TUI: it records the choice and quits so
// the caller can spawn the shell after the terminal is released.
func TestEnterRecordsChoiceAndQuits(t *testing.T) {
	m := send(twoRows(), "j")
	next, cmd := m.Update(key("enter"))
	m = next.(Model)
	if m.Result().Enter != "web" {
		t.Errorf("Enter = %q, want web", m.Result().Enter)
	}
	if cmd == nil {
		t.Error("expected a quit command")
	}
}

func TestDeleteRequiresConfirmation(t *testing.T) {
	m := send(twoRows(), "d")
	if m.mode != modeConfirmDelete {
		t.Fatal("d did not open the confirmation")
	}
	if !strings.Contains(m.View(), "DELETE WORKSPACE") {
		t.Error("confirmation not rendered")
	}
	// Cancelling records nothing.
	m = send(m, "n")
	if m.Result().Delete != "" || m.mode != modeList {
		t.Errorf("cancel left state %+v mode %v", m.Result(), m.mode)
	}
	// Confirming records the name.
	m = send(m, "d", "y")
	if m.Result().Delete != "auth" {
		t.Errorf("Delete = %q, want auth", m.Result().Delete)
	}
}

func TestApplyReportsFailure(t *testing.T) {
	m := New([]Row{{Name: "auth"}}, func(string) error { return errBoom })
	m = send(m, "a")
	if !strings.Contains(m.status, "apply failed") {
		t.Errorf("status = %q, want an apply failure", m.status)
	}
}

var errBoom = boom{}

type boom struct{}

func (boom) Error() string { return "boom" }

func TestEmptyListRenders(t *testing.T) {
	m := New(nil, nil)
	out := m.View()
	if !strings.Contains(out, "no workspaces") {
		t.Errorf("empty view missing hint:\n%s", out)
	}
	// Keys must not panic with no rows.
	send(m, "j", "k", "a", "d", "enter")
}

func TestSummary(t *testing.T) {
	if got := (Row{}).Summary(); got != "in sync" {
		t.Errorf("Summary() = %q, want in sync", got)
	}
	got := Row{Drift: 2, Dangling: 1, Foreign: 3}.Summary()
	for _, want := range []string{"2 pending", "1 dangling", "3 foreign"} {
		if !strings.Contains(got, want) {
			t.Errorf("Summary() = %q, missing %q", got, want)
		}
	}
}
