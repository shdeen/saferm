package desktop

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"

	"github.com/shdeen/saferm/internal/errs"
	"golang.org/x/sys/windows/registry"
)

// registerNotifier adds SafeRM Notify to the current user's Run registry key
// and starts SafeRM Notify. It does nothing when the registry value already
// exists.
func registerNotifier(notifierPath string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("%w: %w", errs.ErrNotifierSetup, err)
	}

	_, _, existingErr := key.GetStringValue(NotifierID)
	if existingErr == nil {
		if err = key.Close(); err != nil {
			return fmt.Errorf("%w: %w", errs.ErrNotifierSetup, err)
		}

		return nil
	}

	if !errors.Is(existingErr, registry.ErrNotExist) {
		return fmt.Errorf("%w: %w", errs.ErrNotifierSetup, errors.Join(existingErr, key.Close()))
	}

	err = errors.Join(key.SetStringValue(NotifierID, syscall.EscapeArg(notifierPath)), key.Close())
	if err != nil {
		return fmt.Errorf("%w: %w", errs.ErrNotifierSetup, err)
	}

	command := exec.Command(notifierPath) //nolint:noctx // SafeRM Notify must continue after the GUI exits.

	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}

	if err = command.Start(); err != nil {
		return fmt.Errorf("%q: %w: %w", notifierPath, errs.ErrNotifierSetup, err)
	}
	go waitProcess(command)

	return nil
}
