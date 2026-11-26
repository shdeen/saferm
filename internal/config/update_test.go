package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shdeen/saferm/internal/errs"
)

// Invariants tested:
//  1. Settings persistence: Accepted edits create the configuration and preserve unchanged values.
//  2. Invalid settings: Invalid directory or retention edits do not alter the configuration.
//  3. Directory validation: Only existing directories are accepted, including home-relative paths.

// TestUpdate verifies invariant #1: Settings persistence.
// Test class: Expanded.
// Test layer: Coverage.
// What is under test:
// Updating retention creates a configuration using the remaining defaults;
// subsequent directory and unrestricted review-list edits survive a reload.
func TestUpdate(t *testing.T) {
	homeDirectory := t.TempDir()
	retentionDays := 42

	settings, err := Update(context.Background(), homeDirectory, Settings{RetainPeriod: &retentionDays})
	if err != nil {
		t.Fatalf("💣 update failed: %v", err)
	}

	defaults, err := Decode(DefaultDocument)
	if err != nil {
		t.Fatalf("💣 decode defaults: %v", err)
	}

	if !settings.External || *settings.RetainPeriod != retentionDays || !reflect.DeepEqual(settings.TrashPath, defaults.TrashPath) {
		t.Errorf("✗ saved settings: %#v", settings)
	}

	trashDirectory := t.TempDir()
	reviewPaths := []string{"", "any value", "**/.git/", "any value"}

	_, err = Update(context.Background(), homeDirectory, Settings{TrashPath: &trashDirectory, RequireReview: reviewPaths})
	if err != nil {
		t.Fatalf("💣 update failed: %v", err)
	}

	settings, err = Load(homeDirectory)
	if err != nil {
		t.Fatalf("💣 reload failed: %v", err)
	}

	if *settings.TrashPath != trashDirectory || *settings.RetainPeriod != retentionDays || !reflect.DeepEqual(settings.RequireReview, reviewPaths) {
		t.Errorf("✗ edits did not survive reload: %#v", settings)
	}

	settings, err = Update(context.Background(), homeDirectory, Settings{RequireReview: []string{}})
	if err != nil || len(settings.RequireReview) != 0 {
		t.Errorf("✗ clearing review paths: %#v, %v", settings, err)
	}

	if !t.Failed() {
		t.Log("✓ accepted edits persist without replacing unchanged values")
	}
}

// TestUpdateRejectsInvalidSettings verifies invariant #2: Invalid settings.
// Test class: Expanded.
// Test layer: Coverage.
// What is under test:
// A nonexistent directory and a negative retention period leave the file unchanged.
func TestUpdateRejectsInvalidSettings(t *testing.T) {
	homeDirectory := t.TempDir()
	if err := Setup(context.Background(), homeDirectory, false); err != nil {
		t.Fatalf("💣 setup: %v", err)
	}

	missingDirectory := filepath.Join(homeDirectory, "missing")

	invalidRetention := -1
	for _, changes := range []Settings{{TrashPath: &missingDirectory}, {RetainPeriod: &invalidRetention}} {
		if _, err := Update(context.Background(), homeDirectory, changes); !errors.Is(err, errs.ErrConfigInvalid) {
			t.Errorf("✗ invalid setting accepted: %v", err)
		}
	}

	document, err := os.ReadFile(filepath.Join(homeDirectory, Directory, Filename))
	if err != nil {
		t.Fatalf("💣 read configuration: %v", err)
	}

	if string(document) != DefaultDocument {
		t.Error("✗ rejected edits changed the file")
	}

	if !t.Failed() {
		t.Log("✓ rejected edits preserve the file")
	}
}

// TestDirectoryExists verifies invariant #3: Directory validation.
// Test class: Expanded.
// Test layer: Coverage.
// What is under test:
// Home-relative and absolute directories are valid; files, empty paths and absent paths are invalid.
func TestDirectoryExists(t *testing.T) {
	homeDirectory := t.TempDir()

	filePath := filepath.Join(homeDirectory, "file")
	if err := os.WriteFile(filePath, []byte("contents"), 0o600); err != nil {
		t.Fatalf("💣 fixture: %v", err)
	}

	for _, directoryCase := range []struct {
		path   string
		exists bool
	}{{homeDirectory, true}, {"~/", true}, {filePath, false}, {filePath + "/child", false}, {"", false}, {filepath.Join(homeDirectory, "missing"), false}} {
		directoryExists, err := DirectoryExists(homeDirectory, directoryCase.path)
		if err != nil || directoryExists != directoryCase.exists {
			t.Errorf("✗ %q: exists=%v, error=%v", directoryCase.path, directoryExists, err)
		}
	}

	if !t.Failed() {
		t.Log("✓ only existing directories pass validation")
	}
}
