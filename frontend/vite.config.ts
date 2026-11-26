import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import wails from '@wailsio/runtime/plugins/vite';

export default defineConfig({
  plugins: [react(), wails('./bindings')],
  server: { host: '127.0.0.1', port: 9246, strictPort: true },
  build: { emptyOutDir: true },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/testing/setup.ts'],
    clearMocks: true,
    // Node's own storage global is unusable without a backing file and hides the one jsdom provides.
    execArgv: ['--no-experimental-webstorage'],
    // Full component interactions can exceed five seconds on hosted runners.
    testTimeout: 15_000,
  },
});
