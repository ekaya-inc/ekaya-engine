//go:build !debug

package ui

import "embed"

//go:embed dist
var distFS embed.FS

// DistFS returns the embedded UI filesystem (production: baked into binary).
func DistFS() embed.FS {
	return distFS
}
