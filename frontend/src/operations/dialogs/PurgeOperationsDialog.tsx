import { Button, Group, Modal, Stack, Text } from '@mantine/core';
import type { Action } from '../../bridge/operations';
import { appstrings } from '../../shared/text';
import type { OperationDialogProps } from './OperationDialogProps';

/**
 * Submits the selected actions for permanent deletion and closes when the request
 * finishes, whether or not every item was deleted.
 */
export function PurgeOperationsDialog({
  actions,
  busy,
  perform,
  onClose,
}: OperationDialogProps & { actions: Action[] }) {
  async function purge() {
    await perform({
      Command: 'purge',
      IDs: actions.map(function actionID(action) {
        return String(action.id);
      }),
    });
    onClose();
  }
  return (
    <Modal
      opened
      onClose={onClose}
      title={appstrings.purgeTitle}
      closeOnClickOutside={!busy}
      closeOnEscape={!busy}
      withCloseButton={!busy}
    >
      <Stack>
        <Text size="sm">{appstrings.purgeDescription}</Text>
        <div className="purge-paths">
          {actions.map(function purgePath(action) {
            return (
              <Text key={action.id} size="sm" className="path-block">
                #{action.id} {action.source}
              </Text>
            );
          })}
        </div>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose} disabled={busy}>
            {appstrings.cancel}
          </Button>
          <Button color="red" onClick={purge} loading={busy}>
            {appstrings.purge}
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
