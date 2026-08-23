// Package workspace validates workspace definitions and reconciles the
// symlink directory that backs them.
package workspace

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// MaxNameLen bounds names and aliases so they stay valid filenames everywhere.
const MaxNameLen = 64

// nameRe requires a leading alphanumeric, which rules out "-flag" style names
// as well as dotfiles, "." and "..".
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validateLabel(kind, s string) error {
	switch {
	case s == "":
		return fmt.Errorf("%s cannot be empty", kind)
	case s != strings.TrimSpace(s):
		return fmt.Errorf("%s %q has leading or trailing whitespace", kind, s)
	case len(s) > MaxNameLen:
		return fmt.Errorf("%s %q is longer than %d characters", kind, s, MaxNameLen)
	case !nameRe.MatchString(s):
		return fmt.Errorf(
			"%s %q is invalid: use letters, digits, dot, dash or underscore, starting with a letter or digit",
			kind, s)
	}
	return nil
}

// ValidateName checks a workspace name. The name becomes a directory name, so
// anything that could escape the root is rejected outright.
func ValidateName(s string) error { return validateLabel("workspace name", s) }

// ValidateAlias checks a link alias, which becomes a symlink filename.
func ValidateAlias(s string) error { return validateLabel("alias", s) }

// childPath joins name onto root and verifies the result is still directly
// inside root. This is the second line of defence behind ValidateName: even if
// a malformed name reaches here, it cannot address anything outside root.
func childPath(root, name string) (string, error) {
	p := filepath.Join(root, name)
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return "", fmt.Errorf("resolve %q inside %s: %w", name, root, err)
	}
	if rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(rel) ||
		strings.ContainsRune(rel, filepath.Separator) {
		return "", fmt.Errorf("%q would escape %s", name, root)
	}
	return p, nil
}
