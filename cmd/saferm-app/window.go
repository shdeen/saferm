package main

import (
	"strconv"
	"sync"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// reviewEvent is emitted to the frontend each time another launch activates
// this instance.
const reviewEvent = "saferm:review"

// WindowService shows native message dialogs and holds the action that a launch
// asked to review.
type WindowService struct {
	// window is the application window, which dialogs attach to.
	window *application.WebviewWindow
	// mutex guards reviewID.
	mutex sync.Mutex
	// reviewID is the ID of the action that the frontend has yet to open for
	// review, or zero.
	reviewID int64
}

// Error shows the message in a native error dialog attached to the window.
func (service *WindowService) Error(message string) {
	application.Get().Dialog.Error().SetTitle(appstrings.Text("notification.appName")).SetMessage(message).AttachToWindow(service.window).Show()
}

// Information shows the message in a native information dialog attached to the
// window.
func (service *WindowService) Information(message string) {
	application.Get().Dialog.Info().SetTitle(appstrings.Text("notification.appName")).SetMessage(message).AttachToWindow(service.window).Show()
}

// TakeReview returns the ID of the action that a launch asked to review, or
// zero when there is none. It clears that ID, so the next call returns zero.
func (service *WindowService) TakeReview() int64 {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	actionID := service.reviewID
	service.reviewID = 0

	return actionID
}

// activate handles another launch of the application, given that launch's
// command line. It performs the following:
//   - replaces the review ID with the one that the launch requested, or zero
//   - shows and focuses the window
//   - emits reviewEvent
func (service *WindowService) activate(data application.SecondInstanceData) {
	service.mutex.Lock()
	service.reviewID = requestedReview(data.Args)
	service.mutex.Unlock()

	if service.window != nil {
		service.window.Show()
		service.window.Focus()
	}

	application.Get().Event.Emit(reviewEvent)
}

// requestedReview returns the action ID from a command line of the form
// "<program> --review <id>". It returns zero for any other command line and for
// an ID that is not a positive integer.
func requestedReview(arguments []string) int64 {
	if len(arguments) != 3 || arguments[1] != "--review" {
		return 0
	}

	actionID, err := strconv.ParseInt(arguments[2], 10, 64)
	if err != nil || actionID <= 0 {
		return 0
	}

	return actionID
}
