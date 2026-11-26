package main

import (
	"os"

	"github.com/shdeen/saferm/cmd/saferm-app/desktop"
	"github.com/shdeen/saferm/frontend"
	"github.com/shdeen/saferm/internal/config"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Initial and minimum dimensions of the application window.
const (
	windowWidth   = 1220
	windowHeight  = 820
	minimumWidth  = 780
	minimumHeight = 580
)

// runApplication creates the application window with its services and runs the
// application until it quits. A launch while another instance is running is
// handed to that instance instead.
func runApplication() error {
	if forwarded, err := desktop.ForwardActivation("com.shdeen.saferm"); forwarded || err != nil {
		return err
	}

	windowService := &WindowService{reviewID: requestedReview(os.Args)}
	app := application.New(application.Options{
		Name: "SafeRM",
		Services: []application.Service{
			application.NewService(&OperationService{}),
			application.NewService(&FileDialogs{}),
			application.NewService(windowService),
		},
		Assets:         application.AssetOptions{Handler: application.AssetFileServerFS(frontend.Assets)},
		Mac:            application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		SingleInstance: &application.SingleInstanceOptions{UniqueID: "com.shdeen.saferm", OnSecondInstanceLaunch: windowService.activate},
	})
	windowService.window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "SafeRM", Width: windowWidth, Height: windowHeight,
		MinWidth: minimumWidth, MinHeight: minimumHeight,
		URL: "/",
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, setupNotifier)

	return app.Run() //nolint:wrapcheck // The application error is reported unchanged by the executable.
}

// setupNotifier handles application startup by calling desktop.Setup for this
// executable and logging any failure. It does nothing in a debug build.
func setupNotifier(_ *application.ApplicationEvent) {
	if application.Get().Env.Info().Debug {
		return
	}

	home, err := config.Home()
	if err != nil {
		desktop.LogError(err)

		return
	}

	executable, err := os.Executable()
	if err == nil {
		err = desktop.Setup(home, executable)
	}

	desktop.LogError(err)
}
