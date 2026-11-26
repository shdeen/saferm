package saferm

// Invariants tested:
//  1. Invalid requests: Missing and unusable inputs fail before recording an action.
//  2. Failed recording: A failed result write reports completed filesystem work separately.
//  3. Interrupted operations: Busy records cannot execute again or be discarded by cleanup.
//  4. Changed pending paths: Approval rechecks mutable source and destination conditions.
//  5. Maintenance retention: Pending and recoverable actions survive age and orphan cleanup.
//  6. Missing purge items: A batch reports missing items while completing eligible removals.
//  7. Unusable trash: A configured trash directory that can't be used gives way to the embedded one without changing what blocks it.
//  8. Cancellation: Canceled requests do not mutate files or create history.
//  9. Denial recording: A failed denial write must not claim that a filesystem operation succeeded.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/store"
)

// TestInvalidRequests verifies invariant #1: Invalid requests.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Missing paths, reasons, IDs, bad filters and decisions, conflicting config
// options, and extra arguments return identifiable input errors without a database.
func TestInvalidRequests(t *testing.T) {
	home := configuredHome(t, 30)

	if _, err := Run(t.Context(), nil); !errors.Is(err, errs.ErrInputMissing) {
		t.Errorf("✗ missing request: %v", err)
	}

	requests := []Request{
		{Command: Delete},
		{Command: Delete, Source: "source"},
		{Command: Move, Source: "source", Reason: "reason"},
		{Command: View},
		{Command: View, ID: "0"},
		{Command: View, ID: "-1"},
		{Command: View, ID: "abc"},
		{Command: Dispose, ID: "1", Decision: "maybe"},
		{Command: List, Type: "unknown"},
		{Command: List, Status: "unknown"},
		{Command: Purge},
		{Command: Purge, IDs: []string{"1", "bad"}},
		{Command: Config, Reset: true},
		{Command: Config, Default: true, Setup: true},
		{Command: "unknown"},
		{Command: List, ExtraArguments: []string{"extra"}},
	}
	for _, request := range requests {
		_, err := Run(t.Context(), &request)
		if !errors.Is(err, errs.ErrInputMissing) && !errors.Is(err, errs.ErrInputInvalid) {
			t.Errorf("✗ invalid request %+v: %v", request, err)
		}
	}

	assertAbsent(t, filepath.Join(home, config.Directory, config.Database))

	if !t.Failed() {
		t.Log("✓ invalid input isolation")
	}
}

// TestFailedResultWrite verifies invariant #2: Failed recording.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// SQLite triggers reject either part of recording after deletion. The item stays
// recoverable in trash; the result has no saved ID and history stays empty.
func TestFailedResultWrite(t *testing.T) {
	for _, table := range []string{"actions", "deleted"} {
		t.Run(table, func(t *testing.T) {
			home := configuredHome(t, 30)
			if _, err := Run(t.Context(), &Request{Command: List}); err != nil {
				t.Fatalf("💣 database setup: %v", err)
			}

			executeSQL(t, home, fmt.Sprintf(`CREATE TRIGGER fail_create BEFORE INSERT ON %s BEGIN SELECT RAISE(FAIL,'unit test write failure'); END`, table))
			source := filepath.Join(home, "source")
			writeFile(t, source, "recover me")

			result, err := Run(t.Context(), &Request{Command: Delete, Source: source, Reason: "remove"})
			if !errors.Is(err, errs.ErrDatabaseResult) {
				t.Errorf("✗ result error: %v", err)
			}

			if len(result.Actions) != 1 {
				t.Errorf("✗ recovery information absent: %+v", result)

				return
			}

			action := result.Actions[0]

			assertAbsent(t, source)
			assertFile(t, filepath.Join(home, "trash", action.TrashID), "recover me")

			if action.ID != 0 || action.Status != store.Completed || action.InProgress || action.ResolvedAt.IsZero() {
				t.Errorf("✗ completed outcome absent: %+v", action)
			}

			listed, err := Run(t.Context(), &Request{Command: List})
			if err != nil || len(listed.Actions) != 0 {
				t.Errorf("✗ unrecorded operation appears in history: %+v, %v", listed, err)
			}

			if !t.Failed() {
				t.Log("✓ filesystem success distinct from recording failure")
			}
		})
	}
}

// TestInterruptedRecords verifies invariant #3: Interrupted operations.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A persisted unfinished operation is retained by cleanup and refused by dispose,
// restore, and purge; inspection still exposes the record and recovery locations.
func TestInterruptedRecords(t *testing.T) {
	home := configuredHome(t, 0)
	source := filepath.Join(home, "source")
	writeFile(t, source, "payload")
	action := performAction(t, Request{Command: Delete, Source: source, Reason: "remove"})
	id := strconv.FormatInt(action.ID, 10)
	executeSQL(t, home, `UPDATE actions SET in_progress=1,status=2 WHERE action_id=?`, action.ID)

	for _, request := range []Request{{Command: Dispose, ID: id, Decision: "approve"}, {Command: Restore, ID: id}, {Command: Purge, IDs: []string{id}}, {Command: Cleanup, Orphans: true}} {
		if _, err := Run(t.Context(), &request); err == nil {
			t.Errorf("✗ interrupted operation accepted: %+v", request)
		}
	}

	viewed := performAction(t, Request{Command: View, ID: id})
	if !viewed.InProgress {
		t.Error("✗ interrupted record was changed")
	}

	assertFile(t, filepath.Join(home, "trash", action.TrashID), "payload")

	if !t.Failed() {
		t.Log("✓ interrupted records retained")
	}
}

// TestPendingPathChanges verifies invariant #4: Changed pending paths.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A destination created after a move request blocks approval without replacing it;
// a missing source blocks approval; both requests remain pending and may be denied.
func TestPendingPathChanges(t *testing.T) {
	home := configuredHome(t, 0)
	writeConfig(t, home, fmt.Sprintf("trash-path=%q\nretain-period=0\nrequire-review=[%q]\n", filepath.Join(home, "trash"), "**/source"))
	source, destination := filepath.Join(home, "source"), filepath.Join(home, "destination")
	writeFile(t, source, "original")
	action := performAction(t, Request{Command: Move, Source: source, Destination: destination, Reason: "move"})
	writeFile(t, destination, "occupant")

	_, err := Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(action.ID, 10), Decision: "approve"})
	if !errors.Is(err, errs.ErrFileExists) {
		t.Errorf("✗ changed destination: %v", err)
	}

	assertFile(t, source, "original")
	assertFile(t, destination, "occupant")

	deleted := performAction(t, Request{Command: Delete, Source: source, Reason: "delete"})
	if err = os.Rename(source, filepath.Join(home, "manual")); err != nil {
		t.Fatalf("💣 fixture move: %v", err)
	}

	_, err = Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: "approve"})
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("✗ missing source: %v", err)
	}

	result, err := Run(t.Context(), &Request{Command: Cleanup, Orphans: true})
	if err != nil || len(result.Removed) != 0 {
		t.Errorf("✗ pending history removed: %+v, %v", result, err)
	}

	denied := performAction(t, Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: "deny"})
	if denied.Status != store.Denied {
		t.Error("✗ denial after missing source failed")
	}

	if !t.Failed() {
		t.Log("✓ pending operations recheck paths")
	}
}

// TestRetentionBoundaries verifies invariant #5: Maintenance retention.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Recently purged and restored records survive orphan cleanup. Expired completed
// deletions and failed records with retained files survive; expired purge history is removed.
func TestRetentionBoundaries(t *testing.T) {
	home := configuredHome(t, 30)
	source := filepath.Join(home, "source")
	writeFile(t, source, "purge")
	purged := performAction(t, Request{Command: Delete, Source: source, Reason: "purge"})

	_, err := Run(t.Context(), &Request{Command: Purge, IDs: []string{strconv.FormatInt(purged.ID, 10)}})
	if err != nil {
		t.Errorf("✗ purge: %v", err)
	}

	writeFile(t, source, "restore")
	restored := performAction(t, Request{Command: Delete, Source: source, Reason: "restore"})
	performAction(t, Request{Command: Restore, ID: strconv.FormatInt(restored.ID, 10)})
	retained := performAction(t, Request{Command: Delete, Source: source, Reason: "retain"})
	expired := time.Now().UTC().AddDate(0, 0, -31).Format(time.RFC3339Nano)
	executeSQL(t, home, `UPDATE actions SET resolved_at=? WHERE action_id=?`, expired, retained.ID)

	result, err := Run(t.Context(), &Request{Command: Cleanup, Orphans: true})
	if err != nil || len(result.Removed) != 0 {
		t.Errorf("✗ protected history: %+v, %v", result, err)
	}

	executeSQL(t, home, `UPDATE actions SET status=6 WHERE action_id=?`, retained.ID)
	executeSQL(t, home, `UPDATE actions SET resolved_at=? WHERE action_id=?`, expired, purged.ID)

	result, err = Run(t.Context(), &Request{Command: Cleanup})
	if err != nil || len(result.Removed) != 1 || result.Removed[0] != purged.ID {
		t.Errorf("✗ retention result: %+v, %v", result, err)
	}

	assertFile(t, filepath.Join(home, "trash", retained.TrashID), "restore")

	if !t.Failed() {
		t.Log("✓ history retention boundaries")
	}
}

// TestPurgeMissingBatch verifies invariant #6: Missing purge items.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A manually relocated trash item is reported missing and not marked purged;
// another selected item is purged, and relocated contents are untouched.
func TestPurgeMissingBatch(t *testing.T) {
	home := configuredHome(t, 30)
	source := filepath.Join(home, "source")
	writeFile(t, source, "missing")
	missing := performAction(t, Request{Command: Delete, Source: source, Reason: "remove"})
	writeFile(t, source, "present")

	present := performAction(t, Request{Command: Delete, Source: source, Reason: "remove"})
	if err := os.Rename(filepath.Join(home, "trash", missing.TrashID), filepath.Join(home, "relocated")); err != nil {
		t.Fatalf("💣 fixture move: %v", err)
	}

	result, err := Run(t.Context(), &Request{Command: Purge, IDs: []string{strconv.FormatInt(missing.ID, 10), strconv.FormatInt(present.ID, 10)}})
	if !errors.Is(err, errs.ErrBatchPartial) || len(result.Actions) != 1 || len(result.Problems) != 1 {
		t.Errorf("✗ batch result: %+v, %v", result, err)
	}

	viewed := performAction(t, Request{Command: View, ID: strconv.FormatInt(missing.ID, 10)})
	if viewed.Status == store.Purged {
		t.Error("✗ missing item marked purged")
	}

	assertFile(t, filepath.Join(home, "relocated"), "missing")
	assertAbsent(t, filepath.Join(home, "trash", present.TrashID))

	if !t.Failed() {
		t.Log("✓ partial purge reports actual outcomes")
	}
}

// TestUnusableTrash verifies invariant #7: Unusable trash.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// With a file occupying the configured trash directory, a delete puts the item
// in the embedded trash directory, and the blocking file remains untouched.
func TestUnusableTrash(t *testing.T) {
	home := configuredHome(t, 30)
	source := filepath.Join(home, "source")
	writeFile(t, source, "payload")
	writeFile(t, filepath.Join(home, "trash"), "blocks directory")

	embedded, err := config.Decode(config.DefaultDocument)
	if err != nil {
		t.Fatalf("💣 embedded defaults: %v", err)
	}

	embeddedTrash, err := embedded.Trash(home)
	if err != nil {
		t.Fatalf("💣 embedded trash: %v", err)
	}

	result, err := Run(t.Context(), &Request{Command: Delete, Source: source, Reason: "remove"})
	if err != nil || len(result.Actions) != 1 || result.Actions[0].TrashPath != embeddedTrash {
		t.Fatalf("✗ delete with unusable trash: %+v, %v", result, err)
	}

	assertFile(t, filepath.Join(embeddedTrash, result.Actions[0].TrashID), "payload")
	assertFile(t, filepath.Join(home, "trash"), "blocks directory")

	if !t.Failed() {
		t.Log("✓ unusable trash gives way to the embedded trash")
	}
}

// TestCanceledRequest verifies invariant #8: Cancellation.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A context canceled before deletion leaves the source and the absent database unchanged.
func TestCanceledRequest(t *testing.T) {
	home := configuredHome(t, 30)
	source := filepath.Join(home, "source")
	writeFile(t, source, "payload")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := Run(ctx, &Request{Command: Delete, Source: source, Reason: "remove"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("✗ cancellation: %v", err)
	}

	assertFile(t, source, "payload")
	assertAbsent(t, filepath.Join(home, config.Directory, config.Database))

	if !t.Failed() {
		t.Log("✓ cancellation precedes mutation")
	}
}

// TestFailedDenialRecording verifies invariant #9: Denial recording.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A rejected final database write during denial reports a database failure,
// preserves the source, and does not report a completed filesystem operation.
func TestFailedDenialRecording(t *testing.T) {
	home := configuredHome(t, 30)
	writeConfig(t, home, fmt.Sprintf("trash-path=%q\nrequire-review=[\"**/source\"]\n", filepath.Join(home, "trash")))
	source := filepath.Join(home, "source")
	writeFile(t, source, "untouched")
	action := performAction(t, Request{Command: Delete, Source: source, Reason: "review"})
	executeSQL(t, home, `CREATE TRIGGER fail_denial BEFORE UPDATE OF status ON actions BEGIN SELECT RAISE(FAIL,'unit test write failure'); END`)

	_, err := Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(action.ID, 10), Decision: store.DenyDecision})
	if !errors.Is(err, errs.ErrDatabaseWrite) || errors.Is(err, errs.ErrDatabaseResult) {
		t.Errorf("✗ inaccurate denial failure: %v", err)
	}

	assertFile(t, source, "untouched")

	if !t.Failed() {
		t.Log("✓ denial write failures do not claim filesystem changes")
	}
}

func executeSQL(t *testing.T, home, query string, arguments ...any) {
	t.Helper()

	database, err := sql.Open("sqlite", filepath.Join(home, config.Directory, config.Database))
	if err != nil {
		t.Fatalf("💣 fixture database: %v", err)
	}

	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("✗ fixture database close: %v", err)
		}
	})

	if _, err = database.ExecContext(t.Context(), query, arguments...); err != nil {
		t.Fatalf("💣 fixture SQL: %v", err)
	}
}
