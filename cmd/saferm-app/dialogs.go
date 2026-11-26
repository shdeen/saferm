package main

import (
	"fmt"
	"path/filepath"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/filesystem"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// FileDialogs lets the frontend prompt for paths with native dialogs.
type FileDialogs struct{}

// Directory prompts for an existing directory and returns the selected path, or
// an empty path on cancellation. The dialog opens in the supplied directory when
// that directory exists.
func (*FileDialogs) Directory(currentPath string) (string, error) {
	homeDirectory, err := config.Home()
	if err != nil {
		return "", err
	}

	dialog := application.Get().Dialog.OpenFile().CanChooseFiles(false).CanChooseDirectories(true).
		ShowHiddenFiles(true).SetButtonText(appstrings.Text("dialog.select"))

	directoryExists, err := config.DirectoryExists(homeDirectory, currentPath)
	if err != nil {
		return "", err
	}

	if directoryExists {
		dialog.SetDirectory(filesystem.ExpandHome(currentPath, homeDirectory))
	}

	selectedPath, err := dialog.PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("%w: %w", errs.ErrPathResolve, err)
	}

	return selectedPath, nil
}

// Source prompts for an existing file or directory and returns the selected
// path, or an empty path on cancellation.
func (*FileDialogs) Source() (string, error) {
	selectedPath, err := application.Get().Dialog.OpenFile().CanChooseFiles(true).CanChooseDirectories(true).ShowHiddenFiles(true).PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("%w: %w", errs.ErrPathResolve, err)
	}

	return selectedPath, nil
}

// Destination prompts for a new path, proposing the source's base name, and
// returns the selected path, or an empty path on cancellation. It creates
// nothing at the selected path.
func (*FileDialogs) Destination(sourcePath string) (string, error) {
	selectedPath, err := application.Get().Dialog.SaveFile().SetFilename(filepath.Base(sourcePath)).PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("%q: %w: %w", sourcePath, errs.ErrPathResolve, err)
	}

	return selectedPath, nil
}
