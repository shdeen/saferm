import { useComputedColorScheme, useMantineColorScheme } from '@mantine/core';
import { CircleHelp, History, ListChecks, Moon, Settings, Sun } from 'lucide-react';
import birdForDarkScheme from '../assets/saferm-icon-small-dark.svg';
import birdForLightScheme from '../assets/saferm-icon-small-light.svg';
import logoForDarkScheme from '../assets/saferm-logo-horizontal-dark.svg';
import logoForLightScheme from '../assets/saferm-logo-horizontal-light.svg';
import { appstrings } from '../shared/text';

/** Media query for the window widths at which the stylesheet collapses the sidebar to icons. */
const collapsedSidebarQuery = '(max-width: 850px)';

/**
 * For each color scheme: the full logo, the bird icon that replaces it in the collapsed
 * sidebar, and the scheme, label, and icon of the control that switches to the opposite scheme.
 */
const colorSchemes = {
  light: {
    logo: logoForLightScheme,
    bird: birdForLightScheme,
    oppositeScheme: 'dark',
    oppositeLabel: appstrings.darkMode,
    OppositeIcon: Moon,
  },
  dark: {
    logo: logoForDarkScheme,
    bird: birdForDarkScheme,
    oppositeScheme: 'light',
    oppositeLabel: appstrings.lightMode,
    OppositeIcon: Sun,
  },
} as const;

/**
 * Shows the logo, the section navigation, the command that cleans up history, and the
 * control that switches the color scheme.
 */
export function Sidebar({
  section,
  onNavigate,
  onCleanup,
  busy,
}: {
  /** The section on display: operations, help, or settings. */
  section: string;
  onNavigate: (section: string) => void;
  onCleanup: () => void;
  busy: boolean;
}) {
  const { setColorScheme } = useMantineColorScheme();
  const colorScheme =
    colorSchemes[useComputedColorScheme('light', { getInitialValueInEffect: false })];
  return (
    <aside className="sidebar">
      <picture className="brand">
        <source media={collapsedSidebarQuery} srcSet={colorScheme.bird} />
        <img src={colorScheme.logo} alt={appstrings.appName} />
      </picture>
      <nav aria-label={appstrings.appName} className="main-nav">
        <button
          className={section === 'operations' ? 'nav-item active' : 'nav-item'}
          aria-current={section === 'operations' ? 'page' : undefined}
          onClick={function operations() {
            onNavigate('operations');
          }}
        >
          <ListChecks size={18} />
          {appstrings.operations}
        </button>
      </nav>
      <div className="sidebar-bottom">
        <button className="nav-item" onClick={onCleanup} disabled={busy}>
          <History size={18} />
          {appstrings.maintenance}
        </button>
        <button
          className={section === 'help' ? 'nav-item active' : 'nav-item'}
          aria-current={section === 'help' ? 'page' : undefined}
          onClick={function help() {
            onNavigate('help');
          }}
        >
          <CircleHelp size={18} />
          {appstrings.help}
        </button>
        <button
          className="nav-item"
          onClick={function switchColorScheme() {
            setColorScheme(colorScheme.oppositeScheme);
          }}
        >
          <colorScheme.OppositeIcon size={18} />
          {colorScheme.oppositeLabel}
        </button>
        <button
          className={section === 'settings' ? 'nav-item active' : 'nav-item'}
          aria-current={section === 'settings' ? 'page' : undefined}
          onClick={function settings() {
            onNavigate('settings');
          }}
        >
          <Settings size={18} />
          {appstrings.settings}
        </button>
      </div>
    </aside>
  );
}
