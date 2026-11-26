import { useState } from 'react';
import { Button, Checkbox, Group, Modal, Stack, Text } from '@mantine/core';
import { appstrings } from '../../shared/text';
import type { OperationDialogProps } from './OperationDialogProps';

/** Submits a history cleanup with the chosen orphan option and closes after success. */
export function CleanupHistoryDialog({ busy, perform, onClose }: OperationDialogProps) {
  const [orphans, setOrphans] = useState(false);
  async function cleanup() {
    if (await perform({ Command: 'cleanup', Orphans: orphans })) onClose();
  }
  return (
    <Modal
      opened
      onClose={onClose}
      title={appstrings.maintenance}
      closeOnClickOutside={!busy}
      closeOnEscape={!busy}
      withCloseButton={!busy}
    >
      <Stack>
        <Text size="sm">{appstrings.cleanupDescription}</Text>
        <Checkbox
          label={appstrings.cleanupOrphans}
          description={appstrings.cleanupOrphansDescription}
          checked={orphans}
          onChange={function changeOrphans(event) {
            setOrphans(event.currentTarget.checked);
          }}
          disabled={busy}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose} disabled={busy}>
            {appstrings.cancel}
          </Button>
          <Button onClick={cleanup} loading={busy}>
            {appstrings.cleanupSubmit}
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
