import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { NewOperationDialog } from './NewOperationDialog';
import { appstrings } from '../../shared/text';

vi.mock('../../bridge/operations', function mockFileDialogs() {
  return { chooseSourcePath: vi.fn(), chooseDestinationPath: vi.fn() };
});
afterEach(cleanup);

// Permanent: move requests retain the source, destination, reason, and optional metadata.
it('submits a move with its metadata and closes after success', async function createMove() {
  const perform = vi.fn().mockResolvedValue(true);
  const closeDialog = vi.fn();
  const user = userEvent.setup();
  render(
    <MantineProvider env="test">
      <NewOperationDialog busy={false} perform={perform} onClose={closeDialog} />
    </MantineProvider>,
  );
  await user.click(screen.getByRole('radio', { name: appstrings.type.move }));
  await user.type(
    screen.getByRole('textbox', { name: new RegExp(appstrings.source) }),
    '/Users/test/source',
  );
  await user.type(
    screen.getByRole('textbox', { name: new RegExp(appstrings.destination) }),
    '/Users/test/destination',
  );
  await user.type(
    screen.getByRole('textbox', { name: new RegExp(appstrings.reason) }),
    'Organize drafts',
  );
  await user.type(screen.getByLabelText(appstrings.requester), 'Human');
  await user.type(screen.getByLabelText(appstrings.description), 'Keep the original contents');
  await user.click(screen.getByRole('button', { name: appstrings.moveSubmit }));
  expect(perform).toHaveBeenCalledWith({
    Command: 'move',
    Source: '/Users/test/source',
    Destination: '/Users/test/destination',
    Reason: 'Organize drafts',
    Requester: 'Human',
    Description: 'Keep the original contents',
    Overwrite: false,
  });
  expect(closeDialog).toHaveBeenCalledOnce();
});

// Kind: permanent. Replacement consent belongs only to the selected move request.
it.each(['move', 'delete'])(
  'limits checked replacement to %s submissions',
  async function moveReplacement(operationType) {
    const perform = vi.fn().mockResolvedValue(false);
    const user = userEvent.setup();
    render(
      <MantineProvider env="test">
        <NewOperationDialog busy={false} perform={perform} onClose={vi.fn()} />
      </MantineProvider>,
    );
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    await user.click(screen.getByRole('radio', { name: appstrings.type.move }));
    await user.type(
      screen.getByRole('textbox', { name: new RegExp(appstrings.source) }),
      '/source',
    );
    await user.type(
      screen.getByRole('textbox', { name: new RegExp(appstrings.destination) }),
      '/destination',
    );
    await user.type(
      screen.getByRole('textbox', { name: new RegExp(appstrings.reason) }),
      'Organize files',
    );
    const overwrite = screen.getByRole('checkbox');
    expect(overwrite).not.toBeChecked();
    await user.click(overwrite);
    if (operationType === 'delete') {
      await user.click(screen.getByRole('radio', { name: appstrings.type.delete }));
      expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    }
    await user.click(
      screen.getByRole('button', {
        name: operationType === 'move' ? appstrings.moveSubmit : appstrings.deleteSubmit,
      }),
    );
    expect(perform).toHaveBeenCalledWith(
      expect.objectContaining({
        Command: operationType,
        Overwrite: operationType === 'move',
        Destination: operationType === 'move' ? '/destination' : '',
      }),
    );
  },
);
