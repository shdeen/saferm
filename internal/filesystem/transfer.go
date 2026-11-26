package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/shdeen/saferm/internal/errs"
)

// Transfer moves an item from the source to an empty destination. It returns
// ErrFileExists when the destination is occupied and preserves both items.
//
// Across filesystems, Transfer copies the item and removes the source only
// after the copy completes. A failed copy leaves the source intact and leaves
// the partial copy at the destination.
func Transfer(ctx context.Context, source, destination string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", errs.ErrOperationCanceled, err)
	}

	if _, err := CheckTransfer(source, destination, false); err != nil {
		return err
	}

	err := renameExclusive(source, destination)
	if err == nil {
		return nil
	}

	if !crossDevice(err) {
		return fmt.Errorf("%q → %q: %w: %w", source, destination, errs.ErrFileTransfer, err)
	}

	if err := copyItem(ctx, source, destination); err != nil {
		return err
	}

	return Remove(ctx, source)
}

// Remove permanently deletes the item at the path, including the contents of a
// directory. It deletes a symbolic link, not the link's target. It returns
// ErrFileMissing when nothing exists at the path.
func Remove(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", errs.ErrOperationCanceled, err)
	}

	exists, err := Exists(path)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("%q: %w", path, errs.ErrFileMissing)
	}

	if err = os.RemoveAll(path); err != nil {
		return fmt.Errorf("%q: %w: %w", path, errs.ErrFileRemove, err)
	}

	return nil
}

// copyItem copies a regular file, a directory with its contents, or a symbolic
// link to a destination that must not exist. It returns ErrFileUnsupported for
// any other kind of item.
func copyItem(ctx context.Context, source, destination string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", errs.ErrOperationCanceled, err)
	}

	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("%q: %w: %w", source, errs.ErrFileInspect, err)
	}

	switch {
	case sourceInfo.Mode()&os.ModeSymlink != 0:
		linkTarget, readErr := os.Readlink(source)
		if readErr != nil {
			return fmt.Errorf("%q: %w: %w", source, errs.ErrFileTransfer, readErr)
		}

		err = os.Symlink(linkTarget, destination)
	case sourceInfo.IsDir():
		return copyDirectory(ctx, source, destination, sourceInfo.Mode())
	case sourceInfo.Mode().IsRegular():
		return copyFile(ctx, source, destination, sourceInfo.Mode())
	default:
		return fmt.Errorf("%q: %w", source, errs.ErrFileUnsupported)
	}

	if err != nil {
		return fmt.Errorf("%q: %w: %w", destination, errs.ErrFileTransfer, err)
	}

	return nil
}

// copyDirectory creates the destination directory, copies each child of the
// source into it through copyItem, and then gives the destination the supplied
// permissions.
func copyDirectory(ctx context.Context, source, destination string, mode os.FileMode) error {
	if err := os.Mkdir(destination, PrivateDirectoryMode); err != nil {
		return fmt.Errorf("%q: %w: %w", destination, errs.ErrFileTransfer, err)
	}

	children, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("%q: %w: %w", source, errs.ErrFileTransfer, err)
	}

	for _, child := range children {
		if err := copyItem(ctx, filepath.Join(source, child.Name()), filepath.Join(destination, child.Name())); err != nil {
			return err
		}
	}

	if err = os.Chmod(destination, mode.Perm()); err != nil {
		return fmt.Errorf("%q: %w: %w", destination, errs.ErrFileTransfer, err)
	}

	return nil
}

// copyFile copies the contents of the source file to a new destination file
// that has the supplied permissions, and syncs the destination to disk.
// Canceling the context stops the copy.
func copyFile(ctx context.Context, source, destination string, mode os.FileMode) (resultErr error) {
	sourceFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("%q: %w: %w", source, errs.ErrFileTransfer, err)
	}
	defer closeTransfer(sourceFile, &resultErr)

	destinationFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return fmt.Errorf("%q: %w: %w", destination, errs.ErrFileTransfer, err)
	}
	defer closeTransfer(destinationFile, &resultErr)

	reader := contextReader{ctx: ctx, reader: sourceFile}
	if _, err = io.Copy(destinationFile, &reader); err != nil {
		return fmt.Errorf("%q: %w: %w", destination, errs.ErrFileTransfer, err)
	}

	if err = destinationFile.Sync(); err != nil {
		return fmt.Errorf("%q: %w: %w", destination, errs.ErrFileTransfer, err)
	}

	return nil
}

// closeTransfer closes the file and joins a failure to close it into the error
// that resultErr points to.
func closeTransfer(file *os.File, resultErr *error) {
	if err := file.Close(); err != nil {
		*resultErr = errors.Join(*resultErr, fmt.Errorf("%q: %w: %w", file.Name(), errs.ErrFileTransfer, err))
	}
}

// contextReader reads from the wrapped reader until its context is canceled.
//
//nolint:containedctx // io.Reader has no context parameter; this adapter lives for one copy.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// Read reads from the wrapped reader, or returns ErrOperationCanceled once the
// context is canceled.
func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, fmt.Errorf("%w: %w", errs.ErrOperationCanceled, err)
	}

	return reader.reader.Read(buffer) //nolint:wrapcheck // Preserve raw io.EOF; copyFile adds transfer context to other errors.
}
