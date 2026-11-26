import { useEffect, useState } from 'react';
import { Alert, Button, Divider, Drawer, Group, Loader, Stack, Text } from '@mantine/core';
import { runOperation, type Action } from '../bridge/operations';
import { OperationStatusBadge } from './OperationStatusBadge';
import { canPurge, canRestore } from './operationStatus';
import { appstrings, formatDate, formatText } from '../shared/text';

type OperationDetailsDrawerProps = {
  /** The action to show. The drawer reloads it by ID. */
  action: Action;
  /** Counter of finished operations. The action reloads when it changes. */
  revision: number;
  /** Status codes keyed by status name. */
  statuses: Record<string, number | undefined>;
  busy: boolean;
  onClose: () => void;
  onRestore: (action: Action) => void;
  onPurge: (action: Action) => void;
  onDecision: (action: Action, decision: string) => void;
};

/**
 * Loads the current record of an action and shows its details, with the controls that
 * its status permits.
 */
export function OperationDetailsDrawer(props: OperationDetailsDrawerProps) {
  const [action, setAction] = useState(props.action);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  useEffect(
    function loadDetails() {
      let active = true;
      setLoading(true);
      async function load() {
        try {
          const response = await runOperation({ Command: 'view', ID: String(props.action.id) });
          if (!active) return;
          setError(response.error);
          if (response.result.actions?.[0]) setAction(response.result.actions[0]);
        } catch (error) {
          if (active) setError(String(error));
        } finally {
          if (active) setLoading(false);
        }
      }
      void load();
      return function cancelLoad() {
        active = false;
      };
    },
    [props.action.id, props.revision],
  );

  const disabled = props.busy || loading || Boolean(error);
  return (
    <Drawer
      opened
      onClose={props.onClose}
      position="right"
      size={500}
      title={formatText(appstrings.operationDetails, { id: props.action.id })}
    >
      <Stack gap="lg">
        {loading && <Loader size="sm" />}
        {error && <Alert color="red">{error}</Alert>}
        <Group justify="space-between">
          <OperationStatusBadge action={action} statuses={props.statuses} />
          <Text size="sm" c="dimmed">
            {appstrings.type[action.type as keyof typeof appstrings.type] ?? action.type}
          </Text>
        </Group>
        <Text fw={600}>{action.reason}</Text>
        {action.inProgress && <Alert color="orange">{appstrings.busyDescription}</Alert>}
        <Group gap="sm">
          {action.status === props.statuses.pending && !action.inProgress && (
            <>
              <Button
                disabled={disabled}
                onClick={function approve() {
                  props.onDecision(action, 'approve');
                }}
              >
                {appstrings.approve}
              </Button>
              <Button
                variant="default"
                disabled={disabled}
                onClick={function deny() {
                  props.onDecision(action, 'deny');
                }}
              >
                {appstrings.deny}
              </Button>
            </>
          )}
          {canRestore(action, props.statuses) && (
            <Button
              disabled={disabled}
              onClick={function restore() {
                props.onRestore(action);
              }}
            >
              {appstrings.restore}
            </Button>
          )}
          {canPurge(action, props.statuses) && (
            <Button
              color="red"
              variant="light"
              disabled={disabled}
              onClick={function purge() {
                props.onPurge(action);
              }}
            >
              {appstrings.purge}
            </Button>
          )}
        </Group>
        <Divider />
        <ActionFields action={action} />
        {action.failure && (
          <Alert color="red" title={appstrings.failure}>
            <Text size="sm" className="recorded-errors">
              {action.failure}
            </Text>
          </Alert>
        )}
      </Stack>
    </Drawer>
  );
}

/** Lists the recorded fields of an action, leaving out the empty ones. */
function ActionFields({ action }: { action: Action }) {
  const fields: [string, string | undefined][] = [
    [appstrings.source, action.source],
    [appstrings.destination, action.destination],
    [appstrings.trashPath, action.trashPath],
    [appstrings.trashID, action.trashID],
    [appstrings.requester, action.requester],
    [appstrings.description, action.description],
    [appstrings.reviewRule, action.reviewRule],
    [appstrings.blocker, action.blockerID ? String(action.blockerID) : undefined],
    [appstrings.decision, action.decision],
    [appstrings.lastOperation, action.operation],
    [appstrings.created, formatDate(action.createdAt)],
    [appstrings.updated, formatDate(action.updatedAt)],
    [appstrings.resolved, action.resolvedAt ? formatDate(action.resolvedAt) : undefined],
  ];
  return (
    <dl className="detail-fields">
      {fields
        .filter(function presentField([, value]) {
          return Boolean(value);
        })
        .map(function detailField([label, value]) {
          return (
            <div key={label}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          );
        })}
    </dl>
  );
}
