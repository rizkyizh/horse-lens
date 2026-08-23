package cli

import (
	"io/fs"
	"os"
)

// statPath is a thin wrapper so the commands do not import os directly just
// to warn about a missing source.
func statPath(p string) (fs.FileInfo, error) { return os.Stat(p) }
