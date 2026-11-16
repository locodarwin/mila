// Package webui embeds the built frontend. The Makefile copies web/build
// into dist before the Go build; a binary built without it still runs and
// says so on every page.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built frontend, rooted at its index.html.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
