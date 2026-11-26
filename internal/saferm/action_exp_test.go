package saferm

// Invariants tested:
//  1. Failed transfers: A failed transfer retains source data and records its diagnostic.
//  2. History preservation: Transfers cannot remove the active database or a directory containing it.
//  3. Restore failure results: A completed occupant delete remains recoverable when later recording or claiming fails.
//  4. Restore preparation: Invalid or ambiguous replacement never claims the original or removes the occupant.
//  5. Restore transfer failures: Failed occupant or original transfers retain both items and record their outcomes.
//  6. Failed execution: A failed transfer records its outcome and diagnostic after execution.
//  7. Restore refusal: Ineligible records, unsafe recovery paths, and rejected review recording preserve the original.
//  8. Competing restore claims: A changed original cannot be claimed, while its completed occupying delete remains recoverable.
//  9. Linked refusal: Missing files leave linked requests pending and deniable.
//  10. Linked failures: Blocker failure denies its move, while dependent errors do not stop other eligible decisions.
//  11. Fresh consent: Decisions reread records after confirmation and allow direct decisions after blocker removal.
//  12. Move preparation: Refused replacement changes neither files nor history and asks no question.
//  13. Move failure results: Failed transfer or pair recording preserves both items and returns recoverable partial results.
//  14. Retained trash location: Replacement cannot relocate the trash used to preserve its occupant.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/store"
)

// TestTransferAfterChangedTrash verifies invariant #1: Failed transfers.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A trash directory made unusable after request preparation causes transfer failure.
// The original source survives and the persisted action contains failed status,
// its recovery identity, and a diagnostic.
func TestTransferAfterChangedTrash(t *testing.T) {
	home := configuredHome(t, 30)
	source := filepath.Join(home, "source")
	writeFile(t, source, "payload")

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("💣 fixture config: %v", err)
	}

	action, _, err := prepareAction(&Request{Command: Delete, Source: source, Reason: "remove"}, home, settings)
	if err != nil {
		t.Fatalf("💣 fixture preparation: %v", err)
	}

	history, err := store.Open(t.Context(), filepath.Join(home, config.Directory, config.Database))
	if err != nil {
		t.Fatalf("💣 fixture history: %v", err)
	}

	t.Cleanup(func() {
		if err := history.Close(); err != nil {
			t.Errorf("✗ history close: %v", err)
		}
	})

	writeFile(t, filepath.Join(home, "trash"), "blocking file")

	result, err := runRecordedAction(t.Context(), home, settings, history, &action)
	if err == nil || len(result.Actions) != 1 {
		t.Errorf("✗ failed transfer: %+v, %v", result, err)
	}

	viewed, err := history.Get(t.Context(), action.ID)
	if err != nil || viewed.Status != store.Failed || viewed.Failure == "" || viewed.TrashID != action.TrashID {
		t.Errorf("✗ failure record: %+v, %v", viewed, err)
	}

	assertFile(t, source, "payload")

	if !t.Failed() {
		t.Log("✓ failures retain data and diagnostics")
	}
}

// TestHistoryPreservation verifies invariant #2: History preservation.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A symbolic home path does not allow moving its database directory, and direct
// deletion of the active database is refused before any history is moved.
func TestHistoryPreservation(t *testing.T) {
	for _, sourceIsDatabase := range []bool{false, true} {
		home := configuredHome(t, 30)

		alias := filepath.Join(t.TempDir(), "home")
		if err := os.Symlink(home, alias); err != nil {
			t.Fatalf("💣 home alias: %v", err)
		}

		t.Setenv("HOME", alias)
		t.Setenv("USERPROFILE", alias)

		if _, err := Run(t.Context(), &Request{Command: List}); err != nil {
			t.Fatalf("💣 initialize history: %v", err)
		}

		databasePath := filepath.Join(home, config.Directory, config.Database)

		originalHistory, err := os.ReadFile(databasePath)
		if err != nil {
			t.Fatalf("💣 read initialized history: %v", err)
		}

		source := filepath.Join(alias, config.Directory)

		request := Request{Command: Move, Source: source, Destination: filepath.Join(t.TempDir(), "moved"), Reason: "relocate"}
		if sourceIsDatabase {
			request.Command = Delete
			request.Source = filepath.Join(source, config.Database)
		}

		if _, err := Run(t.Context(), &request); !errors.Is(err, errs.ErrFileOverlap) {
			t.Errorf("✗ active history transfer allowed: %v", err)
		}

		preservedHistory, err := os.ReadFile(databasePath)
		if err != nil || !bytes.Equal(originalHistory, preservedHistory) {
			t.Errorf("✗ active history changed: %v", err)
		}
	}

	if !t.Failed() {
		t.Log("✓ active history remains in place")
	}
}

// TestRestoreRecordingFailures verifies invariant #3: Restore failure results.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Real SQLite failures reject the occupant insertion, the original claim, or
// the restore outcome. Each response retains the occupying delete's location;
// the original is untouched unless its restoration actually began.
func TestRestoreRecordingFailures(t *testing.T) {
	for _, failureAt := range []string{"occupant insertion", "original claim", "restore outcome"} {
		t.Run(failureAt, func(t *testing.T) {
			home := configuredHome(t, 30)
			source := filepath.Join(home, "source")
			writeFile(t, source, "original contents")
			original := performAction(t, Request{Command: Delete, Source: source, Reason: "retain original"})
			writeFile(t, source, "occupying contents")

			var query string

			switch failureAt {
			case "occupant insertion":
				query = `CREATE TRIGGER reject_occupant BEFORE INSERT ON actions BEGIN SELECT RAISE(FAIL,'occupant recording rejected'); END`
			case "original claim":
				query = `CREATE TRIGGER reject_restore_claim BEFORE UPDATE ON actions WHEN NEW.in_progress=1 BEGIN SELECT RAISE(FAIL,'restore claim rejected'); END`
			case "restore outcome":
				query = `CREATE TRIGGER reject_restore_outcome BEFORE UPDATE ON actions WHEN NEW.status=4 BEGIN SELECT RAISE(FAIL,'restore outcome rejected'); END`
			}

			executeSQL(t, home, query)

			result, err := Run(t.Context(), &Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true, Force: true})
			if !errors.Is(err, errs.ErrDatabaseWrite) || len(result.Actions) == 0 {
				t.Errorf("✗ recording failure omitted recovery result: %+v, %v", result, err)

				return
			}

			deleted := result.Actions[0]
			if deleted.Type != Delete || deleted.ID == original.ID || deleted.Status != store.Completed || deleted.TrashID == original.TrashID {
				t.Errorf("✗ completed occupying delete omitted: %+v", deleted)
			}

			assertFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")

			if failureAt == "occupant insertion" {
				if !errors.Is(err, errs.ErrDatabaseResult) || deleted.ID != 0 {
					t.Errorf("✗ unrecorded deletion did not identify unsaved filesystem result: %+v, %v", deleted, err)
				}
			} else {
				recorded := performAction(t, Request{Command: View, ID: strconv.FormatInt(deleted.ID, 10)})
				if !reflect.DeepEqual(recorded, deleted) {
					t.Errorf("✗ completed delete missing from history: %+v", recorded)
				}
			}

			if failureAt == "restore outcome" {
				assertFile(t, source, "original contents")

				if len(result.Actions) != 2 || result.Actions[1].ID != original.ID {
					t.Errorf("✗ partial restore result omitted original: %+v", result)
				}
			} else {
				assertAbsent(t, source)
				assertFile(t, filepath.Join(home, "trash", original.TrashID), "original contents")

				recorded := performAction(t, Request{Command: View, ID: strconv.FormatInt(original.ID, 10)})
				if !reflect.DeepEqual(recorded, original) {
					t.Errorf("✗ original changed before its claim: %+v", recorded)
				}
			}

			if !t.Failed() {
				t.Log("✓ restore recording failure retains recoverable occupant and partial results")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ each recording failure preserves the occupying item")
	}
}

// TestRestorePreparationRefusals verifies invariant #4: Restore preparation.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A failed original with an occupied source, or an invalid trash directory for the
// occupying delete, refuses replacement without changing files or history.
func TestRestorePreparationRefusals(t *testing.T) {
	for _, failedOriginal := range []bool{false, true} {
		t.Run("failed_original="+strconv.FormatBool(failedOriginal), func(t *testing.T) {
			home := configuredHome(t, 30)
			source, destination := filepath.Join(home, "source"), filepath.Join(home, "destination")
			writeFile(t, source, "original contents")
			original := performAction(t, Request{Command: Move, Source: source, Destination: destination, Reason: "retain original"})
			writeFile(t, source, "occupying contents")

			if failedOriginal {
				executeSQL(t, home, `UPDATE actions SET status=? WHERE action_id=?`, store.Failed, original.ID)
				original.Status = store.Failed
			} else {
				writeFile(t, filepath.Join(home, "trash"), "invalid trash directory")
			}

			result, err := Run(t.Context(), &Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true, Force: true})
			if err == nil || len(result.Actions) != 0 {
				t.Errorf("✗ invalid restore changed actions: %+v, %v", result, err)
			}

			if failedOriginal && !errors.Is(err, errs.ErrActionAmbiguous) {
				t.Errorf("✗ failed original did not report ambiguity: %v", err)
			}

			recorded := performAction(t, Request{Command: View, ID: strconv.FormatInt(original.ID, 10)})
			if !reflect.DeepEqual(recorded, original) {
				t.Errorf("✗ original changed before preparation succeeded: %+v", recorded)
			}

			assertFile(t, source, "occupying contents")
			assertFile(t, destination, "original contents")

			if !t.Failed() {
				t.Log("✓ restore case preserves its required outcome")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ failed preparation preserves both items and the original record")
	}
}

// TestRestoreTransferFailures verifies invariant #5: Restore transfer failures.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Filesystem restrictions prevent either the occupying delete or the original restore.
// The former leaves the original record untouched; the latter records a failed
// restore and retains the completed occupying delete and its recoverable bytes.
func TestRestoreTransferFailures(t *testing.T) {
	for _, failOriginalTransfer := range []bool{false, true} {
		t.Run("original_transfer="+strconv.FormatBool(failOriginalTransfer), func(t *testing.T) {
			home := configuredHome(t, 30)
			source, destination := filepath.Join(home, "original", "source"), filepath.Join(home, "stored", "destination")
			writeFile(t, source, "original contents")

			if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
				t.Fatalf("💣 destination directory: %v", err)
			}

			original := performAction(t, Request{Command: Move, Source: source, Destination: destination, Reason: "retain original"})
			writeFile(t, source, "occupying contents")

			if failOriginalTransfer {
				preventFileTransfer(t, destination)
			} else {
				preventFileTransfer(t, source)
			}

			result, err := Run(t.Context(), &Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true, Force: true})
			if !errors.Is(err, errs.ErrFileTransfer) || len(result.Actions) == 0 {
				t.Errorf("✗ transfer failure omitted result: %+v, %v", result, err)

				return
			}

			deleted := result.Actions[0]

			recorded := performAction(t, Request{Command: View, ID: strconv.FormatInt(original.ID, 10)})
			if failOriginalTransfer {
				if len(result.Actions) != 2 || result.Actions[1].ID != original.ID || deleted.Status != store.Completed || recorded.Status != store.Failed || recorded.InProgress || recorded.Failure == "" {
					t.Errorf("✗ restore failure did not retain both outcomes: %+v; %+v", result, recorded)
				}

				assertAbsent(t, source)
				assertFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")
			} else {
				if len(result.Actions) != 1 || deleted.ID == original.ID || deleted.Status != store.Failed || deleted.Failure == "" || !reflect.DeepEqual(recorded, original) {
					t.Errorf("✗ occupying transfer failure changed the original: %+v; %+v", result, recorded)
				}

				assertFile(t, source, "occupying contents")
			}

			assertFile(t, destination, "original contents")

			if !t.Failed() {
				t.Log("✓ restore case preserves its required outcome")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ failed transfers preserve both items and the corresponding action outcomes")
	}
}

// TestFailedTransferRecording verifies invariant #6: Failed execution.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A destination occupied after validation prevents a move. Its failed outcome
// is recorded with the failure text and resolution time, preserving both files.
func TestFailedTransferRecording(t *testing.T) {
	home := configuredHome(t, 30)
	source, destination := filepath.Join(home, "source"), filepath.Join(home, "destination")
	writeFile(t, source, "source contents")

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("💣 configuration: %v", err)
	}

	action, _, err := prepareAction(&Request{Command: Move, Source: source, Destination: destination, Reason: "relocate"}, home, settings)
	if err != nil {
		t.Fatalf("💣 prepare move: %v", err)
	}

	history, err := store.Open(t.Context(), filepath.Join(home, config.Directory, config.Database))
	if err != nil {
		t.Fatalf("💣 open history: %v", err)
	}
	defer history.Close()

	writeFile(t, destination, "destination contents")

	if _, err = runRecordedAction(t.Context(), home, settings, history, &action); err == nil {
		t.Error("✗ occupied destination accepted")
	}

	recorded, err := history.Get(t.Context(), action.ID)
	if err != nil || recorded.Status != store.Failed || recorded.InProgress || recorded.Failure == "" || recorded.ResolvedAt.IsZero() {
		t.Errorf("✗ failed outcome missing: %+v, %v", recorded, err)
	}

	assertFile(t, source, "source contents")
	assertFile(t, destination, "destination contents")

	if !t.Failed() {
		t.Log("✓ failed execution records its outcome and preserves files")
	}
}

// TestRestoreRefusals verifies invariant #7: Restore refusal.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Resolved records, missing trash configuration, invalid trash identities,
// destinations containing the active database, and rejected pending-delete
// insertion all refuse before changing the original record or either item.
func TestRestoreRefusals(t *testing.T) {
	for _, refusal := range []string{"resolved", "trash unset", "trash identity", "database", "database directory", "review insertion"} {
		t.Run(refusal, func(t *testing.T) {
			home := configuredHome(t, 30)
			source := filepath.Join(home, "source")
			writeFile(t, source, "original contents")
			original := performAction(t, Request{Command: Delete, Source: source, Reason: "retain original"})
			retainedItem := filepath.Join(home, "trash", original.TrashID)

			writeFile(t, source, "occupying contents")

			var refusalError error

			switch refusal {
			case "resolved":
				executeSQL(t, home, `UPDATE actions SET status=? WHERE action_id=?`, store.Denied, original.ID)

				refusalError = errs.ErrActionResolved
			case "trash unset":
				writeConfig(t, home, "retain-period = 30\n")

				refusalError = errs.ErrConfigMissing
			case "trash identity":
				executeSQL(t, home, `UPDATE deleted SET trash_id=? WHERE action_id=?`, "../outside", original.ID)

				refusalError = errs.ErrActionAmbiguous
			case "database", "database directory":
				databaseSource := filepath.Join(home, config.Directory)
				if refusal == "database" {
					databaseSource = filepath.Join(databaseSource, config.Database)
				}

				executeSQL(t, home, `UPDATE deleted SET source_path=? WHERE action_id=?`, databaseSource, original.ID)

				refusalError = errs.ErrFileOverlap
			case "review insertion":
				writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = [%q]\n", filepath.Join(home, "trash"), source))
				executeSQL(t, home, `CREATE TRIGGER reject_review BEFORE INSERT ON actions BEGIN SELECT RAISE(FAIL,'review recording rejected'); END`)

				refusalError = errs.ErrDatabaseWrite
			}

			beforeRestore, err := Run(t.Context(), &Request{Command: List})
			if err != nil {
				t.Fatalf("💣 inspect initial history: %v", err)
			}

			result, err := Run(t.Context(), &Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true, Force: true})
			if !errors.Is(err, refusalError) || len(result.Actions) != 0 {
				t.Errorf("✗ %s refusal: %+v, %v", refusal, result, err)
			}

			afterRestore, err := Run(t.Context(), &Request{Command: List})
			if err != nil || !reflect.DeepEqual(beforeRestore.Actions, afterRestore.Actions) {
				t.Errorf("✗ %s refusal changed history: %+v, %v", refusal, afterRestore, err)
			}

			assertFile(t, retainedItem, "original contents")
			assertFile(t, source, "occupying contents")

			if !t.Failed() {
				t.Log("✓ restore case preserves its required outcome")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ restore refuses invalid records and paths before changing files or actions")
	}
}

// TestRestoreClaimConflict verifies invariant #8: Competing restore claims.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// An original record changed while the occupying delete is saved rejects the
// stale claim. Its item stays retained and the completed occupying delete is
// returned with the busy error and remains available for restoration.
func TestRestoreClaimConflict(t *testing.T) {
	home := configuredHome(t, 30)
	source := filepath.Join(home, "source")
	writeFile(t, source, "original contents")
	original := performAction(t, Request{Command: Delete, Source: source, Reason: "retain original"})
	writeFile(t, source, "occupying contents")
	executeSQL(t, home, fmt.Sprintf(`CREATE TRIGGER competing_claim AFTER INSERT ON actions BEGIN UPDATE actions SET in_progress=1 WHERE action_id=%d; END`, original.ID))

	result, err := Run(t.Context(), &Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true, Force: true})
	if !errors.Is(err, errs.ErrActionBusy) || len(result.Actions) != 1 || result.Actions[0].ID == original.ID || result.Actions[0].Status != store.Completed {
		t.Errorf("✗ competing claim lost the completed delete: %+v, %v", result, err)

		return
	}

	deleted := result.Actions[0]

	assertFile(t, filepath.Join(home, "trash", original.TrashID), "original contents")
	assertFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")
	assertAbsent(t, source)

	recovered := performAction(t, Request{Command: Restore, ID: strconv.FormatInt(deleted.ID, 10)})
	if recovered.Status != store.Restored {
		t.Errorf("✗ preserved occupant could not be restored: %+v", recovered)
	}

	assertFile(t, source, "occupying contents")

	if !t.Failed() {
		t.Log("✓ a competing claim preserves both recovery items and returns the occupying delete")
	}
}

// TestLinkedRefusal verifies invariant #9: Linked refusal.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Missing occupants prevent combined approval without claiming either action.
// A missing moving source after delete approval remains pending and deniable.
func TestLinkedRefusal(t *testing.T) {
	for _, missingOccupant := range []bool{false, true} {
		t.Run("missing occupant="+strconv.FormatBool(missingOccupant), func(t *testing.T) {
			_, moved, deleted := pendingPair(t, true)

			missingSource := moved.Source
			if missingOccupant {
				missingSource = deleted.Source
			}

			if err := os.Rename(missingSource, missingSource+"-retained"); err != nil {
				t.Fatalf("💣 retain missing item: %v", err)
			}

			if !missingOccupant {
				performAction(t, Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: store.ApproveDecision})
			}

			_, err := Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(moved.ID, 10), Decision: store.ApproveDecision, Confirmed: true})
			if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("✗ missing source approval not refused: %v", err)
			}

			assertRecorded(t, moved)
			denyID := moved.ID

			if missingOccupant {
				assertRecorded(t, deleted)
				denyID = deleted.ID
			}

			_, err = Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(denyID, 10), Decision: store.DenyDecision})
			if err != nil {
				t.Errorf("✗ refused approval prevented denial: %v", err)
			}

			storedMove := performAction(t, Request{Command: View, ID: strconv.FormatInt(moved.ID, 10)})
			if storedMove.Status != store.Denied || storedMove.InProgress {
				t.Errorf("✗ move left unresolved after denial: %+v", storedMove)
			}

			if !t.Failed() {
				t.Log("✓ failed preliminary approval remains deniable")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ missing linked items do not strand pending requests")
	}
}

// TestLinkedTransferFailure verifies invariant #10: Linked failures.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A filesystem restriction transferring an approved occupant records its failure
// and denies its move, retaining both source files and earlier move diagnostics.
func TestLinkedTransferFailure(t *testing.T) {
	_, moved, deleted := pendingPair(t, true)
	preventFileTransfer(t, deleted.Source)

	result, err := Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(moved.ID, 10), Decision: store.ApproveDecision, Confirmed: true})
	if !errors.Is(err, errs.ErrFileTransfer) || len(result.Actions) != 2 {
		t.Errorf("✗ failed blocker did not return both outcomes: %+v, %v", result, err)
	}

	assertUniqueResults(t, result)
	storedDelete := performAction(t, Request{Command: View, ID: strconv.FormatInt(deleted.ID, 10)})

	storedMove := performAction(t, Request{Command: View, ID: strconv.FormatInt(moved.ID, 10)})
	if storedDelete.Status != store.Failed || storedDelete.ResolvedAt.IsZero() || storedDelete.InProgress || storedDelete.Failure == "" {
		t.Errorf("✗ delete failure was not resolved: %+v", storedDelete)
	}

	if storedMove.Status != store.Denied || storedMove.Decision != store.DenyDecision || storedMove.ResolvedAt.IsZero() || storedMove.InProgress || !strings.HasPrefix(storedMove.Failure, moved.Failure+"\n") || !strings.Contains(storedMove.Failure, fmt.Sprintf(strings.ReplaceAll(appstrings.Text("diagnostic.blockerFailed"), "%w", appstrings.Text("error.blockerFailed")), deleted.ID)) {
		t.Errorf("✗ failed blocker did not deny move with retained diagnostics: %+v", storedMove)
	}

	assertFile(t, moved.Source, "moving contents")
	assertFile(t, deleted.Source, "occupying contents")

	if !t.Failed() {
		t.Log("✓ actual blocker transfer failure denies its move and retains both files")
	}
}

// TestDependentErrors verifies invariant #10: Linked failures.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Busy and missing-source dependents report both errors while a later eligible
// dependent still completes. A protected move stays pending, earlier claims stay
// untouched, and each returned record occurs once with its persisted state.
func TestDependentErrors(t *testing.T) {
	home, moved, deleted := pendingPair(t, true)

	history, err := store.Open(t.Context(), filepath.Join(home, config.Directory, config.Database))
	if err != nil {
		t.Fatalf("💣 dependent history: %v", err)
	}
	defer history.Close()

	dependents := make([]store.Action, 0, 3)

	for _, condition := range []string{"busy", "missing", "eligible"} {
		source := filepath.Join(home, condition)
		if condition != "missing" {
			writeFile(t, source, condition)
		}

		action := store.Action{Type: Move, Status: store.Pending, Source: source, Destination: source + "-destination", Reason: "relocate", BlockerID: deleted.ID, Operation: Move}
		if err = history.Create(t.Context(), &action); err != nil {
			t.Fatalf("💣 save dependent: %v", err)
		}

		if condition == "busy" {
			if err = history.Claim(t.Context(), &action, Move, store.ApproveDecision); err != nil {
				t.Fatalf("💣 competing claim: %v", err)
			}
		}

		dependents = append(dependents, action)
	}

	result, err := Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: store.ApproveDecision})
	if !errors.Is(err, errs.ErrActionBusy) || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("✗ dependent errors not retained: %+v, %v", result, err)
	}

	assertUniqueResults(t, result)
	assertRecorded(t, moved, dependents[0], dependents[1])

	completed := performAction(t, Request{Command: View, ID: strconv.FormatInt(dependents[2].ID, 10)})
	if completed.Status != store.Completed || completed.Decision != store.ApproveDecision {
		t.Errorf("✗ other dependent was not processed: %+v", completed)
	}

	assertFile(t, completed.Destination, "eligible")
	assertAbsent(t, completed.Source)

	if !t.Failed() {
		t.Log("✓ partial errors do not prevent other eligible dependents")
	}
}

// TestLinkedClaimConflict verifies invariant #10: Linked failures.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A competing claim during blocker denial remains untouched and reports busy,
// while another dependent is denied with the blocker diagnostic. A stale move
// decision cannot overwrite a newer decision and returns its current state.
func TestLinkedClaimConflict(t *testing.T) {
	home, moved, deleted := pendingPair(t, true)

	history, err := store.Open(t.Context(), filepath.Join(home, config.Directory, config.Database))
	if err != nil {
		t.Fatalf("💣 claim fixture history: %v", err)
	}
	defer history.Close()

	staleMove := moved
	if err = history.Claim(t.Context(), &moved, Move, store.ApproveDecision); err != nil {
		t.Fatalf("💣 competing move claim: %v", err)
	}

	otherMove := store.Action{Type: Move, Status: store.Pending, Source: filepath.Join(home, "other"), Destination: filepath.Join(home, "other-destination"), Reason: "relocate", BlockerID: deleted.ID, Operation: Move}
	if err = history.Create(t.Context(), &otherMove); err != nil {
		t.Fatalf("💣 second dependent: %v", err)
	}

	result, err := Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: store.DenyDecision})
	if !errors.Is(err, errs.ErrActionBusy) {
		t.Errorf("✗ competing claim was not reported: %+v, %v", result, err)
	}

	assertUniqueResults(t, result)
	assertRecorded(t, moved)

	denied := performAction(t, Request{Command: View, ID: strconv.FormatInt(otherMove.ID, 10)})
	if denied.Status != store.Denied || denied.Decision != store.DenyDecision || denied.ResolvedAt.IsZero() {
		t.Errorf("✗ competing claim blocked the other denial: %+v", denied)
	}

	if err = history.Finish(t.Context(), &moved, store.Denied, nil); err != nil {
		t.Fatalf("💣 finish competing decision: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("💣 fixture configuration: %v", err)
	}

	result, err = decide(t.Context(), &Request{Decision: store.DenyDecision}, home, settings, history, &staleMove)
	if !errors.Is(err, errs.ErrActionBusy) || len(result.Actions) != 1 || !reflect.DeepEqual(result.Actions[0], moved) {
		t.Errorf("✗ stale decision changed the latest action: %+v, %v", result, err)
	}

	assertRecorded(t, moved)
	assertFile(t, moved.Source, "moving contents")
	assertFile(t, deleted.Source, "occupying contents")

	if !t.Failed() {
		t.Log("✓ competing claims preserve current decisions without blocking other denials")
	}
}

// TestFreshLinkedConsent verifies invariant #11: Fresh consent.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A completed or removed blocker permits direct approval of its still-pending
// move after an earlier question. A move resolved during the question is refused
// as resolved without changing either record again.
func TestFreshLinkedConsent(t *testing.T) {
	for _, change := range []string{"completed", "removed", "resolved"} {
		t.Run(change, func(t *testing.T) {
			home, moved, deleted := pendingPair(t, true)
			request := Request{Command: Dispose, ID: strconv.FormatInt(moved.ID, 10), Decision: store.ApproveDecision}

			question, err := Run(t.Context(), &request)
			if !errors.Is(err, errs.ErrConfirmation) || question.Confirmation == nil {
				t.Errorf("✗ initial question missing: %+v, %v", question, err)
			}

			storedDelete := performAction(t, Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: store.ApproveDecision})
			if change == "removed" {
				history, openErr := store.Open(t.Context(), filepath.Join(home, config.Directory, config.Database))
				if openErr != nil {
					t.Fatalf("💣 blocker history: %v", openErr)
				}
				defer history.Close()

				if err = history.Delete(t.Context(), &storedDelete); err != nil {
					t.Fatalf("💣 remove resolved blocker: %v", err)
				}
			}

			if change == "resolved" {
				moved = performAction(t, Request{Command: Dispose, ID: request.ID, Decision: store.DenyDecision})
			}

			request.Confirmed = true

			result, err := Run(t.Context(), &request)
			if change == "resolved" {
				if !errors.Is(err, errs.ErrActionResolved) {
					t.Errorf("✗ stale consent changed resolved move: %+v, %v", result, err)
				}

				assertRecorded(t, moved, storedDelete)
			} else {
				if err != nil || len(result.Actions) != 1 || result.Actions[0].Status != store.Completed || result.Actions[0].BlockerID != deleted.ID {
					t.Errorf("✗ refreshed move cannot proceed: %+v, %v", result, err)
				}

				assertFile(t, moved.Destination, "moving contents")
				assertAbsent(t, moved.Source)
			}

			if !t.Failed() {
				t.Log("✓ consent uses current records")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ completed, removed and resolved records are reread after questions")
	}
}

// TestMovePreparationRefusals verifies invariant #12: Move preparation.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Replacement without overwrite, without trash, across overlapping paths, into
// the active database directory, or into its own trash is refused before consent
// or any mutation. Force alone never authorizes replacing the destination.
func TestMovePreparationRefusals(t *testing.T) {
	for _, condition := range []string{"no overwrite", "force alone", "missing trash", "overlap", "database", "trash overlap"} {
		t.Run(condition, func(t *testing.T) {
			homeDirectory := configuredHome(t, 30)

			home, err := filepath.EvalSymlinks(homeDirectory)
			if err != nil {
				t.Fatalf("💣 resolve fixture home: %v", err)
			}

			source, destination := filepath.Join(home, "source"), filepath.Join(home, "destination")

			occupant := destination

			switch condition {
			case "overlap":
				source = filepath.Join(destination, "source")
				occupant = filepath.Join(destination, "occupant")
			case "database":
				destination = filepath.Join(home, config.Directory)
				occupant = filepath.Join(destination, "occupant")
			case "trash overlap":
				occupant = filepath.Join(destination, "occupant")
				writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = []\n", filepath.Join(destination, "trash")))
			case "missing trash":
				writeConfig(t, home, "require-review = []\n")
			}

			writeFile(t, source, "moving contents")
			writeFile(t, occupant, "occupying contents")

			request := Request{Command: Move, Source: source, Destination: destination, Reason: "replace", Overwrite: condition != "no overwrite" && condition != "force alone", Force: condition == "force alone"}

			result, err := Run(t.Context(), &request)
			if err == nil || errors.Is(err, errs.ErrConfirmation) || result.Confirmation != nil || len(result.Actions) != 0 {
				t.Errorf("✗ unsafe move was not refused before consent: %+v, %v", result, err)
			}

			history, err := Run(t.Context(), &Request{Command: List})
			if err != nil || len(history.Actions) != 0 {
				t.Errorf("✗ refused move changed history: %+v, %v", history, err)
			}

			assertFile(t, source, "moving contents")
			assertFile(t, occupant, "occupying contents")

			if !t.Failed() {
				t.Log("✓ invalid replacement leaves both items and history unchanged")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ move preparation rejects unsafe replacement before effects")
	}
}

// TestMoveTransferFailures verifies invariant #13: Move failure results.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Filesystem restrictions in the occupying delete deny its move. A later move
// transfer failure preserves the completed occupying delete. Both cases retain
// both items, final diagnostics and resolution times, and send no notification.
func TestMoveTransferFailures(t *testing.T) {
	for _, failedOperation := range []string{Delete, Move} {
		t.Run(failedOperation, func(t *testing.T) {
			homeDirectory := configuredHome(t, 30)

			home, err := filepath.EvalSymlinks(homeDirectory)
			if err != nil {
				t.Fatalf("💣 resolve fixture home: %v", err)
			}

			source, destination := filepath.Join(home, "moving", "source"), filepath.Join(home, "occupying", "destination")
			writeFile(t, source, "moving contents")
			writeFile(t, destination, "occupying contents")
			listener := notificationReceiver(t, home)

			restricted := destination
			if failedOperation == Move {
				restricted = source
			}

			preventFileTransfer(t, restricted)

			result, err := Run(t.Context(), &Request{Command: Move, Source: source, Destination: destination, Reason: "replace", Overwrite: true, Force: true})
			if !errors.Is(err, errs.ErrFileTransfer) || len(result.Actions) != 2 {
				t.Errorf("✗ failed replacement omitted its outcomes: %+v, %v", result, err)

				return
			}

			moved, deleted := result.Actions[0], result.Actions[1]
			assertUniqueResults(t, result)

			if moved.ID <= 0 || moved.ID >= deleted.ID || moved.BlockerID != deleted.ID || moved.ResolvedAt.IsZero() || deleted.ResolvedAt.IsZero() || moved.InProgress || deleted.InProgress {
				t.Errorf("✗ failed replacement left incomplete records: %+v", result)
			}

			if failedOperation == Delete {
				if moved.Status != store.Denied || moved.Decision != store.DenyDecision || moved.Failure == "" || deleted.Status != store.Failed || deleted.Failure == "" {
					t.Errorf("✗ failed delete did not deny move: %+v", result)
				}

				assertFile(t, destination, "occupying contents")
			} else {
				if moved.Status != store.Failed || moved.Failure == "" || deleted.Status != store.Completed {
					t.Errorf("✗ failed move lost its completed delete: %+v", result)
				}

				assertAbsent(t, destination)
				assertFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")
			}

			assertFile(t, source, "moving contents")
			assertActionNotifications(t, listener, nil)

			if !t.Failed() {
				t.Log("✓ failed transfer preserves both items and final partial results")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ delete and move transfer failures retain both items")
	}
}

// TestMovePairRecordingFailure verifies invariant #13: Move failure results.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A rejected pair saves neither action and exposes no assigned ID. The moving
// source remains untouched. An already-completed delete returns its recovery
// paths and trash ID with ErrDatabaseResult; a reviewed occupant remains in place.
// Neither failed save sends a pending notification.
func TestMovePairRecordingFailure(t *testing.T) {
	for _, occupantReview := range []bool{false, true} {
		t.Run("occupant review="+strconv.FormatBool(occupantReview), func(t *testing.T) {
			homeDirectory := configuredHome(t, 30)

			home, err := filepath.EvalSymlinks(homeDirectory)
			if err != nil {
				t.Fatalf("💣 resolve fixture home: %v", err)
			}

			source, destination := filepath.Join(home, "source"), filepath.Join(home, "destination")
			writeFile(t, source, "moving contents")
			writeFile(t, destination, "occupying contents")

			if occupantReview {
				writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = [%q]\n", filepath.Join(home, "trash"), destination))
			}

			listener := notificationReceiver(t, home)
			if _, err := Run(t.Context(), &Request{Command: List}); err != nil {
				t.Fatalf("💣 initialize recording fixture: %v", err)
			}

			executeSQL(t, home, `CREATE TRIGGER reject_pair BEFORE INSERT ON actions BEGIN SELECT RAISE(FAIL,'pair rejected'); END`)

			result, err := Run(t.Context(), &Request{Command: Move, Source: source, Destination: destination, Reason: "replace", Overwrite: true, Force: true})
			if !errors.Is(err, errs.ErrDatabaseWrite) {
				t.Errorf("✗ pair recording error lost: %+v, %v", result, err)
			}

			if occupantReview {
				if len(result.Actions) != 0 || errors.Is(err, errs.ErrDatabaseResult) {
					t.Errorf("✗ unsaved pending requests reported as actions: %+v, %v", result, err)
				}

				assertFile(t, destination, "occupying contents")
			} else {
				if len(result.Actions) != 1 {
					t.Errorf("✗ completed delete must be the only unsaved result: %+v", result)

					return
				}

				deleted := result.Actions[0]
				if deleted.ID != 0 || deleted.BlockerID != 0 || deleted.Type != Delete || deleted.Source != destination || deleted.TrashID == "" || deleted.TrashPath == "" {
					t.Errorf("✗ unsaved delete lost recovery identity or exposed IDs: %+v", result)
				}

				if deleted.Status != store.Completed || !errors.Is(err, errs.ErrDatabaseResult) {
					t.Errorf("✗ completed delete not reported as an unsaved result: %+v, %v", result, err)
				}

				assertAbsent(t, destination)
				assertFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")
			}

			assertFile(t, source, "moving contents")

			history, err := Run(t.Context(), &Request{Command: List})
			if err != nil || len(history.Actions) != 0 {
				t.Errorf("✗ rejected pair partially persisted: %+v, %v", history, err)
			}

			assertActionNotifications(t, listener, nil)

			if !t.Failed() {
				t.Log("✓ failed recording preserves the moving source and exposes retained occupant data")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ pair-save failure preserves recoverability without partial history or notifications")
	}
}

// TestCanceledReplacement verifies invariant #13: Move failure results.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Cancellation after move preparation prevents the occupying transfer, records
// its failure, and still closes the linked move as denied. Both items stay in
// place and no pending notification is sent for the denied move.
func TestCanceledReplacement(t *testing.T) {
	home := configuredHome(t, 30)
	source, destination := filepath.Join(home, "source"), filepath.Join(home, "destination")
	writeFile(t, source, "moving contents")
	writeFile(t, destination, "occupying contents")
	listener := notificationReceiver(t, home)

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("💣 cancellation configuration: %v", err)
	}

	history, err := store.Open(t.Context(), filepath.Join(home, config.Directory, config.Database))
	if err != nil {
		t.Fatalf("💣 cancellation history: %v", err)
	}
	defer history.Close()

	request := Request{Command: Move, Source: source, Destination: destination, Reason: "replace", Overwrite: true, Force: true}

	moved, _, err := prepareAction(&request, home, settings)
	if err != nil {
		t.Fatalf("💣 cancellation preparation: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	result, err := replaceMove(ctx, &request, home, settings, history, &moved)
	if !errors.Is(err, context.Canceled) || len(result.Actions) != 2 {
		t.Errorf("✗ canceled replacement omitted its outcomes: %+v, %v", result, err)
	} else if result.Actions[0].Status != store.Denied || result.Actions[0].Decision != store.DenyDecision || result.Actions[0].ResolvedAt.IsZero() || result.Actions[1].Status != store.Failed {
		t.Errorf("✗ canceled delete left its move pending: %+v", result)
	}

	assertUniqueResults(t, result)
	assertFile(t, source, "moving contents")
	assertFile(t, destination, "occupying contents")
	assertActionNotifications(t, listener, nil)

	if !t.Failed() {
		t.Log("✓ cancellation records failure and closes the blocked move")
	}
}

// TestReplacementTrashLocation verifies invariant #14: Retained trash location.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Move and restore replacement refuse before changing files or history when
// configured trash is inside the directory being transferred. This holds for
// existing and absent trash directories, with and without source/occupant review.
// Refusal emits no pending notification and leaves the original restore usable.
func TestReplacementTrashLocation(t *testing.T) {
	for _, operation := range []string{Move, Restore} {
		for _, trashExists := range []bool{false, true} {
			for _, needsReview := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/trash=%t/review=%t", operation, trashExists, needsReview), func(t *testing.T) {
					home := configuredHome(t, 30)
					movingDirectory := filepath.Join(home, "source")
					occupantPath := filepath.Join(home, "destination")
					writeFile(t, filepath.Join(movingDirectory, "document"), "moving contents")
					request := Request{Command: Move, Source: movingDirectory, Destination: occupantPath, Reason: "replace", Overwrite: true, Force: true}

					if operation == Restore {
						original := performAction(t, Request{Command: Move, Source: movingDirectory, Destination: occupantPath, Reason: "relocate"})
						movingDirectory, occupantPath = occupantPath, movingDirectory
						request = Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true, Force: true}
					}

					writeFile(t, occupantPath, "occupying contents")

					trashDirectory := filepath.Join(movingDirectory, "trash")
					if trashExists {
						if err := os.Mkdir(trashDirectory, 0o700); err != nil {
							t.Fatalf("💣 create trash fixture: %v", err)
						}
					}

					reviewRules := "[]"
					if needsReview {
						reviewRules = fmt.Sprintf("[%q, %q]", movingDirectory, occupantPath)
					}

					writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = %s\n", trashDirectory, reviewRules))
					listener := notificationReceiver(t, home)

					previousHistory, err := Run(t.Context(), &Request{Command: List})
					if err != nil {
						t.Fatalf("💣 read original history: %v", err)
					}

					result, err := Run(t.Context(), &request)
					if !errors.Is(err, errs.ErrFileOverlap) || len(result.Actions) != 0 || result.Confirmation != nil {
						t.Errorf("✗ replacement accepted a trash location it would relocate: %+v, %v", result, err)
					}

					currentHistory, err := Run(t.Context(), &Request{Command: List})
					if err != nil || !reflect.DeepEqual(currentHistory.Actions, previousHistory.Actions) {
						t.Errorf("✗ refused replacement changed history: %+v, %v", currentHistory, err)
					}

					assertFile(t, filepath.Join(movingDirectory, "document"), "moving contents")
					assertFile(t, occupantPath, "occupying contents")

					if trashExists {
						trashItems, readErr := os.ReadDir(trashDirectory)
						if readErr != nil || len(trashItems) != 0 {
							t.Errorf("✗ refused replacement changed trash: %v, %v", trashItems, readErr)
						}
					} else {
						assertAbsent(t, trashDirectory)
					}

					assertActionNotifications(t, listener, nil)

					if !t.Failed() {
						t.Log("✓ replacement refuses to relocate its retained trash")
					}
				})
			}
		}
	}

	if !t.Failed() {
		t.Log("✓ both replacements preserve files and history for unsafe trash locations")
	}
}

// TestReplacementTrashAliases verifies invariant #14: Retained trash location.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Replacement refuses when moving the directory would relocate either the
// physical trash or any symbolic link used to address it, including intermediate
// links in the root or its parent. Both move and restore preserve history,
// documents, the link and the still-empty trash directory.
func TestReplacementTrashAliases(t *testing.T) {
	for _, operation := range []string{Move, Restore} {
		for _, aliasLocation := range []string{"inside source", "outside source", "chain through source", "chain parent through source"} {
			t.Run(operation+"/"+aliasLocation, func(t *testing.T) {
				home := configuredHome(t, 30)
				movingDirectory := filepath.Join(home, "source")
				occupantPath := filepath.Join(home, "destination")
				writeFile(t, filepath.Join(movingDirectory, "document"), "moving contents")

				request := Request{Command: Move, Source: movingDirectory, Destination: occupantPath, Reason: "replace", Overwrite: true, Force: true}
				if operation == Restore {
					original := performAction(t, Request{Command: Move, Source: movingDirectory, Destination: occupantPath, Reason: "relocate"})
					movingDirectory, occupantPath = occupantPath, movingDirectory
					request = Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true, Force: true}
				}

				writeFile(t, occupantPath, "occupying contents")

				aliasPath, physicalTrash := filepath.Join(movingDirectory, "alias"), filepath.Join(home, "trash")
				if aliasLocation == "outside source" {
					aliasPath, physicalTrash = filepath.Join(home, "alias"), filepath.Join(movingDirectory, "trash")
				}

				if err := os.Mkdir(physicalTrash, 0o700); err != nil {
					t.Fatalf("💣 create physical trash: %v", err)
				}

				if err := os.Symlink(physicalTrash, aliasPath); err != nil {
					t.Fatalf("💣 create trash alias: %v", err)
				}

				trashDirectory := aliasPath
				if aliasLocation == "inside source" {
					trashDirectory = filepath.Join(aliasPath, "nested")
				}

				if strings.HasPrefix(aliasLocation, "chain") {
					trashDirectory = filepath.Join(home, "external-alias")

					relativeTarget, err := filepath.Rel(home, aliasPath)
					if err != nil {
						t.Fatalf("💣 relative alias target: %v", err)
					}

					if err := os.Symlink(relativeTarget, trashDirectory); err != nil {
						t.Fatalf("💣 create intermediate trash alias: %v", err)
					}

					if aliasLocation == "chain parent through source" {
						trashDirectory = filepath.Join(trashDirectory, "nested")
					}
				}

				writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = []\n", trashDirectory))

				previousHistory, err := Run(t.Context(), &Request{Command: List})
				if err != nil {
					t.Fatalf("💣 read original history: %v", err)
				}

				result, err := Run(t.Context(), &request)
				if !errors.Is(err, errs.ErrFileOverlap) || len(result.Actions) != 0 {
					t.Errorf("✗ replacement would relocate its trash or its alias: %+v, %v", result, err)
				}

				currentHistory, err := Run(t.Context(), &Request{Command: List})
				if err != nil || !reflect.DeepEqual(currentHistory.Actions, previousHistory.Actions) {
					t.Errorf("✗ refused replacement changed history: %+v, %v", currentHistory, err)
				}

				assertFile(t, filepath.Join(movingDirectory, "document"), "moving contents")
				assertFile(t, occupantPath, "occupying contents")

				linkTarget, err := os.Readlink(aliasPath)
				if err != nil || linkTarget != physicalTrash {
					t.Errorf("✗ trash alias changed: %q, %v", linkTarget, err)
				}

				trashItems, err := os.ReadDir(physicalTrash)
				if err != nil || len(trashItems) != 0 {
					t.Errorf("✗ refused replacement changed trash: %v, %v", trashItems, err)
				}

				if !t.Failed() {
					t.Log("✓ replacement preserves the trash and every alias used to reach it")
				}
			})
		}
	}

	if !t.Failed() {
		t.Log("✓ replacement refuses to relocate physical or configured trash paths")
	}
}

// TestReplacementTrashIndependentAliases verifies invariant #14: Retained trash location.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A chain of trash links wholly outside the transferred directory is permitted,
// including a missing final directory. Validation preserves the source and links.
func TestReplacementTrashIndependentAliases(t *testing.T) {
	for _, nestedDirectory := range []bool{false, true} {
		t.Run(strconv.FormatBool(nestedDirectory), func(t *testing.T) {
			home := t.TempDir()
			sourcePath := filepath.Join(home, "source")
			writeFile(t, filepath.Join(sourcePath, "document"), "moving contents")

			physicalTrash := filepath.Join(home, "trash")
			if err := os.Mkdir(physicalTrash, 0o700); err != nil {
				t.Fatalf("💣 create physical trash: %v", err)
			}

			aliasPath := filepath.Join(home, "alias")
			if err := os.Symlink(physicalTrash, aliasPath); err != nil {
				t.Fatalf("💣 create trash alias: %v", err)
			}

			trashPath := filepath.Join(home, "configured-alias")
			if err := os.Symlink(aliasPath, trashPath); err != nil {
				t.Fatalf("💣 create configured alias: %v", err)
			}

			if nestedDirectory {
				trashPath = filepath.Join(trashPath, "nested")
			}

			if err := checkReplacementTrash(sourcePath, home, config.Settings{TrashPath: &trashPath}); err != nil {
				t.Errorf("✗ independent trash was refused: %v", err)
			}

			assertFile(t, filepath.Join(sourcePath, "document"), "moving contents")

			linkTarget, err := os.Readlink(aliasPath)
			if err != nil || linkTarget != physicalTrash {
				t.Errorf("✗ independent trash alias changed: %q, %v", linkTarget, err)
			}

			if !t.Failed() {
				t.Log("✓ independent trash links remain usable")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ independent trash chains permit replacement")
	}
}

// TestReplacementTrashErrors verifies invariant #14: Retained trash location.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Missing trash configuration and circular links in either the trash directory
// or its parent return their identifying error without changing the source.
func TestReplacementTrashErrors(t *testing.T) {
	for _, condition := range []string{"missing setting", "circular parent", "circular root"} {
		t.Run(condition, func(t *testing.T) {
			home := t.TempDir()
			sourcePath := filepath.Join(home, "source")
			writeFile(t, filepath.Join(sourcePath, "document"), "moving contents")

			settings := config.Settings{}
			expectedError := errs.ErrConfigMissing

			if condition != "missing setting" {
				aliasPath := filepath.Join(home, "alias")
				if err := os.Symlink(aliasPath, aliasPath); err != nil {
					t.Fatalf("💣 circular trash alias: %v", err)
				}

				trashPath := aliasPath
				if condition == "circular parent" {
					trashPath = filepath.Join(aliasPath, "trash")
				}

				settings.TrashPath = &trashPath
				expectedError = errs.ErrPathResolve
			}

			if err := checkReplacementTrash(sourcePath, home, settings); !errors.Is(err, expectedError) {
				t.Errorf("✗ invalid trash location error: %v", err)
			}

			assertFile(t, filepath.Join(sourcePath, "document"), "moving contents")

			if !t.Failed() {
				t.Log("✓ unusable trash paths fail without changing source contents")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ trash validation identifies configuration and resolution errors")
	}
}

func preventFileTransfer(t *testing.T, source string) {
	t.Helper()

	parent := filepath.Dir(source)
	if runtime.GOOS == "windows" {
		// os.Open permits shared reads and writes on Windows, but not renaming
		// or deletion until its handle closes. Metadata inspection still works.
		sourceFile, err := os.Open(source)
		if err != nil {
			t.Fatalf("💣 open fixture source: %v", err)
		}

		t.Cleanup(func() {
			if err := sourceFile.Close(); err != nil {
				t.Errorf("✗ close fixture source: %v", err)
			}
		})
	} else {
		metadata, err := os.Stat(parent)
		if err != nil {
			t.Fatalf("💣 directory permissions: %v", err)
		}

		if err := os.Chmod(parent, 0o500); err != nil { //nolint:gosec // G302: the private directory needs read and traversal, but must deny writes.
			t.Fatalf("💣 restrict directory: %v", err)
		}

		t.Cleanup(func() {
			if err := os.Chmod(parent, metadata.Mode().Perm()); err != nil {
				t.Errorf("✗ restore directory permissions: %v", err)
			}
		})
	}

	if err := os.Rename(source, source+"-transfer-probe"); err == nil {
		t.Fatal("💣 fixture did not prevent transfer")
	}
}
