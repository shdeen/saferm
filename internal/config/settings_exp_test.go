package config

// Invariants tested:
//  1. Unusable configuration: Malformed files fail, while omitted unused settings remain unset.
//  2. Configuration cancellation: Canceled setup preserves an existing configuration.
//  3. Arbitrary TOML: Decoding arbitrary text never panics or merges omitted defaults.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shdeen/saferm/internal/errs"
)

// TestUnusableSettings verifies invariant #1: Unusable configuration.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Missing and invalid values fail only their consuming operation, malformed TOML
// fails decoding, and an active directory or dangling link is not treated as absence.
func TestUnusableSettings(t *testing.T) {
	for _, document := range []string{"", "trash-path=''\nretain-period=-1"} {
		settings, err := Decode(document)
		if err != nil {
			t.Errorf("✗ decoding: %v", err)

			continue
		}

		if _, err = settings.Trash(t.TempDir()); err == nil {
			t.Error("✗ unusable trash accepted")
		}

		if _, err = settings.Retention(); err == nil {
			t.Error("✗ unusable retention accepted")
		}
	}

	if _, err := Decode("trash-path = ["); !errors.Is(err, errs.ErrConfigDecode) {
		t.Errorf("✗ malformed TOML: %v", err)
	}

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, Directory, Filename), 0o700); err != nil {
		t.Fatalf("💣 fixture directory: %v", err)
	}

	if _, err := Load(home); !errors.Is(err, errs.ErrConfigRead) {
		t.Errorf("✗ config directory: %v", err)
	}

	linkHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(linkHome, Directory), 0o700); err != nil {
		t.Fatalf("💣 link directory: %v", err)
	}

	if err := os.Symlink(filepath.Join(linkHome, "missing.toml"), filepath.Join(linkHome, Directory, Filename)); err != nil {
		t.Fatalf("💣 config link: %v", err)
	}

	if _, err := Load(linkHome); !errors.Is(err, errs.ErrConfigRead) {
		t.Errorf("✗ dangling config link: %v", err)
	}

	if !t.Failed() {
		t.Log("✓ unusable settings are not defaulted")
	}
}

// TestCanceledSetup verifies invariant #2: Configuration cancellation.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A canceled confirmed reset preserves the bytes of the existing file.
func TestCanceledSetup(t *testing.T) {
	home := t.TempDir()
	if err := Setup(t.Context(), home, false); err != nil {
		t.Fatalf("💣 setup: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := Setup(ctx, home, true); !errors.Is(err, context.Canceled) {
		t.Errorf("✗ canceled setup: %v", err)
	}

	document, err := os.ReadFile(filepath.Join(home, Directory, Filename))
	if err != nil || string(document) != DefaultDocument {
		t.Errorf("✗ configuration changed: %v", err)
	}

	if !t.Failed() {
		t.Log("✓ cancellation preserves configuration")
	}
}

// FuzzDecode checks invariant #3: Arbitrary TOML.
// Test class: Expanded.
// Test layer: Fuzzing.
// Test kind: Permanent.
// What is under test:
// Arbitrary TOML either decodes without a panic or returns a configuration error;
// successful decoding never introduces an external source path.
func FuzzDecode(f *testing.F) {
	f.Add("")
	f.Add(DefaultDocument)
	f.Add("trash-path = [")
	f.Fuzz(func(t *testing.T, document string) {
		settings, err := Decode(document)
		if err != nil && !errors.Is(err, errs.ErrConfigDecode) {
			t.Errorf("✗ unexpected error: %v", err)
		}

		if settings.External || settings.Path != "" {
			t.Error("✗ decoding manufactured a source")
		}

		if !t.Failed() {
			t.Log("✓ decoding preserves source ownership")
		}
	})

	if !f.Failed() {
		f.Log("✓ arbitrary TOML handled")
	}
}
