package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// Move relocates a materialised workspace directory. A plain rename keeps
// everything inside it — including files horselens does not manage, such as a
// per-directory .claude — which tearing the directory down and rebuilding it
// cannot do.
//
// It reports whether the directory was actually moved. A workspace that has
// never been applied has nothing to move and is not an error.
func Move(oldDir, newDir string) (bool, error) {
	if oldDir == newDir {
		return false, nil
	}
	if _, err := os.Lstat(oldDir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect %s: %w", oldDir, err)
	}
	if _, err := os.Lstat(newDir); err == nil {
		return false, fmt.Errorf("%s already exists", newDir)
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect %s: %w", newDir, err)
	}

	parent := filepath.Dir(newDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", parent, err)
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return false, err
	}
	return true, nil
}
