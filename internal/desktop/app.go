/*
Purpose: Wire the native Wails application to domain services.
Used by: The desktop executable.
Responsibilities: Register services, size the first window to the selected display and publish session events.
Important: Geometry persistence is independent of Kubernetes/session policy and uses logical bounds.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/preferences"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"io/fs"
	"sync"
	"time"
)

func init() { application.RegisterEvent[model.Event]("session") }
func Run(assets fs.FS) error {
	service := NewService()
	prefs := newPreferencesService()
	explorerService := newExplorerService(service)
	var lock sync.Mutex
	var timer *time.Timer
	closing := false
	restored := false
	app := application.New(application.Options{Name: "KubeSift", Description: "Cluster workspace", Services: []application.Service{application.NewService(service), application.NewService(prefs), application.NewService(explorerService)}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)}, Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true}, OnShutdown: func() {
		lock.Lock()
		closing = true
		if timer != nil {
			timer.Stop()
		}
		lock.Unlock()
	}})
	service.setEmitter(func(event model.Event) { app.Event.Emit("session", event) })
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "KubeSift", Width: 1440, Height: 900, URL: "/"})
	workAreas := func() []preferences.Rect {
		screens := app.Screen.GetAll()
		areas := []preferences.Rect{}
		for _, s := range screens {
			r := s.WorkArea
			a := preferences.Rect{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height}
			if s.IsPrimary {
				areas = append([]preferences.Rect{a}, areas...)
			} else {
				areas = append(areas, a)
			}
		}
		return areas
	}
	normal := func() bool { return !window.IsMinimised() && !window.IsMaximised() && !window.IsFullscreen() }
	setBounds := func(r preferences.Rect) {
		// Size first: macOS converts top-left position using the current frame height.
		window.SetSize(r.Width, r.Height)
		window.SetPosition(r.X, r.Y)
	}
	window.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
		lock.Lock()
		if restored || closing {
			lock.Unlock()
			return
		}
		restored = true
		lock.Unlock()
		p, _ := prefs.GetPreferences()
		r := preferences.Rect{Width: 1440, Height: 900}
		if p.Window != nil {
			r = *p.Window
		}
		setBounds(preferences.LaunchWindowBounds(r, workAreas()))
	})
	save := func(*application.WindowEvent) {
		lock.Lock()
		defer lock.Unlock()
		if closing || !restored {
			return
		}
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(400*time.Millisecond, func() {
			lock.Lock()
			defer lock.Unlock()
			if closing || !normal() {
				return
			}
			r := window.Bounds()
			if r.Width > 0 && r.Height > 0 {
				prefs.saveWindow(preferences.Rect{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height})
			}
		})
	}
	window.OnWindowEvent(events.Common.WindowDidMove, save)
	window.OnWindowEvent(events.Common.WindowDidResize, save)
	app.Event.OnApplicationEvent(events.Mac.ApplicationDidChangeScreenParameters, func(*application.ApplicationEvent) {
		if normal() {
			r := window.Bounds()
			setBounds(preferences.ClampWindow(preferences.Rect{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height}, workAreas()))
		}
	})
	return app.Run()
}
