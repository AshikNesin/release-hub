// Package web embeds the built React UI (web/dist, produced by `make web`)
// into the Go binary, so release-hub ships as one self-contained file.
package web

import (
	"embed"
	"io/fs"
)

// dist holds the Vite build output. The committed .gitkeep keeps the
// directory present (go:embed needs a match) before the UI is built.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the built UI rooted at dist/ (index.html, assets/…).
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
