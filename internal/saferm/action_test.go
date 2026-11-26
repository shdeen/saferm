package saferm

// Invariants tested:
//  1. Recoverable replacement: Restore records the occupant's delete before returning the original item.
//  2. Reviewed replacement: An occupant requiring approval leaves the original restore unchanged.
//  3. Occupant types: Replacement retains directories and symbolic links as recoverable items.
//  4. Linked approval: A delete approval runs an unprotected move, while combined consent approves both named actions.
//  5. Linked denial: Denying a blocker denies its move with a retained diagnostic and no file changes.
//  6. Move replacement: Each overwrite preserves its occupant under an independently reviewed delete before moving the source.
//  7. Replacement consent: An occupied move asks before effects, while an empty move requires no trash.
//  8. Independent replacements: Separate moves to one destination retain separate occupying deletes in either approval order.
//  9. Protected requests: A delete or move waits for approval when a pattern protects its source, a directory above the source, or an item inside the source, and it records the first listed pattern that protects the source, or else an item inside it.
//  10. Unfollowed links: A symbolic link inside a deleted directory is judged as a link, and the item it points to is neither tested nor changed.
//  11. Approval status: Only operations requiring a separate decision are pending.
//  12. Request notifications: Creating a pending request pushes its saved identity once.
//  13. Malformed rules: While the list holds a malformed pattern, every delete and move is saved pending, and the request returns an error that names the pattern.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/store"
)

// TestRestorePreservesOccupant verifies invariant #1: Recoverable replacement.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Confirmed restoration of a deleted or moved file returns the original bytes
// and preserves the occupant under its own completed, recoverable delete.
func TestRestorePreservesOccupant(t *testing.T) {
	for _, originalType := range []string{Delete, Move} {
		t.Run(originalType, func(t *testing.T) {
			home := configuredHome(t, 30)
			source := filepath.Join(home, "source")
			writeFile(t, source, "original contents")
			original := performAction(t, Request{Command: originalType, Source: source, Destination: filepath.Join(home, "destination"), Reason: "retain original"})
			writeFile(t, source, "occupying contents")

			result, err := Run(t.Context(), &Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true, Confirmed: true})
			if err != nil || len(result.Actions) != 2 {
				t.Errorf("✗ %s restore must return occupant delete and restored original: %+v, %v", originalType, result, err)

				return
			}

			deleted, restored := result.Actions[0], result.Actions[1]
			if deleted.ID == original.ID || deleted.Type != Delete || deleted.Status != store.Completed || deleted.Source != original.Source || !strings.Contains(deleted.Reason, strconv.FormatInt(original.ID, 10)) {
				t.Errorf("✗ occupant delete: %+v", deleted)
			}

			if restored.ID != original.ID || restored.Status != store.Restored || restored.InProgress {
				t.Errorf("✗ restored original: %+v", restored)
			}

			assertFile(t, source, "original contents")
			assertFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")

			recorded := performAction(t, Request{Command: View, ID: strconv.FormatInt(deleted.ID, 10)})
			if !reflect.DeepEqual(deleted, recorded) {
				t.Errorf("✗ occupant delete was not recorded: %+v", recorded)
			}

			if !t.Failed() {
				t.Log("✓ restore case preserves its required outcome")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ restore preserves both items and records the occupying delete")
	}
}

// TestRestoreRequiresOccupantReview verifies invariant #2: Reviewed replacement.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Force and explicit consent both respect review of the occupant. The original
// record and bytes remain unchanged until a separate delete approval and restore.
func TestRestoreRequiresOccupantReview(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run("force="+strconv.FormatBool(force), func(t *testing.T) {
			home := configuredHome(t, 30)
			source := filepath.Join(home, "source")
			writeFile(t, source, "original contents")
			original := performAction(t, Request{Command: Delete, Source: source, Reason: "retain original"})
			writeFile(t, source, "occupying contents")
			writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = [%q]\n", filepath.Join(home, "trash"), source))

			result, err := Run(t.Context(), &Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true, Force: force, Confirmed: !force})
			if !errors.Is(err, errs.ErrRestorePending) || len(result.Actions) != 1 || result.Actions[0].Type != Delete || result.Actions[0].Status != store.Pending {
				t.Errorf("✗ protected occupant must await review: %+v, %v", result, err)

				return
			}

			deleted := result.Actions[0]

			pendingMessage := fmt.Sprintf(strings.ReplaceAll(appstrings.Text("error.restorePendingDetail"), "%w", errs.ErrRestorePending.Error()), deleted.ID)
			if err.Error() != pendingMessage {
				t.Errorf("✗ pending restore did not render its complete catalog message: %v", err)
			}

			recorded := performAction(t, Request{Command: View, ID: strconv.FormatInt(original.ID, 10)})
			if !reflect.DeepEqual(recorded, original) {
				t.Errorf("✗ pending occupant changed original action: %+v", recorded)
			}

			assertFile(t, source, "occupying contents")
			assertFile(t, filepath.Join(home, "trash", original.TrashID), "original contents")
			performAction(t, Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: store.ApproveDecision})

			restored := performAction(t, Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10)})
			if restored.Status != store.Restored {
				t.Errorf("✗ original could not be restored after approval: %+v", restored)
			}

			assertFile(t, source, "original contents")
			assertFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")

			if !t.Failed() {
				t.Log("✓ restore case preserves its required outcome")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ review leaves the original restore available until the occupant is approved")
	}
}

// TestRestoreOccupantTypes verifies invariant #3: Occupant types.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// A directory retains nested contents; a live or dangling link retains its
// literal target. Each occupying delete can subsequently be restored, and link
// targets remain untouched throughout both operations.
func TestRestoreOccupantTypes(t *testing.T) {
	for _, occupantType := range []string{"directory", "symlink", "dangling symlink"} {
		t.Run(occupantType, func(t *testing.T) {
			home := configuredHome(t, 30)
			source := filepath.Join(home, "source")
			writeFile(t, source, "original contents")
			original := performAction(t, Request{Command: Delete, Source: source, Reason: "retain original"})

			target := filepath.Join(home, "target")
			if occupantType == "directory" {
				writeFile(t, filepath.Join(source, "nested", "file"), "occupying contents")
			} else {
				if occupantType == "symlink" {
					writeFile(t, target, "link target contents")
				}

				if err := os.Symlink(target, source); err != nil {
					t.Fatalf("💣 occupying symbolic link: %v", err)
				}
			}

			result, err := Run(t.Context(), &Request{Command: Restore, ID: strconv.FormatInt(original.ID, 10), Overwrite: true, Confirmed: true})
			if err != nil || len(result.Actions) != 2 {
				t.Errorf("✗ %s replacement: %+v, %v", occupantType, result, err)

				return
			}

			assertFile(t, source, "original contents")

			deleted := result.Actions[0]
			trashItem := filepath.Join(home, "trash", deleted.TrashID)
			assertOccupantType(t, trashItem, target, occupantType)
			performAction(t, Request{Command: Delete, Source: source, Reason: "retain original again"})

			recovered := performAction(t, Request{Command: Restore, ID: strconv.FormatInt(deleted.ID, 10)})
			if recovered.Status != store.Restored {
				t.Errorf("✗ occupying delete cannot be restored: %+v", recovered)
			}

			assertOccupantType(t, source, target, occupantType)

			if !t.Failed() {
				t.Log("✓ restore case preserves its required outcome")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ directories and symbolic links survive replacement and remain restorable")
	}
}

// TestLinkedApproval verifies invariant #4: Linked approval.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Direct delete approval automatically executes only an unprotected move.
// Combined consent names both IDs and the occupant path, changes nothing before
// consent, and then approves both named actions exactly once, even when the
// source requires review. Results contain each changed action once, in its final
// persisted state, and later decisions on a resolved move are refused.
func TestLinkedApproval(t *testing.T) {
	for _, protected := range []bool{false, true} {
		for _, combined := range []bool{false, true} {
			t.Run(fmt.Sprintf("protected=%t/combined=%t", protected, combined), func(t *testing.T) {
				home, moved, deleted := pendingPair(t, protected)

				request := Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: store.ApproveDecision}
				if combined {
					request.ID = strconv.FormatInt(moved.ID, 10)

					question, err := Run(t.Context(), &request)
					if !errors.Is(err, errs.ErrConfirmation) || question.Confirmation == nil {
						t.Errorf("✗ blocked move did not ask for consent: %+v, %v", question, err)
					} else if question.Confirmation.MoveID != moved.ID || question.Confirmation.BlockerID != deleted.ID || question.Confirmation.Path != deleted.Source || question.Confirmation.Prompt != appstrings.Text("prompt.blockedApprove") || question.Confirmation.DefaultYes {
						t.Errorf("✗ combined question lacks the named actions and path: %+v", question.Confirmation)
					}

					assertRecorded(t, moved, deleted)
					assertFile(t, moved.Source, "moving contents")
					assertFile(t, deleted.Source, "occupying contents")

					request.Confirmed = true
				}

				result, err := Run(t.Context(), &request)
				if err != nil {
					t.Errorf("✗ linked approval failed: %+v, %v", result, err)
				}

				assertUniqueResults(t, result)

				affectedIDs := map[int64]bool{deleted.ID: true}
				if !protected || combined {
					affectedIDs[moved.ID] = true
				}

				for _, action := range result.Actions {
					if !affectedIDs[action.ID] {
						t.Errorf("✗ unrelated action returned: %+v", action)
					}

					delete(affectedIDs, action.ID)
				}

				if len(affectedIDs) != 0 {
					t.Errorf("✗ affected actions missing from result: %v", affectedIDs)
				}

				assertFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")
				storedMove := performAction(t, Request{Command: View, ID: strconv.FormatInt(moved.ID, 10)})

				storedDelete := performAction(t, Request{Command: View, ID: strconv.FormatInt(deleted.ID, 10)})
				if storedDelete.Status != store.Completed || storedDelete.Decision != store.ApproveDecision || storedDelete.ResolvedAt.IsZero() {
					t.Errorf("✗ blocker was not approved: %+v", storedDelete)
				}

				if protected && !combined {
					if !reflect.DeepEqual(storedMove, moved) {
						t.Errorf("✗ direct delete approval changed a protected source: %+v", storedMove)
					}

					assertFile(t, moved.Source, "moving contents")
					assertAbsent(t, moved.Destination)
					storedMove = performAction(t, Request{Command: Dispose, ID: strconv.FormatInt(moved.ID, 10), Decision: store.ApproveDecision})
				}

				if storedMove.Status != store.Completed || storedMove.Decision != store.ApproveDecision || storedMove.ResolvedAt.IsZero() || storedMove.BlockerID != deleted.ID {
					t.Errorf("✗ linked move did not complete: %+v", storedMove)
				}

				assertFile(t, moved.Destination, "moving contents")
				assertAbsent(t, moved.Source)

				_, err = Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(moved.ID, 10), Decision: store.ApproveDecision, Confirmed: true})
				if !errors.Is(err, errs.ErrActionResolved) {
					t.Errorf("✗ resolved move accepted another decision: %v", err)
				}

				if !t.Failed() {
					t.Log("✓ approval respects independent review and combined consent")
				}
			})
		}
	}

	if !t.Failed() {
		t.Log("✓ linked approvals preserve both items and execute each move once")
	}
}

// TestLinkedDenial verifies invariant #5: Linked denial.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Direct blocker denial and confirmed combined denial close both records,
// retain earlier diagnostics, and append the blocker's ID and denial outcome.
// Before combined consent, neither record nor either file changes.
func TestLinkedDenial(t *testing.T) {
	for _, combined := range []bool{false, true} {
		t.Run("combined="+strconv.FormatBool(combined), func(t *testing.T) {
			_, moved, deleted := pendingPair(t, true)

			request := Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: store.DenyDecision}
			if combined {
				request.ID = strconv.FormatInt(moved.ID, 10)

				question, err := Run(t.Context(), &request)
				if !errors.Is(err, errs.ErrConfirmation) || question.Confirmation == nil {
					t.Errorf("✗ blocked denial did not ask: %+v, %v", question, err)
				} else if question.Confirmation.MoveID != moved.ID || question.Confirmation.BlockerID != deleted.ID || question.Confirmation.Path != deleted.Source || question.Confirmation.Prompt != appstrings.Text("prompt.blockedDeny") || question.Confirmation.DefaultYes {
					t.Errorf("✗ denial question lacks action context: %+v", question.Confirmation)
				}

				assertRecorded(t, moved, deleted)

				request.Confirmed = true
			}

			result, err := Run(t.Context(), &request)
			if err != nil || len(result.Actions) != 2 {
				t.Errorf("✗ denial must return both changed actions: %+v, %v", result, err)
			}

			assertUniqueResults(t, result)
			storedMove := performAction(t, Request{Command: View, ID: strconv.FormatInt(moved.ID, 10)})

			storedDelete := performAction(t, Request{Command: View, ID: strconv.FormatInt(deleted.ID, 10)})
			for _, action := range []store.Action{storedMove, storedDelete} {
				if action.Status != store.Denied || action.Decision != store.DenyDecision || action.ResolvedAt.IsZero() || action.InProgress {
					t.Errorf("✗ denied action remains unresolved: %+v", action)
				}
			}

			if !strings.HasPrefix(storedMove.Failure, moved.Failure+"\n") || !strings.Contains(storedMove.Failure, fmt.Sprintf(strings.ReplaceAll(appstrings.Text("diagnostic.blockerDenied"), "%w", appstrings.Text("error.blockerDenied")), deleted.ID)) {
				t.Errorf("✗ blocker denial lost diagnostic history: %+v", storedMove)
			}

			assertFile(t, moved.Source, "moving contents")
			assertFile(t, deleted.Source, "occupying contents")

			if !t.Failed() {
				t.Log("✓ linked denial closes both records without transferring files")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ direct and combined denials retain diagnostic history")
	}
}

// TestMoveOverwrite verifies invariant #6: Move replacement.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Each source/destination review combination saves an ordered move/delete pair
// with independent review causes and copied request metadata. Force skips only
// the question. Files remain intact until authorized, and only pending actions
// send notifications, with the occupying delete before its move.
func TestMoveOverwrite(t *testing.T) {
	for _, sourceReview := range []bool{false, true} {
		for _, occupantReview := range []bool{false, true} {
			t.Run(fmt.Sprintf("source=%t/occupant=%t", sourceReview, occupantReview), func(t *testing.T) {
				homeDirectory := configuredHome(t, 30)

				home, err := filepath.EvalSymlinks(homeDirectory)
				if err != nil {
					t.Fatalf("💣 resolve fixture home: %v", err)
				}

				source, destination := filepath.Join(home, "source"), filepath.Join(home, "destination")
				writeFile(t, source, "moving contents")
				writeFile(t, destination, "occupying contents")

				reviewPaths := []string{}
				if sourceReview {
					reviewPaths = append(reviewPaths, source)
				}

				if occupantReview {
					reviewPaths = append(reviewPaths, destination)
				}

				reviewDocument, err := json.Marshal(reviewPaths)
				if err != nil {
					t.Fatalf("💣 review configuration: %v", err)
				}

				writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = %s\n", filepath.Join(home, "trash"), reviewDocument))
				listener := notificationReceiver(t, home)
				request := Request{Command: Move, Source: source, Destination: destination, Reason: "replace", Requester: "Agent", Description: "retain both", Overwrite: true, Force: true}

				result, err := Run(t.Context(), &request)
				if err != nil || len(result.Actions) != 2 {
					t.Errorf("✗ overwrite did not return its pair: %+v, %v", result, err)

					return
				}

				moved, deleted := result.Actions[0], result.Actions[1]
				if moved.ID <= 0 || moved.ID >= deleted.ID || moved.BlockerID != deleted.ID || moved.Type != Move || deleted.Type != Delete || deleted.Source != destination {
					t.Errorf("✗ pair identity and order: %+v", result)
				}

				for _, action := range result.Actions {
					if action.Reason != request.Reason || action.Requester != request.Requester || action.Description != request.Description {
						t.Errorf("✗ request metadata lost: %+v", action)
					}
				}

				if (moved.ReviewRule != "") != sourceReview || (deleted.ReviewRule != "") != occupantReview || deleted.BlockerID != 0 {
					t.Errorf("✗ review causes were not independent: %+v", result)
				}

				pendingIDs := []int64{}

				if occupantReview {
					if deleted.Status != store.Pending || moved.Status != store.Pending {
						t.Errorf("✗ reviewed occupant did not hold both actions: %+v", result)
					}

					assertFile(t, source, "moving contents")
					assertFile(t, destination, "occupying contents")

					pendingIDs = append(pendingIDs, deleted.ID)
				} else {
					if deleted.Status != store.Completed || deleted.ResolvedAt.IsZero() {
						t.Errorf("✗ unreviewed delete did not complete: %+v", deleted)
					}

					assertFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")
				}

				if sourceReview || occupantReview {
					pendingIDs = append(pendingIDs, moved.ID)
					if moved.Status != store.Pending {
						t.Errorf("✗ pending cause did not retain move: %+v", moved)
					}
				} else if moved.Status != store.Completed {
					t.Errorf("✗ unprotected move did not complete: %+v", moved)
				}

				assertActionNotifications(t, listener, pendingIDs)
				assertUniqueResults(t, result)

				if occupantReview {
					_, err = Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: store.ApproveDecision})
					if err != nil {
						t.Errorf("✗ occupying approval failed: %v", err)
					}
				}

				if sourceReview {
					performAction(t, Request{Command: Dispose, ID: strconv.FormatInt(moved.ID, 10), Decision: store.ApproveDecision})
				}

				assertFile(t, destination, "moving contents")
				assertAbsent(t, source)
				assertFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")
				assertActionNotifications(t, listener, nil)

				if !t.Failed() {
					t.Log("✓ overwrite preserves contents and independent reviews")
				}
			})
		}
	}

	if !t.Failed() {
		t.Log("✓ all four review combinations preserve their required outcomes")
	}
}

// TestMoveConsent verifies invariant #7: Replacement consent.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// An occupied overwrite question has the resolved path and no default consent.
// Refusing it leaves files and history intact. Confirming preserves the occupant
// and moves the source. Empty moves with overwrite work without a trash setting.
func TestMoveConsent(t *testing.T) {
	for _, occupied := range []bool{false, true} {
		t.Run("occupied="+strconv.FormatBool(occupied), func(t *testing.T) {
			homeDirectory := configuredHome(t, 30)

			home, err := filepath.EvalSymlinks(homeDirectory)
			if err != nil {
				t.Fatalf("💣 resolve fixture home: %v", err)
			}

			source, destination := filepath.Join(home, "source"), filepath.Join(home, "destination")
			writeFile(t, source, "moving contents")

			request := Request{Command: Move, Source: source, Destination: destination, Reason: "replace", Overwrite: true}
			if occupied {
				writeFile(t, destination, "occupying contents")

				question, err := Run(t.Context(), &request)
				if !errors.Is(err, errs.ErrConfirmation) || question.Confirmation == nil {
					t.Errorf("✗ overwrite did not ask: %+v, %v", question, err)
				} else if question.Confirmation.Path != destination || question.Confirmation.DefaultYes || question.Confirmation.Prompt != appstrings.Text("prompt.moveOverwrite") {
					t.Errorf("✗ invalid overwrite question: %+v", question.Confirmation)
				}

				assertFile(t, source, "moving contents")
				assertFile(t, destination, "occupying contents")

				history, err := Run(t.Context(), &Request{Command: List})
				if err != nil || len(history.Actions) != 0 {
					t.Errorf("✗ question changed history: %+v, %v", history, err)
				}

				request.Confirmed = true
			} else {
				writeConfig(t, home, "require-review = []\n")
			}

			result, err := Run(t.Context(), &request)
			if err != nil || len(result.Actions) == 0 || result.Actions[0].Type != Move || result.Actions[0].Status != store.Completed {
				t.Errorf("✗ permitted move failed: %+v, %v", result, err)
			}

			if occupied {
				if len(result.Actions) != 2 {
					t.Errorf("✗ occupying delete omitted: %+v", result)
				} else {
					assertFile(t, filepath.Join(home, "trash", result.Actions[1].TrashID), "occupying contents")
				}
			} else if len(result.Actions) != 1 || result.Actions[0].BlockerID != 0 {
				t.Errorf("✗ ordinary move created an unnecessary pair: %+v", result)
			}

			assertFile(t, destination, "moving contents")
			assertAbsent(t, source)

			if !t.Failed() {
				t.Log("✓ move consent applies only to an occupying item")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ consent and trash requirements follow actual replacement")
	}
}

// TestIndependentMoves verifies invariant #8: Independent replacements.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Two overwrite requests to one destination create distinct deletes. In either
// approval order, each move preserves the item actually occupying its destination
// and the last approved source becomes the destination's contents.
func TestIndependentMoves(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run("reverse="+strconv.FormatBool(reverse), func(t *testing.T) {
			homeDirectory := configuredHome(t, 30)

			home, err := filepath.EvalSymlinks(homeDirectory)
			if err != nil {
				t.Fatalf("💣 resolve fixture home: %v", err)
			}

			destination := filepath.Join(home, "destination")
			writeFile(t, destination, "original occupant")
			writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = [%q]\n", filepath.Join(home, "trash"), destination))

			pairs := make([]Result, 0, 2)

			for _, name := range []string{"first", "second"} {
				source := filepath.Join(home, name)
				writeFile(t, source, name)

				result, err := Run(t.Context(), &Request{Command: Move, Source: source, Destination: destination, Reason: "replace", Overwrite: true, Force: true})
				if err != nil || len(result.Actions) != 2 {
					t.Errorf("✗ independent pair missing: %+v, %v", result, err)

					return
				}

				pairs = append(pairs, result)
			}

			if pairs[0].Actions[1].ID == pairs[1].Actions[1].ID || pairs[0].Actions[1].TrashID == pairs[1].Actions[1].TrashID {
				t.Errorf("✗ requests share an occupying delete: %+v", pairs)
			}

			order := []int{0, 1}
			if reverse {
				order = []int{1, 0}
			}

			occupyingContents := "original occupant"

			for _, index := range order {
				moved, deleted := pairs[index].Actions[0], pairs[index].Actions[1]

				result, err := Run(t.Context(), &Request{Command: Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: store.ApproveDecision})
				if err != nil || len(result.Actions) != 2 {
					t.Errorf("✗ pair did not complete: %+v, %v", result, err)
				}

				assertUniqueResults(t, result)
				assertFile(t, filepath.Join(home, "trash", deleted.TrashID), occupyingContents)
				occupyingContents = filepath.Base(moved.Source)
				assertFile(t, destination, occupyingContents)
				assertAbsent(t, moved.Source)
			}

			if !t.Failed() {
				t.Log("✓ independent replacements preserve each displaced item")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ either approval order preserves both displaced items")
	}
}

// TestProtectedRequests verifies invariant #9: Protected requests.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Each case configures its patterns, creates its files and directories under
// the home directory, and submits one delete or move. The request must be
// pending and record the listed pattern. Every file must keep its contents,
// every directory must remain, and a move's destination must stay empty.
func TestProtectedRequests(t *testing.T) {
	for _, example := range []struct {
		name         string
		patterns     []string
		files        []string
		directories  []string
		command      string
		source       string
		destination  string
		recordedRule string
	}{
		{"file under a protected directory", []string{"~/vault/"}, []string{"vault/deep/file"}, nil, Delete, "vault/deep/file", "", "~/vault/"},
		{"name pattern listed first", []string{"*.pem", "~/vault/"}, []string{"vault/key.pem"}, nil, Delete, "vault/key.pem", "", "*.pem"},
		{"path pattern listed first", []string{"~/vault/", "*.pem"}, []string{"vault/key.pem"}, nil, Delete, "vault/key.pem", "", "~/vault/"},
		{"two patterns match one name", []string{"*.txt", "notes*"}, []string{"notes.txt"}, nil, Delete, "notes.txt", "", "*.txt"},
		{"directory that holds a protected directory", []string{".git/"}, []string{"proj/.git/config"}, nil, Delete, "proj", "", ".git/"},
		{"first listed pattern among the contents", []string{"*.pem", ".git/"}, []string{"proj/.git/config", "proj/deep/key.pem"}, nil, Delete, "proj", "", "*.pem"},
		{"move of a directory with protected contents", []string{".git/"}, []string{"proj/.git/config"}, []string{"archive"}, Move, "proj", "archive/proj", ".git/"},
		{"empty directory matched together with its contents", []string{"~/box/**"}, nil, []string{"box"}, Delete, "box", "", "~/box/**"},
	} {
		t.Run(example.name, func(t *testing.T) {
			home := isolateHome(t)

			reviewDocument, err := json.Marshal(example.patterns)
			if err != nil {
				t.Fatalf("💣 review configuration: %v", err)
			}

			writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = %s\n", filepath.Join(home, "trash"), reviewDocument))

			for _, file := range example.files {
				writeFile(t, filepath.Join(home, filepath.FromSlash(file)), "contents of "+file)
			}

			for _, directory := range example.directories {
				if err := os.MkdirAll(filepath.Join(home, directory), 0o700); err != nil {
					t.Fatalf("💣 fixture directory: %v", err)
				}
			}

			request := Request{Command: example.command, Source: filepath.Join(home, filepath.FromSlash(example.source)), Reason: "review"}
			if example.destination != "" {
				request.Destination = filepath.Join(home, filepath.FromSlash(example.destination))
			}

			action := performAction(t, request)
			if action.Status != store.Pending || action.ReviewRule != example.recordedRule {
				t.Errorf("✗ pending record: %+v", action)
			}

			for _, file := range example.files {
				assertFile(t, filepath.Join(home, filepath.FromSlash(file)), "contents of "+file)
			}

			for _, directory := range example.directories {
				if directoryInfo, err := os.Stat(filepath.Join(home, directory)); err != nil || !directoryInfo.IsDir() {
					t.Errorf("✗ directory %q changed: %v", directory, err)
				}
			}

			if example.destination != "" {
				assertAbsent(t, request.Destination)
			}

			if !t.Failed() {
				t.Log("✓ the request waits and records its pattern")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ protected sources, parents, and contents wait for approval")
	}
}

// TestUnfollowedLinks verifies invariant #10: Unfollowed links.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// With the pattern ".git/", the directory linker holds only the link ref,
// which points to a project that holds a .git directory. Deleting linker
// completes and records no pattern. linker is gone, its trash item holds ref
// as a link, and the project keeps its contents.
func TestUnfollowedLinks(t *testing.T) {
	home := isolateHome(t)
	writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = [\".git/\"]\n", filepath.Join(home, "trash")))

	project := filepath.Join(home, "proj")
	linker := filepath.Join(home, "linker")
	writeFile(t, filepath.Join(project, ".git", "config"), "git data")

	if err := os.Mkdir(linker, 0o700); err != nil {
		t.Fatalf("💣 fixture directory: %v", err)
	}

	if err := os.Symlink(project, filepath.Join(linker, "ref")); err != nil {
		t.Fatalf("💣 fixture link: %v", err)
	}

	action := performAction(t, Request{Command: Delete, Source: linker, Reason: "remove links"})
	if action.Status != store.Completed || action.ReviewRule != "" {
		t.Errorf("✗ completed record: %+v", action)
	}

	assertAbsent(t, linker)

	if linkTarget, err := os.Readlink(filepath.Join(home, "trash", action.TrashID, "ref")); err != nil || linkTarget != project {
		t.Errorf("✗ trashed link changed: %q, %v", linkTarget, err)
	}

	assertFile(t, filepath.Join(project, ".git", "config"), "git data")

	if !t.Failed() {
		t.Log("✓ a link inside a deleted directory stays a link")
	}
}

// TestPreparationStatus verifies invariant #11: Approval status.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Delete and move requests have no status before execution. A matching
// rule leaves the same request pending and idle before any filesystem transfer.
func TestPreparationStatus(t *testing.T) {
	for _, command := range []string{Delete, Move} {
		home := configuredHome(t, 30)
		source := filepath.Join(home, "source")
		writeFile(t, source, "contents")

		settings, err := config.Load(home)
		if err != nil {
			t.Fatalf("💣 configuration: %v", err)
		}

		request := Request{Command: command, Source: source, Destination: filepath.Join(home, "destination"), Reason: "remove obsolete file"}

		action, _, err := prepareAction(&request, home, settings)
		if err != nil || action.Status != 0 || action.InProgress || action.ReviewRule != "" {
			t.Errorf("✗ ordinary %s waits for approval: %+v, %v", command, action, err)
		}

		settings.RequireReview = []string{source}

		action, _, err = prepareAction(&request, home, settings)
		if err != nil || action.Status != store.Pending || action.InProgress || action.ReviewRule != source {
			t.Errorf("✗ protected %s does not await approval: %+v, %v", command, action, err)
		}

		assertFile(t, source, "contents")
	}

	if !t.Failed() {
		t.Log("✓ pending status means approval is required")
	}
}

// TestReviewNotification verifies invariant #12: Request notifications.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Creating a request requiring approval sends its ID to the listening notifier
// after saving history, without transferring the source file.
func TestReviewNotification(t *testing.T) {
	home := configuredHome(t, 30)
	source := filepath.Join(home, "source")
	writeFile(t, source, "contents")
	writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = [%q]\n", filepath.Join(home, "trash"), source))

	listener := notificationReceiver(t, home)

	result, err := Run(t.Context(), &Request{Command: Delete, Source: source, Reason: "remove"})
	if err != nil || len(result.Actions) != 1 {
		t.Fatalf("💣 create request: %+v, %v", result, err)
	}

	if err = listener.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("💣 receiver deadline: %v", err)
	}

	connection, err := listener.Accept()
	if err != nil {
		t.Errorf("✗ pending request was not pushed: %v", err)

		return
	}
	defer connection.Close()

	var message struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	if err = json.NewDecoder(connection).Decode(&message); err != nil || message.ID != result.Actions[0].ID || message.Token != "test-token" {
		t.Errorf("✗ incorrect pushed request: %+v, %v", message, err)
	}

	viewed, err := Run(t.Context(), &Request{Command: View, ID: strconv.FormatInt(message.ID, 10)})
	if err != nil || len(viewed.Actions) != 1 || viewed.Actions[0].Status != store.Pending {
		t.Errorf("✗ pushed request not persisted: %+v, %v", viewed, err)
	}

	assertFile(t, source, "contents")

	if !t.Failed() {
		t.Log("✓ pending request pushed after persistence")
	}
}

// TestMalformedRule verifies invariant #13: Malformed rules.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// With "*.pem" listed before "[abc", a delete of notes.txt and a delete of
// a.pem each return ErrRuleMalformed naming "[abc" together with one saved
// pending action, which records "[abc" for notes.txt and "*.pem" for a.pem.
// Both files stay in place. A replacing move returns the same error with its
// move and its occupying delete, both saved pending. A replacing restore
// returns it with the pending delete of its occupant.
func TestMalformedRule(t *testing.T) {
	home := configuredHome(t, 30)
	notes, keyFile := filepath.Join(home, "notes.txt"), filepath.Join(home, "a.pem")
	original := filepath.Join(home, "original")

	writeFile(t, notes, "notes")
	writeFile(t, keyFile, "key")
	writeFile(t, original, "original")

	deleted := performAction(t, Request{Command: Delete, Source: original, Reason: "retain original"})
	writeFile(t, original, "occupant")
	writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = [\"*.pem\", \"[abc\"]\n", filepath.Join(home, "trash")))

	for _, testCase := range []struct {
		request       Request
		recordedRules []string
	}{
		{Request{Command: Delete, Source: notes, Reason: "remove"}, []string{"[abc"}},
		{Request{Command: Delete, Source: keyFile, Reason: "remove"}, []string{"*.pem"}},
		{Request{Command: Move, Source: notes, Destination: keyFile, Reason: "replace", Overwrite: true, Force: true}, []string{"[abc", "*.pem"}},
		{Request{Command: Restore, ID: strconv.FormatInt(deleted.ID, 10), Overwrite: true, Force: true}, []string{"[abc"}},
	} {
		result, err := Run(t.Context(), &testCase.request)
		if err == nil || !errors.Is(err, errs.ErrRuleMalformed) || !strings.Contains(err.Error(), strconv.Quote("[abc")) {
			t.Errorf("✗ %s error: %v", testCase.request.Command, err)
		}

		if len(result.Actions) != len(testCase.recordedRules) {
			t.Errorf("✗ %s actions: %+v", testCase.request.Command, result)

			continue
		}

		for index, action := range result.Actions {
			if action.Status != store.Pending || action.ReviewRule != testCase.recordedRules[index] {
				t.Errorf("✗ %s pending record: %+v", testCase.request.Command, action)
			}
		}

		assertRecorded(t, result.Actions...)
	}

	assertFile(t, notes, "notes")
	assertFile(t, keyFile, "key")
	assertFile(t, original, "occupant")

	if !t.Failed() {
		t.Log("✓ a malformed rule holds every request and is reported with it")
	}
}

// Test class: Core: Helper.
func assertOccupantType(t *testing.T, path, target, occupantType string) {
	t.Helper()

	if occupantType == "directory" {
		assertFile(t, filepath.Join(path, "nested", "file"), "occupying contents")

		return
	}

	linkTarget, err := os.Readlink(path)
	if err != nil || linkTarget != target {
		t.Errorf("✗ symbolic link changed: %q, %v", linkTarget, err)
	}

	if occupantType == "symlink" {
		assertFile(t, target, "link target contents")
	} else {
		assertAbsent(t, target)
	}
}

// pendingPair creates isolated linked pending records and their actual files.
// Test class: Core: Helper.
func pendingPair(t *testing.T, protected bool) (string, store.Action, store.Action) {
	t.Helper()
	home := configuredHome(t, 30)
	source, destination := filepath.Join(home, "moving", "source"), filepath.Join(home, "occupying", "destination")
	writeFile(t, source, "moving contents")
	writeFile(t, destination, "occupying contents")

	moved := store.Action{Type: Move, Status: store.Pending, Source: source, Destination: destination, Reason: "replace", Operation: Move, Failure: "earlier diagnostic"}
	if protected {
		moved.ReviewRule = source
	}

	deleted := store.Action{Type: Delete, Status: store.Pending, Source: destination, TrashPath: filepath.Join(home, "trash"), TrashID: "occupant", Reason: "replace", Operation: Delete, ReviewRule: destination}

	history, err := store.Open(t.Context(), filepath.Join(home, config.Directory, config.Database))
	if err != nil {
		t.Fatalf("💣 open linked history: %v", err)
	}
	defer history.Close()

	if err = history.CreatePair(t.Context(), &moved, &deleted); err != nil {
		t.Fatalf("💣 save linked fixture: %v", err)
	}

	return home, moved, deleted
}

// assertRecorded checks complete persisted actions against their earlier states.
// Test class: Core: Helper.
func assertRecorded(t *testing.T, actions ...store.Action) {
	t.Helper()

	for _, action := range actions {
		recorded := performAction(t, Request{Command: View, ID: strconv.FormatInt(action.ID, 10)})
		if !reflect.DeepEqual(recorded, action) {
			t.Errorf("✗ persisted record differs: %+v; expected %+v", recorded, action)
		}
	}
}

// assertUniqueResults checks that each returned ID occurs once in its latest stored state.
// Test class: Core: Helper.
func assertUniqueResults(t *testing.T, result Result) {
	t.Helper()

	actionIDs := make(map[int64]bool)
	for _, action := range result.Actions {
		if actionIDs[action.ID] {
			t.Errorf("✗ repeated result for action %d", action.ID)
		}

		actionIDs[action.ID] = true
		assertRecorded(t, action)
	}
}

// notificationReceiver publishes an isolated notifier endpoint in place of SafeRM Notify.
// Test class: Core: Helper.
func notificationReceiver(t *testing.T, home string) *net.TCPListener {
	t.Helper()

	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("💣 notification receiver: %v", err)
	}

	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Errorf("✗ close notification receiver: %v", err)
		}
	})

	endpoint, err := json.Marshal(map[string]string{"address": listener.Addr().String(), "token": "test-token"})
	if err != nil {
		t.Fatalf("💣 notification address: %v", err)
	}

	writeFile(t, filepath.Join(home, config.Directory, "notifications.json"), string(endpoint))

	return listener
}

// assertActionNotifications checks the exact pending IDs and rejects extra messages.
// Test class: Core: Helper.
func assertActionNotifications(t *testing.T, listener *net.TCPListener, actionIDs []int64) {
	t.Helper()

	for _, actionID := range actionIDs {
		if err := listener.SetDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("💣 notification deadline: %v", err)
		}

		connection, err := listener.Accept()
		if err != nil {
			t.Errorf("✗ pending action %d was not notified: %v", actionID, err)

			continue
		}

		if err = connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("💣 notification read deadline: %v", err)
		}

		var message struct {
			ID    int64  `json:"id"`
			Token string `json:"token"`
		}
		if err = json.NewDecoder(connection).Decode(&message); err != nil || message.ID != actionID || message.Token != "test-token" {
			t.Errorf("✗ incorrect pending notification: %+v, %v", message, err)
		}

		if err = connection.Close(); err != nil {
			t.Errorf("✗ close received notification: %v", err)
		}

		recorded := performAction(t, Request{Command: View, ID: strconv.FormatInt(actionID, 10)})
		if recorded.Status != store.Pending {
			t.Errorf("✗ notification preceded a pending saved record: %+v", recorded)
		}
	}

	if err := listener.SetDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("💣 extra notification deadline: %v", err)
	}

	connection, err := listener.Accept()
	if err == nil {
		if closeErr := connection.Close(); closeErr != nil {
			t.Errorf("✗ close unexpected notification: %v", closeErr)
		}

		t.Error("✗ unexpected pending notification")
	} else {
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Errorf("✗ checking notifications failed: %v", err)
		}
	}
}
