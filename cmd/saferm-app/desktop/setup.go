// Package desktop connects the GUI and SafeRM Notify to the operating system:
// handing a launch to a running instance, registering SafeRM Notify to start at
// login, opening the GUI for a review, and logging errors.
package desktop

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/filesystem"
)

// NotifierID identifies SafeRM Notify to the operating system.
const NotifierID = "com.shdeen.saferm.notify"

// setupFile names the marker file that records SafeRM Notify's registration.
const setupFile = "notifications.setup"

// Setup registers the SafeRM Notify executable that is packaged with the GUI
// executable to start at login, then writes a marker file in the SafeRM
// directory. Setup does nothing when the marker exists, so a user who later
// disables SafeRM Notify stays unregistered. It also does nothing when SafeRM
// Notify is not packaged.
func Setup(home, guiExecutable string) error {
	marker := filepath.Join(home, config.Directory, setupFile)
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%q: %w: %w", marker, errs.ErrNotifierSetup, err)
	}

	notifierPath := notifierExecutablePath(guiExecutable)
	if _, err := os.Stat(notifierPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("%q: %w: %w", notifierPath, errs.ErrNotifierSetup, err)
	}

	if err := registerNotifier(notifierPath); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(marker), filesystem.PrivateDirectoryMode); err != nil {
		return fmt.Errorf("%q: %w: %w", marker, errs.ErrNotifierSetup, err)
	}

	if err := os.WriteFile(marker, []byte(notifierPath+"\n"), filesystem.PrivateFileMode); err != nil {
		return fmt.Errorf("%q: %w: %w", marker, errs.ErrNotifierSetup, err)
	}

	return nil
}

// notifierExecutablePath returns the location where this operating system's
// package places the SafeRM Notify executable, given the GUI executable's path.
func notifierExecutablePath(guiExecutable string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(filepath.Dir(guiExecutable), "..", "Library", "LoginItems", "SafeRM Notify.app", "Contents", "MacOS", "saferm-notify")
	}

	notifierName := "saferm-notify"
	if runtime.GOOS == "windows" {
		notifierName += ".exe"
	}

	return filepath.Join(filepath.Dir(guiExecutable), notifierName)
}

// Review starts the GUI executable that is packaged with the running SafeRM Notify,
// passing --review and the action ID. It returns ErrInputInvalid for a
// nonpositive ID.
func Review(actionID int64) error {
	if actionID <= 0 {
		return errs.ErrInputInvalid
	}

	notifierExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%w: %w", errs.ErrApplicationLaunch, err)
	}

	guiDirectory := filepath.Dir(notifierExecutable)
	guiName := "saferm-app"

	switch runtime.GOOS {
	case "darwin":
		guiDirectory = filepath.Clean(filepath.Join(guiDirectory, "..", "..", "..", "..", "..", "MacOS"))
	case "windows":
		guiName += ".exe"
	}

	// #nosec G204 -- The executable is the installed GUI beside this SafeRM Notify executable; the argument is a validated integer.
	command := exec.Command(filepath.Join(guiDirectory, guiName), "--review", strconv.FormatInt(actionID, 10)) //nolint:noctx // The independent GUI must outlive this SafeRM Notify process.

	if err = command.Start(); err != nil {
		return fmt.Errorf("%d: %w: %w", actionID, errs.ErrApplicationLaunch, err)
	}
	go waitProcess(command)

	return nil
}

// waitProcess waits for the started process to exit and logs its failure.
func waitProcess(command *exec.Cmd) {
	if err := command.Wait(); err != nil {
		LogError(err)
	}
}

// LogError appends the error, with a timestamp and the process ID, to
// notifications.log in the SafeRM directory. A nil error writes nothing. When
// the log cannot be written, the error goes to the standard logger instead.
func LogError(operationErr error) {
	if operationErr == nil {
		return
	}

	home, err := config.Home()
	if err != nil {
		log.Print(errors.Join(operationErr, err))

		return
	}

	directory := filepath.Join(home, config.Directory)
	if err = os.MkdirAll(directory, filesystem.PrivateDirectoryMode); err != nil {
		log.Print(errors.Join(operationErr, err))

		return
	}

	file, err := os.OpenFile(filepath.Join(directory, "notifications.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, filesystem.PrivateFileMode)
	if err != nil {
		log.Print(errors.Join(operationErr, err))

		return
	}

	logger := log.New(file, fmt.Sprintf("pid=%d ", os.Getpid()), log.Ldate|log.Ltime|log.LUTC)
	logger.Print(operationErr)

	if err = file.Close(); err != nil {
		log.Print(err)
	}
}
