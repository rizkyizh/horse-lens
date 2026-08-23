package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/vt"
)

// newTestTerminal builds a TerminalModel backed by an emulator but no PTY.
// Commands returned by Update are never run, so the nil ptmx is safe.
func newTestTerminal(cols, rows int) TerminalModel {
	t := TerminalModel{cols: cols, rows: rows, width: cols + 2, height: rows + 2}
	t.term = vt.NewEmulator(cols, rows)
	t.term.SetScrollbackSize(scrollbackLines)
	t.ready = true
	return t
}

// visibleRows returns the plain text of each visible row via the same cellAt
// mapping the renderer uses.
func visibleRows(t TerminalModel) []string {
	rows := make([]string, t.rows)
	for y := 0; y < t.rows; y++ {
		var sb strings.Builder
		for x := 0; x < t.cols; x++ {
			c := t.cellAt(x, y)
			if c == nil || c.Content == "" {
				sb.WriteString(" ")
				continue
			}
			sb.WriteString(c.Content)
		}
		rows[y] = strings.TrimRight(sb.String(), " ")
	}
	return rows
}

func writeLines(t *TerminalModel, from, to int) {
	for i := from; i <= to; i++ {
		fmt.Fprintf(t.term, "line %d\r\n", i)
	}
}

func TestScrollbackWindow(t *testing.T) {
	term := newTestTerminal(20, 5)
	writeLines(&term, 1, 20)

	if got := term.term.ScrollbackLen(); got != 16 {
		t.Fatalf("ScrollbackLen() = %d, want 16", got)
	}

	// Live view (offset 0) shows the tail.
	if got, want := visibleRows(term)[0], "line 17"; got != want {
		t.Errorf("live row 0 = %q, want %q", got, want)
	}

	// Scrolled up by 5, the window starts 5 lines earlier.
	term.ScrollBy(5)
	rows := visibleRows(term)
	want := []string{"line 12", "line 13", "line 14", "line 15", "line 16"}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("scrolled row %d = %q, want %q", i, rows[i], want[i])
		}
	}

	// Scrolling past the oldest line clamps to the top of the scrollback.
	term.ScrollBy(9999)
	if term.scrollOffset != 16 {
		t.Errorf("scrollOffset = %d, want clamp to 16", term.scrollOffset)
	}
	if got, want := visibleRows(term)[0], "line 1"; got != want {
		t.Errorf("top row = %q, want %q", got, want)
	}

	// Scrolling back down past the bottom clamps to live.
	term.ScrollBy(-9999)
	if term.scrollOffset != 0 {
		t.Errorf("scrollOffset = %d, want 0", term.scrollOffset)
	}
}

// While scrolled, incoming output must not drag the viewport along.
func TestScrollAnchorsWhileOutputArrives(t *testing.T) {
	term := newTestTerminal(20, 5)
	writeLines(&term, 1, 20)
	term.ScrollBy(5)
	before := visibleRows(term)

	term, _ = term.Update(ptyReadMsg{data: []byte("line 21\r\nline 22\r\n")})

	if term.scrollOffset != 7 {
		t.Errorf("scrollOffset = %d, want 7 (5 + 2 new lines)", term.scrollOffset)
	}
	after := visibleRows(term)
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("row %d drifted: %q -> %q", i, before[i], after[i])
		}
	}
}

// Alt-screen programs own the grid; scrollback must be bypassed.
func TestAltScreenIgnoresScrollback(t *testing.T) {
	term := newTestTerminal(20, 5)
	writeLines(&term, 1, 20)
	term.ScrollBy(10)

	fmt.Fprint(term.term, "\x1b[?1049h")
	if !term.IsAltScreen() {
		t.Fatal("IsAltScreen() = false after DECSET 1049")
	}
	fmt.Fprint(term.term, "ALT")

	if got, want := visibleRows(term)[0], "ALT"; got != want {
		t.Errorf("alt-screen row 0 = %q, want %q", got, want)
	}
	if strings.Contains(term.renderBorderedGrid(), "↑ -") {
		t.Error("scroll indicator drawn while in alt-screen")
	}
}

// Every rendered line must be exactly cols+2 display columns wide, or the
// panel border tears. Covers plain, styled, wide-rune and scrolled output.
func TestRenderedRowsHaveExactWidth(t *testing.T) {
	cases := []struct {
		name     string
		write    string
		scrollBy int
	}{
		{name: "plain", write: "hello\r\nworld\r\n"},
		{name: "styled", write: "\x1b[1;31mred\x1b[0m \x1b[38;2;0;255;136mtruecolor\x1b[0m\r\n"},
		{name: "wide runes", write: "日本語テキスト\r\n"},
		{name: "wide rune at edge", write: strings.Repeat("a", 19) + "日\r\n"},
		{name: "scrolled", write: strings.Repeat("filler line\r\n", 40), scrollBy: 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			term := newTestTerminal(20, 5)
			fmt.Fprint(term.term, tc.write)
			if tc.scrollBy > 0 {
				term.ScrollBy(tc.scrollBy)
			}

			for i, line := range strings.Split(term.renderBorderedGrid(), "\n") {
				if w := lipgloss.Width(line); w != term.cols+2 {
					t.Errorf("line %d width = %d, want %d\n  %q", i, w, term.cols+2, line)
				}
			}
		})
	}
}

// Resizing must not discard scrollback or leave the offset out of range.
func TestResizePreservesScrollback(t *testing.T) {
	term := newTestTerminal(20, 5)
	writeLines(&term, 1, 20)
	term.ScrollBy(16)

	term.SetSize(42, 10)

	if term.term.ScrollbackLen() != 16 {
		t.Errorf("ScrollbackLen() = %d after resize, want 16", term.term.ScrollbackLen())
	}
	if term.scrollOffset > term.term.ScrollbackLen() {
		t.Errorf("scrollOffset %d exceeds scrollback %d", term.scrollOffset, term.term.ScrollbackLen())
	}
	for i, line := range strings.Split(term.renderBorderedGrid(), "\n") {
		if w := lipgloss.Width(line); w != term.cols+2 {
			t.Errorf("after resize line %d width = %d, want %d", i, w, term.cols+2)
		}
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	term := newTestTerminal(20, 5)
	writeLines(&term, 1, 20)
	term.Close()
	term.Close()

	if term.IsReady() {
		t.Error("IsReady() = true after Close()")
	}
	if term.IsAltScreen() {
		t.Error("IsAltScreen() = true after Close()")
	}
	if term.maxScroll() != 0 {
		t.Error("maxScroll() != 0 after Close()")
	}
	term.View() // must not panic on a closed terminal
}

// screenText flattens the whole visible grid into one string.
func screenText(t TerminalModel) string {
	return strings.Join(visibleRows(t), "\n")
}

// TestStartDrivesRealShell exercises the full path — Start, the PTY read loop,
// Update and the renderer — against an actual shell process.
func TestStartDrivesRealShell(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real shell")
	}
	t.Setenv("SHELL", "/bin/sh")

	term := NewTerminal()
	term.SetWorkspaceName("test")
	term.SetSize(82, 12)

	cmd := term.Start(t.TempDir())
	if !term.IsReady() {
		t.Skip("could not allocate a PTY in this environment")
	}
	defer term.Close()

	if _, err := term.ptmx.Write([]byte("echo horse''lens-ok\n")); err != nil {
		t.Fatalf("write to pty: %v", err)
	}

	msgs := make(chan tea.Msg, 1)
	run := func(c tea.Cmd) {
		if c != nil {
			go func() { msgs <- c() }()
		}
	}
	run(cmd)

	deadline := time.After(15 * time.Second)
	for {
		select {
		case msg := <-msgs:
			var next tea.Cmd
			term, next = term.Update(msg)
			if !term.IsReady() {
				t.Fatalf("terminal exited early; screen:\n%s", screenText(term))
			}
			// The echoed command line contains "horse''lens-ok"; only the
			// shell's own output produces the joined form.
			if strings.Contains(screenText(term), "horselens-ok") {
				for i, line := range strings.Split(term.renderBorderedGrid(), "\n") {
					if w := lipgloss.Width(line); w != term.cols+2 {
						t.Errorf("live render line %d width = %d, want %d", i, w, term.cols+2)
					}
				}
				return
			}
			run(next)
		case <-deadline:
			t.Fatalf("shell output not seen in time; screen:\n%s", screenText(term))
		}
	}
}
