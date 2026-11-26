package saferm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/filesystem"
	"github.com/shdeen/saferm/internal/notify"
	"github.com/shdeen/saferm/internal/review"
	"github.com/shdeen/saferm/internal/store"
)

// createAction carries out a delete or move request. When a review rule makes
// the request need approval, it records a pending action, pushes the action's
// ID to SafeRM Notify, and transfers nothing. The result then carries a warning
// for each directory that the review check could not read, and the saved
// action comes with ErrRuleMalformed when the rules hold a malformed pattern.
// Move replacement first preserves its occupant through a linked delete.
// Ordinary unreviewed actions transfer before their outcome is recorded.
func createAction(ctx context.Context, request *Request, home string, settings config.Settings, history *store.History) (Result, error) {
	action, warnings, err := prepareAction(request, home, settings)
	if err != nil {
		return Result{}, err
	}

	if request.Command == Move && request.Overwrite {
		occupied, inspectErr := filesystem.Exists(action.Destination)
		if inspectErr != nil {
			return Result{}, inspectErr
		}

		if occupied {
			result, replaceErr := replaceMove(ctx, request, home, settings, history, &action)
			if action.ID == 0 {
				return result, replaceErr
			}

			// The move was saved, so its warnings and a malformed rule are reported.
			result.Problems = append(warnings, result.Problems...)

			return result, errors.Join(replaceErr, review.Validate(settings.RequireReview))
		}
	}

	if action.ReviewRule != "" {
		if err := history.Create(ctx, &action); err != nil {
			return Result{}, err
		}

		notifyPending(ctx, home, action.ID)

		return Result{Actions: []store.Action{action}, Problems: warnings}, review.Validate(settings.RequireReview)
	}

	return runRecordedAction(ctx, home, settings, history, &action)
}

// replaceMove prepares an ordinary occupying delete and asks for replacement
// consent before any changes. An unreviewed delete transfers before the linked
// pair is saved; the moving source stays untouched until that save succeeds.
func replaceMove(ctx context.Context, request *Request, home string, settings config.Settings, history *store.History, moved *store.Action) (Result, error) {
	deleteRequest := Request{Command: Delete, Source: moved.Destination, Reason: moved.Reason, Requester: moved.Requester, Description: moved.Description}

	deleted, warnings, err := prepareAction(&deleteRequest, home, settings)
	if err != nil {
		return Result{}, err
	}

	if err := checkReplacementTrash(moved.Source, home, settings); err != nil {
		return Result{}, err
	}

	if !request.Force && !request.Confirmed {
		return Result{Confirmation: &Confirmation{Prompt: appstrings.Text("prompt.moveOverwrite"), Path: moved.Destination}}, errs.ErrConfirmation
	}

	moved.Status = store.Pending

	if deleted.ReviewRule == "" {
		err = transferItem(ctx, home, settings, &deleted)

		status := store.Completed
		if err != nil {
			status = store.Failed
		}

		deleted.Resolve(status, err)
	}

	result, err := finishReplacement(ctx, home, settings, history, moved, &deleted, err)
	if deleted.ID != 0 {
		// The occupying delete was saved, so its warnings belong in the result.
		result.Problems = append(result.Problems, warnings...)
	}

	return result, err
}

// checkReplacementTrash refuses a replacement that would move its own trash
// directory or any link used to reach it. Invalid link chains fail before the
// path inspection, which accepts directories that have not been created yet.
func checkReplacementTrash(sourcePath, home string, settings config.Settings) error {
	trashPath, err := settings.Trash(home)
	if err != nil {
		return err
	}

	if _, err := filepath.EvalSymlinks(trashPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%q: %w: %w", trashPath, errs.ErrPathResolve, err)
	}

	return checkTrashPath(sourcePath, trashPath, make(map[string]bool))
}

// checkTrashPath checks each parent and symbolic-link target against the source.
// The paths are absolute; checkedPaths avoids repeating shared parents or links.
// No path is created, and a missing final directory is permitted.
func checkTrashPath(sourcePath, trashPath string, checkedPaths map[string]bool) error {
	if checkedPaths[trashPath] {
		return nil
	}

	checkedPaths[trashPath] = true

	resolvedPath, err := filesystem.Resolve(trashPath, "")
	if err != nil {
		return err
	}

	if filesystem.Contains(sourcePath, resolvedPath) {
		return fmt.Errorf("%q → %q: %w", sourcePath, trashPath, errs.ErrFileOverlap)
	}

	if parentPath := filepath.Dir(trashPath); parentPath != trashPath {
		if err := checkTrashPath(sourcePath, parentPath, checkedPaths); err != nil {
			return err
		}
	}

	pathInfo, err := os.Lstat(resolvedPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("%q: %w: %w", resolvedPath, errs.ErrPathResolve, err)
	}

	if pathInfo.Mode()&os.ModeSymlink == 0 {
		return nil
	}

	linkTarget, err := os.Readlink(resolvedPath)
	if err != nil {
		return fmt.Errorf("%q: %w: %w", resolvedPath, errs.ErrPathResolve, err)
	}

	if !filepath.IsAbs(linkTarget) {
		linkTarget = filepath.Join(filepath.Dir(resolvedPath), linkTarget)
	}

	return checkTrashPath(sourcePath, linkTarget, checkedPaths)
}

// finishReplacement saves the linked pair and applies its occupying delete's
// outcome. Notifications refer only to committed actions still pending. The
// result keeps move-first order and preserves recovery data after save failure.
func finishReplacement(ctx context.Context, home string, settings config.Settings, history *store.History, moved, deleted *store.Action, operationErr error) (Result, error) {
	recordContext := ctx
	if deleted.Status != store.Pending {
		recordContext = context.WithoutCancel(ctx)
	}

	if recordErr := history.CreatePair(recordContext, moved, deleted); recordErr != nil {
		if deleted.Status == store.Completed {
			recordErr = fmt.Errorf("%q: %w: %w", deleted.Source, errs.ErrDatabaseResult, recordErr)
		}

		var result Result
		if deleted.Status != store.Pending {
			result.Actions = []store.Action{*deleted}
		}

		return result, errors.Join(operationErr, recordErr)
	}

	result := Result{Actions: []store.Action{*moved, *deleted}}
	if deleted.Status == store.Pending {
		notifyPending(ctx, home, deleted.ID)
	} else {
		dependents, dependentErr := applyBlockerOutcome(ctx, home, settings, history, deleted)
		result.mergeActions(dependents.Actions)

		operationErr = errors.Join(operationErr, dependentErr)
	}

	if result.Actions[0].Status == store.Pending {
		notifyPending(ctx, home, moved.ID)
	}

	return result, operationErr
}

// notifyPending delivers a saved pending action's ID to the optional SafeRM Notify,
// reporting delivery failure without changing the action's outcome.
func notifyPending(ctx context.Context, home string, actionID int64) {
	if err := notify.Send(context.WithoutCancel(ctx), home, actionID); err != nil {
		slog.Warn(appstrings.Text("notification.delivery"), "operation", actionID, "error", err)
	}
}

// prepareAction resolves and checks the paths of a delete or move request and
// returns the action to record, marked pending when a review rule makes the
// request need approval. It also returns a warning for each directory inside
// the source that the review check could not read. It changes no files. It
// returns ErrFileOverlap when the source is the history database or contains
// it, and an error when the parent of a move's destination is not an existing
// directory.
func prepareAction(request *Request, home string, settings config.Settings) (store.Action, []string, error) {
	action := store.Action{Type: request.Command, Reason: request.Reason, Requester: request.Requester, Description: request.Description, Operation: request.Command}

	source, err := filesystem.Resolve(request.Source, home)
	if err != nil {
		return action, nil, err
	}

	action.Source = source

	destination, err := actionDestination(request, home, settings, &action)
	if err != nil {
		return action, nil, err
	}

	sourceInfo, err := filesystem.CheckTransfer(source, destination, request.Command == Move && request.Overwrite)
	if err != nil {
		return action, nil, err
	}

	databasePath, err := filesystem.Resolve(filepath.Join(home, config.Directory, config.Database), home)
	if err != nil {
		return action, nil, err
	}

	if filesystem.Contains(source, databasePath) {
		return action, nil, fmt.Errorf("%q: %w", source, errs.ErrFileOverlap)
	}

	if request.Command == Move {
		parentInfo, inspectErr := os.Stat(filepath.Dir(destination))
		if inspectErr != nil {
			return action, nil, fmt.Errorf("%q: %w: %w", destination, errs.ErrFileInspect, inspectErr)
		}

		if !parentInfo.IsDir() {
			return action, nil, fmt.Errorf("%q: %w", destination, errs.ErrInputInvalid)
		}
	}

	reviewRule, unreadableDirectories := review.Match(settings.RequireReview, source, sourceInfo.IsDir(), home)
	if reviewRule != "" {
		action.ReviewRule, action.Status = reviewRule, store.Pending
	}

	var warnings []string
	for _, directory := range unreadableDirectories {
		warnings = append(warnings, fmt.Sprintf(appstrings.Text("warning.unreadable"), directory))
	}

	return action, warnings, nil
}

// actionDestination returns the path that would receive the request's source:
// the resolved destination for a move, or a newly named item in the trash
// directory for a delete. It stores the move's destination, or the delete's
// trash directory and item name, in the action.
func actionDestination(request *Request, home string, settings config.Settings, action *store.Action) (string, error) {
	if request.Command == Move {
		destination, err := filesystem.Resolve(request.Destination, home)
		action.Destination = destination

		return destination, err
	}

	trashPath, err := settings.Trash(home)
	if err != nil {
		return "", err
	}

	action.TrashPath = trashPath

	action.TrashID = uuid.NewString()

	return filesystem.Resolve(filepath.Join(trashPath, action.TrashID), home)
}

// runRecordedAction transfers the action's source to the path that
// storedDestination returns, creating the trash directory first for a delete,
// and records the outcome through finishAction.
func runRecordedAction(ctx context.Context, home string, settings config.Settings, history *store.History, action *store.Action) (Result, error) {
	err := transferItem(ctx, home, settings, action)

	return finishAction(ctx, history, action, store.Completed, err)
}

// transferItem transfers an ordinary delete or move without recording its
// outcome. A delete creates its trash directory before transferring the item.
func transferItem(ctx context.Context, home string, settings config.Settings, action *store.Action) error {
	destination, err := storedDestination(action, home, settings)
	if err == nil && action.Type == Delete {
		err = os.MkdirAll(filepath.Dir(destination), filesystem.PrivateDirectoryMode)
	}

	if err == nil {
		err = filesystem.Transfer(ctx, action.Source, destination)
	}

	if err != nil {
		err = fmt.Errorf("%q: %w: %w", action.Source, errs.ErrFileTransfer, err)
	}

	return err
}

// storedDestination returns the path that receives the action's item: the
// recorded destination for a move, or the action's trash item inside the
// currently configured trash directory for a delete. It returns
// ErrActionAmbiguous when the recorded name of the trash item is not a plain
// file name.
func storedDestination(action *store.Action, home string, settings config.Settings) (string, error) {
	if action.Type == Move {
		return action.Destination, nil
	}

	trashPath, err := settings.Trash(home)
	if err != nil {
		return "", err
	}

	if !filepath.IsLocal(action.TrashID) || filepath.Base(action.TrashID) != action.TrashID {
		return "", fmt.Errorf("%d: %w", action.ID, errs.ErrActionAmbiguous)
	}

	return filepath.Join(trashPath, action.TrashID), nil
}

// decide applies a pending action's decision and propagates its outcome to
// dependents. A move whose delete is pending first needs combined consent.
// Resubmitted requests use the records just read by Run.
func decide(ctx context.Context, request *Request, home string, settings config.Settings, history *store.History, action *store.Action) (Result, error) {
	if action.Status != store.Pending {
		return Result{}, fmt.Errorf("%d: %w", action.ID, errs.ErrActionResolved)
	}

	if action.BlockerID != 0 {
		blocker, err := history.Get(ctx, action.BlockerID)
		if err != nil && !errors.Is(err, errs.ErrActionMissing) {
			return Result{}, err
		}

		if err == nil && blocker.Status == store.Pending {
			return decideBlocked(ctx, request, home, settings, history, action, &blocker)
		}
	}

	result, decisionErr := decideAction(ctx, request.Decision, home, settings, history, action)
	dependents, dependentErr := applyBlockerOutcome(ctx, home, settings, history, action)
	result.mergeActions(dependents.Actions)

	return result, errors.Join(decisionErr, dependentErr)
}

// decideBlocked asks before deciding a pending occupying delete. Combined
// approval also permits the named move's source review, but never executes a
// move that propagation already attempted. Errors retain completed results.
func decideBlocked(ctx context.Context, request *Request, home string, settings config.Settings, history *store.History, moved, deleted *store.Action) (Result, error) {
	if !request.Confirmed {
		promptKey := "prompt.blockedApprove"
		if request.Decision == store.DenyDecision {
			promptKey = "prompt.blockedDeny"
		}

		return Result{Actions: []store.Action{*moved, *deleted}, Confirmation: &Confirmation{
			Prompt: appstrings.Text(promptKey), Path: deleted.Source, MoveID: moved.ID, BlockerID: deleted.ID,
		}}, errs.ErrConfirmation
	}

	result, decisionErr := decide(ctx, request, home, settings, history, deleted)
	if deleted.Status != store.Completed || request.Decision != store.ApproveDecision {
		return result, decisionErr
	}

	for index := range result.Actions {
		if result.Actions[index].ID == moved.ID {
			return result, decisionErr
		}
	}

	currentMove, err := history.Get(ctx, moved.ID)
	if err != nil {
		return result, errors.Join(decisionErr, err)
	}

	moveResult, err := decide(ctx, request, home, settings, history, &currentMove)
	result.mergeActions(moveResult.Actions)

	return result, errors.Join(decisionErr, err)
}

// decideAction preflights and claims one pending action before applying an
// explicit approval or denial. A preliminary refusal leaves it pending.
func decideAction(ctx context.Context, decision, home string, settings config.Settings, history *store.History, action *store.Action) (Result, error) {
	if action.InProgress {
		return Result{Actions: []store.Action{*action}}, fmt.Errorf("%d: %w", action.ID, errs.ErrActionBusy)
	}

	if decision == store.ApproveDecision {
		destination, err := storedDestination(action, home, settings)
		if err != nil {
			return Result{Actions: []store.Action{*action}}, err
		}

		if _, err = filesystem.CheckTransfer(action.Source, destination, false); err != nil {
			return Result{Actions: []store.Action{*action}}, err
		}
	}

	if err := history.Claim(ctx, action, action.Type, decision); err != nil {
		currentAction, readErr := history.Get(ctx, action.ID)
		if readErr == nil {
			*action = currentAction
		}

		return Result{Actions: []store.Action{*action}}, errors.Join(err, readErr)
	}

	if decision == store.DenyDecision {
		return finishAction(ctx, history, action, store.Denied, nil)
	}

	return runRecordedAction(ctx, home, settings, history, action)
}

// applyBlockerOutcome resolves pending dependents after completion, denial or
// failure. Source review still requires its own approval. Each dependent is
// visited once; a busy or failed decision does not prevent other decisions.
func applyBlockerOutcome(ctx context.Context, home string, settings config.Settings, history *store.History, blocker *store.Action) (Result, error) {
	var (
		result    Result
		resultErr error
	)

	visitedActionIDs := make(map[int64]bool)

	actionOutcomes := []store.Action{*blocker}
	for index := 0; index < len(actionOutcomes); index++ {
		blockerOutcome := actionOutcomes[index]
		if blockerOutcome.Status != store.Completed && blockerOutcome.Status != store.Denied && blockerOutcome.Status != store.Failed {
			continue
		}

		outcomeContext := ctx
		if blockerOutcome.Status == store.Denied || blockerOutcome.Status == store.Failed {
			// Denial is final bookkeeping after a decided blocker, not new filesystem work.
			outcomeContext = context.WithoutCancel(ctx)
		}

		dependents, err := history.PendingDependents(outcomeContext, blockerOutcome.ID)
		resultErr = errors.Join(resultErr, err)

		for dependentIndex := range dependents {
			dependent := &dependents[dependentIndex]
			if visitedActionIDs[dependent.ID] || (blockerOutcome.Status == store.Completed && dependent.ReviewRule != "") {
				continue
			}

			visitedActionIDs[dependent.ID] = true

			var dependentResult Result
			if blockerOutcome.Status == store.Completed {
				dependentResult, err = decideAction(ctx, store.ApproveDecision, home, settings, history, dependent)
			} else {
				dependentResult, err = denyBlocked(outcomeContext, history, dependent, &blockerOutcome)
			}

			result.mergeActions(dependentResult.Actions)

			resultErr = errors.Join(resultErr, err)

			actionOutcomes = append(actionOutcomes, dependentResult.Actions...)
		}
	}

	return result, resultErr
}

// denyBlocked records a dependent denial with its blocker diagnostic. The
// diagnostic is history, not an operation error that changes Denied to Failed.
func denyBlocked(ctx context.Context, history *store.History, action, blocker *store.Action) (Result, error) {
	if err := history.Claim(ctx, action, action.Type, store.DenyDecision); err != nil {
		currentAction, readErr := history.Get(ctx, action.ID)
		if readErr == nil {
			*action = currentAction
		}

		return Result{Actions: []store.Action{*action}}, errors.Join(err, readErr)
	}

	diagnosticKey, blockerError := "diagnostic.blockerDenied", errs.ErrBlockerDenied
	if blocker.Status == store.Failed {
		diagnosticKey, blockerError = "diagnostic.blockerFailed", errs.ErrBlockerFailed
	}

	diagnostic := fmt.Errorf(appstrings.Text(diagnosticKey), blocker.ID, blockerError) //nolint:err113 // The catalog's %w wraps a static blocker sentinel.
	err := history.Finish(context.WithoutCancel(ctx), action, store.Denied, diagnostic)

	return Result{Actions: []store.Action{*action}}, err
}

// restore transfers the item of a completed or failed action back to its source
// path and records the action as restored. A confirmed replacement first
// deletes the occupant recoverably and records that delete. The returned
// actions put the occupying delete before the original action.
//
// When the source path is occupied, restore returns the first of the following
// that applies:
//   - ErrFileExists, unless the request allows overwriting
//   - ErrActionAmbiguous, for a failed action
//   - ErrConfirmation with a Confirmation, unless the request is forced or confirmed
//   - the error that creating the occupying delete reports, with that delete,
//     as happens while a review rule is malformed
//   - ErrRestorePending with the occupying delete, when that delete needs approval
//
// A failed occupying delete or its recording leaves the original unclaimed.
// Later failures retain the completed delete in the partial result.
func restore(ctx context.Context, request *Request, home string, settings config.Settings, history *store.History, action *store.Action) (Result, error) {
	destination, occupied, err := checkRestore(request, home, settings, action)
	if err != nil {
		return Result{}, err
	}

	if occupied && !request.Force && !request.Confirmed {
		return Result{Actions: []store.Action{*action}, Confirmation: &Confirmation{Prompt: appstrings.Text("prompt.restoreOverwrite"), Path: action.Source}}, errs.ErrConfirmation
	}

	var result Result

	if occupied {
		if err := checkReplacementTrash(destination, home, settings); err != nil {
			return Result{}, err
		}

		deleteRequest := Request{Command: Delete, Source: action.Source, Reason: fmt.Sprintf(appstrings.Text("reason.restoreOverwrite"), action.ID)}

		result, err = createAction(ctx, &deleteRequest, home, settings, history)
		if err != nil {
			return result, err
		}

		if result.Actions[0].Status == store.Pending {
			return result, fmt.Errorf(appstrings.Text("error.restorePendingDetail"), result.Actions[0].ID, errs.ErrRestorePending) //nolint:err113 // The catalog's %w wraps the static sentinel; copy remains externalized.
		}
	}

	if err := history.Claim(ctx, action, Restore, action.Decision); err != nil {
		return result, err
	}

	err = filesystem.Transfer(ctx, destination, action.Source)
	restored, err := finishAction(ctx, history, action, store.Restored, err)
	result.Actions = append(result.Actions, restored.Actions...)

	return result, err
}

// checkRestore validates the original action and its current paths without
// changing either. It returns the stored item's location and whether the
// original path is occupied. Failed actions cannot replace an occupant.
func checkRestore(request *Request, home string, settings config.Settings, action *store.Action) (destination string, occupied bool, resultErr error) {
	if action.Status != store.Completed && action.Status != store.Failed {
		return "", false, fmt.Errorf("%d: %w", action.ID, errs.ErrActionResolved)
	}

	destination, err := storedDestination(action, home, settings)
	if err != nil {
		return "", false, err
	}

	if _, err = filesystem.CheckTransfer(destination, action.Source, request.Overwrite); err != nil {
		return "", false, err
	}

	occupied, err = filesystem.Exists(action.Source)
	if err != nil {
		return "", false, err
	}

	if action.Status == store.Failed && occupied {
		return "", false, fmt.Errorf("%d: %w", action.ID, errs.ErrActionAmbiguous)
	}

	return destination, occupied, nil
}

// finishAction records the action with the supplied status, or as failed when
// the operation returned an error, and returns the action with that error
// joined to any recording error. It records even when the context is canceled.
// When the operation succeeded and the status is not denied, a recording error
// is wrapped in ErrDatabaseResult.
func finishAction(ctx context.Context, history *store.History, action *store.Action, status int, operationErr error) (Result, error) {
	if operationErr != nil {
		status = store.Failed
	}
	// Final recording must survive cancellation after a filesystem operation.
	recordErr := history.Finish(context.WithoutCancel(ctx), action, status, operationErr)
	if recordErr != nil && operationErr == nil && status != store.Denied {
		recordErr = fmt.Errorf("%q: %w: %w", action.Source, errs.ErrDatabaseResult, recordErr)
	}

	return Result{Actions: []store.Action{*action}}, errors.Join(operationErr, recordErr)
}
