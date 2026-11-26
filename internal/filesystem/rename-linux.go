//go:build linux

package filesystem

import (
	"errors"

	"golang.org/x/sys/unix"
)

// renameExclusive renames the source to the destination and fails when the
// destination already exists.
//
//nolint:wrapcheck // Transfer adds source, destination, and operation context to rename failures.
func renameExclusive(source, destination string) error {
	return unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
}

// crossDevice reports whether a rename failed because the source and the
// destination are on different filesystems.
func crossDevice(err error) bool { return errors.Is(err, unix.EXDEV) }
