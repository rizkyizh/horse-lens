package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	tea "github.com/charmbracelet/bubbletea"
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
	maxRawBuf       = 256 * 1024
	scrollStep      = 5
	maxScrollHistory = 500
)

type TerminalModel struct {
	focused       bool
	width, height int
	workspaceName string
	ptmx          *os.File
	term          vt10x.Terminal
	ready         bool
	cols, rows    int
	rawBuf        []byte // accumulated raw PTY bytes for scrollback
	scrollOffset  int    // lines scrolled up from bottom; 0 = live
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
	t.term = vt10x.New(vt10x.WithSize(t.cols, t.rows))
	t.ready = true
	return t.readPty()
}

func (t TerminalModel) readPty() tea.Cmd {
	ptmx := t.ptmx             // capture by value (safe across struct copies)
	ws := t.workspaceName      // identify which terminal this read belongs to
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
			t.term.Write(msg.data) //nolint:errcheck
		}
		// Accumulate raw bytes for scrollback; trim front if over limit
		if len(t.rawBuf)+len(msg.data) > maxRawBuf {
			keep := maxRawBuf - len(msg.data)
			if keep < 0 {
				keep = 0
			}
			t.rawBuf = t.rawBuf[len(t.rawBuf)-keep:]
		}
		t.rawBuf = append(t.rawBuf, msg.data...)
		return t, t.readPty()

	case ptyErrorMsg:
		if t.ptmx != nil {
			t.ptmx.Close()
			t.ptmx = nil
		}
		t.ready = false
		t.term = nil
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
	t.ready = false
	t.term = nil
}

func (t *TerminalModel) ScrollBy(delta int) {
	t.scrollOffset += delta
	if t.scrollOffset < 0 {
		t.scrollOffset = 0
	}
	if t.scrollOffset > maxScrollHistory {
		t.scrollOffset = maxScrollHistory
	}
}

func (t *TerminalModel) ScrollToBottom() { t.scrollOffset = 0 }

func (t TerminalModel) IsScrolled() bool { return t.scrollOffset > 0 }

// IsAltScreen returns true when a full-screen program (vim, htop, etc.) is active.
func (t TerminalModel) IsAltScreen() bool {
	if t.term == nil {
		return false
	}
	return t.term.Mode()&vt10x.ModeAltScreen != 0
}

func (t TerminalModel) View() string {
	if t.ready && t.term != nil {
		if t.IsScrolled() && !t.IsAltScreen() {
			return t.renderScrolled()
		}
		return t.renderBorderedGrid()
	}
	// Idle view: use lipgloss with correct outer dimensions
	borderStyle := styleTerminalBorder
	if t.focused {
		borderStyle = styleTerminalBorderFocus
	}
	return borderStyle.Width(t.width).Height(t.height).Render(t.renderIdle())
}

// renderScrolled replays rawBuf into a tall virtual terminal and extracts
// the scrolled window.
func (t TerminalModel) renderScrolled() string {
	replayRows := t.rows + maxScrollHistory
	replay := vt10x.New(vt10x.WithSize(t.cols, replayRows))
	replay.Write(t.rawBuf) //nolint:errcheck

	curY := replay.Cursor().Y
	startRow := curY - t.rows + 1 - t.scrollOffset
	if startRow < 0 {
		startRow = 0
	}
	return t.renderBorderedGridFrom(replay, startRow)
}

// renderBorderedGrid draws the border manually and writes cell content directly,
// bypassing lipgloss so it cannot clip, reflow, or miscount ANSI-heavy content.
func (t TerminalModel) renderBorderedGrid() string {
	return t.renderBorderedGridFrom(t.term, 0)
}

// renderBorderedGridFrom renders a bordered grid using term starting at startRow.
func (t TerminalModel) renderBorderedGridFrom(term vt10x.Terminal, startRow int) string {
	var borderSeq string
	if t.focused {
		borderSeq = "\x1b[38;2;0;255;136m" // colorBorderFocus #00FF88
	} else {
		borderSeq = "\x1b[38;2;68;68;68m" // colorBorder #444444
	}
	const rst = "\x1b[0m"

	cols, rows := t.cols, t.rows
	var sb strings.Builder
	sb.Grow((cols + 32) * (rows + 2))

	// Top border — show scroll indicator when scrolled
	if t.scrollOffset > 0 {
		indicator := fmt.Sprintf(" ↑ -%d ", t.scrollOffset)
		padding := cols - len(indicator)
		if padding < 0 {
			padding = 0
		}
		sb.WriteString(borderSeq + "╭" + indicator + strings.Repeat("─", padding) + "╮" + rst + "\n")
	} else {
		sb.WriteString(borderSeq + "╭" + strings.Repeat("─", cols) + "╮" + rst + "\n")
	}

	// Track current SGR state to suppress redundant escape sequences.
	var (
		curFG, curBG   = vt10x.DefaultFG, vt10x.DefaultBG
		curMode  int16 = 0
		stateSet       = false
	)

	for y := 0; y < rows; y++ {
		sb.WriteString(borderSeq + "│" + rst)
		for x := 0; x < cols; x++ {
			cell := term.Cell(x, startRow+y)
			if stateSet && cell.FG == curFG && cell.BG == curBG && cell.Mode == curMode {
				ch := cell.Char
				if ch == 0 {
					ch = ' '
				}
				sb.WriteRune(ch)
			} else {
				sb.WriteString(cellToANSI(cell)) // emits full SGR + char
				curFG, curBG, curMode = cell.FG, cell.BG, cell.Mode
				stateSet = true
			}
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

// Attribute bit positions from vt10x state.go (unexported constants, raw values):
//
//	attrReverse   = 1 << 0
//	attrUnderline = 1 << 1
//	attrBold      = 1 << 2
//	attrItalic    = 1 << 4
const (
	vtAttrReverse   int16 = 1 << 0
	vtAttrUnderline int16 = 1 << 1
	vtAttrBold      int16 = 1 << 2
	vtAttrItalic    int16 = 1 << 4
)

func cellToANSI(g vt10x.Glyph) string {
	var seq strings.Builder
	seq.WriteString("\x1b[0")

	if g.Mode&vtAttrBold != 0      { seq.WriteString(";1") }
	if g.Mode&vtAttrUnderline != 0 { seq.WriteString(";4") }
	if g.Mode&vtAttrReverse != 0   { seq.WriteString(";7") }
	if g.Mode&vtAttrItalic != 0    { seq.WriteString(";3") }

	if s := colorCode(g.FG, true); s != "" {
		seq.WriteString(";")
		seq.WriteString(s)
	}
	if s := colorCode(g.BG, false); s != "" {
		seq.WriteString(";")
		seq.WriteString(s)
	}

	seq.WriteString("m")

	ch := g.Char
	if ch == 0 {
		ch = ' '
	}
	seq.WriteRune(ch)
	return seq.String()
}

func colorCode(c vt10x.Color, fg bool) string {
	if c == vt10x.DefaultFG || c == vt10x.DefaultBG || c == vt10x.DefaultCursor {
		return ""
	}
	base := 30
	if !fg {
		base = 40
	}
	idx := uint32(c)
	if idx < 8 {
		return fmt.Sprintf("%d", base+int(idx))
	}
	if idx < 16 {
		return fmt.Sprintf("%d", base+60+int(idx-8))
	}
	if idx < 256 {
		if fg {
			return fmt.Sprintf("38;5;%d", idx)
		}
		return fmt.Sprintf("48;5;%d", idx)
	}
	// true color packed as r<<16|g<<8|b
	r, g, b := (idx>>16)&0xff, (idx>>8)&0xff, idx&0xff
	if fg {
		return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
	}
	return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
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
