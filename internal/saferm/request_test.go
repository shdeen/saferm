package saferm

// Invariants tested:
//  1. Configuration selection: An active file replaces embedded defaults completely.
//  2. Configuration setup: Existing files survive without confirmed reset.
//  3. Recoverable deletion: Deletion stores unique identities and restore uses the current trash directory.
//  4. Reversible movement: Moves preserve contents, refuse replacement, and restore to the source.
//  5. Source review: Matching sources wait for approval or denial.
//  6. History inspection: Filtering and inspection preserve complete action information.
//  7. Permanent disposal: Purge targets deleted items only and reports missing items.
//  8. Record maintenance: Cleanup retains recoverable files and pending requests without deleting contents.
//  9. ID parsing: IDs are positive decimal integers.
//  10. Status parsing: Filters use catalog names or their numeric values.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/errs"

	"github.com/shdeen/saferm/internal/store"
)

// TestConfigSelection verifies invariant #1: Configuration selection.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Embedded defaults are present without an active file. An empty active file
// leaves every key unset; a move still works without trash or retention settings.
func TestConfigSelection(t *testing.T) {
	home := isolateHome(t)

	result, err := Run(t.Context(), &Request{Command: "config"})
	if err != nil || result.Config == nil {
		t.Errorf("✗ default config: %v, %+v", err, result)

		return
	}

	if result.Config.TrashPath == nil || result.Config.RetainPeriod == nil {
		t.Error("✗ embedded settings absent")
	}

	writeConfig(t, home, "")

	result, err = Run(t.Context(), &Request{Command: "config"})
	if err != nil || result.Config == nil {
		t.Errorf("✗ active config: %v", err)

		return
	}

	if result.Config.TrashPath != nil || result.Config.RetainPeriod != nil || len(result.Config.RequireReview) != 0 {
		t.Error("✗ active config inherited defaults")
	}

	source := filepath.Join(home, "source")
	writeFile(t, source, "contents")

	_, err = Run(t.Context(), &Request{Command: "move", Source: source, Destination: filepath.Join(home, "destination"), Reason: "relocate"})
	if err != nil {
		t.Errorf("✗ move required unused configuration: %v", err)
	}

	if !t.Failed() {
		t.Log("✓ configuration selection")
	}
}

// TestConfigSetup verifies invariant #2: Configuration setup.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Setup creates defaults, preserves edited content, requests reset confirmation,
// and only replaces it after confirmation. Default display ignores malformed active TOML.
func TestConfigSetup(t *testing.T) {
	home := isolateHome(t)

	_, err := Run(t.Context(), &Request{Command: "config", Setup: true})
	if err != nil {
		t.Errorf("✗ setup: %v", err)
	}

	configPath := filepath.Join(home, ".saferm", "config.toml")

	defaults, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Errorf("✗ setup file missing: %v", readErr)
	}

	writeConfig(t, home, "invalid = [")
	_, _ = Run(t.Context(), &Request{Command: "config", Setup: true})
	assertFile(t, configPath, "invalid = [")

	result, err := Run(t.Context(), &Request{Command: "config", Setup: true, Reset: true})
	if result.Confirmation == nil {
		t.Errorf("✗ reset lacks confirmation: %v", err)
	}

	assertFile(t, configPath, "invalid = [")

	result, err = Run(t.Context(), &Request{Command: "config", Default: true})
	if err != nil || result.Document != string(defaults) {
		t.Errorf("✗ default display consulted active file: %v", err)
	}

	_, err = Run(t.Context(), &Request{Command: "config", Setup: true, Reset: true, Confirmed: true})
	if err != nil {
		t.Errorf("✗ confirmed reset: %v", err)
	}

	assertFile(t, configPath, string(defaults))

	if !t.Failed() {
		t.Log("✓ setup preserves existing configuration")
	}
}

// TestDeleteRestore verifies invariant #3: Recoverable deletion.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Distinct deletions with the same basename retain separate contents. After trash
// relocation, restoration uses the configured root and keeps the historical path.
func TestDeleteRestore(t *testing.T) {
	home := configuredHome(t, 30)
	sourceA, sourceB := filepath.Join(home, "a", "file"), filepath.Join(home, "b", "file")
	writeFile(t, sourceA, "alpha")
	writeFile(t, sourceB, "beta")
	actionA := performAction(t, Request{Command: "delete", Source: sourceA, Reason: "remove alpha"})

	actionB := performAction(t, Request{Command: "delete", Source: sourceB, Reason: "remove beta"})
	if actionA.ID == actionB.ID || actionA.TrashID == actionB.TrashID {
		t.Error("✗ trash identities collided")
	}

	assertAbsent(t, sourceA)
	assertAbsent(t, sourceB)

	oldTrash := filepath.Join(home, "trash")

	newTrash := filepath.Join(home, "relocated")
	if err := os.Rename(oldTrash, newTrash); err != nil {
		t.Fatalf("💣 fixture relocation: %v", err)
	}

	writeConfig(t, home, fmt.Sprintf("trash-path = %q\nretain-period = 30\n", newTrash))

	restored := performAction(t, Request{Command: "restore", ID: strconv.FormatInt(actionA.ID, 10)})
	if restored.Status != 4 || restored.TrashPath != oldTrash {
		t.Errorf("✗ restore record: %+v", restored)
	}

	assertFile(t, sourceA, "alpha")
	assertFile(t, filepath.Join(newTrash, actionB.TrashID), "beta")

	if !t.Failed() {
		t.Log("✓ independent recoverable deletions")
	}
}

// TestMoveRestore verifies invariant #4: Reversible movement.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// A moved directory retains its nested file, refuses an occupied source on restore,
// requests confirmation for overwrite, and preserves the occupant with overwrite plus force.
func TestMoveRestore(t *testing.T) {
	home := configuredHome(t, 30)
	source, destination := filepath.Join(home, "source"), filepath.Join(home, "destination")
	writeFile(t, filepath.Join(source, "nested", "file"), "payload")
	action := performAction(t, Request{Command: "move", Source: source, Destination: destination, Reason: "relocate"})
	assertAbsent(t, source)
	assertFile(t, filepath.Join(destination, "nested", "file"), "payload")
	writeFile(t, source, "occupant")

	_, err := Run(t.Context(), &Request{Command: "restore", ID: strconv.FormatInt(action.ID, 10), Force: true})
	if err == nil {
		t.Error("✗ force alone replaced source")
	}

	assertFile(t, source, "occupant")

	result, _ := Run(t.Context(), &Request{Command: "restore", ID: strconv.FormatInt(action.ID, 10), Overwrite: true})
	if result.Confirmation == nil {
		t.Error("✗ overwrite did not request confirmation")
	}

	assertFile(t, source, "occupant")

	result, err = Run(t.Context(), &Request{Command: "restore", ID: strconv.FormatInt(action.ID, 10), Overwrite: true, Force: true})
	if err != nil || len(result.Actions) != 2 {
		t.Errorf("✗ recoverable replacement: %+v, %v", result, err)
	} else {
		if result.Actions[1].ID != action.ID || result.Actions[1].Status != store.Restored {
			t.Errorf("✗ restored original: %+v", result.Actions[1])
		}

		assertFile(t, filepath.Join(home, "trash", result.Actions[0].TrashID), "occupant")
	}

	assertFile(t, filepath.Join(source, "nested", "file"), "payload")
	assertAbsent(t, destination)

	if !t.Failed() {
		t.Log("✓ move reversal and replacement consent")
	}
}

// TestReviewDecisions verifies invariant #5: Source review.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// A home dotfile and a .git directory that review rules name wait for review.
// Approval executes once and denial leaves files.
func TestReviewDecisions(t *testing.T) {
	home := isolateHome(t)
	secret := filepath.Join(home, ".secret")
	gitPath := filepath.Join(home, "project", ".git")
	writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = [%q, %q]\n", filepath.Join(home, "trash"), secret, gitPath))
	writeFile(t, secret, "protected")

	action := performAction(t, Request{Command: "delete", Source: secret, Reason: "review"})
	if action.Status != 2 || action.ReviewRule == "" {
		t.Errorf("✗ pending record: %+v", action)
	}

	assertFile(t, secret, "protected")

	denied := performAction(t, Request{Command: "dispose", ID: strconv.FormatInt(action.ID, 10), Decision: "deny"})
	if denied.Status != 3 {
		t.Errorf("✗ denial status: %d", denied.Status)
	}

	assertFile(t, secret, "protected")

	_, err := Run(t.Context(), &Request{Command: "dispose", ID: strconv.FormatInt(action.ID, 10), Decision: "approve"})
	if err == nil {
		t.Error("✗ resolved decision repeated")
	}

	writeFile(t, filepath.Join(gitPath, "config"), "git data")

	action = performAction(t, Request{Command: "move", Source: gitPath, Destination: filepath.Join(home, "git-moved"), Reason: "review"})
	if action.Status != 2 {
		t.Error("✗ .git directory did not wait")
	}

	approved := performAction(t, Request{Command: "dispose", ID: strconv.FormatInt(action.ID, 10), Decision: "approve"})
	if approved.Status != 1 {
		t.Errorf("✗ approval: %+v", approved)
	}

	if !t.Failed() {
		t.Log("✓ source review decisions")
	}
}

// TestInspectHistory verifies invariant #6: History inspection.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// List filters combine type and status; view returns the recorded reason and optional metadata.
func TestInspectHistory(t *testing.T) {
	home := configuredHome(t, 30)
	source := filepath.Join(home, "source")
	writeFile(t, source, "payload")
	action := performAction(t, Request{Command: "delete", Source: source, Reason: "reason", Requester: "requester", Description: "description"})

	listed, err := Run(t.Context(), &Request{Command: "list", Type: "delete", Status: "completed"})
	if err != nil || len(listed.Actions) != 1 {
		t.Errorf("✗ filtered actions: %+v, %v", listed, err)
	}

	listed, err = Run(t.Context(), &Request{Command: "list", Type: "move"})
	if err != nil || len(listed.Actions) != 0 {
		t.Errorf("✗ type filter: %+v, %v", listed, err)
	}

	viewed := performAction(t, Request{Command: "view", ID: strconv.FormatInt(action.ID, 10)})
	if !reflect.DeepEqual(action, viewed) {
		t.Errorf("✗ action changed during inspection: %+v", viewed)
	}

	encoded, err := json.Marshal(viewed)
	if err != nil || !json.Valid(encoded) {
		t.Errorf("✗ action JSON: %v", err)
	}

	if !t.Failed() {
		t.Log("✓ history filters and inspection")
	}
}

// TestPurge verifies invariant #7: Permanent disposal.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Purge deletes a selected trashed file and keeps its record; it rejects a move
// without touching its destination and does not claim a missing item was removed.
func TestPurge(t *testing.T) {
	home := configuredHome(t, 30)
	source := filepath.Join(home, "source")
	writeFile(t, source, "trash")
	deleted := performAction(t, Request{Command: "delete", Source: source, Reason: "purge"})

	result, err := Run(t.Context(), &Request{Command: "purge", IDs: []string{strconv.FormatInt(deleted.ID, 10)}})
	if err != nil || len(result.Actions) != 1 || result.Actions[0].Status != 5 {
		t.Errorf("✗ purge: %+v, %v", result, err)
	}

	assertAbsent(t, filepath.Join(home, "trash", deleted.TrashID))
	writeFile(t, source, "move")
	moved := performAction(t, Request{Command: "move", Source: source, Destination: filepath.Join(home, "destination"), Reason: "relocate"})

	_, err = Run(t.Context(), &Request{Command: "purge", IDs: []string{strconv.FormatInt(moved.ID, 10)}})
	if err == nil {
		t.Error("✗ move accepted for purge")
	}

	assertFile(t, moved.Destination, "move")

	_, err = Run(t.Context(), &Request{Command: "purge", IDs: []string{strconv.FormatInt(deleted.ID, 10)}})
	if err == nil {
		t.Error("✗ repeated purge claimed removal")
	}

	if !t.Failed() {
		t.Log("✓ bounded permanent disposal")
	}
}

// TestCleanup verifies invariant #8: Record maintenance.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Zero-day cleanup removes eligible move history but leaves its destination and
// recoverable deleted contents; orphan cleanup removes a manually missing deletion.
func TestCleanup(t *testing.T) {
	home := configuredHome(t, 0)
	source := filepath.Join(home, "source")
	writeFile(t, source, "recoverable")
	deleted := performAction(t, Request{Command: "delete", Source: source, Reason: "retain"})
	writeFile(t, source, "move")
	moved := performAction(t, Request{Command: "move", Source: source, Destination: filepath.Join(home, "destination"), Reason: "relocate"})

	result, err := Run(t.Context(), &Request{Command: "cleanup"})
	if err != nil || !reflect.DeepEqual(result.Removed, []int64{moved.ID}) {
		t.Errorf("✗ cleanup: %+v, %v", result, err)
	}

	assertFile(t, moved.Destination, "move")

	trashFile := filepath.Join(home, "trash", deleted.TrashID)
	assertFile(t, trashFile, "recoverable")

	if err = os.Rename(trashFile, filepath.Join(home, "manual-relocation")); err != nil {
		t.Fatalf("💣 fixture relocation: %v", err)
	}

	result, err = Run(t.Context(), &Request{Command: "cleanup", Orphans: true})
	if err != nil || !reflect.DeepEqual(result.Removed, []int64{deleted.ID}) {
		t.Errorf("✗ orphan cleanup: %+v, %v", result, err)
	}

	assertFile(t, filepath.Join(home, "manual-relocation"), "recoverable")

	if !t.Failed() {
		t.Log("✓ maintenance preserves filesystem contents")
	}
}

// TestIDParsing verifies invariant #9: ID parsing.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// IDs accept positive decimal integers, including padded values; invalid, empty,
// negative, and overflowing input fails.
func TestIDParsing(t *testing.T) {
	for _, testCase := range []struct {
		input string
		id    int64
	}{{"1", 1}, {"0002", 2}, {"9223372036854775807", 9223372036854775807}, {"", 0}, {"0", 0}, {"-1", 0}, {"word", 0}, {"9223372036854775808", 0}} {
		id, err := parseID(testCase.input)
		if testCase.id > 0 {
			if err != nil || id != testCase.id {
				t.Errorf("✗ ID %q: %d, %v", testCase.input, id, err)
			}
		} else if !errors.Is(err, errs.ErrInputInvalid) && !errors.Is(err, errs.ErrInputMissing) {
			t.Errorf("✗ invalid ID %q: %v", testCase.input, err)
		}
	}

	if !t.Failed() {
		t.Log("✓ IDs follow the input contract")
	}
}

// TestStatusParsing verifies invariant #10: Status parsing.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Catalog names and numeric values select the same statuses. Empty status means
// no filter; unsupported names and values fail with the invalid-input error.
func TestStatusParsing(t *testing.T) {
	for status := store.Completed; status <= store.Failed; status++ {
		for _, input := range []string{strconv.Itoa(status), appstrings.Text("status." + strconv.Itoa(status))} {
			parsed, err := parseStatus(input)
			if err != nil || parsed != status {
				t.Errorf("✗ status %q: %d, %v", input, parsed, err)
			}
		}
	}

	if parsed, err := parseStatus(""); err != nil || parsed != 0 {
		t.Errorf("✗ no status filter: %d, %v", parsed, err)
	}

	for _, input := range []string{"0", "7", "-1", "invalid", "executing"} {
		if _, err := parseStatus(input); !errors.Is(err, errs.ErrInputInvalid) {
			t.Errorf("✗ invalid status %q: %v", input, err)
		}
	}

	if !t.Failed() {
		t.Log("✓ status names follow the catalog")
	}
}

// Test class: Core: Helper.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	return home
}

// Test class: Core: Helper.
func configuredHome(t *testing.T, days int) string {
	t.Helper()
	home := isolateHome(t)
	writeConfig(t, home, fmt.Sprintf("trash-path = %q\nretain-period = %d\n", filepath.Join(home, "trash"), days))

	return home
}

// Test class: Core: Helper.
func writeConfig(t *testing.T, home, document string) {
	t.Helper()
	writeFile(t, filepath.Join(home, ".saferm", "config.toml"), document)
}

// Test class: Core: Helper.
func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("💣 fixture directory: %v", err)
	}

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("💣 fixture file: %v", err)
	}
}

// Test class: Core: Helper.
func assertFile(t *testing.T, path, contents string) {
	t.Helper()

	fileBytes, err := os.ReadFile(path)
	if err != nil || string(fileBytes) != contents {
		t.Errorf("✗ file %q: %q, %v", path, fileBytes, err)
	}
}

// Test class: Core: Helper.
func assertAbsent(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("✗ path %q not absent: %v", path, err)
	}
}

// Test class: Core: Helper.
func performAction(t *testing.T, request Request) store.Action {
	t.Helper()

	result, err := Run(context.Background(), &request)
	if err != nil || len(result.Actions) != 1 {
		t.Errorf("✗ %s: %+v, %v", request.Command, result, err)

		return store.Action{}
	}

	return result.Actions[0]
}
