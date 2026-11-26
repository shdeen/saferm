import { cleanup, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';

vi.mock('./bridge/operations', function mockBridge() {
  return {
    getOperationPage: vi.fn().mockResolvedValue({ actions: [], total: 0, pending: 0 }),
    getOperationStatuses: vi.fn().mockResolvedValue({}),
    runOperation: vi.fn(),
    chooseSourcePath: vi.fn(),
    chooseDestinationPath: vi.fn(),
  };
});

vi.mock('./bridge/desktop', function mockDesktop() {
  return {
    showError: vi.fn(),
    showInformation: vi.fn(),
    takeReview: vi.fn().mockResolvedValue(0),
    onReview: vi.fn().mockReturnValue(function unsubscribe() {}),
  };
});

afterEach(function resetDocument() {
  cleanup();
  window.localStorage.clear();
  document.documentElement.removeAttribute('data-mantine-color-scheme');
  document.body.replaceChildren();
});

// Permanent: with no stored choice, the interface starts in the operating system's scheme.
it('starts in the dark scheme when the operating system is dark', async function systemScheme() {
  vi.mocked(window.matchMedia).mockImplementation(function darkSystem(query: string) {
    return {
      matches: query.includes('prefers-color-scheme: dark'),
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    } as unknown as MediaQueryList;
  });
  const root = document.createElement('div');
  root.id = 'root';
  document.body.append(root);
  await import('./main');
  await waitFor(function schemeApplied() {
    expect(document.documentElement.getAttribute('data-mantine-color-scheme')).toBe('dark');
  });
});
