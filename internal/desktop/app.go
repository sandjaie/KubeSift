package desktop

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"io/fs"
)

func Run(assets fs.FS) error {
	app := application.New(application.Options{Name: "KubeSift", Description: "Phase 1A validation", Services: []application.Service{application.NewService(&Service{})}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)}, Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true}})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "KubeSift · Phase 1A", Width: 1100, Height: 760, URL: "/"})
	return app.Run()
}
