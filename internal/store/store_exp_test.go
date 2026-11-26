package store

// Invariants tested:
//  1. Storage failures: Closed or corrupt storage returns identifiable errors without partial records.
//  2. Pair rollback: Any rejected pair write or commit saves neither action and exposes no assigned ID.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/shdeen/saferm/internal/errs"
)

// TestStorageFailures verifies invariant #1: Storage failures.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Missing IDs, corrupted timestamps, and operations on a closed connection return
// the appropriate errors. Failed reads never produce an apparently valid action.
func TestStorageFailures(t *testing.T) {
	history := openHistory(t)
	if _, err := history.Get(t.Context(), 1); !errors.Is(err, errs.ErrActionMissing) {
		t.Errorf("✗ missing action: %v", err)
	}

	action := Action{Type: "move", Status: Pending, Reason: "relocate", Source: "/source", Destination: "/destination"}
	if err := history.Create(t.Context(), &action); err != nil {
		t.Fatalf("💣 fixture action: %v", err)
	}

	if _, err := history.database.ExecContext(t.Context(), `UPDATE actions SET created_at='invalid'`); err != nil {
		t.Fatalf("💣 fixture corruption: %v", err)
	}

	if _, err := history.Get(t.Context(), action.ID); !errors.Is(err, errs.ErrDatabaseRead) {
		t.Errorf("✗ corrupt timestamp: %v", err)
	}

	if _, err := history.List(t.Context(), "", 0); !errors.Is(err, errs.ErrDatabaseRead) {
		t.Errorf("✗ corrupt list: %v", err)
	}

	if err := history.Close(); err != nil {
		t.Fatalf("💣 fixture close: %v", err)
	}

	if _, err := history.List(t.Context(), "", 0); !errors.Is(err, errs.ErrDatabaseRead) {
		t.Errorf("✗ closed read: %v", err)
	}

	if _, err := history.PendingDependents(t.Context(), action.ID); !errors.Is(err, errs.ErrDatabaseRead) {
		t.Errorf("✗ closed dependent read: %v", err)
	}

	moved := Action{Type: MoveType, Status: Pending, Source: "/moving", Destination: "/occupied"}

	deleted := Action{Type: DeleteType, Status: Pending, Source: "/occupied", TrashID: "occupant", TrashPath: "/trash"}
	if err := history.CreatePair(t.Context(), &moved, &deleted); !errors.Is(err, errs.ErrDatabaseWrite) || moved.ID != 0 || deleted.ID != 0 {
		t.Errorf("✗ closed pair write exposed IDs or lost its error: %+v, %+v, %v", moved, deleted, err)
	}

	if err := history.Create(t.Context(), &action); !errors.Is(err, errs.ErrDatabaseWrite) {
		t.Errorf("✗ closed create: %v", err)
	}

	if err := history.Claim(t.Context(), &action, "move", ""); !errors.Is(err, errs.ErrDatabaseWrite) {
		t.Errorf("✗ closed claim: %v", err)
	}

	if err := history.Finish(t.Context(), &action, Completed, nil); !errors.Is(err, errs.ErrDatabaseWrite) {
		t.Errorf("✗ closed finish: %v", err)
	}

	if err := history.Delete(t.Context(), &action); !errors.Is(err, errs.ErrDatabaseWrite) {
		t.Errorf("✗ closed delete: %v", err)
	}

	if !t.Failed() {
		t.Log("✓ storage failures stay visible")
	}
}

// TestPairRollback verifies invariant #2: Pair rollback.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// SQLite failures at either action insert, either child insert, linkage, or a
// deferred constraint at commit leave no pair rows or assigned IDs. Existing
// history and later writes remain usable after every rollback.
func TestPairRollback(t *testing.T) {
	for _, failureCase := range []struct {
		name string
		sql  string
	}{
		{"move action", `CREATE TRIGGER reject_pair BEFORE INSERT ON actions WHEN NEW.action_type='move' BEGIN SELECT RAISE(FAIL,'reject move'); END`},
		{"delete action", `CREATE TRIGGER reject_pair BEFORE INSERT ON actions WHEN NEW.action_type='delete' BEGIN SELECT RAISE(FAIL,'reject delete'); END`},
		{"move details", `CREATE TRIGGER reject_pair BEFORE INSERT ON moved BEGIN SELECT RAISE(FAIL,'reject move details'); END`},
		{"delete details", `CREATE TRIGGER reject_pair BEFORE INSERT ON deleted BEGIN SELECT RAISE(FAIL,'reject delete details'); END`},
		{"link", `CREATE TRIGGER reject_pair BEFORE UPDATE OF blocker_id ON actions BEGIN SELECT RAISE(FAIL,'reject link'); END`},
		{"commit", `CREATE TABLE invalid_reference(action_id INTEGER REFERENCES actions(action_id) DEFERRABLE INITIALLY DEFERRED); CREATE TRIGGER reject_pair AFTER UPDATE OF blocker_id ON actions BEGIN INSERT INTO invalid_reference VALUES(-1); END`},
	} {
		t.Run(failureCase.name, func(t *testing.T) {
			history := openHistory(t)

			original := createMoveAction(t, history)
			if _, err := history.database.ExecContext(t.Context(), failureCase.sql); err != nil {
				t.Fatalf("💣 failing transaction fixture: %v", err)
			}

			moved := Action{Type: MoveType, Status: Pending, Source: "/source", Destination: "/destination", Reason: "relocate"}

			deleted := Action{Type: DeleteType, Status: Completed, Source: "/destination", TrashID: "occupant", TrashPath: "/trash", Reason: "relocate"}
			if err := history.CreatePair(t.Context(), &moved, &deleted); !errors.Is(err, errs.ErrDatabaseWrite) {
				t.Errorf("✗ rejected pair did not report a write failure: %v", err)
			}

			if moved.ID != 0 || deleted.ID != 0 || moved.BlockerID != 0 {
				t.Errorf("✗ uncommitted identities escaped: %+v, %+v", moved, deleted)
			}

			actions, err := history.List(t.Context(), "", 0)
			if err != nil || !reflect.DeepEqual(actions, []Action{original}) {
				t.Errorf("✗ pair partially persisted: %+v, %v", actions, err)
			}

			var childCount int
			if err = history.database.QueryRowContext(t.Context(), `SELECT (SELECT COUNT(*) FROM moved)+(SELECT COUNT(*) FROM deleted)`).Scan(&childCount); err != nil || childCount != 1 {
				t.Errorf("✗ failed pair retained child records: %d, %v", childCount, err)
			}

			if _, err = history.database.ExecContext(t.Context(), `DROP TRIGGER reject_pair`); err != nil {
				t.Fatalf("💣 remove transaction rejection: %v", err)
			}

			if err = history.CreatePair(t.Context(), &moved, &deleted); err != nil || moved.ID <= original.ID || deleted.ID <= moved.ID || moved.BlockerID != deleted.ID {
				t.Errorf("✗ history unusable after rollback: %+v, %+v, %v", moved, deleted, err)
			}

			if !t.Failed() {
				t.Log("✓ failed pair leaves no records or identities and subsequent persistence succeeds")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ every pair write failure rolls back")
	}
}
