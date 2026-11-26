package main

// Invariants tested:
//  1. Shared operations: GUI requests use the same history and filesystem behavior as the CLI.
//  2. Partial results: Errors preserve successful actions and individual failures.
//  3. Confirmation: Replacements wait for explicit consent and preserve their prompt and path.
//  4. Restore partial results: The adapter returns occupying deletes alongside review or history errors.
//  5. Linked responses: The adapter preserves both actions, complete consent metadata, and partial failures.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/saferm"
	"github.com/shdeen/saferm/internal/store"
)

// TestSharedOperations verifies invariant #1: Shared operations.
// Test class: Core.
// Test kind: Permanent.
func TestSharedOperations(t *testing.T) {
	homeDirectory := t.TempDir()
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	sourcePath := filepath.Join(homeDirectory, ".review")
	if err := os.WriteFile(sourcePath, []byte("recoverable contents"), 0o600); err != nil {
		t.Fatalf("💣 create source: %v", err)
	}

	writeReviewRules(t, homeDirectory, sourcePath)

	service := OperationService{}

	response := service.Run(t.Context(), &saferm.Request{Command: saferm.Delete, Source: sourcePath, Reason: "review this deletion", Requester: "human"})
	if response.Error != "" || len(response.Result.Actions) != 1 {
		t.Fatalf("💣 no action available for inspection: %+v", response)
	}

	action := response.Result.Actions[0]
	if action.Status != store.Pending || action.Requester != "human" {
		t.Errorf("✗ pending request metadata: %+v", action)
	}

	response = service.Run(t.Context(), &saferm.Request{Command: saferm.Dispose, ID: strconv.FormatInt(action.ID, 10), Decision: store.ApproveDecision})
	if response.Error != "" || len(response.Result.Actions) != 1 || response.Result.Actions[0].Status != store.Completed {
		t.Errorf("✗ approval: %+v", response)
	}

	response = service.Run(t.Context(), &saferm.Request{Command: saferm.Restore, ID: strconv.FormatInt(action.ID, 10)})
	if response.Error != "" {
		t.Errorf("✗ restore: %+v", response)
	}

	contents, err := os.ReadFile(sourcePath)
	if err != nil || string(contents) != "recoverable contents" {
		t.Errorf("✗ restored contents: %q, %v", contents, err)
	}

	listed, err := saferm.Run(t.Context(), &saferm.Request{Command: saferm.List, Status: "restored"})
	if err != nil || len(listed.Actions) != 1 || listed.Actions[0].ID != action.ID {
		t.Errorf("✗ shared history: %+v, %v", listed, err)
	}

	if !t.Failed() {
		t.Log("✓ GUI requests share review, recovery, and history")
	}
}

// TestPartialResults verifies invariant #2: Partial results.
// Test class: Core.
// Test kind: Permanent.
func TestPartialResults(t *testing.T) {
	homeDirectory := t.TempDir()
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	sourcePath := filepath.Join(homeDirectory, "disposable")
	if err := os.WriteFile(sourcePath, []byte("contents"), 0o600); err != nil {
		t.Fatalf("💣 create source: %v", err)
	}

	service := OperationService{}

	response := service.Run(t.Context(), &saferm.Request{Command: saferm.Delete, Source: sourcePath, Reason: "test recovery"})
	if len(response.Result.Actions) != 1 {
		t.Fatalf("💣 no action to purge: %+v", response)
	}

	actionID := strconv.FormatInt(response.Result.Actions[0].ID, 10)

	response = service.Run(t.Context(), &saferm.Request{Command: saferm.Purge, IDs: []string{actionID, "999999"}})
	if response.Error == "" || len(response.Result.Problems) != 1 || len(response.Result.Actions) != 1 || response.Result.Actions[0].Status != store.Purged {
		t.Errorf("✗ partial result lost: %+v", response)
	}

	if !t.Failed() {
		t.Log("✓ batch errors preserve successful outcomes")
	}
}

// TestConfirmation verifies invariant #3: Confirmation.
// Test class: Core.
// Test kind: Permanent.
func TestConfirmation(t *testing.T) {
	homeDirectory := t.TempDir()
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	if err := config.Setup(t.Context(), homeDirectory, false); err != nil {
		t.Fatalf("💣 install configuration: %v", err)
	}

	configPath := filepath.Join(homeDirectory, config.Directory, config.Filename)

	customDocument := "retain-period = 90\n"
	if err := os.WriteFile(configPath, []byte(customDocument), 0o600); err != nil {
		t.Fatalf("💣 customize configuration: %v", err)
	}

	service := OperationService{}
	request := saferm.Request{Command: saferm.Config, Setup: true, Reset: true}

	response := service.Run(t.Context(), &request)
	if response.Result.Confirmation == nil || response.Result.Confirmation.Path != configPath || response.Result.Confirmation.Prompt == "" {
		t.Errorf("✗ confirmation not preserved: %+v", response)
	}

	contents, err := os.ReadFile(configPath)
	if err != nil || string(contents) != customDocument {
		t.Errorf("✗ configuration changed before consent: %q, %v", contents, err)
	}

	request.Confirmed = true

	response = service.Run(t.Context(), &request)
	if response.Error != "" || response.Result.Config == nil || response.Result.Notice == "" {
		t.Errorf("✗ confirmed reset: %+v", response)
	}

	if !t.Failed() {
		t.Log("✓ configuration replacement requires consent")
	}
}

// TestRestorePartialResults verifies invariant #4: Restore partial results.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// A real restore first asks for consent. Its confirmed response preserves the
// occupying delete alongside an approval error or failed restore-history write.
func TestRestorePartialResults(t *testing.T) {
	for _, reviewOccupant := range []bool{false, true} {
		t.Run("review_occupant="+strconv.FormatBool(reviewOccupant), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)

			source := filepath.Join(home, "source")
			if err := os.WriteFile(source, []byte("original contents"), 0o600); err != nil {
				t.Fatalf("💣 original file: %v", err)
			}

			service := OperationService{}

			created := service.Run(t.Context(), &saferm.Request{Command: saferm.Delete, Source: source, Reason: "retain original"})
			if created.Error != "" || len(created.Result.Actions) != 1 {
				t.Fatalf("💣 original action: %+v", created)
			}

			original := created.Result.Actions[0]

			if err := os.WriteFile(source, []byte("occupying contents"), 0o600); err != nil {
				t.Fatalf("💣 occupant file: %v", err)
			}

			if reviewOccupant {
				document := fmt.Sprintf("trash-path = %q\nrequire-review = [%q]\n", original.TrashPath, source)
				if err := os.WriteFile(filepath.Join(home, config.Directory, config.Filename), []byte(document), 0o600); err != nil {
					t.Fatalf("💣 review configuration: %v", err)
				}
			} else {
				rejectRestoreRecording(t, home)
			}

			request := saferm.Request{Command: saferm.Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true}

			response := service.Run(t.Context(), &request)
			if response.Result.Confirmation == nil || response.Result.Confirmation.Path != original.Source || !strings.Contains(response.Error, errs.ErrConfirmation.Error()) {
				t.Errorf("✗ restore confirmation was discarded: %+v", response)
			}

			request.Confirmed = true

			response = service.Run(t.Context(), &request)
			if response.Error == "" || len(response.Result.Actions) == 0 || response.Result.Actions[0].ID == original.ID {
				t.Errorf("✗ restore error discarded occupying delete: %+v", response)

				return
			}

			deleted := response.Result.Actions[0]
			if reviewOccupant {
				if len(response.Result.Actions) != 1 || deleted.Status != store.Pending || !strings.Contains(response.Error, errs.ErrRestorePending.Error()) {
					t.Errorf("✗ pending deletion result: %+v", response)
				}
			} else {
				if len(response.Result.Actions) != 2 || response.Result.Actions[1].ID != original.ID || deleted.Status != store.Completed || !strings.Contains(response.Error, errs.ErrDatabaseWrite.Error()) {
					t.Errorf("✗ partial restore result: %+v", response)
				}

				settings, err := config.Load(home)
				if err != nil {
					t.Fatalf("💣 trash configuration: %v", err)
				}

				trashPath, err := settings.Trash(home)
				if err != nil {
					t.Fatalf("💣 trash location: %v", err)
				}

				contents, err := os.ReadFile(filepath.Join(trashPath, deleted.TrashID))
				if err != nil || string(contents) != "occupying contents" {
					t.Errorf("✗ recovery location lost occupant: %q, %v", contents, err)
				}
			}

			if !t.Failed() {
				t.Log("✓ restore case preserves its required outcome")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ desktop restore retains actions alongside errors")
	}
}

// TestLinkedResponses verifies invariant #5: Linked responses.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// A reviewed replacement returns its ordered pair. Combined decisions preserve
// both IDs and the literal path before consent. Approval and denial resolve both;
// a missing moving source returns its pending record with the completed delete and error.
func TestLinkedResponses(t *testing.T) {
	for _, decision := range []string{store.ApproveDecision, store.DenyDecision, "missing_source"} {
		t.Run(decision, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			source := filepath.Join(home, "source")

			destination := filepath.Join(home, ".$& {{.Path}}")
			for path, contents := range map[string]string{source: "moving contents", destination: "occupying contents"} {
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatalf("💣 prepare files: %v", err)
				}
			}

			writeReviewRules(t, home, filepath.Join(home, ".$& [{][{].Path[}][}]"))

			service := OperationService{}

			response := service.Run(t.Context(), &saferm.Request{Command: saferm.Move, Source: source, Destination: destination, Reason: "replace", Overwrite: true, Force: true})
			if response.Error != "" || len(response.Result.Actions) != 2 {
				t.Fatalf("💣 create reviewed pair: %+v", response)
			}

			moved, deleted := response.Result.Actions[0], response.Result.Actions[1]
			if moved.BlockerID != deleted.ID || moved.ID >= deleted.ID || moved.Status != store.Pending || deleted.Status != store.Pending {
				t.Errorf("✗ ordered pending pair: %+v", response)
			}

			request := saferm.Request{Command: saferm.Dispose, ID: strconv.FormatInt(moved.ID, 10), Decision: decision}
			if decision == "missing_source" {
				request.Decision = store.ApproveDecision
			}

			response = service.Run(t.Context(), &request)

			question := response.Result.Confirmation
			if question == nil {
				t.Fatalf("💣 no confirmation available: %+v", response)
			}

			if question.MoveID != moved.ID || question.BlockerID != deleted.ID || question.Path != deleted.Source || !strings.Contains(response.Error, errs.ErrConfirmation.Error()) || len(response.Result.Actions) != 2 {
				t.Errorf("✗ complete confirmation response: %+v", response)
			}

			if decision == "missing_source" {
				removed := service.Run(t.Context(), &saferm.Request{Command: saferm.Delete, Source: source, Reason: "remove moving source before approval"})
				if removed.Error != "" {
					t.Fatalf("💣 remove source: %+v", removed)
				}
			}

			request.Confirmed = true

			response = service.Run(t.Context(), &request)
			if len(response.Result.Actions) != 2 {
				t.Errorf("✗ linked result lost actions: %+v", response)

				return
			}

			for _, action := range response.Result.Actions {
				status := store.Completed
				if decision == store.DenyDecision {
					status = store.Denied
				} else if decision == "missing_source" && action.ID == moved.ID {
					status = store.Pending
				}

				if action.Status != status {
					t.Errorf("✗ final action status: %+v", action)
				}
			}

			if decision == "missing_source" {
				if !strings.Contains(response.Error, errs.ErrFileInspect.Error()) {
					t.Errorf("✗ partial error lost: %+v", response)
				}
			} else if response.Error != "" {
				t.Errorf("✗ linked decision error: %s", response.Error)
			}

			path := deleted.Source
			contents := "occupying contents"

			if decision != store.DenyDecision {
				path = filepath.Join(deleted.TrashPath, deleted.TrashID)
			}

			preserved, err := os.ReadFile(path)
			if err != nil || string(preserved) != contents {
				t.Errorf("✗ occupant not preserved: %q, %v", preserved, err)
			}

			if !t.Failed() {
				t.Log("✓ linked response preserves actions, consent, and errors")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ desktop linked outcomes are complete")
	}
}

// Test class: Core: Helper.
func writeReviewRules(t *testing.T, home string, patterns ...string) {
	t.Helper()

	configDirectory := filepath.Join(home, config.Directory)
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		t.Fatalf("💣 configuration directory: %v", err)
	}

	quotedPatterns := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		quotedPatterns = append(quotedPatterns, strconv.Quote(pattern))
	}

	document := fmt.Sprintf("trash-path = %q\nrequire-review = [%s]\n", filepath.Join(home, "trash"), strings.Join(quotedPatterns, ", "))
	if err := os.WriteFile(filepath.Join(configDirectory, config.Filename), []byte(document), 0o600); err != nil {
		t.Fatalf("💣 review configuration: %v", err)
	}
}

// Test class: Core: Helper.
func rejectRestoreRecording(t *testing.T, home string) {
	t.Helper()

	database, err := sql.Open("sqlite", filepath.Join(home, config.Directory, config.Database))
	if err != nil {
		t.Fatalf("💣 fixture history: %v", err)
	}

	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("✗ close fixture history: %v", err)
		}
	})

	if _, err := database.ExecContext(t.Context(), `CREATE TRIGGER reject_restore BEFORE UPDATE ON actions WHEN NEW.status=4 BEGIN SELECT RAISE(FAIL,'restore recording rejected'); END`); err != nil {
		t.Fatalf("💣 recording failure fixture: %v", err)
	}
}
