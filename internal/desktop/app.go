/*
Purpose: Wire the native Wails application to domain services.
Used by: The desktop executable.
Responsibilities: Register the service, create the window and publish typed session events.
Important: Keep operational policy in domain packages; the desktop bridge only translates calls and events.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
	"io/fs"
)

func init() { application.RegisterEvent[model.Event]("session") }
func Run(assets fs.FS) error {
	service := NewService()
	app := application.New(application.Options{Name: "KubeSift", Description: "Phase 1A validation", Services: []application.Service{application.NewService(service)}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)}, Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true}})
	service.setEmitter(func(event model.Event) { app.Event.Emit("session", event) })
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "KubeSift · Phase 1A", Width: 1100, Height: 760, URL: "/"})
	return app.Run()
}
