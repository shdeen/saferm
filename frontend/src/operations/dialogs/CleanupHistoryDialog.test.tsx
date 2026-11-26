import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { CleanupHistoryDialog } from './CleanupHistoryDialog';
import { appstrings } from '../../shared/text';

afterEach(cleanup);

// Permanent: history cleanup exposes the existing orphan option without purging files.
it('submits the selected orphan cleanup option', async function cleanupOrphans() {
  const perform = vi.fn().mockResolvedValue(true);
  const user = userEvent.setup();
  render(
    <MantineProvider env="test">
      <CleanupHistoryDialog busy={false} perform={perform} onClose={vi.fn()} />
    </MantineProvider>,
  );
  await user.click(screen.getByRole('checkbox', { name: appstrings.cleanupOrphans }));
  await user.click(screen.getByRole('button', { name: appstrings.cleanupSubmit }));
  expect(perform).toHaveBeenCalledWith({ Command: 'cleanup', Orphans: true });
});
