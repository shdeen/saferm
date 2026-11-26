package desktop

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/filesystem"
)

// autostartTemplate is the autostart entry for SafeRM Notify, with placeholders
// for the path of the SafeRM Notify executable.
//
//go:embed autostart.desktop
var autostartTemplate string

// registerNotifier writes an autostart entry for SafeRM Notify in the user's
// configuration directory and starts SafeRM Notify. It does nothing when the
// entry already exists.
func registerNotifier(notifierPath string) error {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("%w: %w", errs.ErrNotifierSetup, err)
	}

	entryPath := filepath.Join(configDirectory, "autostart", NotifierID+".desktop")
	if _, err = os.Stat(entryPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%q: %w: %w", entryPath, errs.ErrNotifierSetup, err)
	}

	if err = os.MkdirAll(filepath.Dir(entryPath), filesystem.PrivateDirectoryMode); err != nil {
		return fmt.Errorf("%q: %w: %w", entryPath, errs.ErrNotifierSetup, err)
	}
	// Desktop Exec fields have their own escaping rules, independent of a shell.
	escapedPath := strings.NewReplacer("\\", "\\\\\\\\", "\"", "\\\\\"", "`", "\\\\`", "$", "\\\\$", "%", "%%").Replace(notifierPath)
	document := strings.ReplaceAll(autostartTemplate, "@EXECUTABLE@", "\""+escapedPath+"\"")

	document = strings.ReplaceAll(document, "@TRY_EXECUTABLE@", strings.ReplaceAll(notifierPath, "\\", "\\\\"))
	if err = os.WriteFile(entryPath, []byte(document), filesystem.PrivateFileMode); err != nil {
		return fmt.Errorf("%q: %w: %w", entryPath, errs.ErrNotifierSetup, err)
	}

	command := exec.Command(notifierPath) //nolint:noctx // SafeRM Notify must continue after the GUI exits.

	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err = command.Start(); err != nil {
		return fmt.Errorf("%q: %w: %w", notifierPath, errs.ErrNotifierSetup, err)
	}
	go waitProcess(command)

	return nil
}
