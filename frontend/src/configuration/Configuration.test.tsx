import { MantineProvider } from '@mantine/core';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Configuration } from './Configuration';
import {
  runOperation,
  updateSettings,
  directoryExists,
  ruleMalformed,
  chooseDirectory,
  openConfiguration,
} from '../bridge/operations';
import { appstrings } from '../shared/text';

vi.mock('../bridge/operations', function mockSettingsBridge() {
  return {
    runOperation: vi.fn(),
    updateSettings: vi.fn(),
    directoryExists: vi.fn(),
    ruleMalformed: vi.fn(),
    chooseDirectory: vi.fn(),
    openConfiguration: vi.fn(),
  };
});

const storedSettings = {
  trashPath: '/trash',
  retainPeriod: 30,
  requireReview: ['~/.*'],
  path: '/config.toml',
  external: true,
};
const perform = vi.fn();

beforeEach(function prepareSettings() {
  vi.useFakeTimers();
  vi.mocked(runOperation).mockResolvedValue({ result: { config: storedSettings }, error: '' });
  vi.mocked(directoryExists).mockImplementation(async function existingDirectory(path) {
    return path !== '/missing';
  });
  vi.mocked(ruleMalformed).mockImplementation(async function unclosedSet(pattern) {
    return pattern.includes('[') && !pattern.includes(']');
  });
  vi.mocked(updateSettings).mockImplementation(async function persistSettings(changes) {
    return { ...storedSettings, ...changes };
  });
  vi.mocked(chooseDirectory).mockResolvedValue('/selected');
  vi.mocked(openConfiguration).mockResolvedValue(undefined);
});

afterEach(function restoreTimers() {
  cleanup();
  vi.useRealTimers();
});

async function renderSettings() {
  await act(async function mountSettings() {
    render(
      <MantineProvider env="test">
        <Configuration revision={0} busy={false} perform={perform} />
      </MantineProvider>,
    );
  });
}

async function advanceTime(milliseconds: number) {
  await act(async function advanceTimers() {
    await vi.advanceTimersByTimeAsync(milliseconds);
  });
}

describe('Settings editing', function settingsTests() {
  it('saves the final valid integer after 250 ms and flags invalid input only after blur', async function retentionEditing() {
    await renderSettings();
    const retentionField = screen.getByLabelText(appstrings.retention);
    fireEvent.change(retentionField, { target: { value: '4' } });
    await advanceTime(200);
    fireEvent.change(retentionField, { target: { value: '42' } });
    await advanceTime(249);
    expect(updateSettings).not.toHaveBeenCalled();
    await advanceTime(1);
    expect(updateSettings).toHaveBeenLastCalledWith({ retainPeriod: 42 });
    fireEvent.change(retentionField, { target: { value: '4.2' } });
    await advanceTime(250);
    expect(screen.queryByText('not a valid number')).not.toBeInTheDocument();
    fireEvent.blur(retentionField);
    expect(screen.getByText('not a valid number')).toBeVisible();
    expect(updateSettings).toHaveBeenCalledTimes(1);
  });

  it('validates typed directories on blur and clears valid feedback after five seconds', async function directoryEditing() {
    await renderSettings();
    const directoryField = screen.getByLabelText(appstrings.configuredTrashPath);
    fireEvent.focus(directoryField);
    fireEvent.change(directoryField, { target: { value: '/missing' } });
    await advanceTime(300);
    expect(screen.queryByText('directory does not exist')).not.toBeInTheDocument();
    await act(async function blurMissingDirectory() {
      fireEvent.blur(directoryField);
    });
    expect(screen.getByText('directory does not exist')).toBeVisible();
    await advanceTime(6000);
    expect(screen.getByText('directory does not exist')).toBeVisible();
    expect(updateSettings).not.toHaveBeenCalled();
    fireEvent.focus(directoryField);
    fireEvent.change(directoryField, { target: { value: '/valid' } });
    await act(async function blurValidDirectory() {
      fireEvent.blur(directoryField);
    });
    expect(screen.getByText('valid')).toBeVisible();
    await advanceTime(250);
    expect(updateSettings).toHaveBeenLastCalledWith({ trashPath: '/valid' });
    await advanceTime(4750);
    expect(screen.queryByText('valid')).not.toBeInTheDocument();
  });

  it('flags an invalid directory loaded from configuration', async function invalidSavedDirectory() {
    vi.mocked(runOperation).mockResolvedValue({
      result: { config: { ...storedSettings, trashPath: '/missing' } },
      error: '',
    });
    await renderSettings();
    expect(screen.getByText('directory does not exist')).toBeVisible();
    expect(updateSettings).not.toHaveBeenCalled();
  });

  it('uses a selected directory without showing validation feedback', async function selectedDirectory() {
    await renderSettings();
    await act(async function selectDirectory() {
      fireEvent.click(screen.getByRole('button', { name: 'Change' }));
    });
    expect(screen.getByLabelText(appstrings.configuredTrashPath)).toHaveValue('/selected');
    await advanceTime(250);
    expect(updateSettings).toHaveBeenLastCalledWith({ trashPath: '/selected' });
    expect(screen.queryByText('valid')).not.toBeInTheDocument();
    expect(screen.queryByText('directory does not exist')).not.toBeInTheDocument();
  });

  it('opens the configuration through the desktop and preserves reset behavior', async function configurationActions() {
    await renderSettings();
    await act(async function openFile() {
      fireEvent.click(screen.getByRole('button', { name: 'Open' }));
    });
    expect(openConfiguration).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('button', { name: appstrings.resetConfig }));
    await advanceTime(0);
    expect(perform).toHaveBeenCalledWith({ Command: 'config', Setup: true, Reset: true });
  });

  it('adds, edits and removes arbitrary review paths', async function reviewPathEditing() {
    await renderSettings();
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    const reviewList = screen.getByRole('group', { name: appstrings.reviewRules });
    const reviewInputs = within(reviewList).getAllByRole('textbox');
    fireEvent.change(reviewInputs[1], { target: { value: 'any value' } });
    await advanceTime(250);
    expect(updateSettings).toHaveBeenLastCalledWith({ requireReview: ['~/.*', 'any value'] });
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    await advanceTime(250);
    expect(updateSettings).toHaveBeenLastCalledWith({ requireReview: ['~/.*'] });
  });

  // Kind: permanent. A malformed rule typed into a row is not saved, and its row says so.
  it('keeps a malformed review rule unsaved and flags its row after blur', async function malformedRuleEditing() {
    await renderSettings();
    const ruleField = screen.getByLabelText(`${appstrings.reviewRule} 1`);
    fireEvent.focus(ruleField);
    fireEvent.change(ruleField, { target: { value: '[abc' } });
    await advanceTime(300);
    expect(updateSettings).not.toHaveBeenCalled();
    expect(screen.queryByText(appstrings.invalidRule)).not.toBeInTheDocument();
    fireEvent.blur(ruleField);
    expect(screen.getByText(appstrings.invalidRule)).toBeVisible();
    expect(ruleField).toHaveValue('[abc');
    expect(screen.queryByText(appstrings.malformedRule)).not.toBeInTheDocument();
    fireEvent.focus(ruleField);
    fireEvent.change(ruleField, { target: { value: '[abc]' } });
    await advanceTime(250);
    expect(updateSettings).toHaveBeenLastCalledWith({ requireReview: ['[abc]'] });
    fireEvent.blur(ruleField);
    expect(screen.queryByText(appstrings.invalidRule)).not.toBeInTheDocument();
  });

  // Kind: permanent. A malformed rule that is already saved is flagged beside its row and on the page.
  it('flags a malformed review rule loaded from configuration', async function malformedSavedRule() {
    const serviceError = 'review rule error from the service';
    vi.mocked(runOperation).mockResolvedValue({
      result: { config: { ...storedSettings, requireReview: ['~/.*', '[abc'] } },
      error: serviceError,
    });
    await renderSettings();
    expect(screen.getByText(appstrings.invalidRule)).toBeVisible();
    expect(screen.getByText(appstrings.malformedRule)).toBeVisible();
    expect(screen.queryByText(serviceError)).not.toBeInTheDocument();
    expect(updateSettings).not.toHaveBeenCalled();
    const ruleField = screen.getByLabelText(`${appstrings.reviewRule} 2`);
    fireEvent.focus(ruleField);
    fireEvent.change(ruleField, { target: { value: '[abc]' } });
    await advanceTime(250);
    expect(updateSettings).toHaveBeenLastCalledWith({ requireReview: ['~/.*', '[abc]'] });
    expect(screen.queryByText(appstrings.malformedRule)).not.toBeInTheDocument();
  });

  // Kind: permanent. A valid rule text that a malformed edit replaces before it is saved is never saved.
  it('does not save an earlier valid rule after the row becomes malformed', async function cancelSupersededRule() {
    await renderSettings();
    const ruleField = screen.getByLabelText(`${appstrings.reviewRule} 1`);
    fireEvent.change(ruleField, { target: { value: 'notes' } });
    await advanceTime(100);
    fireEvent.change(ruleField, { target: { value: 'notes[' } });
    await advanceTime(300);
    expect(updateSettings).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    fireEvent.change(screen.getByLabelText(`${appstrings.reviewRule} 2`), {
      target: { value: 'other' },
    });
    await advanceTime(250);
    expect(updateSettings).toHaveBeenLastCalledWith({ requireReview: ['~/.*', 'other'] });
  });

  // Kind: permanent. A rule counts as saved only once it is written, also when another field delays the write.
  it('keeps the written rule when a malformed edit follows a delayed save', async function delayedRuleSave() {
    for (const correctsRule of [false, true]) {
      vi.mocked(updateSettings).mockClear();
      await renderSettings();
      const ruleField = screen.getByLabelText(`${appstrings.reviewRule} 1`);
      fireEvent.change(ruleField, { target: { value: 'notes' } });
      await advanceTime(100);
      fireEvent.change(screen.getByLabelText(appstrings.retention), { target: { value: '42' } });
      await advanceTime(200);
      fireEvent.change(ruleField, { target: { value: 'notes[' } });
      await advanceTime(300);
      expect(updateSettings).toHaveBeenCalledTimes(1);
      expect(updateSettings).toHaveBeenLastCalledWith({ retainPeriod: 42 });
      if (correctsRule) {
        fireEvent.change(ruleField, { target: { value: 'notes' } });
        await advanceTime(250);
        expect(updateSettings).toHaveBeenLastCalledWith({ requireReview: ['notes'] });
      } else {
        fireEvent.click(screen.getByRole('button', { name: 'Add' }));
        fireEvent.change(screen.getByLabelText(`${appstrings.reviewRule} 2`), {
          target: { value: 'other' },
        });
        await advanceTime(250);
        expect(updateSettings).toHaveBeenLastCalledWith({ requireReview: ['~/.*', 'other'] });
      }
      cleanup();
    }
  });

  // Kind: permanent. A rule restored while an earlier edit is being written is written afterwards.
  it('writes a rule that is restored while an earlier edit is being written', async function restoreDuringWrite() {
    await renderSettings();
    let finishWrite!: () => void;
    const slowWrite = new Promise<typeof storedSettings>(function waitForWrite(resolve) {
      finishWrite = function resolveWrite() {
        resolve({ ...storedSettings, requireReview: ['notes'] });
      };
    });
    vi.mocked(updateSettings).mockReturnValueOnce(slowWrite);
    const ruleField = screen.getByLabelText(`${appstrings.reviewRule} 1`);
    fireEvent.change(ruleField, { target: { value: 'notes' } });
    await advanceTime(250);
    expect(updateSettings).toHaveBeenLastCalledWith({ requireReview: ['notes'] });
    fireEvent.change(ruleField, { target: { value: '~/.*' } });
    await advanceTime(100);
    await act(async function completeWrite() {
      finishWrite();
    });
    await advanceTime(300);
    expect(updateSettings).toHaveBeenLastCalledWith({ requireReview: ['~/.*'] });
    expect(ruleField).toHaveValue('~/.*');
  });

  // Kind: permanent. A row with malformed text keeps its last successfully saved rule when another row is saved.
  it('keeps the saved rule of a malformed row whether the write before it fails or succeeds', async function rowEditAfterWrite() {
    for (const firstWriteFails of [true, false]) {
      vi.mocked(updateSettings).mockClear();
      vi.mocked(runOperation).mockResolvedValue({
        result: { config: { ...storedSettings, requireReview: ['annual', 'monthly'] } },
        error: '',
      });
      await renderSettings();
      let settleWrite!: () => void;
      const slowWrite = new Promise<typeof storedSettings>(function waitForWrite(resolve, reject) {
        settleWrite = function finishWrite() {
          if (firstWriteFails) reject(new Error('write failed'));
          else resolve({ ...storedSettings, requireReview: ['quarterly', 'monthly'] });
        };
      });
      vi.mocked(updateSettings).mockReturnValueOnce(slowWrite);
      const firstField = screen.getByLabelText(`${appstrings.reviewRule} 1`);
      fireEvent.change(firstField, { target: { value: 'quarterly' } });
      await advanceTime(250);
      expect(updateSettings).toHaveBeenLastCalledWith({ requireReview: ['quarterly', 'monthly'] });
      fireEvent.change(firstField, { target: { value: 'quarterly[' } });
      fireEvent.change(screen.getByLabelText(`${appstrings.reviewRule} 2`), {
        target: { value: 'weekly' },
      });
      await advanceTime(100);
      await act(async function finishFirstWrite() {
        settleWrite();
      });
      await advanceTime(300);
      expect(updateSettings).toHaveBeenLastCalledWith({
        requireReview: [firstWriteFails ? 'annual' : 'quarterly', 'weekly'],
      });
      cleanup();
    }
  });

  // Kind: permanent. A rule edit that is still being checked when the page closes is saved only when well formed.
  it('saves a well-formed rule edit when navigation occurs during its check', async function navigateDuringRuleCheck() {
    for (const checkedAsMalformed of [false, true]) {
      vi.mocked(updateSettings).mockClear();
      await renderSettings();
      let resolveCheck!: (malformed: boolean) => void;
      const checkPromise = new Promise<boolean>(function waitForCheck(resolve) {
        resolveCheck = resolve;
      });
      vi.mocked(ruleMalformed).mockReturnValueOnce(checkPromise);
      fireEvent.change(screen.getByLabelText(`${appstrings.reviewRule} 1`), {
        target: { value: 'notes' },
      });
      cleanup();
      await act(async function completeCheck() {
        resolveCheck(checkedAsMalformed);
      });
      if (checkedAsMalformed) expect(updateSettings).not.toHaveBeenCalled();
      else expect(updateSettings).toHaveBeenCalledWith({ requireReview: ['notes'] });
    }
  });

  it('does not save an earlier valid number after the field becomes invalid', async function cancelSupersededNumber() {
    await renderSettings();
    const retentionField = screen.getByLabelText(appstrings.retention);
    fireEvent.change(retentionField, { target: { value: '42' } });
    await advanceTime(100);
    fireEvent.change(retentionField, { target: { value: '42x' } });
    await advanceTime(300);
    expect(updateSettings).not.toHaveBeenCalled();
  });

  it('finishes directory validation and saving before resetting', async function resetDuringValidation() {
    await renderSettings();
    let resolveValidation!: (exists: boolean) => void;
    const validationPromise = new Promise<boolean>(function waitForValidation(resolve) {
      resolveValidation = resolve;
    });
    vi.mocked(directoryExists).mockReturnValueOnce(validationPromise);
    const directoryField = screen.getByLabelText(appstrings.configuredTrashPath);
    fireEvent.change(directoryField, { target: { value: '/valid' } });
    fireEvent.blur(directoryField);
    fireEvent.click(screen.getByRole('button', { name: appstrings.resetConfig }));
    await advanceTime(0);
    expect(perform).not.toHaveBeenCalled();
    await act(async function completeValidation() {
      resolveValidation(true);
    });
    expect(updateSettings).toHaveBeenCalledWith({ trashPath: '/valid' });
    expect(perform).toHaveBeenCalledWith({ Command: 'config', Setup: true, Reset: true });
    await advanceTime(500);
    expect(updateSettings).toHaveBeenCalledTimes(1);
  });

  it('saves a directory edit when navigation occurs during validation', async function navigateDuringValidation() {
    await renderSettings();
    let resolveValidation!: (exists: boolean) => void;
    const validationPromise = new Promise<boolean>(function waitForValidation(resolve) {
      resolveValidation = resolve;
    });
    vi.mocked(directoryExists).mockReturnValueOnce(validationPromise);
    const directoryField = screen.getByLabelText(appstrings.configuredTrashPath);
    fireEvent.change(directoryField, { target: { value: '/valid' } });
    fireEvent.blur(directoryField);
    cleanup();
    await act(async function completeValidation() {
      resolveValidation(true);
    });
    expect(updateSettings).toHaveBeenCalledWith({ trashPath: '/valid' });
  });

  it('displays reloaded settings after reset', async function displayResetValues() {
    const mountedSettings = render(
      <MantineProvider env="test">
        <Configuration revision={0} busy={false} perform={perform} />
      </MantineProvider>,
    );
    await advanceTime(0);
    fireEvent.change(screen.getByLabelText(appstrings.retention), { target: { value: '42' } });
    await advanceTime(250);
    vi.mocked(runOperation).mockResolvedValue({
      result: {
        config: {
          ...storedSettings,
          trashPath: '/reset',
          retainPeriod: 7,
          requireReview: ['reset rule'],
        },
      },
      error: '',
    });
    mountedSettings.rerender(
      <MantineProvider env="test">
        <Configuration revision={1} busy={false} perform={perform} />
      </MantineProvider>,
    );
    await advanceTime(0);
    expect(screen.getByLabelText(appstrings.retention)).toHaveValue('7');
    expect(screen.getByLabelText(appstrings.configuredTrashPath)).toHaveValue('/reset');
    expect(screen.getByLabelText(`${appstrings.reviewRule} 1`)).toHaveValue('reset rule');
  });
});
