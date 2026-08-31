package tui

import (
	"fmt"
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

// displayName is the final segment of a candidate, with its trailing slash.
func displayName(candidate string) string {
	trimmed := strings.TrimSuffix(candidate, string(filepath.Separator))
	if trimmed == "" {
		return candidate
	}
	return filepath.Base(trimmed) + string(filepath.Separator)
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

// suggestionRows is how many candidate lines the form reserves. The modal is a
// fixed size, so the space is always there whether or not anything matches.
const suggestionRows = 6

// renderSuggestions lays out the candidate list, windowed around the selected
// entry so a long list stays navigable in a fixed number of rows.
//
// It returns plain lines; styling is applied by the caller.
func renderSuggestions(candidates []string, selected, rows int) []string {
	if rows < 1 {
		return nil
	}
	if len(candidates) == 0 {
		return []string{"no matching directories"}
	}

	// Keep the selection in view, leaving room for a "more" line when the list
	// does not fit.
	visible := rows
	if len(candidates) > rows {
		visible = rows - 1
	}
	start := 0
	if selected >= visible {
		start = selected - visible + 1
	}
	if start+visible > len(candidates) {
		start = len(candidates) - visible
	}
	if start < 0 {
		start = 0
	}

	out := make([]string, 0, rows)
	for i := start; i < start+visible && i < len(candidates); i++ {
		marker := "  "
		if i == selected {
			marker = "> "
		}
		// Only the last segment is shown. Candidates all share a parent, so
		// the name is unambiguous, and a full path would run off a modal that
		// is only wide enough for the field above it.
		out = append(out, marker+displayName(candidates[i]))
	}
	if len(candidates) > visible {
		out = append(out, fmt.Sprintf("  %d of %d", selected+1, len(candidates)))
	}
	return out
}
