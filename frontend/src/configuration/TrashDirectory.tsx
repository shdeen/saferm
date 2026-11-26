import { useEffect, useRef, useState } from 'react';
import { Button, TextInput } from '@mantine/core';
import { chooseDirectory, directoryExists } from '../bridge/operations';
import { appstrings } from '../shared/text';

/** Milliseconds that the valid label stays on display. */
const validLabelDuration = 5000;

/**
 * Edits the trash directory. A typed path is checked once focus leaves the field and
 * is reported only when the directory exists. A path chosen in the native dialog is
 * reported without a check. An initial path that does not exist is marked invalid.
 */
export function TrashDirectory({
  initialPath,
  disabled,
  onChange,
  onError,
  onEdit,
  onValidation,
}: {
  initialPath: string;
  disabled: boolean;
  /** Receives each accepted path. */
  onChange: (path: string) => void;
  onError: (error: string) => void;
  /** Called on every typed edit, before the typed path is checked. */
  onEdit: () => void;
  /** Receives a promise that settles when a check or a directory choice finishes. */
  onValidation: (validation: Promise<void>) => void;
}) {
  const [directoryPath, setDirectoryPath] = useState(initialPath);
  const [validation, setValidation] = useState<'valid' | 'invalid' | ''>('');
  const editedSinceFocus = useRef(false);
  const validationRequest = useRef(0);
  const mounted = useRef(true);
  const focused = useRef(false);

  useEffect(
    function checkInitialDirectory() {
      mounted.current = true;
      const requestNumber = validationRequest.current;
      directoryExists(initialPath)
        .then(function showInitialValidation(exists) {
          if (mounted.current && requestNumber === validationRequest.current && !exists)
            setValidation('invalid');
        })
        .catch(function showValidationError(error: unknown) {
          if (mounted.current) onError(String(error));
        });
      return function cancelValidation() {
        mounted.current = false;
      };
    },
    [initialPath, onError],
  );

  useEffect(
    function expireValidLabel() {
      if (validation !== 'valid') return;
      const labelTimer = window.setTimeout(function clearValidLabel() {
        setValidation('');
      }, validLabelDuration);
      return function cancelLabelTimer() {
        window.clearTimeout(labelTimer);
      };
    },
    [validation],
  );

  /**
   * Checks the typed path and reports it when the directory exists. Discards the
   * result when the path was edited again during the check.
   */
  async function validateManualEdit() {
    if (!editedSinceFocus.current) return;
    editedSinceFocus.current = false;
    const requestNumber = ++validationRequest.current;
    try {
      const exists = await directoryExists(directoryPath);
      if (requestNumber !== validationRequest.current) return;
      if (mounted.current && !focused.current) setValidation(exists ? 'valid' : 'invalid');
      if (focused.current) editedSinceFocus.current = true;
      if (exists) onChange(directoryPath);
    } catch (error) {
      onError(String(error));
    }
  }

  /**
   * Prompts for a directory in the native dialog and reports the chosen path. When
   * the dialog is canceled, it checks the typed path instead.
   */
  async function selectDirectory() {
    ++validationRequest.current;
    try {
      const selectedPath = await chooseDirectory(directoryPath);
      if (!selectedPath) {
        await validateManualEdit();
        return;
      }
      editedSinceFocus.current = false;
      setDirectoryPath(selectedPath);
      setValidation('');
      onChange(selectedPath);
    } catch (error) {
      onError(String(error));
    }
  }

  return (
    <div>
      <div className="settings-label">
        <label htmlFor="trash-directory">{appstrings.configuredTrashPath}</label>
        <span
          id="trash-directory-validation"
          role="status"
          className={`settings-validation ${validation}`}
        >
          {validation === 'valid'
            ? appstrings.validDirectory
            : validation === 'invalid'
              ? appstrings.invalidDirectory
              : ''}
        </span>
      </div>
      <div className="configuration-path">
        <TextInput
          id="trash-directory"
          value={directoryPath}
          disabled={disabled}
          aria-describedby="trash-directory-validation"
          aria-invalid={validation === 'invalid'}
          onChange={function editDirectory(event) {
            editedSinceFocus.current = true;
            ++validationRequest.current;
            onEdit();
            setDirectoryPath(event.currentTarget.value);
            setValidation('');
          }}
          onBlur={function leaveDirectory(event) {
            focused.current = false;
            if (
              event.relatedTarget instanceof HTMLElement &&
              event.relatedTarget.dataset.directoryPicker
            )
              return;
            onValidation(validateManualEdit());
          }}
          onFocus={function enterDirectory() {
            focused.current = true;
          }}
        />
        <Button
          variant="default"
          disabled={disabled}
          data-directory-picker="true"
          onClick={function changeDirectory() {
            onValidation(selectDirectory());
          }}
        >
          {appstrings.changeDirectory}
        </Button>
      </div>
    </div>
  );
}
