package store

// Invariants tested:
//  1. Transactional records: Action and recovery records are created and deleted together.
//  2. Exclusive execution: Only one caller can claim an unchanged action for execution.
//  3. Failure history: Subsequent success preserves earlier failure diagnostics.
//  4. Permanent action IDs: An action ID is never issued again, even after the record that held it is deleted.
//  5. Linked records: A move and its occupying delete are saved together with ordered IDs and an immutable blocker reference.
//  6. History upgrades: Opening existing history preserves records and issued IDs while enabling linked actions.

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/shdeen/saferm/internal/errs"
)

// TestActionTransactions verifies invariant #1: Transactional records.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A duplicate trash identity rolls back its action insertion. Deleting an action
// cascades to its deletion record; moved records persist with their own paths.
func TestActionTransactions(t *testing.T) {
	history := openHistory(t)

	deleted := Action{Type: "delete", Status: Pending, Reason: "remove", Source: "/source", TrashID: "unique", TrashPath: "/trash"}
	if err := history.Create(t.Context(), &deleted); err != nil {
		t.Errorf("✗ create deletion: %v", err)
	}

	duplicate := deleted
	if err := history.Create(t.Context(), &duplicate); !errors.Is(err, errs.ErrDatabaseWrite) {
		t.Errorf("✗ duplicate identity: %v", err)
	}

	moved := Action{Type: "move", Status: Pending, Reason: "relocate", Source: "/source", Destination: "/destination"}
	if err := history.Create(t.Context(), &moved); err != nil {
		t.Errorf("✗ create movement: %v", err)
	}

	actions, err := history.List(t.Context(), "", 0)
	if err != nil || len(actions) != 2 {
		t.Errorf("✗ action insertion not atomic: %+v, %v", actions, err)
	}

	if err = history.Delete(t.Context(), &deleted); err != nil {
		t.Errorf("✗ delete record: %v", err)
	}

	var count int
	if err = history.database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM deleted`).Scan(&count); err != nil || count != 0 {
		t.Errorf("✗ deletion record survived: %d, %v", count, err)
	}

	viewed, err := history.Get(t.Context(), moved.ID)
	if err != nil || viewed.Source != moved.Source || viewed.Destination != moved.Destination || viewed.CreatedAt.IsZero() || viewed.UpdatedAt.IsZero() {
		t.Errorf("✗ movement record: %+v, %v", viewed, err)
	}

	if !t.Failed() {
		t.Log("✓ atomic creation and removal")
	}
}

// TestClaimExclusion verifies invariant #2: Exclusive execution.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A second claimant holding the same record version is refused after the first
// commits its intent. Final recording resolves the action and clears its busy state.
func TestClaimExclusion(t *testing.T) {
	history := openHistory(t)

	action := Action{Type: "move", Status: Pending, Reason: "relocate", Source: "/source", Destination: "/destination"}
	if err := history.Create(t.Context(), &action); err != nil {
		t.Fatalf("💣 fixture action: %v", err)
	}

	secondClaim := action
	if err := history.Claim(t.Context(), &action, "move", "approve"); err != nil {
		t.Errorf("✗ first claim: %v", err)
	}

	if err := history.Claim(t.Context(), &secondClaim, "move", "approve"); !errors.Is(err, errs.ErrActionBusy) {
		t.Errorf("✗ second claim: %v", err)
	}

	if err := history.Finish(t.Context(), &action, Completed, nil); err != nil {
		t.Errorf("✗ finish: %v", err)
	}

	viewed, err := history.Get(t.Context(), action.ID)
	if err != nil || viewed.InProgress || viewed.Status != Completed || viewed.Decision != "approve" || viewed.ResolvedAt.IsZero() {
		t.Errorf("✗ final record: %+v, %v", viewed, err)
	}

	if !t.Failed() {
		t.Log("✓ exclusive action claims")
	}
}

// TestFailureHistory verifies invariant #3: Failure history.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A failed operation records a diagnostic. A later successful restoration retains
// that diagnostic while recording the new status and resolution time.
func TestFailureHistory(t *testing.T) {
	history := openHistory(t)

	action := Action{Type: "move", Status: Pending, Reason: "relocate", Source: "/source", Destination: "/destination", InProgress: true, Operation: "move"}
	if err := history.Create(t.Context(), &action); err != nil {
		t.Fatalf("💣 fixture action: %v", err)
	}

	if err := history.Finish(t.Context(), &action, Failed, errs.ErrFileTransfer); err != nil {
		t.Errorf("✗ failure update: %v", err)
	}

	diagnostic := action.Failure
	if diagnostic == "" {
		t.Error("✗ missing diagnostic")
	}

	if err := history.Claim(t.Context(), &action, "restore", ""); err != nil {
		t.Errorf("✗ restore claim: %v", err)
	}

	if err := history.Finish(t.Context(), &action, Restored, nil); err != nil {
		t.Errorf("✗ restoration update: %v", err)
	}

	viewed, err := history.Get(t.Context(), action.ID)
	if err != nil || viewed.Failure != diagnostic || viewed.Status != Restored {
		t.Errorf("✗ failure history lost: %+v, %v", viewed, err)
	}

	if !t.Failed() {
		t.Log("✓ failure diagnostics survive success")
	}
}

// TestActionIDsNeverReused verifies invariant #4: Permanent action IDs.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// After the most recently created records are deleted while an older record
// remains, and again after every record is deleted, each new action receives an
// ID greater than every ID issued before it.
func TestActionIDsNeverReused(t *testing.T) {
	history := openHistory(t)

	oldestAction, middleAction, newestAction := createMoveAction(t, history), createMoveAction(t, history), createMoveAction(t, history)
	highestIssuedID := newestAction.ID

	deleteActions(t, history, &middleAction, &newestAction)

	afterNewestDeleted := createMoveAction(t, history)
	if afterNewestDeleted.ID <= highestIssuedID {
		t.Errorf("✗ ID %d issued after deleting the newest records; highest earlier ID %d", afterNewestDeleted.ID, highestIssuedID)
	}

	highestIssuedID = max(highestIssuedID, afterNewestDeleted.ID)

	deleteActions(t, history, &oldestAction, &afterNewestDeleted)

	afterHistoryEmptied := createMoveAction(t, history)
	if afterHistoryEmptied.ID <= highestIssuedID {
		t.Errorf("✗ ID %d issued after deleting every record; highest earlier ID %d", afterHistoryEmptied.ID, highestIssuedID)
	}

	if !t.Failed() {
		t.Log("✓ deleted records' IDs are never issued again")
	}
}

// TestLinkedRecords verifies invariant #5: Linked records.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A saved pair has the move's lower ID and the delete as blocker. Pending
// dependents include only unresolved matches, and their query releases the
// connection before claiming or finishing. Deleting blocker history preserves
// the reference and never permits reuse of its ID.
func TestLinkedRecords(t *testing.T) {
	history := openHistory(t)
	moved := Action{Type: MoveType, Status: Pending, Source: "/source", Destination: "/destination", Reason: "relocate", ReviewRule: "/source"}
	deleted := Action{Type: DeleteType, Status: Pending, Source: "/destination", TrashID: "occupant", TrashPath: "/trash", Reason: "relocate"}

	if err := history.CreatePair(t.Context(), &moved, &deleted); err != nil || moved.ID <= 0 || moved.ID >= deleted.ID || moved.BlockerID != deleted.ID {
		t.Errorf("✗ pair identities and link: %+v, %+v, %v", moved, deleted, err)
	}

	recorded, err := history.List(t.Context(), "", 0)
	if err != nil || !reflect.DeepEqual(recorded, []Action{moved, deleted}) {
		t.Errorf("✗ complete pair was not persisted: %+v, %v", recorded, err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	dependents, err := history.PendingDependents(ctx, deleted.ID)
	if err != nil || !reflect.DeepEqual(dependents, []Action{moved}) {
		t.Errorf("✗ pending dependents: %+v, %v", dependents, err)
	}

	if err = history.Claim(ctx, &moved, MoveType, DenyDecision); err != nil {
		t.Errorf("✗ claim after dependent query: %v", err)
	}

	if err = history.Finish(ctx, &moved, Denied, nil); err != nil {
		t.Errorf("✗ finish after dependent query: %v", err)
	}

	dependents, err = history.PendingDependents(ctx, deleted.ID)
	if err != nil || len(dependents) != 0 {
		t.Errorf("✗ resolved dependent remains pending: %+v, %v", dependents, err)
	}

	if err = history.Delete(ctx, &deleted); err != nil {
		t.Errorf("✗ remove blocker history: %v", err)
	}

	preserved, err := history.Get(ctx, moved.ID)
	if err != nil || preserved.BlockerID != deleted.ID || !reflect.DeepEqual(preserved, moved) {
		t.Errorf("✗ blocker removal changed its move: %+v, %v", preserved, err)
	}

	later := createMoveAction(t, history)
	if later.ID <= deleted.ID {
		t.Errorf("✗ removed blocker ID reused: %+v", later)
	}

	if !t.Failed() {
		t.Log("✓ linked history persists, releases queries, and preserves permanent references")
	}
}

// TestHistoryUpgrade verifies invariant #6: History upgrades.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Repeated and concurrent opens upgrade an earlier database without changing
// its action, metadata, diagnostic, timestamps, or previously issued IDs.
// Subsequent linked actions can be read with their complete blocker identity.
func TestHistoryUpgrade(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(map[bool]string{false: "repeated", true: "concurrent"}[concurrent], func(t *testing.T) {
			path, original := earlierHistory(t)

			openCount := 2
			if concurrent {
				openCount = 8
			}

			failures := make(chan error, openCount)
			for range openCount {
				if concurrent {
					go openEarlierHistory(t, path, failures)
				} else {
					openEarlierHistory(t, path, failures)
				}
			}

			for range openCount {
				if err := <-failures; err != nil {
					t.Errorf("✗ opening earlier history: %v", err)
				}
			}

			history, err := Open(t.Context(), path)
			if err != nil {
				t.Errorf("✗ upgraded history unavailable: %v", err)

				return
			}
			defer history.Close()

			preserved, err := history.Get(t.Context(), original.ID)
			if err != nil || !reflect.DeepEqual(preserved, original) {
				t.Errorf("✗ upgrade changed prior record: %+v, %v", preserved, err)
			}

			moved := Action{Type: MoveType, Status: Pending, Source: "/new", Destination: "/destination", Reason: "relocate"}

			deleted := Action{Type: DeleteType, Status: Completed, Source: "/destination", TrashID: "occupant", TrashPath: "/trash", Reason: "relocate"}
			if err = history.CreatePair(t.Context(), &moved, &deleted); err != nil || moved.ID <= 11 || deleted.ID <= moved.ID {
				t.Errorf("✗ upgrade lost issued IDs or cannot save a pair: %+v, %+v, %v", moved, deleted, err)
			}

			storedMove, err := history.Get(t.Context(), moved.ID)
			if err != nil || storedMove.BlockerID != deleted.ID || storedMove.BlockerID == 0 {
				t.Errorf("✗ upgraded history lost blocker: %+v, %v", storedMove, err)
			}

			if !t.Failed() {
				t.Log("✓ earlier records and issued IDs survive opening and support linked actions")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ serial and concurrent upgrades preserve history")
	}
}

func openHistory(t *testing.T) *History {
	t.Helper()

	history, err := Open(t.Context(), filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatalf("💣 fixture database: %v", err)
	}

	t.Cleanup(func() {
		if err := history.Close(); err != nil {
			t.Errorf("✗ database close: %v", err)
		}
	})

	return history
}

// earlierHistory creates the former schema with a retained action and a deleted
// newer ID, exercising preservation of both record data and the ID sequence.
func earlierHistory(t *testing.T) (string, Action) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "history.sqlite")

	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("💣 earlier database: %v", err)
	}
	defer database.Close()

	ddl, err := os.ReadFile("testdata/history.sql")
	if err != nil {
		t.Fatalf("💣 earlier schema: %v", err)
	}

	if _, err = database.ExecContext(t.Context(), string(ddl)); err != nil {
		t.Fatalf("💣 create earlier schema: %v", err)
	}

	timestamp := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	original := Action{ID: 7, Type: MoveType, Status: Failed, Source: "/old", Destination: "/destination", Reason: "retain", Requester: "Agent", Description: "old metadata", ReviewRule: "/old", Decision: ApproveDecision, Failure: "earlier diagnostic", Operation: MoveType, CreatedAt: timestamp, UpdatedAt: timestamp, ResolvedAt: timestamp}

	_, err = database.ExecContext(t.Context(), `INSERT INTO actions(action_id,action_type,status,reason,requester,description,created_at,updated_at,resolved_at,review_rule,decision,failure,operation)
 VALUES(7,'move',6,'retain','Agent','old metadata',?,?,?,'/old','approve','earlier diagnostic','move');
 INSERT INTO moved VALUES(7,'/old','/destination');
 INSERT INTO actions(action_id,action_type,status,reason,created_at,updated_at) VALUES(11,'move',3,'removed',?,?);
 DELETE FROM actions WHERE action_id=11`, formatTime(timestamp), formatTime(timestamp), formatTime(timestamp), formatTime(timestamp), formatTime(timestamp))
	if err != nil {
		t.Fatalf("💣 seed earlier history: %v", err)
	}

	return path, original
}

func openEarlierHistory(t *testing.T, path string, failures chan<- error) {
	t.Helper()

	history, err := Open(t.Context(), path)
	if err == nil {
		err = history.Close()
	}

	failures <- err
}

// createMoveAction stores a completed move action and returns it with its ID.
func createMoveAction(t *testing.T, history *History) Action {
	t.Helper()

	action := Action{Type: MoveType, Status: Completed, Reason: "relocate", Source: "/source", Destination: "/destination"}
	if err := history.Create(t.Context(), &action); err != nil {
		t.Fatalf("💣 fixture action: %v", err)
	}

	return action
}

// deleteActions deletes the stored records of the actions.
func deleteActions(t *testing.T, history *History, actions ...*Action) {
	t.Helper()

	for _, action := range actions {
		if err := history.Delete(t.Context(), action); err != nil {
			t.Fatalf("💣 delete fixture action %d: %v", action.ID, err)
		}
	}
}
