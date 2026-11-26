import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Group, Loader, Stack, Text, TextInput, Title } from '@mantine/core';
import {
  openConfiguration,
  runOperation,
  updateSettings,
  type Settings,
  type Request,
} from '../bridge/operations';
import { appstrings, formatText } from '../shared/text';
import { TrashDirectory } from './TrashDirectory';
import { ReviewPaths } from './ReviewPaths';

/** Milliseconds without a further edit before the pending changes are saved. */
const saveDelay = 250;

/**
 * Shows the settings for editing and saves each accepted edit after a short pause in
 * typing. Edits still pending when the component unmounts are saved then.
 */
export function Configuration({
  revision,
  busy,
  perform,
}: {
  /** Counter of finished operations. The settings reload when it changes. */
  revision: number;
  busy: boolean;
  /** Submits the reset request. */
  perform: (request: Partial<Request>) => Promise<boolean>;
}) {
  const [settings, setSettings] = useState<Settings>();
  const [savedReview, setSavedReview] = useState<string[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [resetting, setResetting] = useState(false);
  const pendingChanges = useRef<Partial<Settings>>({});
  const saveTimer = useRef<number | undefined>(undefined);
  const saveInProgress = useRef<Promise<unknown>>(Promise.resolve());
  const directoryValidation = useRef<Promise<void>>(Promise.resolve());
  const ruleValidation = useRef<Promise<void>>(Promise.resolve());
  const latestSavedReview = useRef<string[]>([]);

  /**
   * Saves the pending changes after any save that is already running. Returns a
   * promise that settles when the saves finish; a failed save sets the error message.
   */
  function savePendingChanges() {
    window.clearTimeout(saveTimer.current);
    const changes = pendingChanges.current;
    pendingChanges.current = {};
    if (Object.keys(changes).length === 0) return saveInProgress.current;
    saveInProgress.current = saveInProgress.current.then(async function saveChanges() {
      try {
        const savedSettings = await updateSettings(changes);
        if (changes.requireReview) {
          latestSavedReview.current = changes.requireReview;
          setSavedReview(changes.requireReview);
        }
        setSettings(function updateSource(previousSettings) {
          return previousSettings
            ? {
                ...previousSettings,
                path: savedSettings.path,
                external: savedSettings.external,
                invalidTrashPath: savedSettings.invalidTrashPath,
                invalidRetainPeriod: savedSettings.invalidRetainPeriod,
              }
            : savedSettings;
        });
        setError('');
      } catch (saveError) {
        setError(String(saveError));
      }
    });
    return saveInProgress.current;
  }

  /** Adds the changes to the pending changes and restarts the save delay. */
  function scheduleSave(changes: Partial<Settings>) {
    pendingChanges.current = { ...pendingChanges.current, ...changes };
    window.clearTimeout(saveTimer.current);
    saveTimer.current = window.setTimeout(savePendingChanges, saveDelay);
  }

  function cancelRetentionSave() {
    delete pendingChanges.current.retainPeriod;
  }
  function cancelDirectorySave() {
    delete pendingChanges.current.trashPath;
  }
  function cancelReviewSave() {
    delete pendingChanges.current.requireReview;
  }

  /** Waits for every save that has started and returns the review list as last saved. */
  async function awaitSavedReview() {
    await saveInProgress.current;
    return latestSavedReview.current;
  }

  useEffect(
    function loadConfiguration() {
      let mounted = true;
      setLoading(true);
      async function loadSettings() {
        await directoryValidation.current;
        await ruleValidation.current;
        await saveInProgress.current;
        try {
          const response = await runOperation({ Command: 'config' });
          if (!mounted) return;
          // Settings that arrive with an error hold a malformed review rule, which the
          // rule list reports beside the rule.
          setError(response.result.config ? '' : response.error);
          setSettings(response.result.config ?? undefined);
          latestSavedReview.current = response.result.config?.requireReview ?? [];
          setSavedReview(latestSavedReview.current);
        } catch (loadError) {
          if (mounted) setError(String(loadError));
        } finally {
          if (mounted) setLoading(false);
        }
      }
      void loadSettings();
      return function finishEditing() {
        mounted = false;
        void Promise.all([directoryValidation.current, ruleValidation.current]).then(
          savePendingChanges,
        );
      };
    },
    [revision],
  );

  /** Requests a reset to the default configuration once the checks and saving finish. */
  async function resetConfiguration() {
    setResetting(true);
    try {
      await directoryValidation.current;
      await ruleValidation.current;
      await savePendingChanges();
      await perform({ Command: 'config', Setup: true, Reset: true });
    } finally {
      setResetting(false);
    }
  }

  return (
    <section className="configuration-panel">
      <Stack gap="xl">
        <Title order={2}>{appstrings.settings}</Title>
        {error && <Alert color="red">{error}</Alert>}
        {loading ? (
          <Loader size="sm" />
        ) : (
          settings && (
            <form
              className="configuration-fields"
              onSubmit={function preventSubmit(event) {
                event.preventDefault();
              }}
            >
              <div>
                <div id="configuration-source-label" className="settings-label">
                  {appstrings.configSource}
                </div>
                <div className="configuration-path">
                  <div
                    className="configuration-source"
                    aria-labelledby="configuration-source-label"
                    title={settings.external ? settings.path : appstrings.embedded}
                  >
                    {settings.external ? settings.path : appstrings.embedded}
                  </div>
                  <Button
                    variant="default"
                    disabled={!settings.external || busy}
                    onClick={async function openFile() {
                      try {
                        await openConfiguration();
                      } catch (openError) {
                        setError(String(openError));
                      }
                    }}
                  >
                    {appstrings.openConfig}
                  </Button>
                </div>
                {settings.path && !settings.external && (
                  <Text size="xs" c="dimmed" mt={6}>
                    {appstrings.configPath}: {settings.path}
                  </Text>
                )}
              </div>
              <TrashDirectory
                initialPath={settings.trashPath ?? ''}
                disabled={busy || resetting}
                onError={setError}
                onEdit={cancelDirectorySave}
                onValidation={function trackValidation(validation) {
                  directoryValidation.current = validation;
                }}
                onChange={function saveDirectory(trashPath) {
                  scheduleSave({ trashPath });
                }}
              />
              {settings.invalidTrashPath && (
                <InvalidValueMessage invalidValue={settings.invalidTrashPath} />
              )}
              <HistoryRetention
                initialValue={settings.retainPeriod}
                disabled={busy || resetting}
                onChange={function saveRetention(retainPeriod) {
                  scheduleSave({ retainPeriod });
                }}
                onInvalid={cancelRetentionSave}
              />
              {settings.invalidRetainPeriod && (
                <InvalidValueMessage invalidValue={settings.invalidRetainPeriod} />
              )}
              <ReviewPaths
                initialPaths={settings.requireReview ?? []}
                savedPaths={savedReview}
                awaitSavedPaths={awaitSavedReview}
                disabled={busy || resetting}
                onError={setError}
                onEdit={cancelReviewSave}
                onValidation={function trackRuleValidation(validation) {
                  ruleValidation.current = validation;
                }}
                onChange={function saveReviewPaths(requireReview) {
                  scheduleSave({ requireReview });
                }}
              />
            </form>
          )
        )}
        <Group>
          <Button
            variant="default"
            disabled={busy || loading || resetting}
            onClick={function reset() {
              void resetConfiguration();
            }}
          >
            {appstrings.resetConfig}
          </Button>
        </Group>
      </Stack>
    </section>
  );
}

/**
 * Shows the error for a value in the configuration file that could not be applied,
 * which the built-in value replaces.
 */
function InvalidValueMessage({ invalidValue }: { invalidValue: string }) {
  return (
    <Text size="sm" c="red" role="alert">
      {formatText(appstrings.invalidConfigValue, { value: invalidValue })}
    </Text>
  );
}

/**
 * Edits the retention period in days. Reports each whole number as it is typed, and
 * marks other text as invalid once focus leaves the field.
 */
function HistoryRetention({
  initialValue,
  disabled,
  onChange,
  onInvalid,
}: {
  initialValue: number | null;
  disabled: boolean;
  onChange: (days: number) => void;
  /** Called when an edit leaves text that is not a whole number. */
  onInvalid: () => void;
}) {
  const [retentionText, setRetentionText] = useState(initialValue?.toString() ?? '');
  const [invalid, setInvalid] = useState(false);
  const editedSinceFocus = useRef(false);
  const validNumber = /^\d+$/.test(retentionText) && Number.isSafeInteger(Number(retentionText));

  return (
    <div>
      <div className="settings-label">
        <label htmlFor="history-retention">{appstrings.retention}</label>
        <span id="retention-validation" className="settings-validation invalid" role="status">
          {invalid ? appstrings.invalidRetention : ''}
        </span>
      </div>
      <TextInput
        id="history-retention"
        className="retention-input"
        inputMode="numeric"
        value={retentionText}
        disabled={disabled}
        aria-invalid={invalid}
        aria-describedby="retention-validation"
        onChange={function editRetention(event) {
          const editedText = event.currentTarget.value;
          setRetentionText(editedText);
          setInvalid(false);
          editedSinceFocus.current = true;
          const retentionDays = Number(editedText);
          if (/^\d+$/.test(editedText) && Number.isSafeInteger(retentionDays))
            onChange(retentionDays);
          else onInvalid();
        }}
        onBlur={function validateRetention() {
          if (editedSinceFocus.current) setInvalid(!validNumber);
          editedSinceFocus.current = false;
        }}
      />
    </div>
  );
}
