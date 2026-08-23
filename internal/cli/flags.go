package cli

import (
	"flag"
	"strings"
)

// permute moves flags ahead of positional arguments. Go's flag package stops
// parsing at the first non-flag argument, which would make the natural
// `horselens delete web --force` fail; GNU-style interleaving is what users
// expect, so the arguments are reordered before parsing.
func permute(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]

		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			positional = append(positional, a)
			continue
		}

		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.ContainsRune(name, '=') {
			continue // value is attached
		}
		// A non-boolean flag consumes the next argument as its value.
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// parse permutes then parses, so every command accepts flags in any position.
func (a *app) parse(fs *flag.FlagSet, args []string) error {
	return fs.Parse(permute(fs, args))
}
