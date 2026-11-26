import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';
import { getOperationPage, getOperationStatuses, runOperation } from '../bridge/operations';
import { showError, showInformation, takeReview, onReview } from '../bridge/desktop';
import { appstrings, formatText } from '../shared/text';

vi.mock('../bridge/operations', function mockBridge() {
  return {
    getOperationPage: vi.fn(),
    getOperationStatuses: vi.fn(),
    runOperation: vi.fn(),
    chooseSourcePath: vi.fn(),
    chooseDestinationPath: vi.fn(),
  };
});

vi.mock('../bridge/desktop', function mockDesktop() {
  return { showError: vi.fn(), showInformation: vi.fn(), takeReview: vi.fn(), onReview: vi.fn() };
});

const pendingAction = {
  id: 1,
  type: 'delete',
  status: 2,
  source: '/Users/test/.draft',
  reason: 'Remove obsolete draft',
  requester: 'Agent',
  createdAt: '2026-11-03T12:00:00Z',
  updatedAt: '2026-11-03T12:00:00Z',
  inProgress: false,
};

beforeEach(function prepareBridge() {
  vi.mocked(runOperation).mockReset();
  vi.mocked(takeReview).mockResolvedValue(0);
  vi.mocked(onReview).mockReturnValue(function unsubscribe() {});
  vi.mocked(showError).mockResolvedValue(undefined);
  vi.mocked(getOperationStatuses).mockResolvedValue({
    completed: 1,
    pending: 2,
    denied: 3,
    restored: 4,
    purged: 5,
    failed: 6,
  });
  vi.mocked(getOperationPage).mockResolvedValue({
    actions: [pendingAction],
    total: 45,
    pending: 1,
    page: 1,
    pageSize: 20,
  });
  vi.mocked(runOperation).mockResolvedValue({
    result: { actions: [{ ...pendingAction, status: 1 }] },
    error: '',
  });
});

afterEach(cleanup);

describe('Operations interface', function operationsTests() {
  // Kind: permanent. Move replacement asks before resubmitting the unchanged request with consent.
  it('confirms replacement from the move form', async function moveConsent() {
    const destination = '/destination/$& {{.Path}}';
    const moveReason = 'Organize files';
    vi.mocked(runOperation).mockResolvedValueOnce({
      result: {
        confirmation: { prompt: '{{.Path}}?', path: destination, defaultYes: false },
      },
      error: 'Confirmation required',
    });
    const user = userEvent.setup();
    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );
    await user.click(await screen.findByRole('button', { name: appstrings.newOperation }));
    await user.click(screen.getByRole('radio', { name: appstrings.type.move }));
    await user.type(
      screen.getByRole('textbox', { name: new RegExp(appstrings.source) }),
      '/source',
    );
    await user.click(screen.getByRole('textbox', { name: new RegExp(appstrings.destination) }));
    await user.paste(destination);
    await user.type(
      screen.getByRole('textbox', { name: new RegExp(appstrings.reason) }),
      moveReason,
    );
    await user.click(screen.getByRole('checkbox'));
    await user.click(screen.getByRole('button', { name: appstrings.moveSubmit }));
    const dialog = await screen.findByRole('dialog', { name: appstrings.confirmationTitle });
    expect(within(dialog).getByText(`${destination}?`)).toBeVisible();
    expect(runOperation).toHaveBeenCalledTimes(1);
    const request = vi.mocked(runOperation).mock.calls[0][0];
    expect(request).toEqual(expect.objectContaining({ Command: 'move', Overwrite: true }));
    fireEvent.focus(window);
    vi.mocked(getOperationPage).mockResolvedValue({
      actions: [
        {
          ...pendingAction,
          type: 'move',
          source: '/source',
          destination,
          reason: moveReason,
          status: 1,
        },
      ],
      total: 1,
      pending: 0,
      page: 1,
      pageSize: 20,
    });
    await user.click(within(dialog).getByRole('button', { name: appstrings.confirm }));
    await waitFor(function confirmedMove() {
      expect(runOperation).toHaveBeenLastCalledWith({ ...request, Confirmed: true });
      expect(within(screen.getByRole('table')).getByText(moveReason)).toBeVisible();
      expect(
        within(screen.getByRole('table')).getByText(appstrings.status.completed),
      ).toBeVisible();
    });
  });

  // Kind: permanent. Combined decisions display both IDs and literal paths, and wait for consent.
  it.each([
    ['approve', 'confirm'],
    ['approve', 'cancel'],
    ['approve', 'close'],
    ['deny', 'confirm'],
    ['deny', 'cancel'],
    ['deny', 'close'],
  ])('asks before linked %s with answer %s', async function linkedConsent(decision, answer) {
    const path = '/Users/test/$& $$ {{.MoveID}} {{.BlockerID}} (y/N)';
    const move = { ...pendingAction, type: 'move', blockerID: 12 };
    const decidedActions = [
      { ...move, status: 3 },
      { ...pendingAction, id: 12, status: 3 },
    ];
    vi.mocked(getOperationPage).mockResolvedValue({
      actions: [move],
      total: 1,
      pending: 1,
      page: 1,
      pageSize: 20,
    });
    vi.mocked(runOperation)
      .mockResolvedValueOnce({
        result: {
          actions: [move, { ...pendingAction, id: 12 }],
          confirmation: {
            prompt: '{{.MoveID}} / {{.BlockerID}} / {{.Path}}',
            path,
            moveID: 1,
            blockerID: 12,
            defaultYes: false,
          },
        },
        error: 'Confirmation required',
      })
      .mockResolvedValueOnce({
        result: {
          actions: decidedActions,
        },
        error: decision === 'approve' ? 'Move could not finish' : '',
      });
    const user = userEvent.setup();
    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );
    await user.click(
      await screen.findByRole('button', {
        name: appstrings[decision as 'approve' | 'deny'],
      }),
    );
    const dialog = await screen.findByRole('dialog', { name: appstrings.confirmationTitle });
    expect(within(dialog).getByText(`1 / 12 / ${path}`)).toBeVisible();
    expect(within(dialog).getByText(path)).toBeVisible();
    expect(runOperation).toHaveBeenCalledTimes(1);
    expect(within(screen.getByRole('table')).getByText(appstrings.status.pending)).toBeVisible();
    if (answer === 'confirm') {
      vi.mocked(getOperationPage).mockResolvedValue({
        actions: decidedActions,
        total: 2,
        pending: 0,
        page: 1,
        pageSize: 20,
      });
    }
    if (answer === 'close') await user.keyboard('{Escape}');
    else
      await user.click(
        within(dialog).getByRole('button', {
          name: appstrings[answer as 'confirm' | 'cancel'],
        }),
      );
    if (answer === 'confirm') {
      await waitFor(function decisionRefreshes() {
        expect(runOperation).toHaveBeenLastCalledWith({
          Command: 'dispose',
          ID: '1',
          Decision: decision,
          Confirmed: true,
        });
        expect(
          within(screen.getByRole('table')).getAllByText(appstrings.status.denied),
        ).toHaveLength(2);
      });
      if (decision === 'approve') expect(showError).toHaveBeenCalledWith('Move could not finish');
    } else {
      expect(runOperation).toHaveBeenCalledTimes(1);
      expect(
        screen.queryByRole('dialog', { name: appstrings.confirmationTitle }),
      ).not.toBeInTheDocument();
    }
  });

  // Kind: permanent. Details distinguish the independent review rule and the blocking delete.
  it('shows both pending causes in operation details', async function pendingCauses() {
    const move = { ...pendingAction, type: 'move', blockerID: 12, reviewRule: '/protected/*' };
    vi.mocked(runOperation).mockResolvedValue({ result: { actions: [move] }, error: '' });
    const user = userEvent.setup();
    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );
    await user.click(await screen.findByRole('button', { name: `${appstrings.details} #1` }));
    const dialog = await screen.findByRole('dialog', {
      name: formatText(appstrings.operationDetails, { id: 1 }),
    });
    expect(await within(dialog).findByText('/protected/*')).toBeVisible();
    expect(within(dialog).getByText('12')).toBeVisible();
  });

  // Permanent: Review opens the requested drawer even if its operation is not on the visible page.
  it('opens the operation requested by a notification', async function reviewRequest() {
    const reviewAction = { ...pendingAction, id: 99 };
    vi.mocked(takeReview).mockResolvedValueOnce(99);
    vi.mocked(runOperation).mockResolvedValue({ result: { actions: [reviewAction] }, error: '' });
    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );
    expect(
      await screen.findByRole('dialog', {
        name: formatText(appstrings.operationDetails, { id: 99 }),
      }),
    ).toBeVisible();
    expect(runOperation).toHaveBeenCalledWith({ Command: 'view', ID: '99' });
  });

  // Permanent: approving or denying updates history without a success popup.
  it.each(['approve', 'deny'])(
    'confirms %s through history',
    async function decisionFeedback(decision) {
      const user = userEvent.setup();
      render(
        <MantineProvider env="test">
          <App />
        </MantineProvider>,
      );
      const decisionButton = await screen.findByRole('button', {
        name: appstrings[decision as 'approve' | 'deny'],
      });
      vi.mocked(getOperationPage).mockResolvedValue({
        actions: [{ ...pendingAction, status: decision === 'approve' ? 1 : 3 }],
        total: 1,
        pending: 0,
        page: 1,
        pageSize: 20,
      });
      await user.click(decisionButton);
      await waitFor(function decisionCompletes() {
        expect(runOperation).toHaveBeenCalledWith(expect.objectContaining({ Decision: decision }));
        expect(
          within(screen.getByRole('table')).getByText(
            appstrings.status[decision === 'approve' ? 'completed' : 'denied'],
          ),
        ).toBeVisible();
      });
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
      expect(screen.getByRole('button', { name: appstrings.newOperation })).toBeEnabled();
    },
  );

  // Permanent: an operation failure is presented through the native desktop dialog.
  it('shows errors through a native dialog', async function nativeFailure() {
    vi.mocked(runOperation).mockResolvedValue({ result: {}, error: 'Permission denied' });
    const user = userEvent.setup();
    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );
    await user.click(await screen.findByRole('button', { name: appstrings.approve }));
    await waitFor(function errorPresented() {
      expect(showError).toHaveBeenCalledWith(expect.stringContaining('Permission denied'));
    });
  });

  // Kind: permanent. A request that waits because a folder could not be read shows its warning.
  it('shows a request warning through a native dialog', async function requestWarning() {
    const warning = 'Could not read "/Users/test/old/locked".';
    vi.mocked(runOperation).mockResolvedValue({
      result: { actions: [pendingAction], problems: [warning] },
      error: '',
    });
    const user = userEvent.setup();
    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );
    await user.click(await screen.findByRole('button', { name: appstrings.newOperation }));
    await user.type(
      screen.getByRole('textbox', { name: new RegExp(appstrings.source) }),
      '/Users/test/old',
    );
    await user.type(
      screen.getByRole('textbox', { name: new RegExp(appstrings.reason) }),
      'Remove old downloads',
    );
    await user.click(screen.getByRole('button', { name: appstrings.deleteSubmit }));
    await waitFor(function warningPresented() {
      expect(showInformation).toHaveBeenCalledWith(warning);
    });
    expect(showError).not.toHaveBeenCalled();
  });

  // Permanent: humans can decide a pending request directly from the operations list.
  it('approves the selected pending operation from the main list', async function approveRequest() {
    const user = userEvent.setup();
    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );
    await user.click(await screen.findByRole('button', { name: appstrings.approve }));
    await waitFor(function checkDecision() {
      expect(runOperation).toHaveBeenCalledWith(
        expect.objectContaining({ Command: 'dispose', ID: '1', Decision: 'approve' }),
      );
    });
  });

  // Permanent: the default page size is twenty and navigation requests the next page.
  it('requests twenty operations and supports the next page', async function changePage() {
    const user = userEvent.setup();
    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );
    await screen.findByText('Remove obsolete draft');
    expect(getOperationPage).toHaveBeenCalledWith('', '', 1, 20);
    await user.click(screen.getByRole('button', { name: '2' }));
    await waitFor(function checkPage() {
      expect(getOperationPage).toHaveBeenCalledWith('', '', 2, 20);
    });
  });

  // Permanent: operation errors are visible instead of presenting success.
  it('shows a failed operation and its diagnostic details', async function failedDecision() {
    vi.mocked(runOperation).mockResolvedValue({
      result: { problems: ['Permission denied at /Users/test/.draft'] },
      error: 'Unable to transfer the item',
    });
    const user = userEvent.setup();
    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );
    await user.click(await screen.findByRole('button', { name: appstrings.approve }));
    await waitFor(function errorPresented() {
      expect(showError).toHaveBeenCalledWith(
        expect.stringContaining('Unable to transfer the item'),
      );
      expect(showError).toHaveBeenCalledWith(
        expect.stringContaining('Permission denied at /Users/test/.draft'),
      );
    });
  });

  // Permanent: a partial purge does not resubmit already purged items on retry.
  it('retries only remaining eligible items after a partial purge', async function retryPartialPurge() {
    const firstAction = { ...pendingAction, status: 1 };
    const secondAction = { ...pendingAction, id: 2, status: 1, source: '/Users/test/second' };
    vi.mocked(getOperationPage).mockResolvedValue({
      actions: [firstAction, secondAction],
      total: 2,
      pending: 0,
      page: 1,
      pageSize: 20,
    });
    vi.mocked(runOperation).mockResolvedValueOnce({
      result: { actions: [{ ...firstAction, status: 5 }], problems: ['Second file is busy'] },
      error: 'Partial failure',
    });
    const user = userEvent.setup();
    render(
      <MantineProvider env="test">
        <App />
      </MantineProvider>,
    );
    await user.click(
      await screen.findByRole('checkbox', {
        name: formatText(appstrings.selectOperation, { id: 1 }),
      }),
    );
    await user.click(
      screen.getByRole('checkbox', { name: formatText(appstrings.selectOperation, { id: 2 }) }),
    );
    await user.click(
      screen.getByRole('button', { name: formatText(appstrings.purgeSelected, { count: 2 }) }),
    );
    vi.mocked(getOperationPage).mockResolvedValue({
      actions: [{ ...firstAction, status: 5 }, secondAction],
      total: 2,
      pending: 0,
      page: 1,
      pageSize: 20,
    });
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: appstrings.purge }),
    );
    await waitFor(function dialogCloses() {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('Second file is busy'));
    await user.click(
      screen.getByRole('button', { name: formatText(appstrings.purgeSelected, { count: 1 }) }),
    );
    vi.mocked(runOperation).mockResolvedValueOnce({
      result: { actions: [{ ...secondAction, status: 5 }] },
      error: '',
    });
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: appstrings.purge }),
    );
    await waitFor(function onlyRemainingID() {
      expect(runOperation).toHaveBeenLastCalledWith({ Command: 'purge', IDs: ['2'] });
    });
  });

  // Permanent: replacing a restored path waits for the user's explicit confirmation.
  it.each(['confirm', 'cancel'])(
    'keeps literal paths and honors %s during restore confirmation',
    async function confirmRestore(answer) {
      const originalAction = { ...pendingAction, status: 1, source: '/Users/test/$&/$$/{{.Path}}' };
      vi.mocked(getOperationPage).mockResolvedValue({
        actions: [originalAction],
        total: 1,
        pending: 0,
        page: 1,
        pageSize: 20,
      });
      vi.mocked(runOperation).mockResolvedValueOnce({
        result: {
          confirmation: {
            prompt: 'Restore {{.Path}}?',
            path: originalAction.source,
            defaultYes: false,
          },
        },
        error: 'Confirmation required',
      });
      const user = userEvent.setup();
      render(
        <MantineProvider env="test">
          <App />
        </MantineProvider>,
      );
      await user.click(await screen.findByRole('button', { name: appstrings.restore }));
      await user.click(screen.getByRole('checkbox', { name: appstrings.overwrite }));
      await user.click(
        within(screen.getByRole('dialog')).getByRole('button', { name: appstrings.restore }),
      );
      await screen.findByText(`Restore ${originalAction.source}?`);
      const confirmationDialog = screen.getByRole('dialog', { name: appstrings.confirmationTitle });
      expect(within(confirmationDialog).queryByText(/\(y\/N\)/)).not.toBeInTheDocument();
      expect(runOperation).toHaveBeenCalledTimes(1);
      expect(runOperation).toHaveBeenLastCalledWith({
        Command: 'restore',
        ID: '1',
        Overwrite: true,
      });
      await user.click(
        within(confirmationDialog).getByRole('button', {
          name: appstrings[answer as 'confirm' | 'cancel'],
        }),
      );
      if (answer === 'confirm') {
        await waitFor(function explicitConsent() {
          expect(runOperation).toHaveBeenLastCalledWith({
            Command: 'restore',
            ID: '1',
            Overwrite: true,
            Confirmed: true,
          });
        });
      } else {
        expect(runOperation).toHaveBeenCalledTimes(1);
        expect(
          screen.queryByRole('dialog', { name: appstrings.confirmationTitle }),
        ).not.toBeInTheDocument();
      }
    },
  );

  // Kind: permanent. Pending deletes and completed partial results appear after a failed restore.
  it.each([2, 1])(
    'refreshes history when restore returns an occupying delete with status %s',
    async function restorePartialHistory(deleteStatus) {
      const originalAction = { ...pendingAction, status: 1 };
      const occupyingDelete = {
        ...pendingAction,
        id: 2,
        status: deleteStatus,
        reason: 'Preserved occupying item',
      };
      vi.mocked(getOperationPage).mockResolvedValue({
        actions: [originalAction],
        total: 1,
        pending: 0,
        page: 1,
        pageSize: 20,
      });
      vi.mocked(runOperation)
        .mockResolvedValueOnce({
          result: {
            confirmation: {
              prompt: 'Restore {{.Path}}?',
              path: originalAction.source,
              defaultYes: false,
            },
          },
          error: 'Confirmation required',
        })
        .mockResolvedValueOnce({
          result: { actions: [occupyingDelete] },
          error: 'Inspect delete #2 before retrying restore',
        });
      const user = userEvent.setup();
      render(
        <MantineProvider env="test">
          <App />
        </MantineProvider>,
      );
      await user.click(await screen.findByRole('button', { name: appstrings.restore }));
      await user.click(screen.getByRole('checkbox', { name: appstrings.overwrite }));
      await user.click(
        within(screen.getByRole('dialog')).getByRole('button', { name: appstrings.restore }),
      );
      vi.mocked(getOperationPage).mockResolvedValue({
        actions: [occupyingDelete, originalAction],
        total: 2,
        pending: deleteStatus === 2 ? 1 : 0,
        page: 1,
        pageSize: 20,
      });
      await user.click(await screen.findByRole('button', { name: appstrings.confirm }));
      await waitFor(function historyRefreshesAfterError() {
        expect(showError).toHaveBeenCalledWith('Inspect delete #2 before retrying restore');
      });
      await user.click(
        within(screen.getByRole('dialog', { name: appstrings.restoreTitle })).getByRole('button', {
          name: appstrings.cancel,
        }),
      );
      const deleteRow = (await screen.findByText(occupyingDelete.reason)).closest('tr');
      const originalRow = screen.getByText(originalAction.reason).closest('tr');
      expect(deleteRow).not.toBeNull();
      expect(originalRow).not.toBeNull();
      expect(within(originalRow!).getByRole('button', { name: appstrings.restore })).toBeEnabled();
      if (deleteStatus === 2) {
        await user.click(within(deleteRow!).getByRole('button', { name: appstrings.approve }));
        await waitFor(function pendingDeleteCanBeApproved() {
          expect(runOperation).toHaveBeenLastCalledWith({
            Command: 'dispose',
            ID: '2',
            Decision: 'approve',
          });
        });
      }
    },
  );
});
