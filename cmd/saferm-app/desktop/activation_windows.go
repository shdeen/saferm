package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unsafe"

	"github.com/shdeen/saferm/internal/errs"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
)

// shutdownTimeout limits the wait for SafeRM Notify to exit after a stop request.
const shutdownTimeout = 10 * time.Second

// ForwardActivation sends this process's command line and working directory to
// a running instance of the identified application, and reports whether such an
// instance exists. After sending --quit to SafeRM Notify, it also waits for
// SafeRM Notify to exit, so that an installer can replace its files.
//
// Wails looks up the running instance's message-only window with FindWindowW,
// which cannot find a message-only window. ForwardActivation looks it up with
// FindWindowExW and sends the message in the format that Wails expects.
func ForwardActivation(applicationID string) (bool, error) {
	windowClass := windows.StringToUTF16Ptr("wails-app-" + applicationID + "-sic")
	windowName := windows.StringToUTF16Ptr("wails-app-" + applicationID + "-siw")
	findWindow := windows.NewLazySystemDLL("user32.dll").NewProc("FindWindowExW")

	// #nosec G103 -- The UTF-16 names remain alive for this synchronous OS call.
	window, _, _ := findWindow.Call(w32.HWND_MESSAGE, 0, uintptr(unsafe.Pointer(windowClass)), uintptr(unsafe.Pointer(windowName))) //nolint:errcheck // A missing window is normal; only the returned handle establishes an existing instance.
	if window == 0 {
		return false, nil
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("%w: %w", errs.ErrApplicationLaunch, err)
	}

	document, err := json.Marshal(application.SecondInstanceData{Args: os.Args, WorkingDir: workingDirectory})
	if err != nil {
		return false, fmt.Errorf("%w: %w", errs.ErrApplicationLaunch, err)
	}

	stopNotifier := applicationID == NotifierID && len(os.Args) == 2 && os.Args[1] == "--quit"

	return true, deliverActivation(window, string(document), stopNotifier)
}

// deliverActivation sends the activation document to the running instance's
// window. For a SafeRM Notify stop request, it then waits up to shutdownTimeout for
// that process to exit and returns ErrNotifierStop when the process is still
// running.
func deliverActivation(window w32.HWND, document string, stopNotifier bool) error {
	var process windows.Handle

	if stopNotifier {
		var processID uint32
		if _, err := windows.GetWindowThreadProcessId(windows.HWND(window), &processID); err != nil {
			return fmt.Errorf("%w: %w", errs.ErrNotifierStop, err)
		}

		var err error

		process, err = windows.OpenProcess(windows.SYNCHRONIZE, false, processID)
		if err != nil {
			return fmt.Errorf("%w: %w", errs.ErrNotifierStop, err)
		}
		defer windows.CloseHandle(process) //nolint:errcheck // Closing the local handle cannot change the process's exit outcome.
	}

	w32.SendMessageToWindow(window, document)

	if process != 0 {
		status, err := windows.WaitForSingleObject(process, uint32(shutdownTimeout/time.Millisecond))
		if err != nil {
			return fmt.Errorf("%w: %w", errs.ErrNotifierStop, err)
		}

		if status != windows.WAIT_OBJECT_0 {
			return errs.ErrNotifierStop
		}
	}

	return nil
}
