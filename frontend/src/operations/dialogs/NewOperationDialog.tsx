import { useState, type FormEvent } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  Group,
  Modal,
  SegmentedControl,
  Stack,
  Text,
  Textarea,
  TextInput,
} from '@mantine/core';
import { FolderOpen } from 'lucide-react';
import { chooseDestinationPath, chooseSourcePath } from '../../bridge/operations';
import { appstrings } from '../../shared/text';
import type { OperationDialogProps } from './OperationDialogProps';

/** Collects a delete or move request and closes after a successful submission. */
export function NewOperationDialog({ busy, onClose, perform }: OperationDialogProps) {
  const [operationType, setOperationType] = useState('delete');
  const [source, setSource] = useState('');
  const [destination, setDestination] = useState('');
  const [overwrite, setOverwrite] = useState(false);
  const [reason, setReason] = useState('');
  const [requester, setRequester] = useState('');
  const [description, setDescription] = useState('');
  const [pathError, setPathError] = useState('');
  const [choosing, setChoosing] = useState(false);

  async function browse(destinationField: boolean) {
    setChoosing(true);
    setPathError('');
    try {
      const selectedPath = await (destinationField
        ? chooseDestinationPath(source)
        : chooseSourcePath());
      if (selectedPath) {
        if (destinationField) setDestination(selectedPath);
        else setSource(selectedPath);
      }
    } catch (error) {
      setPathError(String(error));
    } finally {
      setChoosing(false);
    }
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (
      await perform({
        Command: operationType,
        Source: source,
        Destination: operationType === 'move' ? destination : '',
        Overwrite: operationType === 'move' && overwrite,
        Reason: reason,
        Requester: requester,
        Description: description,
      })
    )
      onClose();
  }

  return (
    <Modal
      opened
      onClose={onClose}
      title={appstrings.newOperation}
      size="lg"
      closeOnClickOutside={!busy}
      closeOnEscape={!busy}
      withCloseButton={!busy}
    >
      <form onSubmit={submit}>
        <Stack gap="md">
          <SegmentedControl
            value={operationType}
            onChange={setOperationType}
            disabled={busy}
            data={[
              { value: 'delete', label: appstrings.type.delete },
              { value: 'move', label: appstrings.type.move },
            ]}
          />
          <Text size="sm" c="dimmed">
            {operationType === 'delete' ? appstrings.deleteDescription : appstrings.moveDescription}
          </Text>
          {pathError && <Alert color="red">{pathError}</Alert>}
          <Group align="flex-end" wrap="nowrap">
            <TextInput
              className="grow"
              label={appstrings.source}
              placeholder={appstrings.sourcePlaceholder}
              value={source}
              onChange={function sourceChanged(event) {
                setSource(event.currentTarget.value);
              }}
              required
              disabled={busy}
            />
            <Button
              variant="default"
              aria-label={`${appstrings.choose} ${appstrings.source}`}
              onClick={function browseSource() {
                void browse(false);
              }}
              disabled={busy || choosing}
            >
              <FolderOpen size={16} />
            </Button>
          </Group>
          {operationType === 'move' && (
            <Group align="flex-end" wrap="nowrap">
              <TextInput
                className="grow"
                label={appstrings.destination}
                placeholder={appstrings.destinationPlaceholder}
                value={destination}
                onChange={function destinationChanged(event) {
                  setDestination(event.currentTarget.value);
                }}
                required
                disabled={busy}
              />
              <Button
                variant="default"
                aria-label={`${appstrings.choose} ${appstrings.destination}`}
                onClick={function browseDestination() {
                  void browse(true);
                }}
                disabled={busy || choosing || !source.trim()}
              >
                <FolderOpen size={16} />
              </Button>
            </Group>
          )}
          {operationType === 'move' && (
            <Checkbox
              label={appstrings.moveOverwrite}
              description={appstrings.overwriteDescription}
              checked={overwrite}
              onChange={function overwriteChanged(event) {
                setOverwrite(event.currentTarget.checked);
              }}
              disabled={busy}
            />
          )}
          <Textarea
            label={appstrings.reason}
            placeholder={appstrings.reasonPlaceholder}
            required
            value={reason}
            onChange={function reasonChanged(event) {
              setReason(event.currentTarget.value);
            }}
            minRows={2}
            disabled={busy}
          />
          <TextInput
            label={appstrings.requester}
            placeholder={appstrings.optional}
            value={requester}
            onChange={function requesterChanged(event) {
              setRequester(event.currentTarget.value);
            }}
            disabled={busy}
          />
          <Textarea
            label={appstrings.description}
            placeholder={appstrings.optional}
            value={description}
            onChange={function descriptionChanged(event) {
              setDescription(event.currentTarget.value);
            }}
            disabled={busy}
          />
          <Group justify="flex-end" mt="sm">
            <Button variant="default" onClick={onClose} disabled={busy}>
              {appstrings.cancel}
            </Button>
            <Button
              type="submit"
              loading={busy}
              disabled={
                !source.trim() ||
                !reason.trim() ||
                (operationType === 'move' && !destination.trim())
              }
            >
              {operationType === 'delete' ? appstrings.deleteSubmit : appstrings.moveSubmit}
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
