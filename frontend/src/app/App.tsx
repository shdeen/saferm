import { useEffect, useState } from 'react';
import { Badge, Button, Group, Text, Title } from '@mantine/core';
import { Plus } from 'lucide-react';
import { Sidebar } from './Sidebar';
import { Help } from './Help';
import { onReview, takeReview, showError } from '../bridge/desktop';
import { runOperation } from '../bridge/operations';
import type { Action } from '../bridge/operations';
import { OperationList } from '../operations/OperationList';
import { OperationDetailsDrawer } from '../operations/OperationDetailsDrawer';
import { CleanupHistoryDialog } from '../operations/dialogs/CleanupHistoryDialog';
import { ConfirmationDialog } from '../operations/dialogs/ConfirmationDialog';
import { NewOperationDialog } from '../operations/dialogs/NewOperationDialog';
import { PurgeOperationsDialog } from '../operations/dialogs/PurgeOperationsDialog';
import { RestoreOperationDialog } from '../operations/dialogs/RestoreOperationDialog';
import { Configuration } from '../configuration/Configuration';
import { canPurge } from '../operations/operationStatus';
import { useOperationHistory } from '../operations/useOperationHistory';
import { useOperationRequest } from '../operations/useOperationRequest';
import { appstrings, formatText } from '../shared/text';

type OperationDialog =
  | { kind: 'create' | 'cleanup' }
  | { kind: 'restore'; action: Action }
  | { kind: 'purge'; actions: Action[] };

/**
 * Composes navigation, operation history, dialogs, and configuration. Opens the
 * details of an action when a launch of the application asks for its review.
 */
export function App() {
  const [section, setSection] = useState('operations');
  const [dialog, setDialog] = useState<OperationDialog>();
  const [viewedAction, setViewedAction] = useState<Action>();
  const [selectedIDs, setSelectedIDs] = useState<number[]>([]);
  const operation = useOperationRequest();
  const historyState = useOperationHistory(operation.revision);
  useEffect(function receiveReview() {
    let active = true;
    let requestNumber = 0;
    /**
     * Takes the action that a launch asked to review and opens its details. Discards
     * the response when a later review request has started or the component has unmounted.
     */
    async function openRequestedOperation() {
      const currentRequest = ++requestNumber;
      try {
        const actionID = await takeReview();
        if (!actionID) return;
        const response = await runOperation({ Command: 'view', ID: String(actionID) });
        if (!active || currentRequest !== requestNumber) return;
        if (response.error) {
          await showError(response.error);
        } else if (response.result.actions?.[0]) {
          setSection('operations');
          setViewedAction(response.result.actions[0]);
        }
      } catch (error) {
        if (active) await showError(String(error));
      }
    }
    function requestedReview() {
      void openRequestedOperation();
    }
    const unsubscribe = onReview(requestedReview);
    requestedReview();
    return function stopReview() {
      active = false;
      unsubscribe();
    };
  }, []);

  const selectedActions = (historyState.history?.actions ?? []).filter(
    function selectedAction(action) {
      return selectedIDs.includes(action.id) && canPurge(action, historyState.statuses);
    },
  );

  function closeDialog() {
    if (!operation.busy) setDialog(undefined);
  }
  function restore(action: Action) {
    setDialog({ kind: 'restore', action });
  }
  function purge(action: Action) {
    setDialog({ kind: 'purge', actions: [action] });
  }
  function decide(action: Action, decision: string) {
    void operation.perform({ Command: 'dispose', ID: String(action.id), Decision: decision });
  }
  function select(actionID: number, selected: boolean) {
    setSelectedIDs(function updateSelection(previousIDs) {
      return selected
        ? [...previousIDs, actionID]
        : previousIDs.filter(function differentID(id) {
            return id !== actionID;
          });
    });
  }
  async function confirm() {
    if (
      operation.confirmation &&
      (await operation.perform({ ...operation.confirmation.request, Confirmed: true }))
    )
      setDialog(undefined);
  }

  return (
    <div className="app-layout">
      <Sidebar
        section={section}
        onNavigate={setSection}
        onCleanup={function cleanup() {
          setDialog({ kind: 'cleanup' });
        }}
        busy={operation.busy}
      />
      <main className="main-content">
        {section === 'operations' ? (
          <>
            <header className="page-header">
              <div>
                <Group gap="sm">
                  <Title order={1}>{appstrings.operations}</Title>
                  {Boolean(historyState.history?.pending) && (
                    <Badge color="yellow" variant="light">
                      {formatText(appstrings.pendingCount, {
                        count: historyState.history?.pending ?? 0,
                      })}
                    </Badge>
                  )}
                </Group>
                <Text c="dimmed" size="sm" mt={8}>
                  {appstrings.operationsDescription}
                </Text>
              </div>
              <Button
                leftSection={<Plus size={16} />}
                onClick={function create() {
                  setDialog({ kind: 'create' });
                }}
                disabled={operation.busy}
              >
                {appstrings.newOperation}
              </Button>
            </header>
            <OperationList
              historyState={historyState}
              busy={operation.busy}
              selectedIDs={selectedActions.map(function selectedID(action) {
                return action.id;
              })}
              onSelect={select}
              onView={setViewedAction}
              onDecision={decide}
              onRestore={restore}
              onPurge={function purgeSelected() {
                setDialog({ kind: 'purge', actions: selectedActions });
              }}
            />
          </>
        ) : section === 'settings' ? (
          <Configuration
            revision={operation.revision}
            busy={operation.busy}
            perform={operation.perform}
          />
        ) : (
          <Help />
        )}
      </main>
      {viewedAction && (
        <OperationDetailsDrawer
          action={viewedAction}
          revision={operation.revision}
          statuses={historyState.statuses}
          busy={operation.busy}
          onClose={function closeDetails() {
            setViewedAction(undefined);
          }}
          onRestore={restore}
          onPurge={purge}
          onDecision={decide}
        />
      )}
      {dialog?.kind === 'create' && (
        <NewOperationDialog
          busy={operation.busy}
          onClose={closeDialog}
          perform={operation.perform}
        />
      )}
      {dialog?.kind === 'restore' && (
        <RestoreOperationDialog
          action={dialog.action}
          busy={operation.busy}
          onClose={closeDialog}
          perform={operation.perform}
        />
      )}
      {dialog?.kind === 'purge' && (
        <PurgeOperationsDialog
          actions={dialog.actions}
          busy={operation.busy}
          onClose={closeDialog}
          perform={operation.perform}
        />
      )}
      {dialog?.kind === 'cleanup' && (
        <CleanupHistoryDialog
          busy={operation.busy}
          onClose={closeDialog}
          perform={operation.perform}
        />
      )}
      {operation.confirmation && (
        <ConfirmationDialog
          confirmation={operation.confirmation.details}
          busy={operation.busy}
          onClose={operation.cancelConfirmation}
          onConfirm={confirm}
        />
      )}
    </div>
  );
}
