package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxCompletions bounds how many candidates are offered for one prefix.
const maxCompletions = 50

// completePath lists directories matching a partially typed path.
//
// Only directories are offered, since a workspace links to project folders.
// The candidates come back in the same shape the user typed: a path begun with
// ~ stays written with ~, so accepting one does not silently rewrite a
// portable config entry into an absolute one.
func completePath(input string) []string {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return nil
	}

	tilde := raw == "~" || strings.HasPrefix(raw, "~/")
	expanded := raw
	if tilde {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		if raw == "~" {
			expanded = home
		} else {
			expanded = filepath.Join(home, raw[2:])
		}
	}

	// A trailing slash means "list this directory"; otherwise the last segment
	// is a prefix to match within its parent.
	var dir, prefix string
	if raw == "~" || strings.HasSuffix(raw, string(filepath.Separator)) {
		dir, prefix = expanded, ""
	} else {
		dir, prefix = filepath.Dir(expanded), filepath.Base(expanded)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if prefix == "" {
			// Hidden directories are noise until they are asked for by name.
			if strings.HasPrefix(name, ".") {
				continue
			}
		} else if !strings.HasPrefix(name, prefix) {
			continue
		}
		out = append(out, shorten(filepath.Join(dir, name), tilde)+string(filepath.Separator))
	}

	sort.Strings(out)
	if len(out) > maxCompletions {
		out = out[:maxCompletions]
	}
	return out
}

// shorten writes a path back with ~ when that is how the user typed it.
func shorten(path string, tilde bool) string {
	if !tilde {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == home {
		return "~"
	}
	if rest := strings.TrimPrefix(path, home+string(filepath.Separator)); rest != path {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

// cycle steps through candidates, returning the index after moving by step and
// wrapping at both ends.
func cycle(current, step, n int) int {
	if n == 0 {
		return -1
	}
	next := current + step
	if next < 0 {
		next = n - 1
	}
	if next >= n {
		next = 0
	}
	return next
}
