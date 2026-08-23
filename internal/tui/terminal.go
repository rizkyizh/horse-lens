package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

type ptyReadMsg struct {
	workspace string
	data      []byte
}
type ptyErrorMsg struct {
	workspace string
	err       error
}

const (
	scrollStep = 5

	// scrollbackLines is how many lines the emulator retains above the visible
	// screen. The emulator owns this buffer, so unlike the old replay-based
	// scrollback it costs nothing per frame and can be generous.
	scrollbackLines = 5000
)

type TerminalModel struct {
	focused       bool
	width, height int
	workspaceName string
	ptmx          *os.File
	term          *vt.Emulator
	ready         bool
	cols, rows    int
	scrollOffset  int // lines scrolled up from the bottom; 0 = live
}

func NewTerminal() TerminalModel {
	return TerminalModel{}
}

func (t *TerminalModel) Start(dir string) tea.Cmd {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	t.cols = t.width - 2
	t.rows = t.height - 2
	if t.cols < 1 {
		t.cols = 80
	}
	if t.rows < 1 {
		t.rows = 24
	}

	cmd := exec.Command(shell)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color",
		fmt.Sprintf("COLUMNS=%d", t.cols),
		fmt.Sprintf("LINES=%d", t.rows))

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(t.rows), Cols: uint16(t.cols),
	})
	if err != nil {
		return func() tea.Msg { return ptyErrorMsg{workspace: t.workspaceName, err: err} }
	}

	t.ptmx = ptmx
	t.term = vt.NewEmulator(t.cols, t.rows)
	t.term.SetScrollbackSize(scrollbackLines)
	t.ready = true
	return t.readPty()
}

func (t TerminalModel) readPty() tea.Cmd {
	ptmx := t.ptmx        // capture by value (safe across struct copies)
	ws := t.workspaceName // identify which terminal this read belongs to
	return func() tea.Msg {
		buf := make([]byte, 4096)
		n, err := ptmx.Read(buf)
		if err != nil {
			return ptyErrorMsg{workspace: ws, err: err}
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		return ptyReadMsg{workspace: ws, data: data}
	}
}

func (t TerminalModel) Update(msg tea.Msg) (TerminalModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ptyReadMsg:
		if t.term != nil {
			before := t.term.ScrollbackLen()
			t.term.Write(msg.data) //nolint:errcheck
			// Every line pushed into scrollback shifts the window down by one.
			// While the user is scrolled up, grow the offset by the same amount
			// so the viewport stays anchored to the content they are reading.
			if t.scrollOffset > 0 {
				if grew := t.term.ScrollbackLen() - before; grew > 0 {
					t.scrollOffset += grew
					t.clampScroll()
				}
			}
		}
		return t, t.readPty()

	case ptyErrorMsg:
		t.Close()
		return t, nil

	case tea.KeyMsg:
		if t.focused && t.ready && t.ptmx != nil {
			if data := keyToBytes(msg); data != nil {
				t.ptmx.Write(data) //nolint:errcheck
			}
		}
		return t, nil
	}
	return t, nil
}

func (t *TerminalModel) SetSize(w, h int) {
	t.width = w
	t.height = h
	t.cols = w - 2
	t.rows = h - 2
	if t.cols < 1 {
		t.cols = 1
	}
	if t.rows < 1 {
		t.rows = 1
	}
	if t.term != nil {
		t.term.Resize(t.cols, t.rows)
	}
	if t.ptmx != nil {
		pty.Setsize(t.ptmx, &pty.Winsize{Rows: uint16(t.rows), Cols: uint16(t.cols)}) //nolint:errcheck
	}
	t.clampScroll()
}

func (t *TerminalModel) SetFocused(f bool) {
	t.focused = f
}

func (t *TerminalModel) SetWorkspaceName(name string) {
	t.workspaceName = name
}

func (t *TerminalModel) IsReady() bool {
	return t.ready
}

func (t *TerminalModel) WorkspaceName() string {
	return t.workspaceName
}

func (t *TerminalModel) Close() {
	if t.ptmx != nil {
		t.ptmx.Close()
		t.ptmx = nil
	}
	if t.term != nil {
		t.term.Close() //nolint:errcheck
		t.term = nil
	}
	t.ready = false
	t.scrollOffset = 0
}

// maxScroll is how far back the emulator can currently scroll.
func (t TerminalModel) maxScroll() int {
	if t.term == nil {
		return 0
	}
	return t.term.ScrollbackLen()
}

func (t *TerminalModel) clampScroll() {
	if t.scrollOffset < 0 {
		t.scrollOffset = 0
	}
	if m := t.maxScroll(); t.scrollOffset > m {
		t.scrollOffset = m
	}
}

func (t *TerminalModel) ScrollBy(delta int) {
	t.scrollOffset += delta
	t.clampScroll()
}

func (t *TerminalModel) ScrollToBottom() { t.scrollOffset = 0 }

func (t TerminalModel) IsScrolled() bool { return t.scrollOffset > 0 }

// IsAltScreen returns true when a full-screen program (vim, htop, etc.) is active.
func (t TerminalModel) IsAltScreen() bool {
	if t.term == nil {
		return false
	}
	return t.term.IsAltScreen()
}

func (t TerminalModel) View() string {
	if t.ready && t.term != nil {
		return t.renderBorderedGrid()
	}
	// Idle view: use lipgloss with correct outer dimensions
	borderStyle := styleTerminalBorder
	if t.focused {
		borderStyle = styleTerminalBorderFocus
	}
	return borderStyle.Width(t.width).Height(t.height).Render(t.renderIdle())
}

// cellAt maps a visible row to either the scrollback buffer or the live screen,
// according to the current scroll offset. Scrollback is bypassed entirely in
// alt-screen mode, where a full-screen program owns the grid.
func (t TerminalModel) cellAt(x, y int) *uv.Cell {
	if t.term.IsAltScreen() {
		return t.term.CellAt(x, y)
	}
	sbLen := t.term.ScrollbackLen()
	row := sbLen - t.scrollOffset + y
	if row < sbLen {
		return t.term.ScrollbackCellAt(x, row)
	}
	return t.term.CellAt(x, row-sbLen)
}

// renderBorderedGrid draws the border manually and writes cell content directly,
// bypassing lipgloss so it cannot clip, reflow, or miscount ANSI-heavy content.
func (t TerminalModel) renderBorderedGrid() string {
	var borderSeq string
	if t.focused {
		borderSeq = "\x1b[38;2;0;255;136m" // colorBorderFocus #00FF88
	} else {
		borderSeq = "\x1b[38;2;68;68;68m" // colorBorder #444444
	}
	const rst = "\x1b[0m"

	cols, rows := t.cols, t.rows
	scrolled := t.scrollOffset > 0 && !t.IsAltScreen()

	var sb strings.Builder
	sb.Grow((cols + 32) * (rows + 2))

	// Top border — show scroll indicator when scrolled
	if scrolled {
		indicator := fmt.Sprintf(" ↑ -%d ", t.scrollOffset)
		padding := cols - lipgloss.Width(indicator)
		if padding < 0 {
			padding = 0
		}
		sb.WriteString(borderSeq + "╭" + indicator + strings.Repeat("─", padding) + "╮" + rst + "\n")
	} else {
		sb.WriteString(borderSeq + "╭" + strings.Repeat("─", cols) + "╮" + rst + "\n")
	}

	for y := 0; y < rows; y++ {
		sb.WriteString(borderSeq + "│" + rst)

		// The border reset above leaves the SGR state at its default, so each
		// row starts from a zero style and emits only the deltas between cells.
		var prev uv.Style

		for x := 0; x < cols; {
			cell := t.cellAt(x, y)
			if cell == nil {
				sb.WriteString(" ")
				x++
				continue
			}
			if cell.Width == 0 {
				// Continuation column of a wide grapheme; already emitted.
				x++
				continue
			}
			if x+cell.Width > cols {
				// A wide grapheme would overflow the panel — pad the remainder.
				sb.WriteString(rst + strings.Repeat(" ", cols-x))
				break
			}
			if !cell.Style.Equal(&prev) {
				sb.WriteString(cell.Style.Diff(&prev))
				prev = cell.Style
			}
			if cell.Content == "" {
				sb.WriteString(" ")
			} else {
				sb.WriteString(cell.Content)
			}
			x += cell.Width
		}

		sb.WriteString(rst + borderSeq + "│" + rst + "\n")
	}

	// Bottom border (no trailing newline — lipgloss/BubbleTea adds its own)
	sb.WriteString(borderSeq + "╰" + strings.Repeat("─", cols) + "╯" + rst)

	return sb.String()
}

func (t TerminalModel) renderIdle() string {
	var sb strings.Builder
	if t.workspaceName != "" {
		sb.WriteString(styleDimItem.Render("Workspace: ") + styleActiveItem.Render(t.workspaceName) + "\n\n")
		sb.WriteString(styleDimItem.Render("Press [↵] on workspace to re-enter terminal.") + "\n")
	} else {
		sb.WriteString(styleDimItem.Render("No workspace active.") + "\n\n")
		sb.WriteString(styleDimItem.Render("Select a workspace and press [↵] to open terminal.") + "\n")
	}
	return sb.String()
}

func keyToBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		return []byte(string(msg.Runes))
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyBackspace:
		return []byte{'\x7f'}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyEsc:
		return []byte{'\x1b'}
	case tea.KeyUp:
		return []byte{'\x1b', '[', 'A'}
	case tea.KeyDown:
		return []byte{'\x1b', '[', 'B'}
	case tea.KeyRight:
		return []byte{'\x1b', '[', 'C'}
	case tea.KeyLeft:
		return []byte{'\x1b', '[', 'D'}
	case tea.KeyHome:
		return []byte{'\x1b', '[', 'H'}
	case tea.KeyEnd:
		return []byte{'\x1b', '[', 'F'}
	case tea.KeyPgUp:
		return []byte{'\x1b', '[', '5', '~'}
	case tea.KeyPgDown:
		return []byte{'\x1b', '[', '6', '~'}
	case tea.KeyDelete:
		return []byte{'\x1b', '[', '3', '~'}
	case tea.KeyCtrlA:
		return []byte{'\x01'}
	case tea.KeyCtrlB:
		return []byte{'\x02'}
	case tea.KeyCtrlC:
		return []byte{'\x03'}
	case tea.KeyCtrlD:
		return []byte{'\x04'}
	case tea.KeyCtrlE:
		return []byte{'\x05'}
	case tea.KeyCtrlF:
		return []byte{'\x06'}
	case tea.KeyCtrlG:
		return []byte{'\x07'}
	case tea.KeyCtrlH:
		return []byte{'\x08'}
	case tea.KeyCtrlJ:
		return []byte{'\x0a'}
	case tea.KeyCtrlK:
		return []byte{'\x0b'}
	case tea.KeyCtrlL:
		return []byte{'\x0c'}
	case tea.KeyCtrlN:
		return []byte{'\x0e'}
	case tea.KeyCtrlO:
		return []byte{'\x0f'}
	case tea.KeyCtrlP:
		return []byte{'\x10'}
	case tea.KeyCtrlQ:
		return []byte{'\x11'}
	case tea.KeyCtrlR:
		return []byte{'\x12'}
	case tea.KeyCtrlS:
		return []byte{'\x13'}
	case tea.KeyCtrlT:
		return []byte{'\x14'}
	case tea.KeyCtrlU:
		return []byte{'\x15'}
	case tea.KeyCtrlV:
		return []byte{'\x16'}
	case tea.KeyCtrlW:
		return []byte{'\x17'}
	case tea.KeyCtrlX:
		return []byte{'\x18'}
	case tea.KeyCtrlY:
		return []byte{'\x19'}
	case tea.KeyCtrlZ:
		return []byte{'\x1a'}
	}
	return nil
}
