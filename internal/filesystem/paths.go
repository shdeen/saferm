// Package filesystem owns path inspection and recoverable file transfers.
package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shdeen/saferm/internal/errs"
)

// Resolve returns the absolute form of a path, expanding a leading ~ to the
// home directory and resolving the symbolic links in its parent directories. A
// symbolic link in the final component stays unresolved, and trailing components
// that do not exist are kept as given. It returns ErrInputMissing for a blank
// path and ErrPathResolve when an existing parent is not a directory.
func Resolve(path, home string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%w", errs.ErrInputMissing)
	}

	absolute, err := filepath.Abs(ExpandHome(path, home))
	if err != nil {
		return "", fmt.Errorf("%q: %w: %w", path, errs.ErrPathResolve, err)
	}

	parent := filepath.Dir(absolute)
	components := []string{filepath.Base(absolute)}

	for {
		resolved, resolveErr := filepath.EvalSymlinks(parent)
		if resolveErr == nil {
			parentInfo, inspectErr := os.Stat(resolved)
			if inspectErr != nil {
				return "", fmt.Errorf("%q: %w: %w", path, errs.ErrPathResolve, inspectErr)
			}

			if !parentInfo.IsDir() {
				return "", fmt.Errorf("%q: %w", path, errs.ErrPathResolve)
			}

			for _, component := range slices.Backward(components) {
				resolved = filepath.Join(resolved, component)
			}

			return resolved, nil
		}

		if !errors.Is(resolveErr, os.ErrNotExist) || filepath.Dir(parent) == parent {
			return "", fmt.Errorf("%q: %w: %w", path, errs.ErrPathResolve, resolveErr)
		}

		components = append(components, filepath.Base(parent))
		parent = filepath.Dir(parent)
	}
}

// Exists reports whether an item exists at the path. It does not follow a
// symbolic link in the final component, so a dangling link exists.
func Exists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("%q: %w: %w", path, errs.ErrFileInspect, err)
	}

	return true, nil
}

// Contains reports whether the candidate path is the directory itself or lies
// anywhere beneath it. It compares the two paths without reading the filesystem.
func Contains(directory, candidate string) bool {
	relative, err := filepath.Rel(directory, candidate)

	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// CheckTransfer verifies that the source can be transferred to the destination
// and returns the source's file information. It returns ErrFileOverlap when the
// two paths are the same or one contains the other, and ErrFileExists when the
// destination is occupied and overwriting is not allowed.
func CheckTransfer(source, destination string, overwrite bool) (os.FileInfo, error) {
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return nil, fmt.Errorf("%q: %w: %w", source, errs.ErrFileInspect, err)
	}

	if source == destination || (sourceInfo.IsDir() && Contains(source, destination)) || Contains(destination, source) {
		return nil, fmt.Errorf("%q → %q: %w", source, destination, errs.ErrFileOverlap)
	}

	exists, err := Exists(destination)
	if err != nil {
		return nil, err
	}

	if exists && !overwrite {
		return nil, fmt.Errorf("%q: %w", destination, errs.ErrFileExists)
	}

	return sourceInfo, nil
}

// ExpandHome replaces a leading ~ with the home directory when the ~ stands
// alone or precedes a slash or backslash. It also converts the path's slashes
// to the operating system's separator.
func ExpandHome(path, home string) string {
	if path == "~" {
		return home
	}

	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		return filepath.Join(home, filepath.FromSlash(path[2:]))
	}

	return filepath.FromSlash(path)
}

// PrivateDirectoryMode and PrivateFileMode restrict new SafeRM data to its owner.
const (
	PrivateDirectoryMode os.FileMode = 0o700
	PrivateFileMode      os.FileMode = 0o600
)
