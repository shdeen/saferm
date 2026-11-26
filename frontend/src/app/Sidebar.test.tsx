import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { Sidebar } from './Sidebar';
import { appstrings } from '../shared/text';

function ignoreSidebarCommand() {}

function renderSidebar() {
  return render(
    <MantineProvider env="test">
      <Sidebar
        section="operations"
        onNavigate={ignoreSidebarCommand}
        onCleanup={ignoreSidebarCommand}
        busy={false}
      />
    </MantineProvider>,
  );
}

function appliedColorScheme() {
  return document.documentElement.getAttribute('data-mantine-color-scheme');
}

function logoFiles(sidebar: HTMLElement) {
  return [...sidebar.querySelectorAll('picture source, picture img')].map(function logoFile(image) {
    return image.getAttribute('srcset') ?? image.getAttribute('src');
  });
}

afterEach(function resetColorScheme() {
  cleanup();
  window.localStorage.clear();
  document.documentElement.removeAttribute('data-mantine-color-scheme');
});

describe('Sidebar color scheme', function colorSchemeTests() {
  // Permanent: the sidebar control switches the interface to dark and back to light.
  it('switches between the light and dark schemes', async function switchScheme() {
    renderSidebar();
    expect(appliedColorScheme()).toBe('light');
    await userEvent.click(screen.getByRole('button', { name: appstrings.darkMode }));
    expect(appliedColorScheme()).toBe('dark');
    await userEvent.click(screen.getByRole('button', { name: appstrings.lightMode }));
    expect(appliedColorScheme()).toBe('light');
  });

  // Permanent: a chosen scheme is applied when the interface next starts.
  it('applies the chosen scheme on the next start', async function storedScheme() {
    const firstStart = renderSidebar();
    await userEvent.click(screen.getByRole('button', { name: appstrings.darkMode }));
    firstStart.unmount();
    document.documentElement.removeAttribute('data-mantine-color-scheme');
    renderSidebar();
    expect(appliedColorScheme()).toBe('dark');
    expect(screen.getByRole('button', { name: appstrings.lightMode })).toBeVisible();
  });
});

describe('Sidebar logo', function logoTests() {
  // Permanent: the logo is an image named for the application, with separate files per scheme.
  it('shows a different logo file in each scheme', async function schemeLogo() {
    const sidebar = renderSidebar().container;
    expect(screen.getByRole('img', { name: appstrings.appName })).toBeVisible();
    const lightFiles = logoFiles(sidebar);
    expect(lightFiles).toHaveLength(2);
    await userEvent.click(screen.getByRole('button', { name: appstrings.darkMode }));
    const darkFiles = logoFiles(sidebar);
    expect(darkFiles).toHaveLength(2);
    expect(darkFiles[0]).not.toBe(lightFiles[0]);
    expect(darkFiles[1]).not.toBe(lightFiles[1]);
  });
});
