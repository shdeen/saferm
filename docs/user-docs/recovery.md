# Recovery and history

## Restore an item

`saferm restore <id>`, or **Restore** in the desktop app, returns the item of a completed or failed delete or move to its original path. A deleted item is taken from the trash directory configured now, not the one recorded with the action. A moved item is taken from its recorded destination. A restored action can't be restored again.

An action marked in progress can't be restored; see [Interrupted or ambiguous operations](troubleshooting.md#interrupted-or-ambiguous-operations).

### When something occupies the original path

Restoring onto an occupied path takes `--overwrite` in the CLI, or **Allow replacement at the original path** in the desktop app. SafeRM deletes the occupant to trash as a new action, with the reason `Make room to restore action #<id>.`, and restores the original item. The result lists the new delete before the restored action.

The CLI asks before replacing, with no as the default answer. `-f` (`--force`) skips that question, and has no effect without `--overwrite`. The desktop app always asks.

If the occupant's delete needs approval under a review rule, it is pending and the restore stops. Approve that delete, then run the restore again.

Only a completed action can replace an occupant. A failed action can't, even with `--overwrite`.

### Restore after a replacement

A move that replaced an item has two actions: the move, and the occupant's delete, which `saferm view` shows as the move's blocker. To undo both, restore the move first, which frees its destination, then restore the delete:

```sh
saferm restore <move-id>
saferm restore <delete-id>
```

This order assumes the move completed. If it is pending, denied, or failed, check where both items actually are before restoring either one.

## Action statuses

| Status | Code | Meaning |
| --- | --- | --- |
| `completed` | 1 | The delete or move was carried out. |
| `pending` | 2 | Waiting for approval. Nothing has moved. |
| `denied` | 3 | The request was denied. Nothing moved. |
| `restored` | 4 | The item was returned to its original path. |
| `purged` | 5 | The trashed item was permanently erased. |
| `failed` | 6 | The latest operation on the action failed. |

`failed` describes the latest operation, which may be a restore rather than the original delete or move. A failed action can often still be restored; its details show the recorded errors.

Separately from its status, an action can be marked in progress. A marked action can't be decided, restored, or purged.

IDs are never reused. `saferm list` shows actions oldest first. The desktop app shows pending actions first, then the newest.

## Purge trash

`saferm purge <id>...` permanently erases the trashed items of completed or failed deletes. The history records stay. The CLI doesn't ask for confirmation. **Permanently delete** in the desktop app does the same, after confirmation. Moves can't be purged, because their items are not in trash.

SafeRM looks for each item in the trash directory configured now. It handles each ID separately: if one fails, for example because its item is missing, the rest are still purged, and the command exits with status 1.

## Clean up history

`saferm cleanup`, or **History cleanup** in the desktop app, removes records from history. It never removes files. A record becomes removable `retain-period` days after its action was last resolved, subject to the table below. `--orphans`, or **Include orphaned records** in the app, also removes records whose items are missing.

### Cleanup eligibility

For a delete, the item means the trashed item in the current trash directory. For a move, it means the item at the recorded destination.

| Record | Removed by cleanup |
| --- | --- |
| Pending | Never. |
| In progress | Never. Reported as a problem. |
| Restored, purged, or denied | After the retention period. |
| Completed delete, item exists | Never. |
| Completed delete, item missing | Only with `--orphans`. |
| Completed move, item exists | After the retention period. |
| Completed move, item missing | After the retention period, or at once with `--orphans`. |
| Failed, item exists | Never. |
| Failed, item missing | After the retention period, or at once with `--orphans` if the source is also missing. |

A completed delete whose item is still in trash is kept no matter how old it is. To clear its record, purge the item, then wait out the retention period.

A completed move's record is removed after the retention period, even though the item is still at its destination. After that, SafeRM can't reverse the move.

Before you use `--orphans`, check that `trash-path` points where your trash actually is. Items left in an earlier trash directory look missing.

Cleanup reports the IDs it removed and any records it couldn't process, and exits with status 1 if any failed.

Revised 2026-11-25
