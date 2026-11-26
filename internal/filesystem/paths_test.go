package filesystem

// Invariants tested:
//  1. Path resolution: Missing destination components remain under the resolved existing parent.
//  2. Home expansion: Leading home components expand with either supported separator.
//  3. Directory parents: A file cannot serve as a parent of a resolved destination.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shdeen/saferm/internal/errs"
)

// TestMissingParents verifies invariant #1: Path resolution.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Resolving a destination whose parent components do not yet exist returns an
// absolute path beneath the canonical existing parent without creating files.
func TestMissingParents(t *testing.T) {
	home := t.TempDir()

	canonical, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("💣 fixture home: %v", err)
	}

	resolved, err := Resolve(filepath.Join(home, "missing", "nested", "item"), home)
	if err != nil || resolved != filepath.Join(canonical, "missing", "nested", "item") {
		t.Errorf("✗ unresolved destination: %q, %v", resolved, err)
	}

	if exists, err := Exists(filepath.Join(home, "missing")); err != nil || exists {
		t.Errorf("✗ resolution changed filesystem: %v, %v", exists, err)
	}

	if !t.Failed() {
		t.Log("✓ missing components preserve resolved parents")
	}
}

// TestHomeExpansion verifies invariant #2: Home expansion.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A leading tilde or tilde plus either slash expands; embedded tildes and other
// user names are preserved. Both separators use the native home directory path.
func TestHomeExpansion(t *testing.T) {
	home := t.TempDir()
	for _, testCase := range []struct{ input, path string }{
		{"~", home},
		{"~/item", filepath.Join(home, "item")},
		{`~\item`, filepath.Join(home, "item")},
		{"~someone", "~someone"},
		{"item~", "item~"},
	} {
		if expanded := ExpandHome(testCase.input, home); expanded != testCase.path {
			t.Errorf("✗ expansion %q: %q", testCase.input, expanded)
		}
	}

	if !t.Failed() {
		t.Log("✓ home expansion handles both separators")
	}
}

// TestFileParents verifies invariant #3: Directory parents.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A regular file directly above a destination or above missing parent components
// causes ErrPathResolve. Resolution preserves that file and creates nothing.
func TestFileParents(t *testing.T) {
	for _, suffix := range []string{"item", filepath.Join("missing", "nested", "item")} {
		t.Run(suffix, func(t *testing.T) {
			home := t.TempDir()

			parentPath := filepath.Join(home, "parent")
			if err := os.WriteFile(parentPath, []byte("retained contents"), 0o600); err != nil {
				t.Fatalf("💣 create file parent: %v", err)
			}

			resolvedPath, err := Resolve(filepath.Join(parentPath, suffix), home)
			if !errors.Is(err, errs.ErrPathResolve) || resolvedPath != "" {
				t.Errorf("✗ file parent accepted: %q, %v", resolvedPath, err)
			}

			contents, err := os.ReadFile(parentPath)
			if err != nil || string(contents) != "retained contents" {
				t.Errorf("✗ file parent changed: %q, %v", contents, err)
			}

			children, err := os.ReadDir(home)
			if err != nil || len(children) != 1 || children[0].Name() != "parent" {
				t.Errorf("✗ refused resolution changed directory: %v, %v", children, err)
			}

			if !t.Failed() {
				t.Log("✓ a file parent refuses resolution without filesystem changes")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ resolved parents must be directories")
	}
}
