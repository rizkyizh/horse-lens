package cli

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rizkyizh/horse-lens/internal/config"
	"github.com/rizkyizh/horse-lens/internal/shell"
	"github.com/rizkyizh/horse-lens/internal/tui"
	"github.com/rizkyizh/horse-lens/internal/workspace"
)

// Picker runs the interactive workspace list. Entering a workspace happens
// after the TUI has exited and released the terminal, never inside it.
func Picker(out, errOut io.Writer, over config.Overrides) int {
	a := &app{out: out, errOut: errOut, over: over}

	paths, f, err := a.load()
	if err != nil {
		fmt.Fprintln(errOut, "horselens:", err)
		return 1
	}
	all, err := workspace.ResolveAll(f, paths.Root)
	if err != nil {
		fmt.Fprintln(errOut, "horselens:", err)
		return 1
	}
	rows, err := tui.RowsFrom(all)
	if err != nil {
		fmt.Fprintln(errOut, "horselens:", err)
		return 1
	}

	applyFn := func(name string) error {
		ws, err := a.resolveOne(f, paths.Root, name)
		if err != nil {
			return err
		}
		p, err := workspace.BuildPlan(ws)
		if err != nil {
			return err
		}
		return p.Apply()
	}

	model, err := tea.NewProgram(tui.New(rows, applyFn), tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintln(errOut, "horselens:", err)
		return 1
	}

	m, ok := model.(tui.Model)
	if !ok {
		return 0
	}
	res := m.Result()

	switch {
	case res.Delete != "":
		if err := a.cmdDelete([]string{res.Delete}); err != nil {
			fmt.Fprintln(errOut, "horselens:", err)
			return 1
		}
	case res.Enter != "":
		ws, err := a.resolveOne(f, paths.Root, res.Enter)
		if err != nil {
			fmt.Fprintln(errOut, "horselens:", err)
			return 1
		}
		if err := a.applyOne(ws); err != nil {
			fmt.Fprintln(errOut, "horselens:", err)
			return 1
		}
		fmt.Fprintf(out, "entering %s (exit to leave)\n", ws.Dir)
		if err := shell.Enter(ws.Dir, ws.Name); err != nil {
			fmt.Fprintln(errOut, "horselens:", err)
			return 1
		}
	}
	return 0
}
