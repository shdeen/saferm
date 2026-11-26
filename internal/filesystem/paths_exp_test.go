package filesystem

// Invariants tested:
//  1. Literal path components: Resolving a linked parent preserves the final component and creates nothing.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzResolvePath checks invariant #1: Literal path components.
// Test class: Expanded.
// Test layer: Fuzzing.
// Test kind: Permanent.
// What is under test:
// Arbitrary portable filename components, including spaces, braces, and dollar
// signs, remain unchanged after resolving a symbolic parent. No item is created.
func FuzzResolvePath(f *testing.F) {
	for _, component := range []string{"document", "$&{{.Path}}", "a b", "été", "..", "trailing "} {
		f.Add(component)
	}

	f.Fuzz(func(t *testing.T, component string) {
		if len(component) > 128 || !utf8.ValidString(component) || strings.ContainsAny(component, "/\\:*?\"<>|") {
			return
		}

		for _, character := range component {
			if character < ' ' {
				return
			}
		}

		home := t.TempDir()

		canonicalHome, err := filepath.EvalSymlinks(home)
		if err != nil {
			t.Fatalf("💣 canonical directory: %v", err)
		}

		alias := filepath.Join(t.TempDir(), "alias")
		if err := os.Symlink(home, alias); err != nil {
			t.Fatalf("💣 linked parent: %v", err)
		}

		fileName := "item-" + component + ".txt"

		resolvedPath, err := Resolve(filepath.Join(alias, fileName), home)
		if err != nil || resolvedPath != filepath.Join(canonicalHome, fileName) {
			t.Errorf("✗ final component changed: %q, %v", resolvedPath, err)
		}

		children, err := os.ReadDir(home)
		if err != nil || len(children) != 0 {
			t.Errorf("✗ resolution changed directory: %v, %v", children, err)
		}

		if !t.Failed() {
			t.Log("✓ resolving a linked parent preserves the final component without changes")
		}
	})
}
