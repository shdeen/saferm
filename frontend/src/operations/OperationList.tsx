import {
  ActionIcon,
  Alert,
  Button,
  Checkbox,
  Group,
  Loader,
  Pagination,
  Select,
  Table,
  Text,
  Tooltip,
} from '@mantine/core';
import {
  ArrowRight,
  ArrowUpDown,
  ArchiveRestore,
  ChevronRight,
  FolderOpen,
  RefreshCw,
  Trash2,
} from 'lucide-react';
import type { Action } from '../bridge/operations';
import { OperationStatusBadge } from './OperationStatusBadge';
import { canPurge, canRestore } from './operationStatus';
import { appstrings, fileName, formatDate, formatText } from '../shared/text';
import type { useOperationHistory } from './useOperationHistory';

type OperationListProps = {
  historyState: ReturnType<typeof useOperationHistory>;
  busy: boolean;
  /** IDs of the actions selected for permanent deletion. */
  selectedIDs: number[];
  onSelect: (actionID: number, selected: boolean) => void;
  onView: (action: Action) => void;
  onDecision: (action: Action, decision: string) => void;
  onRestore: (action: Action) => void;
  /** Requests permanent deletion of the selected actions. */
  onPurge: () => void;
};

/**
 * Shows the operation history with its filters and pagination, the selection for
 * permanent deletion, and the controls that each action's status permits.
 */
export function OperationList(props: OperationListProps) {
  const { historyState, selectedIDs } = props;
  const { history, error, loading } = historyState;
  return (
    <section className="operations-panel">
      <OperationFilters historyState={historyState} />
      {error && (
        <Alert color="red" title={appstrings.refreshFailed} m="lg">
          {error}
        </Alert>
      )}
      {loading ? (
        <div className="empty-state" role="status">
          <Loader size="sm" />
          <Text>{appstrings.loading}</Text>
        </div>
      ) : !history?.actions?.length ? (
        <EmptyHistory filtered={Boolean(historyState.statusFilter || historyState.typeFilter)} />
      ) : (
        <Table.ScrollContainer type="native" minWidth={720} className="operation-scroll">
          <Table
            verticalSpacing="md"
            horizontalSpacing="lg"
            className="operation-table"
            stickyHeader
          >
            <Table.Thead>
              <Table.Tr>
                <Table.Th className="selection-column" />
                <Table.Th>{appstrings.file}</Table.Th>
                <Table.Th>{appstrings.statusLabel}</Table.Th>
                <Table.Th>{appstrings.created}</Table.Th>
                <Table.Th className="action-column">{appstrings.actions}</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {history.actions.map(function renderAction(action) {
                return <OperationRow key={action.id} action={action} {...props} />;
              })}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}
      <footer className="history-footer">
        <Text size="sm" c="dimmed">
          {formatText(appstrings.range, {
            start: history?.total ? (history.page - 1) * history.pageSize + 1 : 0,
            end: history ? Math.min(history.page * history.pageSize, history.total) : 0,
            total: history?.total ?? 0,
          })}
        </Text>
        {selectedIDs.length > 0 && (
          <Button
            color="red"
            variant="light"
            size="xs"
            disabled={props.busy}
            onClick={props.onPurge}
          >
            {formatText(appstrings.purgeSelected, { count: selectedIDs.length })}
          </Button>
        )}
        <Group gap="md">
          <Select
            aria-label={appstrings.pageSize}
            className="page-size"
            data={['20', '50', '100']}
            value={String(historyState.pageSize)}
            onChange={function resize(size) {
              historyState.resizePage(Number(size));
            }}
            allowDeselect={false}
          />
          <Pagination
            total={Math.max(1, Math.ceil((history?.total ?? 0) / historyState.pageSize))}
            value={history?.page ?? 1}
            onChange={historyState.setPageNumber}
            size="sm"
          />
        </Group>
      </footer>
    </section>
  );
}

/** Shows the status and type filters, the operation count, and the refresh control. */
function OperationFilters({ historyState }: Pick<OperationListProps, 'historyState'>) {
  const { history, statuses } = historyState;
  const statusOptions = Object.keys(statuses).map(function statusOption(status) {
    return {
      value: status,
      label: appstrings.status[status as keyof typeof appstrings.status] ?? status,
    };
  });
  return (
    <div className="list-toolbar">
      <Group gap="sm">
        <Select
          aria-label={appstrings.statusFilter}
          value={historyState.statusFilter}
          data={[{ value: '', label: appstrings.allStatuses }, ...statusOptions]}
          onChange={function selectStatus(status) {
            historyState.filterStatus(status ?? '');
          }}
          allowDeselect={false}
        />
        <Select
          aria-label={appstrings.typeFilter}
          value={historyState.typeFilter}
          data={[
            { value: '', label: appstrings.allTypes },
            ...Object.entries(appstrings.type).map(function typeOption([value, label]) {
              return { value, label };
            }),
          ]}
          onChange={function selectType(actionType) {
            historyState.filterType(actionType ?? '');
          }}
          allowDeselect={false}
        />
      </Group>
      <Group gap="md">
        <Text className="sort-caption" size="xs" c="dimmed">
          <ArrowUpDown size={13} />
          {appstrings.pendingFirst}
        </Text>
        <Text size="xs" c="dimmed">
          {formatText(appstrings.operationCount, { count: history?.total ?? 0 })}
        </Text>
        <Tooltip label={appstrings.refresh}>
          <ActionIcon
            aria-label={appstrings.refresh}
            variant="subtle"
            color="gray"
            onClick={function refresh() {
              void historyState.refresh();
            }}
          >
            <RefreshCw size={16} />
          </ActionIcon>
        </Tooltip>
      </Group>
    </div>
  );
}

/** Shows one action with the controls that its status permits. */
function OperationRow({ action, ...props }: OperationListProps & { action: Action }) {
  const statuses = props.historyState.statuses;
  const pending = action.status === statuses.pending && !action.inProgress;
  return (
    <Table.Tr className={pending ? 'pending-operation' : undefined}>
      <Table.Td>
        {canPurge(action, statuses) && (
          <Checkbox
            size="xs"
            aria-label={formatText(appstrings.selectOperation, { id: action.id })}
            checked={props.selectedIDs.includes(action.id)}
            disabled={props.busy}
            onChange={function toggleSelection(event) {
              props.onSelect(action.id, event.currentTarget.checked);
            }}
          />
        )}
      </Table.Td>
      <Table.Td>
        <div className="file-summary">
          <span className={`operation-icon ${action.type}`}>
            {action.type === 'delete' ? <Trash2 size={18} /> : <ArrowRight size={18} />}
          </span>
          <div className="file-description">
            <button
              className="file-link"
              title={action.source}
              onClick={function openDetails() {
                props.onView(action);
              }}
            >
              {fileName(action.source)}
              <span className="operation-id">#{action.id}</span>
            </button>
            <Text size="xs" className="file-path" title={action.source}>
              {action.source}
            </Text>
            <Text size="sm" className="operation-reason">
              {action.reason}
            </Text>
            {action.requester && (
              <Text size="xs" c="dimmed">
                {action.requester}
              </Text>
            )}
          </div>
        </div>
      </Table.Td>
      <Table.Td>
        <OperationStatusBadge action={action} statuses={statuses} />
        <Text size="xs" c="dimmed" mt={5}>
          {appstrings.type[action.type as keyof typeof appstrings.type] ?? action.type}
        </Text>
      </Table.Td>
      <Table.Td>
        <Text size="xs" c="dimmed" className="operation-date">
          {formatDate(action.createdAt)}
        </Text>
      </Table.Td>
      <Table.Td>
        <Group gap={6} justify="flex-end" wrap="nowrap">
          {pending && (
            <>
              <Button
                size="xs"
                variant="default"
                disabled={props.busy}
                onClick={function deny() {
                  props.onDecision(action, 'deny');
                }}
              >
                {appstrings.deny}
              </Button>
              <Button
                size="xs"
                disabled={props.busy}
                onClick={function approve() {
                  props.onDecision(action, 'approve');
                }}
              >
                {appstrings.approve}
              </Button>
            </>
          )}
          {canRestore(action, statuses) && (
            <Button
              size="xs"
              variant="light"
              leftSection={<ArchiveRestore size={14} />}
              disabled={props.busy}
              onClick={function restore() {
                props.onRestore(action);
              }}
            >
              {appstrings.restore}
            </Button>
          )}
          <ActionIcon
            variant="subtle"
            color="gray"
            aria-label={`${appstrings.details} #${action.id}`}
            onClick={function view() {
              props.onView(action);
            }}
          >
            <ChevronRight size={16} />
          </ActionIcon>
        </Group>
      </Table.Td>
    </Table.Tr>
  );
}

/** Shows the message for an empty history, worded for whether a filter is applied. */
function EmptyHistory({ filtered }: { filtered: boolean }) {
  return (
    <div className="empty-state">
      <div className="empty-icon">
        <FolderOpen size={32} strokeWidth={1.4} />
      </div>
      <Text fw={600} size="lg">
        {filtered ? appstrings.filteredEmptyTitle : appstrings.emptyTitle}
      </Text>
      <Text c="dimmed" size="sm" maw={420} ta="center">
        {filtered ? appstrings.filteredEmptyDescription : appstrings.emptyDescription}
      </Text>
    </div>
  );
}
