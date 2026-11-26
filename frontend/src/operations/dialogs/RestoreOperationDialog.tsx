import { useState } from 'react';
import { Button, Checkbox, Group, Modal, Stack, Text } from '@mantine/core';
import type { Action } from '../../bridge/operations';
import { appstrings } from '../../shared/text';
import type { OperationDialogProps } from './OperationDialogProps';

/** Submits a restore request with the chosen overwrite permission and closes after success. */
export function RestoreOperationDialog({
  action,
  busy,
  perform,
  onClose,
}: OperationDialogProps & { action: Action }) {
  const [overwrite, setOverwrite] = useState(false);
  async function restore() {
    if (await perform({ Command: 'restore', ID: String(action.id), Overwrite: overwrite }))
      onClose();
  }
  return (
    <Modal
      opened
      onClose={onClose}
      title={appstrings.restoreTitle}
      closeOnClickOutside={!busy}
      closeOnEscape={!busy}
      withCloseButton={!busy}
    >
      <Stack>
        <Text size="sm">{appstrings.restoreDescription}</Text>
        <Text size="sm" className="path-block">
          {action.source}
        </Text>
        <Checkbox
          label={appstrings.overwrite}
          description={appstrings.overwriteDescription}
          checked={overwrite}
          onChange={function allowOverwrite(event) {
            setOverwrite(event.currentTarget.checked);
          }}
          disabled={busy}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose} disabled={busy}>
            {appstrings.cancel}
          </Button>
          <Button onClick={restore} loading={busy}>
            {appstrings.restore}
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
