// Package ui carries the built React application so the release artifact is a
// single binary. This file lives beside the Vite project because go:embed
// cannot reach outside its own directory.
package ui

import (
	"embed"
	"io/fs"
)

// dist is the Vite build output. `all:` keeps files whose names begin with an
// underscore or dot, which Vite emits for hashed assets.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the built SPA rooted at its index.html, or an error if the UI
// has not been built yet.
func Dist() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
