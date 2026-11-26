package desktop

// Invariants tested:
//  1. Optional installation: A GUI without SafeRM Notify does not create a registration marker.
//  2. User opt-outs: Reopening a configured GUI never re-registers SafeRM Notify.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shdeen/saferm/internal/config"
)

// TestUnpackagedGUI verifies invariant #1: Optional installation.
// Test class: Expanded.
// Test layer: Coverage.
func TestUnpackagedGUI(t *testing.T) {
	home := t.TempDir()
	if err := Setup(home, filepath.Join(home, "saferm-app")); err != nil {
		t.Errorf("✗ unpackaged GUI setup: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, config.Directory, setupFile)); !os.IsNotExist(err) {
		t.Errorf("✗ marker without SafeRM Notify: %v", err)
	}

	if !t.Failed() {
		t.Log("✓ unpackaged GUI makes no registration")
	}
}

// TestExistingSetup verifies invariant #2: User opt-outs.
// Test class: Expanded.
// Test layer: Coverage.
func TestExistingSetup(t *testing.T) {
	home := t.TempDir()

	marker := filepath.Join(home, config.Directory, setupFile)
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatalf("💣 setup directory: %v", err)
	}

	contents := []byte("previous registration\n")
	if err := os.WriteFile(marker, contents, 0o600); err != nil {
		t.Fatalf("💣 setup marker: %v", err)
	}

	if err := Setup(home, filepath.Join(home, "saferm-app")); err != nil {
		t.Errorf("✗ subsequent GUI launch: %v", err)
	}

	stored, err := os.ReadFile(marker)
	if err != nil || string(stored) != string(contents) {
		t.Errorf("✗ subsequent launch changed registration: %q, %v", stored, err)
	}

	if !t.Failed() {
		t.Log("✓ subsequent launches preserve registration choices")
	}
}
