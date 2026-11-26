package config

// Invariants tested:
//  1. Required settings: The trash path and the retention period are the ones the configuration file sets, or the embedded ones when the file is absent, omits them, or sets the trash path blank; omitted review rules stay unset.

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestLoadRequiredSettings verifies invariant #1: Required settings.
// Test class: Expanded.
// Test layer: Coverage.
// What is under test:
// With no configuration file, the loaded trash path and retention period equal
// the embedded ones. A file that omits trash-path or sets it to an empty or
// blank value loads the embedded trash path, and a file that omits
// retain-period loads the embedded retention period. A file that sets either
// one keeps its own value. In every case Trash resolves the loaded path to an
// absolute path, a file that omits require-review leaves it unset, and no value
// is reported as invalid.
func TestLoadRequiredSettings(t *testing.T) {
	embedded, err := Decode(DefaultDocument)
	if err != nil || embedded.TrashPath == nil || embedded.RetainPeriod == nil {
		t.Fatalf("💣 embedded defaults: %v, %+v", err, embedded)
	}

	configuredTrash := filepath.Join(t.TempDir(), "configured-trash")
	configuredDays := *embedded.RetainPeriod + 7

	for _, testCase := range []struct {
		name          string
		document      *string
		trashPath     string
		retentionDays int
	}{
		{"no file", nil, *embedded.TrashPath, *embedded.RetainPeriod},
		{"omitted", new(""), *embedded.TrashPath, *embedded.RetainPeriod},
		{"empty trash", new("trash-path = ''\n"), *embedded.TrashPath, *embedded.RetainPeriod},
		{"blank trash", new("trash-path = ' \t '\n"), *embedded.TrashPath, *embedded.RetainPeriod},
		{"configured trash", new("trash-path = '" + configuredTrash + "'\n"), configuredTrash, *embedded.RetainPeriod},
		{"configured retention", new("retain-period = " + strconv.Itoa(configuredDays) + "\n"), *embedded.TrashPath, configuredDays},
		{"zero retention", new("retain-period = 0\n"), *embedded.TrashPath, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			home := t.TempDir()
			if testCase.document != nil {
				writeSettings(t, home, *testCase.document)
			}

			settings, err := Load(home)
			if err != nil || settings.TrashPath == nil || *settings.TrashPath != testCase.trashPath {
				t.Fatalf("✗ loaded trash path: %v, %+v", err, settings.TrashPath)
			}

			if settings.RetainPeriod == nil || *settings.RetainPeriod != testCase.retentionDays {
				t.Errorf("✗ loaded retention period: got %v, want %d", settings.RetainPeriod, testCase.retentionDays)
			}

			if resolved, err := settings.Trash(home); err != nil || !filepath.IsAbs(resolved) {
				t.Errorf("✗ resolved trash path: %q, %v", resolved, err)
			}

			if testCase.document != nil && settings.RequireReview != nil {
				t.Errorf("✗ omitted require-review inherited a value: %q", settings.RequireReview)
			}

			if settings.InvalidTrashPath != "" || settings.InvalidRetainPeriod != "" {
				t.Errorf("✗ applicable values reported as invalid: %q, %q", settings.InvalidTrashPath, settings.InvalidRetainPeriod)
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ required settings always have a value")
	}
}

// writeSettings writes the document as the configuration file under the home
// directory.
func writeSettings(t *testing.T, home, document string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(home, Directory), 0o700); err != nil {
		t.Fatalf("💣 configuration directory: %v", err)
	}

	if err := os.WriteFile(filepath.Join(home, Directory, Filename), []byte(document), 0o600); err != nil {
		t.Fatalf("💣 configuration file: %v", err)
	}
}
