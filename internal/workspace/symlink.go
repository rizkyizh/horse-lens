package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// WorkspaceRoot returns the directory where symlinks for a workspace are stored.
func WorkspaceRoot(wsName string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "horselens", "workspaces", wsName)
}

func CreateSymlinks(ws Workspace) error {
	root := WorkspaceRoot(ws.Name)
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	for _, l := range ws.Links {
		src := expandHome(l.Src)
		dst := filepath.Join(root, l.Alias)
		// Remove existing symlink if present
		os.Remove(dst)
		if err := os.Symlink(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// RemoveWorkspaceDir deletes the workspace symlink directory entirely.
func RemoveWorkspaceDir(wsName string) error {
	return os.RemoveAll(WorkspaceRoot(wsName))
}

func RemoveSymlinks(ws Workspace) error {
	root := WorkspaceRoot(ws.Name)
	for _, l := range ws.Links {
		dst := filepath.Join(root, l.Alias)
		os.Remove(dst)
	}
	// Try to remove root dir (only succeeds if empty)
	os.Remove(root)
	return nil
}
