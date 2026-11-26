import { Badge } from '@mantine/core';
import { statusName, statusLabel } from './operationStatus';
import { appstrings } from '../shared/text';
import type { Action } from '../bridge/operations';

const statusColors: Record<string, string> = {
  pending: 'yellow',
  completed: 'blue',
  denied: 'gray',
  restored: 'teal',
  purged: 'gray',
  failed: 'red',
};

/** Shows the status of an action, or the in-progress label while an operation holds the action. */
export function OperationStatusBadge({
  action,
  statuses,
}: {
  action: Action;
  statuses: Record<string, number | undefined>;
}) {
  const statusKey = statusName(action.status, statuses);
  return (
    <Badge
      variant="light"
      color={action.inProgress ? 'orange' : statusColors[statusKey]}
      className="status-badge"
    >
      {action.inProgress ? appstrings.inProgress : statusLabel(action.status, statuses)}
    </Badge>
  );
}
