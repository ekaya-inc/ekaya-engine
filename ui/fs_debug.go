//go:build debug

package ui

import (
	"io/fs"
	"os"
)

// DistFS returns a live filesystem rooted at ui/ (debug: reads from disk).
// Wrap in os.DirFS so vite build --watch changes are visible without recompiling Go.
func DistFS() fs.FS {
	return os.DirFS("ui")
}
