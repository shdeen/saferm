package saferm

import (
	"context"
	"fmt"
	"time"

	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/filesystem"
	"github.com/shdeen/saferm/internal/store"
)

// purge permanently removes the trash item of each requested action through
// purgeAction. It records each failure as a problem in the result, continues
// with the remaining actions, and returns ErrBatchPartial when any failed.
func purge(ctx context.Context, request *Request, home string, settings config.Settings, history *store.History) (Result, error) {
	result := Result{Actions: make([]store.Action, 0, len(request.IDs))}
	for _, idText := range request.IDs {
		actionID, err := parseID(idText)
		if err != nil {
			return result, err
		}

		action, err := history.Get(ctx, actionID)
		if err == nil {
			var removal Result

			removal, err = purgeAction(ctx, home, settings, history, &action)
			result.Actions = append(result.Actions, removal.Actions...)
		}

		if err != nil {
			result.Problems = append(result.Problems, err.Error())
		}
	}

	if len(result.Problems) > 0 {
		return result, errs.ErrBatchPartial
	}

	return result, nil
}

// purgeAction permanently removes the trash item of a completed or failed
// delete and records the action as purged. It returns ErrActionBusy for an
// action in progress, ErrActionResolved for any other type or status, and
// ErrFileMissing when the trash item does not exist.
func purgeAction(ctx context.Context, home string, settings config.Settings, history *store.History, action *store.Action) (Result, error) {
	if action.InProgress {
		return Result{}, fmt.Errorf("%d: %w", action.ID, errs.ErrActionBusy)
	}

	if action.Type != Delete || (action.Status != store.Completed && action.Status != store.Failed) {
		return Result{}, fmt.Errorf("%d: %w", action.ID, errs.ErrActionResolved)
	}

	trashItem, err := storedDestination(action, home, settings)
	if err != nil {
		return Result{}, err
	}

	exists, err := filesystem.Exists(trashItem)
	if err != nil {
		return Result{}, err
	}

	if !exists {
		return Result{}, fmt.Errorf("%d, %q: %w", action.ID, trashItem, errs.ErrFileMissing)
	}

	if err := history.Claim(ctx, action, Purge, action.Decision); err != nil {
		return Result{}, err
	}

	err = filesystem.Remove(ctx, trashItem)

	return finishAction(ctx, history, action, store.Purged, err)
}

// cleanup deletes the history record of every action that cleanupEligible
// accepts and returns the IDs of the deleted records. It removes no files. It
// records each failure as a problem in the result and returns ErrBatchPartial
// when any occurred.
func cleanup(ctx context.Context, request *Request, home string, settings config.Settings, history *store.History) (Result, error) {
	days, err := settings.Retention()
	if err != nil {
		return Result{}, err
	}

	actions, err := history.List(ctx, "", 0)
	if err != nil {
		return Result{}, err
	}

	result := Result{Removed: make([]int64, 0)}

	for actionIndex := range actions {
		action := &actions[actionIndex]

		eligible, inspectErr := cleanupEligible(action, home, settings, days, request.Orphans)
		if eligible && inspectErr == nil {
			inspectErr = history.Delete(ctx, action)
			if inspectErr == nil {
				result.Removed = append(result.Removed, action.ID)
			}
		}

		if inspectErr != nil {
			result.Problems = append(result.Problems, inspectErr.Error())
		}
	}

	if len(result.Problems) > 0 {
		return result, errs.ErrBatchPartial
	}

	return result, nil
}

// cleanupEligible reports whether cleanup may delete the action's record, given
// the retention period in days and whether orphaned records are included:
//   - a pending action is never eligible
//   - a restored, purged, or denied action is eligible once the retention period has passed since it was resolved
//   - a completed or failed action is decided by inspectRecovery
//
// It returns ErrActionBusy for an action in progress.
func cleanupEligible(action *store.Action, home string, settings config.Settings, days int, orphans bool) (bool, error) {
	if action.InProgress {
		return false, fmt.Errorf("%d: %w", action.ID, errs.ErrActionBusy)
	}

	if action.Status == store.Pending {
		return false, nil
	}

	expired := !action.ResolvedAt.IsZero() && time.Since(action.ResolvedAt)/retentionDay >= time.Duration(days)
	if action.Status == store.Restored || action.Status == store.Purged || action.Status == store.Denied {
		return expired, nil
	}

	return inspectRecovery(action, home, settings, expired, orphans)
}

// inspectRecovery reports whether cleanup may delete the record of a completed
// or failed action. The answer depends on whether the retention period has
// expired, whether orphaned records are included, and whether an item exists
// at the path that storedDestination returns:
//   - a completed action whose item is missing is eligible when orphaned records are included
//   - any other completed move is eligible once expired, and any other completed delete is not eligible
//   - a failed action whose item exists is not eligible
//   - any other failed action is eligible once expired, or when orphaned records are included and its source is also missing
func inspectRecovery(action *store.Action, home string, settings config.Settings, expired, orphans bool) (bool, error) {
	destination, err := storedDestination(action, home, settings)
	if err != nil {
		return false, err
	}

	exists, err := filesystem.Exists(destination)
	if err != nil {
		return false, err
	}

	if action.Status == store.Completed {
		if !exists && orphans {
			return true, nil
		}

		return action.Type == Move && expired, nil
	}

	if exists {
		return false, nil
	}

	sourceExists, err := filesystem.Exists(action.Source)
	if err != nil {
		return false, err
	}

	return expired || (orphans && !sourceExists), nil
}

// retentionDay is the length of one day of the retention period.
const retentionDay = 24 * time.Hour
