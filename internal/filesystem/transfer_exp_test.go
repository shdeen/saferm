package filesystem

// Invariants tested:
//  1. Failed copies: Collisions and canceled copies preserve the original source.
//  2. Invalid paths: Missing, overlapping, and invalid source paths cannot destroy contents.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shdeen/saferm/internal/errs"
)

// TestFailedCopies verifies invariant #1: Failed copies.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Copying to an occupied destination refuses replacement; cancellation before
// copying creates no destination and leaves source bytes intact.
func TestFailedCopies(t *testing.T) {
	root := t.TempDir()

	source, destination := filepath.Join(root, "source"), filepath.Join(root, "destination")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatalf("💣 fixture: %v", err)
	}

	if err := os.WriteFile(destination, []byte("destination"), 0o600); err != nil {
		t.Fatalf("💣 fixture: %v", err)
	}

	if err := copyItem(t.Context(), source, destination); !errors.Is(err, os.ErrExist) {
		t.Errorf("✗ collision: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := copyItem(ctx, source, filepath.Join(root, "canceled")); !errors.Is(err, context.Canceled) {
		t.Errorf("✗ cancellation: %v", err)
	}

	if err := Transfer(ctx, source, destination); !errors.Is(err, context.Canceled) {
		t.Errorf("✗ canceled replacement: %v", err)
	}

	for _, path := range []string{source, destination} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != filepath.Base(path) {
			t.Errorf("✗ failed copy altered %q: %v", path, err)
		}
	}

	if !t.Failed() {
		t.Log("✓ failed copies preserve original contents")
	}
}

// TestInvalidPaths verifies invariant #2: Invalid paths.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Overlapping directories, missing sources, and empty paths are refused; removing
// a missing item reports absence instead of claiming a removal.
func TestInvalidPaths(t *testing.T) {
	root := t.TempDir()
	if _, err := CheckTransfer(root, filepath.Join(root, "child"), false); !errors.Is(err, errs.ErrFileOverlap) {
		t.Errorf("✗ overlap: %v", err)
	}

	if err := Transfer(t.Context(), filepath.Join(root, "missing"), filepath.Join(root, "target")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("✗ missing source: %v", err)
	}

	if _, err := Resolve("", root); !errors.Is(err, errs.ErrInputMissing) {
		t.Errorf("✗ empty path: %v", err)
	}

	if err := Remove(t.Context(), filepath.Join(root, "missing")); !errors.Is(err, errs.ErrFileMissing) {
		t.Errorf("✗ missing removal: %v", err)
	}

	if !t.Failed() {
		t.Log("✓ invalid paths fail before mutation")
	}
}
