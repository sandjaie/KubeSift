/*
Purpose: Bundle the production frontend into the desktop executable.
Used by: cmd/kubesift/main.go and the native asset server.
Responsibilities: Embed Vite output and expose its dist directory as a filesystem.
Important: Build the frontend before compiling the executable; no sidecar server is shipped.
*/
package assets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

func Files() fs.FS {
	assets, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return assets
}
