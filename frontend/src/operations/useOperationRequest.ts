import { useRef, useState } from 'react';
import { runOperation, type Request, type Confirmation } from '../bridge/operations';
import { showError, showInformation } from '../bridge/desktop';
import { appstrings, formatText } from '../shared/text';

/**
 * Runs operation requests one at a time and reports each outcome in a native dialog.
 *
 * `perform` resolves to true only when the operation finishes without an error and
 * without asking for confirmation. A request made while another is running resolves to
 * false without running. `confirmation` holds the request that awaits the user's
 * confirmation, and `revision` counts the finished operations.
 */
export function useOperationRequest() {
  const [busy, setBusy] = useState(false);
  const operationRunning = useRef(false);
  const [confirmation, setConfirmation] = useState<{
    request: Partial<Request>;
    details: Confirmation;
  }>();
  const [revision, setRevision] = useState(0);

  async function perform(request: Partial<Request>): Promise<boolean> {
    if (operationRunning.current) return false;
    operationRunning.current = true;
    setBusy(true);
    try {
      const response = await runOperation(request);
      if (response.result.confirmation) {
        setConfirmation({ request, details: response.result.confirmation });
        return false;
      }
      setConfirmation(undefined);
      setRevision(function advanceRevision(previousRevision) {
        return previousRevision + 1;
      });
      if (response.error) {
        await showError([response.error, ...(response.result.problems ?? [])].join('\n\n'));
      } else if (response.result.problems?.length) {
        await showInformation(response.result.problems.join('\n\n'));
      } else if (response.result.notice) {
        await showInformation(response.result.notice);
      } else if (response.result.removed) {
        await showInformation(
          formatText(appstrings.removedCount, { count: response.result.removed.length }),
        );
      }
      return !response.error;
    } catch (error) {
      await showError(String(error));
      return false;
    } finally {
      operationRunning.current = false;
      setBusy(false);
    }
  }

  function cancelConfirmation() {
    if (!operationRunning.current) setConfirmation(undefined);
  }

  return { perform, busy, confirmation, cancelConfirmation, revision };
}
