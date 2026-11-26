package filesystem

// Invariants tested:
//  1. Directory copies: Copying preserves nested file contents and symlinks without removing the source.
//  2. Occupied destinations: Transfer always refuses replacement and preserves both items.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shdeen/saferm/internal/errs"
)

// TestDirectoryCopy verifies invariant #1: Directory copies.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A recursive copy contains the original bytes and relative symlink target,
// including an empty directory, while the source remains present.
func TestDirectoryCopy(t *testing.T) {
	root := t.TempDir()

	source, destination := filepath.Join(root, "source"), filepath.Join(root, "destination")
	if err := os.MkdirAll(filepath.Join(source, "empty"), 0o700); err != nil {
		t.Fatalf("💣 fixture directory: %v", err)
	}

	if err := os.WriteFile(filepath.Join(source, "file"), []byte("payload"), 0o600); err != nil {
		t.Fatalf("💣 fixture file: %v", err)
	}

	if err := os.Symlink("file", filepath.Join(source, "link")); err != nil {
		t.Fatalf("💣 fixture symlink: %v", err)
	}

	if err := copyItem(t.Context(), source, destination); err != nil {
		t.Errorf("✗ copy: %v", err)
	}

	for _, directory := range []string{source, destination} {
		contents, err := os.ReadFile(filepath.Join(directory, "file"))
		if err != nil || string(contents) != "payload" {
			t.Errorf("✗ copied file: %q, %v", contents, err)
		}

		target, err := os.Readlink(filepath.Join(directory, "link"))
		if err != nil || target != "file" {
			t.Errorf("✗ symlink changed: %q, %v", target, err)
		}

		if metadata, err := os.Stat(filepath.Join(directory, "empty")); err != nil || !metadata.IsDir() {
			t.Errorf("✗ empty directory: %v", err)
		}
	}

	if !t.Failed() {
		t.Log("✓ copies preserve data and source")
	}
}

// TestTransferCollision verifies invariant #2: Occupied destinations.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// An occupied destination produces an identifiable error and both original file
// contents survive. The transfer operation cannot remove an occupying item.
func TestTransferCollision(t *testing.T) {
	root := t.TempDir()

	source, destination := filepath.Join(root, "source"), filepath.Join(root, "destination")
	for _, path := range []string{source, destination} {
		if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
			t.Fatalf("💣 fixture file: %v", err)
		}
	}

	if err := Transfer(t.Context(), source, destination); !errors.Is(err, errs.ErrFileExists) {
		t.Errorf("✗ collision: %v", err)
	}

	for _, path := range []string{source, destination} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != path {
			t.Errorf("✗ content changed: %q, %v", path, err)
		}
	}

	if !t.Failed() {
		t.Log("✓ occupied destination is preserved")
	}
}
