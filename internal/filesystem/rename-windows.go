//go:build windows

package filesystem

import (
	"errors"

	"golang.org/x/sys/windows"
)

// renameExclusive renames the source to the destination and fails when the
// destination already exists.
//
//nolint:wrapcheck // Transfer adds source, destination, and operation context to rename failures.
func renameExclusive(source, destination string) error {
	sourcePath, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}

	destinationPath, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}

	return windows.MoveFile(sourcePath, destinationPath)
}

// crossDevice reports whether a rename failed because the source and the
// destination are on different volumes.
func crossDevice(err error) bool { return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) }
