import { appstrings } from '../shared/text';
import type { Action } from '../bridge/operations';

/** Returns the name of a status code, or the code as text when it has no name. */
export function statusName(status: number, statuses: Record<string, number | undefined>): string {
  return (
    Object.keys(statuses).find(function matchingStatus(statusKey) {
      return statuses[statusKey] === status;
    }) ?? String(status)
  );
}

/** Returns the display label of a status code, or its status name when it has no label. */
export function statusLabel(status: number, statuses: Record<string, number | undefined>): string {
  const statusKey = statusName(status, statuses);
  return appstrings.status[statusKey as keyof typeof appstrings.status] ?? statusKey;
}

/** Reports whether the action can be restored: it is completed or failed, and not in progress. */
export function canRestore(action: Action, statuses: Record<string, number | undefined>) {
  return (
    !action.inProgress &&
    (action.status === statuses.completed || action.status === statuses.failed)
  );
}

/** Reports whether the action's item can be permanently deleted: it is a restorable delete. */
export function canPurge(action: Action, statuses: Record<string, number | undefined>) {
  return action.type === 'delete' && canRestore(action, statuses);
}
