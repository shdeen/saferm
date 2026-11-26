package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/review"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// UpdateSettings saves the supplied settings changes through config.Update and
// returns the resulting settings.
func (*OperationService) UpdateSettings(ctx context.Context, changes config.Settings) (config.Settings, error) {
	homeDirectory, err := config.Home()
	if err != nil {
		return config.Settings{}, err
	}

	return config.Update(ctx, homeDirectory, changes)
}

// DirectoryExists reports whether the supplied path names an existing
// directory, as config.DirectoryExists determines it.
func (*OperationService) DirectoryExists(directoryPath string) (bool, error) {
	homeDirectory, err := config.Home()
	if err != nil {
		return false, err
	}

	return config.DirectoryExists(homeDirectory, directoryPath)
}

// RuleMalformed reports whether the review pattern is malformed, as
// review.Malformed determines it.
func (*OperationService) RuleMalformed(pattern string) bool {
	return review.Malformed(pattern)
}

// OpenConfiguration opens the configuration file in its associated application.
func (*OperationService) OpenConfiguration() error {
	homeDirectory, err := config.Home()
	if err != nil {
		return err
	}

	configPath := filepath.Join(homeDirectory, config.Directory, config.Filename)
	if err = application.Get().Browser.OpenFile(configPath); err != nil {
		return fmt.Errorf("%q: %w: %w", configPath, errs.ErrApplicationLaunch, err)
	}

	return nil
}
