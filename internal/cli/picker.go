package cli

import (
	"fmt"
	"io"

	"github.com/rizkyizh/horse-lens/internal/config"
	"github.com/rizkyizh/horse-lens/internal/shell"
	"github.com/rizkyizh/horse-lens/internal/store"
	"github.com/rizkyizh/horse-lens/internal/tui"
)

// Picker runs the full-screen interface. Entering a workspace happens after
// the UI has exited and released the terminal, never inside it.
func Picker(out, errOut io.Writer, over config.Overrides) int {
	fail := func(err error) int {
		fmt.Fprintln(errOut, "horselens:", err)
		return 1
	}

	st, err := store.Open(over)
	if err != nil {
		return fail(err)
	}
	res, err := tui.Run(st)
	if err != nil {
		return fail(err)
	}
	if res.Enter == "" {
		return 0
	}

	ws, err := st.Resolve(res.Enter)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(out, "entering %s (exit to leave)\n", ws.Dir)
	if err := shell.Enter(ws.Dir, ws.Name); err != nil {
		return fail(err)
	}
	return 0
}
