package frontend

import (
	"embed"
	"io/fs"
)

//go:embed index.html static
var Assets embed.FS

// FS returns the embedded filesystem
func FS() fs.FS {
	return Assets
}
