package config

// Invariants tested:
//  1. Unusable configuration: Malformed files fail, while omitted unused settings remain unset.
//  2. Configuration cancellation: Canceled setup preserves an existing configuration.
//  3. Replaced values: A trash path or retention period in the file that can't be applied gives way to the embedded one and is reported as the file wrote it.
//  4. Arbitrary TOML: Decoding arbitrary text never panics or merges omitted defaults.
//  5. Loaded required settings: Every configuration that loads has a retention period and a trash path that is not blank.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// TestLoadInvalidValues verifies invariant #3: Replaced values.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A retention period that is negative, a string, or a fraction loads as the
// embedded retention period. A trash path that is a file, leads through a file,
// leads through a loop of symbolic links, or is not a string loads as the
// embedded trash path. Each replaced value is reported in the form the file
// wrote it, the other setting is not reported, and the file in the way keeps
// its contents. A trash directory that does not exist yet, under an existing
// directory or behind a symbolic link, is kept and not reported.
func TestLoadInvalidValues(t *testing.T) {
	embedded, err := Decode(DefaultDocument)
	if err != nil || embedded.TrashPath == nil || embedded.RetainPeriod == nil {
		t.Fatalf("💣 embedded defaults: %v, %+v", err, embedded)
	}

	for _, testCase := range []struct {
		name, retentionText, retentionShown string
	}{
		{"negative", "-5", "-5"},
		{"string", "'thirty'", "thirty"},
		{"fraction", "1.5", "1.5"},
	} {
		t.Run("retention "+testCase.name, func(t *testing.T) {
			home := t.TempDir()
			writeSettings(t, home, "retain-period = "+testCase.retentionText+"\n")

			settings, err := Load(home)
			if err != nil || settings.RetainPeriod == nil || *settings.RetainPeriod != *embedded.RetainPeriod {
				t.Fatalf("✗ replaced retention period: %v, %v", err, settings.RetainPeriod)
			}

			if settings.InvalidRetainPeriod != testCase.retentionShown || settings.InvalidTrashPath != "" {
				t.Errorf("✗ reported values: %q, %q", settings.InvalidRetainPeriod, settings.InvalidTrashPath)
			}
		})
	}

	for _, condition := range []string{"file", "through file", "link loop", "dangling link", "number", "missing directory"} {
		t.Run("trash "+condition, func(t *testing.T) {
			home := t.TempDir()
			blockingFile := filepath.Join(home, "blocking-file")

			if err := os.WriteFile(blockingFile, []byte("blocking contents"), 0o600); err != nil {
				t.Fatalf("💣 blocking file: %v", err)
			}

			configuredPath, linkTarget := blockingFile, ""

			switch condition {
			case "through file":
				configuredPath = filepath.Join(blockingFile, "nested", "trash")
			case "link loop":
				linkTarget = filepath.Join(home, "loop")
				configuredPath = filepath.Join(linkTarget, "trash")
			case "dangling link":
				linkTarget = filepath.Join(home, "absent")
				configuredPath = filepath.Join(home, "dangling")
			case "missing directory":
				configuredPath = filepath.Join(home, "absent", "trash")
			}

			if linkTarget != "" {
				linkPath := filepath.Dir(configuredPath)
				if condition == "dangling link" {
					linkPath = configuredPath
				}

				if err := os.Symlink(linkTarget, linkPath); err != nil {
					t.Fatalf("💣 fixture link: %v", err)
				}
			}

			document, reportedValue := "trash-path = '"+configuredPath+"'\n", configuredPath
			if condition == "number" {
				document, reportedValue = "trash-path = 5\n", "5"
			}

			expectedTrash := *embedded.TrashPath
			if condition == "missing directory" || condition == "dangling link" {
				expectedTrash, reportedValue = configuredPath, ""
			}

			writeSettings(t, home, document)

			settings, err := Load(home)
			if err != nil || settings.TrashPath == nil {
				t.Fatalf("✗ load: %v, %+v", err, settings)
			}

			if *settings.TrashPath != expectedTrash || settings.InvalidTrashPath != reportedValue || settings.InvalidRetainPeriod != "" {
				t.Errorf("✗ trash %q, reported %q and %q", *settings.TrashPath, settings.InvalidTrashPath, settings.InvalidRetainPeriod)
			}

			if contents, err := os.ReadFile(blockingFile); err != nil || string(contents) != "blocking contents" {
				t.Errorf("✗ blocking file changed: %q, %v", contents, err)
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ values that can't be applied give way to the embedded ones")
	}
}

// FuzzDecode checks invariant #4: Arbitrary TOML.
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

// FuzzLoadRequiredSettings checks invariant #5: Loaded required settings.
// Test class: Expanded.
// Test layer: Fuzzing.
// Test kind: Permanent.
// What is under test:
// Whatever document the configuration file holds, Load either returns a
// configuration error or returns settings that have a retention period and a
// trash path that is not blank and that Trash resolves without an error.
func FuzzLoadRequiredSettings(f *testing.F) {
	f.Add("")
	f.Add(DefaultDocument)
	f.Add("trash-path = ''")
	f.Add("trash-path = \"\\t\\n\"\nretain-period = 1")
	f.Add("trash-path = 5")
	f.Fuzz(func(t *testing.T, document string) {
		home := t.TempDir()
		writeSettings(t, home, document)

		settings, err := Load(home)
		if err != nil {
			if !errors.Is(err, errs.ErrConfigDecode) {
				t.Errorf("✗ unexpected error: %v", err)
			}

			return
		}

		if settings.TrashPath == nil || strings.TrimSpace(*settings.TrashPath) == "" {
			t.Fatalf("✗ loaded settings have no trash path: %+v", settings.TrashPath)
		}

		if _, err := settings.Trash(home); err != nil {
			t.Errorf("✗ loaded trash path does not resolve: %v", err)
		}

		if settings.RetainPeriod == nil {
			t.Error("✗ loaded settings have no retention period")
		}
	})

	if !f.Failed() {
		f.Log("✓ every loaded configuration has its required settings")
	}
}
