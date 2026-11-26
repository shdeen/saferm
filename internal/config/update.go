package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/filesystem"
)

// DirectoryExists reports whether the path names an existing directory,
// expanding a leading ~ to the home directory. It reports false for a blank
// path, a missing path, and a path to a file.
func DirectoryExists(home, directoryPath string) (bool, error) {
	if strings.TrimSpace(directoryPath) == "" {
		return false, nil
	}

	fileInfo, err := os.Stat(filesystem.ExpandHome(directoryPath, home))
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("%q: %w: %w", directoryPath, errs.ErrFileInspect, err)
	}

	return fileInfo.IsDir(), nil
}

// Update applies the non-nil TrashPath, RetainPeriod, and RequireReview of the
// changes to the loaded settings, writes the result to the configuration file,
// and returns it. An empty RequireReview that is not nil clears the review
// rules. When the changes contain none of the three, Update returns the loaded
// settings without writing. The written file holds the value in effect for
// every setting, so a value that Load could not apply is replaced there by the
// value that took its place.
//
// Update returns ErrConfigInvalid for a trash path that is not an existing
// directory and for a negative retention period.
func Update(ctx context.Context, home string, changes Settings) (Settings, error) {
	settings, err := Load(home)
	if err != nil {
		return Settings{}, err
	}

	if changes.TrashPath == nil && changes.RetainPeriod == nil && changes.RequireReview == nil {
		return settings, nil
	}

	if err := applySettingsChanges(home, &settings, changes); err != nil {
		return Settings{}, err
	}

	var document bytes.Buffer
	if err = toml.NewEncoder(&document).Encode(settings); err != nil {
		return Settings{}, fmt.Errorf("%q: %w: %w", settings.Path, errs.ErrConfigWrite, err)
	}

	if err = ctx.Err(); err != nil {
		return Settings{}, fmt.Errorf("%w: %w", errs.ErrOperationCanceled, err)
	}

	if err = os.MkdirAll(filepath.Dir(settings.Path), filesystem.PrivateDirectoryMode); err != nil {
		return Settings{}, fmt.Errorf("%q: %w: %w", settings.Path, errs.ErrConfigWrite, err)
	}

	if err = os.WriteFile(settings.Path, document.Bytes(), filesystem.PrivateFileMode); err != nil {
		return Settings{}, fmt.Errorf("%q: %w: %w", settings.Path, errs.ErrConfigWrite, err)
	}

	settings.External = true
	settings.InvalidTrashPath, settings.InvalidRetainPeriod = "", ""

	return settings, nil
}

// applySettingsChanges validates each non-nil field of the changes and copies
// it into the settings.
func applySettingsChanges(home string, settings *Settings, changes Settings) error {
	if changes.TrashPath != nil {
		directoryExists, err := DirectoryExists(home, *changes.TrashPath)
		if err != nil {
			return err
		}

		if !directoryExists {
			return fmt.Errorf("%q: %w", *changes.TrashPath, errs.ErrConfigInvalid)
		}

		settings.TrashPath = changes.TrashPath
	}

	if changes.RetainPeriod != nil {
		if _, err := changes.Retention(); err != nil {
			return err
		}

		settings.RetainPeriod = changes.RetainPeriod
	}

	if changes.RequireReview != nil {
		settings.RequireReview = changes.RequireReview
	}

	return nil
}
