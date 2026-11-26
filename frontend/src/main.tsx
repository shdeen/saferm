import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { MantineProvider, createTheme } from '@mantine/core';
import '@mantine/core/styles.css';
import './styles/app.css';
import { App } from './app/App';

const theme = createTheme({
  primaryColor: 'blue',
  primaryShade: 7,
  fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  headings: { fontFamily: 'inherit', fontWeight: '650' },
  defaultRadius: 'md',
  colors: {
    blue: [
      '#edf3ff',
      '#dce7fb',
      '#b7cef4',
      '#8eb1ec',
      '#6a97e5',
      '#5286e1',
      '#447cde',
      '#326ac5',
      '#285eb1',
      '#1d509c',
    ],
    // Navy shades anchored on the logo's dark tile; Mantine reads its dark surfaces from these.
    dark: [
      '#e3e9f2',
      '#c5d3e6',
      '#9aa7bc',
      '#6f7c94',
      '#33405a',
      '#283449',
      '#1f2a3f',
      '#161f30',
      '#121a29',
      '#0d1420',
    ],
  },
});

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <MantineProvider theme={theme} defaultColorScheme="auto">
      <App />
    </MantineProvider>
  </StrictMode>,
);
