import { Button, Group, Modal, Stack, Text } from '@mantine/core';
import { appstrings } from '../../shared/text';
import type { Confirmation } from '../../bridge/operations';

/**
 * Shows a confirmation prompt from the service and waits for the user to confirm or
 * cancel. It substitutes the IDs and literal path in the question template.
 */
export function ConfirmationDialog({
  confirmation,
  busy,
  onConfirm,
  onClose,
}: {
  confirmation: Confirmation;
  busy: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const values = {
    Path: confirmation.path,
    MoveID: String(confirmation.moveID ?? 0),
    BlockerID: String(confirmation.blockerID ?? 0),
  };
  const promptText = confirmation.prompt.replace(
    /\{\{\.(Path|MoveID|BlockerID)\}\}/g,
    function substituteValue(_match, key: keyof typeof values) {
      return values[key];
    },
  );
  return (
    <Modal
      opened
      onClose={onClose}
      title={appstrings.confirmationTitle}
      zIndex={400}
      closeOnClickOutside={!busy}
      closeOnEscape={!busy}
      withCloseButton={!busy}
    >
      <Stack>
        <Text size="sm" className="wrap-anywhere">
          {promptText}
        </Text>
        <Text size="sm" className="path-block">
          {confirmation.path}
        </Text>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose} disabled={busy} data-autofocus>
            {appstrings.cancel}
          </Button>
          <Button color="red" onClick={onConfirm} loading={busy}>
            {appstrings.confirm}
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
