package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/shdeen/saferm/cmd/saferm-app/desktop"
	"github.com/shdeen/saferm/cmd/saferm/output"
	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/notify"
	"github.com/shdeen/saferm/internal/saferm"
	"github.com/shdeen/saferm/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// reviewCategory identifies the notification category that offers the Approve,
// Deny, and Review responses.
const reviewCategory = "saferm-review"

// notRunningExitCode is the exit code of a --quit launch that found no running
// SafeRM Notify.
const notRunningExitCode = 2

// notificationNameKey selects the name that SafeRM Notify uses as its application
// name and as the title of its native dialogs.
const notificationNameKey = "notification.appName"

// showMessageDialog presents a native dialog. Production sets it once; serial
// tests replace only this OS interaction and restore it before returning.
var showMessageDialog = (*application.MessageDialog).Show //nolint:gochecknoglobals // The replaceable OS dialog boundary is immutable in production.

// Service shows a notification for each pending action that is pushed to
// SafeRM Notify, and carries out the response chosen on that notification.
type Service struct {
	// notifications is the native notification service.
	notifications *notifications.NotificationService
	// listener receives the pushed action IDs. It is nil until startup opens it.
	listener *notify.Listener
	// workers tracks the goroutine that receives from the listener.
	workers sync.WaitGroup
}

// runNotifier runs SafeRM Notify until it quits and returns the exit code for
// the process. A launch while another SafeRM Notify is running is handed to the
// running one, which quits when the launch passed --quit. A --quit launch that
// finds no running SafeRM Notify returns notRunningExitCode without running.
func runNotifier() (int, error) {
	if forwarded, err := desktop.ForwardActivation(desktop.NotifierID); forwarded || err != nil {
		return 0, err
	}

	service := &Service{notifications: notifications.New()}
	app := application.New(application.Options{
		Name:           appstrings.Text(notificationNameKey),
		Services:       []application.Service{application.NewService(service.notifications), application.NewService(service)},
		Mac:            application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
		SingleInstance: &application.SingleInstanceOptions{UniqueID: desktop.NotifierID, OnSecondInstanceLaunch: activate},
	})

	if quitRequested(os.Args) {
		return notRunningExitCode, nil
	}

	return 0, app.Run() //nolint:wrapcheck // The executable reports the application error unchanged.
}

// activate handles another launch of SafeRM Notify, given that launch's command
// line, by quitting when the launch passed --quit.
func activate(data application.SecondInstanceData) {
	if quitRequested(data.Args) {
		application.Get().Quit()
	}
}

// quitRequested reports whether the command line consists of the program name
// and --quit.
func quitRequested(arguments []string) bool {
	return len(arguments) == 2 && arguments[1] == "--quit"
}

// ServiceStartup prepares SafeRM Notify when the application starts:
//   - registers the response handler and the notification category for reviews
//   - requests permission to show notifications
//   - opens the listener through notify.Listen and starts receiving from it
//
// It returns an error only when the home directory cannot be determined. It logs
// any other failure and returns nil; a failure to register the category or to
// open the listener also ends the preparation there.
func (service *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	home, err := config.Home()
	if err != nil {
		return err
	}

	service.notifications.OnNotificationResponse(service.respond)

	err = service.notifications.RegisterNotificationCategory(notifications.NotificationCategory{
		ID: reviewCategory,
		Actions: []notifications.NotificationAction{
			{ID: store.ApproveDecision, Title: appstrings.Text("notification.approve")},
			{ID: store.DenyDecision, Title: appstrings.Text("notification.deny")},
			{ID: "review", Title: appstrings.Text("notification.review")},
		},
	})
	if err != nil {
		desktop.LogError(err)

		return nil
	}
	// Keep receiving future requests if permission is denied. The user can
	// enable notifications in system settings without restarting SafeRM Notify.
	_, err = service.notifications.RequestNotificationAuthorization()
	desktop.LogError(err)

	service.listener, err = notify.Listen(ctx, home)
	if err != nil {
		desktop.LogError(err)

		return nil
	}

	service.workers.Add(1)
	go service.receive(ctx)

	return nil
}

// ServiceShutdown closes the listener and waits for the receiving goroutine to
// exit.
func (service *Service) ServiceShutdown() error {
	if service.listener == nil {
		return nil
	}

	err := service.listener.Close()
	service.workers.Wait()

	return err
}

// receive shows a review notification for each action ID that arrives on the
// listener, until the listener closes. It skips an action that is no longer
// pending and logs every failure.
func (service *Service) receive(ctx context.Context) {
	defer service.workers.Done()

	for {
		actionID, err := service.listener.Receive()
		if errors.Is(err, net.ErrClosed) {
			return
		}

		if err != nil {
			desktop.LogError(err)

			continue
		}

		result, err := saferm.Run(ctx, &saferm.Request{Command: saferm.View, ID: strconv.FormatInt(actionID, 10)})
		if err != nil {
			desktop.LogError(err)

			continue
		}

		if len(result.Actions) == 1 && result.Actions[0].Status == store.Pending {
			desktop.LogError(service.notifications.SendNotificationWithActions(requestNotification(&result.Actions[0])))
		}
	}
}

// respond carries out the response chosen on a review notification. Approve and
// Deny dispose of the action through saferm.Run, asking for linked consent when
// required. Review, or a click on the notification itself, opens the action in
// the GUI through desktop.Review. A
// failure is logged and shown in a native error dialog.
func (*Service) respond(result notifications.NotificationResult) { //nolint:gocritic // Wails requires a value parameter for this callback.
	if result.Error != nil {
		desktop.LogError(result.Error)

		return
	}

	actionID, err := strconv.ParseInt(result.Response.ID, 10, 64)
	if err != nil || actionID <= 0 {
		return
	}

	switch result.Response.ActionIdentifier {
	case store.ApproveDecision, store.DenyDecision:
		disposeNotification(&saferm.Request{Command: saferm.Dispose, ID: result.Response.ID, Decision: result.Response.ActionIdentifier})

		return
	case "review", notifications.DefaultActionIdentifier:
		err = desktop.Review(actionID)
	default:
		return
	}

	reportError(err)
}

// disposeNotification submits a notification decision and presents any required
// linked question. Only the Yes callback submits a fresh confirmed request.
func disposeNotification(request *saferm.Request) {
	result, err := saferm.Run(context.Background(), request)
	if errors.Is(err, errs.ErrConfirmation) && result.Confirmation != nil {
		message, renderErr := output.RenderString(result.Confirmation.Prompt, result.Confirmation)
		if renderErr != nil {
			reportError(renderErr)

			return
		}

		dialog := application.Get().Dialog.Question().SetTitle(appstrings.Text(notificationNameKey)).SetMessage(message)
		dialog.AddButton(appstrings.Text("dialog.no")).SetAsDefault().SetAsCancel()
		dialog.AddButton(appstrings.Text("dialog.yes")).OnClick(func() {
			confirmedRequest := *request
			confirmedRequest.Confirmed = true
			disposeNotification(&confirmedRequest)
		})
		showMessageDialog(dialog)

		return
	}

	reportError(err)
}

// reportError logs a failed notification response and shows its diagnostic.
// Successful responses have no dialog.
func reportError(operationErr error) {
	if operationErr == nil {
		return
	}

	desktop.LogError(operationErr)
	showMessageDialog(application.Get().Dialog.Error().SetTitle(appstrings.Text(notificationNameKey)).SetMessage(operationErr.Error()))
}

// requestNotification builds the review notification for an action. The title
// gives the operation type and the base name of the source, and the body names
// the requester when the action has one.
func requestNotification(action *store.Action) notifications.NotificationOptions {
	body := ""
	if action.Requester != "" {
		body = fmt.Sprintf(appstrings.Text("notification.requester"), action.Requester)
	}

	return notifications.NotificationOptions{
		ID:         strconv.FormatInt(action.ID, 10),
		Title:      appstrings.Text("operation."+action.Type) + ": " + filepath.Base(action.Source),
		Body:       body,
		CategoryID: reviewCategory,
	}
}
